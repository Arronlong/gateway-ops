package sub2api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/guliansheng/gateway-ops/internal/connector"
)

func TestCountUsageLargePartialDayUsesBoundedBoundarySearch(t *testing.T) {
	day := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get("page_size") != "1" {
			t.Error("counting enumerated full usage pages")
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		stamp := day.Add(time.Duration(50000-page) * time.Second)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"total": 50000, "page": page, "page_size": 1, "items": []map[string]any{{"id": 50001 - page, "created_at": stamp.Format(time.RFC3339)}}}})
	}))
	defer server.Close()
	count, err := New().CountUsage(context.Background(), &connector.Channel{SiteURL: server.URL}, &connector.AuthSession{AccessToken: "test"}, connector.UsageQuery{Since: day.Add(10000 * time.Second), Until: day.Add(40000 * time.Second)})
	if err != nil || count != 30000 {
		t.Fatalf("count=%d error=%v", count, err)
	}
	if requests > 40 {
		t.Fatalf("count required %d requests for one partial day", requests)
	}
}

func TestCountUsageMissingTotalDoesNotFallBackToHistoryEnumeration(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"code":0,"data":{"page_size":1,"items":[{"id":1,"created_at":"2026-10-22T12:00:00Z"}]}}`))
	}))
	defer server.Close()
	day := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	_, err := New().CountUsage(context.Background(), &connector.Channel{SiteURL: server.URL}, &connector.AuthSession{AccessToken: "test"}, connector.UsageQuery{Since: day, Until: day.Add(24 * time.Hour)})
	if err == nil || requests != 1 {
		t.Fatalf("error=%v requests=%d", err, requests)
	}
}
