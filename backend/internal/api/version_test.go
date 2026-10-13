package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/guliansheng/gateway-ops/internal/auth"
	"github.com/guliansheng/gateway-ops/internal/version"
)

func TestVersionRoutesReturnBuildInfoAndUpdateState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"tag_name":"v1.1.0"}`)) }))
	defer server.Close()
	router := gin.New()
	Register(router, &Deps{VersionChecker: version.NewChecker(version.Info{Name: "GatewayOps", Version: "1.0.0", Revision: "abc123", BuildTime: "2026-10-13T00:00:00Z"}, server.Client(), server.URL)})
	for _, path := range []string{"/api/version", "/api/version/check"} {
		res := httptest.NewRecorder()
		router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		if res.Code != http.StatusOK {
			t.Fatalf("%s: HTTP %d: %s", path, res.Code, res.Body)
		}
		var body map[string]any
		if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if path == "/api/version" {
			if body["version"] != "1.0.0" || body["revision"] != "abc123" {
				t.Fatalf("build metadata: %+v", body)
			}
		} else if body["update_available"] != true || body["latest_version"] != "1.1.0" {
			t.Fatalf("update result: %+v", body)
		}
	}
}

func TestVersionRouteUsesLatestStableDefault(t *testing.T) {
	router := gin.New()
	Register(router, &Deps{})
	res := httptest.NewRecorder()
	router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/version", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", res.Code, res.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["version"] != "1.0.1" || body["channel"] != "stable" {
		t.Fatalf("version response: %+v", body)
	}
}

func TestVersionCheckFailureDoesNotReturnUpToDate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer server.Close()
	router := gin.New()
	Register(router, &Deps{VersionChecker: version.NewChecker(version.Info{Version: "1.0.0"}, server.Client(), server.URL)})
	res := httptest.NewRecorder()
	router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/version/check", nil))
	var body map[string]any
	json.Unmarshal(res.Body.Bytes(), &body)
	if res.Code != http.StatusBadGateway || body["status"] != "check_failed" || body["update_available"] != nil {
		t.Fatalf("HTTP %d: %s", res.Code, res.Body)
	}
}

func TestVersionCheckRequiresAuthentication(t *testing.T) {
	service, err := auth.New("admin", "test-password", "test-secret", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	Register(router, &Deps{Auth: service})
	for _, tc := range []struct {
		path string
		want int
	}{{"/api/version", 200}, {"/api/version/check", 401}} {
		res := httptest.NewRecorder()
		router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if res.Code != tc.want {
			t.Fatalf("%s: HTTP %d, want %d", tc.path, res.Code, tc.want)
		}
	}
}
