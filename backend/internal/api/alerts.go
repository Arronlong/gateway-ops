package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/guliansheng/gateway-ops/internal/alert"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
)

type alertRuleInput struct {
	Name            string           `json:"name" binding:"required"`
	TargetType      alert.TargetType `json:"target_type" binding:"required"`
	Metric          alert.MetricKind `json:"metric" binding:"required"`
	Severity        string           `json:"severity"`
	Enabled         *bool            `json:"enabled"`
	Scope           json.RawMessage  `json:"scope"`
	Condition       json.RawMessage  `json:"condition"`
	Action          json.RawMessage  `json:"action"`
	Recovery        json.RawMessage  `json:"recovery"`
	CooldownSeconds int              `json:"cooldown_seconds"`
	Remark          string           `json:"remark"`
	ChannelIDs      []uint           `json:"channel_ids"`
}

func registerAlerts(g *gin.RouterGroup, d *Deps) {
	alerts := g.Group("/alerts")
	alerts.GET("/rules", func(c *gin.Context) {
		var enabled *bool
		if raw := c.Query("enabled"); raw != "" {
			value, err := strconv.ParseBool(raw)
			if err != nil {
				fail(c, http.StatusBadRequest, err)
				return
			}
			enabled = &value
		}
		list, err := d.Alerts.ListRules(storage.AlertRuleFilter{Enabled: enabled, TargetType: c.Query("target_type"), Metric: c.Query("metric")})
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": list})
	})
	alerts.POST("/rules", func(c *gin.Context) { createAlertRule(c, d) })
	alerts.GET("/rules/:id", func(c *gin.Context) { getAlertRule(c, d) })
	alerts.PUT("/rules/:id", func(c *gin.Context) { updateAlertRule(c, d) })
	alerts.PATCH("/rules/:id/enabled", func(c *gin.Context) {
		id, err := uintParam(c, "id")
		if err != nil {
			fail(c, 400, err)
			return
		}
		var in struct {
			Enabled *bool `json:"enabled"`
		}
		if err := c.ShouldBindJSON(&in); err != nil || in.Enabled == nil {
			fail(c, 400, errors.New("请提供 enabled 布尔值"))
			return
		}
		if err := d.Alerts.SetRuleEnabled(c.Request.Context(), id, *in.Enabled); err != nil {
			fail(c, 400, err)
			return
		}
		c.JSON(200, gin.H{"ok": true})
	})
	alerts.DELETE("/rules/:id", func(c *gin.Context) { deleteAlertRule(c, d) })
	alerts.POST("/rules/:id/test", func(c *gin.Context) { testAlertRule(c, d) })
	alerts.GET("/events", func(c *gin.Context) {
		stationID, parseErr := strconv.ParseUint(c.DefaultQuery("station_id", "0"), 10, 32)
		limit, limitErr := strconv.Atoi(c.DefaultQuery("limit", "50"))
		offset, offsetErr := strconv.Atoi(c.DefaultQuery("offset", "0"))
		if parseErr != nil || limitErr != nil || offsetErr != nil || limit < 1 || limit > 500 || offset < 0 {
			fail(c, 400, errors.New("查询参数无效"))
			return
		}
		if value := c.Query("state"); value != "" && value != "open" && value != "firing" && value != "recovering" && value != "recovered" {
			fail(c, 400, errors.New("事件状态无效"))
			return
		}
		if value := c.Query("target_type"); value != "" && value != "relay_account" && value != "relay_user" {
			fail(c, 400, errors.New("对象类型无效"))
			return
		}
		if value := c.Query("severity"); value != "" && value != "info" && value != "warning" && value != "critical" {
			fail(c, 400, errors.New("预警级别无效"))
			return
		}
		filter := storage.AlertEventFilter{State: c.Query("state"), TargetType: c.Query("target_type"), StationID: uint(stationID), Limit: limit, Offset: offset, Severity: c.Query("severity")}
		for _, bound := range []struct {
			key  string
			dest **time.Time
		}{{"from", &filter.From}, {"to", &filter.To}} {
			if raw := c.Query(bound.key); raw != "" {
				value, err := time.Parse(time.RFC3339, raw)
				if err != nil {
					fail(c, 400, errors.New("时间范围必须使用含时区的 RFC3339 格式"))
					return
				}
				*bound.dest = &value
			}
		}
		if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
			fail(c, 400, errors.New("开始时间不能晚于结束时间"))
			return
		}
		list, err := d.Alerts.ListEvents(filter)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": list})
	})
	alerts.GET("/events/:id", func(c *gin.Context) { getAlertEvent(c, d) })
	alerts.POST("/events/:id/ack", func(c *gin.Context) { updateAlertEventState(c, d, true) })
	alerts.POST("/events/:id/recover", func(c *gin.Context) { updateAlertEventState(c, d, false) })
	alerts.GET("/summary", func(c *gin.Context) {
		summary, err := d.Alerts.Summary(time.Now().UTC())
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		if channels, channelErr := d.Notifies.ListEnabledChannels(); channelErr == nil {
			summary.EnabledChannels = int64(len(channels))
		}
		c.JSON(http.StatusOK, gin.H{"data": summary})
	})
	alerts.GET("/settings", func(c *gin.Context) { getAlertSettings(c, d) })
	alerts.PUT("/settings", func(c *gin.Context) { updateAlertSettings(c, d) })
}

func createAlertRule(c *gin.Context, d *Deps) {
	var in alertRuleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	rule, err := alertRuleFromInput(0, in)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	if err := d.Alerts.CreateRule(rule, in.ChannelIDs); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rule})
}

func getAlertRule(c *gin.Context, d *Deps) {
	id, err := uintParam(c, "id")
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	rule, err := d.Alerts.FindRule(id)
	if err != nil {
		fail(c, http.StatusNotFound, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rule})
}

func updateAlertRule(c *gin.Context, d *Deps) {
	id, err := uintParam(c, "id")
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	var in alertRuleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	rule, err := alertRuleFromInput(id, in)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	if err := d.Alerts.UpdateRule(rule, in.ChannelIDs); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rule})
}

func deleteAlertRule(c *gin.Context, d *Deps) {
	id, err := uintParam(c, "id")
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	if err := d.Alerts.DeleteRule(id); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func testAlertRule(c *gin.Context, d *Deps) {
	id, err := uintParam(c, "id")
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	preview, err := d.Alerts.TestRule(c.Request.Context(), id, time.Now().UTC())
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": preview})
}

func getAlertEvent(c *gin.Context, d *Deps) {
	id, err := uintParam(c, "id")
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	event, err := d.Alerts.FindEvent(id)
	if err != nil {
		fail(c, http.StatusNotFound, err)
		return
	}
	logs, err := d.Alerts.ListActionLogs(id)
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	notifications, err := d.Notifies.AlertLogs(id)
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	rule, ruleErr := d.Alerts.FindRule(event.RuleID)
	if ruleErr != nil && !errors.Is(ruleErr, gorm.ErrRecordNotFound) {
		fail(c, 500, ruleErr)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"event": event, "actions": logs, "notifications": notifications, "rule": rule}})
}

func updateAlertEventState(c *gin.Context, d *Deps, acknowledge bool) {
	id, err := uintParam(c, "id")
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	if acknowledge {
		err = d.Alerts.AcknowledgeEvent(id, time.Now().UTC())
	} else {
		err = d.Alerts.RecoverEvent(id, time.Now().UTC())
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func alertRuleFromInput(id uint, in alertRuleInput) (*storage.AlertRule, error) {
	jsonValue := func(raw json.RawMessage) string {
		if len(raw) == 0 || string(raw) == "null" {
			return "{}"
		}
		return string(raw)
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	rule := &storage.AlertRule{ID: id, Name: in.Name, TargetType: string(in.TargetType), Metric: string(in.Metric), Severity: in.Severity, Enabled: enabled, ScopeJSON: jsonValue(in.Scope), ConditionJSON: jsonValue(in.Condition), ActionJSON: jsonValue(in.Action), RecoveryJSON: jsonValue(in.Recovery), CooldownSeconds: in.CooldownSeconds, Remark: in.Remark, ChannelIDs: in.ChannelIDs}
	if rule.Severity == "" {
		rule.Severity = "warning"
	}
	if !json.Valid([]byte(rule.ScopeJSON)) || !json.Valid([]byte(rule.ConditionJSON)) || !json.Valid([]byte(rule.ActionJSON)) || !json.Valid([]byte(rule.RecoveryJSON)) {
		return nil, errors.New("rule configuration must be valid JSON")
	}
	return rule, nil
}

type alertSettingsInput struct {
	Enabled           bool `json:"enabled"`
	EvaluationSeconds int  `json:"evaluation_interval_seconds"`
	RateSeconds       int  `json:"rate_interval_seconds"`
	UsageSeconds      int  `json:"usage_interval_seconds"`
	BalanceSeconds    int  `json:"balance_interval_seconds"`
}

func getAlertSettings(c *gin.Context, d *Deps) {
	settings, err := d.Scheduler.SyncSettings()
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"enabled": settings.AlertsEnabled, "evaluation_interval_seconds": settings.AlertEvaluationIntervalSeconds, "rate_interval_seconds": settings.AlertRateIntervalSeconds, "usage_interval_seconds": settings.AlertUsageIntervalSeconds, "balance_interval_seconds": settings.AlertBalanceIntervalSeconds}})
}

func updateAlertSettings(c *gin.Context, d *Deps) {
	var in alertSettingsInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	if in.EvaluationSeconds != 60 || in.RateSeconds < 60 || in.RateSeconds > 120 || in.UsageSeconds < 60 || in.UsageSeconds > 120 || in.BalanceSeconds < 300 || in.BalanceSeconds > 600 {
		fail(c, 400, fmt.Errorf("评估周期固定 60 秒；倍率与用量周期 60–120 秒；余额周期 300–600 秒"))
		return
	}
	settings, err := d.Scheduler.SyncSettings()
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	settings.AlertsEnabled = in.Enabled
	settings.AlertEvaluationIntervalSeconds = in.EvaluationSeconds
	settings.AlertRateIntervalSeconds = in.RateSeconds
	settings.AlertUsageIntervalSeconds = in.UsageSeconds
	settings.AlertBalanceIntervalSeconds = in.BalanceSeconds
	if err := d.Scheduler.UpdateSyncSettings(settings); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	getAlertSettings(c, d)
}
