package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func brandingRouter(t *testing.T) (*gin.Engine, *storage.Settings) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "branding.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&storage.AppSetting{}, &storage.PelicanConfig{}, &storage.GroupPelicanConfig{}); err != nil {
		t.Fatal(err)
	}
	settings := storage.NewSettings(db)
	router := gin.New()
	registerSystemSettings(router.Group(""), &Deps{Settings: settings})
	return router, settings
}

func brandingRequest(t *testing.T, router *gin.Engine, method, path string, value any, status int) map[string]any {
	t.Helper()
	body, _ := json.Marshal(value)
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != status {
		t.Fatalf("%s %s: status=%d want=%d body=%s", method, path, response.Code, status, response.Body.String())
	}
	var result struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(response.Body.Bytes(), &result)
	return result.Data
}

func brandingPNG(t *testing.T, width int) string {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, width, width))); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestBrandingSettingsPersistDefaultsAndPartialUpdates(t *testing.T) {
	router, _ := brandingRouter(t)
	defaults := brandingRequest(t, router, "GET", "/system-settings", nil, 200)
	if defaults["product_name"] != "GatewayOps" || defaults["product_icon"] != "" {
		t.Fatalf("defaults=%v", defaults)
	}
	icon := brandingPNG(t, 128)
	saved := brandingRequest(t, router, "PUT", "/system-settings", map[string]any{"product_name": "  测试产品  ", "product_icon": icon}, 200)
	if saved["product_name"] != "测试产品" || saved["product_icon"] != icon {
		t.Fatal("brand fields not saved")
	}
	saved = brandingRequest(t, router, "PUT", "/system-settings", map[string]any{"recent_metrics_limit": 100}, 200)
	if saved["product_name"] != "测试产品" || saved["product_icon"] != icon {
		t.Fatal("partial update lost brand")
	}
	read := brandingRequest(t, router, "GET", "/system-settings", nil, 200)
	if read["product_name"] != "测试产品" || read["product_icon"] != icon {
		t.Fatal("brand not persisted")
	}
	saved = brandingRequest(t, router, "PUT", "/system-settings", map[string]any{"product_name": " ", "product_icon": ""}, 200)
	if saved["product_name"] != "GatewayOps" || saved["product_icon"] != "" {
		t.Fatal("reset did not restore defaults")
	}
}

func TestBrandingSettingsRejectInvalidInput(t *testing.T) {
	router, _ := brandingRouter(t)
	for _, input := range []map[string]any{
		{"product_name": strings.Repeat("名", 41)},
		{"product_name": "bad\nname"},
		{"product_icon": "https://example.com/icon.png"},
		{"product_icon": "data:image/svg+xml;base64,PHN2Zz4="},
		{"product_icon": "data:image/png;base64,AAAA"},
		{"product_icon": brandingPNG(t, 129)},
		{"product_icon": "data:image/png;base64," + strings.Repeat("A", 800000)},
	} {
		brandingRequest(t, router, "PUT", "/system-settings", input, 400)
	}
}

func TestPublicBrandingOnlyExposesSavedBrandFields(t *testing.T) {
	router, settings := brandingRouter(t)
	if err := settings.Set(storage.SettingProductName, "公开产品"); err != nil {
		t.Fatal(err)
	}
	if err := settings.Set(storage.SettingProductIcon, brandingPNG(t, 32)); err != nil {
		t.Fatal(err)
	}
	if err := settings.Set(storage.SettingPelicanPrompt, "private prompt"); err != nil {
		t.Fatal(err)
	}
	registerBranding(router.Group("/api/public"), &Deps{Settings: settings})
	data := brandingRequest(t, router, "GET", "/api/public/branding", nil, 200)
	if len(data) != 2 || data["product_name"] != "公开产品" || data["product_icon"] == "" {
		t.Fatalf("unexpected public data: %v", data)
	}
}

func TestBrandingRequestBodyIsBounded(t *testing.T) {
	router, _ := brandingRouter(t)
	brandingRequest(t, router, "PUT", "/system-settings", map[string]any{"product_icon": strings.Repeat("x", 3<<20)}, 400)
}
