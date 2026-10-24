package storage

import (
	"errors"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"os"
	"testing"
	"time"
)

func TestTaskRunsPostgresLifecycle(t *testing.T) {
	dsn := os.Getenv("GATEWAYOPS_TEST_DSN")
	if dsn == "" {
		t.Skip("GATEWAYOPS_TEST_DSN is not configured")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	rollback := errors.New("rollback isolated test schema")
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("CREATE SCHEMA task_runs_integration_test").Error; err != nil {
			return err
		}
		if err := tx.Exec("SET LOCAL search_path TO task_runs_integration_test").Error; err != nil {
			return err
		}
		if err := tx.AutoMigrate(&TaskRun{}); err != nil {
			return err
		}
		repo := NewTaskRuns(tx)
		success, err := repo.Start("channel_sync")
		if err != nil {
			return err
		}
		if err := repo.Finish(success, "success", "completed", "scoped-log"); err != nil {
			return err
		}
		if err := repo.Finish(success, "failed", "must not overwrite terminal state", "bad-log"); err != nil {
			return err
		}
		failed, err := repo.Start("channel_sync")
		if err != nil {
			return err
		}
		if err := repo.Finish(failed, "failed", "upstream failed", "error-log"); err != nil {
			return err
		}
		if _, err := repo.Start("channel_sync"); err != nil {
			return err
		}
		if _, err := repo.Start("relay_rate_sync"); err != nil {
			return err
		}
		page, err := repo.List("channel_sync", 2, 2)
		if err != nil {
			return err
		}
		if page.Total != 3 || page.Succeeded != 1 || page.Failed != 1 || page.Running != 1 || len(page.Runs) != 1 || page.Page != 2 || page.PageSize != 2 || page.Runs[0].Logs != "" {
			t.Errorf("invalid scoped page: %#v", page)
		}
		detail, err := repo.Find("channel_sync", success.ID)
		if err != nil {
			return err
		}
		if detail.Logs != "scoped-log" || detail.Status != "success" || detail.FinishedAt == nil || detail.DurationMS < 0 {
			t.Errorf("invalid detail: %#v", detail)
		}
		if _, err := repo.Find("relay_rate_sync", success.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Errorf("cross-task detail lookup: %v", err)
		}
		if err := repo.InterruptRunning(time.Now().UTC().Add(time.Second)); err != nil {
			return err
		}
		page, err = repo.List("channel_sync", 1, 20)
		if err != nil {
			return err
		}
		if page.Running != 0 || page.Failed != 2 {
			t.Errorf("recovery: %#v", page)
		}
		empty, err := repo.List("retention", 1, 20)
		if err != nil {
			return err
		}
		if empty.Runs == nil || empty.Total != 0 || empty.Page != 1 || empty.PageSize != 20 {
			t.Errorf("invalid empty page: %#v", empty)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
}
