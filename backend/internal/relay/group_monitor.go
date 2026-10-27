package relay

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

const (
	groupMonitorLimit    = 60
	groupMonitorPageSize = 200
	groupMonitorCacheTTL = 15 * time.Second
	groupMonitorWorkers  = 6
	groupMonitorMaxPages = 1000
)

// PublicGroupMonitor is deliberately a small, non-sensitive projection of the
// relay station. It contains no users, accounts, API keys, IPs, or raw errors.
type PublicGroupMonitor struct {
	StationID   uint                      `json:"station_id"`
	StationName string                    `json:"station_name"`
	Range       string                    `json:"range"`
	UpdatedAt   time.Time                 `json:"updated_at"`
	Groups      []PublicGroupMonitorGroup `json:"groups"`
	Summary     PublicGroupMonitorSummary `json:"summary"`
}

type PublicGroupMonitorSummary struct {
	Total       int `json:"total"`
	Available   int `json:"available"`
	Degraded    int `json:"degraded"`
	Unavailable int `json:"unavailable"`
	Idle        int `json:"idle"`
	Disabled    int `json:"disabled"`
	Unknown     int `json:"unknown"`
}

type PublicGroupMonitorGroup struct {
	ExternalID          int64                             `json:"external_id"`
	Name                string                            `json:"name"`
	Description         string                            `json:"description,omitempty"`
	Platform            string                            `json:"platform,omitempty"`
	RateMultiplier      float64                           `json:"rate_multiplier"`
	Enabled             bool                              `json:"enabled"`
	GroupStatus         string                            `json:"group_status,omitempty"`
	Status              string                            `json:"status"`
	DataComplete        bool                              `json:"data_complete"`
	UpdatedAt           time.Time                         `json:"updated_at"`
	LatestCallAt        *time.Time                        `json:"latest_call_at,omitempty"`
	SuccessCount        int                               `json:"success_count"`
	FailureCount        int                               `json:"failure_count"`
	ConsecutiveFailures int                               `json:"consecutive_failures"`
	FailureSummary      string                            `json:"failure_summary,omitempty"`
	Calls               []PublicGroupMonitorCall          `json:"calls"`
	LatencyBuckets      []PublicGroupMonitorLatencyBucket `json:"latency_buckets"`
	RecentMetrics       PublicGroupRecentMetrics          `json:"recent_metrics"`
}

type PublicGroupMonitorCall struct {
	Success      bool      `json:"success"`
	CreatedAt    time.Time `json:"created_at"`
	Model        string    `json:"model,omitempty"`
	FirstTokenMS *int64    `json:"first_token_ms,omitempty"`
	DurationMS   *int64    `json:"duration_ms,omitempty"`
}

type PublicGroupMonitorLatencyBucket struct {
	Start           time.Time `json:"start"`
	End             time.Time `json:"end"`
	FirstTokenP50MS *int64    `json:"first_token_p50_ms"`
	FirstTokenP95MS *int64    `json:"first_token_p95_ms"`
	DurationP50MS   *int64    `json:"duration_p50_ms"`
	DurationP95MS   *int64    `json:"duration_p95_ms"`
	SuccessRate     *float64  `json:"success_rate"`
	RequestCount    int       `json:"request_count"`
}

type publicMonitorCacheKey struct {
	stationID    uint
	rangeName    string
	metricsHours int
	metricsLimit int
}

type publicMonitorCacheEntry struct {
	ExpiresAt time.Time
	View      PublicGroupMonitor
}

type monitorEvent struct {
	success         bool
	createdAt       time.Time
	model           string
	requestID       string
	clientRequestID string
	firstTokenMS    *int64
	durationMS      *int64
	failureReason   string
}

type monitorModelStats struct {
	failures  int
	successes int
}

const (
	monitorFailureModelUnavailable = "模型当前不可用"
	monitorFailureUpstream         = "上游响应异常"
)

type remoteRequestError struct {
	ID                 int64  `json:"id"`
	RequestType        string `json:"request_type"`
	IsAuxiliary        bool   `json:"is_auxiliary"`
	IsProbe            bool   `json:"is_probe"`
	IsCyber            bool   `json:"is_cyber"`
	RequestID          string `json:"request_id"`
	ClientRequestID    string `json:"client_request_id"`
	CreatedAt          string `json:"created_at"`
	ErrorPhase         string `json:"error_phase"`
	Phase              string `json:"phase"`
	Type               string `json:"type"`
	Message            string `json:"message"`
	ErrorOwner         string `json:"error_owner"`
	StatusCode         int    `json:"status_code"`
	Platform           string `json:"platform"`
	Model              string `json:"model"`
	RequestedModel     string `json:"requested_model"`
	UpstreamModel      string `json:"upstream_model"`
	GroupID            int64  `json:"group_id"`
	DurationMS         *int64 `json:"duration_ms"`
	TimeToFirstTokenMS *int64 `json:"time_to_first_token_ms"`
}

type remoteRequestErrorPage struct {
	Items    []remoteRequestError `json:"items"`
	Total    int                  `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
	Pages    int                  `json:"pages"`
}

type monitorFetchKind uint8

const (
	monitorFetchUsage monitorFetchKind = iota
	monitorFetchErrors
)

type monitorFetchResult struct {
	index   int
	kind    monitorFetchKind
	usage   []monitorEvent
	errors  []monitorEvent
	err     error
	samples []groupMetricSample
}

func (s *Service) invalidatePublicMonitor(stationID uint) {
	s.publicMonitorMu.Lock()
	for key := range s.publicMonitorCache {
		if key.stationID == stationID {
			delete(s.publicMonitorCache, key)
		}
	}
	s.publicMonitorMu.Unlock()
}

func publicGroupMonitorWindowAt(now time.Time, rawRange string) (string, time.Time, time.Time) {
	rangeName := strings.ToLower(strings.TrimSpace(rawRange))
	switch rangeName {
	case "today", "24h", "7d", "30d":
	default:
		rangeName = "24h"
	}
	since, until := dailyConsumptionWindowAt(now, rangeName)
	return rangeName, since.UTC(), until.UTC()
}

func groupMonitorEndpoint(baseURL string, groupID int64, kind monitorFetchKind, since, until time.Time) string {
	return groupMonitorEndpointForPage(baseURL, groupID, kind, since, until, 1)
}

func groupMonitorEndpointForPage(baseURL string, groupID int64, kind monitorFetchKind, since, until time.Time, page int) string {
	if page < 1 {
		page = 1
	}
	query := url.Values{
		"page":      {strconv.Itoa(page)},
		"page_size": {strconv.Itoa(groupMonitorPageSize)},
		"group_id":  {strconv.FormatInt(groupID, 10)},
	}
	path := "/api/v1/admin/usage"
	if kind == monitorFetchUsage {
		setUsageDateRange(query, since, until)
	} else {
		path = "/api/v1/admin/ops/request-errors"
		query.Set("start_time", since.UTC().Format(time.RFC3339Nano))
		query.Set("end_time", until.UTC().Format(time.RFC3339Nano))
	}
	return strings.TrimRight(baseURL, "/") + path + "?" + query.Encode()
}

func fetchGroupMonitorUsageEvents(ctx context.Context, client *Service, baseURL, apiKey string, groupID int64, since, until time.Time) ([]monitorEvent, error) {
	items, err := fetchGroupMonitorUsagePages(ctx, client, baseURL, apiKey, groupID, since, until)
	if err != nil {
		return nil, err
	}
	return usageMonitorEvents(items), nil
}

func fetchGroupMonitorErrorEvents(ctx context.Context, client *Service, baseURL, apiKey string, groupID int64, since, until time.Time) ([]monitorEvent, error) {
	items, err := fetchGroupMonitorErrorPages(ctx, client, baseURL, apiKey, groupID, since, until)
	if err != nil {
		return nil, err
	}
	return errorMonitorEvents(items, groupID), nil
}

func groupMonitorPageCount(total, pages int) int {
	if pages > 0 {
		return pages
	}
	if total <= 0 {
		return 0
	}
	return (total + groupMonitorPageSize - 1) / groupMonitorPageSize
}

func fetchGroupMonitorUsagePages(ctx context.Context, client *Service, baseURL, apiKey string, groupID int64, since, until time.Time) ([]remoteUsage, error) {
	var first remoteUsagePage
	if err := client.get(ctx, groupMonitorEndpointForPage(baseURL, groupID, monitorFetchUsage, since, until, 1), apiKey, &first); err != nil {
		return nil, err
	}
	items := append([]remoteUsage(nil), first.Items...)
	if len(first.Items) < groupMonitorPageSize {
		return items, nil
	}
	pageCount := max(1, min(groupMonitorPageCount(first.Total, first.Pages), groupMonitorMaxPages))
	pages, err := fetchGroupMonitorUsagePagesConcurrent(ctx, client, baseURL, apiKey, groupID, since, until, 2, pageCount)
	if err != nil {
		return nil, err
	}
	items = append(items, pages...)
	if len(pages) < (pageCount-1)*groupMonitorPageSize {
		return items, nil
	}
	// 上游页数可能是估算值，末页满页时继续读取，直到返回不足一页的数据。
	for page := pageCount + 1; page <= groupMonitorMaxPages; page++ {
		var response remoteUsagePage
		if err := client.get(ctx, groupMonitorEndpointForPage(baseURL, groupID, monitorFetchUsage, since, until, page), apiKey, &response); err != nil {
			return nil, err
		}
		items = append(items, response.Items...)
		if len(response.Items) < groupMonitorPageSize {
			return items, nil
		}
	}
	return nil, fmt.Errorf("group monitor usage exceeded pagination limit (%d pages)", groupMonitorMaxPages)
}

func fetchGroupMonitorUsagePagesConcurrent(ctx context.Context, client *Service, baseURL, apiKey string, groupID int64, since, until time.Time, startPage, endPage int) ([]remoteUsage, error) {
	if startPage > endPage {
		return nil, nil
	}
	type result struct {
		page  int
		items []remoteUsage
		err   error
	}
	pageCount := endPage - startPage + 1
	workerCount := groupMonitorWorkers
	if workerCount > pageCount {
		workerCount = pageCount
	}
	jobs := make(chan int, pageCount)
	results := make(chan result, pageCount)
	var workers sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for page := range jobs {
				var response remoteUsagePage
				err := client.get(ctx, groupMonitorEndpointForPage(baseURL, groupID, monitorFetchUsage, since, until, page), apiKey, &response)
				results <- result{page: page, items: response.Items, err: err}
			}
		}()
	}
	for page := startPage; page <= endPage; page++ {
		jobs <- page
	}
	close(jobs)
	workers.Wait()
	close(results)
	byPage := make(map[int][]remoteUsage, pageCount)
	for response := range results {
		if response.err != nil {
			return nil, response.err
		}
		byPage[response.page] = response.items
	}
	items := make([]remoteUsage, 0, pageCount*groupMonitorPageSize)
	for page := startPage; page <= endPage; page++ {
		pageItems := byPage[page]
		items = append(items, pageItems...)
		if len(pageItems) < groupMonitorPageSize {
			break
		}
	}
	return items, nil
}

func fetchGroupMonitorErrorPages(ctx context.Context, client *Service, baseURL, apiKey string, groupID int64, since, until time.Time) ([]remoteRequestError, error) {
	var first remoteRequestErrorPage
	if err := client.get(ctx, groupMonitorEndpointForPage(baseURL, groupID, monitorFetchErrors, since, until, 1), apiKey, &first); err != nil {
		return nil, err
	}
	items := append([]remoteRequestError(nil), first.Items...)
	if len(first.Items) < groupMonitorPageSize {
		return items, nil
	}
	for page := 2; page <= groupMonitorMaxPages; page++ {
		var response remoteRequestErrorPage
		if err := client.get(ctx, groupMonitorEndpointForPage(baseURL, groupID, monitorFetchErrors, since, until, page), apiKey, &response); err != nil {
			return nil, err
		}
		items = append(items, response.Items...)
		if len(response.Items) < groupMonitorPageSize {
			return items, nil
		}
	}
	return nil, fmt.Errorf("group monitor errors exceeded pagination limit (%d pages)", groupMonitorMaxPages)
}

// PublicGroupMonitor aggregates the latest successful calls and provider-side
// failures for monitored groups. Upstream failures are retained as events so a
// group that stopped working is visible even when its last successful call is
// old. The short cache prevents a public page refresh from fanning out to the
// remote admin API on every request.
func (s *Service) PublicGroupMonitor(ctx context.Context, stationID uint) (PublicGroupMonitor, error) {
	return s.PublicGroupMonitorForRange(ctx, stationID, "24h")
}

// 按时间范围读取最近调用，日历区间与渠道管理使用相同的本地时区。
func (s *Service) PublicGroupMonitorForRange(ctx context.Context, stationID uint, rawRange string) (PublicGroupMonitor, error) {
	rangeName, since, now := publicGroupMonitorWindowAt(time.Now(), rawRange)
	metricsHours, metricsLimit, err := s.groupMetricsSettings()
	if err != nil {
		return PublicGroupMonitor{}, err
	}
	cacheKey := publicMonitorCacheKey{stationID: stationID, rangeName: rangeName, metricsHours: metricsHours, metricsLimit: metricsLimit}
	metricsSince := now.Add(-45 * time.Second).Add(-time.Duration(metricsHours) * time.Hour)
	fetchSince := since
	if metricsSince.Before(fetchSince) {
		fetchSince = metricsSince
	}
	s.publicMonitorMu.Lock()
	if cached, ok := s.publicMonitorCache[cacheKey]; ok && now.Before(cached.ExpiresAt) {
		s.publicMonitorMu.Unlock()
		return cached.View, nil
	}
	s.publicMonitorMu.Unlock()

	station, err := s.stations.FindByID(stationID)
	if err != nil {
		return PublicGroupMonitor{}, err
	}
	groups, err := s.stations.ListGroups(stationID)
	if err != nil {
		return PublicGroupMonitor{}, err
	}
	view := PublicGroupMonitor{StationID: stationID, StationName: station.Name, Range: rangeName, UpdatedAt: now, Groups: []PublicGroupMonitorGroup{}}
	monitored := make([]int, 0, len(groups))
	for index := range groups {
		if groups[index].MonitorEnabled {
			monitored = append(monitored, index)
		}
	}
	if len(monitored) == 0 {
		view.Summary.Total = 0
		s.storePublicMonitor(cacheKey, view, now)
		return view, nil
	}

	apiKey, err := s.cipher.Decrypt(station.APIKeyCipher)
	if err != nil {
		return PublicGroupMonitor{}, fmt.Errorf("decrypt admin API key: %w", err)
	}

	results := make(chan monitorFetchResult, len(monitored)*2)
	jobs := make(chan struct {
		index int
		kind  monitorFetchKind
	}, len(monitored)*2)
	workerCount := groupMonitorWorkers
	if workerCount > len(monitored)*2 {
		workerCount = len(monitored) * 2
	}
	var workers sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for job := range jobs {
				group := groups[monitored[job.index]]
				result := monitorFetchResult{index: job.index, kind: job.kind}
				if job.kind == monitorFetchUsage {
					var items []remoteUsage
					items, result.err = fetchGroupMonitorUsagePages(ctx, s, station.BaseURL, apiKey, group.ExternalID, fetchSince, now)
					result.usage = usageMonitorEvents(items)
					result.samples = groupMetricUsageSamples(items)
				} else {
					var items []remoteRequestError
					items, result.err = fetchGroupMonitorErrorPages(ctx, s, station.BaseURL, apiKey, group.ExternalID, fetchSince, now)
					result.errors = errorMonitorEvents(items, group.ExternalID)
					result.samples = groupMetricErrorSamples(items, group.ExternalID)
				}
				results <- result
			}
		}()
	}
	for index := range monitored {
		jobs <- struct {
			index int
			kind  monitorFetchKind
		}{index: index, kind: monitorFetchUsage}
		jobs <- struct {
			index int
			kind  monitorFetchKind
		}{index: index, kind: monitorFetchErrors}
	}
	close(jobs)
	workers.Wait()
	close(results)

	perGroup := make([]struct {
		usage, errors []monitorEvent
		samples       []groupMetricSample
		usageOK       bool
		errorsOK      bool
	}, len(monitored))
	for result := range results {
		perGroup[result.index].samples = append(perGroup[result.index].samples, result.samples...)
		if result.kind == monitorFetchUsage {
			perGroup[result.index].usageOK = result.err == nil
			perGroup[result.index].usage = result.usage
		} else {
			perGroup[result.index].errorsOK = result.err == nil
			perGroup[result.index].errors = result.errors
		}
	}
	for index, groupIndex := range monitored {
		group := groups[groupIndex]
		item := buildPublicGroupMonitorGroupForWindow(group, perGroup[index].usage, perGroup[index].errors, perGroup[index].usageOK && perGroup[index].errorsOK, now, since, now)
		item.RecentMetrics = buildGroupRecentMetrics(perGroup[index].samples, now, metricsHours, metricsLimit, perGroup[index].usageOK && perGroup[index].errorsOK)
		view.Groups = append(view.Groups, item)
		switch item.Status {
		case "available":
			view.Summary.Available++
		case "degraded":
			view.Summary.Degraded++
		case "unavailable":
			view.Summary.Unavailable++
		case "idle":
			view.Summary.Idle++
		case "disabled":
			view.Summary.Disabled++
		case "unknown":
			view.Summary.Unknown++
		}
	}
	view.Summary.Total = len(view.Groups)
	s.storePublicMonitor(cacheKey, view, now)
	return view, nil
}

func (s *Service) storePublicMonitor(key publicMonitorCacheKey, view PublicGroupMonitor, now time.Time) {
	s.publicMonitorMu.Lock()
	s.publicMonitorCache[key] = publicMonitorCacheEntry{ExpiresAt: now.Add(groupMonitorCacheTTL), View: view}
	s.publicMonitorMu.Unlock()
}

func usageMonitorEvents(items []remoteUsage) []monitorEvent {
	events := make([]monitorEvent, 0, len(items))
	for _, item := range items {
		created, ok := parseMonitorTime(item.CreatedAt)
		if !ok {
			continue
		}
		model := strings.TrimSpace(item.RequestedModel)
		if model == "" {
			model = strings.TrimSpace(item.Model)
		}
		first, duration := item.FirstTokenMS, item.DurationMS
		events = append(events, monitorEvent{success: true, createdAt: created, model: model, requestID: strings.TrimSpace(item.RequestID), clientRequestID: strings.TrimSpace(item.ClientRequestID), firstTokenMS: &first, durationMS: &duration})
	}
	return events
}

func errorMonitorEvents(items []remoteRequestError, groupID int64) []monitorEvent {
	events := make([]monitorEvent, 0, len(items))
	for _, item := range items {
		if item.GroupID != 0 && item.GroupID != groupID {
			continue
		}
		owner := strings.ToLower(strings.TrimSpace(item.ErrorOwner))
		phase := strings.ToLower(strings.TrimSpace(item.ErrorPhase))
		if phase == "" {
			phase = strings.ToLower(strings.TrimSpace(item.Phase))
		}
		if owner == "client" || (owner != "provider" && phase != "upstream") {
			continue
		}
		// The admin endpoint exposes the upstream status, not the final gateway
		// status. Its message still preserves the request outcome: failover rows
		// say "Recovered upstream error ...", while a client disconnect (499)
		// has an empty message. Neither is a user-visible group failure.
		message := strings.TrimSpace(item.Message)
		if message == "" || strings.HasPrefix(strings.ToLower(message), "recovered upstream error") {
			continue
		}
		if item.StatusCode < http.StatusBadRequest || item.StatusCode >= 600 || item.StatusCode == 499 {
			continue
		}
		created, ok := parseMonitorTime(item.CreatedAt)
		if !ok {
			continue
		}
		model := strings.TrimSpace(item.RequestedModel)
		if model == "" {
			model = strings.TrimSpace(item.Model)
		}
		if model == "" {
			model = strings.TrimSpace(item.UpstreamModel)
		}
		events = append(events, monitorEvent{createdAt: created, model: model, requestID: strings.TrimSpace(item.RequestID), clientRequestID: strings.TrimSpace(item.ClientRequestID), firstTokenMS: item.TimeToFirstTokenMS, durationMS: item.DurationMS, failureReason: monitorFailureReason(item)})
	}
	return events
}

func monitorFailureReason(item remoteRequestError) string {
	detail := strings.ToLower(strings.Join([]string{item.Type, item.Message}, " "))
	containsAny := func(values ...string) bool {
		for _, value := range values {
			if strings.Contains(detail, value) {
				return true
			}
		}
		return false
	}
	switch {
	case containsAny("model_not_found", "model_not_supported", "model not found", "unsupported model", "model is not supported", "model does not exist", "does not exist or you do not have access", "不支持该模型", "未配置模型", "no available account"):
		return monitorFailureModelUnavailable
	case containsAny("insufficient", "quota", "credit", "balance", "额度不足", "余额不足"):
		return "可用额度不足"
	case item.StatusCode == http.StatusTooManyRequests || containsAny("rate limit", "too many request", "concurrency", "并发", "频率限制"):
		return "请求频率或并发已达上限"
	case item.StatusCode == http.StatusUnauthorized || item.StatusCode == http.StatusForbidden || containsAny("unauthorized", "forbidden", "invalid api key", "authentication", "permission", "凭证", "权限"):
		return "凭证或权限异常"
	case item.StatusCode == http.StatusBadRequest || item.StatusCode == http.StatusUnprocessableEntity || containsAny("invalid parameter", "invalid request", "bad request", "参数错误"):
		return "请求参数不符合要求"
	case item.StatusCode >= http.StatusInternalServerError || containsAny("timeout", "timed out", "temporarily unavailable", "connection", "network", "服务不可用", "超时"):
		return monitorFailureUpstream
	default:
		return "请求未能完成"
	}
}

func parseMonitorTime(raw string) (time.Time, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return parsed, err == nil
}

func monitorEventRequestKeys(event monitorEvent) []string {
	keys := make([]string, 0, 2)
	add := func(raw, kind string) {
		value := strings.TrimSpace(raw)
		if value == "" {
			return
		}
		key := "request:" + value
		if kind == "client" || strings.HasPrefix(strings.ToLower(value), "client:") {
			if strings.HasPrefix(strings.ToLower(value), "client:") {
				value = strings.TrimSpace(value[len("client:"):])
			}
			key = "client:" + value
		}
		for _, existing := range keys {
			if existing == key {
				return
			}
		}
		keys = append(keys, key)
	}
	add(event.requestID, "request")
	add(event.clientRequestID, "client")
	return keys
}

func monitorEventRequestGroupKey(keys []string) string {
	for _, key := range keys {
		if strings.HasPrefix(key, "client:") {
			return key
		}
	}
	if len(keys) > 0 {
		return keys[0]
	}
	return ""
}

// filterRecoveredMonitorFailures removes failover attempts that belong to a
// request which eventually succeeded, and collapses multiple error rows for a
// request that did end in failure to its last recorded error.
func filterRecoveredMonitorFailures(usage, failures []monitorEvent) []monitorEvent {
	completed := make(map[string]struct{}, len(usage))
	for _, event := range usage {
		for _, key := range monitorEventRequestKeys(event) {
			completed[key] = struct{}{}
		}
	}
	latest := make(map[string]monitorEvent, len(failures))
	uncorrelated := make([]monitorEvent, 0, len(failures))
	for _, event := range failures {
		keys := monitorEventRequestKeys(event)
		if len(keys) == 0 {
			uncorrelated = append(uncorrelated, event)
			continue
		}
		recovered := false
		for _, key := range keys {
			if _, ok := completed[key]; ok {
				recovered = true
				break
			}
		}
		if recovered {
			continue
		}
		key := monitorEventRequestGroupKey(keys)
		if previous, ok := latest[key]; !ok || event.createdAt.After(previous.createdAt) {
			latest[key] = event
		}
	}
	filtered := make([]monitorEvent, 0, len(uncorrelated)+len(latest))
	filtered = append(filtered, uncorrelated...)
	for _, event := range latest {
		filtered = append(filtered, event)
	}
	return filtered
}

func buildPublicGroupMonitorGroup(group storage.RelayGroup, usage, failures []monitorEvent, complete bool, now time.Time) PublicGroupMonitorGroup {
	return buildPublicGroupMonitorGroupForWindow(group, usage, failures, complete, now, now.Add(-24*time.Hour), now.Add(time.Nanosecond))
}

func buildPublicGroupMonitorGroupForWindow(group storage.RelayGroup, usage, failures []monitorEvent, complete bool, now, since, until time.Time) PublicGroupMonitorGroup {
	failures = filterRecoveredMonitorFailures(usage, failures)
	events := append(append([]monitorEvent{}, usage...), failures...)
	filtered := events[:0]
	for _, event := range events {
		if !event.createdAt.Before(since) && event.createdAt.Before(until) {
			filtered = append(filtered, event)
		}
	}
	events = filtered
	sort.SliceStable(events, func(i, j int) bool { return events[i].createdAt.After(events[j].createdAt) })
	recentEvents := events
	if len(recentEvents) > groupMonitorLimit {
		recentEvents = recentEvents[:groupMonitorLimit]
	}
	successCount := 0
	failureCount := 0
	failureReasons := make(map[string]int)
	failureReasonOrder := make([]string, 0)
	for _, event := range events {
		if event.success {
			successCount++
		} else {
			failureCount++
			reason := strings.TrimSpace(event.failureReason)
			if reason == "" {
				reason = "请求未能完成"
			}
			if failureReasons[reason] == 0 {
				failureReasonOrder = append(failureReasonOrder, reason)
			}
			failureReasons[reason]++
		}
	}
	failureSummary := buildFailureSummary(events, failureReasons, failureReasonOrder)
	status := classifyGroupMonitor(group.Status, complete, events)
	consecutiveFailures := 0
	for _, event := range events {
		if event.success {
			break
		}
		consecutiveFailures++
	}
	var latest *time.Time
	if len(events) > 0 {
		value := events[0].createdAt
		latest = &value
	}
	calls := make([]PublicGroupMonitorCall, 0, len(recentEvents))
	for _, event := range recentEvents {
		calls = append(calls, PublicGroupMonitorCall{Success: event.success, CreatedAt: event.createdAt, Model: event.model, FirstTokenMS: event.firstTokenMS, DurationMS: event.durationMS})
	}
	latencyBuckets := buildPublicGroupMonitorLatencyBuckets(events, since, until, groupMonitorLimit)
	return PublicGroupMonitorGroup{ExternalID: group.ExternalID, Name: group.Name, Description: group.Description, Platform: group.Platform, RateMultiplier: group.RateMultiplier, Enabled: group.MonitorEnabled, GroupStatus: group.Status, Status: status, DataComplete: complete, UpdatedAt: now, LatestCallAt: latest, SuccessCount: successCount, FailureCount: failureCount, ConsecutiveFailures: consecutiveFailures, FailureSummary: failureSummary, Calls: calls, LatencyBuckets: latencyBuckets}
}

func buildPublicGroupMonitorLatencyBuckets(events []monitorEvent, since, until time.Time, count int) []PublicGroupMonitorLatencyBucket {
	if count <= 0 || !until.After(since) {
		return []PublicGroupMonitorLatencyBucket{}
	}
	total := until.Sub(since)
	type bucketValues struct {
		first    []int64
		duration []int64
		success  int
		requests int
	}
	values := make([]bucketValues, count)
	for _, event := range events {
		if event.createdAt.Before(since) || !event.createdAt.Before(until) {
			continue
		}
		index := int(event.createdAt.Sub(since) * time.Duration(count) / total)
		if index < 0 {
			continue
		}
		if index >= count {
			index = count - 1
		}
		values[index].requests++
		if event.firstTokenMS != nil {
			values[index].first = append(values[index].first, *event.firstTokenMS)
		}
		if event.durationMS != nil {
			values[index].duration = append(values[index].duration, *event.durationMS)
		}
		values[index].success += boolToInt(event.success)
	}
	buckets := make([]PublicGroupMonitorLatencyBucket, count)
	for index := range buckets {
		start := since.Add(time.Duration(index) * total / time.Duration(count))
		end := since.Add(time.Duration(index+1) * total / time.Duration(count))
		requestCount := values[index].requests
		bucket := PublicGroupMonitorLatencyBucket{Start: start, End: end, RequestCount: requestCount}
		if requestCount > 0 {
			successRate := float64(values[index].success) / float64(requestCount)
			bucket.SuccessRate = &successRate
		}
		if len(values[index].first) > 0 {
			p50, p95 := percentileInt64(values[index].first, 0.50), percentileInt64(values[index].first, 0.95)
			bucket.FirstTokenP50MS, bucket.FirstTokenP95MS = &p50, &p95
		}
		if len(values[index].duration) > 0 {
			p50, p95 := percentileInt64(values[index].duration, 0.50), percentileInt64(values[index].duration, 0.95)
			bucket.DurationP50MS, bucket.DurationP95MS = &p50, &p95
		}
		buckets[index] = bucket
	}
	return buckets
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// buildFailureSummary deliberately uses only the requested model and a small
// set of safe categories. Raw provider messages, account names, credentials,
// request IDs, and internal endpoints never leave the backend.
func buildFailureSummary(events []monitorEvent, failureReasons map[string]int, failureReasonOrder []string) string {
	mainReason, mainReasonCount := mostCommonFailureReason(failureReasons, failureReasonOrder)
	modelStatsByName := make(map[string]monitorModelStats)
	modelOrder := make([]string, 0)
	for _, event := range events {
		model := strings.TrimSpace(event.model)
		if model == "" {
			continue
		}
		if _, ok := modelStatsByName[model]; !ok {
			modelOrder = append(modelOrder, model)
		}
		stats := modelStatsByName[model]
		if event.success {
			stats.successes++
		} else {
			stats.failures++
		}
		modelStatsByName[model] = stats
	}

	// An explicit model-not-available response is the strongest signal. Only
	// surface its model when it is the leading failure category, so one
	// isolated model error does not hide a larger unrelated problem.
	explicitModel, explicitCount := mostCommonUnavailableModel(events, modelOrder)
	if explicitCount > 0 && explicitCount >= mainReasonCount {
		if explicitModel != "" {
			return fmt.Sprintf("报错主要原因：%s 模型当前不可用（%d 次）", explicitModel, explicitCount)
		}
		return fmt.Sprintf("报错主要原因：请求的模型当前不可用（%d 次）", explicitCount)
	}

	// Some gateways return a generic 5xx even when every attempt for one model
	// fails. If another model succeeds in the same 60-call window, that model
	// distribution is enough to explain the user-visible problem without
	// exposing the provider's raw error text.
	totalFailures := 0
	totalSuccesses := 0
	for _, stats := range modelStatsByName {
		totalFailures += stats.failures
		totalSuccesses += stats.successes
	}
	if totalFailures >= 2 && totalSuccesses > 0 {
		candidate, candidateCount := mostCommonFailedOnlyModel(modelStatsByName, modelOrder)
		if candidate != "" && candidateCount >= 2 && candidateCount*100 >= totalFailures*60 {
			otherModelSuccess := false
			for model, stats := range modelStatsByName {
				if model != candidate && stats.successes > 0 {
					otherModelSuccess = true
					break
				}
			}
			if otherModelSuccess {
				return fmt.Sprintf("报错主要原因：%s 模型当前不可用（%d 次）", candidate, candidateCount)
			}
		}
	}

	if mainReasonCount > 0 {
		return fmt.Sprintf("报错主要原因：%s（%d 次）", mainReason, mainReasonCount)
	}
	return ""
}

func mostCommonFailureReason(counts map[string]int, order []string) (string, int) {
	mainReason := ""
	mainCount := 0
	for _, reason := range order {
		if counts[reason] > mainCount {
			mainReason = reason
			mainCount = counts[reason]
		}
	}
	return mainReason, mainCount
}

func mostCommonUnavailableModel(events []monitorEvent, order []string) (string, int) {
	counts := make(map[string]int)
	for _, event := range events {
		if event.success || strings.TrimSpace(event.failureReason) != monitorFailureModelUnavailable {
			continue
		}
		model := strings.TrimSpace(event.model)
		counts[model]++
	}
	return mostCommonModel(counts, order)
}

func mostCommonFailedOnlyModel(stats map[string]monitorModelStats, order []string) (string, int) {
	counts := make(map[string]int)
	for model, item := range stats {
		if item.failures > 0 && item.successes == 0 {
			counts[model] = item.failures
		}
	}
	return mostCommonModel(counts, order)
}

func mostCommonModel(counts map[string]int, order []string) (string, int) {
	model := ""
	count := 0
	for _, candidate := range order {
		if counts[candidate] > count {
			model = candidate
			count = counts[candidate]
		}
	}
	return model, count
}

func classifyGroupMonitor(groupStatus string, complete bool, events []monitorEvent) string {
	if !strings.EqualFold(strings.TrimSpace(groupStatus), "active") {
		return "disabled"
	}
	if !complete {
		return "unknown"
	}
	if len(events) == 0 {
		return "idle"
	}
	consecutiveFailures := 0
	for _, event := range events {
		if event.success {
			break
		}
		consecutiveFailures++
	}
	if consecutiveFailures >= 3 {
		return "unavailable"
	}
	if consecutiveFailures > 0 {
		return "degraded"
	}
	return "available"
}
