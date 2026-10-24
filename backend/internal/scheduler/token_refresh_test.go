package scheduler

import (
	"github.com/guliansheng/gateway-ops/internal/storage"
	"github.com/robfig/cron/v3"
	"testing"
	"time"
)

func TestTokenRefreshScheduleCanEnableChangeAndDisable(t *testing.T) {
	s := &Scheduler{cron: cron.New(cron.WithSeconds())}
	settings := storage.SyncSettings{ChannelTokenRefreshEnabled: true, ChannelTokenRefreshIntervalMinutes: 30}
	if err := s.configureTokenRefreshJob(settings); err != nil {
		t.Fatal(err)
	}
	if len(s.cron.Entries()) != 1 {
		t.Fatal("refresh job missing")
	}
	now := time.Now().Truncate(time.Second)
	if got := s.cron.Entries()[0].Schedule.Next(now).Sub(now); got != 30*time.Minute {
		t.Fatalf("interval=%v", got)
	}
	settings.ChannelTokenRefreshIntervalMinutes = 45
	if err := s.configureTokenRefreshJob(settings); err != nil {
		t.Fatal(err)
	}
	if len(s.cron.Entries()) != 1 || s.cron.Entries()[0].Schedule.Next(now).Sub(now) != 45*time.Minute {
		t.Fatal("schedule not replaced")
	}
	settings.ChannelTokenRefreshEnabled = false
	if err := s.configureTokenRefreshJob(settings); err != nil {
		t.Fatal(err)
	}
	if len(s.cron.Entries()) != 0 {
		t.Fatal("disabled job still registered")
	}
	if TaskNames["channel_token_refresh"] == "" {
		t.Fatal("task history name missing")
	}
}
