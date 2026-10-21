package relay

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"github.com/robfig/cron/v3"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	_ "time/tzdata"
)

const DefaultPelicanPrompt = "创建一个 HTML，内容是 SVG 绘制一个鹈鹕骑自行车的 2D 动画，你不需要任何测试。"
const pelicanContract = "\n\n直接返回独立 HTML，不使用 Markdown 代码块或外部依赖。只输出 HTML，不要解释。"
const pelicanTimeout = 10 * time.Minute

var ErrPelicanRunning = errors.New("该账号正在进行鹈鹕测智")
var ErrPelicanUnsupportedStation = errors.New("鹈鹕测智仅支持 Sub2API 中转站")
var pelicanStart = regexp.MustCompile(`(?i)<(?:!doctype\s+html|html|svg)[\s>]`)

func validatePelicanStation(station *storage.RelayStation) error {
	if station != nil && station.Type == string(storage.ChannelTypeNewAPI) {
		return ErrPelicanUnsupportedStation
	}
	return nil
}

func nextPelicanRun(expression string, now time.Time) (time.Time, error) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse("CRON_TZ=Asia/Shanghai " + expression)
	if err != nil {
		return time.Time{}, fmt.Errorf("频率必须是有效的五段 Cron: %w", err)
	}
	next := schedule.Next(now)
	if next.IsZero() {
		return time.Time{}, errors.New("频率没有可执行的下次时间")
	}
	return next.UTC(), nil
}
func normalizePelicanConfig(config *storage.PelicanConfig, now time.Time) error {
	config.Model = strings.TrimSpace(config.Model)
	config.Prompt = strings.TrimSpace(config.Prompt)
	if config.Model == "" || len(config.Model) > 256 {
		return errors.New("请填写有效模型，最长 256 字节")
	}
	if config.Prompt == "" {
		config.Prompt = DefaultPelicanPrompt
	}
	if len(config.Prompt) > 32000 {
		return errors.New("题目最长 32000 字节")
	}
	config.ReasoningEffort = strings.ToLower(strings.TrimSpace(config.ReasoningEffort))
	if config.ReasoningEffort == "" {
		config.ReasoningEffort = "medium"
	}
	switch config.ReasoningEffort {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra":
	default:
		return errors.New("思考强度无效")
	}
	if config.ParallelCount == 0 {
		config.ParallelCount = 1
	}
	if config.ParallelCount < 1 || config.ParallelCount > 8 {
		return errors.New("并行份数必须是 1 到 8")
	}
	config.CronExpression = strings.TrimSpace(config.CronExpression)
	if config.CronExpression == "" {
		config.CronExpression = "*/30 * * * *"
	}
	if len(config.CronExpression) > 128 {
		return errors.New("Cron 表达式过长")
	}
	next, err := nextPelicanRun(config.CronExpression, now)
	if err != nil {
		return err
	}
	config.NextRunAt = next
	return nil
}
func extractPelicanArtwork(text string) (string, error) {
	start := pelicanStart.FindStringIndex(text)
	if start == nil {
		return "", errors.New("模型未返回 HTML/SVG 作品")
	}
	html := text[start[0]:]
	ending := "</html>"
	if strings.HasPrefix(strings.ToLower(html), "<svg") {
		ending = "</svg>"
	}
	end := strings.LastIndex(strings.ToLower(html), ending)
	if end < 0 {
		return "", errors.New("HTML/SVG 输出不完整")
	}
	html = html[:end+len(ending)]
	if len(html) > 2<<20 {
		return "", errors.New("作品超过 2 MiB 保存上限")
	}
	return html, nil
}
func parsePelicanStream(body string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(body))
	scanner.Buffer(make([]byte, 4096), 4<<20)
	var output strings.Builder
	completed := false
	var failure string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event struct {
			Type    string `json:"type"`
			Text    string `json:"text"`
			Success bool   `json:"success"`
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event) != nil {
			continue
		}
		switch event.Type {
		case "content":
			output.WriteString(event.Text)
		case "error":
			failure = event.Error
			if failure == "" {
				failure = event.Message
			}
			if failure == "" {
				failure = "生成失败"
			}
		case "test_complete":
			completed = event.Success
			if !event.Success && failure == "" {
				failure = "上游报告生成失败"
			}
		}
	}
	if scanner.Err() != nil {
		return "", scanner.Err()
	}
	if failure != "" {
		return "", errors.New(failure)
	}
	if !completed {
		return "", errors.New("生成流未完整结束，不能保存为成功作品")
	}
	return extractPelicanArtwork(output.String())
}
func (s *Service) SavePelicanConfig(stationID uint, accountID int64, config storage.PelicanConfig) (*storage.PelicanConfig, error) {
	station, err := s.stations.FindByID(stationID)
	if err != nil {
		return nil, err
	}
	if err = validatePelicanStation(station); err != nil {
		return nil, err
	}
	config.ID = 0
	config.RelayStationID = stationID
	config.AccountExternalID = accountID
	config.LeaseUntil = nil
	config.CreatedAt = time.Time{}
	config.UpdatedAt = time.Time{}
	if err := normalizePelicanConfig(&config, time.Now()); err != nil {
		return nil, err
	}
	if err := s.stations.SavePelicanConfig(&config); err != nil {
		return nil, err
	}
	return s.stations.PelicanConfig(stationID, accountID)
}
func (s *Service) generatePelican(ctx context.Context, station storage.RelayStation, key string, config storage.PelicanConfig) (string, error) {
	encoded, err := json.Marshal(map[string]any{"model_id": config.Model, "prompt": config.Prompt + pelicanContract, "reasoning_effort": config.ReasoningEffort})
	if err != nil {
		return "", err
	}
	endpoint := fmt.Sprintf("%s/api/v1/admin/accounts/%d/pelican-test", strings.TrimRight(station.BaseURL, "/"), config.AccountExternalID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(encoded)))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	client := *s.client
	client.Timeout = pelicanTimeout
	response, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil {
		return "", err
	}
	if len(data) > 4<<20 {
		return "", errors.New("上游生成响应超过 4 MiB")
	}
	if response.StatusCode == 404 || response.StatusCode == 405 {
		return "", errors.New("中转站未提供账号 pelican-test 接口，请升级至支持鹈鹕测智的 Sub2API 版本")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d: %s", response.StatusCode, redactTestText(string(data), key))
	}
	html, err := parsePelicanStream(string(data))
	if err != nil {
		return "", err
	}
	// 不截断作品，只移除不应回显的管理员密钥。
	if key != "" {
		html = strings.ReplaceAll(html, key, "[REDACTED]")
	}
	return html, nil
}
func (s *Service) StartPelican(ctx context.Context, stationID uint, accountID int64) (*storage.PelicanConfig, error) {
	station, err := s.stations.FindByID(stationID)
	if err != nil {
		return nil, err
	}
	if err = validatePelicanStation(station); err != nil {
		return nil, err
	}
	config, err := s.stations.PelicanConfig(stationID, accountID)
	if err != nil {
		return nil, err
	}
	if _, err = s.stations.FindAccountByExternalID(stationID, accountID); err != nil {
		return nil, err
	}
	until := time.Now().UTC().Add(15 * time.Minute).Truncate(time.Microsecond)
	claimed, err := s.stations.ClaimPelican(config.ID, time.Now(), until, config.NextRunAt, false)
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, ErrPelicanRunning
	}
	config.LeaseUntil = &until
	go s.runPelicanBatch(context.Background(), *config, until, time.Time{}, "manual")
	return config, nil
}
func (s *Service) RunDuePelicans(ctx context.Context) error {
	configs, err := s.stations.DuePelicanConfigs(time.Now())
	if err != nil {
		return err
	}
	var jobs sync.WaitGroup
	defer jobs.Wait()
	for _, config := range configs {
		if err := ctx.Err(); err != nil {
			return err
		}
		next, err := nextPelicanRun(config.CronExpression, time.Now())
		if err != nil {
			return err
		}
		until := time.Now().UTC().Add(15 * time.Minute).Truncate(time.Microsecond)
		claimed, err := s.stations.ClaimPelican(config.ID, time.Now(), until, config.NextRunAt, true)
		if err != nil {
			return err
		}
		if !claimed {
			continue
		}
		jobs.Add(1)
		go func(config storage.PelicanConfig, next time.Time) {
			defer jobs.Done()
			s.runPelicanBatch(ctx, config, until, next, "scheduled")
		}(config, next)
	}
	return nil
}
func (s *Service) savePelicanFailures(config storage.PelicanConfig, trigger, accountName string, cause error) bool {
	saved := true
	if accountName == "" {
		accountName = fmt.Sprintf("账号 #%d", config.AccountExternalID)
	}
	count := config.ParallelCount
	if count < 1 {
		count = 1
	}
	errorText := redactTestText(cause.Error(), "")
	for index := 0; index < count; index++ {
		started := time.Now().UTC()
		result := storage.PelicanResult{RelayStationID: config.RelayStationID, AccountExternalID: config.AccountExternalID, AccountName: accountName, Model: config.Model, Prompt: config.Prompt, ReasoningEffort: config.ReasoningEffort, Trigger: trigger, Status: "failed", Error: errorText, StartedAt: started, FinishedAt: started, DurationMS: 0}
		if err := s.stations.SavePelicanResult(&result); err != nil {
			saved = false
			slog.Error("save pelican failure result failed", "station", config.RelayStationID, "account", config.AccountExternalID, "error", err)
		}
	}
	return saved
}

func (s *Service) runPelicanBatch(parent context.Context, config storage.PelicanConfig, until, next time.Time, trigger string) {
	var saved atomic.Bool
	defer func() {
		if !saved.Load() {
			return
		}
		var err error
		if trigger == "scheduled" && !next.IsZero() {
			err = s.stations.CompletePelican(config.ID, until, config.NextRunAt, next)
		} else {
			err = s.stations.FinishPelican(config.ID, until)
		}
		if err != nil {
			slog.Error("finish pelican lease failed", "station", config.RelayStationID, "account", config.AccountExternalID, "error", err)
		}
	}()
	ctx, cancel := context.WithTimeout(parent, pelicanTimeout)
	defer cancel()
	station, err := s.stations.FindByID(config.RelayStationID)
	if err != nil {
		slog.Error("load pelican station failed", "station", config.RelayStationID, "account", config.AccountExternalID, "error", err)
		return
	}
	account, err := s.stations.FindAccountByExternalID(config.RelayStationID, config.AccountExternalID)
	if err != nil {
		slog.Error("load pelican account failed", "station", config.RelayStationID, "account", config.AccountExternalID, "error", err)
		saved.Store(s.savePelicanFailures(config, trigger, "", err))
		return
	}
	key, credentialErr := s.cipher.Decrypt(station.APIKeyCipher)
	saved.Store(true)
	var calls sync.WaitGroup
	for index := 0; index < config.ParallelCount; index++ {
		calls.Add(1)
		go func() {
			defer calls.Done()
			result := storage.PelicanResult{RelayStationID: config.RelayStationID, AccountExternalID: config.AccountExternalID, AccountName: account.Name, Model: config.Model, Prompt: config.Prompt, ReasoningEffort: config.ReasoningEffort, Trigger: trigger, Status: "success", StartedAt: time.Now().UTC()}
			html, err := "", credentialErr
			if err == nil {
				html, err = s.generatePelican(ctx, *station, key, config)
			}
			result.FinishedAt = time.Now().UTC()
			result.DurationMS = result.FinishedAt.Sub(result.StartedAt).Milliseconds()
			if err != nil {
				result.Status = "failed"
				result.Error = redactTestText(err.Error(), key)
			} else {
				result.HTML = html
			}
			if err := s.stations.SavePelicanResult(&result); err != nil {
				saved.Store(false)
				slog.Error("save pelican result failed", "station", config.RelayStationID, "account", config.AccountExternalID, "error", err)
			}
		}()
	}
	calls.Wait()
}
