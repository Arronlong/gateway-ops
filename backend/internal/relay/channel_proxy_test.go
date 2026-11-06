package relay

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/glebarez/sqlite"
	appcrypto "github.com/guliansheng/gateway-ops/internal/crypto"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
)

func TestChannelPelicanUsesConfiguredProxy(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host != "upstream.invalid" || r.Header.Get("Proxy-Authorization") != "Basic dXNlcjpwYXNz" {
			t.Error("channel test bypassed proxy authentication")
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"<svg></svg>"},"finish_reason":"stop"}]}`))
	}))
	defer proxy.Close()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "proxy.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := db.AutoMigrate(&storage.ProxyConfig{}, &storage.Channel{}, &storage.ChannelAccount{}, &storage.ChannelRelayAccountLink{}); err != nil {
		t.Fatal(err)
	}
	cipher, _ := appcrypto.NewCipher("proxy-test-secret")
	raw := "http://user:pass@" + proxy.Listener.Addr().String()
	encrypted, _ := cipher.Encrypt(raw)
	config := storage.ProxyConfig{URLCipher: encrypted, URLHash: fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))}
	proxies := storage.NewProxies(db)
	if err := proxies.CreateOrFind(&config); err != nil {
		t.Fatal(err)
	}
	keyCipher, _ := cipher.Encrypt("manual-key")
	ch := storage.Channel{Name: "proxy", SiteURL: "http://upstream.invalid", ProxyID: &config.ID, PelicanKeyCipher: keyCipher}
	channels := storage.NewChannels(db)
	if err := channels.Create(&ch); err != nil {
		t.Fatal(err)
	}
	account := storage.ChannelAccount{ChannelID: ch.ID, IsPrimary: true}
	if err := db.Create(&account).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&storage.ChannelRelayAccountLink{ChannelAccountID: account.ID, RelayStationID: 4, RelayAccountExternalID: 20}).Error; err != nil {
		t.Fatal(err)
	}
	s := NewService(storage.NewRelayStations(db), cipher, channels, nil, nil, proxies)
	target, err := s.prepareDirectPelican(ch, "manual-key", storage.PelicanConfig{Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	defer target.close()
	html, err := target.generate(context.Background(), storage.PelicanConfig{Model: "test-model", Prompt: "draw"})
	if err != nil || html != "<svg></svg>" {
		t.Fatalf("html=%q err=%v", html, err)
	}
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/accounts/20/models" {
			t.Errorf("unexpected admin request: %s", r.URL.Path)
		}
		w.Write([]byte(`{"code":0,"data":["test-model"]}`))
	}))
	defer admin.Close()
	station := storage.RelayStation{ID: 4, Type: "sub2api", BaseURL: admin.URL}
	key, sourceID, err := s.resolvePelicanChannelKey(station, 20, 0)
	if err != nil || sourceID != ch.ID {
		t.Fatalf("intermediary lost source: id=%d err=%v", sourceID, err)
	}
	viaStation, err := s.preparePelicanForStation(context.Background(), station, "admin-key", key, storage.RelayAccount{BaseURL: ch.SiteURL}, storage.PelicanConfig{AccountExternalID: 20, ChannelID: sourceID, Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	defer viaStation.close()
	if html, err := viaStation.generate(context.Background(), storage.PelicanConfig{Model: "test-model", Prompt: "draw"}); err != nil || html != "<svg></svg>" {
		t.Fatalf("intermediary request bypassed channel proxy: html=%q err=%v", html, err)
	}
	linked := &pelicanTarget{client: *s.client}
	if err := s.applyChannelProxy(linked, ch); err != nil {
		t.Fatal(err)
	}
	defer linked.close()
	if linked.transport == nil {
		t.Fatal("linked target did not inherit channel proxy")
	}
	missing := uint(9999)
	ch.ProxyID = &missing
	if _, err := s.prepareDirectPelican(ch, "manual-key", storage.PelicanConfig{Model: "test-model"}); err == nil {
		t.Fatal("missing proxy must fail closed")
	}
}

func TestPelicanKeySourceRetainsProxyAndRejectsAmbiguousExit(t *testing.T) {
	s, mock := pelicanExecutionDB(t)
	s.cipher, _ = appcrypto.NewCipher("channel-proxy-source")
	keyCipher, _ := s.cipher.Encrypt("generation-key")
	for _, ambiguous := range []bool{false, true} {
		rows := sqlmock.NewRows([]string{"channel_id", "ciphertext", "proxy_id"}).AddRow(11, keyCipher, 7)
		if ambiguous {
			rows.AddRow(12, keyCipher, 8)
		}
		mock.ExpectQuery(`SELECT DISTINCT channels.id AS channel_id, channels.pelican_key_cipher AS ciphertext`).WithArgs(uint(4), int64(20)).WillReturnRows(rows)
		key, sourceID, err := s.resolvePelicanChannelKey(storage.RelayStation{ID: 4}, 20, 0)
		if ambiguous {
			if err == nil {
				t.Fatal("same key with different proxies must require an explicit channel")
			}
		} else if err != nil || key != "generation-key" || sourceID != 11 {
			t.Fatalf("key source lost: id=%d err=%v", sourceID, err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPelicanKeyGroupUsesEachChannelProxy(t *testing.T) {
	proxyFor := func(wantUser, body string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Host != wantUser+".invalid" {
				t.Errorf("channel used another channel proxy: got %s want %s", r.URL.Host, wantUser)
			}
			if r.URL.Path != "/v1/sub2api/billing" && r.Header.Get("Authorization") != "Bearer "+wantUser {
				t.Error("wrong channel session")
			}
			if r.URL.Path == "/api/v1/groups/rates" {
				w.Write([]byte(`{"code":0,"data":{"1":1}}`))
				return
			}
			if r.URL.Path == "/v1/sub2api/billing" {
				http.NotFound(w, r)
				return
			}
			w.Write([]byte(body))
		}))
	}
	a := proxyFor("a", `{"code":0,"data":{"items":[],"pages":1}}`)
	b := proxyFor("b", `{"code":0,"data":{"items":[{"key":"generation-key","group":{"id":1,"name":"matched"}}],"pages":1}}`)
	defer a.Close()
	defer b.Close()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "proxy.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := db.AutoMigrate(&storage.ProxyConfig{}, &storage.Channel{}, &storage.ChannelAccount{}, &storage.ChannelRelayAccountLink{}, &storage.AuthSession{}); err != nil {
		t.Fatal(err)
	}
	cipher, _ := appcrypto.NewCipher("proxy-lookup-test")
	proxies := storage.NewProxies(db)
	var first storage.Channel
	for index, proxy := range []*httptest.Server{a, b} {
		user := []string{"a", "b"}[index]
		urlCipher, _ := cipher.Encrypt(proxy.URL)
		proxyConfig := storage.ProxyConfig{URLCipher: urlCipher, URLHash: user}
		if err := proxies.CreateOrFind(&proxyConfig); err != nil {
			t.Fatal(err)
		}
		credentialCipher, _ := cipher.Encrypt(`{"access_token":"` + user + `"}`)
		ch := storage.Channel{Name: user, SiteURL: "http://" + user + ".invalid", Type: storage.ChannelTypeSub2API, CredentialMode: storage.CredentialModeToken, PasswordCipher: credentialCipher, ProxyID: &proxyConfig.ID}
		if err := db.Create(&ch).Error; err != nil {
			t.Fatal(err)
		}
		account := storage.ChannelAccount{ChannelID: ch.ID, IsPrimary: true}
		if err := db.Create(&account).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&storage.ChannelRelayAccountLink{ChannelAccountID: account.ID, RelayStationID: 4, RelayAccountExternalID: 20}).Error; err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			first = ch
		}
	}
	s := NewService(storage.NewRelayStations(db), cipher, storage.NewChannels(db), nil, nil, proxies)
	target := &pelicanTarget{client: *a.Client(), token: "generation-key"}
	if err := s.applyChannelProxy(target, first); err != nil {
		t.Fatal(err)
	}
	defer target.close()
	info := s.pelicanKeyGroup(context.Background(), target, 4, 20)
	if info.Name != "matched" {
		t.Fatalf("metadata not found through channel B proxy: %#v", info)
	}
}
