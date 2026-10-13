package api

import (
	"math"
	"testing"
	"time"

	"github.com/guliansheng/gateway-ops/internal/relay"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

func TestPercentilePtrCalculatesP95(t *testing.T) {
	values := make([]int64, 20)
	for i := range values {
		values[i] = int64(i + 1)
	}

	got := percentilePtr(values, 0.95)
	if got == nil || *got != 19 {
		t.Fatalf("P95 = %v, want 19", got)
	}
}

func TestBuildChannelLatencyBucketsComputesOutputTokenSpeedPercentilesFromTotalDuration(t *testing.T) {
	start := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	first := int64(1000)
	firstLate := int64(9500)
	durationA := int64(10000)
	durationB := int64(7000)
	durationC := int64(10000)
	events := []storage.ChannelUsageEvent{
		{OccurredAt: start.Add(time.Minute), FirstTokenMS: &firstLate, DurationMS: &durationA, OutputTokens: 900},
		{OccurredAt: start.Add(2 * time.Minute), FirstTokenMS: &first, DurationMS: &durationB, OutputTokens: 1200},
		{OccurredAt: start.Add(3 * time.Minute), DurationMS: &durationC, OutputTokens: 1000},
	}

	bucket := buildChannelLatencyBuckets(events, start, start.Add(10*time.Minute), relay.LatencyBucketSpec{Size: 10 * time.Minute})[0]
	if bucket.OutputTokenSpeedP50 == nil || math.Abs(*bucket.OutputTokenSpeedP50-100) > 1e-9 {
		t.Fatalf("output token speed P50 = %v, want 100 tokens/s", bucket.OutputTokenSpeedP50)
	}
	if bucket.OutputTokenSpeedP95 == nil || math.Abs(*bucket.OutputTokenSpeedP95-164.28571428571428) > 1e-9 {
		t.Fatalf("output token speed P95 = %v, want 164.2857 tokens/s", bucket.OutputTokenSpeedP95)
	}
}
