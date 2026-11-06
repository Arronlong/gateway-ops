package relay

import (
	"encoding/json"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"math"
	"testing"
	"time"
)

func TestPublicGroupMonitorBucketsAndCallsKeepOutputSpeed(t *testing.T) {
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	usage := []remoteUsage{
		{ID: 1, CreatedAt: now.Add(-time.Minute).Format(time.RFC3339), OutputTokens: 100, DurationMS: 10000},
		{ID: 2, CreatedAt: now.Add(-2 * time.Minute).Format(time.RFC3339), OutputTokens: 600, DurationMS: 20000},
		{ID: 3, CreatedAt: now.Add(-3 * time.Minute).Format(time.RFC3339), OutputTokens: 0, DurationMS: 20000},
	}
	view := buildPublicGroupMonitorGroupForWindow(storage.RelayGroup{ExternalID: 1, Status: "active", MonitorEnabled: true}, usageMonitorEvents(usage), nil, true, now, now.Add(-time.Hour), now)
	raw, _ := json.Marshal(view)
	var decoded struct {
		Calls []struct {
			OutputTokens *int64 `json:"output_tokens"`
		}
		Buckets []struct {
			P50 *float64 `json:"output_token_speed_p50"`
			P95 *float64 `json:"output_token_speed_p95"`
		} `json:"latency_buckets"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Calls) != 3 || decoded.Calls[0].OutputTokens == nil || *decoded.Calls[0].OutputTokens != 100 {
		t.Fatalf("recent call lost output token count: %s", raw)
	}
	buckets := buildPublicGroupMonitorLatencyBuckets(usageMonitorEvents(usage), now.Add(-time.Hour), now, 1)
	raw, _ = json.Marshal(buckets)
	if err := json.Unmarshal(raw, &decoded.Buckets); err != nil {
		t.Fatal(err)
	}
	if decoded.Buckets[0].P50 == nil || decoded.Buckets[0].P95 == nil || math.Abs(*decoded.Buckets[0].P50-20) > 1e-9 || math.Abs(*decoded.Buckets[0].P95-29) > 1e-9 {
		t.Fatalf("token speed percentiles missing or wrong: %s", raw)
	}
	empty := buildPublicGroupMonitorLatencyBuckets(usageMonitorEvents(usage[2:]), now.Add(-time.Hour), now, 1)
	raw, _ = json.Marshal(empty)
	if err := json.Unmarshal(raw, &decoded.Buckets); err != nil {
		t.Fatal(err)
	}
	if decoded.Buckets[0].P50 != nil || decoded.Buckets[0].P95 != nil {
		t.Fatalf("unknown output must stay unknown: %s", raw)
	}
}
