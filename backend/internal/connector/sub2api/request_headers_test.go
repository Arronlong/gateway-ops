package sub2api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guliansheng/gateway-ops/internal/connector"
)

func TestConfiguredAuthorizationUsesCurrentSession(t *testing.T) {
	cases := []struct {
		name     string
		headers  []connector.RequestKV
		expected func(string) string
	}{
		{"legacy_default", nil, func(token string) string { return "Bearer " + token }},
		{"custom_scheme", []connector.RequestKV{{Key: "Authorization", Value: "Token {{access_token}}"}}, func(token string) string { return "Token " + token }},
		{"lowercase_static", []connector.RequestKV{{Key: "authorization", Value: "Custom static"}}, func(string) string { return "Custom static" }},
		{"token_alias", []connector.RequestKV{{Key: "Authorization", Value: "Bearer {{token}}"}}, func(token string) string { return "Bearer " + token }},
		{"explicit_empty", []connector.RequestKV{{Key: "Authorization", Value: ""}}, func(string) string { return "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			currentToken := "access-first"
			seen := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen++
				if got := r.Header.Get("Authorization"); got != tc.expected(currentToken) {
					t.Errorf("%s Authorization=%q want=%q", r.URL.Path, got, tc.expected(currentToken))
				}
				if got := r.Header.Get("X-Current-Token"); got != currentToken {
					t.Errorf("%s current token=%q want=%q", r.URL.Path, got, currentToken)
				}
				if got := r.Header.Get("X-Channel-User"); got != "monitor" {
					t.Errorf("custom username header=%q", got)
				}
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/api/v1/usage" {
					w.Write([]byte(`{"code":0,"data":{"items":[],"page":1,"pages":1,"total":0}}`))
				} else {
					w.Write([]byte(`{"code":0,"data":{"balance":12}}`))
				}
			}))
			defer server.Close()
			headers := append([]connector.RequestKV{}, tc.headers...)
			headers = append(headers, connector.RequestKV{Key: "X-Current-Token", Value: "{{access_token}}"}, connector.RequestKV{Key: "X-Channel-User", Value: "{{username}}"})
			ch := &connector.Channel{SiteURL: server.URL, Username: "monitor", LoginHeaders: headers}
			client := New()
			for _, token := range []string{"access-first", "access-next"} {
				currentToken = token
				session := &connector.AuthSession{AccessToken: token}
				if _, err := client.GetBalance(context.Background(), ch, session); err != nil {
					t.Fatal(err)
				}
				if _, err := client.ListUsage(context.Background(), ch, session, connector.UsageQuery{Page: 1}); err != nil {
					t.Fatal(err)
				}
			}
			if seen != 4 {
				t.Fatalf("request count=%d want=4", seen)
			}
		})
	}
}

func TestSessionTemplatesAreOmittedFromUnauthenticatedRequests(t *testing.T) {
	seen := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen++
		for _, key := range []string{"Authorization", "X-Current-Token"} {
			if got := r.Header.Get(key); got != "" {
				t.Errorf("%s sent unresolved %s=%q", r.URL.Path, key, got)
			}
		}
		if got := r.Header.Get("X-Custom-Header"); got != "configured" {
			t.Errorf("custom header=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/settings/public" {
			w.Write([]byte(`{"code":0,"data":{}}`))
		} else {
			w.Write([]byte(`{"code":0,"data":{"access_token":"access-new","refresh_token":"refresh-new","expires_in":3600}}`))
		}
	}))
	defer server.Close()
	ch := &connector.Channel{SiteURL: server.URL, LoginHeaders: []connector.RequestKV{{Key: "Authorization", Value: "Bearer {{access_token}}"}, {Key: "X-Current-Token", Value: "{{token}}"}, {Key: "X-Custom-Header", Value: "configured"}}}
	client := New()
	if _, err := client.GetTurnstileSiteKey(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Login(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	if _, err := client.RefreshToken(context.Background(), ch, "refresh-old"); err != nil {
		t.Fatal(err)
	}
	if seen != 3 {
		t.Fatalf("request count=%d want=3", seen)
	}
}
