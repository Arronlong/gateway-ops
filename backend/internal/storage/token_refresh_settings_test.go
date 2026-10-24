package storage

import (
	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"
	"testing"
)

func TestTokenRefreshDefaultsAndSavedFrequency(t *testing.T) {
	for _, tc := range []struct {
		enabled, interval string
		wantEnabled       bool
		wantInterval      int
	}{{"", "", false, 30}, {"true", "45", true, 45}, {"false", "10", false, 10}} {
		t.Run(tc.enabled+tc.interval, func(t *testing.T) {
			repo, mock := pelicanDB(t)
			for _, item := range [][2]string{{"sync.channel.token_refresh.enabled", tc.enabled}, {"sync.channel.token_refresh.interval_minutes", tc.interval}} {
				query := mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(item[0], 1)
				if item[1] == "" {
					query.WillReturnError(gorm.ErrRecordNotFound)
				} else {
					query.WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow(item[0], item[1]))
				}
			}
			enabled, interval, err := NewSettings(repo.db).TokenRefreshSettings()
			if err != nil || enabled != tc.wantEnabled || interval != tc.wantInterval {
				t.Fatalf("enabled=%v interval=%d err=%v", enabled, interval, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
