package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

func TestGroupMonitorUsageFetchKeepsHistoryPastEstimatedPages(t *testing.T) {
	for _, metadata := range []struct {
		name  string
		total int
		pages int
	}{
		{name: "capped at two pages", total: 400, pages: 2},
		{name: "missing page counts"},
		{name: "overestimated page counts", total: 1000, pages: 5},
	} {
		t.Run(metadata.name, func(t *testing.T) {
			since := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
			until := since.Add(24 * time.Hour)
			var mu sync.Mutex
			requested := make(map[int]bool)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				page, _ := strconv.Atoi(r.URL.Query().Get("page"))
				mu.Lock()
				requested[page] = true
				mu.Unlock()
				items := make([]remoteUsage, 0, groupMonitorPageSize)
				for index := (page - 1) * groupMonitorPageSize; index < page*groupMonitorPageSize && index < 630; index++ {
					items = append(items, remoteUsage{
						ID:           int64(index + 1),
						RequestID:    fmt.Sprintf("request-%d", index),
						CreatedAt:    since.Add(time.Duration(629-index) * until.Sub(since) / 630).Format(time.RFC3339Nano),
						FirstTokenMS: 100,
						DurationMS:   1000,
					})
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": remoteUsagePage{
					Items: items, Page: page, PageSize: groupMonitorPageSize, Total: metadata.total, Pages: metadata.pages,
				}})
			}))
			defer server.Close()
			service := &Service{client: server.Client()}
			events, err := fetchGroupMonitorUsageEvents(context.Background(), service, server.URL, "key", 28, since, until)
			if err != nil {
				t.Fatal(err)
			}
			if len(events) != 630 {
				t.Fatalf("events = %d, want all 630 requests, including older pages", len(events))
			}
			if len(requested) > 5 || !requested[4] {
				t.Fatalf("requested pages = %v, want a bounded fetch through the short fourth page", requested)
			}
			group := storage.RelayGroup{ExternalID: 28, Status: "active", MonitorEnabled: true}
			view := buildPublicGroupMonitorGroupForWindow(group, events, nil, true, until, since, until)
			if len(view.Calls) != groupMonitorLimit || view.SuccessCount != 630 {
				t.Fatalf("recent calls = %d, successes = %d, want 60 recent calls and 630 successes", len(view.Calls), view.SuccessCount)
			}
			for index, bucket := range view.LatencyBuckets {
				if bucket.RequestCount == 0 {
					t.Fatalf("bucket %d is empty despite continuous traffic across all 24 hours", index)
				}
			}
		})
	}
}

func TestGroupMonitorErrorFetchKeepsHistoryPastEstimatedPages(t *testing.T) {
	since := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	until := since.Add(24 * time.Hour)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		items := make([]remoteRequestError, 0, groupMonitorPageSize)
		for index := (page - 1) * groupMonitorPageSize; index < page*groupMonitorPageSize && index < 430; index++ {
			items = append(items, remoteRequestError{
				RequestID:  fmt.Sprintf("error-%d", index),
				CreatedAt:  since.Add(time.Duration(429-index) * until.Sub(since) / 430).Format(time.RFC3339Nano),
				ErrorOwner: "provider", StatusCode: 502, Message: "Upstream request failed", GroupID: 28,
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": remoteRequestErrorPage{
			Items: items, Page: page, PageSize: groupMonitorPageSize, Total: 400, Pages: 2,
		}})
	}))
	defer server.Close()
	service := &Service{client: server.Client()}
	events, err := fetchGroupMonitorErrorEvents(context.Background(), service, server.URL, "key", 28, since, until)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 430 {
		t.Fatalf("events = %d, want all 430 failures including the third page", len(events))
	}
}

func TestGroupMonitorUsageFetchReportsContinuationFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page >= 3 {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": remoteUsagePage{
			Items: make([]remoteUsage, groupMonitorPageSize), Page: page, PageSize: groupMonitorPageSize, Total: 400, Pages: 2,
		}})
	}))
	defer server.Close()
	service := &Service{client: server.Client()}
	until := time.Now()
	if _, err := fetchGroupMonitorUsagePages(context.Background(), service, server.URL, "key", 28, until.Add(-24*time.Hour), until); err == nil {
		t.Fatal("missing error when older history cannot be loaded")
	}
}
