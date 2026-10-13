package relay

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

func TestBuildLatencyBucketsComputesPercentilesSuccessAndCounts(t *testing.T) {
	start := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	samples := []storage.RelayLatencySample{
		{FirstTokenMS: 100, DurationMS: 1000, StatusCode: 200, CreatedAt: start.Add(2 * time.Minute)},
		{FirstTokenMS: 200, DurationMS: 2000, StatusCode: 200, CreatedAt: start.Add(4 * time.Minute)},
		{FirstTokenMS: 300, DurationMS: 3000, StatusCode: 500, CreatedAt: start.Add(7 * time.Minute)},
		{FirstTokenMS: 400, DurationMS: 4000, StatusCode: 200, CreatedAt: start.Add(12 * time.Minute)},
	}
	buckets := buildLatencyBuckets(samples, start, start.Add(20*time.Minute), 10*time.Minute)
	if len(buckets) != 2 {
		t.Fatalf("len(buckets) = %d, want 2", len(buckets))
	}
	first := buckets[0]
	if first.RequestCount != 3 {
		t.Fatalf("first request count = %d, want 3", first.RequestCount)
	}
	if first.FirstTokenP50MS != 200 || first.FirstTokenP95MS != 290 {
		t.Fatalf("first TTFT percentiles = %d/%d, want 200/290", first.FirstTokenP50MS, first.FirstTokenP95MS)
	}
	if first.DurationP50MS != 2000 || first.DurationP95MS != 2900 {
		t.Fatalf("first duration percentiles = %d/%d, want 2000/2900", first.DurationP50MS, first.DurationP95MS)
	}
	if math.Abs(first.SuccessRate-(2.0/3.0)) > 1e-9 {
		t.Fatalf("first success rate = %f, want %f", first.SuccessRate, 2.0/3.0)
	}
}

func TestBuildLatencyBucketsComputesOutputTokenSpeedPercentilesFromTotalDuration(t *testing.T) {
	start := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	samples := []storage.RelayLatencySample{
		{FirstTokenMS: 9500, DurationMS: 10000, OutputTokens: 900, CreatedAt: start.Add(time.Minute)},
		{FirstTokenMS: 100, DurationMS: 7000, OutputTokens: 1200, CreatedAt: start.Add(2 * time.Minute)},
		{FirstTokenMS: 0, DurationMS: 10000, OutputTokens: 1000, CreatedAt: start.Add(3 * time.Minute)},
	}

	bucket := buildLatencyBuckets(samples, start, start.Add(10*time.Minute), 10*time.Minute)[0]
	if bucket.OutputTokenSpeedP50 == nil || math.Abs(*bucket.OutputTokenSpeedP50-100) > 1e-9 {
		t.Fatalf("output token speed P50 = %v, want 100 tokens/s", bucket.OutputTokenSpeedP50)
	}
	if bucket.OutputTokenSpeedP95 == nil || math.Abs(*bucket.OutputTokenSpeedP95-164.28571428571428) > 1e-9 {
		t.Fatalf("output token speed P95 = %v, want 164.2857 tokens/s", bucket.OutputTokenSpeedP95)
	}
}

func TestBuildLatencyBucketsKeepsEmptyBuckets(t *testing.T) {
	start := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	samples := []storage.RelayLatencySample{
		{FirstTokenMS: 100, DurationMS: 1000, StatusCode: 200, CreatedAt: start.Add(time.Minute)},
		{FirstTokenMS: 300, DurationMS: 3000, StatusCode: 200, CreatedAt: start.Add(21 * time.Minute)},
	}
	buckets := buildLatencyBuckets(samples, start, start.Add(30*time.Minute), 10*time.Minute)
	if len(buckets) != 3 {
		t.Fatalf("len(buckets) = %d, want 3", len(buckets))
	}
	if buckets[1].RequestCount != 0 {
		t.Fatalf("middle bucket request count = %d, want 0", buckets[1].RequestCount)
	}
}

func TestBuildLatencyBucketsByCountReturnsExactCountAndCoversRange(t *testing.T) {
	start := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	samples := []storage.RelayLatencySample{
		{FirstTokenMS: 100, DurationMS: 1000, StatusCode: 200, CreatedAt: start.Add(time.Minute)},
		{FirstTokenMS: 200, DurationMS: 2000, StatusCode: 200, CreatedAt: end.Add(-time.Minute)},
	}
	buckets := buildLatencyBucketsByCount(samples, start, end, 60)
	if len(buckets) != 60 {
		t.Fatalf("len(buckets) = %d, want 60", len(buckets))
	}
	if !buckets[0].Start.Equal(start) {
		t.Fatalf("first start = %s, want %s", buckets[0].Start, start)
	}
	if !buckets[len(buckets)-1].End.Equal(end) {
		t.Fatalf("last end = %s, want %s", buckets[len(buckets)-1].End, end)
	}
	if buckets[0].RequestCount != 1 || buckets[len(buckets)-1].RequestCount != 1 {
		t.Fatalf("edge bucket request counts = %d/%d, want 1/1", buckets[0].RequestCount, buckets[len(buckets)-1].RequestCount)
	}
}

func TestBuildLatencyBucketsForSpecUsesFixedDuration(t *testing.T) {
	start := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	buckets := buildLatencyBucketsForSpec(nil, start, end, LatencyBucketSpec{Size: 10 * time.Minute})
	if len(buckets) != 6 {
		t.Fatalf("len(buckets) = %d, want 6", len(buckets))
	}
	if got := buckets[0].End.Sub(buckets[0].Start); got != 10*time.Minute {
		t.Fatalf("first bucket size = %s, want 10m", got)
	}
}

func TestUsageListEndpointUsesSub2APIDateRangeAndAccount(t *testing.T) {
	since := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	endpoint := usageListEndpoint("https://relay.example/", 42, since, until, 3, 200)
	parsed, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if parsed.Path != "/api/v1/admin/usage" {
		t.Fatalf("path = %s", parsed.Path)
	}
	if query.Get("account_id") != "42" || query.Get("page") != "3" || query.Get("page_size") != "200" {
		t.Fatalf("pagination/account query = %s", endpoint)
	}
	if query.Get("start_date") != "2026-09-17" || query.Get("end_date") != "2026-09-17" || query.Get("timezone") != "UTC" {
		t.Fatalf("date range query = %s", endpoint)
	}
	if query.Get("start_time") != "" || query.Get("end_time") != "" {
		t.Fatalf("unsupported time range query = %s", endpoint)
	}
}

func TestStationUsageListEndpointOmitsAccountFilter(t *testing.T) {
	since := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	endpoint := stationUsageListEndpoint("https://relay.example/", since, until, 2, 200)
	parsed, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if query.Get("account_id") != "" {
		t.Fatalf("station usage endpoint unexpectedly contains account_id: %s", endpoint)
	}
	if query.Get("page") != "2" || query.Get("page_size") != "200" {
		t.Fatalf("pagination query = %s", endpoint)
	}
	if query.Get("start_date") != "2026-09-17" || query.Get("end_date") != "2026-09-17" || query.Get("timezone") != "UTC" {
		t.Fatalf("date range query = %s", endpoint)
	}
	if query.Get("start_time") != "" || query.Get("end_time") != "" {
		t.Fatalf("unsupported time range query = %s", endpoint)
	}
}

func TestGroupLatencySamplesByAccountFiltersAndAssignsPlatform(t *testing.T) {
	accounts := []storage.RelayAccount{
		{ExternalID: 11, Platform: "anthropic"},
		{ExternalID: 22, Platform: "openai"},
	}
	items := []remoteUsage{
		{AccountID: 11, FirstTokenMS: 100, DurationMS: 1000, CreatedAt: "2026-09-18T10:00:00Z"},
		{AccountID: 22, FirstTokenMS: 200, DurationMS: 2000, CreatedAt: "2026-09-18T10:01:00Z"},
		{AccountID: 33, FirstTokenMS: 300, DurationMS: 3000, CreatedAt: "2026-09-18T10:02:00Z"},
	}
	grouped := groupLatencySamplesByAccount(items, accounts)
	if len(grouped) != 2 {
		t.Fatalf("group count = %d, want 2", len(grouped))
	}
	if len(grouped[11]) != 1 || grouped[11][0].Platform != "anthropic" {
		t.Fatalf("account 11 samples = %#v", grouped[11])
	}
	if len(grouped[22]) != 1 || grouped[22][0].Platform != "openai" {
		t.Fatalf("account 22 samples = %#v", grouped[22])
	}
	if _, ok := grouped[33]; ok {
		t.Fatalf("unexpected unrequested account 33 in grouped samples")
	}
}

func TestBuildConsumptionBucketsUsesRequestTimestampsAndSameAutoWindows(t *testing.T) {
	start := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	samples := []consumptionSample{
		{CreatedAt: start.Add(5 * time.Minute), Amount: 2.5},
		{CreatedAt: start.Add(35 * time.Minute), Amount: 1.5},
	}
	buckets := buildConsumptionBuckets(samples, start, end, LatencyBucketSpec{Count: 6})
	if len(buckets) != 6 {
		t.Fatalf("len(buckets) = %d, want 6", len(buckets))
	}
	if buckets[0].Amount != 2.5 || buckets[3].Amount != 1.5 {
		t.Fatalf("amounts = %#v, want 2.5 in bucket 0 and 1.5 in bucket 3", buckets)
	}
	if !buckets[0].Start.Equal(start) || !buckets[len(buckets)-1].End.Equal(end) {
		t.Fatalf("bucket range does not cover requested interval")
	}
}

func TestBuildAccountConsumptionBucketsGroupsRequestsByAccountAndKeepsEmptyAccounts(t *testing.T) {
	start := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	accountStats := 0.75
	accountMultiplier := 2.0
	items := []remoteUsage{
		{AccountID: 11, CreatedAt: start.Add(5 * time.Minute).Format(time.RFC3339), ActualCost: 1.25, TotalCost: 0.5, AccountStatsCost: &accountStats},
		{AccountID: 11, CreatedAt: start.Add(35 * time.Minute).Format(time.RFC3339), ActualCost: 2.5, TotalCost: 1.5, AccountRateMultiplier: &accountMultiplier},
		{AccountID: 22, CreatedAt: start.Add(45 * time.Minute).Format(time.RFC3339), ActualCost: 3.0, TotalCost: 2.0},
		{AccountID: 99, CreatedAt: start.Add(10 * time.Minute).Format(time.RFC3339), ActualCost: 9.0, TotalCost: 9.0},
	}
	accounts := []storage.RelayAccount{{ExternalID: 11}, {ExternalID: 22}, {ExternalID: 33}}

	series := buildAccountConsumptionBuckets(items, accounts, start, end, LatencyBucketSpec{Count: 6})

	if len(series) != 3 {
		t.Fatalf("series count = %d, want 3", len(series))
	}
	if len(series[11]) != 6 || len(series[22]) != 6 || len(series[33]) != 6 {
		t.Fatalf("bucket lengths = %d/%d/%d, want 6/6/6", len(series[11]), len(series[22]), len(series[33]))
	}
	if series[11][0].UserCharge != 1.25 || series[11][0].AccountCost != 0.75 || series[11][0].RequestCount != 1 {
		t.Fatalf("account 11 bucket 0 = %#v", series[11][0])
	}
	if series[11][3].UserCharge != 2.5 || series[11][3].AccountCost != 3.0 || series[11][3].RequestCount != 1 {
		t.Fatalf("account 11 bucket 3 = %#v", series[11][3])
	}
	if series[22][4].UserCharge != 3.0 || series[22][4].AccountCost != 2.0 || series[22][4].RequestCount != 1 {
		t.Fatalf("account 22 bucket 4 = %#v", series[22][4])
	}
	for _, bucket := range series[33] {
		if bucket.UserCharge != 0 || bucket.AccountCost != 0 || bucket.RequestCount != 0 {
			t.Fatalf("empty account bucket = %#v", bucket)
		}
	}
	if _, ok := series[99]; ok {
		t.Fatalf("unexpected unrequested account 99 in consumption series")
	}
}

func TestUsageCostForBindingPrefersGatewayOpsMultiplier(t *testing.T) {
	accountStats := 9.9
	accountMultiplier := 3.3
	bindingMultiplier := 1.5
	usage := remoteUsage{TotalCost: 2, AccountStatsCost: &accountStats, AccountRateMultiplier: &accountMultiplier}
	got, complete := usageCostForBinding(usage, channelUsageCostBinding{Multiplier: &bindingMultiplier})
	if !complete || got != 3 {
		t.Fatalf("cost = %v complete=%t, want 3 true", got, complete)
	}
}

func TestUsageCostForBindingUsesSub2APIAccountCostSnapshot(t *testing.T) {
	accountStats := 4.25
	usage := remoteUsage{TotalCost: 2, AccountStatsCost: &accountStats}
	got, complete := usageCostForBinding(usage, channelUsageCostBinding{})
	if !complete || got != accountStats {
		t.Fatalf("cost = %v complete=%t, want %v true", got, complete, accountStats)
	}
}

func TestUsageCostForBindingFallsBackToAccountMultiplierThenBaseCost(t *testing.T) {
	accountMultiplier := 1.8
	got, complete := usageCostForBinding(remoteUsage{TotalCost: 2, AccountRateMultiplier: &accountMultiplier}, channelUsageCostBinding{})
	if !complete || got != 3.6 {
		t.Fatalf("multiplier cost = %v complete=%t, want 3.6 true", got, complete)
	}
	got, complete = usageCostForBinding(remoteUsage{TotalCost: 2}, channelUsageCostBinding{})
	if !complete || got != 2 {
		t.Fatalf("fallback cost = %v complete=%t, want 2 true", got, complete)
	}
}

func TestFetchStationUsageItemsDoesNotTrustApproximatePageCount(t *testing.T) {
	page1 := make([]remoteUsage, 200)
	for i := range page1 {
		page1[i] = remoteUsage{ID: int64(i + 1), CreatedAt: "2026-09-18T10:00:00Z"}
	}
	page2 := []remoteUsage{{ID: 201, CreatedAt: "2026-09-18T10:01:00Z"}}
	requestedPages := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		requestedPages = append(requestedPages, page)
		data := remoteUsagePage{Page: 1, Pages: 1, Total: 10, PageSize: 200, Items: page1}
		if page == "2" {
			data = remoteUsagePage{Page: 2, Pages: 1, Total: 10, PageSize: 200, Items: page2}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
	}))
	defer server.Close()

	service := &Service{client: server.Client()}
	start := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	items, err := service.fetchStationUsageItems(context.Background(), server.URL, "key", start, start.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 201 {
		t.Fatalf("len(items) = %d, want 201", len(items))
	}
	if len(requestedPages) < 2 || requestedPages[0] != "1" || requestedPages[1] != "2" {
		t.Fatalf("requested pages = %#v, want page 1 then page 2", requestedPages)
	}
}

func TestBuildDailyConsumptionAggregatesChargeCostAndRequests(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	since := time.Date(2026, 9, 18, 0, 0, 0, 0, loc)
	until := time.Date(2026, 9, 20, 12, 0, 0, 0, loc)
	accountStats := 1.25
	accountMultiplier := 1.5
	items := []remoteUsage{
		{CreatedAt: "2026-09-18T01:00:00+08:00", ActualCost: 2.0, TotalCost: 1.0, InputTokens: 100, OutputTokens: 20, CacheReadTokens: 10, CacheCreationTokens: 5, AccountStatsCost: &accountStats},
		{CreatedAt: "2026-09-18T23:30:00+08:00", ActualCost: 3.0, TotalCost: 2.0, InputTokens: 200, OutputTokens: 30, CacheReadTokens: 20, CacheCreationTokens: 10, AccountRateMultiplier: &accountMultiplier},
		{CreatedAt: "2026-09-20T08:00:00+08:00", ActualCost: 4.0, TotalCost: 2.0, InputTokens: 500, OutputTokens: 200, CacheReadTokens: 30, CacheCreationTokens: 20},
	}
	points := buildDailyConsumption(items, since, until, loc)
	if len(points) != 3 {
		t.Fatalf("len(points) = %d, want 3", len(points))
	}
	if points[0].Date != "2026-09-18" || points[0].UserCharge != 5 || points[0].AccountCost != 4.25 || points[0].RequestCount != 2 || points[0].TotalTokens != 395 {
		t.Fatalf("day 1 = %#v", points[0])
	}
	if points[1].UserCharge != 0 || points[1].AccountCost != 0 || points[1].RequestCount != 0 || points[1].TotalTokens != 0 {
		t.Fatalf("day 2 = %#v, want empty day", points[1])
	}
	if points[2].UserCharge != 4 || points[2].AccountCost != 2 || points[2].RequestCount != 1 || points[2].TotalTokens != 750 {
		t.Fatalf("day 3 = %#v", points[2])
	}
}

func TestDailyConsumptionWindowAtUsesSharedUsageRanges(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, 10, 3, 15, 30, 0, 0, loc)
	tests := []struct {
		name      string
		rangeName string
		wantStart time.Time
	}{
		{name: "today", rangeName: "today", wantStart: time.Date(2026, 10, 3, 0, 0, 0, 0, loc)},
		{name: "24h", rangeName: "24h", wantStart: now.Add(-24 * time.Hour)},
		{name: "7d", rangeName: "7d", wantStart: time.Date(2026, 9, 27, 0, 0, 0, 0, loc)},
		{name: "30d", rangeName: "30d", wantStart: time.Date(2026, 9, 4, 0, 0, 0, 0, loc)},
		{name: "all falls back to the supported daily window", rangeName: "all", wantStart: time.Date(2026, 9, 4, 0, 0, 0, 0, loc)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, until := dailyConsumptionWindowAt(now, tt.rangeName)
			if !start.Equal(tt.wantStart) {
				t.Fatalf("start = %s, want %s", start, tt.wantStart)
			}
			if !until.Equal(now) {
				t.Fatalf("until = %s, want %s", until, now)
			}
		})
	}
}

func TestUsageEventRoundTripPreservesMonitoringFields(t *testing.T) {
	rate := 1.7
	stats := 3.25
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	usage := remoteUsage{
		ID: 77, AccountID: 9, GroupID: 4, GroupName: "g", UserEmail: "u@example.com",
		FirstTokenMS: 123, DurationMS: 456, StatusCode: 200, CreatedAt: "2026-09-20T11:30:00Z",
		Model: "m", RequestType: "chat", InputTokens: 10, OutputTokens: 20,
		CacheReadTokens: 3, CacheCreationTokens: 4, CacheCreation5mTokens: 5, CacheCreation1hTokens: 6,
		ActualCost: 1.2, TotalCost: 2.3, AccountRateMultiplier: &rate, AccountStatsCost: &stats,
	}
	event, ok := usageEventFromRemote(5, usage, now)
	if !ok {
		t.Fatal("usage event rejected")
	}
	got := remoteUsageFromEvent(event)
	if got.ID != usage.ID || got.AccountID != usage.AccountID || got.FirstTokenMS != usage.FirstTokenMS ||
		got.DurationMS != usage.DurationMS || got.ActualCost != usage.ActualCost || got.TotalCost != usage.TotalCost ||
		got.AccountStatsCost == nil || *got.AccountStatsCost != stats {
		t.Fatalf("round trip = %#v", got)
	}
}

func TestFetchRecentUsageItemsForAccountsOnlyUsesFirstPageForActiveAPIKeys(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/usage" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("page") != "1" {
			t.Errorf("page = %q, want 1", r.URL.Query().Get("page"))
		}
		if r.URL.Query().Get("page_size") != "60" {
			t.Errorf("page_size = %q, want 60", r.URL.Query().Get("page_size"))
		}
		accountID := r.URL.Query().Get("account_id")
		mu.Lock()
		seen[accountID]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":` + accountID + `,"account_id":` + accountID + `,"created_at":"2026-09-20T12:00:00Z"}]}}`))
	}))
	defer server.Close()

	service := &Service{client: server.Client()}
	accounts := []remoteAccount{
		{ID: 11, Type: "apikey", Status: "active", Schedulable: true},
		{ID: 12, Type: "apikey", Status: "active", Schedulable: true},
		{ID: 13, Type: "apikey", Status: "disabled", Schedulable: true},
		{ID: 14, Type: "oauth", Status: "active", Schedulable: true},
		{ID: 15, Type: "apikey", Status: "active", Schedulable: false},
	}
	items := service.fetchRecentUsageItemsForAccounts(context.Background(), server.URL, "key", accounts)
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	mu.Lock()
	defer mu.Unlock()
	if seen["11"] != 1 || seen["12"] != 1 || len(seen) != 2 {
		t.Fatalf("seen = %#v, want only active schedulable API-key accounts 11 and 12", seen)
	}
}

func TestPercentileInt64CalculatesP95(t *testing.T) {
	values := make([]int64, 20)
	for i := range values {
		values[i] = int64(i + 1)
	}

	if got := percentileInt64(values, 0.95); got != 19 {
		t.Fatalf("P95 = %d, want 19", got)
	}
}

func TestPercentileInt64MatchesChannelInterpolation(t *testing.T) {
	values := []int64{100, 200}

	if got := percentileInt64(values, 0.50); got != 150 {
		t.Fatalf("P50 = %d, want 150", got)
	}
	if got := percentileInt64(values, 0.95); got != 195 {
		t.Fatalf("P95 = %d, want 195", got)
	}
}
