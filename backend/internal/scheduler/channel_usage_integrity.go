package scheduler

import (
	"context"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/guliansheng/gateway-ops/internal/monitor"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"github.com/guliansheng/gateway-ops/internal/tasklog"
	"github.com/robfig/cron/v3"
)

func mustLoadUsageIntegrityLocation() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		panic(err)
	}
	return location
}

func (s *Scheduler) configureUsageIntegrityJob() error {
	settings, err := s.settings.SyncSettings()
	if err != nil {
		return err
	}
	return s.configureUsageIntegrityJobWithSettings(settings)
}

func (s *Scheduler) configureUsageIntegrityJobWithSettings(settings storage.SyncSettings) error {
	s.usageIntegrityScheduleMu.Lock()
	defer s.usageIntegrityScheduleMu.Unlock()
	if s.usageIntegrityCron == nil {
		s.usageIntegrityCron = cron.New(cron.WithSeconds(), cron.WithLocation(mustLoadUsageIntegrityLocation()))
	}
	if s.usageIntegrityID != 0 {
		s.usageIntegrityCron.Remove(s.usageIntegrityID)
		s.usageIntegrityID = 0
	}
	if !settings.ChannelUsageIntegrityEnabled {
		return nil
	}
	parts := strings.Split(settings.ChannelUsageIntegrityTime, ":")
	if len(parts) != 2 {
		return fmt.Errorf("invalid channel usage integrity time %q", settings.ChannelUsageIntegrityTime)
	}
	spec := fmt.Sprintf("0 %s %s * * *", parts[1], parts[0])
	id, err := s.usageIntegrityCron.AddFunc(spec, s.runChannelUsageIntegrity)
	if err == nil {
		s.usageIntegrityID = id
	}
	return err
}

func (s *Scheduler) runChannelUsageIntegrity() {
	s.startChannelUsageIntegrity()
}

func (s *Scheduler) RunChannelUsageIntegrity() bool {
	return s.startChannelUsageIntegrity()
}

func (s *Scheduler) startChannelUsageIntegrity() bool {
	if s.monitor == nil || s.settings == nil {
		return false
	}
	s.usageIntegrityMu.Lock()
	if s.usageIntegrityStopping.Load() || !s.usageIntegrityRunning.CompareAndSwap(false, true) {
		s.usageIntegrityMu.Unlock()
		return false
	}
	s.usageIntegrityWG.Add(1)
	s.usageIntegrityMu.Unlock()
	go func() {
		defer s.usageIntegrityWG.Done()
		defer s.usageIntegrityRunning.Store(false)
		root := s.pelicanCtx
		if root == nil {
			root = context.Background()
		}
		s.track(root, "channel_usage_integrity", 24*time.Hour, func(ctx context.Context) error {
			settings, err := s.settings.SyncSettings()
			if err != nil {
				return fmt.Errorf("read channel usage integrity settings: %w", err)
			}
			now := time.Now().UTC()
			location := mustLoadUsageIntegrityLocation()
			since, until, err := monitor.UsageHistoryBounds(settings.ChannelUsageIntegrityRange, now, location)
			if err != nil {
				return err
			}
			sinceLabel := "\u5168\u90e8\u5386\u53f2"
			if !since.IsZero() {
				sinceLabel = since.In(location).Format("2006-01-02 15:04:05")
			}
			tasklog.Add(ctx, "INFO", "\u6838\u5bf9\u8303\u56f4 %s\uff1a%s \u81f3 %s\uff08\u5317\u4eac\u65f6\u95f4\uff09", settings.ChannelUsageIntegrityRange, sinceLabel, until.In(location).Format("2006-01-02 15:04:05"))
			result, err := s.monitor.RefetchAllUsageHistory(ctx, since, until)
			tasklog.Add(ctx, "INFO", "全渠道用量核对：总数 %d，成功 %d，失败 %d，跳过 %d", result.Total, result.Succeeded, result.Failed, result.Skipped)
			for _, item := range result.SkippedChannels {
				tasklog.Add(ctx, "INFO", "跳过手动余额渠道 #%d %s：没有上游用量历史接口", item.ChannelID, item.ChannelName)
			}
			for _, item := range result.Errors {
				if item.ChannelID > 0 {
					tasklog.Add(ctx, "ERROR", "渠道 #%d %s 核对失败：%v", item.ChannelID, item.ChannelName, item.Err)
				}
			}
			if err != nil {
				return err
			}
			if result.Failed > 0 {
				return fmt.Errorf("%d channel usage integrity checks failed", result.Failed)
			}
			return ctx.Err()
		})
	}()
	return true
}
