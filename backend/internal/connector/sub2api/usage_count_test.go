package sub2api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/guliansheng/gateway-ops/internal/connector"
)

func TestCountUsageSeparatesPartialBoundaryDaysFromFullDayTotals(t *testing.T) {
	for _, metadata := range []bool{true, false} {
		t.Run(strconv.FormatBool(metadata), func(t *testing.T) {
			stamps := []string{"2026-10-22T15:00:00Z", "2026-10-22T16:00:00Z", "2026-10-23T00:00:00Z", "2026-10-23T12:00:00Z", "2026-10-24T15:59:59Z", "2026-10-24T16:00:00Z"}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				start, end := r.URL.Query().Get("start_date"), r.URL.Query().Get("end_date")
				items := []map[string]any{}
				for i, stamp := range stamps {
					if (start == "" || stamp[:10] >= start) && stamp[:10] <= end {
						items = append(items, map[string]any{"id": i + 1, "created_at": stamp})
					}
				}
				total := len(items)
				for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
					items[left], items[right] = items[right], items[left]
				}
				page, _ := strconv.Atoi(r.URL.Query().Get("page"))
				size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
				if !metadata && size > 2 {
					size = 2
				}
				offset := (page - 1) * size
				if offset >= len(items) {
					items = nil
				} else {
					stop := offset + size
					if stop > len(items) {
						stop = len(items)
					}
					items = items[offset:stop]
				}
				data := map[string]any{"items": items}
				if metadata {
					data["total"] = total
					data["page_size"] = size
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
			}))
			defer server.Close()
			since, _ := time.Parse(time.RFC3339, "2026-10-22T16:00:00Z")
			until := since.Add(48 * time.Hour)
			count, err := New().CountUsage(context.Background(), &connector.Channel{SiteURL: server.URL}, &connector.AuthSession{AccessToken: "test"}, connector.UsageQuery{Since: since, Until: until})
			if !metadata {
				if err == nil {
					t.Fatal("missing totals must not trigger history enumeration")
				}
				return
			}
			if err != nil || count != 4 {
				t.Fatalf("count=%d error=%v, want 4", count, err)
			}
		})
	}
}
