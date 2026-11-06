package monitor

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/guliansheng/gateway-ops/internal/connector"
	"github.com/guliansheng/gateway-ops/internal/progress"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"golang.org/x/sync/singleflight"
)

type UsageAccountIntegrity struct {
	RetainedCount         int64      `json:"retained_count,omitempty"`
	HistoryAvailableSince *time.Time `json:"history_available_since,omitempty"`
	HistoryVerifiedUntil  *time.Time `json:"history_verified_until,omitempty"`
	AccountID             uint       `json:"account_id"`
	AccountName           string     `json:"account_name"`
	LocalCount            int64      `json:"local_count"`
	UpstreamCount         *int64     `json:"upstream_count"`
	Error                 string     `json:"error,omitempty"`
}

type UsageIntegrity struct {
	RetainedCount int64                   `json:"retained_count,omitempty"`
	Phase         string                  `json:"phase,omitempty"`
	Error         string                  `json:"error,omitempty"`
	RetryAt       *time.Time              `json:"retry_at,omitempty"`
	RetryStatus   int                     `json:"retry_status,omitempty"`
	Audit         *UsageHistoryReport     `json:"audit,omitempty"`
	Status        string                  `json:"status"`
	LocalCount    int64                   `json:"local_count"`
	UpstreamCount *int64                  `json:"upstream_count"`
	Since         time.Time               `json:"since"`
	Until         time.Time               `json:"until"`
	CheckedAt     time.Time               `json:"checked_at"`
	Accounts      []UsageAccountIntegrity `json:"accounts"`
}

type usageRemoteCount struct {
	count     int64
	err       error
	checkedAt time.Time
	expires   time.Time
}
type usageAuditCache struct {
	mu       sync.Mutex
	entries  map[string]usageRemoteCount
	versions map[uint]uint64
	windows  map[string]uint64
	calls    singleflight.Group
	slots    chan struct{}
}

func (cache *usageAuditCache) invalidateWindow(channelID uint, since, until time.Time) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.windows == nil {
		cache.windows = make(map[string]uint64)
	}
	cache.windows[fmt.Sprintf("%d:%d:%d", channelID, since.UnixNano(), until.UnixNano())]++
}

func (cache *usageAuditCache) invalidate(channelID uint) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	prefix := fmt.Sprintf("%d:", channelID)
	if cache.versions == nil {
		cache.versions = make(map[uint]uint64)
	}
	cache.versions[channelID]++
	for key := range cache.entries {
		if strings.HasPrefix(key, prefix) {
			delete(cache.entries, key)
		}
	}
}

func (s *Service) cachedUsageCountWith(ctx context.Context, ch *storage.Channel, account storage.ChannelAccount, since, until time.Time, read func(context.Context) (int64, error)) usageRemoteCount {
	cache := &s.usageAudit
	cache.mu.Lock()
	windowKey := fmt.Sprintf("%d:%d:%d", ch.ID, since.UnixNano(), until.UnixNano())
	key := fmt.Sprintf("%d:%d:%d:%d:%d:%d:%d", ch.ID, account.ID, since.UnixNano(), until.UnixNano(), ch.UpdatedAt.UnixNano(), cache.versions[ch.ID], cache.windows[windowKey])
	if cache.entries == nil {
		cache.entries = make(map[string]usageRemoteCount)
		cache.slots = make(chan struct{}, 3)
	}
	entry, found := cache.entries[key]
	cache.mu.Unlock()
	if found && time.Now().Before(entry.expires) {
		return entry
	}
	result := cache.calls.DoChan(key, func() (any, error) {
		cache.mu.Lock()
		entry, found := cache.entries[key]
		cache.mu.Unlock()
		if found && time.Now().Before(entry.expires) {
			return entry, nil
		}
		sharedCtx := progress.WithObserver(context.WithoutCancel(ctx), progress.NopObserver{})
		readCtx, cancel := context.WithTimeout(sharedCtx, 5*time.Minute)
		defer cancel()
		entry = usageRemoteCount{checkedAt: time.Now().UTC()}
		select {
		case cache.slots <- struct{}{}:
			defer func() { <-cache.slots }()
		case <-readCtx.Done():
			entry.err = readCtx.Err()
			return entry, nil
		}
		entry.count, entry.err = read(readCtx)
		entry.checkedAt = time.Now().UTC()
		entry.expires = time.Now().Add(5 * time.Minute)
		if readCtx.Err() != nil || entry.err != nil {
			return entry, nil
		}
		cache.mu.Lock()
		for oldKey, oldEntry := range cache.entries {
			if time.Now().After(oldEntry.expires) {
				delete(cache.entries, oldKey)
			}
		}
		cache.entries[key] = entry
		cache.mu.Unlock()
		return entry, nil
	})
	select {
	case <-ctx.Done():
		return usageRemoteCount{err: ctx.Err(), checkedAt: time.Now().UTC()}
	case value := <-result:
		if value.Err != nil {
			return usageRemoteCount{err: value.Err, checkedAt: time.Now().UTC()}
		}
		return value.Val.(usageRemoteCount)
	}
}

func (s *Service) CheckUsageIntegrity(ctx context.Context, ch *storage.Channel, since, until time.Time) (*UsageIntegrity, error) {
	result, _, err := s.checkUsageIntegrity(ctx, ch, since, until, false)
	return result, err
}

func (s *Service) CheckUsageTotals(ctx context.Context, ch *storage.Channel, since, until time.Time) (*UsageIntegrity, error) {
	result, _, err := s.checkUsageIntegrity(ctx, ch, since, until, false, true)
	return result, err
}

func (s *Service) UsageRequestWait(site string) connector.UsageWaitInfo {
	return s.usageLimiter.WaitInfo(site)
}

func (s *Service) emitUsageFailure(ctx context.Context, ch *storage.Channel, result *UsageIntegrity, message string) {
	info := s.UsageRequestWait(ch.SiteURL)
	phase := "interrupted"
	if info.Status > 0 && info.Until.After(time.Now()) {
		result.RetryAt = &info.Until
		result.RetryStatus = info.Status
		result.Error = (&connector.UsagePauseError{Until: info.Until, Status: info.Status}).Error()
		message = result.Error
		phase = "paused"
	}
	emitUsageIntegrity(ctx, result, phase, message)
}
