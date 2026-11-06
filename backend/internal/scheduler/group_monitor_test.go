package scheduler

import (
	"github.com/guliansheng/gateway-ops/internal/relay"
	"github.com/robfig/cron/v3"
	"testing"
	"time"
)

func TestGroupMonitorScheduleCanChangeWithoutDuplicateJobs(t *testing.T) {
	s := &Scheduler{cron: cron.New(cron.WithSeconds()), relay: &relay.Service{}}
	now := time.Now().Truncate(time.Second)
	for _, interval := range []int{30, 120, 10, 3600} {
		if err := s.configureGroupMonitorSchedule(interval); err != nil {
			t.Fatal(err)
		}
		entries := s.cron.Entries()
		if len(entries) != 1 || entries[0].Schedule.Next(now).Sub(now) != time.Duration(interval)*time.Second {
			t.Fatalf("wrong schedule for %d: %+v", interval, entries)
		}
	}
	for _, invalid := range []int{0, 9, 3601} {
		if err := s.configureGroupMonitorSchedule(invalid); err == nil {
			t.Fatalf("accepted invalid interval %d", invalid)
		}
	}
	if len(s.cron.Entries()) != 1 {
		t.Fatal("invalid settings deleted existing schedule")
	}
}
