package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/guliansheng/gateway-ops/internal/version"
)

func registerVersion(g *gin.RouterGroup, checker *version.Checker) {
	if checker == nil {
		checker = version.NewChecker(version.Current(), nil, "")
	}
	g.GET("/version", func(c *gin.Context) { c.JSON(http.StatusOK, checker.Info()) })
	// 远端查询沿用后台鉴权，避免公开访客消耗 GitHub 配额。
	g.GET("/version/check", func(c *gin.Context) {
		result, err := checker.Check(c.Request.Context())
		status := http.StatusOK
		if err != nil {
			status = http.StatusBadGateway
		}
		c.JSON(status, result)
	})
}
