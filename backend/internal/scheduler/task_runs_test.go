package scheduler

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"github.com/guliansheng/gateway-ops/internal/tasklog"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestTrackedTaskPersistsActualOutcome(t *testing.T) {
	tests := []struct {
		name, status string
		fn           func(context.Context) error
	}{
		{"success", "success", func(ctx context.Context) error { tasklog.Add(ctx, "INFO", "work completed"); return nil }},
		{"error", "failed", func(context.Context) error { return errors.New("upstream failure") }},
		{"partial", "failed", func(ctx context.Context) error { tasklog.Add(ctx, "ERROR", "one item failed"); return nil }},
		{"panic", "failed", func(context.Context) error { panic("unexpected") }},
		{"timeout", "failed", func(ctx context.Context) error { <-ctx.Done(); return nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer sqlDB.Close()
			db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{SkipDefaultTransaction: true})
			if err != nil {
				t.Fatal(err)
			}
			mock.ExpectQuery(`INSERT INTO "task_runs"`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
			mock.ExpectExec(`UPDATE "task_runs" SET .* WHERE id = .*AND status =`).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), tt.status, sqlmock.AnyArg(), uint(7), "running").WillReturnResult(sqlmock.NewResult(0, 1))
			scheduler := &Scheduler{settings: storage.NewSettings(db), log: slog.New(slog.NewTextHandler(io.Discard, nil))}
			scheduler.track(context.Background(), "channel_sync", 10*time.Millisecond, tt.fn)
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTaskRunFinishRetriesAndCompensates(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`INSERT INTO "task_runs"`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(8))
	for i := 0; i < 3; i++ {
		mock.ExpectExec(`UPDATE "task_runs"`).WillReturnError(errors.New("temporary database outage"))
	}
	scheduler := &Scheduler{settings: storage.NewSettings(db), log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	scheduler.track(context.Background(), "channel_sync", time.Second, func(context.Context) error { return nil })
	if len(scheduler.pendingTaskRuns) != 1 {
		t.Fatalf("pending=%d", len(scheduler.pendingTaskRuns))
	}
	mock.ExpectExec(`UPDATE "task_runs"`).WillReturnResult(sqlmock.NewResult(0, 1))
	scheduler.Stop()
	if len(scheduler.pendingTaskRuns) != 0 {
		t.Fatal("completed record was not recovered")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
