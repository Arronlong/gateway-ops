package notify

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/guliansheng/gateway-ops/internal/crypto"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAlertDispatchSelectionCooldownRecoveryAndRetry(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "notifications.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	t.Cleanup(func() { _ = sql.Close() })
	if err := db.AutoMigrate(&storage.NotificationChannel{}, &storage.NotificationLog{}); err != nil {
		t.Fatal(err)
	}
	cipher, err := crypto.NewCipher("alert-test-secret")
	if err != nil {
		t.Fatal(err)
	}
	var goodCalls, retryCalls, unselectedCalls atomic.Int32
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { goodCalls.Add(1); w.WriteHeader(200) }))
	defer good.Close()
	retry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if retryCalls.Add(1) == 1 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
	}))
	defer retry.Close()
	unselected := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { unselectedCalls.Add(1); w.WriteHeader(200) }))
	defer unselected.Close()
	repo := storage.NewNotifications(db)
	channel := func(name, url string) storage.NotificationChannel {
		raw, _ := json.Marshal(map[string]string{"url": url})
		secret, err := cipher.Encrypt(string(raw))
		if err != nil {
			t.Fatal(err)
		}
		ch := storage.NotificationChannel{Name: name, Type: storage.NotifyWebhook, ConfigCipher: secret, Enabled: true}
		if err := repo.CreateChannel(&ch); err != nil {
			t.Fatal(err)
		}
		return ch
	}
	a, b, c := channel("正常渠道", good.URL), channel("暂时失败渠道", retry.URL), channel("未订阅渠道", unselected.URL)
	_ = c
	dispatch := NewDispatcher(repo, cipher, slog.New(slog.NewTextHandler(io.Discard, nil)), Policy{SendMaxAttempts: 1})
	msg := Message{Event: storage.EventAlertFiring, Subject: "消耗预警", Body: "窗口累计消耗达到阈值", AlertEventID: 42, AlertRuleID: 7, AlertCooldown: 30 * time.Minute}
	if err := dispatch.DispatchToChannels(context.Background(), []uint{a.ID, a.ID, b.ID}, msg); err == nil {
		t.Fatal("expected failed channel error")
	}
	if err := dispatch.DispatchToChannels(context.Background(), []uint{a.ID, b.ID}, msg); err != nil {
		t.Fatal(err)
	}
	if goodCalls.Load() != 1 || retryCalls.Load() != 2 || unselectedCalls.Load() != 0 {
		t.Fatalf("firing calls = %d/%d/%d", goodCalls.Load(), retryCalls.Load(), unselectedCalls.Load())
	}
	msg.Event = storage.EventAlertRecovered
	if err := dispatch.DispatchToChannels(context.Background(), []uint{a.ID, b.ID}, msg); err != nil {
		t.Fatal(err)
	}
	if err := dispatch.DispatchToChannels(context.Background(), []uint{a.ID, b.ID}, msg); err != nil {
		t.Fatal(err)
	}
	if goodCalls.Load() != 2 || retryCalls.Load() != 3 {
		t.Fatal("recovery must deliver once per channel, independently of firing cooldown")
	}
	var logs []storage.NotificationLog
	if err := db.Order("id").Find(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if len(logs) != 5 {
		t.Fatalf("delivery logs = %d, want 5", len(logs))
	}
	for _, entry := range logs {
		if entry.AlertEventID != 42 || entry.AlertRuleID != 7 {
			t.Fatalf("missing alert linkage: %+v", entry)
		}
	}
}
