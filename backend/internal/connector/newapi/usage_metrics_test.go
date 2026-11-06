package newapi

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestUsageIdentitySurvivesDisplayIDRenumbering(t *testing.T) {
	for _, withRequestID := range []bool{true, false} {
		t.Run(fmt.Sprint(withRequestID), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/status" {
					_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
					return
				}
				calls++
				old := map[string]any{"id": calls, "type": 2, "created_at": 1770000000, "model_name": "model", "quota": 7, "request_id": "old"}
				newer := map[string]any{"id": 1, "type": 2, "created_at": 1770000060, "model_name": "model", "quota": 11, "request_id": "new"}
				if !withRequestID {
					delete(old, "request_id")
					delete(newer, "request_id")
				}
				items := []map[string]any{old}
				if calls > 1 {
					items = []map[string]any{newer, old}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"items": items, "page_size": 100, "total": len(items)}})
			}))
			defer server.Close()
			reader := New()
			ch := &connector.Channel{SiteURL: server.URL}
			session := &connector.AuthSession{Headers: map[string]string{"Authorization": "Bearer test"}}
			first, err := reader.ListUsage(context.Background(), ch, session, connector.UsageQuery{})
			if err != nil {
				t.Fatal(err)
			}
			second, err := reader.ListUsage(context.Background(), ch, session, connector.UsageQuery{})
			if err != nil {
				t.Fatal(err)
			}
			if first.Records[0].RemoteID != second.Records[1].RemoteID {
				t.Fatal("same request changed identity when display ID shifted")
			}
			if first.Records[0].RemoteID == second.Records[0].RemoteID {
				t.Fatal("new request overwrote a reused display ID")
			}
		})
	}
}

func TestUsageIdentityCanonicalizesMetadataAndKeepsRequestDetails(t *testing.T) {
	first := usageLog{Type: 2, CreatedAt: 1770000000, ModelName: "model", RequestID: "shared", Other: `{"frt":12,"cache_tokens":20}`}
	second := first
	second.Other = `{"cache_tokens":20, "frt":12}`
	if usageIdentity(first) != usageIdentity(second) {
		t.Fatal("metadata key order changed request identity")
	}
	second.Type = 5
	if usageIdentity(first) == usageIdentity(second) {
		t.Fatal("failed retry merged with successful request")
	}
	second = first
	second.TokenID = 7
	if usageIdentity(first) == usageIdentity(second) {
		t.Fatal("distinct token requests merged")
	}
}

func TestUsageFallsBackToRequestedPageSizeWhenMetadataIsMissing(t *testing.T) {
	items := make([]map[string]any, 100)
	for i := range items {
		items[i] = map[string]any{"id": i + 1, "type": 2, "created_at": 1770000000, "model_name": "gpt-5"}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/status" {
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"items": items, "page": 1, "total": 250}})
	}))
	defer server.Close()

	page, err := New().ListUsage(context.Background(), &connector.Channel{SiteURL: server.URL}, &connector.AuthSession{Headers: map[string]string{"Authorization": "Bearer test-token"}}, connector.UsageQuery{Page: 1, PageSize: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if page.PageSize != 100 || page.Pages != 3 || page.RawCount != 100 {
		t.Fatalf("page metadata = size:%d pages:%d raw:%d, want size:100 pages:3 raw:100", page.PageSize, page.Pages, page.RawCount)
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

func TestUsageWithoutPageSizeDoesNotAssumeUpstreamHonorsRequestedSize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/status" {
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"items": []map[string]any{{"id": 1, "type": 2, "created_at": 1770000000, "model_name": "model"}}, "total": 2}})
	}))
	defer server.Close()
	page, err := New().ListUsage(context.Background(), &connector.Channel{SiteURL: server.URL}, &connector.AuthSession{Headers: map[string]string{"Authorization": "Bearer test"}}, connector.UsageQuery{PageSize: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if page.PageSize != 1 || page.RawCount != 1 {
		t.Fatalf("page metadata = size:%d raw:%d, want a full page with size:1", page.PageSize, page.RawCount)
	}
}
