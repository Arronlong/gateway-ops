package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	_ "github.com/guliansheng/gateway-ops/internal/connector/sub2api"
	appcrypto "github.com/guliansheng/gateway-ops/internal/crypto"
	"github.com/guliansheng/gateway-ops/internal/progress"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

func TestSub2APIContinuousHistoryIncludesLiveAndFinalWindowDetails(t *testing.T) {
	for _, interrupt := range []bool{false, true} {
		t.Run(fmt.Sprint(interrupt), func(t *testing.T) {
			f := newUsageHistoryFixture(t, nil, -1)
			recorder := &usageProgressRecorder{}
			start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
			until := start.AddDate(0, 1, 0)
			var detailBeforeLaterPage bool
			var readPages []int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/api/v1/auth/me" {
					_, _ = w.Write([]byte(`{"code":0,"data":{"id":7}}`))
					return
				}
				if r.URL.Path != "/api/v1/usage" {
					http.NotFound(w, r)
					return
				}
				page, _ := strconv.Atoi(r.URL.Query().Get("page"))
				size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
				if size == 100 {
					readPages = append(readPages, page)
					if page > 1 {
						for _, event := range recorder.events {
							data, ok := event.Data.(map[string]any)
							if !ok || data["kind"] != "usage_integrity" {
								continue
							}
							if result, ok := data["result"].(UsageIntegrity); ok && result.Audit != nil {
								for _, item := range result.Audit.Windows {
									if item.Level == "history" && item.Status == "repairing" {
										detailBeforeLaterPage = true
									}
								}
							}
						}
					}
					if interrupt && page == 2 {
						w.WriteHeader(400)
						return
					}
				}
				items := []map[string]any{}
				for index := (page - 1) * size; index < page*size && index < 205; index++ {
					items = append(items, map[string]any{"id": index + 1, "model": "test", "created_at": start.Add(time.Duration(205-index) * time.Minute).Format(time.RFC3339), "actual_cost": 1})
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"items": items, "total": 205, "page": page, "page_size": size, "pages": (205 + size - 1) / size}})
			}))
			defer server.Close()
			cipher, _ := appcrypto.NewCipher("usage-history-test")
			credential, _ := cipher.Encrypt(`{"access_token":"test"}`)
			f.ch.Type, f.ch.SiteURL, f.ch.PasswordCipher = storage.ChannelTypeSub2API, server.URL, credential
			if err := f.db.Save(&f.ch).Error; err != nil {
				t.Fatal(err)
			}
			f.account.PasswordCipher = credential
			if err := f.db.Save(&f.account).Error; err != nil {
				t.Fatal(err)
			}
			report, err := f.service.RefetchUsageHistoryWithReport(progress.WithObserver(context.Background(), recorder), &f.ch, time.Time{}, until)
			if (err != nil) != interrupt {
				t.Fatalf("error=%v interrupt=%v", err, interrupt)
			}
			if !detailBeforeLaterPage {
				t.Fatal("later pages were fetched without live history window details")
			}
			if len(readPages) != 3 || readPages[0] != 1 || readPages[1] != 3 || readPages[2] != 2 {
				t.Fatalf("extra history reads: %v", readPages)
			}
			if len(report.Windows) != 1 || report.Windows[0].Level != "history" {
				t.Fatalf("missing or duplicate history details: %#v", report.Windows)
			}
			item := report.Windows[0]
			wantStatus := "repaired"
			wantLocal := int64(205)
			wantPages := 3
			if interrupt {
				wantStatus = "interrupted"
				wantLocal = 105
				wantPages = 2
			}
			if item.Status != wantStatus || item.LocalCount != wantLocal || item.UpstreamCount != 205 || item.AccountID != f.account.ID || !item.Until.Equal(until) {
				t.Fatalf("wrong history detail: %#v", item)
			}
			if item.SavedPages != wantPages || int64(item.SavedRows) != wantLocal || item.CountsPending {
				t.Fatalf("missing page progress: %#v", item)
			}
		})
	}
}

func TestUsageHistoryWindowDetailExistsBeforeRepairStarts(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	report := &UsageHistoryReport{}
	walker := &usageHistoryWalker{AccountID: 7, AccountName: "test", Report: report, Count: func(context.Context, usageHistoryWindow) (usageWindowCounts, error) {
		return usageWindowCounts{Local: 1, Remote: 3}, nil
	}}
	walker.Repair = func(context.Context, usageHistoryWindow, int64) error {
		if len(report.Windows) != 1 || report.Windows[0].Status != "repairing" {
			t.Fatalf("repair started before its detail was visible: %#v", report.Windows)
		}
		return nil
	}
	if _, err := walker.walk(context.Background(), usageHistoryWindow{Since: start, Until: start.AddDate(0, 0, 1)}, "day", nil); err != nil {
		t.Fatal(err)
	}
	if len(report.Windows) != 1 || report.Windows[0].Status != "repaired" {
		t.Fatalf("final detail did not replace the in-progress entry: %#v", report.Windows)
	}
}
