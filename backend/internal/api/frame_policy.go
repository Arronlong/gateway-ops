package api

import (
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/guliansheng/gateway-ops/internal/security"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

type framePolicyHeaderSet struct {
	ContentSecurityPolicy string
	XFrameOptions         string
}

func framePolicyHeaders(settings storage.UISettings, referer string) framePolicyHeaderSet {
	if !settings.IframeEmbeddingEnabled {
		return framePolicyHeaderSet{
			ContentSecurityPolicy: "frame-ancestors 'none'",
			XFrameOptions:         "DENY",
		}
	}

	if settings.IframeEmbeddingMode == storage.IframeEmbeddingModeAll {
		return framePolicyHeaderSet{}
	}
	if settings.IframeEmbeddingMode != storage.IframeEmbeddingModeAllowlist {
		return framePolicyHeaderSet{ContentSecurityPolicy: "frame-ancestors 'none'"}
	}

	origin, host, ok := iframeParentOrigin(referer)
	if !ok {
		return framePolicyHeaderSet{ContentSecurityPolicy: "frame-ancestors 'none'"}
	}

	matched, err := security.MatchAnyHostPattern(settings.IframeEmbeddingAllowlist, host)
	if err != nil || !matched {
		return framePolicyHeaderSet{ContentSecurityPolicy: "frame-ancestors 'none'"}
	}
	if !cspHostSourceCompatible(host) {
		return framePolicyHeaderSet{}
	}
	return framePolicyHeaderSet{ContentSecurityPolicy: "frame-ancestors " + origin}
}

func cspHostSourceCompatible(host string) bool {
	for _, r := range strings.ToLower(strings.TrimSpace(host)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return host != ""
}

func iframeParentOrigin(referer string) (origin, host string, ok bool) {
	if strings.TrimSpace(referer) == "" {
		return "", "", false
	}
	parsed, err := url.Parse(referer)
	if err != nil || parsed.User != nil || parsed.Host == "" {
		return "", "", false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", "", false
	}
	host = parsed.Hostname()
	if host == "" {
		return "", "", false
	}
	return scheme + "://" + strings.ToLower(parsed.Host), strings.ToLower(host), true
}

func applyFramePolicy(c *gin.Context, settingsStore *storage.Settings) {
	settings := storage.UISettings{
		IframeEmbeddingEnabled: false,
		IframeEmbeddingMode:    storage.IframeEmbeddingModeAllowlist,
	}
	if settingsStore != nil {
		loaded, err := settingsStore.UISettings()
		if err == nil {
			settings = loaded
		}
	}

	headers := framePolicyHeaders(settings, c.Request.Referer())
	if headers.ContentSecurityPolicy == "" {
		c.Writer.Header().Del("Content-Security-Policy")
	} else {
		c.Writer.Header().Set("Content-Security-Policy", headers.ContentSecurityPolicy)
	}
	if headers.XFrameOptions == "" {
		c.Writer.Header().Del("X-Frame-Options")
	} else {
		c.Writer.Header().Set("X-Frame-Options", headers.XFrameOptions)
	}
}
