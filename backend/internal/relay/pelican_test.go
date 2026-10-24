package relay

import (
	"context"
	"encoding/json"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPelicanGenerationUsesAccountCredentialsAndFullPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/admin/accounts/42/models" {
			w.Write([]byte(`{"code":0,"data":["test-model"]}`))
			return
		}
		if r.URL.Path == "/api/v1/admin/accounts/data" {
			w.Write([]byte(`{"code":0,"data":{"accounts":[{"platform":"openai","type":"apikey","credentials":{"base_url":"http://unused.invalid","api_key":"upstream-secret"}}]}}`))
			return
		}
		if r.URL.Path != "/v1/responses" {
			t.Errorf("unexpected route %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer upstream-secret" || r.Header.Get("x-api-key") != "" {
			t.Error("incorrect upstream credentials")
		}
		var input struct {
			Model string `json:"model"`
			Input []struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"input"`
			Reasoning struct {
				Effort string `json:"effort"`
			} `json:"reasoning"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		if input.Model != "test-model" || input.Reasoning.Effort != "high" || len(input.Input) != 1 || len(input.Input[0].Content) != 1 {
			t.Errorf("input=%#v", input)
			return
		}
		prompt := input.Input[0].Content[0].Text
		if !strings.HasPrefix(prompt, "custom drawing") || !strings.Contains(prompt, "直接返回独立 HTML") || !strings.Contains(prompt, "只输出 HTML") {
			t.Errorf("HTML-only contract missing: %q", prompt)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(`data: {"type":"response.output_text.delta","delta":"<svg></svg>"}` + "\n\n" + `data: {"type":"response.completed","response":{"status":"completed"}}` + "\n\n"))
	}))
	defer server.Close()
	service := NewService(nil, nil)
	service.client.Transport = pelicanRewriteTransport{base: server.Client().Transport, target: server.URL}
	html, err := service.generatePelican(context.Background(), storage.RelayStation{BaseURL: server.URL}, "admin-secret", storage.PelicanConfig{AccountExternalID: 42, Model: "test-model", Prompt: "custom drawing", ReasoningEffort: "high"})
	if err != nil || html != "<svg></svg>" {
		t.Fatalf("html=%q err=%v", html, err)
	}
}

func TestPelicanExecutionUsesGlobalSettingsWithoutChangingAccountState(t *testing.T) {
	lease := time.Now().Add(time.Minute)
	next := time.Now().Add(time.Hour)
	config := storage.PelicanConfig{ID: 7, RelayStationID: 9, AccountExternalID: 42, Enabled: false, Model: "legacy", Prompt: "legacy", ParallelCount: 8, CronExpression: "0 9 * * *", NextRunAt: next, LeaseUntil: &lease}
	settings := storage.PelicanSettings{Model: "global-model", Prompt: "global-prompt", ReasoningEffort: "high", ParallelCount: 2, CronExpression: "*/15 * * * *"}
	applyPelicanSettings(&config, settings)
	if config.Model != settings.Model || config.Prompt != settings.Prompt || config.ReasoningEffort != settings.ReasoningEffort || config.ParallelCount != 1 || config.CronExpression != settings.CronExpression {
		t.Fatalf("execution must use global settings, got %#v", config)
	}
	if config.ID != 7 || config.RelayStationID != 9 || config.AccountExternalID != 42 || config.Enabled || !config.NextRunAt.Equal(next) || config.LeaseUntil != &lease {
		t.Fatalf("account state must not change: %#v", config)
	}
}
