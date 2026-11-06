package newapi

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

func TestCountUsageIncludesOnlyCallsAndUsesExclusiveEnd(t *testing.T) {
	since := time.Unix(1770000000, 1)
	until := time.Unix(1770000600, 0)
	var types []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		kind, _ := strconv.Atoi(r.URL.Query().Get("type"))
		types = append(types, kind)
		if r.URL.Query().Get("start_timestamp") != "1770000001" || r.URL.Query().Get("end_timestamp") != "1770000599" {
			t.Errorf("incorrect count bounds: %s", r.URL.RawQuery)
		}
		total := 7
		if kind == 5 {
			total = 2
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"total": total, "page_size": 1, "items": []map[string]any{{"type": kind}}}})
	}))
	defer server.Close()
	count, err := New().CountUsage(context.Background(), &connector.Channel{SiteURL: server.URL}, &connector.AuthSession{Headers: map[string]string{"Authorization": "Bearer test"}}, connector.UsageQuery{Since: since, Until: until})
	if err != nil || count != 9 || len(types) != 2 || types[0] != 2 || types[1] != 5 {
		t.Fatalf("count=%d types=%v error=%v", count, types, err)
	}
}

func TestCountUsageRejectsMissingMetadataAndIgnoredTypeFilter(t *testing.T) {
	for _, data := range []string{`{"items":[{"type":2}]}`, `{"total":8,"page_size":1,"items":[{"type":1}]}`, `{"total":8,"items":[{"type":2}]}`, `{"total":8,"page_size":1,"items":[]}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"success":true,"data":` + data + `}`))
		}))
		_, err := New().CountUsage(context.Background(), &connector.Channel{SiteURL: server.URL}, &connector.AuthSession{Headers: map[string]string{"Authorization": "Bearer test"}}, connector.UsageQuery{})
		server.Close()
		if err == nil {
			t.Fatalf("accepted unreliable count: %s", data)
		}
	}
}

func TestCountUsageRejectsPotentiallyCappedTotal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"total":2000,"page_size":1,"items":[{"type":2}]}}`))
	}))
	defer server.Close()
	_, err := New().CountUsage(context.Background(), &connector.Channel{SiteURL: server.URL}, &connector.AuthSession{Headers: map[string]string{"Authorization": "Bearer test"}}, connector.UsageQuery{})
	if err == nil {
		t.Fatal("capped total was reported as an exact count")
	}
}
