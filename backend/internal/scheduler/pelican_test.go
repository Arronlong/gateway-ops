package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

func TestStopCancelsPelicanBeforeWaitingForCron(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	scheduler := &Scheduler{cron: cron.New(), pelicanCtx: ctx, pelicanCancel: cancel}
	started := make(chan struct{})
	_, err := scheduler.cron.AddFunc("@every 1s", func() { close(started); <-ctx.Done() })
	if err != nil {
		t.Fatal(err)
	}
	scheduler.cron.Start()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("job did not start")
	}
	stopped := make(chan struct{})
	go func() { scheduler.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("stop did not cancel active job")
	}
}
