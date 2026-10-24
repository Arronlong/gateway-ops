package relay

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPelicanNewAPIKeyGroupUsesExactKeyAndProxyPath(t *testing.T) {
	for _, ratio := range []string{"0", "0.25"} {
		t.Run(ratio, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Cookie") != "session=user-session" || r.Header.Get("New-Api-User") != "123" {
					t.Error("missing New API session headers")
				}
				switch r.URL.Path {
				case "/proxy/site/api/token/":
					fmt.Fprint(w, `{"success":true,"data":{"items":[{"id":1,"key":"other-key","group":"错误分组"},{"id":2,"key":"actual-key","group":"真实长分组名称"}],"total":2}}`)
				case "/proxy/site/api/user/self/groups":
					fmt.Fprintf(w, `{"success":true,"data":{"真实长分组名称":{"ratio":%s,"desc":"分组说明"},"错误分组":{"ratio":9}}}`, ratio)
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
				}
			}))
			defer server.Close()
			target := &pelicanTarget{client: *server.Client(), endpoint: server.URL + "/proxy/site/v1/responses", token: "sk-actual-key"}
			info := target.readNewAPIKeyGroup(context.Background(), server.URL+"/proxy/site", http.Header{"Cookie": {"session=user-session"}, "New-Api-User": {"123"}})
			want := 0.25
			if ratio == "0" {
				want = 0
			}
			if info.Name != "真实长分组名称" || info.Multiplier == nil || *info.Multiplier != want {
				t.Fatalf("wrong metadata: %#v", info)
			}
		})
	}
}

func TestPelicanNewAPIKeyGroupConfirmsMaskedKeyAndUsesUserGroup(t *testing.T) {
	for _, fullKey := range []string{"actual-secret-key", "another-secret-key"} {
		t.Run(fullKey, func(t *testing.T) {
			var keyReads int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/token/":
					fmt.Fprint(w, `{"success":true,"data":[{"id":2,"key":"act***key","group":""}]}`)
				case "/api/token/2/key":
					if r.Method != http.MethodPost {
						t.Error("full key must use dedicated read endpoint")
					}
					keyReads++
					fmt.Fprintf(w, `{"success":true,"data":{"key":%q}}`, fullKey)
				case "/api/user/self":
					fmt.Fprint(w, `{"success":true,"data":{"group":"用户默认组"}}`)
				case "/api/user/self/groups":
					fmt.Fprint(w, `{"success":true,"data":{"用户默认组":{"ratio":0.5}}}`)
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
				}
			}))
			defer server.Close()
			target := &pelicanTarget{client: *server.Client(), endpoint: server.URL + "/v1/responses", token: "sk-actual-secret-key"}
			info := target.readNewAPIKeyGroup(context.Background(), server.URL, http.Header{"Authorization": {"Bearer user-access-token"}, "New-Api-User": {"123"}})
			if keyReads != 1 {
				t.Fatalf("key reads=%d", keyReads)
			}
			if fullKey == "actual-secret-key" {
				if info.Name != "用户默认组" || info.Multiplier == nil || *info.Multiplier != 0.5 {
					t.Fatalf("wrong metadata: %#v", info)
				}
			} else if info.Name != "" || info.Multiplier != nil {
				t.Fatalf("masked key must not be treated as an exact match: %#v", info)
			}
		})
	}
}

func TestPelicanNewAPIKeyGroupUnknownRatiosAndLinkedProxy(t *testing.T) {
	for _, ratio := range []string{`null`, `"auto"`, `-1`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/token/" {
				fmt.Fprint(w, `{"success":true,"data":{"items":[{"key":"actual","group":"group"}],"total":1}}`)
				return
			}
			fmt.Fprintf(w, `{"success":true,"data":{"group":{"ratio":%s}}}`, ratio)
		}))
		target := &pelicanTarget{client: *server.Client(), endpoint: server.URL + "/v1/responses", token: "sk-actual"}
		info := target.readNewAPIKeyGroup(context.Background(), server.URL, nil)
		if info.Name != "group" || info.Multiplier != nil {
			t.Fatalf("must not invent a rate: %#v", info)
		}
		server.Close()
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "secret" || r.Header.Get("Authorization") != "" {
			t.Error("only the source-specific cookie may be sent")
		}
		fmt.Fprint(w, `{"success":true,"data":{"items":[],"total":0}}`)
	}))
	defer server.Close()
	target := &pelicanTarget{client: *server.Client(), endpoint: "https://different.example/v1/responses", token: "sk-actual"}
	info := target.readNewAPIKeyGroup(context.Background(), server.URL, http.Header{"Cookie": {"secret"}})
	if strings.TrimSpace(info.Name) != "" || info.Multiplier != nil {
		t.Fatal("unexpected metadata")
	}
}
