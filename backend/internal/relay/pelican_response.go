package relay

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const pelicanMaxEventBytes = 16 << 20
const pelicanMaxOutputBytes = 4 << 20

type pelicanNativeResponse struct {
	Type     string          `json:"type"`
	Status   string          `json:"status"`
	Delta    string          `json:"delta"`
	Text     json.RawMessage `json:"text"`
	Error    json.RawMessage `json:"error"`
	Response json.RawMessage `json:"response"`
	Output   []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Content json.RawMessage `json:"content"`
		} `json:"delta"`
		Message struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

func pelicanNativeError(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var object struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &object) == nil && object.Message != "" {
		return object.Message
	}
	return "上游报告生成失败"
}

func pelicanMessageText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var out strings.Builder
	for _, part := range parts {
		if part.Type == "text" || part.Type == "output_text" {
			out.WriteString(part.Text)
		}
	}
	return out.String()
}

func parsePelicanResponse(data []byte) (string, error) {
	return readPelicanResponse(bytes.NewReader(data))
}

func readPelicanResponse(body io.Reader) (string, error) {
	var output strings.Builder
	var final string
	completed := false
	consume := func(raw []byte) error {
		if strings.TrimSpace(string(raw)) == "[DONE]" {
			return nil
		}
		var event pelicanNativeResponse
		if err := json.Unmarshal(raw, &event); err != nil {
			return fmt.Errorf("解析模型生成响应失败: %w", err)
		}
		if message := pelicanNativeError(event.Error); message != "" {
			return errors.New(message)
		}
		if len(event.Response) > 0 {
			var response pelicanNativeResponse
			if err := json.Unmarshal(event.Response, &response); err != nil {
				return err
			}
			if message := pelicanNativeError(response.Error); message != "" {
				return errors.New(message)
			}
			event.Status, event.Output = response.Status, response.Output
		}
		if event.Status == "failed" || event.Status == "incomplete" || event.Status == "cancelled" || event.Type == "response.failed" || event.Type == "response.incomplete" || event.Type == "error" {
			return errors.New("模型生成失败或输出被截断，不能保存为成功作品")
		}
		if event.Type == "response.output_text.delta" {
			if output.Len()+len(event.Delta) > pelicanMaxOutputBytes {
				return errors.New("\u6a21\u578b\u6b63\u6587\u8d85\u8fc7 4 MiB \u4e0a\u9650")
			}
			output.WriteString(event.Delta)
		}
		if event.Type == "response.output_text.done" && output.Len() == 0 {
			final = pelicanMessageText(event.Text)
		}
		if len(event.Output) > 0 {
			var snapshot strings.Builder
			for _, item := range event.Output {
				for _, part := range item.Content {
					if part.Type == "output_text" {
						if snapshot.Len()+len(part.Text) > pelicanMaxOutputBytes {
							return errors.New("\u6a21\u578b\u6b63\u6587\u8d85\u8fc7 4 MiB \u4e0a\u9650")
						}
						snapshot.WriteString(part.Text)
					}
				}
			}
			if snapshot.Len() > 0 {
				final = snapshot.String()
			}
		}
		if event.Type == "response.completed" || event.Status == "completed" {
			completed = true
		}
		for _, choice := range event.Choices {
			if choice.Index != 0 {
				continue
			}
			text := pelicanMessageText(choice.Delta.Content)
			if output.Len()+len(text) > pelicanMaxOutputBytes {
				return errors.New("\u6a21\u578b\u6b63\u6587\u8d85\u8fc7 4 MiB \u4e0a\u9650")
			}
			output.WriteString(text)
			if text := pelicanMessageText(choice.Message.Content); text != "" {
				final = text
			}
			if choice.FinishReason != nil {
				if *choice.FinishReason != "stop" {
					return fmt.Errorf("模型未正常完成作品（finish_reason=%s）", *choice.FinishReason)
				}
				completed = true
			}
		}
		if len(final) > pelicanMaxOutputBytes {
			return errors.New("\u6a21\u578b\u6b63\u6587\u8d85\u8fc7 4 MiB \u4e0a\u9650")
		}
		return nil
	}
	reader := bufio.NewReader(body)
	for {
		first, err := reader.Peek(1)
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if !strings.ContainsRune(" \r\n\t", rune(first[0])) {
			break
		}
		_, _ = reader.Discard(1)
	}
	first, _ := reader.Peek(1)
	if len(first) > 0 && first[0] == '{' {
		data, err := io.ReadAll(io.LimitReader(reader, pelicanMaxEventBytes+1))
		if err != nil {
			return "", err
		}
		if len(data) > pelicanMaxEventBytes {
			return "", errors.New("\u4e0a\u6e38\u5355\u6761 JSON \u54cd\u5e94\u8d85\u8fc7 16 MiB")
		}
		if err := consume(data); err != nil {
			return "", err
		}
	} else {
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 4096), pelicanMaxEventBytes)
		var eventData []string
		eventBytes := 0
		flush := func() error {
			if len(eventData) == 0 {
				return nil
			}
			raw := strings.Join(eventData, "\n")
			eventData = nil
			eventBytes = 0
			return consume([]byte(raw))
		}
		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) == "" {
				if err := flush(); err != nil {
					return "", err
				}
				continue
			}
			if strings.HasPrefix(line, "data:") {
				data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				eventBytes += len(data) + 1
				if eventBytes > pelicanMaxEventBytes {
					return "", errors.New("\u4e0a\u6e38\u5355\u4e2a\u6d41\u4e8b\u4ef6\u8d85\u8fc7 16 MiB")
				}
				eventData = append(eventData, data)
			}
		}
		if scanner.Err() != nil {
			return "", scanner.Err()
		}
		if err := flush(); err != nil {
			return "", err
		}
	}
	if !completed {
		return "", errors.New("生成流未完整结束，不能保存为成功作品")
	}
	if final == "" {
		final = output.String()
	}
	return extractPelicanArtwork(final)
}
