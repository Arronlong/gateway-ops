package relay

import (
	"github.com/guliansheng/gateway-ops/internal/storage"
	"strings"
	"testing"
)

func TestAccountTestCallDetectsStreamFailureDespiteHTTP200(t *testing.T) {
	cases := []struct {
		output  string
		status  int
		success bool
	}{
		{`data: {"type":"test_complete","success":false}`, 200, false},
		{`data: {"type":"test_complete","success":true}`, 200, true},
		{`data: {"type":"error","message":"failed"}`, 200, false},
		{`data: {"type":"content","text":"partial"}`, 200, false},
		{`{"success":false,"message":"failed"}`, 200, false},
		{`{"success":true}`, 200, true},
		{`OK`, 200, true},
		{``, 200, false},
		{`failed`, 500, false},
	}
	for _, tc := range cases {
		call := accountHistoryCall(AccountTestResult{StatusCode: tc.status, DurationMS: 12, Output: tc.output}, nil)
		if call.Success != tc.success {
			t.Errorf("success for %q = %v, want %v", tc.output, call.Success, tc.success)
		}
	}
}
func TestTestHistoryRedactsSecretsAndLimitsOutput(t *testing.T) {
	text := redactTestText("Authorization: Bearer sk-secret-value\nx-api-key: test-admin-key\nsecret="+"test-admin-key", "test-admin-key")
	if len([]rune(redactTestText(strings.Repeat("a", 10000)))) > 8230 {
		t.Fatal("output limit not applied")
	}
	if text == "" || containsSecret(text, "sk-secret-value") || containsSecret(text, "test-admin-key") {
		t.Fatalf("secrets in %q", text)
	}
}
func containsSecret(text, secret string) bool {
	for i := 0; i+len(secret) <= len(text); i++ {
		if text[i:i+len(secret)] == secret {
			return true
		}
	}
	return false
}
func TestTestHistoryRedactsNestedJSONAndSSE(t *testing.T) {
	cases := []string{
		`data: {"type":"error","message":"{\"api_key\":\"sk-upstream-secret\"}"}`,
		`{"details":{"access_token":"sk-upstream-secret"}}`,
		`{"message":"{\"message\":\"{\\\"x-api-key\\\":\\\"sk-upstream-secret\\\"}\"}"}`,
		`{"items":[{"Authorization":"Bearer sk-upstream-secret"}]}`,
	}
	for _, text := range cases {
		redacted := redactTestText(text)
		if strings.Contains(redacted, "sk-upstream-secret") {
			t.Errorf("nested secret remained in %q", redacted)
		}
	}
}

func TestHistorySummaryCountsActualCalls(t *testing.T) {
	run := storage.RelayTestRun{Requested: 10, Results: []storage.RelayTestCall{{Success: true}, {Success: false}}}
	summarizeTestRun(&run)
	if run.Succeeded != 1 || run.Failed != 1 || run.Requested != 10 {
		t.Fatalf("summary = %#v", run)
	}
}
