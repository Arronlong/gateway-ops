// Package scheduler 用 robfig/cron 触发周期性扫描。
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/guliansheng/gateway-ops/internal/alert"
	"github.com/guliansheng/gateway-ops/internal/config"
	"github.com/guliansheng/gateway-ops/internal/monitor"
	"github.com/guliansheng/gateway-ops/internal/relay"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"github.com/guliansheng/gateway-ops/internal/tasklog"
	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	tokenRefreshID       cron.EntryID
	tokenRefreshRunning  atomic.Bool
	cfg                  config.SchedulerConfig
	log                  *slog.Logger
	cron                 *cron.Cron
	monitor              *monitor.Service
	monLogs              *storage.MonitorLogs
	rates                *storage.Rates
	notifies             *storage.Notifications
	settings             *storage.Settings
	relay                *relay.Service
	pelicanCtx           context.Context
	pelicanCancel        context.CancelFunc
	alerts               *alert.Service
	legacyBalanceID      cron.EntryID
	legacyRateID         cron.EntryID
	channelSyncID        cron.EntryID
	relayRateSyncID      cron.EntryID
	relaySnapshotSyncID  cron.EntryID
	alertEvaluationID    cron.EntryID
	alertRateSyncID      cron.EntryID
	alertUsageSyncID     cron.EntryID
	alertBalanceSyncID   cron.EntryID
	dailyBalanceID       cron.EntryID
	retryTaskRunsWG      sync.WaitGroup
	pendingTaskRunsMu    sync.Mutex
	pendingTaskRuns      map[uint]pendingTaskRun
	jobsMu               sync.Mutex
	channelSyncMu        sync.Mutex
	relaySyncMu          sync.Mutex
	relayRateRunning     atomic.Bool
	relaySnapshotRunning atomic.Bool
	alertRunning         atomic.Bool
}

func New(
	cfg config.SchedulerConfig,
	m *monitor.Service,
	monLogs *storage.MonitorLogs,
	rates *storage.Rates,
	notifies *storage.Notifications,
	settings *storage.Settings,
	relaySvc *relay.Service,
	alertSvc *alert.Service,
	log *slog.Logger,
) *Scheduler {
	pelicanCtx, pelicanCancel := context.WithCancel(context.Background())
	return &Scheduler{
		cfg:             cfg,
		log:             log,
		cron:            cron.New(cron.WithSeconds()),
		monitor:         m,
		monLogs:         monLogs,
		rates:           rates,
		notifies:        notifies,
		settings:        settings,
		relay:           relaySvc,
		pelicanCtx:      pelicanCtx,
		pelicanCancel:   pelicanCancel,
		alerts:          alertSvc,
		pendingTaskRuns: make(map[uint]pendingTaskRun),
	}
}

func (s *Scheduler) Start() error {
	if err := s.settings.TaskRuns().InterruptRunning(time.Now().UTC()); err != nil {
		return fmt.Errorf("recover task runs: %w", err)
	}
	s.runDailyBalances()
	entryID, err := s.cron.AddFunc("0 0 0 * * *", s.runDailyBalances)
	if err != nil {
		return err
	}
	s.dailyBalanceID = entryID
	if s.cfg.BalanceCron != "" {
		entryID, err := s.cron.AddFunc(s.cfg.BalanceCron, s.runBalance)
		if err != nil {
			return err
		}
		s.legacyBalanceID = entryID
	}
	if s.cfg.RateCron != "" {
		entryID, err := s.cron.AddFunc(s.cfg.RateCron, s.runRates)
		if err != nil {
			return err
		}
		s.legacyRateID = entryID
	}
	if s.cfg.Retention.Cron != "" && s.hasRetention() {
		if _, err := s.cron.AddFunc(s.cfg.Retention.Cron, s.runRetention); err != nil {
			return err
		}
	}
	if err := s.configureSyncJobs(); err != nil {
		return err
	}
	if s.relay != nil {
		if _, err := s.cron.AddFunc("@every 30s", s.runPelicans); err != nil {
			return err
		}
	}
	s.cron.Start()
	s.retryTaskRunsWG.Add(1)
	go func() { defer s.retryTaskRunsWG.Done(); s.retryTaskRuns() }()
	if settings, settingsErr := s.settings.SyncSettings(); settingsErr == nil && settings.ChannelConfigured && settings.ChannelEnabled {
		// Populate channel-owned usage immediately after startup. This runs
		// asynchronously so API startup is not blocked by remote channel I/O.
		go s.runChannelSync()
	}
	s.log.Info("scheduler started",
		"balanceCron", s.cfg.BalanceCron,
		"rateCron", s.cfg.RateCron,
		"retentionCron", s.cfg.Retention.Cron,
		"dailyBalanceCron", "0 0 0 * * *",
		"concurrency", s.cfg.Concurrency,
	)
	return nil
}

func (s *Scheduler) runDailyBalances() {
	s.track(context.Background(), "daily_balances", 5*time.Minute, func(ctx context.Context) error {
		rows, err := s.rates.CaptureDailyBalances(time.Now())
		if err == nil {
			tasklog.Add(ctx, "INFO", "已保存 %d 条每日余额快照", rows)
		}
		return err
	})
}

func (s *Scheduler) SyncSettings() (storage.SyncSettings, error) {
	return s.settings.SyncSettings()
}

func (s *Scheduler) UpdateSyncSettings(settings storage.SyncSettings) error {
	if settings.AlertEvaluationIntervalSeconds != 60 || settings.AlertRateIntervalSeconds < 60 || settings.AlertRateIntervalSeconds > 120 || settings.AlertUsageIntervalSeconds < 60 || settings.AlertUsageIntervalSeconds > 120 || settings.AlertBalanceIntervalSeconds < 300 || settings.AlertBalanceIntervalSeconds > 600 {
		return fmt.Errorf("评估周期固定 60 秒；倍率与用量周期 60–120 秒；余额周期 300–600 秒")
	}
	if err := s.settings.SaveSyncSettings(settings); err != nil {
		return err
	}
	return s.configureSyncJobs()
}

func (s *Scheduler) configureSyncJobs() error {
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()

	settings, err := s.settings.SyncSettings()
	if err != nil {
		return err
	}
	if err := s.configureTokenRefreshJob(settings); err != nil {
		return err
	}
	if settings.ChannelConfigured {
		if s.legacyBalanceID != 0 {
			s.cron.Remove(s.legacyBalanceID)
			s.legacyBalanceID = 0
		}
		if s.legacyRateID != 0 {
			s.cron.Remove(s.legacyRateID)
			s.legacyRateID = 0
		}
		if s.channelSyncID != 0 {
			s.cron.Remove(s.channelSyncID)
			s.channelSyncID = 0
		}
		if settings.ChannelEnabled {
			s.channelSyncID, err = s.cron.AddFunc(fmt.Sprintf("@every %dm", settings.ChannelIntervalMinutes), s.runChannelSync)
			if err != nil {
				return err
			}
		}
	}
	if s.alertEvaluationID != 0 {
		s.cron.Remove(s.alertEvaluationID)
		s.alertEvaluationID = 0
	}
	for _, id := range []*cron.EntryID{&s.alertRateSyncID, &s.alertUsageSyncID, &s.alertBalanceSyncID} {
		if *id != 0 {
			s.cron.Remove(*id)
			*id = 0
		}
	}
	if s.alerts != nil && settings.AlertsEnabled {
		s.alertEvaluationID, err = s.cron.AddFunc(fmt.Sprintf("@every %ds", settings.AlertEvaluationIntervalSeconds), s.runAlertEvaluation)
		if err != nil {
			return err
		}
		s.alertRateSyncID, err = s.cron.AddFunc(fmt.Sprintf("@every %ds", settings.AlertRateIntervalSeconds), s.runAlertRateSync)
		if err != nil {
			return err
		}
		s.alertUsageSyncID, err = s.cron.AddFunc(fmt.Sprintf("@every %ds", settings.AlertUsageIntervalSeconds), s.runAlertUsageSync)
		if err != nil {
			return err
		}
		s.alertBalanceSyncID, err = s.cron.AddFunc(fmt.Sprintf("@every %ds", settings.AlertBalanceIntervalSeconds), s.runAlertBalanceSync)
		if err != nil {
			return err
		}
	}
	if s.relayRateSyncID != 0 {
		s.cron.Remove(s.relayRateSyncID)
		s.relayRateSyncID = 0
	}
	if s.relaySnapshotSyncID != 0 {
		s.cron.Remove(s.relaySnapshotSyncID)
		s.relaySnapshotSyncID = 0
	}
	if settings.RelaySnapshotConfigured && settings.RelaySnapshotEnabled {
		// The snapshot job is registered first so short intervals start promptly.
		// Per-job running flags prevent queued invocations while the shared relay
		// mutex serializes a snapshot refresh with a rate probe.
		s.relaySnapshotSyncID, err = s.cron.AddFunc(fmt.Sprintf("@every %ds", settings.RelaySnapshotIntervalSeconds), s.runRelaySnapshotSync)
		if err != nil {
			return err
		}
	}
	if settings.RelayRateConfigured && settings.RelayRateEnabled {
		s.relayRateSyncID, err = s.cron.AddFunc(fmt.Sprintf("@every %ds", settings.RelayRateIntervalSeconds), s.runRelayRateSync)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Scheduler) Stop() {
	if s.pelicanCancel != nil {
		s.pelicanCancel()
	}
	if s.cron != nil {
		<-s.cron.Stop().Done()
	}
	s.retryTaskRunsWG.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.flushTaskRunsContext(ctx)
}

func (s *Scheduler) runBalance() {
	s.track(context.Background(), "legacy_balance", 5*time.Minute, func(ctx context.Context) error { s.monitor.ScanAllBalances(ctx); return ctx.Err() })
}

func (s *Scheduler) runRates() {
	s.track(context.Background(), "legacy_rates", 5*time.Minute, func(ctx context.Context) error { s.monitor.ScanAllRates(ctx); return ctx.Err() })
}

func (s *Scheduler) runChannelSync() {
	if !s.channelSyncMu.TryLock() {
		s.log.Warn("skip overlapping channel sync")
		return
	}
	defer s.channelSyncMu.Unlock()
	s.track(context.Background(), "channel_sync", 20*time.Minute, func(ctx context.Context) error {
		result := s.monitor.SyncAll(ctx, false)
		tasklog.Add(ctx, "INFO", "渠道同步：总数 %d，成功 %d，失败 %d", result.Total, result.Succeeded, result.Failed)
		if result.Failed > 0 {
			return fmt.Errorf("%d 个渠道同步失败", result.Failed)
		}
		return ctx.Err()
	})
}

func (s *Scheduler) runAlertEvaluation() {
	if s.alerts == nil || !s.alertRunning.CompareAndSwap(false, true) {
		return
	}
	defer s.alertRunning.Store(false)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := s.alerts.EvaluateAll(ctx, time.Now().UTC()); err != nil {
		s.log.Warn("alert evaluation failed", "err", err)
	}
}

func (s *Scheduler) runAlertRateSync() {
	if s.alerts == nil {
		return
	}
	needed, ids, err := s.alerts.CollectionPlan("rate")
	if err != nil || !needed {
		return
	}
	if s.relay == nil || !s.relayRateRunning.CompareAndSwap(false, true) {
		return
	}
	defer s.relayRateRunning.Store(false)
	if !s.relaySyncMu.TryLock() {
		return
	}
	defer s.relaySyncMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := s.relay.SyncAlertStations(ctx, "rate", ids); err != nil {
		s.log.Warn("alert rate sync failed", "err", err)
	}
}

func (s *Scheduler) runAlertUsageSync() {
	if s.alerts == nil {
		return
	}
	needed, ids, err := s.alerts.CollectionPlan("usage")
	if err != nil || !needed {
		return
	}
	if s.relay == nil || !s.relaySnapshotRunning.CompareAndSwap(false, true) {
		return
	}
	defer s.relaySnapshotRunning.Store(false)
	if !s.relaySyncMu.TryLock() {
		return
	}
	defer s.relaySyncMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := s.relay.SyncAlertStations(ctx, "usage", ids); err != nil {
		s.log.Warn("alert usage sync failed", "err", err)
	}
}

func (s *Scheduler) runAlertBalanceSync() {
	if s.alerts == nil {
		return
	}
	needed, _, err := s.alerts.CollectionPlan("balance")
	if err != nil || !needed {
		return
	}
	if s.monitor == nil || !s.channelSyncMu.TryLock() {
		return
	}
	defer s.channelSyncMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	s.monitor.ScanAllBalances(ctx)
}

func (s *Scheduler) runRelayRateSync() {
	if !s.relayRateRunning.CompareAndSwap(false, true) {
		s.log.Warn("skip overlapping relay rate sync")
		return
	}
	defer s.relayRateRunning.Store(false)
	s.relaySyncMu.Lock()
	defer s.relaySyncMu.Unlock()
	s.track(context.Background(), "relay_rate_sync", 10*time.Minute, func(ctx context.Context) error {
		succeeded, failed, err := s.relay.SyncAllRates(ctx)
		tasklog.Add(ctx, "INFO", "中转站同步：成功 %d，失败 %d", succeeded, failed)
		if err == nil && failed > 0 {
			err = fmt.Errorf("%d 个中转站同步失败", failed)
		}
		return err
	})
}

func (s *Scheduler) runRelaySnapshotSync() {
	if !s.relaySnapshotRunning.CompareAndSwap(false, true) {
		s.log.Warn("skip overlapping relay snapshot sync")
		return
	}
	defer s.relaySnapshotRunning.Store(false)
	s.relaySyncMu.Lock()
	defer s.relaySyncMu.Unlock()
	s.track(context.Background(), "relay_snapshot_sync", 10*time.Minute, func(ctx context.Context) error {
		succeeded, failed, err := s.relay.SyncAllSnapshots(ctx)
		tasklog.Add(ctx, "INFO", "中转站同步：成功 %d，失败 %d", succeeded, failed)
		if err == nil && failed > 0 {
			err = fmt.Errorf("%d 个中转站同步失败", failed)
		}
		return err
	})
}

func (s *Scheduler) hasRetention() bool {
	r := s.cfg.Retention
	return r.MonitorLogsDays > 0 || r.BalanceSnapshotsDays > 0 || r.NotificationLogsDays > 0
}

// runRetention 按配置删除过期历史。任一表失败不影响其它，全部错误写日志。
func (s *Scheduler) runRetention() {
	s.track(context.Background(), "retention", 10*time.Minute, s.retainHistory)
}

func (s *Scheduler) retainHistory(ctx context.Context) error {
	r := s.cfg.Retention
	now := time.Now()

	if r.MonitorLogsDays > 0 {
		cutoff := now.AddDate(0, 0, -r.MonitorLogsDays)
		n, err := s.monLogs.DeleteBefore(cutoff)
		if err != nil {
			tasklog.Add(ctx, "ERROR", "monitor_logs: %v", err)
		} else if n > 0 {
			tasklog.Add(ctx, "INFO", "monitor_logs: deleted=%d before=%s", n, cutoff.Format(time.RFC3339))
		}
	}

	if r.BalanceSnapshotsDays > 0 {
		cutoff := now.AddDate(0, 0, -r.BalanceSnapshotsDays)
		n, err := s.rates.DeleteBalanceSnapshotsBefore(cutoff)
		if err != nil {
			tasklog.Add(ctx, "ERROR", "balance_snapshots: %v", err)
		} else if n > 0 {
			tasklog.Add(ctx, "INFO", "balance_snapshots: deleted=%d before=%s", n, cutoff.Format(time.RFC3339))
		}
	}

	if r.NotificationLogsDays > 0 {
		cutoff := now.AddDate(0, 0, -r.NotificationLogsDays)
		n, err := s.notifies.DeleteLogsBefore(cutoff)
		if err != nil {
			tasklog.Add(ctx, "ERROR", "notification_logs: %v", err)
		} else if n > 0 {
			tasklog.Add(ctx, "INFO", "notification_logs: deleted=%d before=%s", n, cutoff.Format(time.RFC3339))
		}
	}
	return ctx.Err()
}
