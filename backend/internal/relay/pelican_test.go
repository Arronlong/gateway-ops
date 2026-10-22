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

func TestPelicanGenerationUsesStandardAccountTestEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/accounts/42/test" {
			t.Errorf("unexpected route %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "admin-secret" {
			t.Error("missing admin key")
		}
		var input struct {
			ModelID string `json:"model_id"`
			Prompt  string `json:"prompt"`
			Mode    string `json:"mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		if input.ModelID != "test-model" || input.Mode != "default" || !strings.HasPrefix(input.Prompt, "custom drawing") {
			t.Errorf("input=%#v", input)
		}
		if !strings.Contains(input.Prompt, "\u76f4\u63a5\u8fd4\u56de\u72ec\u7acb HTML") || !strings.Contains(input.Prompt, "\u53ea\u8f93\u51fa HTML") {
			t.Errorf("HTML-only contract missing: %q", input.Prompt)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(`data: {"type":"content","text":"<svg></svg>"}` + "\n\n" + `data: {"type":"test_complete","success":true}` + "\n"))
	}))
	defer server.Close()
	service := NewService(nil, nil)
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
	if config.Model != settings.Model || config.Prompt != settings.Prompt || config.ReasoningEffort != settings.ReasoningEffort || config.ParallelCount != settings.ParallelCount || config.CronExpression != settings.CronExpression {
		t.Fatalf("execution must use global settings, got %#v", config)
	}
	if config.ID != 7 || config.RelayStationID != 9 || config.AccountExternalID != 42 || config.Enabled || !config.NextRunAt.Equal(next) || config.LeaseUntil != &lease {
		t.Fatalf("account state must not change: %#v", config)
	}
}
func TestPelicanStreamRequiresCompletionAndCompleteArtwork(t *testing.T) {
	cases := []struct {
		body string
		good bool
	}{
		{`data: {"type":"content","text":"<html><body><svg></svg></body></html>"}` + "\n" + `data: {"type":"test_complete","success":true}`, true},
		{`data: {"type":"content","text":"<svg><circle/></svg>"}` + "\n" + `data: {"type":"test_complete","success":true}`, true},
		{`data: {"type":"content","text":"<svg></svg>"}`, false},
		{`data: {"type":"content","text":"<svg></svg>"}` + "\n" + `data: {"type":"test_complete","success":false}`, false},
		{`data: {"type":"error","error":"generation failed"}`, false},
		{`data: {"type":"content","text":"<html>unfinished"}` + "\n" + `data: {"type":"test_complete","success":true}`, false},
		{`data: {"type":"content","text":"Hello"}` + "\n" + `data: {"type":"test_complete","success":true}`, false},
	}
	for _, tc := range cases {
		html, err := parsePelicanStream(tc.body)
		if (err == nil) != tc.good || (tc.good && html == "") {
			t.Fatalf("parse(%q)=%q,%v", tc.body, html, err)
		}
	}
}
