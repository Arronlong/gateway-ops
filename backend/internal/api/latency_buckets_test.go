package api

import (
	"testing"
	"time"
)

func TestParseLatencyBucketSpecDefaultsToAutoTargetCount(t *testing.T) {
	spec, err := parseLatencyBucketSpec("", "", "", 60)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Count != 60 || spec.Size != 0 {
		t.Fatalf("spec = %#v, want auto count 60", spec)
	}
}

func TestParseLatencyBucketSpecAcceptsFixedMinutes(t *testing.T) {
	spec, err := parseLatencyBucketSpec("fixed", "", "10", 60)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Count != 0 || spec.Size != 10*time.Minute {
		t.Fatalf("spec = %#v, want fixed 10m", spec)
	}
}

func TestParseLatencyBucketSpecRejectsUnsupportedFixedMinutes(t *testing.T) {
	if _, err := parseLatencyBucketSpec("fixed", "", "7", 60); err == nil {
		t.Fatal("expected unsupported 7 minute bucket to fail")
	}
}

func TestParseLatencyBucketSpecAllowsPageSpecificAutoCount(t *testing.T) {
	spec, err := parseLatencyBucketSpec("auto", "30", "", 60)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Count != 30 {
		t.Fatalf("count = %d, want 30", spec.Count)
	}
}

func TestLatencyRangeWindowAtUsesStableMinuteBoundary(t *testing.T) {
	now := time.Date(2026, 9, 19, 15, 42, 37, 456000000, time.FixedZone("UTC+8", 8*60*60))
	since, until, rangeName := latencyRangeWindowAt(now, "24h")
	wantUntil := time.Date(2026, 9, 19, 7, 42, 0, 0, time.UTC)
	if !until.Equal(wantUntil) {
		t.Fatalf("until = %s, want %s", until, wantUntil)
	}
	if !since.Equal(wantUntil.Add(-24 * time.Hour)) {
		t.Fatalf("since = %s, want %s", since, wantUntil.Add(-24*time.Hour))
	}
	if rangeName != "24h" {
		t.Fatalf("range = %q, want 24h", rangeName)
	}
}
