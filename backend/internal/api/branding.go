package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

func registerBranding(g *gin.RouterGroup, d *Deps) {
	g.GET("/branding", func(c *gin.Context) {
		settings := storage.BrandingSettings{ProductName: storage.DefaultProductName}
		if d.Settings != nil {
			var err error
			settings, err = d.Settings.BrandingSettings()
			if err != nil {
				fail(c, http.StatusInternalServerError, err)
				return
			}
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": settings})
	})
}
