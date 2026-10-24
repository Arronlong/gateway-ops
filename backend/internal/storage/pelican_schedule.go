package storage

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/robfig/cron/v3"
)

func normalizePelicanTimezone(timezone string) string {
	if strings.TrimSpace(timezone) == "" {
		return "Asia/Shanghai"
	}
	return strings.TrimSpace(timezone)
}

func defaultPelicanWeekdays() []int {
	return []int{int(time.Sunday), int(time.Monday), int(time.Tuesday), int(time.Wednesday), int(time.Thursday), int(time.Friday), int(time.Saturday)}
}

func normalizedPelicanWeekdays(weekdays []int) []int {
	if weekdays == nil {
		return defaultPelicanWeekdays()
	}
	normalized := append([]int{}, weekdays...)
	sort.Ints(normalized)
	return normalized
}

func validatePelicanWeekdays(weekdays []int) error {
	if weekdays == nil {
		return nil
	}
	if len(weekdays) == 0 || len(weekdays) > 7 {
		return errors.New("每周至少选择一天")
	}
	seen := make(map[int]struct{}, len(weekdays))
	for _, weekday := range weekdays {
		if weekday < int(time.Sunday) || weekday > int(time.Saturday) {
			return errors.New("运行星期无效")
		}
		if _, exists := seen[weekday]; exists {
			return errors.New("运行星期不能重复")
		}
		seen[weekday] = struct{}{}
	}
	return nil
}

func pelicanWeekdaySelected(weekdays []int, weekday time.Weekday) bool {
	for _, selected := range normalizedPelicanWeekdays(weekdays) {
		if selected == int(weekday) {
			return true
		}
	}
	return false
}

type pelicanWindow struct{ start, end int }

func parsePelicanTime(value string) (int, error) {
	if len(value) != 5 || value[2] != ':' {
		return 0, errors.New("\u81ea\u52a8\u6d4b\u667a\u65f6\u95f4\u5fc5\u987b\u4f7f\u7528 HH:mm \u683c\u5f0f")
	}
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return 0, errors.New("\u81ea\u52a8\u6d4b\u667a\u65f6\u95f4\u5fc5\u987b\u4ecb\u4e8e 00:00 \u548c 23:59 \u4e4b\u95f4")
	}
	return parsed.Hour()*60 + parsed.Minute(), nil
}

func (s PelicanSettings) activeWindow() (pelicanWindow, *time.Location, error) {
	location, err := time.LoadLocation(normalizePelicanTimezone(s.ActiveTimezone))
	if err != nil {
		return pelicanWindow{}, nil, errors.New("测智时区无效")
	}
	start, err := parsePelicanTime(s.ActiveStart)
	if err != nil {
		return pelicanWindow{}, nil, err
	}
	end, err := parsePelicanTime(s.ActiveEnd)
	return pelicanWindow{start: start, end: end}, location, err
}

func (w pelicanWindow) contains(now time.Time, location *time.Location) bool {
	local := now.In(location)
	minute := local.Hour()*60 + local.Minute()
	if w.start == w.end {
		return true
	}
	if w.start < w.end {
		return minute >= w.start && minute < w.end
	}
	return minute >= w.start || minute < w.end
}

func (s PelicanSettings) AutomaticRunAllowed(now time.Time) bool {
	window, location, err := s.activeWindow()
	if err != nil || !window.contains(now, location) {
		return false
	}
	local := now.In(location)
	minute := local.Hour()*60 + local.Minute()
	weekday := local.Weekday()
	if window.start > window.end && minute < window.end {
		weekday = (weekday + 6) % 7
	}
	return pelicanWeekdaySelected(s.ActiveWeekdays, weekday)
}

func (s PelicanSettings) nextWindowOpening(after time.Time, window pelicanWindow, location *time.Location) (time.Time, bool) {
	local := after.In(location)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	for offset := 0; offset <= 7; offset++ {
		candidateDay := day.AddDate(0, 0, offset)
		if !pelicanWeekdaySelected(s.ActiveWeekdays, candidateDay.Weekday()) {
			continue
		}
		opening := time.Date(candidateDay.Year(), candidateDay.Month(), candidateDay.Day(), window.start/60, window.start%60, 0, 0, location)
		if opening.After(after) {
			return opening, true
		}
	}
	return time.Time{}, false
}

func (s PelicanSettings) NextRun(now time.Time) (time.Time, error) {
	window, location, err := s.activeWindow()
	if err != nil {
		return time.Time{}, err
	}
	if err := validatePelicanWeekdays(s.ActiveWeekdays); err != nil {
		return time.Time{}, err
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	timezone := normalizePelicanTimezone(s.ActiveTimezone)
	schedule, err := parser.Parse("CRON_TZ=" + timezone + " " + s.CronExpression)
	if err != nil {
		return time.Time{}, fmt.Errorf("\u9891\u7387\u5fc5\u987b\u662f\u6709\u6548\u7684\u4e94\u6bb5 Cron: %w", err)
	}
	limit := now.AddDate(5, 0, 0)
	for next := schedule.Next(now); !next.IsZero() && !next.After(limit); {
		if s.AutomaticRunAllowed(next) {
			return next.UTC(), nil
		}
		opening, ok := s.nextWindowOpening(next, window, location)
		if !ok {
			break
		}
		next = schedule.Next(opening.Add(-time.Nanosecond))
	}
	return time.Time{}, errors.New("\u5b9a\u65f6\u6d4b\u667a\u9891\u7387\u5728\u5141\u8bb8\u65f6\u6bb5\u5185\u6ca1\u6709\u53ef\u6267\u884c\u65f6\u95f4\uff0c\u8bf7\u8c03\u6574\u65f6\u6bb5\u6216\u9891\u7387")
}
