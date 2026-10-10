package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

func TestPublicRelayStationOptionsExposeOnlySafeIdentityFields(t *testing.T) {
	options := publicRelayStationOptions([]storage.RelayStation{
		{ID: 7, Name: "Tokyo", BaseURL: "https://secret.example", APIKeyCipher: "ciphertext"},
		{ID: 9, Name: "Singapore", BaseURL: "https://internal.example", APIKeyCipher: "ciphertext-2"},
	})

	if len(options) != 2 {
		t.Fatalf("len(options) = %d, want 2", len(options))
	}
	if options[0].ID != 7 || options[0].Name != "Tokyo" || options[1].ID != 9 || options[1].Name != "Singapore" {
		t.Fatalf("unexpected options: %#v", options)
	}

	raw, err := json.Marshal(options)
	if err != nil {
		t.Fatal(err)
	}
	payload := string(raw)
	for _, forbidden := range []string{"base_url", "api_key", "ciphertext", "secret.example", "internal.example"} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("public options leaked %q: %s", forbidden, payload)
		}
	}
}

func TestPublicRelayGroupMonitorRejectsUnsupportedRangeBeforeLoadingStation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/relay-stations/:id/group-monitor", func(c *gin.Context) {
		publicRelayGroupMonitor(c, nil)
	})
	for _, rangeName := range []string{"all", "90d", "invalid"} {
		t.Run(rangeName, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/relay-stations/3/group-monitor?range="+rangeName, nil))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body = %s", response.Code, response.Body.String())
			}
		})
	}
}
