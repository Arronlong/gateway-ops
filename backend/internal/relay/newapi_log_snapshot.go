package relay

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// 成功和失败日志分别按原生页拉取，每页只读取一次；统计和记录共用相同的快照。
func (s *Service) newAPILogSnapshot(ctx context.Context, base string, credential newAPICredential, values url.Values, kind string) ([]remoteUsage, error) {
	query, err := s.newAPIReportingLogQuery(ctx, base, credential, values)
	if err != nil {
		return nil, err
	}
	query.Del("p")
	query.Del("page_size")
	query.Set("type", kind)
	cacheQuery := cloneURLValues(query)
	if end, err := strconv.ParseInt(cacheQuery.Get("end_timestamp"), 10, 64); err == nil {
		cacheQuery.Set("end_timestamp", strconv.FormatInt(end/60, 10))
	}
	encoded, _ := json.Marshal(credential)
	key := fmt.Sprintf("newapi-logs:%s:%x:%s", base, sha256.Sum256(encoded), cacheQuery.Encode())
	value, err := s.cachedReport(ctx, key, time.Minute, func(loadCtx context.Context) (any, error) {
		return s.fetchNewAPILogSnapshot(loadCtx, base, credential, query)
	})
	if err != nil {
		return nil, err
	}
	return value.([]remoteUsage), nil
}

func (s *Service) fetchNewAPILogSnapshot(ctx context.Context, base string, credential newAPICredential, values url.Values) ([]remoteUsage, error) {
	query := cloneURLValues(values)
	query.Set("page", "1")
	query.Set("page_size", "100")
	first, err := s.newAPILogs(ctx, base, credential, query)
	if err != nil {
		return nil, err
	}
	if first.Pages > 10000 {
		return nil, fmt.Errorf("New API 日志超过 10000 页，请缩小统计时间范围")
	}
	items := append([]remoteUsage(nil), first.Items...)
	if first.Pages <= 1 {
		return items, nil
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type pageResult struct {
		number int
		items  []remoteUsage
		err    error
	}
	jobs := make(chan int)
	results := make(chan pageResult)
	workers := 6
	if first.Pages-1 < workers {
		workers = first.Pages - 1
	}
	var group sync.WaitGroup
	for index := 0; index < workers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for number := range jobs {
				pageQuery := cloneURLValues(query)
				pageQuery.Set("page", strconv.Itoa(number))
				page, err := s.newAPILogs(ctx, base, credential, pageQuery)
				select {
				case results <- pageResult{number: number, items: page.Items, err: err}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for number := 2; number <= first.Pages; number++ {
			select {
			case jobs <- number:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { group.Wait(); close(results) }()
	pages := make(map[int][]remoteUsage, first.Pages-1)
	for page := range results {
		if page.err != nil {
			return nil, page.err
		}
		pages[page.number] = page.items
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for number := 2; number <= first.Pages; number++ {
		items = append(items, pages[number]...)
	}
	return items, nil
}

func (s *Service) newAPIStationUsageItems(ctx context.Context, base string, credential newAPICredential, since, until time.Time) ([]remoteUsage, error) {
	if since.IsZero() {
		since = time.Unix(0, 0).UTC()
	}
	values := url.Values{"start_time": {since.Format(time.RFC3339Nano)}, "end_time": {until.Format(time.RFC3339Nano)}}
	type result struct {
		items []remoteUsage
		err   error
	}
	results := make(chan result, 2)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for _, kind := range []string{"2", "5"} {
		go func(kind string) {
			items, err := s.newAPILogSnapshot(ctx, base, credential, values, kind)
			results <- result{items: items, err: err}
		}(kind)
	}
	items := []remoteUsage{}
	for index := 0; index < 2; index++ {
		value := <-results
		if value.err != nil {
			return nil, value.err
		}
		items = append(items, value.items...)
	}
	return filterRecentUsageRange(items, since, until), nil
}
