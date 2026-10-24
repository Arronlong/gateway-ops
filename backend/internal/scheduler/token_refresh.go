package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
	"github.com/guliansheng/gateway-ops/internal/tasklog"
)

func (s *Scheduler) configureTokenRefreshJob(settings storage.SyncSettings) error {
	if s.tokenRefreshID != 0 {
		s.cron.Remove(s.tokenRefreshID)
		s.tokenRefreshID = 0
	}
	if !settings.ChannelTokenRefreshEnabled {
		return nil
	}
	interval := settings.ChannelTokenRefreshIntervalMinutes
	if interval < 1 {
		interval = 30
	}
	id, err := s.cron.AddFunc(fmt.Sprintf("@every %dm", interval), s.runChannelTokenRefresh)
	if err == nil {
		s.tokenRefreshID = id
	}
	return err
}

func (s *Scheduler) runChannelTokenRefresh() {
	if !s.tokenRefreshRunning.CompareAndSwap(false, true) {
		return
	}
	defer s.tokenRefreshRunning.Store(false)
	enabled, _, err := s.settings.TokenRefreshSettings()
	if err != nil {
		s.log.Error("read channel token refresh settings failed", "err", err)
		return
	}
	if !enabled {
		return
	}
	root := s.pelicanCtx
	if root == nil {
		root = context.Background()
	}
	s.track(root, "channel_token_refresh", 20*time.Minute, func(ctx context.Context) error {
		result := s.monitor.RefreshChannelTokens(ctx)
		tasklog.Add(ctx, "INFO", "Token \u5237\u65b0\uff1a\u603b\u6570 %d\uff0c\u6210\u529f %d\uff0c\u5931\u8d25 %d", result.Total, result.Succeeded, result.Failed)
		if result.Failed > 0 {
			return fmt.Errorf("%d channel token refreshes failed", result.Failed)
		}
		return ctx.Err()
	})
}
