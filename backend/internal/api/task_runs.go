package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/guliansheng/gateway-ops/internal/scheduler"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
)

func registerTaskRuns(g *gin.RouterGroup, d *Deps) {
	repo := storage.NewTaskRuns(d.DB)
	validTask := func(c *gin.Context) bool {
		if _, ok := scheduler.TaskNames[c.Param("task")]; !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "\u4efb\u52a1\u4e0d\u5b58\u5728"})
			return false
		}
		return true
	}
	g.POST("/tasks/:task/run", func(c *gin.Context) {
		if !validTask(c) {
			return
		}
		if c.Param("task") != "channel_usage_integrity" {
			c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "\u8be5\u4efb\u52a1\u6682\u4e0d\u652f\u6301\u624b\u52a8\u6267\u884c"})
			return
		}
		if d.Scheduler == nil || !d.Scheduler.RunChannelUsageIntegrity() {
			c.JSON(http.StatusConflict, gin.H{"error": "\u5168\u6e20\u9053\u7528\u91cf\u6838\u5bf9\u4efb\u52a1\u6b63\u5728\u6267\u884c\u6216\u6682\u4e0d\u53ef\u7528\uff0c\u8bf7\u67e5\u770b\u6267\u884c\u8bb0\u5f55"})
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"data": gin.H{"accepted": true}})
	})
	g.GET("/tasks/:task/runs", func(c *gin.Context) {
		if !validTask(c) {
			return
		}
		page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
		size, sizeErr := strconv.Atoi(c.DefaultQuery("page_size", "20"))
		if err != nil || sizeErr != nil || page < 1 || page > 1000000 || size < 1 || size > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "\u5206\u9875\u53c2\u6570\u65e0\u6548"})
			return
		}
		result, err := repo.List(c.Param("task"), page, size)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": result})
	})
	g.GET("/tasks/:task/runs/:id", func(c *gin.Context) {
		if !validTask(c) {
			return
		}
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil || id == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "\u6267\u884c\u8bb0\u5f55\u7f16\u53f7\u65e0\u6548"})
			return
		}
		result, err := repo.Find(c.Param("task"), uint(id))
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "\u6267\u884c\u8bb0\u5f55\u4e0d\u5b58\u5728"})
			return
		}
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": result})
	})
}
