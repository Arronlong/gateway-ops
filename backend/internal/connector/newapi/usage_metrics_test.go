package newapi

import (
	"context"
	"encoding/json"
	"github.com/guliansheng/gateway-ops/internal/connector"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUsageIncludesSuccessAndFailureWithoutAccountingLogs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/status" {
			w.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
			return
		}
		if r.URL.Query().Get("type") != "0" {
			t.Errorf("type = %s, want all logs", r.URL.Query().Get("type"))
		}
		items := []map[string]any{
			{"id": 1, "type": 2, "created_at": 1770000000, "request_id": "request-1", "model_name": "gpt-5", "other": `{"cache_tokens":0}`},
			{"id": 2, "type": 5, "created_at": 1770000001, "request_id": "request-1", "model_name": "gpt-5", "other": "{}"},
			{"id": 3, "type": 1, "created_at": 1770000002, "other": "{}"},
			{"id": 4, "type": 2, "created_at": 1770000003, "request_id": "request-2", "model_name": "gpt-5", "other": `{"cache_tokens":120,"cache_creation_tokens":10}`},
		}
		json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"items": items, "page": 1, "page_size": 100, "total": 4}})
	}))
	defer server.Close()
	page, err := New().ListUsage(context.Background(), &connector.Channel{SiteURL: server.URL}, &connector.AuthSession{Headers: map[string]string{"Authorization": "Bearer test-token"}}, connector.UsageQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if page.RawCount != 4 {
		t.Fatalf("raw page count = %d, want 4 despite filtered accounting logs", page.RawCount)
	}
	if page.Records[0].RemoteID == page.Records[1].RemoteID || page.Records[0].RequestID != page.Records[1].RequestID {
		t.Fatalf("retry attempts lost distinct log identities: %#v", page.Records)
	}
	if len(page.Records) != 3 {
		t.Fatalf("records = %d, want 3", len(page.Records))
	}
	if !page.Records[0].SuccessKnown || !page.Records[0].Success || !page.Records[1].SuccessKnown || page.Records[1].Success {
		t.Fatalf("success states = %#v", page.Records)
	}
}
func TestUsageCacheMetadataDistinguishesMissingAndZero(t *testing.T) {
	read, write, known := cacheTokensFromOther(`{"cache_tokens":0}`)
	if read != 0 || write != 0 || !known {
		t.Fatalf("zero cache = %d %d %v", read, write, known)
	}
	_, _, known = cacheTokensFromOther(`{"frt":12}`)
	if known {
		t.Fatal("missing cache must be unknown")
	}
	read, write, known = cacheTokensFromOther(`{"cache_tokens":90,"cache_creation_tokens_5m":10,"cache_creation_tokens_1h":20}`)
	if read != 90 || write != 30 || !known {
		t.Fatalf("cache = %d %d %v", read, write, known)
	}
}
