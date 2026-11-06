package scheduler

import (
	"context"
	"fmt"
	"time"
)

func (s *Scheduler) configureGroupMonitorJob() error {
	if s.relay == nil {
		return nil
	}
	// 与服务启动同时预热，停止时由现有生命周期等待并取消。
	s.retryTaskRunsWG.Add(1)
	go func() { defer s.retryTaskRunsWG.Done(); s.runGroupMonitorSnapshots() }()
	return nil
}

func (s *Scheduler) runGroupMonitorSnapshots() {
	if !s.groupMonitorRunning.CompareAndSwap(false, true) {
		return
	}
	defer s.groupMonitorRunning.Store(false)
	base := s.pelicanCtx
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithTimeout(base, 15*time.Minute)
	defer cancel()
	if err := s.relay.RefreshPublicGroupMonitorSnapshots(ctx); err != nil && ctx.Err() == nil && s.log != nil {
		s.log.Warn("group monitor background sync failed", "err", err)
	}
}

func (s *Scheduler) configureGroupMonitorSchedule(interval int) error {
	if interval < 10 || interval > 3600 {
		return fmt.Errorf("group monitor interval must be between 10 and 3600 seconds")
	}
	if s.relay == nil {
		return nil
	}
	if s.groupMonitorID != 0 {
		s.cron.Remove(s.groupMonitorID)
		s.groupMonitorID = 0
	}
	id, err := s.cron.AddFunc(fmt.Sprintf("@every %ds", interval), s.runGroupMonitorSnapshots)
	if err == nil {
		s.groupMonitorID = id
	}
	return err
}
