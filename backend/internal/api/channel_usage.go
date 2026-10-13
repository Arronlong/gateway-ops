package api

import (
	"math"
	"sort"
	"time"

	"github.com/guliansheng/gateway-ops/internal/relay"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

type channelLatencyBucket struct {
	Start               time.Time `json:"start"`
	End                 time.Time `json:"end"`
	FirstTokenP50MS     *int64    `json:"first_token_p50_ms"`
	FirstTokenP95MS     *int64    `json:"first_token_p95_ms"`
	DurationP50MS       *int64    `json:"duration_p50_ms"`
	DurationP95MS       *int64    `json:"duration_p95_ms"`
	OutputTokenSpeedP50 *float64  `json:"output_token_speed_p50"`
	OutputTokenSpeedP95 *float64  `json:"output_token_speed_p95"`
	SuccessRate         *float64  `json:"success_rate"`
	RequestCount        int       `json:"request_count"`
}

type channelConsumptionBucket struct {
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
	Amount float64   `json:"amount"`
}

type channelBucketWindow struct {
	start time.Time
	end   time.Time
}

func buildChannelWindows(since, until time.Time, spec relay.LatencyBucketSpec) []channelBucketWindow {
	if !until.After(since) {
		return nil
	}
	if spec.Count > 0 {
		windows := make([]channelBucketWindow, spec.Count)
		total := until.Sub(since)
		for i := 0; i < spec.Count; i++ {
			start := since.Add(time.Duration(i) * total / time.Duration(spec.Count))
			end := since.Add(time.Duration(i+1) * total / time.Duration(spec.Count))
			if i == spec.Count-1 {
				end = until
			}
			windows[i] = channelBucketWindow{start: start, end: end}
		}
		return windows
	}
	if spec.Size <= 0 {
		return nil
	}
	count := int(math.Ceil(float64(until.Sub(since)) / float64(spec.Size)))
	windows := make([]channelBucketWindow, count)
	for i := range windows {
		start := since.Add(time.Duration(i) * spec.Size)
		end := start.Add(spec.Size)
		if end.After(until) {
			end = until
		}
		windows[i] = channelBucketWindow{start: start, end: end}
	}
	return windows
}

func buildChannelLatencyBuckets(events []storage.ChannelUsageEvent, since, until time.Time, spec relay.LatencyBucketSpec) []channelLatencyBucket {
	windows := buildChannelWindows(since, until, spec)
	type values struct {
		first        []int64
		duration     []int64
		speeds       []float64
		knownSuccess int
		knownCount   int
		requests     int
	}
	grouped := make([]values, len(windows))
	for _, event := range events {
		index := channelBucketIndex(windows, event.OccurredAt)
		if index < 0 {
			continue
		}
		grouped[index].requests++
		if event.FirstTokenMS != nil {
			grouped[index].first = append(grouped[index].first, *event.FirstTokenMS)
		}
		if event.DurationMS != nil {
			grouped[index].duration = append(grouped[index].duration, *event.DurationMS)
		}
		if event.DurationMS != nil {
			if speed, ok := channelOutputTokenSpeed(event.OutputTokens, *event.DurationMS); ok {
				grouped[index].speeds = append(grouped[index].speeds, speed)
			}
		}
		if event.SuccessKnown {
			grouped[index].knownCount++
			if event.Success {
				grouped[index].knownSuccess++
			}
		}
	}
	out := make([]channelLatencyBucket, len(windows))
	for i, window := range windows {
		bucket := channelLatencyBucket{Start: window.start, End: window.end, RequestCount: grouped[i].requests}
		bucket.FirstTokenP50MS = percentilePtr(grouped[i].first, 0.50)
		bucket.FirstTokenP95MS = percentilePtr(grouped[i].first, 0.95)
		bucket.DurationP50MS = percentilePtr(grouped[i].duration, 0.50)
		bucket.DurationP95MS = percentilePtr(grouped[i].duration, 0.95)
		bucket.OutputTokenSpeedP50 = percentileFloatPtr(grouped[i].speeds, 0.50)
		bucket.OutputTokenSpeedP95 = percentileFloatPtr(grouped[i].speeds, 0.95)
		if grouped[i].knownCount > 0 {
			value := float64(grouped[i].knownSuccess) / float64(grouped[i].knownCount)
			bucket.SuccessRate = &value
		}
		out[i] = bucket
	}
	return out
}

func channelOutputTokenSpeed(outputTokens, durationMS int64) (float64, bool) {
	if outputTokens <= 0 || durationMS <= 0 {
		return 0, false
	}
	return float64(outputTokens) * 1000 / float64(durationMS), true
}

func percentileFloatPtr(values []float64, percentile float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	ordered := append([]float64(nil), values...)
	sort.Float64s(ordered)
	if len(ordered) == 1 {
		value := ordered[0]
		return &value
	}
	position := percentile * float64(len(ordered)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	weight := position - float64(lower)
	value := ordered[lower]*(1-weight) + ordered[upper]*weight
	return &value
}

func buildChannelConsumptionBuckets(events []storage.ChannelUsageEvent, since, until time.Time, spec relay.LatencyBucketSpec) []channelConsumptionBucket {
	windows := buildChannelWindows(since, until, spec)
	out := make([]channelConsumptionBucket, len(windows))
	for i, window := range windows {
		out[i] = channelConsumptionBucket{Start: window.start, End: window.end}
	}
	for _, event := range events {
		if event.ActualCost <= 0 {
			continue
		}
		index := channelBucketIndex(windows, event.OccurredAt)
		if index >= 0 {
			out[index].Amount += event.ActualCost
		}
	}
	return out
}

func channelBucketIndex(windows []channelBucketWindow, at time.Time) int {
	if len(windows) == 0 || at.Before(windows[0].start) || !at.Before(windows[len(windows)-1].end) {
		return -1
	}
	index := sort.Search(len(windows), func(i int) bool { return at.Before(windows[i].end) })
	if index < 0 || index >= len(windows) || at.Before(windows[index].start) {
		return -1
	}
	return index
}

func percentilePtr(values []int64, percentile float64) *int64 {
	if len(values) == 0 {
		return nil
	}
	ordered := append([]int64(nil), values...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	if len(ordered) == 1 {
		value := ordered[0]
		return &value
	}
	position := percentile * float64(len(ordered)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		value := ordered[lower]
		return &value
	}
	weight := position - float64(lower)
	value := int64(math.Round(float64(ordered[lower])*(1-weight) + float64(ordered[upper])*weight))
	return &value
}
