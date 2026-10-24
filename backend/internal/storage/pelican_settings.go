package storage

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

const DefaultPelicanPrompt = "\u521b\u5efa\u4e00\u4e2a HTML\uff0c\u5185\u5bb9\u662f SVG \u7ed8\u5236\u4e00\u4e2a\u9e48\u9e55\u9a91\u81ea\u884c\u8f66\u7684 2D \u52a8\u753b\uff0c\u4f60\u4e0d\u9700\u8981\u4efb\u4f55\u6d4b\u8bd5\u3002"

type PelicanSettings struct {
	PageEnabled     bool
	TasksEnabled    bool
	HistoryCount    int
	Model           string
	Prompt          string
	ReasoningEffort string
	ParallelCount   int
	CronExpression  string
	ActiveTimezone  string
	ActiveStart     string
	ActiveEnd       string
	ActiveWeekdays  []int
	Plans           []PelicanTestPlan
}

type PelicanTestPlan struct {
	Model           string `json:"model"`
	Prompt          string `json:"prompt"`
	ReasoningEffort string `json:"reasoning_effort"`
}

func normalizedPelicanPlans(plans []PelicanTestPlan, legacyModel, legacyPrompt, legacyEffort string) []PelicanTestPlan {
	if plans != nil && len(plans) == 0 {
		return []PelicanTestPlan{{Prompt: DefaultPelicanPrompt, ReasoningEffort: "medium"}}
	}
	if len(plans) > 0 {
		return append([]PelicanTestPlan(nil), plans...)
	}
	return []PelicanTestPlan{{Model: legacyModel, Prompt: legacyPrompt, ReasoningEffort: legacyEffort}}
}

func ConfiguredPelicanPlans(plans []PelicanTestPlan) []PelicanTestPlan {
	configured := make([]PelicanTestPlan, 0, len(plans))
	for _, plan := range plans {
		if len(PelicanModelPriorityList(plan.Model)) > 0 {
			configured = append(configured, plan)
		}
	}
	return configured
}

func PelicanModelPriorityList(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\uFF0C' || r == ';' || r == '\uFF1B' || r == '\n' || r == '\r'
	})
	models := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		model := strings.TrimSpace(field)
		if model == "" {
			continue
		}
		if _, exists := seen[model]; exists {
			continue
		}
		seen[model] = struct{}{}
		models = append(models, model)
	}
	return models
}

func (s UISettings) PelicanSettings() PelicanSettings {
	plans := normalizedPelicanPlans(s.PelicanPlans, s.PelicanModel, s.PelicanPrompt, s.PelicanReasoningEffort)
	return PelicanSettings{
		PageEnabled: s.PelicanPageEnabled, TasksEnabled: s.PelicanTasksEnabled, HistoryCount: s.PelicanHistoryCount,
		Model: plans[0].Model, Prompt: plans[0].Prompt, ReasoningEffort: plans[0].ReasoningEffort,
		ParallelCount: s.PelicanParallelCount, CronExpression: s.PelicanCronExpression, Plans: plans,
		ActiveTimezone: normalizePelicanTimezone(s.PelicanActiveTimezone),
		ActiveStart:    s.PelicanActiveStart, ActiveEnd: s.PelicanActiveEnd,
		ActiveWeekdays: normalizedPelicanWeekdays(s.PelicanActiveWeekdays),
	}
}

func NextPelicanRun(expression string, now time.Time) (time.Time, error) {
	return (PelicanSettings{CronExpression: expression, ActiveStart: "00:00", ActiveEnd: "00:00"}).NextRun(now)
}

func ValidatePelicanSettings(s PelicanSettings) error {
	if s.HistoryCount < 1 || s.HistoryCount > PelicanHistoryLimitMax {
		return errors.New("作品保留数量必须是 1 到 100")
	}
	plans := normalizedPelicanPlans(s.Plans, s.Model, s.Prompt, s.ReasoningEffort)
	if len(plans) > 8 {
		return errors.New("测智方案数量必须是 1 到 8 个")
	}
	for _, plan := range plans {
		if len(plan.Model) > 256 {
			return errors.New("请填写有效模型，最长 256 字节")
		}
		if len(plan.Prompt) > 32000 || strings.TrimSpace(plan.Prompt) == "" {
			return errors.New("题目不能为空，最长 32000 字节")
		}
		switch plan.ReasoningEffort {
		case "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra":
		default:
			return errors.New("思考强度无效")
		}
	}
	if len(plans) > 1 {
		for _, plan := range plans {
			if len(PelicanModelPriorityList(plan.Model)) == 0 {
				return errors.New("每个测智方案都必须配置模型优先列表")
			}
		}
	}
	if s.ParallelCount < 1 || s.ParallelCount > 8 {
		return errors.New("并行份数必须是 1 到 8")
	}
	if len(s.CronExpression) > 128 {
		return errors.New("Cron 表达式过长")
	}
	if err := validatePelicanWeekdays(s.ActiveWeekdays); err != nil {
		return err
	}
	_, err := s.NextRun(time.Now())
	return err
}

func (s *Settings) PelicanSettings() (PelicanSettings, error) {
	settings := defaultUISettings().PelicanSettings()
	for _, field := range []struct {
		key    string
		target *string
	}{
		{SettingPelicanModel, &settings.Model}, {SettingPelicanPrompt, &settings.Prompt},
		{SettingPelicanReasoningEffort, &settings.ReasoningEffort}, {SettingPelicanCronExpression, &settings.CronExpression},
		{SettingPelicanActiveTimezone, &settings.ActiveTimezone},
		{SettingPelicanActiveStart, &settings.ActiveStart}, {SettingPelicanActiveEnd, &settings.ActiveEnd},
	} {
		value, found, err := s.Get(field.key)
		if err != nil {
			return settings, err
		}
		if found {
			*field.target = value
		}
	}
	settings.ActiveTimezone = normalizePelicanTimezone(settings.ActiveTimezone)
	value, found, err := s.Get(SettingPelicanActiveWeekdays)
	if err != nil {
		return settings, err
	}
	if found {
		if err := json.Unmarshal([]byte(value), &settings.ActiveWeekdays); err != nil {
			return settings, err
		}
	}
	settings.ActiveWeekdays = normalizedPelicanWeekdays(settings.ActiveWeekdays)
	value, found, err = s.Get(SettingPelicanParallelCount)
	if err != nil {
		return settings, err
	}
	if found {
		settings.ParallelCount, err = strconv.Atoi(value)
		if err != nil {
			return settings, err
		}
	}
	value, found, err = s.Get(SettingPelicanPlans)
	if err != nil {
		return settings, err
	}
	if found {
		if err := json.Unmarshal([]byte(value), &settings.Plans); err != nil {
			return settings, err
		}
	} else {
		settings.Plans = normalizedPelicanPlans(nil, settings.Model, settings.Prompt, settings.ReasoningEffort)
	}
	if len(settings.Plans) == 0 {
		settings.Plans = normalizedPelicanPlans(settings.Plans, settings.Model, settings.Prompt, settings.ReasoningEffort)
	}
	settings.HistoryCount, err = s.PelicanHistoryCount()
	if err != nil {
		return settings, err
	}
	settings.Model, settings.Prompt, settings.ReasoningEffort = settings.Plans[0].Model, settings.Plans[0].Prompt, settings.Plans[0].ReasoningEffort
	for _, field := range []struct {
		key    string
		target *bool
	}{
		{SettingPelicanPageEnabled, &settings.PageEnabled},
		{SettingPelicanTasksEnabled, &settings.TasksEnabled},
	} {
		value, found, err := s.Get(field.key)
		if err != nil {
			return settings, err
		}
		if found {
			*field.target, err = strconv.ParseBool(strings.TrimSpace(value))
			if err != nil {
				return settings, err
			}
		}
	}
	return settings, ValidatePelicanSettings(settings)
}

func (s *Settings) PelicanHistoryCount() (int, error) {
	value, found, err := s.Get(SettingPelicanHistoryCount)
	if err != nil {
		return 0, err
	}
	if !found {
		return PelicanHistoryLimit, nil
	}
	count, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || count < 1 || count > PelicanHistoryLimitMax {
		return 0, errors.New("invalid pelican history count")
	}
	return count, nil
}

func (s *Settings) MigratePelicanSettings() error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		store := NewSettings(tx)
		if _, found, err := store.Get(SettingPelicanModel); err != nil || found {
			return err
		}
		var config PelicanConfig
		err := tx.Where("model <> ''").Order("updated_at DESC, id DESC").First(&config).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		settings := defaultUISettings()
		settings.PelicanModel = config.Model
		settings.PelicanPrompt = config.Prompt
		settings.PelicanReasoningEffort = config.ReasoningEffort
		settings.PelicanParallelCount = config.ParallelCount
		settings.PelicanCronExpression = config.CronExpression
		if err := ValidatePelicanSettings(settings.PelicanSettings()); err != nil {
			return err
		}
		values, err := uiSettingsValues(settings)
		if err != nil {
			return err
		}
		for _, key := range []string{SettingPelicanModel, SettingPelicanPrompt, SettingPelicanReasoningEffort, SettingPelicanParallelCount, SettingPelicanCronExpression, SettingPelicanPlans} {
			if err := store.Set(key, values[key]); err != nil {
				return err
			}
		}
		next, err := settings.PelicanSettings().NextRun(time.Now())
		if err != nil {
			return err
		}
		return tx.Model(&PelicanConfig{}).Where("cron_expression <> ?", settings.PelicanCronExpression).
			Updates(map[string]any{"cron_expression": settings.PelicanCronExpression, "next_run_at": next}).Error
	})
}
