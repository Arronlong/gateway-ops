package api

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/guliansheng/gateway-ops/internal/auth"
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

func TestPelicanThumbnailStorageFailuresAreNotReportedAsMissing(t *testing.T) {
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 448, 336)), nil); err != nil {
		t.Fatal(err)
	}
	for _, stage := range []string{"read", "source", "write", "lookup"} {
		t.Run(stage, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer sqlDB.Close()
			db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{SkipDefaultTransaction: true, Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			failure := errors.New("database temporarily unavailable")
			method := http.MethodPut
			if stage == "lookup" {
				method = http.MethodGet
				mock.ExpectQuery(`SELECT .*relay_groups`).WillReturnError(failure)
			} else if stage == "read" {
				method = http.MethodGet
				mock.ExpectQuery(`SELECT .*relay_pelican_results`).WillReturnError(failure)
			} else if stage == "source" {
				mock.ExpectQuery(`SELECT .*relay_pelican_results`).WillReturnError(failure)
			} else {
				mock.ExpectQuery(`SELECT .*relay_pelican_results`).WillReturnRows(sqlmock.NewRows([]string{"id", "html"}).AddRow(12, "<svg/>"))
				mock.ExpectExec(`UPDATE .*relay_pelican_results`).WillReturnError(failure)
			}
			router := gin.New()
			deps := &Deps{RelayStations: storage.NewRelayStations(db)}
			registerChannels(router.Group(""), deps)
			registerPublicRelayStations(router.Group("/public"), deps)
			path := "/channels/9/pelican/results/12/thumbnail"
			if stage == "lookup" {
				path = "/public/relay-stations/7/groups/30/pelican/results/12/thumbnail"
			}
			req := httptest.NewRequest(method, path, bytes.NewReader(encoded.Bytes()))
			req.Header.Set("Content-Type", "image/jpeg")
			req.Header.Set("X-Pelican-Source-SHA256", fmt.Sprintf("%x", sha256.Sum256([]byte("<svg/>"))))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != http.StatusInternalServerError {
				t.Fatalf("storage %s failure=%d body=%s", stage, response.Code, response.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPelicanThumbnailAuthenticatedPersistenceAcrossReloadAndScopes(t *testing.T) {
	file := filepath.Join(t.TempDir(), "authenticated-thumbnails.db")
	open := func() *gorm.DB {
		db, err := gorm.Open(sqlite.Open(file), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			t.Fatal(err)
		}
		return db
	}
	db := open()
	if err := db.AutoMigrate(&storage.RelayStation{}, &storage.RelayGroup{}, &storage.PelicanResult{}, &storage.GroupPelicanResult{}); err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{
		&storage.RelayStation{ID: 7, Name: "test"},
		&storage.RelayGroup{RelayStationID: 7, ExternalID: 30, MonitorEnabled: true},
		&storage.PelicanResult{ID: 12, RelayStationID: 7, AccountExternalID: 20, HTML: "<svg/>", Status: "success"},
		&storage.GroupPelicanResult{ID: 13, RelayStationID: 7, GroupExternalID: 30, HTML: "<svg/>", Status: "success"},
	} {
		if err := db.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec("UPDATE relay_pelican_results SET channel_id = 9 WHERE id = 12").Error; err != nil {
		t.Fatal(err)
	}
	service, err := auth.New("test", "test-password", "thumbnail-test-secret", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := service.Login("test", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	makeRouter := func(db *gorm.DB) *gin.Engine {
		router := gin.New()
		deps := &Deps{RelayStations: storage.NewRelayStations(db)}
		registerPublicRelayStations(router.Group("/api/public"), deps)
		private := router.Group("/api", service.Middleware())
		registerChannels(private, deps)
		registerRelayStations(private, deps)
		return router
	}
	router := makeRouter(db)
	paths := []string{
		"/api/channels/9/pelican/results/12/thumbnail",
		"/api/relay-stations/7/accounts/20/pelican/results/12/thumbnail",
		"/api/relay-stations/7/groups/30/pelican/results/13/thumbnail",
	}
	var picture bytes.Buffer
	if err := jpeg.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 448, 336)), nil); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(picture.Bytes()))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "image/jpeg")
		req.Header.Set("X-Pelican-Source-SHA256", fmt.Sprintf("%x", sha256.Sum256([]byte("<svg/>"))))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != 204 {
			t.Fatalf("upload %s: %d %s", path, response.Code, response.Body.String())
		}
	}
	sqlDB, _ := db.DB()
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	db = open()
	sqlDB, _ = db.DB()
	defer sqlDB.Close()
	router = makeRouter(db)
	for _, path := range append(paths, "/api/public/relay-stations/7/groups/30/pelican/results/13/thumbnail") {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if !strings.Contains(path, "/public/") {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != 200 {
			t.Fatalf("read after reopen %s: %d %s", path, response.Code, response.Body.String())
		}
		if _, err := jpeg.Decode(bytes.NewReader(response.Body.Bytes())); err != nil {
			t.Fatal(err)
		}
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, paths[0], nil))
	if response.Code != 401 || !strings.Contains(response.Body.String(), "missing token") {
		t.Fatalf("missing auth token=%d %s", response.Code, response.Body.String())
	}
}
