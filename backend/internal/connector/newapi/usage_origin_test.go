package newapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guliansheng/gateway-ops/internal/connector"
)

func TestUsageHistoryOriginUsesAccountOrTokenCreation(t *testing.T) {
	for _, accountTime := range []int64{0, 1790000000} {
		t.Run(fmt.Sprint(accountTime), func(t *testing.T) {
			tokens := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("New-Api-User") != "7" {
					t.Error("missing user header")
				}
				if r.URL.Path == "/api/user/self" {
					_, _ = fmt.Fprintf(w, `{"success":true,"data":{"created_at":%d}}`, accountTime)
					return
				}
				tokens++
				if r.URL.Query().Get("page_size") != "100" {
					t.Error("unexpected token page size")
				}
				_, _ = w.Write([]byte(`{"success":true,"data":{"items":[{"created_time":1791000000},{"created_time":1790000000}]}}`))
			}))
			defer server.Close()
			stamp, err := New().UsageHistoryOrigin(context.Background(), &connector.Channel{SiteURL: server.URL}, &connector.AuthSession{UserID: "7", Headers: map[string]string{"Authorization": "Bearer test"}})
			if err != nil || !stamp.Equal(time.Unix(1790000000, 0)) {
				t.Fatalf("origin=%s error=%v", stamp, err)
			}
			if accountTime > 0 && tokens != 0 {
				t.Fatal("queried tokens despite account creation metadata")
			}
		})
	}
}
