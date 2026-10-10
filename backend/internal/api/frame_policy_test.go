package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

func TestFramePolicyHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		settings   storage.UISettings
		referer    string
		wantCSP    string
		wantXFrame string
	}{
		{
			name:       "disabled denies every frame",
			settings:   storage.UISettings{IframeEmbeddingEnabled: false, IframeEmbeddingMode: storage.IframeEmbeddingModeAllowlist},
			wantCSP:    "frame-ancestors 'none'",
			wantXFrame: "DENY",
		},
		{
			name: "allow all emits no frame restriction",
			settings: storage.UISettings{
				IframeEmbeddingEnabled: true,
				IframeEmbeddingMode:    storage.IframeEmbeddingModeAll,
			},
		},
		{
			name: "allowlist exact parent origin",
			settings: storage.UISettings{
				IframeEmbeddingEnabled:   true,
				IframeEmbeddingMode:      storage.IframeEmbeddingModeAllowlist,
				IframeEmbeddingAllowlist: []string{"*.example.com"},
			},
			referer: "https://sub.example.com:8443/page",
			wantCSP: "frame-ancestors https://sub.example.com:8443",
		},
		{
			name: "allowlist supports advanced glob",
			settings: storage.UISettings{
				IframeEmbeddingEnabled:   true,
				IframeEmbeddingMode:      storage.IframeEmbeddingModeAllowlist,
				IframeEmbeddingAllowlist: []string{"**.devai.**"},
			},
			referer: "https://a.b.devai.c.d/embed",
			wantCSP: "frame-ancestors https://a.b.devai.c.d",
		},
		{
			name: "allowlist accepts internal hostname with underscore without invalid CSP host source",
			settings: storage.UISettings{
				IframeEmbeddingEnabled:   true,
				IframeEmbeddingMode:      storage.IframeEmbeddingModeAllowlist,
				IframeEmbeddingAllowlist: []string{"devai_cn.xxx.cn"},
			},
			referer: "https://devai_cn.xxx.cn/embed",
			wantCSP: "",
		},
		{
			name: "allowlist rejects unmatched parent",
			settings: storage.UISettings{
				IframeEmbeddingEnabled:   true,
				IframeEmbeddingMode:      storage.IframeEmbeddingModeAllowlist,
				IframeEmbeddingAllowlist: []string{"*.example.com"},
			},
			referer: "https://evil.example.net/page",
			wantCSP: "frame-ancestors 'none'",
		},
		{
			name: "allowlist fails closed without referer",
			settings: storage.UISettings{
				IframeEmbeddingEnabled:   true,
				IframeEmbeddingMode:      storage.IframeEmbeddingModeAllowlist,
				IframeEmbeddingAllowlist: []string{"*"},
			},
			wantCSP: "frame-ancestors 'none'",
		},
		{
			name: "invalid referer fails closed",
			settings: storage.UISettings{
				IframeEmbeddingEnabled:   true,
				IframeEmbeddingMode:      storage.IframeEmbeddingModeAllowlist,
				IframeEmbeddingAllowlist: []string{"*"},
			},
			referer: "not a url",
			wantCSP: "frame-ancestors 'none'",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := framePolicyHeaders(tt.settings, tt.referer)
			if got.ContentSecurityPolicy != tt.wantCSP {
				t.Fatalf("CSP = %q, want %q", got.ContentSecurityPolicy, tt.wantCSP)
			}
			if got.XFrameOptions != tt.wantXFrame {
				t.Fatalf("X-Frame-Options = %q, want %q", got.XFrameOptions, tt.wantXFrame)
			}
		})
	}
}

func TestRegisterFrontendAppliesSafeFramePolicy(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	dist := fstest.MapFS{
		"index.html": {Data: []byte("<html><body>ok</body></html>")},
	}
	registerFrontend(r, dist, nil)

	req := httptest.NewRequest(http.MethodGet, "/channels", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Security-Policy"); got != "frame-ancestors 'none'" {
		t.Fatalf("CSP = %q", got)
	}
	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q", got)
	}
}
