package tasklog

import (
	"context"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func TestRecorderRedactsCredentials(t *testing.T) {
	for _, text := range []string{`{"client_secret":"demo-sensitive-value"}`, `{"clientSecret":"demo-sensitive-value"}`, `{"admin_api_key":"demo-sensitive-value"}`, `{"token":"demo-sensitive-value"}`, `{"token":"very-secret-value"}`, `Authorization: Basic very-secret-value`, `Cookie: sid=very-secret-value; session=other`, `Set-Cookie: sid=very-secret-value; HttpOnly`, `upstream: {\"api_key\":\"very-secret-value\"}`, `{"headers":{"authorization":"Basic very-secret-value"}}`, `Authorization: Bearer very-secret-value`, `{"api_key":"very-secret-value"}`, `https://user:very-secret-value@example.test/path`, `https://example.test?access_token=very-secret-value&foo=bar`, `password='very-secret-value'`} {
		if strings.Contains(Redact(text), "very-secret-value") || strings.Contains(Redact(text), "demo-sensitive-value") {
			t.Errorf("credential leaked: %s", Redact(text))
		}
	}
}
func TestRecorderIsolationConcurrencyAndTruncation(t *testing.T) {
	ctx, recorder := WithRecorder(context.Background())
	other, unrelated := WithRecorder(context.Background())
	var jobs sync.WaitGroup
	for index := 0; index < 20; index++ {
		jobs.Add(1)
		go func() { defer jobs.Done(); Add(ctx, "INFO", "%s", strings.Repeat("\u4e2d", 2000)) }()
	}
	jobs.Wait()
	Add(ctx, "ERROR", "late failure")
	Add(other, "INFO", "unrelated")
	text, failed := recorder.Snapshot()
	if len(text) > 66000 || !utf8.ValidString(text) || !failed || strings.Contains(text, "unrelated") {
		t.Fatalf("invalid bounded recorder: size=%d failed=%v", len(text), failed)
	}
	text, failed = unrelated.Snapshot()
	if failed || !strings.Contains(text, "unrelated") {
		t.Fatal("recorders not isolated")
	}
}
