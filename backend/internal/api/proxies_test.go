package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/guliansheng/gateway-ops/internal/channel"
	appcrypto "github.com/guliansheng/gateway-ops/internal/crypto"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestGlobalProxyCanBeReusedByChannels(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "proxy.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := db.AutoMigrate(&storage.ProxyConfig{}, &storage.Channel{}, &storage.ChannelAccount{}, &storage.AuthSession{}); err != nil {
		t.Fatal(err)
	}
	cipher, _ := appcrypto.NewCipher("proxy-test-secret")
	proxies := storage.NewProxies(db)
	channels := storage.NewChannels(db)
	svc := channel.NewService(channels, storage.NewAuthSessions(db), nil, nil, cipher)
	svc.Proxies = proxies
	deps := &Deps{Proxies: proxies, Channels: channels, ChannelSvc: svc, Cipher: cipher}
	router := gin.New()
	registerProxies(router.Group("/api"), deps)
	registerChannels(router.Group("/api"), deps)
	request := func(method, path, body string, status int) []byte {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("%s %s status=%d body=%s", method, path, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "proxy-password") {
			t.Fatal("proxy credential leaked")
		}
		return w.Body.Bytes()
	}
	for _, raw := range []string{`{"url":"socks5://localhost:1080"}`, `{"url":"http://user:proxy-password@host:bad"}`, `{"url":"http://host/path"}`} {
		request("POST", "/api/proxies", raw, http.StatusBadRequest)
	}
	var created struct {
		Data storage.ProxyConfig `json:"data"`
	}
	input := `{"url":"http://proxy-user:proxy-password@127.0.0.1:7890"}`
	if err := json.Unmarshal(request("POST", "/api/proxies", input, 200), &created); err != nil {
		t.Fatal(err)
	}
	proxyID := created.Data.ID
	if proxyID == 0 {
		t.Fatal("missing proxy ID")
	}
	request("POST", "/api/proxies", input, 200)
	var listed struct {
		Data []storage.ProxyConfig `json:"data"`
	}
	json.Unmarshal(request("GET", "/api/proxies", "", 200), &listed)
	if len(listed.Data) != 1 {
		t.Fatalf("proxy count=%d", len(listed.Data))
	}
	stored, err := proxies.FindByID(proxyID)
	if err != nil || strings.Contains(stored.URLCipher, "proxy-password") {
		t.Fatal("proxy URL must be encrypted")
	}
	for _, name := range []string{"first", "second"} {
		body := `{"name":"` + name + `","type":"newapi","site_url":"http://upstream.invalid","username":"monitor","password":"test","monitor_enabled":false,"proxy_id":` + strconv.Itoa(int(proxyID)) + `}`
		request("POST", "/api/channels", body, 200)
	}
	for _, id := range []uint{1, 2} {
		ch, _ := channels.FindByID(id)
		if ch.ProxyID == nil || *ch.ProxyID != proxyID {
			t.Fatal("proxy binding not persisted")
		}
		resolved, err := svc.Resolve(context.Background(), ch)
		if err != nil || resolved.ProxyURL == nil || resolved.ProxyURL.User.Username() != "proxy-user" {
			t.Fatalf("proxy not resolved: %v", err)
		}
		child := svc.AccountChannel(ch, storage.ChannelAccount{ID: 3})
		if child.ProxyID == nil || *child.ProxyID != proxyID {
			t.Fatal("additional account lost channel proxy")
		}
	}
	request("GET", "/api/channels/1", "", 200)
	request("PUT", "/api/channels/1", `{"remark":"updated"}`, 200)
	first, _ := channels.FindByID(1)
	if first.ProxyID == nil {
		t.Fatal("omitted proxy field cleared binding")
	}
	request("PUT", "/api/channels/1", `{"proxy_id":9999}`, 400)
	request("PUT", "/api/channels/1", `{"proxy_id":null}`, 200)
	first, _ = channels.FindByID(1)
	second, _ := channels.FindByID(2)
	if first.ProxyID != nil || second.ProxyID == nil {
		t.Fatal("clearing a binding changed another channel")
	}
	request("PUT", "/api/channels/1", `{"proxy_id":`+strconv.Itoa(int(proxyID))+`}`, 200)
	request("PUT", "/api/channels/1", `{"proxy_id":0}`, 200)
	first, _ = channels.FindByID(1)
	if first.ProxyID != nil {
		t.Fatal("zero proxy ID must clear binding")
	}
}
