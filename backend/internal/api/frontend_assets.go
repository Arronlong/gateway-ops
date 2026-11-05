package api

import (
	"bytes"
	"compress/gzip"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type frontendAsset struct {
	plain, gzip []byte
	once        sync.Once
}

func acceptsGzip(value string) bool {
	wildcard := false
	for _, part := range strings.Split(value, ",") {
		fields := strings.Split(part, ";")
		name := strings.TrimSpace(fields[0])
		quality := 1.0
		for _, field := range fields[1:] {
			key, val, found := strings.Cut(strings.TrimSpace(field), "=")
			if found && key == "q" {
				parsed, err := strconv.ParseFloat(val, 64)
				if err != nil {
					quality = 0
				} else {
					quality = parsed
				}
			}
		}
		if name == "gzip" {
			return quality > 0
		}
		if name == "*" {
			wildcard = quality > 0
		}
	}
	return wildcard
}

func frontendFiles(dist fs.FS) func(*gin.Context, string) {
	var assets sync.Map
	return func(c *gin.Context, name string) {
		value, _ := assets.LoadOrStore(name, &frontendAsset{})
		asset := value.(*frontendAsset)
		asset.once.Do(func() {
			asset.plain, _ = fs.ReadFile(dist, name)
			if len(asset.plain) < 1024 {
				return
			}
			switch path.Ext(name) {
			case ".js", ".css", ".html", ".svg", ".json":
			default:
				return
			}
			var compressed bytes.Buffer
			writer, _ := gzip.NewWriterLevel(&compressed, gzip.BestCompression)
			_, _ = writer.Write(asset.plain)
			_ = writer.Close()
			if compressed.Len() < len(asset.plain) {
				asset.gzip = compressed.Bytes()
			}
		})
		if asset.plain == nil {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.Header("Vary", "Accept-Encoding")
		if strings.HasPrefix(name, "assets/") {
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			c.Header("Cache-Control", "no-cache")
		}
		data := asset.plain
		if len(asset.gzip) > 0 && acceptsGzip(c.GetHeader("Accept-Encoding")) {
			data = asset.gzip
			c.Header("Content-Encoding", "gzip")
		}
		if strings.HasSuffix(name, ".js") {
			c.Header("Content-Type", "text/javascript; charset=utf-8")
		}
		http.ServeContent(c.Writer, c.Request, path.Base(name), time.Time{}, bytes.NewReader(data))
	}
}
