package api

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

func TestFrontendAssetsCompressCacheAndPreserveSPA(t *testing.T) {
	gin.SetMode(gin.TestMode)
	script := strings.Repeat("const thumbnail = 'cached';\n", 1000)
	router := gin.New()
	registerFrontend(router, fstest.MapFS{"index.html": {Data: []byte("<html>app</html>")}, "assets/app-a123.js": {Data: []byte(script)}}, nil)
	for _, test := range []struct {
		encoding   string
		compressed bool
	}{{"gzip, br", true}, {"gzip;q=0, br", false}, {"*;q=0.5", true}, {"identity", false}} {
		req := httptest.NewRequest("GET", "/assets/app-a123.js", nil)
		req.Header.Set("Accept-Encoding", test.encoding)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != 200 {
			t.Fatalf("status=%d", response.Code)
		}
		if (response.Header().Get("Content-Encoding") == "gzip") != test.compressed {
			t.Fatalf("encoding=%q, response=%v", test.encoding, response.Header())
		}
		body := response.Body.Bytes()
		if test.compressed {
			reader, err := gzip.NewReader(bytes.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			body, err = io.ReadAll(reader)
			_ = reader.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
		if string(body) != script {
			t.Fatal("compressed asset changed content")
		}
		if response.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
			t.Fatalf("asset caching=%q", response.Header().Get("Cache-Control"))
		}
		if !strings.Contains(response.Header().Get("Vary"), "Accept-Encoding") {
			t.Fatal("missing compression negotiation header")
		}
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/pelican", nil))
	if response.Code != 200 || response.Body.String() != "<html>app</html>" || response.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("SPA response=%d %q %v", response.Code, response.Body.String(), response.Header())
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil))
	if response.Code != 404 {
		t.Fatal("missing lazy chunk returned index.html")
	}
}
