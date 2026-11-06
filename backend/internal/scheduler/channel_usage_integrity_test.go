package scheduler

import (
	"testing"
	"time"

	"github.com/guliansheng/gateway-ops/internal/monitor"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"github.com/robfig/cron/v3"
)

func TestChannelUsageIntegrityScheduleUsesBeijingTimeAndCanDisable(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	s := &Scheduler{usageIntegrityCron: cron.New(cron.WithSeconds(), cron.WithLocation(location))}
	settings := storage.SyncSettings{ChannelUsageIntegrityEnabled: true, ChannelUsageIntegrityRange: "24h", ChannelUsageIntegrityTime: "07:00"}
	if err := s.configureUsageIntegrityJobWithSettings(settings); err != nil {
		t.Fatal(err)
	}
	entries := s.usageIntegrityCron.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries=%d, want 1", len(entries))
	}
	now := time.Date(2026, 11, 6, 6, 59, 59, 0, location)
	want := time.Date(2026, 11, 6, 7, 0, 0, 0, location)
	if got := entries[0].Schedule.Next(now); !got.Equal(want) {
		t.Fatalf("next=%s, want %s", got, want)
	}
	settings.ChannelUsageIntegrityTime = "08:15"
	if err := s.configureUsageIntegrityJobWithSettings(settings); err != nil {
		t.Fatal(err)
	}
	if len(s.usageIntegrityCron.Entries()) != 1 {
		t.Fatalf("entries after update=%d", len(s.usageIntegrityCron.Entries()))
	}
	want = time.Date(2026, 11, 6, 8, 15, 0, 0, location)
	if got := s.usageIntegrityCron.Entries()[0].Schedule.Next(now); !got.Equal(want) {
		t.Fatalf("updated next=%s, want %s", got, want)
	}
	settings.ChannelUsageIntegrityEnabled = false
	if err := s.configureUsageIntegrityJobWithSettings(settings); err != nil {
		t.Fatal(err)
	}
	if len(s.usageIntegrityCron.Entries()) != 0 {
		t.Fatal("disabled schedule remains registered")
	}
}

func TestChannelUsageIntegrityRunRejectsOverlapAndShutdown(t *testing.T) {
	s := &Scheduler{monitor: &monitor.Service{}, settings: storage.NewSettings(nil)}
	s.usageIntegrityRunning.Store(true)
	if s.RunChannelUsageIntegrity() {
		t.Fatal("overlapping manual run was accepted")
	}
	s.usageIntegrityRunning.Store(false)
	s.usageIntegrityStopping.Store(true)
	if s.RunChannelUsageIntegrity() {
		t.Fatal("manual run was accepted during shutdown")
	}
}
