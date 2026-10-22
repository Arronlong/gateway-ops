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

func TestPelicanConfigurationDefaultsAndSchedule(t *testing.T) {
	now := time.Date(2026, 10, 15, 0, 59, 0, 0, time.UTC)
	config := storage.PelicanConfig{Model: "gpt-5"}
	if err := normalizePelicanConfig(&config, now); err != nil {
		t.Fatal(err)
	}
	if config.Enabled || config.ParallelCount != 1 || config.Prompt != DefaultPelicanPrompt || config.ReasoningEffort != "medium" {
		t.Fatalf("defaults = %#v", config)
	}
	config.CronExpression = "0 9 * * *"
	if err := normalizePelicanConfig(&config, now); err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 10, 15, 1, 0, 0, 0, time.UTC); !config.NextRunAt.Equal(want) {
		t.Fatalf("next = %v", config.NextRunAt)
	}
	for _, cron := range []string{"* * * * * *", "invalid", "0 0 31 2 *"} {
		config.CronExpression = cron
		if normalizePelicanConfig(&config, now) == nil {
			t.Fatalf("accepted %q", cron)
		}
	}
}
func TestPelicanConfigurationRejectsInvalidOptions(t *testing.T) {
	cases := []storage.PelicanConfig{{Model: ""}, {Model: "gpt", ParallelCount: 9}, {Model: "gpt", ParallelCount: -1}, {Model: "gpt", ReasoningEffort: "bogus"}, {Model: "gpt", Prompt: strings.Repeat("a", 32001)}}
	for _, config := range cases {
		if normalizePelicanConfig(&config, time.Now()) == nil {
			t.Fatalf("accepted %#v", config)
		}
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
