package storage

import (
	"testing"
	"time"
)

func TestPelicanAutomaticRunWindow(t *testing.T) {
	for _, tc := range []struct {
		name, start, end, at string
		want                 bool
	}{
		{"before_open", "08:00", "22:00", "2026-10-22T07:59:59+08:00", false},
		{"opening_included", "08:00", "22:00", "2026-10-22T08:00:00+08:00", true},
		{"before_close", "08:00", "22:00", "2026-10-22T21:59:59+08:00", true},
		{"closing_excluded", "08:00", "22:00", "2026-10-22T22:00:00+08:00", false},
		{"beijing_independent_of_input_zone", "08:00", "22:00", "2026-10-22T01:00:00Z", true},
		{"overnight_evening", "22:00", "06:00", "2026-10-22T23:00:00+08:00", true},
		{"overnight_morning", "22:00", "06:00", "2026-10-23T05:59:59+08:00", true},
		{"overnight_closed", "22:00", "06:00", "2026-10-23T06:00:00+08:00", false},
		{"all_day", "00:00", "00:00", "2026-10-22T23:59:59+08:00", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at, err := time.Parse(time.RFC3339, tc.at)
			if err != nil {
				t.Fatal(err)
			}
			settings := PelicanSettings{ActiveStart: tc.start, ActiveEnd: tc.end}
			if got := settings.AutomaticRunAllowed(at); got != tc.want {
				t.Fatalf("allowed=%t want=%t", got, tc.want)
			}
		})
	}
}

func TestPelicanAutomaticRunRespectsSelectedWeekdays(t *testing.T) {
	for _, tc := range []struct {
		name, at string
		want     bool
	}{
		{"weekday_enabled", "2026-10-23T12:00:00+08:00", true},
		{"saturday_disabled", "2026-10-24T12:00:00+08:00", false},
		{"sunday_disabled", "2026-10-25T12:00:00+08:00", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at, err := time.Parse(time.RFC3339, tc.at)
			if err != nil {
				t.Fatal(err)
			}
			settings := PelicanSettings{ActiveStart: "08:00", ActiveEnd: "22:00", ActiveWeekdays: []int{1, 2, 3, 4, 5}}
			if got := settings.AutomaticRunAllowed(at); got != tc.want {
				t.Fatalf("allowed=%t want=%t", got, tc.want)
			}
		})
	}

	overnight := PelicanSettings{ActiveStart: "22:00", ActiveEnd: "06:00", ActiveWeekdays: []int{5}}
	for _, tc := range []struct {
		at   string
		want bool
	}{
		{"2026-10-23T23:00:00+08:00", true},
		{"2026-10-24T05:59:00+08:00", true},
		{"2026-10-24T22:00:00+08:00", false},
	} {
		at, err := time.Parse(time.RFC3339, tc.at)
		if err != nil {
			t.Fatal(err)
		}
		if got := overnight.AutomaticRunAllowed(at); got != tc.want {
			t.Errorf("at %s allowed=%t want=%t", tc.at, got, tc.want)
		}
	}
}

func TestPelicanNextRunRespectsWindowAndFrequency(t *testing.T) {
	for _, tc := range []struct {
		name, cron, start, end, at, want string
		weekdays                         []int
	}{
		{"before_open", "*/15 * * * *", "08:00", "22:00", "2026-10-22T07:50:00+08:00", "2026-10-22T08:00:00+08:00", nil},
		{"during_window", "*/15 * * * *", "08:00", "22:00", "2026-10-22T08:00:00+08:00", "2026-10-22T08:15:00+08:00", nil},
		{"closing_skipped", "*/15 * * * *", "08:00", "22:00", "2026-10-22T21:59:00+08:00", "2026-10-23T08:00:00+08:00", nil},
		{"late_night", "*/15 * * * *", "08:00", "22:00", "2026-10-22T23:00:00+08:00", "2026-10-23T08:00:00+08:00", nil},
		{"frequency_stays_aligned", "0 */6 * * *", "08:00", "22:00", "2026-10-22T07:00:00+08:00", "2026-10-22T12:00:00+08:00", nil},
		{"partial_hour_window", "*/15 * * * *", "08:10", "08:20", "2026-10-22T07:50:00+08:00", "2026-10-22T08:15:00+08:00", nil},
		{"overnight_opening", "0 * * * *", "22:00", "06:00", "2026-10-22T21:59:00+08:00", "2026-10-22T22:00:00+08:00", nil},
		{"overnight_closing", "0 * * * *", "22:00", "06:00", "2026-10-23T05:59:00+08:00", "2026-10-23T22:00:00+08:00", nil},
		{"all_day", "0 * * * *", "00:00", "00:00", "2026-10-22T23:59:00+08:00", "2026-10-23T00:00:00+08:00", nil},
		{"custom_weekday", "0 9 * * 1", "08:00", "22:00", "2026-10-23T10:00:00+08:00", "2026-10-26T09:00:00+08:00", nil},
		{"skip_weekend", "0 9 * * *", "08:00", "22:00", "2026-10-23T10:00:00+08:00", "2026-10-26T09:00:00+08:00", []int{1, 2, 3, 4, 5}},
		{"overnight_weekday_belongs_to_opening_day", "0 * * * *", "22:00", "06:00", "2026-10-24T06:01:00+08:00", "2026-10-30T22:00:00+08:00", []int{5}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at, err := time.Parse(time.RFC3339, tc.at)
			if err != nil {
				t.Fatal(err)
			}
			want, err := time.Parse(time.RFC3339, tc.want)
			if err != nil {
				t.Fatal(err)
			}
			settings := PelicanSettings{CronExpression: tc.cron, ActiveStart: tc.start, ActiveEnd: tc.end, ActiveWeekdays: tc.weekdays}
			got, err := settings.NextRun(at)
			if err != nil || !got.Equal(want) {
				t.Fatalf("next=%v err=%v want=%v", got, err, want)
			}
		})
	}
}

func TestPelicanScheduleRejectsInvalidWindow(t *testing.T) {
	for _, tc := range []struct{ start, end, cron string }{
		{"24:00", "22:00", "*/15 * * * *"},
		{"8:00", "22:00", "*/15 * * * *"},
		{"08:60", "22:00", "*/15 * * * *"},
		{"", "22:00", "*/15 * * * *"},
		{"08:00", "", "*/15 * * * *"},
		{"10:00", "22:00", "0 9 * * *"},
		{"08:01", "08:10", "*/15 * * * *"},
	} {
		t.Run(tc.start+"_"+tc.end+"_"+tc.cron, func(t *testing.T) {
			settings := defaultUISettings().PelicanSettings()
			settings.ActiveStart, settings.ActiveEnd, settings.CronExpression = tc.start, tc.end, tc.cron
			if err := ValidatePelicanSettings(settings); err == nil {
				t.Fatal("invalid window/frequency accepted")
			}
		})
	}
}

func TestPelicanSettingsRejectInvalidWeekdays(t *testing.T) {
	for _, weekdays := range [][]int{{}, {7}, {-1}, {1, 1}} {
		settings := defaultUISettings().PelicanSettings()
		settings.ActiveWeekdays = weekdays
		if err := ValidatePelicanSettings(settings); err == nil {
			t.Errorf("accepted invalid weekdays %v", weekdays)
		}
	}
}

func TestPelicanScheduleUsesConfiguredTimezone(t *testing.T) {
	settings := PelicanSettings{
		ActiveTimezone: "America/New_York", ActiveStart: "08:00", ActiveEnd: "22:00",
		ActiveWeekdays: []int{5}, CronExpression: "0 * * * *",
	}
	beforeOpen := time.Date(2026, 10, 23, 11, 59, 0, 0, time.UTC)
	opening := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	if settings.AutomaticRunAllowed(beforeOpen) || !settings.AutomaticRunAllowed(opening) {
		t.Fatal("active window did not use the configured timezone")
	}
	next, err := settings.NextRun(time.Date(2026, 10, 23, 11, 30, 0, 0, time.UTC))
	if err != nil || !next.Equal(opening) {
		t.Fatalf("next=%v err=%v want=%v", next, err, opening)
	}
}

func TestPelicanScheduleRejectsUnknownTimezone(t *testing.T) {
	settings := defaultUISettings().PelicanSettings()
	settings.ActiveTimezone = "Mars/Olympus_Mons"
	if err := ValidatePelicanSettings(settings); err == nil {
		t.Fatal("accepted unknown timezone")
	}
}
