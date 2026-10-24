package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
	"github.com/guliansheng/gateway-ops/internal/tasklog"
)

var TaskNames = map[string]string{
	"channel_token_refresh": "\u6e20\u9053 Token \u5237\u65b0",
	"channel_sync":          "\u6e20\u9053\u4f59\u989d\u4e0e\u500d\u7387\u540c\u6b65",
	"relay_rate_sync":       "API Key \u500d\u7387\u63a2\u6d4b",
	"relay_snapshot_sync":   "\u4e2d\u8f6c\u7ad9\u5feb\u7167\u540c\u6b65",
	"daily_balances":        "\u6bcf\u65e5\u4f59\u989d\u5feb\u7167",
	"retention":             "\u5386\u53f2\u6570\u636e\u6e05\u7406",
	"pelican":               "\u9e48\u9e55\u5b9a\u65f6\u8c03\u5ea6\u68c0\u67e5",
	"legacy_balance":        "\u6e20\u9053\u4f59\u989d\u626b\u63cf\uff08\u517c\u5bb9\uff09",
	"legacy_rates":          "\u6e20\u9053\u500d\u7387\u626b\u63cf\uff08\u517c\u5bb9\uff09",
}

func (s *Scheduler) track(root context.Context, task string, timeout time.Duration, fn func(context.Context) error) {
	ctx, cancel := context.WithTimeout(root, timeout)
	defer cancel()
	ctx, recorder := tasklog.WithRecorder(ctx)
	var persist func(string, string, string)
	if s.settings != nil {
		repo := s.settings.TaskRuns()
		run, err := repo.Start(task)
		if err != nil {
			s.log.Error("record task start failed", "task", task, "err", err)
		} else {
			persist = func(status, summary, logs string) {
				pending := pendingTaskRun{run: run, status: status, summary: summary, logs: logs}
				for attempt := 0; attempt < 3; attempt++ {
					if err := pending.save(repo); err == nil {
						return
					}
					if attempt < 2 {
						time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
					}
				}
				s.pendingTaskRunsMu.Lock()
				if s.pendingTaskRuns == nil {
					s.pendingTaskRuns = make(map[uint]pendingTaskRun)
				}
				s.pendingTaskRuns[run.ID] = pending
				s.pendingTaskRunsMu.Unlock()
				s.log.Error("task finish queued for retry", "task", task, "run", run.ID)
			}
		}
	}
	tasklog.Add(ctx, "INFO", "\u5f00\u59cb\u6267\u884c %s", TaskNames[task])
	var runErr error
	defer func() {
		if recovered := recover(); recovered != nil {
			runErr = fmt.Errorf("task panic: %v", recovered)
		}
		if runErr == nil {
			runErr = ctx.Err()
		}
		status, summary := "success", "\u6267\u884c\u5b8c\u6210"
		if runErr != nil {
			tasklog.Add(ctx, "ERROR", "%v", runErr)
		}
		_, failed := recorder.Snapshot()
		if failed {
			status, summary = "failed", "\u6267\u884c\u5931\u8d25\uff0c\u8bf7\u67e5\u770b\u65e5\u5fd7"
		}
		if runErr != nil {
			summary = tasklog.Redact(runErr.Error())
		}
		tasklog.Add(ctx, "INFO", "%s", summary)
		logs, _ := recorder.Snapshot()
		if persist != nil {
			persist(status, summary, logs)
		}
	}()
	runErr = fn(ctx)
}

type pendingTaskRun struct {
	run                   *storage.TaskRun
	status, summary, logs string
}

func (p pendingTaskRun) save(repo *storage.TaskRuns) error {
	return repo.Finish(p.run, p.status, p.summary, p.logs)
}

func (s *Scheduler) flushTaskRunsContext(ctx context.Context) {
	s.pendingTaskRunsMu.Lock()
	pending := make(map[uint]pendingTaskRun, len(s.pendingTaskRuns))
	for id, run := range s.pendingTaskRuns {
		pending[id] = run
	}
	s.pendingTaskRunsMu.Unlock()
	for id, run := range pending {
		if ctx.Err() != nil {
			return
		}
		if err := s.settings.TaskRuns().FinishContext(ctx, run.run, run.status, run.summary, run.logs); err != nil {
			s.log.Warn("retry task finish failed", "run", id, "err", err)
			continue
		}
		s.pendingTaskRunsMu.Lock()
		delete(s.pendingTaskRuns, id)
		s.pendingTaskRunsMu.Unlock()
	}
}

func (s *Scheduler) retryTaskRuns() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	ctx := s.pelicanCtx
	if ctx == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.flushTaskRunsContext(ctx)
		}
	}
}
