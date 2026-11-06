package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/guliansheng/gateway-ops/internal/channel"
	"github.com/guliansheng/gateway-ops/internal/connector"
	appcrypto "github.com/guliansheng/gateway-ops/internal/crypto"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type usageHistoryFixture struct {
	rejectAuth                          bool
	authRequests                        int
	rejectCounts                        bool
	service                             *Service
	ch                                  storage.Channel
	account                             storage.ChannelAccount
	repo                                *storage.ChannelUsage
	db                                  *gorm.DB
	reads                               []usageHistoryWindow
	remote                              []time.Time
	countRequests                       int
	rootCount                           *int
	rootUntil                           time.Time
	beforeRead                          func()
	failRead, missingCounts, incomplete bool
}

func newUsageHistoryFixture(t *testing.T, stamps []time.Time, missing int) *usageHistoryFixture {
	t.Helper()
	fixture := &usageHistoryFixture{remote: stamps}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/self":
			fixture.authRequests++
			if fixture.rejectAuth {
				w.WriteHeader(403)
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":7}}`))
		case "/api/status":
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
		case "/api/log/self":
			start, _ := strconv.ParseInt(r.URL.Query().Get("start_timestamp"), 10, 64)
			end, _ := strconv.ParseInt(r.URL.Query().Get("end_timestamp"), 10, 64)
			items := []map[string]any{}
			for index, stamp := range fixture.remote {
				if (start == 0 || stamp.Unix() >= start) && stamp.Unix() <= end {
					items = append(items, map[string]any{"id": index + 1, "type": 2, "request_id": fmt.Sprint(index + 1), "model_name": "test", "created_at": stamp.Unix(), "quota": 500000})
				}
			}
			sort.SliceStable(items, func(i, j int) bool { return items[i]["created_at"].(int64) > items[j]["created_at"].(int64) })
			if r.URL.Query().Get("type") != "0" {
				fixture.countRequests++
				if fixture.rejectCounts {
					w.WriteHeader(429)
					return
				}
				if fixture.missingCounts {
					_, _ = w.Write([]byte(`{"success":true,"data":{"items":[]}}`))
					return
				}
				total := len(items)
				if start == 0 && fixture.rootCount != nil && (fixture.rootUntil.IsZero() || end == fixture.rootUntil.Add(-time.Nanosecond).Unix()) {
					total = *fixture.rootCount
				}
				if r.URL.Query().Get("type") == "5" {
					total = 0
					items = nil
				}
				if len(items) > 1 {
					items = items[:1]
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"total": total, "page_size": 1, "items": items}})
				return
			}
			fixture.reads = append(fixture.reads, usageHistoryWindow{Since: time.Unix(start, 0), Until: time.Unix(end+1, 0)})
			if fixture.beforeRead != nil {
				fixture.beforeRead()
			}
			if fixture.failRead {
				w.WriteHeader(400)
				return
			}
			total := len(items)
			if fixture.incomplete && len(items) > 0 {
				items = items[:len(items)-1]
			}
			page, _ := strconv.Atoi(r.URL.Query().Get("p"))
			if page < 1 {
				page = 1
			}
			startIndex := (page - 1) * 100
			if startIndex >= len(items) {
				items = nil
			} else {
				endIndex := startIndex + 100
				if endIndex > len(items) {
					endIndex = len(items)
				}
				items = items[startIndex:endIndex]
			}
			data := map[string]any{"total": total, "page_size": 100, "items": items}
			if fixture.missingCounts {
				delete(data, "total")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "history.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&storage.Channel{}, &storage.ChannelAccount{}, &storage.ChannelUsageEvent{}, &storage.ChannelUsageSyncState{}, &storage.ChannelUsageHistorySeen{}); err != nil {
		t.Fatal(err)
	}
	cipher, _ := appcrypto.NewCipher("usage-history-test")
	credential, _ := cipher.Encrypt(`{"auth_type":"access_token","token":"test","user_id":"7"}`)
	fixture.ch = storage.Channel{Name: "history", Type: storage.ChannelTypeNewAPI, SiteURL: server.URL, Username: "test", PasswordCipher: credential, CredentialMode: storage.CredentialModeToken, BalanceMode: storage.BalanceModeAuto}
	if err := db.Create(&fixture.ch).Error; err != nil {
		t.Fatal(err)
	}
	fixture.account = storage.ChannelAccount{ChannelID: fixture.ch.ID, IsPrimary: true, Username: "test", PasswordCipher: credential, CredentialMode: storage.CredentialModeToken}
	if err := db.Create(&fixture.account).Error; err != nil {
		t.Fatal(err)
	}
	fixture.db = db
	fixture.repo = storage.NewChannelUsage(db)
	for index, stamp := range stamps {
		if index != missing {
			if err := fixture.repo.Upsert([]storage.ChannelUsageEvent{{ChannelID: fixture.ch.ID, ChannelAccountID: fixture.account.ID, Source: "newapi", RemoteUsageID: fmt.Sprintf("newapi:v2:seed-%d", index), OccurredAt: stamp}}); err != nil {
				t.Fatal(err)
			}
		}
	}
	channels := storage.NewChannels(db)
	fixture.service = NewService(channels, nil, nil, fixture.repo, channel.NewService(channels, nil, nil, nil, cipher), nil, nil)
	fixture.service.usageLimiter = connector.NewUsageLimiter(0, time.Minute)
	return fixture
}

func TestRefetchUsageHistoryChecksMonthlyFirstPagesAndSkipsHealthyRetry(t *testing.T) {
	day := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
	fixture := newUsageHistoryFixture(t, []time.Time{day.AddDate(0, -1, 0), day.Add(-7 * 24 * time.Hour), day.Add(12 * time.Hour), day.Add(13 * time.Hour), day.AddDate(0, 1, 0)}, 3)
	since := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	if err := fixture.service.RefetchUsageHistory(context.Background(), &fixture.ch, since, until); err != nil {
		t.Fatal(err)
	}
	if len(fixture.reads) != 3 || !fixture.reads[1].Since.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) || !fixture.reads[1].Until.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected monthly probes: %#v", fixture.reads)
	}
	count, err := fixture.repo.CountWindow(fixture.ch.ID, fixture.account.ID, "newapi", since, until)
	if err != nil || count != 5 {
		t.Fatalf("count=%d error=%v", count, err)
	}
	fixture.reads = nil
	report, err := fixture.service.RefetchUsageHistoryWithReport(context.Background(), &fixture.ch, since, until)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixture.reads) != 3 || report.SavedRows != 0 || report.SkippedWindows != 3 {
		t.Fatalf("healthy monthly retry did not skip: %#v", report)
	}
}

func TestRefetchUsageHistoryPreservesDataWhenCountsOrPagesAreIncomplete(t *testing.T) {
	day := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	for _, mode := range []string{"count", "read", "incomplete"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newUsageHistoryFixture(t, []time.Time{day.Add(time.Hour), day.Add(2 * time.Hour)}, 1)
			fixture.missingCounts = mode == "count"
			fixture.failRead = mode == "read"
			fixture.incomplete = mode == "incomplete"
			if err := fixture.service.RefetchUsageHistory(context.Background(), &fixture.ch, day, day.Add(24*time.Hour)); err == nil {
				t.Fatal("unverified history was accepted")
			}
			count, _ := fixture.repo.CountWindow(fixture.ch.ID, fixture.account.ID, "newapi", day, day.Add(24*time.Hour))
			want := int64(1)
			if mode == "count" {
				want = 3
			} else if mode == "incomplete" {
				want = 2
			}
			if count != want {
				t.Fatalf("existing history changed on failure: mode=%s count=%d want=%d reads=%#v", mode, count, want, fixture.reads)
			}
			if mode == "count" && len(fixture.reads) != 1 {
				t.Fatal("missing first-page total triggered additional requests")
			}
		})
	}
}

func TestAutomaticHistoryAuditPersistsContinuousProgressAndRechecksDaily(t *testing.T) {
	now := time.Now().UTC()
	oldest := time.Date(now.Year(), now.Month()-8, 10, 12, 0, 0, 0, time.UTC)
	fixture := newUsageHistoryFixture(t, []time.Time{oldest}, 0)
	if err := fixture.service.RefreshUsage(context.Background(), &fixture.ch); err != nil {
		t.Fatal(err)
	}
	state, _ := fixture.repo.SyncState(fixture.ch.ID, fixture.account.ID)
	if state.HistoryVerifiedUntil == nil || state.HistoryScan == "" {
		t.Fatalf("continuous history was not checkpointed: %#v", state)
	}
	for attempt := 0; attempt < 4; attempt++ {
		if err := fixture.service.RefreshUsage(context.Background(), &fixture.ch); err != nil {
			t.Fatal(err)
		}
	}
	state, _ = fixture.repo.SyncState(fixture.ch.ID, fixture.account.ID)
	if state.HistoryCheckedAt == nil || state.HistoryAuditBefore != nil {
		t.Fatalf("audit did not complete: %#v", state)
	}
	before := fixture.countRequests
	if err := fixture.service.RefreshUsage(context.Background(), &fixture.ch); err != nil {
		t.Fatal(err)
	}
	if fixture.countRequests != before {
		t.Fatal("completed history was re-audited before daily interval")
	}
	yesterday := now.Add(-25 * time.Hour)
	state.HistoryCheckedAt = &yesterday
	if err := fixture.repo.SaveSyncState(state); err != nil {
		t.Fatal(err)
	}
	fixture.service.usageAudit.invalidate(fixture.ch.ID)
	if err := fixture.service.RefreshUsage(context.Background(), &fixture.ch); err != nil {
		t.Fatal(err)
	}
	state, _ = fixture.repo.SyncState(fixture.ch.ID, fixture.account.ID)
	scan := &newAPIUsageScan{}
	if err := json.Unmarshal([]byte(state.HistoryScan), scan); err != nil {
		t.Fatal(err)
	}
	if (state.HistoryCheckedAt == nil || !state.HistoryCheckedAt.After(yesterday)) && scan.Plans["full"] == nil {
		t.Fatal("daily monthly audit did not complete or retain its resumable plan")
	}
}

func TestAllHistoryStopsAtUpstreamCoverageAndKeepsManualCursor(t *testing.T) {
	day := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	fixture := newUsageHistoryFixture(t, []time.Time{day, day.AddDate(0, 2, 0)}, -1)
	cursor := time.Now().Add(-time.Hour).UTC()
	if err := fixture.repo.SaveSyncState(&storage.ChannelUsageSyncState{ChannelID: fixture.ch.ID, ChannelAccountID: fixture.account.ID, ForwardSyncedAt: &cursor, BackfillVersion: channelUsageBackfillVersion}); err != nil {
		t.Fatal(err)
	}
	report, err := fixture.service.RefetchUsageHistoryWithReport(context.Background(), &fixture.ch, time.Time{}, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(fixture.reads) != 4 || report.CheckedWindows != 3 || fixture.countRequests != 0 || report.SkippedWindows != 3 {
		t.Fatalf("unbounded all-history audit: report=%#v counts=%d reads=%d", report, fixture.countRequests, len(fixture.reads))
	}
	state, _ := fixture.repo.SyncState(fixture.ch.ID, fixture.account.ID)
	if state.ForwardSyncedAt == nil || !state.ForwardSyncedAt.Equal(cursor) {
		t.Fatal("manual all-history audit moved the automatic cursor")
	}
	if state.HistoryVerifiedUntil == nil {
		t.Fatal("manual full history did not establish its independent checkpoint")
	}
}

func TestAllHistoryCanDiscoverMissingOlderMonthWithoutLocalRows(t *testing.T) {
	day := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	fixture := newUsageHistoryFixture(t, []time.Time{day}, 0)
	if err := fixture.service.RefetchUsageHistory(context.Background(), &fixture.ch, time.Time{}, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if len(fixture.reads) != 6 {
		t.Fatalf("received history was downloaded repeatedly: %#v", fixture.reads)
	}
	count, _ := fixture.repo.CountWindow(fixture.ch.ID, fixture.account.ID, "newapi", day.Truncate(24*time.Hour), day.Add(24*time.Hour))
	if count != 1 {
		t.Fatal("missing older history was not saved")
	}
}

func TestUsageIntegrityLocatesAnomalousDatesWithoutDownloadingDetails(t *testing.T) {
	day := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	fixture := newUsageHistoryFixture(t, []time.Time{day, day.AddDate(0, 1, 0)}, 0)
	check, err := fixture.service.CheckUsageIntegrity(context.Background(), &fixture.ch, day.AddDate(0, -1, 0), day.AddDate(0, 2, 0))
	if err != nil || check.Status != "mismatch" || check.Audit == nil || check.Audit.AnomalousDays != 1 || len(fixture.reads) != 0 {
		t.Fatalf("check=%#v error=%v details=%d", check, err, len(fixture.reads))
	}
	found := false
	for _, window := range check.Audit.Windows {
		if window.Level == "day" && window.Status == "mismatch" && window.Since.Equal(day.Truncate(24*time.Hour)) {
			found = true
		}
	}
	if !found {
		t.Fatal("anomalous date was not exposed")
	}
}

func TestCanceledHistoryAuditNeverDownloadsOrMutatesHistory(t *testing.T) {
	day := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	fixture := newUsageHistoryFixture(t, []time.Time{day}, 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := fixture.service.RefetchUsageHistory(ctx, &fixture.ch, day.Truncate(24*time.Hour), day.Truncate(24*time.Hour).Add(24*time.Hour)); err == nil {
		t.Fatal("cancellation was ignored")
	}
	if fixture.countRequests != 0 || len(fixture.reads) != 0 {
		t.Fatal("canceled audit contacted upstream")
	}
}

func TestAutomaticAuditDiscoversOlderMissingHistoryWithoutLocalRows(t *testing.T) {
	now := time.Now().UTC()
	oldest := time.Date(now.Year(), now.Month()-5, 10, 12, 0, 0, 0, time.UTC)
	fixture := newUsageHistoryFixture(t, []time.Time{oldest}, 0)
	for attempt := 0; attempt < 4; attempt++ {
		if err := fixture.service.RefreshUsage(context.Background(), &fixture.ch); err != nil {
			t.Fatal(err)
		}
	}
	count, _ := fixture.repo.CountWindow(fixture.ch.ID, fixture.account.ID, "newapi", oldest.Truncate(24*time.Hour), oldest.Truncate(24*time.Hour).Add(24*time.Hour))
	if count != 1 {
		t.Fatal("automatic monthly audit never discovered missing upstream history older than 30 days")
	}
}

func TestAutomaticAuditFailureDoesNotBlockRecentIncrementalUsage(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	fixture := newUsageHistoryFixture(t, []time.Time{now.Add(-time.Minute)}, 0)
	if err := fixture.repo.Upsert([]storage.ChannelUsageEvent{{ChannelID: fixture.ch.ID, ChannelAccountID: fixture.account.ID, Source: "newapi", RemoteUsageID: "retained-old-history", OccurredAt: now.AddDate(0, -2, 0)}}); err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.RefreshUsage(context.Background(), &fixture.ch); err != nil {
		t.Fatal(err)
	}
	last := fixture.reads[len(fixture.reads)-1]
	if last.Until.Sub(last.Since) > 11*time.Minute {
		t.Fatalf("historical anomaly blocked bounded incremental sync: %#v", fixture.reads)
	}
	state, _ := fixture.repo.SyncState(fixture.ch.ID, fixture.account.ID)
	if state.ForwardSyncedAt == nil {
		t.Fatal("successful incremental progress was lost")
	}
	count, _ := fixture.repo.CountWindow(fixture.ch.ID, fixture.account.ID, "newapi", time.Time{}, now)
	if count != 2 {
		t.Fatal("upstream retention removed old local history or blocked recent usage")
	}
}

func TestHistoryRepairIsolatesAccountsAndLeavesHealthyAccountUntouched(t *testing.T) {
	day := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	fixture := newUsageHistoryFixture(t, []time.Time{day.Add(time.Hour), day.Add(2 * time.Hour)}, 1)
	other := fixture.account
	other.ID = 0
	other.IsPrimary = false
	other.Username = "other"
	if err := fixture.db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	for index, stamp := range fixture.remote {
		if err := fixture.repo.Upsert([]storage.ChannelUsageEvent{{ChannelID: fixture.ch.ID, ChannelAccountID: other.ID, Source: "newapi", RemoteUsageID: fmt.Sprint("other-", index), OccurredAt: stamp, ActualCost: 77}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := fixture.service.RefetchUsageHistory(context.Background(), &fixture.ch, day, day.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if len(fixture.reads) != 2 {
		t.Fatalf("expected one first-page probe per account: reads=%d", len(fixture.reads))
	}
	events, err := fixture.repo.EventsBetween(fixture.ch.ID, day, day.Add(24*time.Hour))
	if err != nil || len(events) != 4 {
		t.Fatalf("events=%d error=%v", len(events), err)
	}
	retained := 0
	for _, event := range events {
		if event.ChannelAccountID == other.ID && event.ActualCost == 77 {
			retained++
		}
	}
	if retained != 2 {
		t.Fatal("healthy account rows were replaced")
	}
}

func TestUsageCountCacheAvoidsRepeatedAuthentication(t *testing.T) {
	day := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	fixture := newUsageHistoryFixture(t, []time.Time{day.Add(time.Hour)}, -1)
	for attempt := 0; attempt < 2; attempt++ {
		check, err := fixture.service.CheckUsageIntegrity(context.Background(), &fixture.ch, day, day.Add(24*time.Hour))
		if err != nil || check.Status != "matched" {
			t.Fatalf("check=%#v error=%v", check, err)
		}
	}
	if fixture.authRequests != 1 {
		t.Fatalf("cache hit repeated authentication %d times", fixture.authRequests)
	}
}

func TestUsageCooldownAlsoBlocksAuthenticationForOtherAccounts(t *testing.T) {
	day := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	fixture := newUsageHistoryFixture(t, nil, -1)
	other := fixture.account
	other.ID = 0
	other.IsPrimary = false
	other.Username = "other"
	if err := fixture.db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	fixture.rejectCounts = true
	check, err := fixture.service.CheckUsageIntegrity(context.Background(), &fixture.ch, day, day.Add(24*time.Hour))
	if err != nil || check.Status != "unavailable" {
		t.Fatalf("check=%#v error=%v", check, err)
	}
	if fixture.authRequests != 1 || fixture.countRequests != 1 {
		t.Fatalf("cooldown still contacted upstream: auth=%d count=%d", fixture.authRequests, fixture.countRequests)
	}
}

func TestAuthenticationRejectionPausesOtherUsageAccounts(t *testing.T) {
	fixture := newUsageHistoryFixture(t, nil, -1)
	other := fixture.account
	other.ID = 0
	other.IsPrimary = false
	other.Username = "other"
	if err := fixture.db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	fixture.rejectAuth = true
	day := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	check, err := fixture.service.CheckUsageIntegrity(context.Background(), &fixture.ch, day, day.Add(24*time.Hour))
	if err != nil || check.Status != "unavailable" {
		t.Fatalf("check=%#v error=%v", check, err)
	}
	if fixture.authRequests != 1 || fixture.countRequests != 0 {
		t.Fatalf("authentication rejection still allowed requests: auth=%d count=%d", fixture.authRequests, fixture.countRequests)
	}
}

func TestAutomaticAuditRebasesCheckpointWhenUpstreamHistoryChanges(t *testing.T) {
	now := time.Now().UTC()
	day := time.Date(now.Year(), now.Month()-5, 10, 12, 0, 0, 0, time.UTC)
	for _, mode := range []string{"added", "deleted"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newUsageHistoryFixture(t, []time.Time{day}, 0)
			remaining := int64(1)
			before := time.Date(day.Year(), day.Month()+1, 1, 0, 0, 0, 0, time.UTC)
			if err := fixture.repo.SaveSyncState(&storage.ChannelUsageSyncState{ChannelID: fixture.ch.ID, ChannelAccountID: fixture.account.ID, BackfillVersion: channelUsageBackfillVersion, HistoryAuditBefore: &before, HistoryAuditRemaining: &remaining}); err != nil {
				t.Fatal(err)
			}
			if mode == "added" {
				fixture.remote = append(fixture.remote, day.Add(time.Hour))
			} else {
				fixture.remote = nil
			}
			for attempt := 0; attempt < 4; attempt++ {
				if err := fixture.service.RefreshUsage(context.Background(), &fixture.ch); err != nil {
					t.Fatal(err)
				}
			}
			state, _ := fixture.repo.SyncState(fixture.ch.ID, fixture.account.ID)
			if state.HistoryAuditBefore != nil || state.HistoryCheckedAt == nil {
				t.Fatalf("audit checkpoint did not recover after upstream %s: %#v", mode, state)
			}
		})
	}
}

type usagePageReaderFunc func(context.Context, *connector.Channel, *connector.AuthSession, connector.UsageQuery) (*connector.UsagePage, error)

func (read usagePageReaderFunc) ListUsage(ctx context.Context, ch *connector.Channel, session *connector.AuthSession, query connector.UsageQuery) (*connector.UsagePage, error) {
	return read(ctx, ch, session, query)
}

func TestSub2APIIncrementalReadStopsAfterCrossingWindowStart(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	fixture := newUsageHistoryFixture(t, nil, -1)
	fixture.ch.Type = storage.ChannelTypeSub2API
	state, _ := fixture.repo.SyncState(fixture.ch.ID, fixture.account.ID)
	reads := 0
	reader := usagePageReaderFunc(func(_ context.Context, _ *connector.Channel, _ *connector.AuthSession, query connector.UsageQuery) (*connector.UsagePage, error) {
		reads++
		if reads > 3 {
			return nil, fmt.Errorf("incremental read enumerated the whole day")
		}
		page := &connector.UsagePage{RawCount: 100, Page: query.Page, PageSize: 100, Total: 50000}
		for index := 0; index < 100; index++ {
			offset := (query.Page-1)*100 + index + 1
			page.Records = append(page.Records, connector.UsageRecord{RemoteID: fmt.Sprint(offset), CreatedAt: now.Add(-time.Duration(offset) * time.Second)})
		}
		return page, nil
	})
	job := &usageHistoryAccount{service: fixture.service, channel: &fixture.ch, account: fixture.account, state: state, reader: reader}
	if err := job.readWindow(context.Background(), usageHistoryWindow{Since: now.Add(-10 * time.Second), Until: now}, -1, false); err != nil {
		t.Fatal(err)
	}
	count, _ := fixture.repo.CountWindow(fixture.ch.ID, fixture.account.ID, "sub2api", now.Add(-10*time.Second), now)
	if reads != 1 || count != 10 {
		t.Fatalf("incremental read fetched excess history: pages=%d count=%d", reads, count)
	}
}
