package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type syncSettingsInput struct {
	ChannelTokenRefreshEnabled         *bool `json:"channel_token_refresh_enabled"`
	ChannelTokenRefreshIntervalMinutes *int  `json:"channel_token_refresh_interval_minutes" binding:"omitempty,min=1"`
	ChannelEnabled                     bool  `json:"channel_enabled"`
	ChannelIntervalMinutes             int   `json:"channel_interval_minutes"`
	RelayRateEnabled                   bool  `json:"relay_rate_enabled"`
	RelayRateIntervalMinutes           int   `json:"relay_rate_interval_minutes"`
	RelayRateIntervalSeconds           int   `json:"relay_rate_interval_seconds"`
	RelaySnapshotEnabled               bool  `json:"relay_snapshot_enabled"`
	RelaySnapshotIntervalMinutes       int   `json:"relay_snapshot_interval_minutes"`
	RelaySnapshotIntervalSeconds       int   `json:"relay_snapshot_interval_seconds"`
	AlertsEnabled                      *bool `json:"alerts_enabled"`
	AlertEvaluationIntervalSeconds     *int  `json:"alert_evaluation_interval_seconds"`
	AlertRateIntervalSeconds           *int  `json:"alert_rate_interval_seconds"`
	AlertUsageIntervalSeconds          *int  `json:"alert_usage_interval_seconds"`
	AlertBalanceIntervalSeconds        *int  `json:"alert_balance_interval_seconds"`
}

func registerSyncSettings(g *gin.RouterGroup, d *Deps) {
	g.GET("/sync-settings", func(c *gin.Context) {
		settings, err := d.Scheduler.SyncSettings()
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": settings})
	})
	g.PUT("/sync-settings", func(c *gin.Context) {
		var in syncSettingsInput
		if err := c.ShouldBindJSON(&in); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		settings, err := d.Scheduler.SyncSettings()
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		settings.ChannelEnabled = in.ChannelEnabled
		settings.ChannelIntervalMinutes = in.ChannelIntervalMinutes
		settings.RelayRateEnabled = in.RelayRateEnabled
		settings.RelayRateIntervalMinutes = in.RelayRateIntervalMinutes
		settings.RelayRateIntervalSeconds = in.RelayRateIntervalSeconds
		settings.RelaySnapshotEnabled = in.RelaySnapshotEnabled
		settings.RelaySnapshotIntervalMinutes = in.RelaySnapshotIntervalMinutes
		settings.RelaySnapshotIntervalSeconds = in.RelaySnapshotIntervalSeconds
		if in.AlertsEnabled != nil {
			settings.AlertsEnabled = *in.AlertsEnabled
		}
		if in.AlertEvaluationIntervalSeconds != nil {
			settings.AlertEvaluationIntervalSeconds = *in.AlertEvaluationIntervalSeconds
		}
		if in.AlertRateIntervalSeconds != nil {
			settings.AlertRateIntervalSeconds = *in.AlertRateIntervalSeconds
		}
		if in.AlertUsageIntervalSeconds != nil {
			settings.AlertUsageIntervalSeconds = *in.AlertUsageIntervalSeconds
		}
		if in.AlertBalanceIntervalSeconds != nil {
			settings.AlertBalanceIntervalSeconds = *in.AlertBalanceIntervalSeconds
		}
		if in.ChannelTokenRefreshEnabled != nil {
			settings.ChannelTokenRefreshEnabled = *in.ChannelTokenRefreshEnabled
		}
		if in.ChannelTokenRefreshIntervalMinutes != nil {
			settings.ChannelTokenRefreshIntervalMinutes = *in.ChannelTokenRefreshIntervalMinutes
		}
		if err := d.Scheduler.UpdateSyncSettings(settings); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		saved, err := d.Scheduler.SyncSettings()
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": saved})
	})
}
