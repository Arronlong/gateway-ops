package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/guliansheng/gateway-ops/internal/networkproxy"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

func registerProxies(g *gin.RouterGroup, d *Deps) {
	gp := g.Group("/proxies")
	gp.GET("", func(c *gin.Context) {
		items, err := d.Proxies.List()
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": items})
	})
	gp.POST("", func(c *gin.Context) {
		var input struct {
			URL string `json:"url"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			fail(c, http.StatusBadRequest, networkproxy.ErrInvalidURL)
			return
		}
		u, err := networkproxy.Parse(input.URL)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		cipherText, err := d.Cipher.Encrypt(u.String())
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		hash := sha256.Sum256([]byte(u.String()))
		item := storage.ProxyConfig{DisplayURL: u.Redacted(), URLCipher: cipherText, URLHash: hex.EncodeToString(hash[:])}
		if err := d.Proxies.CreateOrFind(&item); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": item})
	})
}
