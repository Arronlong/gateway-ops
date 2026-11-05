package api

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPelicanThumbnailUploadValidationAndCachedRead(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "thumb.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := db.AutoMigrate(&storage.PelicanResult{}); err != nil {
		t.Fatal(err)
	}
	result := storage.PelicanResult{ID: 12, HTML: "<svg/>", Status: "success"}
	if err := db.Create(&result).Error; err != nil {
		t.Fatal(err)
	}
	db.Exec("UPDATE relay_pelican_results SET channel_id = 9 WHERE id = 12")
	router := gin.New()
	registerChannels(router.Group(""), &Deps{RelayStations: storage.NewRelayStations(db)})
	url := "/channels/9/pelican/results/12/thumbnail"
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 448, 336)), &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	source := fmt.Sprintf("%x", sha256.Sum256([]byte("<svg/>")))
	for _, test := range []struct {
		contentType, source string
		body                []byte
		want                int
	}{
		{"image/jpeg", source, bytes.Repeat([]byte{1}, storage.PelicanThumbnailMaxBytes+1), 413},
		{"image/svg+xml", source, []byte("<svg/>"), 415},
		{"image/jpeg", source, []byte("invalid"), 400},
		{"image/jpeg", "stale", encoded.Bytes(), 409},
		{"image/jpeg", source, encoded.Bytes(), 204},
	} {
		response := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, url, bytes.NewReader(test.body))
		req.Header.Set("Content-Type", test.contentType)
		req.Header.Set("X-Pelican-Source-SHA256", test.source)
		router.ServeHTTP(response, req)
		if response.Code != test.want {
			t.Fatalf("upload=%d want=%d body=%s", response.Code, test.want, response.Body.String())
		}
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, url+"?v=1", nil))
	if response.Code != 200 || response.Header().Get("Content-Type") != "image/jpeg" || response.Header().Get("Cache-Control") == "" {
		t.Fatalf("read=%d %v", response.Code, response.Header())
	}
	if _, err := jpeg.Decode(bytes.NewReader(response.Body.Bytes())); err != nil {
		t.Fatal(err)
	}
	if response.Body.Len() > storage.PelicanThumbnailMaxBytes {
		t.Fatal("stored image exceeds byte limit")
	}
	req := httptest.NewRequest(http.MethodGet, url+"?v=1", nil)
	req.Header.Set("If-None-Match", response.Header().Get("ETag"))
	cached := httptest.NewRecorder()
	router.ServeHTTP(cached, req)
	if cached.Code != 304 || cached.Body.Len() != 0 {
		t.Fatalf("cache response=%d", cached.Code)
	}
	wrong := httptest.NewRecorder()
	router.ServeHTTP(wrong, httptest.NewRequest(http.MethodGet, "/channels/10/pelican/results/12/thumbnail", nil))
	if wrong.Code != 404 {
		t.Fatal("wrong owner can read thumbnail")
	}
}

func TestPelicanThumbnailRejectsExcessiveDimensions(t *testing.T) {
	var data bytes.Buffer
	_ = jpeg.Encode(&data, image.NewRGBA(image.Rect(0, 0, 1024, 768)), nil)
	if _, err := optimizePelicanThumbnail(data.Bytes()); err == nil {
		t.Fatal("oversized thumbnail accepted")
	}
}

func TestPublicPelicanThumbnailRechecksVisibilityBeforeCache(t *testing.T) {
	for _, monitored := range []bool{false, true} {
		t.Run(fmt.Sprint(monitored), func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer sqlDB.Close()
			db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			mock.ExpectQuery(`SELECT .*relay_groups`).WillReturnRows(sqlmock.NewRows([]string{"relay_station_id", "external_id", "monitor_enabled"}).AddRow(7, 20, monitored))
			if monitored {
				mock.ExpectQuery(`SELECT .*relay_stations`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
				mock.ExpectQuery(`SELECT .*relay_group_pelican_results.*status =`).WillReturnRows(sqlmock.NewRows([]string{"id", "thumbnail"}).AddRow(12, []byte("jpeg")))
			}
			router := gin.New()
			registerPublicRelayStations(router.Group(""), &Deps{RelayStations: storage.NewRelayStations(db)})
			req := httptest.NewRequest(http.MethodGet, "/relay-stations/7/groups/20/pelican/results/12/thumbnail", nil)
			req.Header.Set("If-None-Match", "*")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			want := http.StatusNotFound
			if monitored {
				want = http.StatusNotModified
			}
			if response.Code != want {
				t.Fatalf("visibility response=%d want=%d body=%s", response.Code, want, response.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
