package connector_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guliansheng/gateway-ops/internal/connector"
	_ "github.com/guliansheng/gateway-ops/internal/connector/newapi"
	_ "github.com/guliansheng/gateway-ops/internal/connector/sub2api"
)

func TestCustomHeadersApplyToEveryChannelCredentialMode(t *testing.T) {
	for _, tc := range []struct {
		name    string
		kind    connector.ChannelType
		login   bool
		session *connector.AuthSession
	}{
		{"newapi_password", connector.TypeNewAPI, true, nil},
		{"newapi_cookie", connector.TypeNewAPI, false, &connector.AuthSession{Cookie: "session=test", UserID: "7"}},
		{"newapi_access_token", connector.TypeNewAPI, false, &connector.AuthSession{AccessToken: "access-test", UserID: "7", Headers: map[string]string{"Authorization": "Bearer access-test"}}},
		{"sub2api_password", connector.TypeSub2API, true, nil},
		{"sub2api_token", connector.TypeSub2API, false, &connector.AuthSession{AccessToken: "access-test", RefreshToken: "refresh-test"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seen := make(map[string]int)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen[r.URL.Path]++
				if r.Header.Get("X-Custom-Header") != "configured-value" || r.Header.Get("X-Channel-User") != "monitor" {
					t.Errorf("%s: missing custom headers: custom=%q user=%q", r.URL.Path, r.Header.Get("X-Custom-Header"), r.Header.Get("X-Channel-User"))
				}
				if r.URL.Path == "/api/user/self" && r.Header.Get("New-Api-User") != "7" {
					t.Errorf("missing NewAPI user id: %q", r.Header.Get("New-Api-User"))
				}
				if r.URL.Path == "/api/user/self" && tc.name != "newapi_access_token" {
					cookie, err := r.Cookie("session")
					if err != nil || cookie.Value != "test" {
						t.Errorf("missing session cookie: %q", r.Header.Get("Cookie"))
					}
				}
				if r.URL.Path == "/api/v1/auth/me" || (r.URL.Path == "/api/user/self" && tc.name == "newapi_access_token") {
					if r.Header.Get("Authorization") != "Bearer access-test" {
						t.Errorf("missing bearer token: %q", r.Header.Get("Authorization"))
					}
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/status":
					w.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
				case "/api/user/login":
					w.Header().Set("Set-Cookie", "session=test; Path=/")
					w.Write([]byte(`{"success":true,"data":{"id":7}}`))
				case "/api/user/self":
					w.Write([]byte(`{"success":true,"data":{"id":7,"quota":1000000}}`))
				case "/api/user/self/groups":
					w.Write([]byte(`{"success":true,"data":{"default":{"ratio":1}}}`))
				case "/api/log/self":
					w.Write([]byte(`{"success":true,"data":{"items":[],"page":1,"page_size":100,"total":0}}`))
				case "/api/v1/settings/public":
					w.Write([]byte(`{"code":0,"data":{}}`))
				case "/api/v1/auth/login", "/api/v1/auth/refresh":
					w.Write([]byte(`{"code":0,"data":{"access_token":"access-test","refresh_token":"refresh-test","expires_in":3600}}`))
				case "/api/v1/auth/me":
					w.Write([]byte(`{"code":0,"data":{"balance":12}}`))
				case "/api/v1/groups/available":
					w.Write([]byte(`{"code":0,"data":[{"id":1,"name":"default","rate_multiplier":1}]}`))
				case "/api/v1/groups/rates":
					w.Write([]byte(`{"code":0,"data":{}}`))
				case "/api/v1/usage":
					w.Write([]byte(`{"code":0,"data":{"items":[],"page":1,"pages":1,"total":0}}`))
				default:
					t.Errorf("unexpected endpoint: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			client, err := connector.For(tc.kind)
			if err != nil {
				t.Fatal(err)
			}
			ch := &connector.Channel{SiteURL: server.URL, Username: "monitor", Password: "password-test", LoginHeaders: []connector.RequestKV{
				{Key: "X-Custom-Header", Value: "configured-value"},
				{Key: "X-Channel-User", Value: "{{username}}"},
			}}
			ctx := context.Background()
			if _, err := client.GetTurnstileSiteKey(ctx, ch); err != nil {
				t.Fatal(err)
			}
			session := tc.session
			if tc.login {
				session, err = client.Login(ctx, ch)
				if err != nil {
					t.Fatal(err)
				}
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
			if tc.kind == connector.TypeSub2API {
				if _, err := client.(connector.TokenRefresher).RefreshToken(ctx, ch, session.RefreshToken); err != nil {
					t.Fatal(err)
				}
			}
			if len(seen) < 4 {
				t.Fatalf("insufficient endpoints exercised: %v", seen)
			}
		})
	}
}

func TestCustomHeadersDoNotLeakBetweenChannels(t *testing.T) {
	for _, kind := range []connector.ChannelType{connector.TypeNewAPI, connector.TypeSub2API} {
		t.Run(string(kind), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				want := ""
				if strings.HasPrefix(r.URL.Path, "/configured/") {
					want = "configured-value"
				}
				if r.Header.Get("X-Custom-Header") != want {
					t.Errorf("%s: header=%q want=%q", r.URL.Path, r.Header.Get("X-Custom-Header"), want)
				}
				w.Header().Set("Content-Type", "application/json")
				if kind == connector.TypeNewAPI {
					w.Write([]byte(`{"success":true,"data":{"id":7}}`))
				} else {
					w.Write([]byte(`{"code":0,"data":{"id":7}}`))
				}
			}))
			defer server.Close()
			client, err := connector.For(kind)
			if err != nil {
				t.Fatal(err)
			}
			session := &connector.AuthSession{AccessToken: "access-test", Cookie: "session=test", UserID: "7"}
			for _, prefix := range []string{"configured", "plain"} {
				ch := &connector.Channel{SiteURL: server.URL + "/" + prefix}
				if prefix == "configured" {
					ch.LoginHeaders = []connector.RequestKV{{Key: "X-Custom-Header", Value: "configured-value"}}
				}
				if err := client.CheckAuth(context.Background(), ch, session); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
