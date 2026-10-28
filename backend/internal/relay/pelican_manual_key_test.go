package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	appcrypto "github.com/guliansheng/gateway-ops/internal/crypto"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

func TestPreparePelicanWithManualKeyOverridesSub2APIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/admin/accounts/42/models":
			w.Write([]byte(`{"code":0,"data":["test-model"]}`))
		case "/v1/chat/completions":
			if r.Header.Get("Authorization") != "Bearer manual-secret" {
				t.Errorf("manual key did not take precedence: %q", r.Header.Get("Authorization"))
			}
			w.Write([]byte(`{"choices":[{"message":{"content":"<svg></svg>"},"finish_reason":"stop"}]}`))
		default:
			t.Errorf("unexpected route: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	s := NewService(nil, nil)
	target, err := s.preparePelicanForStation(context.Background(), storage.RelayStation{Type: "sub2api", BaseURL: server.URL}, "admin-secret", "manual-secret", storage.RelayAccount{BaseURL: server.URL}, storage.PelicanConfig{AccountExternalID: 42, Model: "missing-model\ntest-model"})
	if err != nil {
		t.Fatal(err)
	}
	defer target.close()
	if target.model != "test-model" || target.client.Timeout != pelicanTimeout {
		t.Fatalf("model=%q timeout=%v", target.model, target.client.Timeout)
	}
	html, err := target.generate(context.Background(), storage.PelicanConfig{Model: "test-model", Prompt: "draw"})
	if err != nil || html != "<svg></svg>" {
		t.Fatalf("html=%q err=%v", html, err)
	}
}

func TestPelicanChannelKeyReadsWithoutPersisting(t *testing.T) {
	for _, stationType := range []string{"newapi", "sub2api"} {
		for _, configured := range []bool{false, true} {
			t.Run(stationType+"/configured="+fmt.Sprint(configured), func(t *testing.T) {
				s, mock := pelicanExecutionDB(t)
				s.cipher, _ = appcrypto.NewCipher("manual-channel-key-test")
				ciphertext := ""
				if configured {
					ciphertext, _ = s.cipher.Encrypt("manual-secret")
				}
				mock.ExpectQuery(`SELECT channels.id AS channel_id, channels.pelican_key_cipher AS ciphertext`).WithArgs(uint(5), uint(7), int64(42), 1).WillReturnRows(sqlmock.NewRows([]string{"channel_id", "ciphertext"}).AddRow(5, ciphertext))
				key, err := s.pelicanChannelKey(storage.RelayStation{ID: 7, Type: stationType}, 42, 5)
				if err != nil || configured && key != "manual-secret" || !configured && key != "" {
					t.Fatalf("key resolved incorrectly: configured=%v err=%v", configured, err)
				}
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestPrepareNewAPIPelicanUsesManualKeyForZhipuChannel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/channel/42":
			if r.Header.Get("Authorization") != "Bearer admin-token" || r.Header.Get("New-Api-User") != "7" {
				t.Errorf("missing NewAPI management credentials: %v", r.Header)
			}
			w.Write([]byte(`{"success":true,"data":{"id":42,"type":16,"base_url":"http://` + r.Host + `/api/paas/v4","models":"glm-5.6"}}`))
		case "/api/paas/v4/chat/completions":
			if r.Header.Get("Authorization") != "Bearer upstream-secret" || r.Header.Get("New-Api-User") != "" {
				t.Errorf("wrong upstream credentials: %v", r.Header)
			}
			var payload struct {
				Model    string `json:"model"`
				Messages []struct {
					Content string `json:"content"`
				} `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			if payload.Model != "glm-5.6" || len(payload.Messages) != 1 || !strings.Contains(payload.Messages[0].Content, "draw") {
				t.Errorf("unexpected payload: %#v", payload)
			}
			w.Write([]byte(`{"choices":[{"message":{"content":"<svg></svg>"},"finish_reason":"stop"}]}`))
		default:
			t.Errorf("unexpected route: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	s := NewService(nil, nil)
	credential := newAPICredential{Token: "admin-token", UserID: "7"}
	station := storage.RelayStation{Type: "newapi", BaseURL: server.URL}
	target, err := s.prepareNewAPIPelican(context.Background(), station, credential, 42, "upstream-secret", storage.RelayAccount{BaseURL: server.URL}, storage.PelicanConfig{AccountExternalID: 42, Model: "glm-5.6"})
	if err != nil {
		t.Fatal(err)
	}
	defer target.close()
	html, err := target.generate(context.Background(), storage.PelicanConfig{Model: "glm-5.6", Prompt: "draw"})
	if err != nil || html != "<svg></svg>" {
		t.Fatalf("html=%q err=%v", html, err)
	}
}

func TestPreparePelicanWithoutManualKeyUsesSub2APIExport(t *testing.T) {
	exports := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/admin/accounts/42/models":
			w.Write([]byte(`{"code":0,"data":["test-model"]}`))
		case "/api/v1/admin/accounts/data":
			exports++
			w.Write([]byte(`{"code":0,"data":{"accounts":[{"platform":"openai","type":"apikey","credentials":{"base_url":"http://` + r.Host + `","api_key":"stored-secret"},"extra":{"openai_responses_supported":false}}]}}`))
		case "/v1/chat/completions":
			if r.Header.Get("Authorization") != "Bearer stored-secret" {
				t.Errorf("automatic credential was not used")
			}
			w.Write([]byte(`{"choices":[{"message":{"content":"<svg></svg>"},"finish_reason":"stop"}]}`))
		default:
			t.Errorf("unexpected route: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	s := NewService(nil, nil)
	config := storage.PelicanConfig{AccountExternalID: 42, Model: "test-model", Prompt: "draw"}
	target, err := s.preparePelicanForStation(context.Background(), storage.RelayStation{Type: "sub2api", BaseURL: server.URL}, "admin-secret", "", storage.RelayAccount{}, config)
	if err != nil {
		t.Fatal(err)
	}
	defer target.close()
	html, err := target.generate(context.Background(), config)
	if err != nil || html != "<svg></svg>" || exports != 1 {
		t.Fatalf("html=%q exports=%d err=%v", html, exports, err)
	}
}

func TestNewAPIPelicanMissingManualKeyMakesNoUpstreamRequest(t *testing.T) {
	s := NewService(nil, nil)
	target, err := s.prepareNewAPIPelican(context.Background(), storage.RelayStation{}, newAPICredential{}, 42, "", storage.RelayAccount{}, storage.PelicanConfig{})
	if err == nil || target != nil {
		t.Fatalf("missing manual key should fail before transport: target=%v err=%v", target, err)
	}
}
