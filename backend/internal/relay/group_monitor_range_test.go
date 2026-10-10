package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

func TestPublicGroupMonitorWindowAt(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, 10, 9, 16, 30, 0, 0, loc)
	tests := []struct {
		raw, wantRange string
		wantStart      time.Time
	}{
		{"", "24h", now.Add(-24 * time.Hour)},
		{"24h", "24h", now.Add(-24 * time.Hour)},
		{"today", "today", time.Date(2026, 10, 9, 0, 0, 0, 0, loc)},
		{"7d", "7d", time.Date(2026, 10, 3, 0, 0, 0, 0, loc)},
		{"30d", "30d", time.Date(2026, 9, 10, 0, 0, 0, 0, loc)},
		{" 7D ", "7d", time.Date(2026, 10, 3, 0, 0, 0, 0, loc)},
		{"invalid", "24h", now.Add(-24 * time.Hour)},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			rangeName, since, until := publicGroupMonitorWindowAt(now, tt.raw)
			if rangeName != tt.wantRange || !since.Equal(tt.wantStart) || !until.Equal(now) {
				t.Fatalf("window = %s [%s, %s), want %s [%s, %s)", rangeName, since, until, tt.wantRange, tt.wantStart, now)
			}
		})
	}
}

func TestBuildPublicGroupMonitorGroupForWindowFiltersAllMetrics(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	since := now.Add(-24 * time.Hour)
	group := storage.RelayGroup{ExternalID: 28, Name: "GPT", Status: "active", MonitorEnabled: true}
	first, duration := int64(100), int64(1000)
	usage := []monitorEvent{
		{createdAt: since, success: true, firstTokenMS: &first, durationMS: &duration},
		{createdAt: now.Add(-time.Hour), success: true, firstTokenMS: &first, durationMS: &duration},
		{createdAt: since.Add(-time.Second), success: true},
		{createdAt: now, success: true},
	}
	failures := []monitorEvent{
		{createdAt: since.Add(-time.Second), failureReason: monitorFailureUpstream},
		{createdAt: now.Add(time.Hour), failureReason: monitorFailureUpstream},
	}
	got := buildPublicGroupMonitorGroupForWindow(group, usage, failures, true, now, since, now)
	if len(got.Calls) != 2 || got.SuccessCount != 2 || got.FailureCount != 0 || got.Status != "available" || got.FailureSummary != "" {
		t.Fatalf("range-filtered group = %#v", got)
	}
	if got.LatestCallAt == nil || !got.LatestCallAt.Equal(now.Add(-time.Hour)) {
		t.Fatalf("latest call = %v", got.LatestCallAt)
	}
	if len(got.LatencyBuckets) != groupMonitorLimit || !got.LatencyBuckets[0].Start.Equal(since) || !got.LatencyBuckets[len(got.LatencyBuckets)-1].End.Equal(now) {
		t.Fatalf("bucket window = %#v", got.LatencyBuckets)
	}
	requests := 0
	for _, bucket := range got.LatencyBuckets {
		requests += bucket.RequestCount
	}
	if requests != 2 {
		t.Fatalf("bucket requests = %d, want 2", requests)
	}
}

func TestBuildPublicGroupMonitorGroupForWindowFiltersBeforeCallLimit(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	since := now.Add(-7 * 24 * time.Hour)
	usage := make([]monitorEvent, 0, 70)
	for index := 0; index < 70; index++ {
		usage = append(usage, monitorEvent{success: true, createdAt: now.Add(time.Duration(index-65) * time.Minute)})
	}
	group := storage.RelayGroup{Status: "active"}
	got := buildPublicGroupMonitorGroupForWindow(group, usage, nil, true, now, since, now)
	if len(got.Calls) != groupMonitorLimit {
		t.Fatalf("calls = %d, want 60", len(got.Calls))
	}
	for _, call := range got.Calls {
		if call.CreatedAt.Before(since) || !call.CreatedAt.Before(now) {
			t.Fatalf("out-of-window call: %#v", call)
		}
	}
	if !got.Calls[0].CreatedAt.Equal(now.Add(-time.Minute)) {
		t.Fatalf("latest call = %#v", got.Calls[0])
	}
}

func TestBuildPublicGroupMonitorGroupForWindowKeepsFullHistoryForStability(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	since := now.Add(-24 * time.Hour)
	first, duration := int64(100), int64(1000)
	usage := make([]monitorEvent, 0, 120)
	for index := 0; index < 120; index++ {
		createdAt := since.Add(time.Duration(index) * 12 * time.Minute)
		usage = append(usage, monitorEvent{success: true, createdAt: createdAt, firstTokenMS: &first, durationMS: &duration})
	}
	group := storage.RelayGroup{ExternalID: 28, Name: "GPT", Status: "active", MonitorEnabled: true}
	got := buildPublicGroupMonitorGroupForWindow(group, usage, nil, true, now, since, now)
	if len(got.Calls) != groupMonitorLimit {
		t.Fatalf("calls = %d, want %d", len(got.Calls), groupMonitorLimit)
	}
	requests := 0
	nonEmptyBuckets := 0
	for _, bucket := range got.LatencyBuckets {
		requests += bucket.RequestCount
		if bucket.RequestCount > 0 {
			nonEmptyBuckets++
		}
	}
	if requests != len(usage) {
		t.Fatalf("bucket requests = %d, want %d", requests, len(usage))
	}
	if nonEmptyBuckets < 2 {
		t.Fatalf("non-empty buckets = %d, want at least 2", nonEmptyBuckets)
	}
}

func TestPublicGroupMonitorRangeCacheIsolationAndInvalidation(t *testing.T) {
	now := time.Now()
	s := &Service{publicMonitorCache: make(map[publicMonitorCacheKey]publicMonitorCacheEntry)}
	for _, rangeName := range []string{"24h", "7d"} {
		s.storePublicMonitor(publicMonitorCacheKey{stationID: 3, rangeName: rangeName}, PublicGroupMonitor{StationID: 3, StationName: rangeName, Range: rangeName}, now)
	}
	s.storePublicMonitor(publicMonitorCacheKey{stationID: 4, rangeName: "24h"}, PublicGroupMonitor{StationID: 4, Range: "24h"}, now)
	for _, rangeName := range []string{"24h", "7d"} {
		view, err := s.PublicGroupMonitorForRange(context.Background(), 3, rangeName)
		if err != nil || view.StationName != rangeName || view.Range != rangeName {
			t.Fatalf("cache for %s = %#v, %v", rangeName, view, err)
		}
	}
	view, err := s.PublicGroupMonitor(context.Background(), 3)
	if err != nil || view.Range != "24h" {
		t.Fatalf("default cache = %#v, %v", view, err)
	}
	s.invalidatePublicMonitor(3)
	if len(s.publicMonitorCache) != 1 {
		t.Fatalf("remaining cache entries = %d, want 1", len(s.publicMonitorCache))
	}
	if _, ok := s.publicMonitorCache[publicMonitorCacheKey{stationID: 4, rangeName: "24h"}]; !ok {
		t.Fatal("unrelated station cache was invalidated")
	}
}

func TestGroupMonitorEndpointsCarryTimeRange(t *testing.T) {
	since := time.Date(2026, 10, 3, 1, 30, 0, 0, time.UTC)
	until := time.Date(2026, 10, 9, 1, 30, 0, 0, time.UTC)
	for _, kind := range []monitorFetchKind{monitorFetchUsage, monitorFetchErrors} {
		endpoint, err := url.Parse(groupMonitorEndpoint("https://example.com/", 28, kind, since, until))
		if err != nil {
			t.Fatal(err)
		}
		query := endpoint.Query()
		if query.Get("group_id") != "28" || query.Get("page_size") != strconv.Itoa(groupMonitorPageSize) || query.Get("page") != "1" {
			t.Fatalf("query = %s", endpoint.RawQuery)
		}
		if kind == monitorFetchUsage {
			if endpoint.Path != "/api/v1/admin/usage" || query.Get("start_date") != "2026-10-03" || query.Get("end_date") != "2026-10-09" || query.Get("timezone") != "UTC" {
				t.Fatalf("usage endpoint = %s", endpoint)
			}
		} else if endpoint.Path != "/api/v1/admin/ops/request-errors" || query.Get("start_time") != since.Format(time.RFC3339Nano) || query.Get("end_time") != until.Format(time.RFC3339Nano) {
			t.Fatalf("error endpoint = %s", endpoint)
		}
	}
}

func TestGroupMonitorUsageFetchPaginatesPastRecentCallLimit(t *testing.T) {
	requestedPages := make([]int, 0, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/usage" || r.URL.Query().Get("group_id") != "28" || r.URL.Query().Get("page_size") != strconv.Itoa(groupMonitorPageSize) {
			t.Errorf("request = %s %s", r.Method, r.URL.String())
		}
		page, err := strconv.Atoi(r.URL.Query().Get("page"))
		if err != nil {
			t.Errorf("page query = %q", r.URL.Query().Get("page"))
			return
		}
		requestedPages = append(requestedPages, page)
		items := make([]remoteUsage, 0, groupMonitorPageSize)
		remaining := 420 - (page-1)*groupMonitorPageSize
		if remaining > groupMonitorPageSize {
			remaining = groupMonitorPageSize
		}
		for index := 0; index < remaining; index++ {
			id := (page-1)*groupMonitorPageSize + index + 1
			items = append(items, remoteUsage{ID: int64(id), CreatedAt: time.Date(2026, 10, 9, 11, 0, id%60, 0, time.UTC).Format(time.RFC3339Nano), FirstTokenMS: 100, DurationMS: 1000})
		}
		data, _ := json.Marshal(map[string]any{"items": items, "page": page, "page_size": groupMonitorPageSize, "pages": 3, "total": 420})
		_, _ = fmt.Fprintf(w, `{"code":0,"data":%s}`, data)
	}))
	defer server.Close()

	service := &Service{client: server.Client()}
	since := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	events, err := fetchGroupMonitorUsageEvents(context.Background(), service, server.URL, "key", 28, since, until)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 420 {
		t.Fatalf("events = %d, want 420", len(events))
	}
	sort.Ints(requestedPages)
	if len(requestedPages) != 3 || requestedPages[0] != 1 || requestedPages[1] != 2 || requestedPages[2] != 3 {
		t.Fatalf("requested pages = %#v, want [1 2 3] to detect the terminal short page", requestedPages)
	}
}
