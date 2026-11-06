package newapi

import (
	"context"
	"github.com/guliansheng/gateway-ops/internal/connector"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestUsageRequestsPauseAfterUpstreamRejection(t *testing.T) {
	for _, status := range []int{403, 429} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; w.WriteHeader(status) }))
			defer server.Close()
			ctx := connector.WithUsageLimiter(context.Background(), connector.NewUsageLimiter(0, time.Minute))
			ch := &connector.Channel{SiteURL: server.URL}
			session := &connector.AuthSession{Headers: map[string]string{"Authorization": "Bearer test"}}
			if _, err := New().CountUsage(ctx, ch, session, connector.UsageQuery{}); err == nil {
				t.Fatal("rejection was ignored")
			}
			if _, err := New().ListUsage(ctx, ch, session, connector.UsageQuery{}); err == nil {
				t.Fatal("cooldown was ignored")
			}
			if requests != 1 {
				t.Fatalf("sent %d requests after rejection", requests)
			}
		})
	}
}
