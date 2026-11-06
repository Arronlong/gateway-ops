package connector

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type usageLimitState struct {
	next, blockedUntil, waitingUntil time.Time
	status                           int
	rejections                       int
	requests                         []time.Time
}
type UsageWaitInfo struct {
	Until  time.Time
	Status int
}
type UsagePauseError struct {
	Until  time.Time
	Status int
}

func (pause *UsagePauseError) Error() string {
	reason := "\u4e0a\u6e38\u62d2\u7edd\u8bbf\u95ee"
	if pause.Status == 429 {
		reason = "\u4e0a\u6e38\u8bf7\u6c42\u9891\u7387\u53d7\u9650"
	}
	return fmt.Sprintf("%s\uff08HTTP %d\uff09\uff0c\u8bf7\u6c42\u6682\u7f13\u81f3 %s", reason, pause.Status, pause.Until.UTC().Format(time.RFC3339))
}

type UsageLimiter struct {
	mu                 sync.Mutex
	sites              map[string]usageLimitState
	locks              map[string]chan struct{}
	interval, cooldown time.Duration
	budget             int
	window             time.Duration
}
type usageLimiterKey struct{}

func NewUsageLimiter(interval, cooldown time.Duration) *UsageLimiter {
	return &UsageLimiter{sites: make(map[string]usageLimitState), locks: make(map[string]chan struct{}), interval: interval, cooldown: cooldown}
}

func NewUsageLimiterWithBudget(interval, cooldown time.Duration, budget int, window time.Duration) *UsageLimiter {
	limiter := NewUsageLimiter(interval, cooldown)
	limiter.budget = budget
	limiter.window = window
	return limiter
}

func (limiter *UsageLimiter) WaitInfo(site string) UsageWaitInfo {
	if limiter == nil {
		return UsageWaitInfo{}
	}
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	state := limiter.sites[usageOrigin(site)]
	if time.Now().Before(state.blockedUntil) {
		return UsageWaitInfo{Until: state.blockedUntil, Status: state.status}
	}
	return UsageWaitInfo{Until: state.waitingUntil}
}

func WithUsageLimiter(ctx context.Context, limiter *UsageLimiter) context.Context {
	return context.WithValue(ctx, usageLimiterKey{}, limiter)
}

func usageOrigin(site string) string {
	if parsed, err := url.Parse(site); err == nil && parsed.Host != "" {
		return strings.ToLower(parsed.Scheme + "://" + parsed.Host)
	}
	return strings.ToLower(strings.TrimRight(site, "/"))
}

func BeginUsageRequest(ctx context.Context, site string) (func(), error) {
	limiter, _ := ctx.Value(usageLimiterKey{}).(*UsageLimiter)
	if limiter == nil {
		return func() {}, ctx.Err()
	}
	origin := usageOrigin(site)
	limiter.mu.Lock()
	slot := limiter.locks[origin]
	if slot == nil {
		slot = make(chan struct{}, 1)
		limiter.locks[origin] = slot
	}
	limiter.mu.Unlock()
	select {
	case slot <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err := WaitUsageRequest(ctx, site); err != nil {
		<-slot
		return nil, err
	}
	var once sync.Once
	return func() { once.Do(func() { <-slot }) }, nil
}

func WaitUsageRequest(ctx context.Context, site string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	limiter, _ := ctx.Value(usageLimiterKey{}).(*UsageLimiter)
	if limiter == nil {
		return nil
	}
	origin := usageOrigin(site)
	for {
		limiter.mu.Lock()
		state := limiter.sites[origin]
		now := time.Now()
		if now.Before(state.blockedUntil) {
			limiter.mu.Unlock()
			return &UsagePauseError{Until: state.blockedUntil, Status: state.status}
		}
		if limiter.window > 0 {
			first := 0
			for first < len(state.requests) && !state.requests[first].After(now.Add(-limiter.window)) {
				first++
			}
			state.requests = state.requests[first:]
		}
		scheduled := state.next
		if scheduled.Before(now) {
			scheduled = now
		}
		budget := limiter.budget
		for step := 0; step < state.rejections && budget > 1; step++ {
			budget = (budget + 1) / 2
		}
		if budget > 0 && len(state.requests) >= budget {
			available := state.requests[0].Add(limiter.window)
			if available.After(scheduled) {
				scheduled = available
			}
		}
		if !scheduled.After(now) {
			state.next = now.Add(limiter.interval)
			state.waitingUntil = time.Time{}
			if limiter.budget > 0 {
				state.requests = append(state.requests, now)
			}
			limiter.sites[origin] = state
			limiter.mu.Unlock()
			return ctx.Err()
		}
		state.waitingUntil = scheduled
		limiter.sites[origin] = state
		limiter.mu.Unlock()
		timer := time.NewTimer(time.Until(scheduled))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func ObserveUsageResponse(ctx context.Context, site string, status int, retryAfter ...string) {
	if status != 403 && status != 429 {
		return
	}
	limiter, _ := ctx.Value(usageLimiterKey{}).(*UsageLimiter)
	if limiter == nil {
		return
	}
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	origin := usageOrigin(site)
	state := limiter.sites[origin]
	state.blockedUntil = time.Now().Add(limiter.cooldown)
	state.status = status
	if status == 429 && state.rejections < 8 {
		state.rejections++
	}
	if len(retryAfter) > 0 {
		if seconds, err := strconv.Atoi(retryAfter[0]); err == nil && seconds > 0 {
			state.blockedUntil = time.Now().Add(time.Duration(seconds) * time.Second)
		} else if until, err := http.ParseTime(retryAfter[0]); err == nil && until.After(time.Now()) {
			state.blockedUntil = until
		}
	}
	limiter.sites[origin] = state
}
