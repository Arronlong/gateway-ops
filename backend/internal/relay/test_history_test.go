package relay

import (
	"strings"
	"testing"
)

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
