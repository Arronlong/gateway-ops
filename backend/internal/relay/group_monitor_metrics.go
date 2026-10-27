package relay

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

type PublicGroupRecentMetrics struct {
	WindowHours         int        `json:"window_hours"`
	SampleLimit         int        `json:"sample_limit"`
	SampleCount         int        `json:"sample_count"`
	LatestAt            *time.Time `json:"latest_at"`
	CacheHitRate        *float64   `json:"cache_hit_rate"`
	SuccessRate         *float64   `json:"success_rate"`
	TPS                 *float64   `json:"tps"`
	TPSScore            *float64   `json:"tps_score"`
	SuccessCount        int        `json:"success_count"`
	SuccessAttemptCount int        `json:"success_attempt_count"`
	CacheInputTokens    int64      `json:"cache_input_tokens"`
	CacheReadTokens     int64      `json:"cache_read_tokens"`
	CacheCreationTokens int64      `json:"cache_creation_tokens"`
	TPSOutputTokens     int64      `json:"tps_output_tokens"`
	TPSDurationSeconds  float64    `json:"tps_duration_seconds"`
	TPSModels           []string   `json:"tps_models"`
}

type groupMetricSample struct {
	usage     remoteUsage
	createdAt time.Time
	success   bool
}

var metricExcludedRequest = regexp.MustCompile(`(^|[_-])(auxiliary|probe|cyber)($|[_-])`)
var metricImageModel = regexp.MustCompile(`grok-imagine|gpt-image|dall-e|imagen|seedream|image-generation`)

func (s *Service) groupMetricsSettings() (int, int, error) {
	hours, limit := 1, 60
	if s.settings != nil {
		for key, target := range map[string]*int{storage.SettingUIRecentMetricsWindowHours: &hours, storage.SettingUIRecentMetricsLimit: &limit} {
			raw, exists, err := s.settings.Get(key)
			if err != nil {
				return 0, 0, err
			}
			if parsed, err := strconv.Atoi(raw); exists && err == nil {
				*target = parsed
			}
		}
	}
	if hours < 1 {
		hours = 1
	}
	if limit < 1 {
		limit = 60
	}
	return min(hours, 24), min(limit, 200), nil
}

func groupMetricUsageSamples(items []remoteUsage) []groupMetricSample {
	samples := make([]groupMetricSample, 0, len(items))
	seen := map[int64]bool{}
	for _, item := range items {
		created, ok := parseMonitorTime(item.CreatedAt)
		if !ok || (item.ID > 0 && seen[item.ID]) {
			continue
		}
		if item.ID > 0 {
			seen[item.ID] = true
		}
		if strings.TrimSpace(item.Model) == "" {
			item.Model = item.RequestedModel
		}
		success := item.StatusCode == 0 || (item.StatusCode >= 200 && item.StatusCode < 300)
		samples = append(samples, groupMetricSample{usage: item, createdAt: created, success: success})
	}
	return samples
}

func groupMetricErrorSamples(items []remoteRequestError, groupID int64) []groupMetricSample {
	samples := make([]groupMetricSample, 0, len(items))
	seen := map[int64]bool{}
	for _, item := range items {
		if item.GroupID != 0 && item.GroupID != groupID {
			continue
		}
		owner := strings.ToLower(strings.TrimSpace(item.ErrorOwner))
		phase := strings.ToLower(strings.TrimSpace(item.ErrorPhase))
		if phase == "" {
			phase = strings.ToLower(strings.TrimSpace(item.Phase))
		}
		if owner == "client" || (owner != "provider" && phase != "upstream") || item.StatusCode < 400 || item.StatusCode >= 600 || item.StatusCode == 499 {
			continue
		}
		created, ok := parseMonitorTime(item.CreatedAt)
		if !ok || (item.ID > 0 && seen[item.ID]) {
			continue
		}
		if item.ID > 0 {
			seen[item.ID] = true
		}
		model := strings.TrimSpace(item.RequestedModel)
		if model == "" {
			model = item.Model
		}
		if model == "" {
			model = item.UpstreamModel
		}
		usage := remoteUsage{Model: model, RequestType: string(item.RequestType), StatusCode: item.StatusCode, IsAuxiliary: item.IsAuxiliary, IsProbe: item.IsProbe, IsCyber: item.IsCyber}
		if item.DurationMS != nil {
			usage.DurationMS = *item.DurationMS
		}
		samples = append(samples, groupMetricSample{usage: usage, createdAt: created})
	}
	return samples
}

func buildGroupRecentMetrics(samples []groupMetricSample, now time.Time, hours, limit int, complete bool) PublicGroupRecentMetrics {
	hours = max(1, min(hours, 24))
	if limit < 1 {
		limit = 60
	}
	limit = min(limit, 200)
	result := PublicGroupRecentMetrics{WindowHours: hours, SampleLimit: limit, TPSModels: []string{}}
	if !complete {
		return result
	}
	cutoff := now.Add(-45 * time.Second)
	start := cutoff.Add(-time.Duration(hours) * time.Hour)
	selected := make([]groupMetricSample, 0, len(samples))
	for _, sample := range samples {
		if !sample.createdAt.Before(start) && !sample.createdAt.After(cutoff) {
			selected = append(selected, sample)
		}
	}
	sort.SliceStable(selected, func(i, j int) bool { return selected[i].createdAt.After(selected[j].createdAt) })
	if len(selected) > limit {
		selected = selected[:limit]
	}
	result.SampleCount = len(selected)
	models := map[string]bool{}
	for _, sample := range selected {
		if result.LatestAt == nil {
			at := sample.createdAt
			result.LatestAt = &at
		}
		usage := sample.usage
		result.CacheInputTokens += max(0, usage.InputTokens)
		result.CacheReadTokens += max(0, usage.CacheReadTokens)
		creation := usage.CacheCreationTokens
		if creation == 0 {
			creation = max(0, usage.CacheCreation5mTokens) + max(0, usage.CacheCreation1hTokens)
		}
		result.CacheCreationTokens += max(0, creation)
		requestType := strings.ToLower(strings.TrimSpace(usage.RequestType))
		excluded := usage.IsAuxiliary || usage.IsProbe || usage.IsCyber || metricExcludedRequest.MatchString(requestType)
		if excluded {
			continue
		}
		progress := usage.InProgress || usage.AttemptStatus == "in_progress"
		result.SuccessAttemptCount++
		if sample.success && !progress {
			result.SuccessCount++
		}
		model := strings.ToLower(strings.TrimSpace(usage.Model))
		if progress || strings.Contains(requestType, "image") || metricImageModel.MatchString(model) || strings.HasPrefix(model, "flux-") || usage.OutputTokens < 0 || usage.DurationMS <= 0 {
			continue
		}
		result.TPSOutputTokens += usage.OutputTokens
		result.TPSDurationSeconds += float64(usage.DurationMS) / 1000
		name := strings.TrimSpace(usage.Model)
		if name != "" && !models[name] {
			models[name] = true
			result.TPSModels = append(result.TPSModels, name)
		}
	}
	tokens := result.CacheInputTokens + result.CacheReadTokens + result.CacheCreationTokens
	if tokens > 0 {
		rate := float64(result.CacheReadTokens) / float64(tokens)
		result.CacheHitRate = &rate
	}
	if result.SuccessAttemptCount > 0 {
		rate := float64(result.SuccessCount) / float64(result.SuccessAttemptCount)
		result.SuccessRate = &rate
	}
	if result.TPSDurationSeconds > 0 {
		tps := float64(result.TPSOutputTokens) / result.TPSDurationSeconds
		score := min(100, tps*2)
		result.TPS = &tps
		result.TPSScore = &score
	}
	return result
}
