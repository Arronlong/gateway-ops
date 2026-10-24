package relay

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

type LatencyBucket struct {
	Start               time.Time `json:"start"`
	End                 time.Time `json:"end"`
	FirstTokenP50MS     int64     `json:"first_token_p50_ms"`
	FirstTokenP95MS     int64     `json:"first_token_p95_ms"`
	DurationP50MS       int64     `json:"duration_p50_ms"`
	DurationP95MS       int64     `json:"duration_p95_ms"`
	OutputTokenSpeedP50 *float64  `json:"output_token_speed_p50"`
	OutputTokenSpeedP95 *float64  `json:"output_token_speed_p95"`
	SuccessRate         float64   `json:"success_rate"`
	RequestCount        int       `json:"request_count"`
}

type LatencyBucketSpec struct {
	Count int
	Size  time.Duration
}

type ConsumptionBucket struct {
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
	Amount float64   `json:"amount"`
}

type DailyConsumptionPoint struct {
	Date         string  `json:"date"`
	UserCharge   float64 `json:"user_charge"`
	AccountCost  float64 `json:"account_cost"`
	RequestCount int     `json:"request_count"`
	TotalTokens  int64   `json:"total_tokens"`
}

type AccountConsumptionBucket struct {
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	UserCharge   float64   `json:"user_charge"`
	AccountCost  float64   `json:"account_cost"`
	RequestCount int       `json:"request_count"`
}

type latencyBucketWindow struct {
	start time.Time
	end   time.Time
}

func buildLatencyBucketsForSpec(samples []storage.RelayLatencySample, since, until time.Time, spec LatencyBucketSpec) []LatencyBucket {
	if spec.Count > 0 {
		return buildLatencyBucketsByCount(samples, since, until, spec.Count)
	}
	return buildLatencyBuckets(samples, since, until, spec.Size)
}

func buildLatencyBucketsByCount(samples []storage.RelayLatencySample, since, until time.Time, count int) []LatencyBucket {
	if count <= 0 || !until.After(since) {
		return []LatencyBucket{}
	}
	total := until.Sub(since)
	windows := make([]latencyBucketWindow, count)
	for i := 0; i < count; i++ {
		start := since.Add(time.Duration(i) * total / time.Duration(count))
		end := since.Add(time.Duration(i+1) * total / time.Duration(count))
		if i == count-1 {
			end = until
		}
		windows[i] = latencyBucketWindow{start: start, end: end}
	}
	return aggregateLatencyBuckets(samples, windows)
}

func buildLatencyBuckets(samples []storage.RelayLatencySample, since, until time.Time, bucketSize time.Duration) []LatencyBucket {
	if bucketSize <= 0 || !until.After(since) {
		return []LatencyBucket{}
	}
	count := int(math.Ceil(float64(until.Sub(since)) / float64(bucketSize)))
	if count <= 0 {
		return []LatencyBucket{}
	}
	windows := make([]latencyBucketWindow, count)
	for i := range windows {
		start := since.Add(time.Duration(i) * bucketSize)
		end := start.Add(bucketSize)
		if end.After(until) {
			end = until
		}
		windows[i] = latencyBucketWindow{start: start, end: end}
	}
	return aggregateLatencyBuckets(samples, windows)
}

func aggregateLatencyBuckets(samples []storage.RelayLatencySample, windows []latencyBucketWindow) []LatencyBucket {
	type bucketValues struct {
		first    []int64
		duration []int64
		speeds   []float64
		success  int
	}
	values := make([]bucketValues, len(windows))
	if len(windows) == 0 {
		return []LatencyBucket{}
	}
	for _, sample := range samples {
		if sample.CreatedAt.Before(windows[0].start) || !sample.CreatedAt.Before(windows[len(windows)-1].end) {
			continue
		}
		index := sort.Search(len(windows), func(i int) bool {
			return sample.CreatedAt.Before(windows[i].end)
		})
		if index < 0 || index >= len(windows) || sample.CreatedAt.Before(windows[index].start) {
			continue
		}
		values[index].first = append(values[index].first, sample.FirstTokenMS)
		values[index].duration = append(values[index].duration, sample.DurationMS)
		if speed, ok := outputTokenSpeed(sample.OutputTokens, sample.DurationMS); ok {
			values[index].speeds = append(values[index].speeds, speed)
		}
		if sample.StatusCode == 0 || (sample.StatusCode >= 200 && sample.StatusCode < 400) {
			values[index].success++
		}
	}
	buckets := make([]LatencyBucket, len(windows))
	for i := range windows {
		requestCount := len(values[i].duration)
		bucket := LatencyBucket{Start: windows[i].start, End: windows[i].end, RequestCount: requestCount}
		if requestCount > 0 {
			bucket.FirstTokenP50MS = percentileInt64(values[i].first, 0.50)
			bucket.FirstTokenP95MS = percentileInt64(values[i].first, 0.95)
			bucket.DurationP50MS = percentileInt64(values[i].duration, 0.50)
			bucket.DurationP95MS = percentileInt64(values[i].duration, 0.95)
			bucket.OutputTokenSpeedP50 = percentileFloat64Ptr(values[i].speeds, 0.50)
			bucket.OutputTokenSpeedP95 = percentileFloat64Ptr(values[i].speeds, 0.95)
			bucket.SuccessRate = float64(values[i].success) / float64(requestCount)
		}
		buckets[i] = bucket
	}
	return buckets
}

func outputTokenSpeed(outputTokens, durationMS int64) (float64, bool) {
	if outputTokens <= 0 || durationMS <= 0 {
		return 0, false
	}
	return float64(outputTokens) * 1000 / float64(durationMS), true
}

func percentileFloat64Ptr(values []float64, percentile float64) *float64 {
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

type consumptionSample struct {
	CreatedAt time.Time
	Amount    float64
}

func buildConsumptionBuckets(samples []consumptionSample, since, until time.Time, spec LatencyBucketSpec) []ConsumptionBucket {
	latencyBuckets := buildLatencyBucketsForSpec(nil, since, until, spec)
	buckets := make([]ConsumptionBucket, len(latencyBuckets))
	for i, bucket := range latencyBuckets {
		buckets[i] = ConsumptionBucket{Start: bucket.Start, End: bucket.End}
	}
	for _, sample := range samples {
		if sample.Amount <= 0 || sample.CreatedAt.Before(since) || !sample.CreatedAt.Before(until) {
			continue
		}
		index := sort.Search(len(buckets), func(i int) bool {
			return sample.CreatedAt.Before(buckets[i].End)
		})
		if index < 0 || index >= len(buckets) || sample.CreatedAt.Before(buckets[index].Start) {
			continue
		}
		buckets[index].Amount += sample.Amount
	}
	return buckets
}

func buildAccountConsumptionBuckets(items []remoteUsage, accounts []storage.RelayAccount, since, until time.Time, spec LatencyBucketSpec) map[int64][]AccountConsumptionBucket {
	windows := buildLatencyBucketsForSpec(nil, since, until, spec)
	accountIDs := make(map[int64]struct{}, len(accounts))
	out := make(map[int64][]AccountConsumptionBucket, len(accounts))
	for _, account := range accounts {
		accountIDs[account.ExternalID] = struct{}{}
		buckets := make([]AccountConsumptionBucket, len(windows))
		for i, window := range windows {
			buckets[i] = AccountConsumptionBucket{Start: window.Start, End: window.End}
		}
		out[account.ExternalID] = buckets
	}
	if len(windows) == 0 {
		return out
	}
	for _, usage := range items {
		if _, ok := accountIDs[usage.AccountID]; !ok {
			continue
		}
		createdAt, err := time.Parse(time.RFC3339Nano, usage.CreatedAt)
		if err != nil || createdAt.Before(since) || !createdAt.Before(until) {
			continue
		}
		buckets := out[usage.AccountID]
		index := sort.Search(len(buckets), func(i int) bool {
			return createdAt.Before(buckets[i].End)
		})
		if index < 0 || index >= len(buckets) || createdAt.Before(buckets[index].Start) {
			continue
		}
		buckets[index].UserCharge += usage.ActualCost
		buckets[index].AccountCost += remoteUsageAccountCost(usage)
		buckets[index].RequestCount++
		out[usage.AccountID] = buckets
	}
	return out
}

func usageCostForBinding(usage remoteUsage, binding channelUsageCostBinding) (float64, bool) {
	if binding.Multiplier != nil && *binding.Multiplier >= 0 {
		return usage.TotalCost * *binding.Multiplier, true
	}
	if usage.AccountStatsCost != nil && *usage.AccountStatsCost >= 0 {
		return *usage.AccountStatsCost, true
	}
	if usage.AccountRateMultiplier != nil && *usage.AccountRateMultiplier >= 0 {
		return usage.TotalCost * *usage.AccountRateMultiplier, true
	}
	// Sub2API treats historical rows without account_rate_multiplier as 1x.
	return usage.TotalCost, true
}

func remoteUsageAccountCost(usage remoteUsage) float64 {
	if usage.AccountStatsCost != nil && *usage.AccountStatsCost >= 0 {
		return *usage.AccountStatsCost
	}
	if usage.AccountRateMultiplier != nil && *usage.AccountRateMultiplier >= 0 {
		return usage.TotalCost * *usage.AccountRateMultiplier
	}
	return usage.TotalCost
}

func buildDailyConsumption(items []remoteUsage, since, until time.Time, loc *time.Location) []DailyConsumptionPoint {
	if loc == nil {
		loc = time.Local
	}
	if !until.After(since) {
		return []DailyConsumptionPoint{}
	}
	startLocal := since.In(loc)
	startDay := time.Date(startLocal.Year(), startLocal.Month(), startLocal.Day(), 0, 0, 0, 0, loc)
	endLocal := until.In(loc)
	endDay := time.Date(endLocal.Year(), endLocal.Month(), endLocal.Day(), 0, 0, 0, 0, loc)
	count := int(endDay.Sub(startDay)/(24*time.Hour)) + 1
	if count <= 0 {
		return []DailyConsumptionPoint{}
	}
	points := make([]DailyConsumptionPoint, count)
	indexByDate := make(map[string]int, count)
	for i := 0; i < count; i++ {
		day := startDay.AddDate(0, 0, i)
		key := day.Format("2006-01-02")
		points[i].Date = key
		indexByDate[key] = i
	}
	for _, usage := range items {
		created, err := time.Parse(time.RFC3339Nano, usage.CreatedAt)
		if err != nil || created.Before(since) || !created.Before(until) {
			continue
		}
		key := created.In(loc).Format("2006-01-02")
		index, ok := indexByDate[key]
		if !ok {
			continue
		}
		points[index].UserCharge += usage.ActualCost
		points[index].AccountCost += remoteUsageAccountCost(usage)
		points[index].RequestCount++
		points[index].TotalTokens += usage.InputTokens + usage.OutputTokens + usage.CacheReadTokens + usage.CacheCreationTokens
	}
	return points
}

func dailyConsumptionWindowAt(now time.Time, rangeName string) (time.Time, time.Time) {
	loc := now.Location()
	localNow := now.In(loc)
	dayStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
	switch rangeName {
	case "24h":
		return now.Add(-24 * time.Hour), now
	case "7d":
		return dayStart.AddDate(0, 0, -6), now
	case "30d", "all":
		return dayStart.AddDate(0, 0, -29), now
	default:
		return dayStart, now
	}
}

func (s *Service) StationDailyConsumptionRange(_ context.Context, stationID uint, rangeName string, now time.Time) ([]DailyConsumptionPoint, error) {
	start, until := dailyConsumptionWindowAt(now, rangeName)
	items, err := s.localUsageItems(stationID, start.UTC(), until.UTC())
	if err != nil {
		return nil, err
	}
	return buildDailyConsumption(items, start.UTC(), until.UTC(), start.Location()), nil
}

func (s *Service) StationDailyConsumption(ctx context.Context, stationID uint, days int, now time.Time) ([]DailyConsumptionPoint, error) {
	if days == 7 {
		return s.StationDailyConsumptionRange(ctx, stationID, "7d", now)
	}
	return s.StationDailyConsumptionRange(ctx, stationID, "30d", now)
}

func percentileInt64(values []int64, percentile float64) int64 {
	if len(values) == 0 {
		return 0
	}
	ordered := append([]int64(nil), values...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	if len(ordered) == 1 {
		return ordered[0]
	}
	position := percentile * float64(len(ordered)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return ordered[lower]
	}
	weight := position - float64(lower)
	return int64(math.Round(float64(ordered[lower])*(1-weight) + float64(ordered[upper])*weight))
}

func setUsageDateRange(query url.Values, since, until time.Time) {
	start := since.UTC()
	end := until.UTC()
	if !end.After(start) {
		end = start.Add(time.Nanosecond)
	}
	// Sub2API's admin usage endpoint accepts calendar dates, not RFC3339
	// start_time/end_time values. end_date is inclusive on the remote side.
	query.Set("start_date", start.Format("2006-01-02"))
	query.Set("end_date", end.Add(-time.Nanosecond).Format("2006-01-02"))
	query.Set("timezone", "UTC")
}

func usageListEndpoint(baseURL string, accountID int64, since, until time.Time, page, pageSize int) string {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 200
	}
	query := url.Values{}
	query.Set("page", strconv.Itoa(page))
	query.Set("page_size", strconv.Itoa(pageSize))
	query.Set("account_id", strconv.FormatInt(accountID, 10))
	setUsageDateRange(query, since, until)
	return strings.TrimRight(baseURL, "/") + "/api/v1/admin/usage?" + query.Encode()
}

func stationUsageListEndpoint(baseURL string, since, until time.Time, page, pageSize int) string {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 200
	}
	query := url.Values{}
	query.Set("page", strconv.Itoa(page))
	query.Set("page_size", strconv.Itoa(pageSize))
	setUsageDateRange(query, since, until)
	return strings.TrimRight(baseURL, "/") + "/api/v1/admin/usage?" + query.Encode()
}

func latencySampleFromUsage(usage remoteUsage) (storage.RelayLatencySample, bool) {
	created, err := time.Parse(time.RFC3339Nano, usage.CreatedAt)
	if err != nil {
		return storage.RelayLatencySample{}, false
	}
	return storage.RelayLatencySample{
		CacheKnown:            usage.CacheKnown || usage.CacheReadTokens > 0,
		FirstTokenMS:          usage.FirstTokenMS,
		DurationMS:            usage.DurationMS,
		StatusCode:            usage.StatusCode,
		CreatedAt:             created.UTC(),
		Model:                 usage.Model,
		RequestType:           usage.RequestType,
		UserEmail:             usage.UserEmail,
		InputTokens:           usage.InputTokens,
		OutputTokens:          usage.OutputTokens,
		CacheReadTokens:       usage.CacheReadTokens,
		CacheCreationTokens:   usage.CacheCreationTokens,
		CacheCreation5mTokens: usage.CacheCreation5mTokens,
		CacheCreation1hTokens: usage.CacheCreation1hTokens,
		GroupID:               usage.GroupID,
		GroupName:             usage.GroupName,
	}, true
}

func groupLatencySamplesByAccount(items []remoteUsage, accounts []storage.RelayAccount) map[int64][]storage.RelayLatencySample {
	platformByAccount := make(map[int64]string, len(accounts))
	out := make(map[int64][]storage.RelayLatencySample, len(accounts))
	for _, account := range accounts {
		platformByAccount[account.ExternalID] = account.Platform
		out[account.ExternalID] = []storage.RelayLatencySample{}
	}
	for _, usage := range items {
		platform, wanted := platformByAccount[usage.AccountID]
		if !wanted {
			continue
		}
		sample, ok := latencySampleFromUsage(usage)
		if !ok {
			continue
		}
		sample.Platform = platform
		out[usage.AccountID] = append(out[usage.AccountID], sample)
	}
	return out
}

const (
	usageInitialBackfill  = 30 * 24 * time.Hour
	usageInitialHotWindow = 2 * time.Hour
	usageRetention        = 35 * 24 * time.Hour
)

func usageEventFromRemote(stationID uint, usage remoteUsage, syncedAt time.Time) (storage.RelayUsageEvent, bool) {
	occurredAt, err := time.Parse(time.RFC3339Nano, usage.CreatedAt)
	if err != nil || usage.ID <= 0 {
		return storage.RelayUsageEvent{}, false
	}
	userID := ""
	if usage.UserID > 0 {
		userID = strconv.FormatInt(usage.UserID, 10)
	}
	return storage.RelayUsageEvent{
		RelayStationID:        stationID,
		RemoteUsageID:         usage.ID,
		AccountExternalID:     usage.AccountID,
		GroupExternalID:       usage.GroupID,
		GroupName:             usage.GroupName,
		UserEmail:             usage.UserEmail,
		UserExternalID:        userID,
		IPAddress:             strings.TrimSpace(usage.IPAddress),
		Model:                 usage.Model,
		RequestType:           usage.RequestType,
		FirstTokenMS:          usage.FirstTokenMS,
		DurationMS:            usage.DurationMS,
		StatusCode:            usage.StatusCode,
		CacheKnown:            usage.CacheKnown,
		InputTokens:           usage.InputTokens,
		OutputTokens:          usage.OutputTokens,
		CacheReadTokens:       usage.CacheReadTokens,
		CacheCreationTokens:   usage.CacheCreationTokens,
		CacheCreation5mTokens: usage.CacheCreation5mTokens,
		CacheCreation1hTokens: usage.CacheCreation1hTokens,
		ActualCost:            usage.ActualCost,
		TotalCost:             usage.TotalCost,
		AccountRateMultiplier: usage.AccountRateMultiplier,
		AccountStatsCost:      usage.AccountStatsCost,
		OccurredAt:            occurredAt.UTC(),
		SyncedAt:              syncedAt.UTC(),
	}, true
}

func remoteUsageFromEvent(event storage.RelayUsageEvent) remoteUsage {
	userID, _ := strconv.ParseInt(event.UserExternalID, 10, 64)
	return remoteUsage{
		ID:                    event.RemoteUsageID,
		AccountID:             event.AccountExternalID,
		UserEmail:             event.UserEmail,
		UserID:                userID,
		IPAddress:             event.IPAddress,
		GroupID:               event.GroupExternalID,
		GroupName:             event.GroupName,
		FirstTokenMS:          event.FirstTokenMS,
		DurationMS:            event.DurationMS,
		CacheKnown:            event.CacheKnown || event.CacheReadTokens > 0,
		StatusCode:            event.StatusCode,
		CreatedAt:             event.OccurredAt.UTC().Format(time.RFC3339Nano),
		Model:                 event.Model,
		RequestType:           event.RequestType,
		InputTokens:           event.InputTokens,
		OutputTokens:          event.OutputTokens,
		CacheReadTokens:       event.CacheReadTokens,
		CacheCreationTokens:   event.CacheCreationTokens,
		CacheCreation5mTokens: event.CacheCreation5mTokens,
		CacheCreation1hTokens: event.CacheCreation1hTokens,
		ActualCost:            event.ActualCost,
		TotalCost:             event.TotalCost,
		AccountRateMultiplier: event.AccountRateMultiplier,
		AccountStatsCost:      event.AccountStatsCost,
	}
}

func (s *Service) syncUsageEvents(ctx context.Context, station storage.RelayStation, apiKey string, accounts []remoteAccount, now time.Time) error {
	// Keep the foreground path equivalent to the old "recent 60" behavior:
	// fetch only page 1 for each active API-key account, in parallel.
	items := s.fetchRecentUsageItemsForAccounts(ctx, station.BaseURL, apiKey, accounts)
	if err := s.persistUsageItems(station.ID, items, now); err != nil {
		return err
	}
	if err := s.stations.DeleteOldUsageEvents(station.ID, now.UTC().Add(-usageRetention)); err != nil {
		return err
	}
	s.startUsageCatchup(station, apiKey, now)
	return nil
}

func (s *Service) fetchRecentUsageItemsForAccounts(ctx context.Context, baseURL, apiKey string, accounts []remoteAccount) []remoteUsage {
	type job struct{ accountID int64 }
	jobs := make(chan job)
	results := make(chan []remoteUsage)
	workers := 8
	if len(accounts) < workers {
		workers = len(accounts)
	}
	if workers == 0 {
		return []remoteUsage{}
	}

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				query := url.Values{}
				query.Set("page", "1")
				query.Set("page_size", strconv.Itoa(latencySampleLimit))
				query.Set("account_id", strconv.FormatInt(item.accountID, 10))
				endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/admin/usage?" + query.Encode()
				var page remoteUsagePage
				if err := s.get(ctx, endpoint, apiKey, &page); err != nil {
					continue
				}
				select {
				case results <- page.Items:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(results)
		for _, account := range accounts {
			if !strings.EqualFold(strings.TrimSpace(account.Type), "apikey") ||
				!strings.EqualFold(strings.TrimSpace(account.Status), "active") ||
				!account.Schedulable || account.ID <= 0 {
				continue
			}
			select {
			case jobs <- job{accountID: account.ID}:
			case <-ctx.Done():
				close(jobs)
				wg.Wait()
				return
			}
		}
		close(jobs)
		wg.Wait()
	}()

	items := make([]remoteUsage, 0, len(accounts)*latencySampleLimit)
	for pageItems := range results {
		items = append(items, pageItems...)
	}
	return items
}

func (s *Service) persistUsageItems(stationID uint, items []remoteUsage, syncedAt time.Time) error {
	events := make([]storage.RelayUsageEvent, 0, len(items))
	for _, item := range items {
		event, ok := usageEventFromRemote(stationID, item, syncedAt)
		if !ok {
			continue
		}
		events = append(events, event)
	}
	return s.stations.UpsertUsageEvents(events)
}

func (s *Service) startUsageCatchup(station storage.RelayStation, apiKey string, now time.Time) {
	key := strconv.FormatUint(uint64(station.ID), 10)
	go func() {
		_, _, _ = s.usageBackfillGroup.Do(key, func() (any, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()
			return nil, s.catchupUsageEvents(ctx, station, apiKey, now)
		})
	}()
}

func (s *Service) catchupUsageEvents(ctx context.Context, station storage.RelayStation, apiKey string, now time.Time) error {
	state, err := s.stations.UsageSyncState(station.ID)
	if err != nil {
		return err
	}
	now = now.UTC()
	target := now.Add(-usageInitialBackfill)

	// Forward synchronization closes gaps in small checkpoints. The first run
	// covers the latest two hours; later runs resume from the last complete
	// cursor with a short overlap for boundary safety.
	forwardStart := now.Add(-usageInitialHotWindow)
	if state.ForwardSyncedAt != nil && !state.ForwardSyncedAt.IsZero() {
		forwardStart = state.ForwardSyncedAt.UTC().Add(-time.Minute)
		if forwardStart.Before(target) {
			forwardStart = target
		}
	}
	// Sub2API filters usage by calendar date, so sub-hour chunks would refetch
	// the same day repeatedly. Keep checkpoints bounded to one day instead.
	const forwardChunk = 24 * time.Hour
	cursor := forwardStart
	for cursor.Before(now) {
		end := cursor.Add(forwardChunk)
		if end.After(now) {
			end = now
		}
		if err := s.syncUsageRange(ctx, station, apiKey, cursor, end); err != nil {
			return err
		}
		completed := end
		state.ForwardSyncedAt = &completed
		if err := s.stations.SaveUsageSyncState(state); err != nil {
			return err
		}
		if end.Equal(now) {
			break
		}
		cursor = end.Add(-time.Minute)
	}

	if state.BackfilledAt == nil || state.BackfilledAt.IsZero() {
		backfilled := forwardStart
		state.BackfilledAt = &backfilled
		if err := s.stations.SaveUsageSyncState(state); err != nil {
			return err
		}
	}

	// Historical backfill proceeds in bounded chunks and checkpoints after
	// every completed chunk, so interruption never requires starting over.
	boundary := forwardStart
	if state.BackfilledAt != nil && !state.BackfilledAt.IsZero() {
		boundary = state.BackfilledAt.UTC()
	}
	const backfillChunk = 24 * time.Hour
	for boundary.After(target) {
		start := boundary.Add(-backfillChunk)
		if start.Before(target) {
			start = target
		}
		if err := s.syncUsageRange(ctx, station, apiKey, start, boundary); err != nil {
			return err
		}
		boundary = start
		checkpoint := boundary
		state.BackfilledAt = &checkpoint
		if err := s.stations.SaveUsageSyncState(state); err != nil {
			return err
		}
	}
	return s.stations.DeleteOldUsageEvents(station.ID, now.Add(-usageRetention))
}

func (s *Service) syncUsageRange(ctx context.Context, station storage.RelayStation, apiKey string, since, until time.Time) error {
	const pageSize = 200
	for pageNumber := 1; pageNumber <= 1000; pageNumber++ {
		var page remoteUsagePage
		if err := s.get(ctx, stationUsageListEndpoint(station.BaseURL, since, until, pageNumber, pageSize), apiKey, &page); err != nil {
			return err
		}
		if err := s.persistUsageItems(station.ID, page.Items, time.Now().UTC()); err != nil {
			return err
		}
		if len(page.Items) < pageSize {
			return nil
		}
	}
	return errors.New("Sub2API usage 分页超过 1000 页，未推进同步游标")
}

func (s *Service) localUsageItems(stationID uint, since, until time.Time) ([]remoteUsage, error) {
	events, err := s.stations.ListUsageEvents(stationID, since, until)
	if err != nil {
		return nil, err
	}
	items := make([]remoteUsage, 0, len(events))
	for _, event := range events {
		items = append(items, remoteUsageFromEvent(event))
	}
	return items, nil
}

func (s *Service) localAccountLatencySnapshots(stationID uint, accounts []remoteAccount, groups []remoteGroup) map[int64]string {
	events, err := s.stations.RecentUsageEventsByAccount(stationID, latencySampleLimit)
	if err != nil {
		return map[int64]string{}
	}
	accountByID := make(map[int64]remoteAccount, len(accounts))
	for _, account := range accounts {
		accountByID[account.ID] = account
	}
	groupByID := make(map[int64]remoteGroup, len(groups))
	for _, group := range groups {
		groupByID[group.ID] = group
	}
	samplesByAccount := make(map[int64][]storage.RelayLatencySample, len(accounts))
	for _, event := range events {
		account, ok := accountByID[event.AccountExternalID]
		if !ok {
			continue
		}
		sample, ok := latencySampleFromUsage(remoteUsageFromEvent(event))
		if !ok {
			continue
		}
		sample.Platform = account.Platform
		if group, ok := groupByID[event.GroupExternalID]; ok {
			value := group.RateMultiplier
			sample.GroupMultiplier = &value
		}
		samplesByAccount[event.AccountExternalID] = append(samplesByAccount[event.AccountExternalID], sample)
	}
	out := make(map[int64]string, len(samplesByAccount))
	for accountID, samples := range samplesByAccount {
		raw, err := json.Marshal(samples)
		if err != nil {
			continue
		}
		out[accountID] = string(raw)
	}
	return out
}

func (s *Service) fetchStationUsageItems(ctx context.Context, baseURL, apiKey string, since, until time.Time) ([]remoteUsage, error) {
	if credential, ok := decodeNewAPICredential(apiKey); ok {
		return s.newAPIStationUsageItems(ctx, baseURL, credential, since, until)
	}
	const pageSize = 200
	var first remoteUsagePage
	if err := s.get(ctx, stationUsageListEndpoint(baseURL, since, until, 1, pageSize), apiKey, &first); err != nil {
		return nil, err
	}
	items := append([]remoteUsage(nil), first.Items...)
	pageCount := first.Pages
	if pageCount <= 0 && first.Total > len(first.Items) {
		pageCount = int(math.Ceil(float64(first.Total) / float64(pageSize)))
	}
	if pageCount <= 1 {
		if len(first.Items) < pageSize {
			return items, nil
		}
		for pageNumber := 2; pageNumber <= 1000; pageNumber++ {
			var page remoteUsagePage
			if err := s.get(ctx, stationUsageListEndpoint(baseURL, since, until, pageNumber, pageSize), apiKey, &page); err != nil {
				return nil, err
			}
			items = append(items, page.Items...)
			if len(page.Items) < pageSize {
				break
			}
		}
		return items, nil
	}

	type pageResult struct {
		number int
		items  []remoteUsage
		err    error
	}
	jobs := make(chan int)
	results := make(chan pageResult)
	workers := 6
	if pageCount-1 < workers {
		workers = pageCount - 1
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for pageNumber := range jobs {
				var page remoteUsagePage
				err := s.get(ctx, stationUsageListEndpoint(baseURL, since, until, pageNumber, pageSize), apiKey, &page)
				select {
				case results <- pageResult{number: pageNumber, items: page.Items, err: err}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(results)
		for pageNumber := 2; pageNumber <= pageCount; pageNumber++ {
			select {
			case jobs <- pageNumber:
			case <-ctx.Done():
				close(jobs)
				wg.Wait()
				return
			}
		}
		close(jobs)
		wg.Wait()
	}()

	pages := make(map[int][]remoteUsage, pageCount-1)
	var firstErr error
	for result := range results {
		if result.err != nil {
			if firstErr == nil {
				firstErr = result.err
			}
			continue
		}
		pages[result.number] = result.items
	}
	if firstErr != nil {
		return nil, firstErr
	}
	lastPageItems := first.Items
	for pageNumber := 2; pageNumber <= pageCount; pageNumber++ {
		pageItems := pages[pageNumber]
		items = append(items, pageItems...)
		lastPageItems = pageItems
	}
	// Sub2API may return an approximate total/pages for the unfiltered admin
	// usage list. A full last page is therefore only a hint that more rows may
	// exist; keep probing until the first short page.
	if len(lastPageItems) == pageSize {
		for pageNumber := pageCount + 1; pageNumber <= 1000; pageNumber++ {
			var page remoteUsagePage
			if err := s.get(ctx, stationUsageListEndpoint(baseURL, since, until, pageNumber, pageSize), apiKey, &page); err != nil {
				return nil, err
			}
			items = append(items, page.Items...)
			if len(page.Items) < pageSize {
				break
			}
		}
	}
	return items, nil
}

func (s *Service) fetchLatencySamplesForAccount(ctx context.Context, baseURL, apiKey string, accountID int64, since, until time.Time, limit int) ([]storage.RelayLatencySample, error) {
	const pageSize = 200
	samples := make([]storage.RelayLatencySample, 0, pageSize)
	for pageNumber := 1; pageNumber <= 1000; pageNumber++ {
		var page remoteUsagePage
		if err := s.get(ctx, usageListEndpoint(baseURL, accountID, since, until, pageNumber, pageSize), apiKey, &page); err != nil {
			return nil, err
		}
		for _, usage := range page.Items {
			sample, ok := latencySampleFromUsage(usage)
			if !ok {
				continue
			}
			samples = append(samples, sample)
			if limit > 0 && len(samples) >= limit {
				return samples[:limit], nil
			}
		}
		if len(page.Items) == 0 || (page.Pages > 0 && pageNumber >= page.Pages) || (page.Total > 0 && len(samples) >= page.Total) {
			break
		}
		if len(page.Items) < pageSize && page.Pages == 0 && page.Total == 0 {
			break
		}
	}
	return samples, nil
}

func (s *Service) latencySamplesForAccounts(_ context.Context, station storage.RelayStation, accounts []storage.RelayAccount, since, until time.Time) (map[int64][]storage.RelayLatencySample, error) {
	if len(accounts) == 0 {
		return map[int64][]storage.RelayLatencySample{}, nil
	}
	items, err := s.localUsageItems(station.ID, since, until)
	if err != nil {
		return nil, err
	}
	return groupLatencySamplesByAccount(items, accounts), nil
}

func (s *Service) StationLatencyBuckets(ctx context.Context, stationID uint, since, until time.Time, spec LatencyBucketSpec) (map[int64][]LatencyBucket, error) {
	station, err := s.stations.FindByID(stationID)
	if err != nil {
		return nil, err
	}
	accounts, err := s.stations.ListAccounts(stationID)
	if err != nil {
		return nil, err
	}
	samplesByAccount, err := s.latencySamplesForAccounts(ctx, *station, accounts, since, until)
	if err != nil {
		return nil, err
	}
	out := make(map[int64][]LatencyBucket, len(accounts))
	for _, account := range accounts {
		out[account.ExternalID] = buildLatencyBucketsForSpec(samplesByAccount[account.ExternalID], since, until, spec)
	}
	return out, nil
}

func (s *Service) StationAccountConsumptionBuckets(_ context.Context, stationID uint, since, until time.Time, spec LatencyBucketSpec) (map[int64][]AccountConsumptionBucket, error) {
	accounts, err := s.stations.ListAccounts(stationID)
	if err != nil {
		return nil, err
	}
	items, err := s.localUsageItems(stationID, since, until)
	if err != nil {
		return nil, err
	}
	return buildAccountConsumptionBuckets(items, accounts, since, until, spec), nil
}

func (s *Service) AccountLatencySamples(_ context.Context, stationID uint, accountID int64, since, until time.Time, limit int) ([]storage.RelayLatencySample, error) {
	account, err := s.stations.FindAccountByExternalID(stationID, accountID)
	if err != nil {
		return nil, err
	}
	items, err := s.localUsageItems(stationID, since, until)
	if err != nil {
		return nil, err
	}
	grouped := groupLatencySamplesByAccount(items, []storage.RelayAccount{*account})
	samples := grouped[accountID]
	sort.SliceStable(samples, func(i, j int) bool { return samples[i].CreatedAt.After(samples[j].CreatedAt) })
	if limit > 0 && len(samples) > limit {
		samples = samples[:limit]
	}
	return samples, nil
}

func (s *Service) RecentAccountLatencySamples(stationID uint, limit int) (map[int64][]storage.RelayLatencySample, error) {
	accounts, err := s.stations.ListAccounts(stationID)
	if err != nil {
		return nil, err
	}
	events, err := s.stations.RecentUsageEventsByAccount(stationID, limit)
	if err != nil {
		return nil, err
	}
	accountByID := make(map[int64]storage.RelayAccount, len(accounts))
	out := make(map[int64][]storage.RelayLatencySample, len(accounts))
	for _, account := range accounts {
		accountByID[account.ExternalID] = account
		out[account.ExternalID] = []storage.RelayLatencySample{}
	}
	for _, event := range events {
		account, ok := accountByID[event.AccountExternalID]
		if !ok {
			continue
		}
		sample, ok := latencySampleFromUsage(remoteUsageFromEvent(event))
		if !ok {
			continue
		}
		sample.Platform = account.Platform
		out[event.AccountExternalID] = append(out[event.AccountExternalID], sample)
	}
	return out, nil
}

func (s *Service) ChannelLatencyBucketsByRange(ctx context.Context, channels []storage.Channel, since, until time.Time, spec LatencyBucketSpec) (map[uint][]LatencyBucket, error) {
	samplesByChannel := make(map[uint][]storage.RelayLatencySample, len(channels))
	for _, channel := range channels {
		samplesByChannel[channel.ID] = []storage.RelayLatencySample{}
	}
	ratesByChannel := make(map[uint][]storage.RateSnapshot, len(channels))
	if s.rates != nil {
		for _, channel := range channels {
			rates, err := s.rates.ListByChannel(channel.ID)
			if err != nil {
				return nil, err
			}
			ratesByChannel[channel.ID] = rates
		}
	}
	stations, err := s.stations.List()
	if err != nil {
		return nil, err
	}
	for _, station := range stations {
		accounts, err := s.stations.ListAccounts(station.ID)
		if err != nil {
			return nil, err
		}
		overrides, err := s.stations.ListCostOverrides(station.ID)
		if err != nil {
			return nil, err
		}
		bindings := resolveChannelUsageCostBindings(channels, accounts, overrides, ratesByChannel)
		if len(bindings) == 0 {
			continue
		}
		boundAccounts := make([]storage.RelayAccount, 0, len(bindings))
		for _, account := range accounts {
			if _, ok := bindings[account.ExternalID]; ok {
				boundAccounts = append(boundAccounts, account)
			}
		}
		samplesByAccount, err := s.latencySamplesForAccounts(ctx, station, boundAccounts, since, until)
		if err != nil {
			return nil, err
		}
		for accountID, binding := range bindings {
			samples := samplesByAccount[accountID]
			for i := range samples {
				if binding.Multiplier != nil {
					value := *binding.Multiplier
					samples[i].ChannelGroupMultiplier = &value
				}
			}
			samplesByChannel[binding.ChannelID] = append(samplesByChannel[binding.ChannelID], samples...)
		}
	}
	out := make(map[uint][]LatencyBucket, len(channels))
	for _, channel := range channels {
		out[channel.ID] = buildLatencyBucketsForSpec(samplesByChannel[channel.ID], since, until, spec)
	}
	return out, nil
}

func (s *Service) ChannelLatencySamplesByRange(ctx context.Context, channels []storage.Channel, channelID uint, since, until time.Time, limit int) ([]storage.RelayLatencySample, error) {
	all := make([]storage.RelayLatencySample, 0)
	ratesByChannel := make(map[uint][]storage.RateSnapshot, len(channels))
	if s.rates != nil {
		for _, channel := range channels {
			rates, err := s.rates.ListByChannel(channel.ID)
			if err != nil {
				return nil, err
			}
			ratesByChannel[channel.ID] = rates
		}
	}
	stations, err := s.stations.List()
	if err != nil {
		return nil, err
	}
	for _, station := range stations {
		accounts, err := s.stations.ListAccounts(station.ID)
		if err != nil {
			return nil, err
		}
		overrides, err := s.stations.ListCostOverrides(station.ID)
		if err != nil {
			return nil, err
		}
		bindings := resolveChannelUsageCostBindings(channels, accounts, overrides, ratesByChannel)
		items, err := s.localUsageItems(station.ID, since, until)
		if err != nil {
			return nil, err
		}
		samplesByAccount := groupLatencySamplesByAccount(items, accounts)
		for _, account := range accounts {
			binding, ok := bindings[account.ExternalID]
			if !ok || binding.ChannelID != channelID {
				continue
			}
			samples := samplesByAccount[account.ExternalID]
			for i := range samples {
				if binding.Multiplier != nil {
					value := *binding.Multiplier
					samples[i].ChannelGroupMultiplier = &value
				}
			}
			all = append(all, samples...)
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].CreatedAt.After(all[j].CreatedAt) })
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

func (s *Service) ChannelConsumptionBuckets(ctx context.Context, channels []storage.Channel, since, until time.Time, spec LatencyBucketSpec) (map[uint][]ConsumptionBucket, error) {
	samplesByChannel := make(map[uint][]consumptionSample, len(channels))
	for _, channel := range channels {
		samplesByChannel[channel.ID] = []consumptionSample{}
	}
	ratesByChannel := make(map[uint][]storage.RateSnapshot, len(channels))
	if s.rates != nil {
		for _, channel := range channels {
			rates, err := s.rates.ListByChannel(channel.ID)
			if err != nil {
				return nil, err
			}
			ratesByChannel[channel.ID] = rates
		}
	}
	stations, err := s.stations.List()
	if err != nil {
		return nil, err
	}
	for _, station := range stations {
		accounts, err := s.stations.ListAccounts(station.ID)
		if err != nil {
			return nil, err
		}
		overrides, err := s.stations.ListCostOverrides(station.ID)
		if err != nil {
			return nil, err
		}
		bindings := resolveChannelUsageCostBindings(channels, accounts, overrides, ratesByChannel)
		if len(bindings) == 0 {
			continue
		}
		items, err := s.localUsageItems(station.ID, since, until)
		if err != nil {
			return nil, err
		}
		for _, usage := range items {
			binding, ok := bindings[usage.AccountID]
			if !ok {
				continue
			}
			createdAt, err := time.Parse(time.RFC3339Nano, usage.CreatedAt)
			if err != nil {
				continue
			}
			amount, _ := usageCostForBinding(usage, binding)
			if amount <= 0 {
				continue
			}
			samplesByChannel[binding.ChannelID] = append(samplesByChannel[binding.ChannelID], consumptionSample{
				CreatedAt: createdAt.UTC(),
				Amount:    amount,
			})
		}
	}
	out := make(map[uint][]ConsumptionBucket, len(channels))
	for _, channel := range channels {
		out[channel.ID] = buildConsumptionBuckets(samplesByChannel[channel.ID], since, until, spec)
	}
	return out, nil
}
