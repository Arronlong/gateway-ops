package newapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guliansheng/gateway-ops/internal/connector"
)

func TestConfiguredCookieHeadersOverrideSessionDefaults(t *testing.T) {
	cases := []struct {
		name     string
		headers  []connector.RequestKV
		expected func(*connector.AuthSession) (string, string)
	}{
		{"legacy_defaults", nil, func(s *connector.AuthSession) (string, string) { return s.Cookie, s.UserID }},
		{"session_templates", []connector.RequestKV{{Key: "Cookie", Value: "{{cookie}}; custom=value"}, {Key: "New-Api-User", Value: "user-{{user_id}}"}}, func(s *connector.AuthSession) (string, string) {
			return s.Cookie + "; custom=value", "user-" + s.UserID
		}},
		{"lowercase_static", []connector.RequestKV{{Key: "cookie", Value: "session=override"}, {Key: "new-api-user", Value: "42"}}, func(*connector.AuthSession) (string, string) { return "session=override", "42" }},
		{"explicit_empty", []connector.RequestKV{{Key: "Cookie", Value: ""}, {Key: "New-Api-User", Value: ""}}, func(*connector.AuthSession) (string, string) { return "", "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			current := &connector.AuthSession{Cookie: "session=first", UserID: "7"}
			seen := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/api/status" {
					w.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
					return
				}
				seen++
				cookie, userID := tc.expected(current)
				if got := r.Header.Get("Cookie"); got != cookie {
					t.Errorf("%s Cookie=%q want=%q", r.URL.Path, got, cookie)
				}
				if got := r.Header.Get("New-Api-User"); got != userID {
					t.Errorf("%s New-Api-User=%q want=%q", r.URL.Path, got, userID)
				}
				if got := r.Header.Get("X-Account"); got != "monitor" {
					t.Errorf("custom header=%q", got)
				}
				if r.URL.Path == "/api/log/self" {
					w.Write([]byte(`{"success":true,"data":{"items":[],"page":1,"page_size":100,"total":0}}`))
				} else {
					w.Write([]byte(`{"success":true,"data":{"id":7,"quota":1000000}}`))
				}
			}))
			defer server.Close()
			headers := append([]connector.RequestKV{}, tc.headers...)
			headers = append(headers, connector.RequestKV{Key: "X-Account", Value: "{{username}}"})
			ch := &connector.Channel{SiteURL: server.URL, Username: "monitor", LoginHeaders: headers}
			client := New()
			for _, session := range []*connector.AuthSession{{Cookie: "session=first", UserID: "7"}, {Cookie: "session=next", UserID: "8"}} {
				current = session
				if err := client.CheckAuth(context.Background(), ch, session); err != nil {
					t.Fatal(err)
				}
				if _, err := client.ListUsage(context.Background(), ch, session, connector.UsageQuery{Page: 1}); err != nil {
					t.Fatal(err)
				}
			}
			if seen != 4 {
				t.Fatalf("authenticated request count=%d want=4", seen)
			}
		})
	}
}

func TestCookieTemplatesAreOmittedFromPublicAndLoginRequests(t *testing.T) {
	seen := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen++
		for _, key := range []string{"Cookie", "New-Api-User"} {
			if got := r.Header.Get(key); got != "" {
				t.Errorf("%s sent unresolved %s=%q", r.URL.Path, key, got)
			}
		}
		if got := r.Header.Get("X-Account"); got != "monitor" {
			t.Errorf("custom header=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/status" {
			w.Write([]byte(`{"success":true,"data":{}}`))
			return
		}
		w.Header().Set("Set-Cookie", "session=logged-in; Path=/")
		w.Write([]byte(`{"success":true,"data":{"id":7}}`))
	}))
	defer server.Close()
	ch := &connector.Channel{SiteURL: server.URL, Username: "monitor", LoginHeaders: []connector.RequestKV{{Key: "Cookie", Value: "{{cookie}}"}, {Key: "New-Api-User", Value: "{{user_id}}"}, {Key: "X-Account", Value: "{{username}}"}}}
	client := New()
	if _, err := client.GetTurnstileSiteKey(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Login(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	if seen != 2 {
		t.Fatalf("request count=%d want=2", seen)
	}
}
