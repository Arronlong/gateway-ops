package connector_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/guliansheng/gateway-ops/internal/connector"
)

func TestEveryChannelRequestUsesSelectedProxy(t *testing.T) {
	for _, kind := range []connector.ChannelType{connector.TypeNewAPI, connector.TypeSub2API} {
		t.Run(string(kind), func(t *testing.T) {
			seen := make(map[string]int)
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen[r.URL.Path]++
				if r.URL.Host != "upstream.invalid" || r.Header.Get("Proxy-Authorization") != "Basic dXNlcjpwYXNz" {
					t.Error("incorrect proxy destination or credentials")
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/status":
					io.WriteString(w, `{"success":true,"data":{"quota_per_unit":500000}}`)
				case "/api/user/login":
					w.Header().Set("Set-Cookie", "session=test; Path=/")
					io.WriteString(w, `{"success":true,"data":{"id":7}}`)
				case "/api/user/self":
					io.WriteString(w, `{"success":true,"data":{"id":7,"quota":1000000}}`)
				case "/api/user/self/groups":
					io.WriteString(w, `{"success":true,"data":{"default":{"ratio":1}}}`)
				case "/api/log/self":
					io.WriteString(w, `{"success":true,"data":{"items":[],"page":1,"page_size":100,"total":0}}`)
				case "/api/token/":
					io.WriteString(w, `{"success":true,"data":{"items":[]}}`)
				case "/api/v1/settings/public":
					io.WriteString(w, `{"code":0,"data":{}}`)
				case "/api/v1/auth/login", "/api/v1/auth/refresh":
					io.WriteString(w, `{"code":0,"data":{"access_token":"test","refresh_token":"refresh","expires_in":3600}}`)
				case "/api/v1/auth/me":
					io.WriteString(w, `{"code":0,"data":{"balance":12}}`)
				case "/api/v1/groups/available":
					io.WriteString(w, `{"code":0,"data":[{"id":1,"name":"default","rate_multiplier":1}]}`)
				case "/api/v1/groups/rates":
					io.WriteString(w, `{"code":0,"data":{}}`)
				case "/api/v1/usage":
					io.WriteString(w, `{"code":0,"data":{"items":[],"page":1,"pages":1,"total":0}}`)
				default:
					t.Errorf("unexpected path: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer proxy.Close()
			u, _ := url.Parse(proxy.URL)
			u.User = url.UserPassword("user", "pass")
			ch := &connector.Channel{SiteURL: "http://upstream.invalid", ProxyURL: u, Username: "test", Password: "test"}
			client, _ := connector.For(kind)
			ctx := context.Background()
			if _, err := client.GetTurnstileSiteKey(ctx, ch); err != nil {
				t.Fatal(err)
			}
			session, err := client.Login(ctx, ch)
			if err != nil {
				t.Fatal(err)
			}
			if err := client.CheckAuth(ctx, ch, session); err != nil {
				t.Fatal(err)
			}
			if _, err := client.GetBalance(ctx, ch, session); err != nil {
				t.Fatal(err)
			}
			if _, err := client.GetRates(ctx, ch, session); err != nil {
				t.Fatal(err)
			}
			if _, err := client.(connector.UsageReader).ListUsage(ctx, ch, session, connector.UsageQuery{Page: 1}); err != nil {
				t.Fatal(err)
			}
			if _, err := client.(connector.UsageCounter).CountUsage(ctx, ch, session, connector.UsageQuery{Since: time.Now().Add(-time.Hour), Until: time.Now()}); err != nil {
				t.Fatal(err)
			}
			if reader, ok := client.(connector.UsageHistoryOriginReader); ok {
				if _, err := reader.UsageHistoryOrigin(ctx, ch, session); err != nil {
					t.Fatal(err)
				}
			}
			if kind == connector.TypeSub2API {
				if _, err := client.(connector.TokenRefresher).RefreshToken(ctx, ch, session.RefreshToken); err != nil {
					t.Fatal(err)
				}
			}
			if len(seen) < 5 {
				t.Fatalf("too few endpoints passed through proxy: %v", seen)
			}
		})
	}
}
