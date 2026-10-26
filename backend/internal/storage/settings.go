package storage

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	SettingChannelSyncConfigured         = "sync.channel.configured"
	SettingChannelSyncEnabled            = "sync.channel.enabled"
	SettingChannelSyncInterval           = "sync.channel.interval_minutes"
	SettingRelayRateConfigured           = "sync.relay.rate.configured"
	SettingRelayRateEnabled              = "sync.relay.rate.enabled"
	SettingRelayRateInterval             = "sync.relay.rate.interval_minutes"
	SettingRelayRateIntervalSeconds      = "sync.relay.rate.interval_seconds"
	SettingRelaySnapshotConfigured       = "sync.relay.snapshot.configured"
	SettingRelaySnapshotEnabled          = "sync.relay.snapshot.enabled"
	SettingRelaySnapshotInterval         = "sync.relay.snapshot.interval_minutes"
	SettingRelaySnapshotIntervalSeconds  = "sync.relay.snapshot.interval_seconds"
	SettingAlertsEnabled                 = "alerts.enabled"
	SettingAlertEvaluationInterval       = "alerts.evaluation.interval_seconds"
	SettingAlertRateInterval             = "alerts.rate.interval_seconds"
	SettingAlertUsageInterval            = "alerts.usage.interval_seconds"
	SettingAlertBalanceInterval          = "alerts.balance.interval_seconds"
	SettingUIOperationsManagementEnabled = "ui.operations.management_enabled"
	SettingUIChannelsPageEnabled         = "ui.channels.page_enabled"
	SettingUIChannelsShowRateChanges     = "ui.channels.show_rate_changes"
	SettingUIChannelsShowChannelInfo     = "ui.channels.show_channel_info"
	SettingUIChannelsShowRecentLatency   = "ui.channels.show_recent_latency"
	SettingUIChannelsShowGroups          = "ui.channels.show_groups"
	SettingUIChannelsGroupsExpanded      = "ui.channels.groups_expanded"
	SettingUIChannelsShowStability       = "ui.channels.show_stability"
	SettingUIChannelsShowConsumption     = "ui.channels.show_consumption"
	SettingUIRecentMetricsLimit          = "ui.monitor.recent_metrics_limit"
	SettingUIRecentMetricsWindowHours    = "ui.monitor.recent_metrics_window_hours"
	SettingPelicanModel                  = "pelican.model"
	SettingPelicanPrompt                 = "pelican.prompt"
	SettingPelicanReasoningEffort        = "pelican.reasoning_effort"
	SettingPelicanParallelCount          = "pelican.parallel_count"
	SettingPelicanCronExpression         = "pelican.cron_expression"
	SettingPelicanActiveTimezone         = "pelican.active_timezone"
	SettingPelicanActiveStart            = "pelican.active_start"
	SettingPelicanActiveEnd              = "pelican.active_end"
	SettingPelicanActiveWeekdays         = "pelican.active_weekdays"
	SettingPelicanPlans                  = "pelican.plans"
	SettingPelicanHistoryCount           = "pelican.history_count"
	SettingPelicanPageEnabled            = "pelican.page_enabled"
	SettingPelicanTasksEnabled           = "pelican.tasks_enabled"
	SettingSecurityIframeEnabled         = "security.iframe.enabled"
	SettingSecurityIframeMode            = "security.iframe.mode"
	SettingSecurityIframeAllowlist       = "security.iframe.allowlist"

	settingLegacyRelaySyncConfigured = "sync.relay.configured"
	settingLegacyRelaySyncEnabled    = "sync.relay.enabled"
	settingLegacyRelaySyncInterval   = "sync.relay.interval_minutes"
)

type Settings struct{ db *gorm.DB }

func NewSettings(db *gorm.DB) *Settings { return &Settings{db: db} }

func (s *Settings) Get(key string) (string, bool, error) {
	var setting AppSetting
	err := s.db.First(&setting, "key = ?", key).Error
	if err == gorm.ErrRecordNotFound {
		return "", false, nil
	}
	return setting.Value, err == nil, err
}

func (s *Settings) Set(key, value string) error {
	return s.db.Where("key = ?", key).
		Assign(map[string]any{"value": value, "updated_at": time.Now().UTC()}).
		FirstOrCreate(&AppSetting{Key: key, Value: value}).Error
}

func (s *Settings) SyncSettings() (SyncSettings, error) {
	tokenEnabled, tokenInterval, tokenErr := s.TokenRefreshSettings()
	if tokenErr != nil {
		return SyncSettings{}, tokenErr
	}
	settings := SyncSettings{
		ChannelTokenRefreshEnabled:         tokenEnabled,
		ChannelTokenRefreshIntervalMinutes: tokenInterval,
		ChannelIntervalMinutes:             30,
		RelayRateIntervalMinutes:           1,
		RelayRateIntervalSeconds:           10,
		RelaySnapshotIntervalMinutes:       60,
		RelaySnapshotIntervalSeconds:       3600,
		AlertsEnabled:                      true,
		AlertEvaluationIntervalSeconds:     60,
		AlertRateIntervalSeconds:           60,
		AlertUsageIntervalSeconds:          60,
		AlertBalanceIntervalSeconds:        300,
	}
	readBool := func(key string, target *bool) error {
		value, found, err := s.Get(key)
		if err != nil || !found {
			return err
		}
		*target, err = strconv.ParseBool(value)
		return err
	}
	readInt := func(key string, target *int) (bool, error) {
		value, found, err := s.Get(key)
		if err != nil || !found {
			return found, err
		}
		*target, err = strconv.Atoi(value)
		if err == nil && *target < 1 {
			*target = 1
		}
		return found, err
	}
	var err error
	if _, settings.ChannelConfigured, err = s.Get(SettingChannelSyncConfigured); err != nil {
		return settings, err
	}
	var legacyRelayConfigured bool
	if _, legacyRelayConfigured, err = s.Get(settingLegacyRelaySyncConfigured); err != nil {
		return settings, err
	}
	if err = readBool(SettingChannelSyncEnabled, &settings.ChannelEnabled); err != nil {
		return settings, err
	}
	var legacyRelayEnabled bool
	if err = readBool(settingLegacyRelaySyncEnabled, &legacyRelayEnabled); err != nil {
		return settings, err
	}
	if _, err = readInt(SettingChannelSyncInterval, &settings.ChannelIntervalMinutes); err != nil {
		return settings, err
	}
	legacyRelayInterval := 60
	legacyRelayIntervalConfigured, err := readInt(settingLegacyRelaySyncInterval, &legacyRelayInterval)
	if err != nil {
		return settings, err
	}
	if legacyRelayConfigured || legacyRelayIntervalConfigured {
		settings.RelayRateConfigured = legacyRelayConfigured
		settings.RelayRateEnabled = legacyRelayEnabled
		settings.RelayRateIntervalMinutes = legacyRelayInterval
		settings.RelayRateIntervalSeconds = legacyRelayInterval * 60
		settings.RelaySnapshotConfigured = legacyRelayConfigured
		settings.RelaySnapshotEnabled = legacyRelayEnabled
		settings.RelaySnapshotIntervalMinutes = legacyRelayInterval
		settings.RelaySnapshotIntervalSeconds = legacyRelayInterval * 60
	}

	if _, configured, getErr := s.Get(SettingRelayRateConfigured); getErr != nil {
		return settings, getErr
	} else if configured {
		settings.RelayRateConfigured = true
		if err = readBool(SettingRelayRateEnabled, &settings.RelayRateEnabled); err != nil {
			return settings, err
		}
		if configured, readErr := readInt(SettingRelayRateInterval, &settings.RelayRateIntervalMinutes); readErr != nil {
			return settings, readErr
		} else if configured {
			settings.RelayRateIntervalSeconds = settings.RelayRateIntervalMinutes * 60
		}
		if _, configured, getErr := s.Get(SettingRelayRateIntervalSeconds); getErr != nil {
			return settings, getErr
		} else if configured {
			if _, readErr := readInt(SettingRelayRateIntervalSeconds, &settings.RelayRateIntervalSeconds); readErr != nil {
				return settings, readErr
			}
		}
	}
	if _, configured, getErr := s.Get(SettingRelaySnapshotConfigured); getErr != nil {
		return settings, getErr
	} else if configured {
		settings.RelaySnapshotConfigured = true
		if err = readBool(SettingRelaySnapshotEnabled, &settings.RelaySnapshotEnabled); err != nil {
			return settings, err
		}
		if _, readErr := readInt(SettingRelaySnapshotInterval, &settings.RelaySnapshotIntervalMinutes); readErr != nil {
			err = readErr
			return settings, err
		}
		settings.RelaySnapshotIntervalSeconds = settings.RelaySnapshotIntervalMinutes * 60
		if _, secondsConfigured, getErr := s.Get(SettingRelaySnapshotIntervalSeconds); getErr != nil {
			return settings, getErr
		} else if secondsConfigured {
			if _, readErr := readInt(SettingRelaySnapshotIntervalSeconds, &settings.RelaySnapshotIntervalSeconds); readErr != nil {
				err = readErr
				return settings, err
			}
		}
	}
	if err = readBool(SettingAlertsEnabled, &settings.AlertsEnabled); err != nil {
		return settings, err
	}
	if _, err = readInt(SettingAlertEvaluationInterval, &settings.AlertEvaluationIntervalSeconds); err != nil {
		return settings, err
	}
	if _, err = readInt(SettingAlertRateInterval, &settings.AlertRateIntervalSeconds); err != nil {
		return settings, err
	}
	if _, err = readInt(SettingAlertUsageInterval, &settings.AlertUsageIntervalSeconds); err != nil {
		return settings, err
	}
	if _, err = readInt(SettingAlertBalanceInterval, &settings.AlertBalanceIntervalSeconds); err != nil {
		return settings, err
	}
	return settings, nil
}

func (s *Settings) SaveSyncSettings(settings SyncSettings) error {
	if settings.ChannelTokenRefreshIntervalMinutes < 1 {
		settings.ChannelTokenRefreshIntervalMinutes = 30
	}
	if settings.ChannelIntervalMinutes < 1 {
		settings.ChannelIntervalMinutes = 30
	}
	if settings.RelayRateIntervalSeconds < 5 {
		if settings.RelayRateIntervalMinutes > 0 {
			settings.RelayRateIntervalSeconds = settings.RelayRateIntervalMinutes * 60
		} else {
			settings.RelayRateIntervalSeconds = 10
		}
	}
	if settings.RelayRateIntervalSeconds < 5 {
		settings.RelayRateIntervalSeconds = 10
	}
	settings.RelayRateIntervalMinutes = (settings.RelayRateIntervalSeconds + 59) / 60
	if settings.RelaySnapshotIntervalSeconds < 5 {
		if settings.RelaySnapshotIntervalMinutes > 0 {
			settings.RelaySnapshotIntervalSeconds = settings.RelaySnapshotIntervalMinutes * 60
		} else {
			settings.RelaySnapshotIntervalSeconds = 3600
		}
	}
	settings.RelaySnapshotIntervalMinutes = (settings.RelaySnapshotIntervalSeconds + 59) / 60
	if settings.AlertEvaluationIntervalSeconds < 30 {
		settings.AlertEvaluationIntervalSeconds = 60
	}
	if settings.AlertRateIntervalSeconds < 30 {
		settings.AlertRateIntervalSeconds = 60
	}
	if settings.AlertUsageIntervalSeconds < 30 {
		settings.AlertUsageIntervalSeconds = 60
	}
	if settings.AlertBalanceIntervalSeconds < 60 {
		settings.AlertBalanceIntervalSeconds = 300
	}
	values := map[string]string{
		SettingChannelTokenRefreshEnabled:   strconv.FormatBool(settings.ChannelTokenRefreshEnabled),
		SettingChannelTokenRefreshInterval:  strconv.Itoa(settings.ChannelTokenRefreshIntervalMinutes),
		SettingChannelSyncConfigured:        "true",
		SettingChannelSyncEnabled:           strconv.FormatBool(settings.ChannelEnabled),
		SettingChannelSyncInterval:          strconv.Itoa(settings.ChannelIntervalMinutes),
		SettingRelayRateConfigured:          "true",
		SettingRelayRateEnabled:             strconv.FormatBool(settings.RelayRateEnabled),
		SettingRelayRateInterval:            strconv.Itoa(settings.RelayRateIntervalMinutes),
		SettingRelayRateIntervalSeconds:     strconv.Itoa(settings.RelayRateIntervalSeconds),
		SettingRelaySnapshotConfigured:      "true",
		SettingRelaySnapshotEnabled:         strconv.FormatBool(settings.RelaySnapshotEnabled),
		SettingRelaySnapshotInterval:        strconv.Itoa(settings.RelaySnapshotIntervalMinutes),
		SettingRelaySnapshotIntervalSeconds: strconv.Itoa(settings.RelaySnapshotIntervalSeconds),
		SettingAlertsEnabled:                strconv.FormatBool(settings.AlertsEnabled),
		SettingAlertEvaluationInterval:      strconv.Itoa(settings.AlertEvaluationIntervalSeconds),
		SettingAlertRateInterval:            strconv.Itoa(settings.AlertRateIntervalSeconds),
		SettingAlertUsageInterval:           strconv.Itoa(settings.AlertUsageIntervalSeconds),
		SettingAlertBalanceInterval:         strconv.Itoa(settings.AlertBalanceIntervalSeconds),
	}
	for key, value := range values {
		if err := s.Set(key, strings.TrimSpace(value)); err != nil {
			return err
		}
	}
	return nil
}

func defaultUISettings() UISettings {
	return UISettings{
		OperationsManagementEnabled: true,
		ChannelsPageEnabled:         true,
		ChannelsShowRateChanges:     true,
		ChannelsShowChannelInfo:     true,
		ChannelsShowRecentLatency:   true,
		ChannelsShowGroups:          true,
		ChannelsGroupsExpanded:      false,
		ChannelsShowStability:       true,
		ChannelsShowConsumption:     true,
		RecentMetricsWindowHours:    1,
		RecentMetricsLimit:          60,
		PelicanPageEnabled:          true,
		PelicanTasksEnabled:         true,
		PelicanHistoryCount:         PelicanHistoryLimit,
		PelicanPrompt:               DefaultPelicanPrompt,
		PelicanReasoningEffort:      "medium",
		PelicanParallelCount:        1,
		PelicanCronExpression:       "*/30 * * * *",
		PelicanActiveTimezone:       "Asia/Shanghai",
		PelicanActiveStart:          "08:00",
		PelicanActiveEnd:            "22:00",
		PelicanActiveWeekdays:       defaultPelicanWeekdays(),
		IframeEmbeddingEnabled:      false,
		IframeEmbeddingMode:         IframeEmbeddingModeAllowlist,
		IframeEmbeddingAllowlist:    []string{},
	}
}

func decodeStringSliceSetting(value string) ([]string, error) {
	var items []string
	if err := json.Unmarshal([]byte(value), &items); err != nil {
		return nil, err
	}
	if items == nil {
		return []string{}, nil
	}
	return items, nil
}

func uiSettingsValues(settings UISettings) (map[string]string, error) {
	plans := normalizedPelicanPlans(settings.PelicanPlans, settings.PelicanModel, settings.PelicanPrompt, settings.PelicanReasoningEffort)
	plansJSON, err := json.Marshal(plans)
	if err != nil {
		return nil, err
	}
	allowlistJSON, err := json.Marshal(settings.IframeEmbeddingAllowlist)
	if err != nil {
		return nil, err
	}
	weekdaysJSON, err := json.Marshal(normalizedPelicanWeekdays(settings.PelicanActiveWeekdays))
	if err != nil {
		return nil, err
	}
	return map[string]string{
		SettingUIOperationsManagementEnabled: strconv.FormatBool(settings.OperationsManagementEnabled),
		SettingUIChannelsPageEnabled:         strconv.FormatBool(settings.ChannelsPageEnabled),
		SettingUIChannelsShowRateChanges:     strconv.FormatBool(settings.ChannelsShowRateChanges),
		SettingUIChannelsShowChannelInfo:     strconv.FormatBool(settings.ChannelsShowChannelInfo),
		SettingUIChannelsShowRecentLatency:   strconv.FormatBool(settings.ChannelsShowRecentLatency),
		SettingUIChannelsShowGroups:          strconv.FormatBool(settings.ChannelsShowGroups),
		SettingUIChannelsGroupsExpanded:      strconv.FormatBool(settings.ChannelsGroupsExpanded),
		SettingUIChannelsShowStability:       strconv.FormatBool(settings.ChannelsShowStability),
		SettingUIChannelsShowConsumption:     strconv.FormatBool(settings.ChannelsShowConsumption),
		SettingUIRecentMetricsWindowHours:    strconv.Itoa(settings.RecentMetricsWindowHours),
		SettingUIRecentMetricsLimit:          strconv.Itoa(settings.RecentMetricsLimit),
		SettingPelicanModel:                  plans[0].Model,
		SettingPelicanPrompt:                 plans[0].Prompt,
		SettingPelicanReasoningEffort:        plans[0].ReasoningEffort,
		SettingPelicanParallelCount:          strconv.Itoa(settings.PelicanParallelCount),
		SettingPelicanCronExpression:         settings.PelicanCronExpression,
		SettingPelicanActiveTimezone:         normalizePelicanTimezone(settings.PelicanActiveTimezone),
		SettingPelicanActiveStart:            settings.PelicanActiveStart,
		SettingPelicanActiveEnd:              settings.PelicanActiveEnd,
		SettingPelicanActiveWeekdays:         string(weekdaysJSON),
		SettingPelicanPlans:                  string(plansJSON),
		SettingPelicanHistoryCount:           strconv.Itoa(settings.PelicanHistoryCount),
		SettingPelicanPageEnabled:            strconv.FormatBool(settings.PelicanPageEnabled),
		SettingPelicanTasksEnabled:           strconv.FormatBool(settings.PelicanTasksEnabled),
		SettingSecurityIframeEnabled:         strconv.FormatBool(settings.IframeEmbeddingEnabled),
		SettingSecurityIframeMode:            strings.TrimSpace(settings.IframeEmbeddingMode),
		SettingSecurityIframeAllowlist:       string(allowlistJSON),
	}, nil
}

func (s *Settings) UISettings() (UISettings, error) {
	settings := defaultUISettings()
	readBool := func(key string, target *bool) error {
		value, found, err := s.Get(key)
		if err != nil || !found {
			return err
		}
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		*target = parsed
		return nil
	}
	readString := func(key string, target *string) error {
		value, found, err := s.Get(key)
		if err != nil || !found {
			return err
		}
		*target = strings.TrimSpace(value)
		return nil
	}
	readWindowHours := func(target *int) error {
		value, found, err := s.Get(SettingUIRecentMetricsWindowHours)
		if err != nil || !found {
			return err
		}
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || parsed < 1 || parsed > 24 {
			return fmt.Errorf("invalid recent metrics window hours %q", value)
		}
		*target = parsed
		return nil
	}
	readStringSlice := func(key string, target *[]string) error {
		value, found, err := s.Get(key)
		if err != nil || !found {
			return err
		}
		decoded, err := decodeStringSliceSetting(value)
		if err != nil {
			return err
		}
		*target = decoded
		return nil
	}

	if err := readBool(SettingUIOperationsManagementEnabled, &settings.OperationsManagementEnabled); err != nil {
		return settings, err
	}
	if err := readBool(SettingUIChannelsPageEnabled, &settings.ChannelsPageEnabled); err != nil {
		return settings, err
	}
	if err := readBool(SettingUIChannelsShowRateChanges, &settings.ChannelsShowRateChanges); err != nil {
		return settings, err
	}
	if err := readBool(SettingUIChannelsShowChannelInfo, &settings.ChannelsShowChannelInfo); err != nil {
		return settings, err
	}
	if err := readBool(SettingUIChannelsShowRecentLatency, &settings.ChannelsShowRecentLatency); err != nil {
		return settings, err
	}
	if err := readBool(SettingUIChannelsShowGroups, &settings.ChannelsShowGroups); err != nil {
		return settings, err
	}
	if err := readBool(SettingUIChannelsGroupsExpanded, &settings.ChannelsGroupsExpanded); err != nil {
		return settings, err
	}
	if err := readBool(SettingUIChannelsShowStability, &settings.ChannelsShowStability); err != nil {
		return settings, err
	}
	if err := readBool(SettingUIChannelsShowConsumption, &settings.ChannelsShowConsumption); err != nil {
		return settings, err
	}
	if value, found, err := s.Get(SettingUIRecentMetricsLimit); err != nil {
		return settings, err
	} else if found {
		limit, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || limit < 1 || limit > 200 {
			return settings, fmt.Errorf("invalid recent metrics limit %q", value)
		}
		settings.RecentMetricsLimit = limit
	}
	if err := readWindowHours(&settings.RecentMetricsWindowHours); err != nil {
		return settings, err
	}
	if err := readBool(SettingSecurityIframeEnabled, &settings.IframeEmbeddingEnabled); err != nil {
		return settings, err
	}
	if err := readString(SettingSecurityIframeMode, &settings.IframeEmbeddingMode); err != nil {
		return settings, err
	}
	if err := readStringSlice(SettingSecurityIframeAllowlist, &settings.IframeEmbeddingAllowlist); err != nil {
		return settings, err
	}
	pelican, err := s.PelicanSettings()
	if err != nil {
		return settings, err
	}
	settings.PelicanModel = pelican.Model
	settings.PelicanPrompt = pelican.Prompt
	settings.PelicanReasoningEffort = pelican.ReasoningEffort
	settings.PelicanParallelCount = pelican.ParallelCount
	settings.PelicanCronExpression = pelican.CronExpression
	settings.PelicanActiveTimezone = pelican.ActiveTimezone
	settings.PelicanActiveStart = pelican.ActiveStart
	settings.PelicanActiveEnd = pelican.ActiveEnd
	settings.PelicanActiveWeekdays = pelican.ActiveWeekdays
	settings.PelicanPlans = pelican.Plans
	settings.PelicanPageEnabled = pelican.PageEnabled
	settings.PelicanTasksEnabled = pelican.TasksEnabled
	settings.PelicanHistoryCount = pelican.HistoryCount
	return settings, nil
}

func (s *Settings) SaveUISettings(settings UISettings) error {
	values, err := uiSettingsValues(settings)
	if err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		store := NewSettings(tx)
		previousHistoryCount, err := store.PelicanHistoryCount()
		if err != nil {
			return err
		}
		scheduleWindowChanged := false
		for _, field := range []struct {
			key, value string
		}{
			{SettingPelicanActiveTimezone, values[SettingPelicanActiveTimezone]},
			{SettingPelicanActiveStart, settings.PelicanActiveStart}, {SettingPelicanActiveEnd, settings.PelicanActiveEnd},
			{SettingPelicanActiveWeekdays, values[SettingPelicanActiveWeekdays]},
		} {
			value, found, err := store.Get(field.key)
			if err != nil {
				return err
			}
			if !found || value != field.value {
				scheduleWindowChanged = true
			}
		}
		for key, value := range values {
			if err := store.Set(key, value); err != nil {
				return err
			}
		}
		if settings.PelicanHistoryCount < previousHistoryCount {
			if err := tx.Exec(`DELETE FROM relay_pelican_results WHERE id IN (
				SELECT id FROM (
					SELECT id, ROW_NUMBER() OVER (PARTITION BY relay_station_id, account_external_id ORDER BY finished_at DESC, id DESC) AS row_number
					FROM relay_pelican_results
				) ranked WHERE row_number > ?
			)`, settings.PelicanHistoryCount).Error; err != nil {
				return err
			}
			if err := NewRelayStations(tx).PruneGroupPelicanResults(settings.PelicanHistoryCount); err != nil {
				return err
			}
		}
		next, err := settings.PelicanSettings().NextRun(time.Now())
		if err != nil {
			return err
		}
		query := tx.Model(&PelicanConfig{})
		if !scheduleWindowChanged {
			query = query.Where("cron_expression <> ?", settings.PelicanCronExpression)
		} else {
			query = query.Where("1 = 1")
		}
		if err := query.Updates(map[string]any{"cron_expression": settings.PelicanCronExpression, "next_run_at": next}).Error; err != nil {
			return err
		}
		groupQuery := tx.Model(&GroupPelicanConfig{})
		if !scheduleWindowChanged {
			groupQuery = groupQuery.Where("cron_expression <> ?", settings.PelicanCronExpression)
		} else {
			groupQuery = groupQuery.Where("1 = 1")
		}
		return groupQuery.Updates(map[string]any{"cron_expression": settings.PelicanCronExpression, "next_run_at": next}).Error
	})
}
