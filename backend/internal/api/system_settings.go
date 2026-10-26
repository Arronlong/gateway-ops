package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/guliansheng/gateway-ops/internal/security"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

func registerSystemSettings(g *gin.RouterGroup, d *Deps) {
	g.GET("/system-settings", func(c *gin.Context) {
		if d.Settings == nil {
			fail(c, http.StatusServiceUnavailable, errors.New("system settings unavailable"))
			return
		}
		settings, err := d.Settings.UISettings()
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": settings})
	})

	g.PUT("/system-settings", func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
		if d.Settings == nil {
			fail(c, http.StatusServiceUnavailable, errors.New("system settings unavailable"))
			return
		}
		settings, err := d.Settings.UISettings()
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		if err := c.ShouldBindJSON(&settings); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		settings.ProductName = storage.NormalizeProductName(settings.ProductName)
		pelican := settings.PelicanSettings()
		settings.PelicanPlans = pelican.Plans
		settings.PelicanModel = pelican.Model
		settings.PelicanPrompt = pelican.Prompt
		settings.PelicanReasoningEffort = pelican.ReasoningEffort
		if err := validateSystemSettings(settings); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		if err := d.Settings.SaveUISettings(settings); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": settings})
	})
}

func validateSystemSettings(settings storage.UISettings) error {
	if err := storage.ValidateBranding(settings.ProductName, settings.ProductIcon); err != nil {
		return err
	}
	if err := storage.ValidatePelicanSettings(settings.PelicanSettings()); err != nil {
		return err
	}
	if settings.RecentMetricsLimit < 1 || settings.RecentMetricsLimit > 200 {
		return fmt.Errorf("recent metrics limit must be between 1 and 200")
	}
	if settings.RecentMetricsWindowHours < 1 || settings.RecentMetricsWindowHours > 24 {
		return fmt.Errorf("recent metrics window hours must be between 1 and 24")
	}
	switch settings.IframeEmbeddingMode {
	case storage.IframeEmbeddingModeAll, storage.IframeEmbeddingModeAllowlist:
	default:
		return fmt.Errorf("invalid iframe embedding mode %q", settings.IframeEmbeddingMode)
	}

	for _, pattern := range settings.IframeEmbeddingAllowlist {
		if err := security.ValidateHostPattern(pattern); err != nil {
			return fmt.Errorf("invalid iframe allowlist pattern %q: %w", pattern, err)
		}
	}
	return nil
}
