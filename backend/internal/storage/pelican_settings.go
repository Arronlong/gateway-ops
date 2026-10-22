package storage

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

const DefaultPelicanPrompt = "\u521b\u5efa\u4e00\u4e2a HTML\uff0c\u5185\u5bb9\u662f SVG \u7ed8\u5236\u4e00\u4e2a\u9e48\u9e55\u9a91\u81ea\u884c\u8f66\u7684 2D \u52a8\u753b\uff0c\u4f60\u4e0d\u9700\u8981\u4efb\u4f55\u6d4b\u8bd5\u3002"

type PelicanSettings struct {
	Model           string
	Prompt          string
	ReasoningEffort string
	ParallelCount   int
	CronExpression  string
}

func (s UISettings) PelicanSettings() PelicanSettings {
	return PelicanSettings{
		Model: s.PelicanModel, Prompt: s.PelicanPrompt, ReasoningEffort: s.PelicanReasoningEffort,
		ParallelCount: s.PelicanParallelCount, CronExpression: s.PelicanCronExpression,
	}
}

func NextPelicanRun(expression string, now time.Time) (time.Time, error) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse("CRON_TZ=Asia/Shanghai " + expression)
	if err != nil {
		return time.Time{}, fmt.Errorf("\u9891\u7387\u5fc5\u987b\u662f\u6709\u6548\u7684\u4e94\u6bb5 Cron: %w", err)
	}
	next := schedule.Next(now)
	if next.IsZero() {
		return time.Time{}, errors.New("\u9891\u7387\u6ca1\u6709\u53ef\u6267\u884c\u7684\u4e0b\u6b21\u65f6\u95f4")
	}
	return next.UTC(), nil
}

func ValidatePelicanSettings(s PelicanSettings) error {
	if len(s.Model) > 256 || s.Model != strings.TrimSpace(s.Model) {
		return errors.New("\u8bf7\u586b\u5199\u6709\u6548\u6a21\u578b\uff0c\u6700\u957f 256 \u5b57\u8282")
	}
	if len(s.Prompt) > 32000 || strings.TrimSpace(s.Prompt) == "" {
		return errors.New("\u9898\u76ee\u4e0d\u80fd\u4e3a\u7a7a\uff0c\u6700\u957f 32000 \u5b57\u8282")
	}
	switch s.ReasoningEffort {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra":
	default:
		return errors.New("\u601d\u8003\u5f3a\u5ea6\u65e0\u6548")
	}
	if s.ParallelCount < 1 || s.ParallelCount > 8 {
		return errors.New("\u5e76\u884c\u4efd\u6570\u5fc5\u987b\u662f 1 \u5230 8")
	}
	if len(s.CronExpression) > 128 {
		return errors.New("Cron \u8868\u8fbe\u5f0f\u8fc7\u957f")
	}
	_, err := NextPelicanRun(s.CronExpression, time.Now())
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
	} {
		value, found, err := s.Get(field.key)
		if err != nil {
			return settings, err
		}
		if found {
			*field.target = value
		}
	}
	value, found, err := s.Get(SettingPelicanParallelCount)
	if err != nil {
		return settings, err
	}
	if found {
		settings.ParallelCount, err = strconv.Atoi(value)
		if err != nil {
			return settings, err
		}
	}
	return settings, ValidatePelicanSettings(settings)
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
		for _, key := range []string{SettingPelicanModel, SettingPelicanPrompt, SettingPelicanReasoningEffort, SettingPelicanParallelCount, SettingPelicanCronExpression} {
			if err := store.Set(key, values[key]); err != nil {
				return err
			}
		}
		next, err := NextPelicanRun(settings.PelicanCronExpression, time.Now())
		if err != nil {
			return err
		}
		return tx.Model(&PelicanConfig{}).Where("cron_expression <> ?", settings.PelicanCronExpression).
			Updates(map[string]any{"cron_expression": settings.PelicanCronExpression, "next_run_at": next}).Error
	})
}
