package api

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
)

var errPelicanThumbnailLookup = errors.New("thumbnail storage lookup failed")

func pelicanThumbnailLookupError(err error) error {
	if err == nil || errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return fmt.Errorf("%w: %w", errPelicanThumbnailLookup, err)
}

func pelicanThumbnailStorageFailure(c *gin.Context, d *Deps, operation string, err error) {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	log.Error("pelican thumbnail storage failed", "operation", operation, "path", c.Request.URL.Path, "error", err)
	c.Header("Cache-Control", "no-store")
	fail(c, http.StatusInternalServerError, fmt.Errorf("thumbnail storage %s failed; check server logs", operation))
}

func optimizePelicanThumbnail(data []byte) ([]byte, error) {
	config, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if config.Width < 1 || config.Height < 1 || config.Width > storage.PelicanThumbnailWidth || config.Height > storage.PelicanThumbnailHeight {
		return nil, errors.New("thumbnail dimensions exceed 448x336")
	}
	picture, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	for _, quality := range []int{80, 65, 50, 35} {
		var encoded bytes.Buffer
		if err := jpeg.Encode(&encoded, picture, &jpeg.Options{Quality: quality}); err != nil {
			return nil, err
		}
		if encoded.Len() <= storage.PelicanThumbnailMaxBytes {
			return encoded.Bytes(), nil
		}
	}
	return nil, errors.New("optimized thumbnail exceeds 96 KiB")
}

func pelicanThumbnailRef(c *gin.Context, d *Deps, scope string, public bool) (storage.PelicanArtifactRef, error) {
	var ref storage.PelicanArtifactRef
	var err error
	ref.ID, err = uintParam(c, "result_id")
	if err != nil {
		return ref, err
	}
	switch scope {
	case "channel":
		ref.ChannelID, err = uintParam(c, "id")
	case "group":
		ref.StationID, ref.GroupID, err = groupPelicanIDs(c)
	case "account":
		ref.StationID, ref.AccountID, err = pelicanIDs(c)
	default:
		return ref, errors.New("invalid artwork scope")
	}
	if err != nil {
		return ref, err
	}
	if public {
		if err := d.RelayStations.PublicPelicanGroup(ref.StationID, ref.GroupID); err != nil {
			return ref, pelicanThumbnailLookupError(err)
		}
		if d.Settings != nil {
			enabled, err := pelicanPublicPageEnabled(d)
			if err != nil {
				return ref, pelicanThumbnailLookupError(err)
			}
			if !enabled {
				return ref, errors.New("pelican page is hidden")
			}
		}
	} else if ref.StationID > 0 {
		_, err = d.RelayStations.FindByID(ref.StationID)
		err = pelicanThumbnailLookupError(err)
	}
	return ref, err
}

func pelicanThumbnail(c *gin.Context, d *Deps, scope string, public bool) {
	ref, err := pelicanThumbnailRef(c, d, scope, public)
	if err != nil {
		if errors.Is(err, errPelicanThumbnailLookup) {
			pelicanThumbnailStorageFailure(c, d, "lookup", err)
		} else {
			fail(c, http.StatusNotFound, errors.New("artwork not found"))
		}
		return
	}
	if c.Request.Method == http.MethodPut {
		if c.ContentType() != "image/jpeg" {
			fail(c, http.StatusUnsupportedMediaType, errors.New("thumbnail must be image/jpeg"))
			return
		}
		data, err := io.ReadAll(io.LimitReader(c.Request.Body, storage.PelicanThumbnailMaxBytes+1))
		if err != nil {
			fail(c, 400, err)
			return
		}
		if len(data) > storage.PelicanThumbnailMaxBytes {
			fail(c, http.StatusRequestEntityTooLarge, errors.New("thumbnail exceeds 96 KiB"))
			return
		}
		data, err = optimizePelicanThumbnail(data)
		if err != nil {
			fail(c, 400, err)
			return
		}
		html, err := d.RelayStations.PelicanThumbnailSource(ref)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				fail(c, 404, errors.New("artwork not found"))
			} else {
				pelicanThumbnailStorageFailure(c, d, "source", err)
			}
			return
		}
		if fmt.Sprintf("%x", sha256.Sum256([]byte(html))) != c.GetHeader("X-Pelican-Source-SHA256") {
			fail(c, http.StatusConflict, errors.New("artwork changed; regenerate its thumbnail"))
			return
		}
		if err := d.RelayStations.SavePelicanThumbnail(ref, html, data); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				fail(c, http.StatusConflict, errors.New("artwork changed; regenerate its thumbnail"))
			} else {
				pelicanThumbnailStorageFailure(c, d, "write", err)
			}
			return
		}
		c.Status(http.StatusNoContent)
		return
	}
	data, err := d.RelayStations.PelicanThumbnail(ref)
	if err != nil {
		c.Header("Cache-Control", "no-store")
		if errors.Is(err, gorm.ErrRecordNotFound) {
			fail(c, 404, errors.New("thumbnail not found"))
		} else {
			pelicanThumbnailStorageFailure(c, d, "read", err)
		}
		return
	}
	etag := fmt.Sprintf("\"%x\"", sha256.Sum256(data))
	c.Header("Cache-Control", "private, max-age=3600")
	if public {
		c.Header("Cache-Control", "private, no-cache")
	}
	c.Header("ETag", etag)
	c.Header("Vary", "Authorization")
	c.Header("X-Content-Type-Options", "nosniff")
	for _, tag := range strings.Split(c.GetHeader("If-None-Match"), ",") {
		if strings.TrimSpace(tag) == etag || strings.TrimSpace(tag) == "*" {
			c.Status(http.StatusNotModified)
			return
		}
	}
	c.Data(http.StatusOK, "image/jpeg", data)
}
