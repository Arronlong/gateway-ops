package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTokenRefreshFrequencyRejectsInvalidValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerSyncSettings(router.Group(""), &Deps{})
	for _, value := range []string{"0", "-1", "1.5", "\"30\""} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPut, "/sync-settings", strings.NewReader(`{"channel_token_refresh_enabled":true,"channel_token_refresh_interval_minutes":`+value+`}`))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("value %s HTTP %d", value, response.Code)
		}
	}
}
