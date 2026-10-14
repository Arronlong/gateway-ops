package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newAPILoadingFixture(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	calls := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/status":
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"quota_per_unit": 100}})
		case "/api/log/":
			calls.Add(1)
			query := r.URL.Query()
			if query.Get("start_timestamp") != "1791972000" || query.Get("end_timestamp") != "1791975600" {
				t.Errorf("exact time range was lost: %v", query)
			}
			page, _ := strconv.Atoi(query.Get("p"))
			size, _ := strconv.Atoi(query.Get("page_size"))
			kind, _ := strconv.Atoi(query.Get("type"))
			total := 250
			if kind == 5 {
				total = 50
			}
			items := []map[string]any{}
			for index := (page - 1) * size; index < page*size && index < total; index++ {
				items = append(items, map[string]any{"id": kind*1000 + index, "type": kind, "created_at": time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC).Unix() + int64(total-index), "user_id": 7, "username": "alice", "quota": 100, "prompt_tokens": 2, "completion_tokens": 3})
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"items": items, "total": total, "page": page, "page_size": size}})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, calls
}

func TestNewAPIFullUsageReadsEachNativePageOnce(t *testing.T) {
	server, calls := newAPILoadingFixture(t)
	raw, _ := encodeNewAPICredential(newAPICredential{Token: "token", UserID: "7"})
	since := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	service := NewService(nil, nil)
	items, err := service.fetchStationUsageItems(context.Background(), server.URL, raw, since, since.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 300 {
		t.Fatalf("rows = %d, want all 300", len(items))
	}
	if calls.Load() != 4 {
		t.Fatalf("native log requests = %d, want exactly 4", calls.Load())
	}
	var ranking remoteUserRanking
	query := url.Values{"start_time": {since.Format(time.RFC3339)}, "end_time": {since.Add(time.Hour).Format(time.RFC3339)}}
	credential, _ := decodeNewAPICredential(raw)
	if err := service.newAPIRanking(context.Background(), server.URL, credential, query, &ranking); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 4 {
		t.Fatalf("user statistics repeated log scans: %d requests", calls.Load())
	}
	if ranking.TotalRequests != 250 || ranking.TotalTokens != 1250 || ranking.TotalActualCost != 250 {
		t.Fatalf("incorrect ranking: %#v", ranking)
	}
}

func TestNewAPIConcurrentRankingsShareOneScan(t *testing.T) {
	server, calls := newAPILoadingFixture(t)
	service := NewService(nil, nil)
	var group sync.WaitGroup
	errors := make(chan error, 4)
	for index := 0; index < 4; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			var result remoteUserRanking
			query := url.Values{"start_time": {"2026-10-14T10:00:00Z"}, "end_time": {"2026-10-14T11:00:00Z"}}
			if err := service.newAPIRanking(context.Background(), server.URL, newAPICredential{Token: "token", UserID: "7"}, query, &result); err != nil {
				errors <- err
			}
		}()
	}
	group.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatal(fmt.Sprintf("concurrent requests scanned %d pages, want 3", calls.Load()))
	}
}
