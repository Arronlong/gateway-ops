package relay

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/guliansheng/gateway-ops/internal/crypto"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

func TestPelicanKeyGroupExpiredTokenDoesNotRefreshOrFallback(t *testing.T) {
	for _, kind := range []string{"sub2api", "newapi"} {
		t.Run(kind, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				want := "/api/v1/keys"
				if kind == "newapi" {
					want = "/api/token/"
				}
				if r.Method != http.MethodGet || r.URL.Path != want {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Bearer expired-token" {
					t.Error("must only use supplied token")
				}
				http.Error(w, "expired token", http.StatusUnauthorized)
			}))
			defer server.Close()
			service, mock := pelicanExecutionDB(t)
			service.channels = storage.NewChannels(nil)
			service.cipher, _ = crypto.NewCipher("pelican-credential-test-secret")
			raw := `{"access_token":"expired-token","refresh_token":"must-not-be-used"}`
			if kind == "newapi" {
				raw = `{"auth_type":"access_token","token":"expired-token","user_id":"123"}`
			}
			cipher, _ := service.cipher.Encrypt(raw)
			mock.ExpectQuery(`SELECT channels.site_url, sessions.access_token_cipher`).WithArgs(storage.AdditionalAccountSessionOffset, uint(7), int64(20), "sub2api", "newapi").WillReturnRows(sqlmock.NewRows([]string{"site_url", "type", "credential_mode", "credential_cipher"}).AddRow(server.URL, kind, "token", cipher))
			info := service.pelicanKeyGroup(context.Background(), &pelicanTarget{client: *server.Client(), token: "generation-key"}, 7, 20)
			if requests != 1 || info.Name != "" || info.Multiplier != nil {
				t.Fatalf("must stop after lookup failure: requests=%d metadata=%#v", requests, info)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPelicanKeyGroupTokenModeDoesNotRequireLoginCache(t *testing.T) {
	for _, kind := range []string{"sub2api", "newapi"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer direct-login" && r.URL.Path != "/v1/sub2api/billing" {
					t.Error("must use saved user credential")
				}
				switch r.URL.Path {
				case "/api/v1/keys":
					fmt.Fprint(w, `{"code":0,"data":{"items":[{"key":"sk-actual-key","group":{"id":2,"name":"已确认分组","rate_multiplier":9}}],"pages":1}}`)
				case "/v1/sub2api/billing":
					fmt.Fprint(w, `{"object":"sub2api.key_billing","resolved_rate_multiplier":0.5}`)
				case "/api/token/":
					fmt.Fprint(w, `{"success":true,"data":{"items":[{"key":"actual-key","group":"已确认分组"}],"total":1}}`)
				case "/api/user/self/groups":
					fmt.Fprint(w, `{"success":true,"data":{"已确认分组":{"ratio":0.5}}}`)
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
				}
			}))
			defer server.Close()
			service, mock := pelicanExecutionDB(t)
			service.channels = storage.NewChannels(nil)
			service.cipher, _ = crypto.NewCipher("pelican-credential-test-secret")
			raw := `{"access_token":"direct-login"}`
			if kind == "newapi" {
				raw = `{"auth_type":"access_token","token":"direct-login","user_id":"123"}`
			}
			cipher, _ := service.cipher.Encrypt(raw)
			mock.ExpectQuery(`SELECT channels.site_url, sessions.access_token_cipher`).WithArgs(storage.AdditionalAccountSessionOffset, uint(7), int64(20), "sub2api", "newapi").WillReturnRows(sqlmock.NewRows([]string{"site_url", "type", "credential_mode", "credential_cipher", "access_token_cipher", "cookie_cipher"}).AddRow(server.URL, kind, "token", cipher, nil, nil))
			target := &pelicanTarget{client: *server.Client(), endpoint: "https://generation-proxy.example/v1/responses", token: "sk-actual-key"}
			info := service.pelicanKeyGroup(context.Background(), target, 7, 20)
			if info.Name != "已确认分组" || info.Multiplier == nil || *info.Multiplier != 0.5 {
				t.Fatalf("missing metadata without cached auth session: %#v", info)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPelicanKeyLookupDoesNotFollowCredentialRedirect(t *testing.T) {
	for _, kind := range []string{"sub2api", "newapi"} {
		t.Run(kind, func(t *testing.T) {
			other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("credential leaked through redirect") }))
			defer other.Close()
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, http.StatusFound) }))
			defer source.Close()
			target := &pelicanTarget{client: *source.Client(), token: "actual-key"}
			var info pelicanKeyGroup
			if kind == "newapi" {
				info = target.readNewAPIKeyGroup(context.Background(), source.URL, http.Header{"Cookie": {"secret"}})
			} else {
				info = target.readKeyGroup(context.Background(), source.URL, "secret")
			}
			if info.Name != "" || info.Multiplier != nil {
				t.Fatal("unexpected metadata")
			}
		})
	}
}
