package relay

import (
	"context"
	"time"
)

type reportCacheEntry struct {
	value     any
	err       error
	expiresAt time.Time
}

func (s *Service) invalidateUserReports(stationID uint) {
	s.reportMu.Lock()
	defer s.reportMu.Unlock()
	if s.userReportRevision == nil {
		s.userReportRevision = make(map[uint]uint64)
	}
	s.userReportRevision[stationID]++
}

// 报表读取共享同一次计算；浏览器轮询或切换模块不会重启已在进行的上游扫描。
func (s *Service) cachedReport(ctx context.Context, key string, ttl time.Duration, load func(context.Context) (any, error)) (any, error) {
	s.reportMu.Lock()
	cached, ok := s.reportCache[key]
	s.reportMu.Unlock()
	if ok && time.Now().Before(cached.expiresAt) {
		return cached.value, cached.err
	}
	result := s.reportGroup.DoChan(key, func() (any, error) {
		s.reportMu.Lock()
		cached, ok := s.reportCache[key]
		s.reportMu.Unlock()
		if ok && time.Now().Before(cached.expiresAt) {
			return cached.value, cached.err
		}
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		value, err := load(loadCtx)
		expires := time.Now().Add(ttl)
		if err != nil {
			expires = time.Now().Add(5 * time.Second)
		}
		s.reportMu.Lock()
		if s.reportCache == nil {
			s.reportCache = make(map[string]reportCacheEntry)
		}
		for entryKey, entry := range s.reportCache {
			if time.Now().After(entry.expiresAt) {
				delete(s.reportCache, entryKey)
			}
		}
		s.reportCache[key] = reportCacheEntry{value: value, err: err, expiresAt: expires}
		s.reportMu.Unlock()
		return value, err
	})
	select {
	case value := <-result:
		return value.Val, value.Err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
