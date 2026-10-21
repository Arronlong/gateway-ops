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

func TestPelicanGenerationUsesDedicatedAccountEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/accounts/42/pelican-test" {
			t.Errorf("unexpected route %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "admin-secret" {
			t.Error("missing admin key")
		}
		var input map[string]string
		json.NewDecoder(r.Body).Decode(&input)
		if input["model_id"] != "test-model" || input["reasoning_effort"] != "high" || !strings.HasPrefix(input["prompt"], "custom drawing") {
			t.Errorf("input=%#v", input)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"type\":\"content\",\"text\":\"<svg><text>" + strings.Repeat("x", 12000) + "</text></svg>\"}\n\ndata: {\"type\":\"test_complete\",\"success\":true}\n"))
	}))
	defer server.Close()
	service := NewService(nil, nil)
	html, err := service.generatePelican(context.Background(), storage.RelayStation{BaseURL: server.URL}, "admin-secret", storage.PelicanConfig{AccountExternalID: 42, Model: "test-model", Prompt: "custom drawing", ReasoningEffort: "high"})
	if err != nil || len(html) < 12000 {
		t.Fatalf("artwork was truncated: %d,%v", len(html), err)
	}
}
func TestPelicanMissingEndpointDoesNotFallbackToGroup(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(404) }))
	defer server.Close()
	service := NewService(nil, nil)
	if _, err := service.generatePelican(context.Background(), storage.RelayStation{BaseURL: server.URL}, "key", storage.PelicanConfig{AccountExternalID: 42}); err == nil || !strings.Contains(err.Error(), "pelican-test") {
		t.Fatalf("error=%v", err)
	}
	if calls != 1 {
		t.Fatalf("fallback sent %d requests", calls)
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
