package storage

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPelicanThumbnailsPersistWithResultAndRespectOwnershipAndSource(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "thumbnail.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := db.AutoMigrate(&PelicanResult{}, &GroupPelicanResult{}); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"channel", "account", "group"} {
		t.Run(scope, func(t *testing.T) {
			ref := PelicanArtifactRef{ID: 1, StationID: 7, AccountID: 20}
			if scope == "group" {
				ref.GroupID = 30
				ref.AccountID = 0
				if err := db.Create(&GroupPelicanResult{ID: 1, RelayStationID: 7, GroupExternalID: 30, HTML: "<svg/>", Status: "success"}).Error; err != nil {
					t.Fatal(err)
				}
			} else {
				if scope == "channel" {
					ref.ChannelID = 9
				}
				if err := db.Exec("INSERT INTO relay_pelican_results (id,channel_id,relay_station_id,account_external_id,html,status,account_name,model,prompt,reasoning_effort,trigger,error,duration_ms,started_at,finished_at,upstream_group_name) VALUES (1,9,7,20,'<svg/>','success','','','','','','',0,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,'')").Error; err != nil {
					t.Fatal(err)
				}
			}
			repo := NewRelayStations(db)
			if _, err := repo.PelicanThumbnail(ref); !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Fatalf("missing thumbnail=%v", err)
			}
			if err := repo.SavePelicanThumbnail(ref, "different source", []byte("jpeg")); !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Fatalf("stale thumbnail write=%v", err)
			}
			if err := repo.SavePelicanThumbnail(ref, "<svg/>", []byte("jpeg")); err != nil {
				t.Fatal(err)
			}
			// 新的仓储实例仍能读取已保存的缩略图。
			stored, err := NewRelayStations(db).PelicanThumbnail(ref)
			if err != nil || !bytes.Equal(stored, []byte("jpeg")) {
				t.Fatalf("persisted thumbnail=%q error=%v", stored, err)
			}
			wrong := ref
			if wrong.ChannelID > 0 {
				wrong.ChannelID++
			} else if wrong.GroupID > 0 {
				wrong.GroupID++
			} else {
				wrong.AccountID++
			}
			if _, err := repo.PelicanThumbnail(wrong); !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Fatal("thumbnail escaped result ownership")
			}
			if ref.GroupID != 0 {
				db.Delete(&GroupPelicanResult{}, 1)
			} else {
				db.Delete(&PelicanResult{}, 1)
			}
			if _, err := repo.PelicanThumbnail(ref); !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Fatal("thumbnail survived result deletion")
			}
		})
	}
}
