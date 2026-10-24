package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAlertLocksIsolateCallbackQueriesAndReleaseSession(t *testing.T) {
	for _, evaluation := range []bool{false, true} {
		for _, failQuery := range []bool{false, true} {
			name := "mutation"
			if evaluation {
				name = "evaluation"
			}
			if failQuery {
				name += "/query_error"
			} else {
				name += "/success"
			}
			t.Run(name, func(t *testing.T) {
				sqlDB, mock, err := sqlmock.New()
				if err != nil {
					t.Fatal(err)
				}
				defer sqlDB.Close()
				sqlDB.SetMaxOpenConns(1)
				db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
				if err != nil {
					t.Fatal(err)
				}
				const lockID int64 = 719420261015
				if evaluation {
					mock.ExpectQuery(`SELECT pg_try_advisory_lock\(\$1\)`).WithArgs(lockID).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
				} else {
					mock.ExpectExec(`SELECT pg_advisory_lock\(\$1\)`).WithArgs(lockID).WillReturnResult(sqlmock.NewResult(0, 1))
				}
				queryErr := errors.New("callback query failed")
				query := mock.ExpectQuery(`SELECT \* FROM "alert_rule_notification_channels" WHERE rule_id = \$1`).WithArgs(uint(7))
				if failQuery {
					query.WillReturnError(queryErr)
				} else {
					query.WillReturnRows(sqlmock.NewRows([]string{"rule_id", "notification_channel_id"}).AddRow(7, 11))
					mock.ExpectQuery(`SELECT \* FROM "alert_rule_notification_channels" WHERE rule_id = \$1`).WithArgs(uint(8)).WillReturnRows(sqlmock.NewRows([]string{"rule_id", "notification_channel_id"}).AddRow(8, 12))
				}
				mock.ExpectExec(`SELECT pg_advisory_unlock\(\$1\)`).WithArgs(lockID).WillReturnResult(sqlmock.NewResult(0, 1))
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				callback := func(repo *Alerts) error {
					ids, err := repo.ListRuleChannelIDs(7)
					if err != nil {
						cancel()
						return err
					}
					if len(ids) != 1 || ids[0] != 11 {
						t.Errorf("first channels = %v", ids)
					}
					ids, err = repo.ListRuleChannelIDs(8)
					if err != nil {
						return err
					}
					if len(ids) != 1 || ids[0] != 12 {
						t.Errorf("second channels = %v", ids)
					}
					return nil
				}
				repo := NewAlerts(db)
				if evaluation {
					err = repo.WithEvaluationLock(ctx, callback)
				} else {
					err = repo.WithMutationLock(ctx, callback)
				}
				if failQuery {
					if !errors.Is(err, queryErr) {
						t.Errorf("callback error = %v, want %v", err, queryErr)
					}
				} else if err != nil {
					t.Errorf("callback failed: %v", err)
				}
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
