package relay

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

func TestPelicanLargeReasoningStreamKeepsSmallArtwork(t *testing.T) {
	for _, protocol := range []string{"chat", "responses", "json"} {
		t.Run(protocol, func(t *testing.T) {
			reasoning := strings.Repeat("r", 1024)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if protocol == "json" {
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprintf(w, `{"status":"completed","reasoning_content":"%s","output":[{"content":[{"type":"output_text","text":"<svg></svg>"}]}]}`, strings.Repeat("r", 5<<20))
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				for index := 0; index < 5000; index++ {
					if protocol == "chat" {
						fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":\"%s\"}}]}\n\n", reasoning)
					} else {
						fmt.Fprintf(w, "data: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"%s\"}\n\n", reasoning)
					}
				}
				if protocol == "chat" {
					fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"<svg></svg>\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				} else {
					fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"<svg></svg>\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
				}
			}))
			defer server.Close()
			target := pelicanTarget{client: *server.Client(), endpoint: server.URL, model: "deepseek-v4.1-flash", headers: make(http.Header), chat: protocol == "chat"}
			got, err := target.generate(context.Background(), storage.PelicanConfig{Prompt: "draw", ReasoningEffort: "medium"})
			if err != nil || got != "<svg></svg>" {
				t.Fatalf("artwork=%q err=%v", got, err)
			}
		})
	}
}

func TestPelicanUnconfiguredModelsNeverGenerate(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/admin/accounts/42/models" {
			t.Errorf("unexpected generation request: %s %s", r.Method, r.URL.Path)
		}
		fmt.Fprint(w, `{"code":0,"data":["codex-auto-review","other-model"]}`)
	}))
	defer server.Close()
	s := NewService(nil, nil)
	_, err := s.generatePelican(context.Background(), storage.RelayStation{BaseURL: server.URL}, "admin-secret", storage.PelicanConfig{AccountExternalID: 42, Model: "gpt-6.1-sol,gpt-5.6-sol,deepseek-v4.1-flash"})
	if err == nil || requests != 1 {
		t.Fatalf("requests=%d err=%v", requests, err)
	}
}

func TestPelicanResponseKeepsOutputAndEventBounds(t *testing.T) {
	for _, tc := range []struct {
		name, body, wantError string
	}{
		{"chat_output", `{"choices":[{"message":{"content":"` + strings.Repeat("x", pelicanMaxOutputBytes+1) + `"},"finish_reason":"stop"}]}`, "4 MiB"},
		{"responses_output", `{"status":"completed","output":[{"content":[{"type":"output_text","text":"` + strings.Repeat("x", pelicanMaxOutputBytes+1) + `"}]}]}`, "4 MiB"},
		{"json_event", `{"reasoning_content":"` + strings.Repeat("r", pelicanMaxEventBytes) + `"}`, "16 MiB"},
		{"sse_event", "data: " + strings.Repeat("r", pelicanMaxEventBytes) + "\n\n", "token too long"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := readPelicanResponse(strings.NewReader(tc.body))
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("err=%v want=%s", err, tc.wantError)
			}
		})
	}
}

func TestPelicanRepeatedSnapshotsDoNotAccumulateOutput(t *testing.T) {
	artwork := "<svg>" + strings.Repeat(" ", 64<<10) + "</svg>"
	event := fmt.Sprintf("data: {\"output\":[{\"content\":[{\"type\":\"output_text\",\"text\":\"%s\"}]}]}\n\n", artwork)
	body := strings.Repeat(event, 80) + "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n"
	got, err := readPelicanResponse(strings.NewReader(body))
	if err != nil || got != artwork {
		t.Fatalf("artwork_bytes=%d err=%v", len(got), err)
	}
}
