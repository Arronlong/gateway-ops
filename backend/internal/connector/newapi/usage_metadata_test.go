package newapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guliansheng/gateway-ops/internal/connector"
)

func TestUsagePagesReuseQuotaMetadata(t *testing.T) {
	statusRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/status" {
			statusRequests++
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"total":0,"page_size":100,"items":[]}}`))
	}))
	defer server.Close()
	client := New()
	for page := 1; page <= 3; page++ {
		if _, err := client.ListUsage(context.Background(), &connector.Channel{SiteURL: server.URL}, &connector.AuthSession{Headers: map[string]string{"Authorization": "Bearer test"}}, connector.UsageQuery{Page: page}); err != nil {
			t.Fatal(err)
		}
	}
	if statusRequests != 1 {
		t.Fatalf("every page reloaded status metadata: %d", statusRequests)
	}
}
