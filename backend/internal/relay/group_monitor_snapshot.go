package relay

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
)

var monitorSnapshotRanges = []string{"today", "24h", "7d", "30d"}

func monitorSourceKey(station *storage.RelayStation) string {
	sum := sha256.Sum256([]byte(station.Type + "\n" + station.BaseURL + "\n" + station.APIKeyCipher))
	return hex.EncodeToString(sum[:])
}

// CachedPublicGroupMonitorForRange 只读取本地快照，冷启动与失败时也不回退上游查询。
func (s *Service) CachedPublicGroupMonitorForRange(ctx context.Context, stationID uint, preset string) (PublicGroupMonitor, error) {
	switch preset {
	case "today", "24h", "7d", "30d":
	default:
		return PublicGroupMonitor{}, errors.New("unsupported monitor range")
	}
	station, err := s.stations.GroupMonitorStation(ctx, stationID)
	if err != nil {
		return PublicGroupMonitor{}, err
	}
	groups, err := s.stations.ListMonitorGroups(ctx, stationID)
	if err != nil {
		return PublicGroupMonitor{}, err
	}
	now := time.Now()
	view := PublicGroupMonitor{StationID: stationID, StationName: station.Name, Range: preset, UpdatedAt: now, Groups: []PublicGroupMonitorGroup{}, SyncStatus: "preparing"}
	row, readErr := s.stations.GetGroupMonitorSnapshot(ctx, stationID, preset)
	valid := readErr == nil && row.SourceKey == monitorSourceKey(station)
	if readErr != nil && !errors.Is(readErr, gorm.ErrRecordNotFound) {
		return view, readErr
	}
	interval := 30
	if s.settings != nil {
		interval, err = s.settings.GroupMonitorIntervalSeconds()
		if err != nil {
			return view, err
		}
	}
	if valid {
		if err := json.Unmarshal(row.Payload, &view); err != nil {
			return view, fmt.Errorf("decode monitor snapshot: %w", err)
		}
		if row.LastError != "" {
			view.SyncStatus = "stale"
			view.SyncError = row.LastError
		} else if now.Sub(row.UpdatedAt) > time.Duration(interval*3)*time.Second {
			view.SyncStatus = "stale"
		}
	}
	saved := make(map[int64]PublicGroupMonitorGroup, len(view.Groups))
	for _, group := range view.Groups {
		saved[group.ExternalID] = group
	}
	view.Groups = []PublicGroupMonitorGroup{}
	view.StationName = station.Name
	_, since, until := publicGroupMonitorWindowAt(now, preset)
	for _, group := range groups {
		if !group.MonitorEnabled {
			continue
		}
		item, exists := saved[group.ExternalID]
		if !exists {
			item = buildPublicGroupMonitorGroupForWindow(group, nil, nil, false, now, since, until)
			view.SyncStatus = "preparing"
		}
		item.Name, item.Description, item.Platform, item.RateMultiplier = group.Name, group.Description, group.Platform, group.RateMultiplier
		previousStatus := item.GroupStatus
		item.Enabled = true
		item.GroupStatus = group.Status
		if !strings.EqualFold(strings.TrimSpace(group.Status), "active") {
			item.Status = "disabled"
		} else if previousStatus != group.Status || item.Status == "disabled" {
			item.Status = "unknown"
			item.DataComplete = false
			view.SyncStatus = "preparing"
		}
		view.Groups = append(view.Groups, item)
	}
	if len(view.Groups) == 0 {
		view.SyncStatus = "ready"
		view.SyncError = ""
	}
	summarizeMonitorSnapshot(&view)
	return view, nil
}

func summarizeMonitorSnapshot(view *PublicGroupMonitor) {
	view.Summary = PublicGroupMonitorSummary{Total: len(view.Groups)}
	for _, group := range view.Groups {
		switch group.Status {
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
		default:
			view.Summary.Unknown++
		}
	}
}

type monitorDayPayload struct {
	Usage  []remoteUsage
	Errors []remoteRequestError
}
type monitorSnapshotGroupData struct {
	usage, failures []monitorEvent
	samples         []groupMetricSample
}

// RefreshPublicGroupMonitorSnapshots 由后台调度执行，与页面是否打开无关。
func (s *Service) RefreshPublicGroupMonitorSnapshots(ctx context.Context) error {
	return s.refreshGroupMonitorSnapshotsAt(ctx, time.Now())
}
func (s *Service) refreshGroupMonitorSnapshotsAt(ctx context.Context, now time.Time) error {
	stations, err := s.stations.ListGroupMonitorStations(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for index := range stations {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		station := &stations[index]
		if err := s.refreshStationGroupMonitorSnapshots(ctx, station, now); err != nil {
			_ = s.stations.SetGroupMonitorSnapshotError(ctx, station.ID, monitorSnapshotSyncError, now)
			failures = append(failures, fmt.Errorf("station %d monitor sync: %w", station.ID, err))
		}
	}
	return errors.Join(failures...)
}

const monitorSnapshotSyncError = "监控数据后台同步失败，正在重试"

func (s *Service) refreshStationGroupMonitorSnapshots(ctx context.Context, station *storage.RelayStation, now time.Time) error {
	groups, err := s.stations.ListMonitorGroups(ctx, station.ID)
	if err != nil {
		return err
	}
	monitored := make([]storage.RelayGroup, 0, len(groups))
	for _, group := range groups {
		if group.MonitorEnabled {
			monitored = append(monitored, group)
		}
	}
	if len(monitored) == 0 {
		return nil
	}
	apiKey, err := s.cipher.Decrypt(station.APIKeyCipher)
	if err != nil {
		return err
	}
	hours, limit, err := s.groupMetricsSettings()
	if err != nil {
		return err
	}
	metricsSince := now.Add(-45 * time.Second).Add(-time.Duration(hours) * time.Hour)
	_, oldest, _ := publicGroupMonitorWindowAt(now, "30d")
	if metricsSince.Before(oldest) {
		oldest = metricsSince
	}
	oldestDay := oldest.UTC().Truncate(24 * time.Hour)
	currentDay := now.UTC().Truncate(24 * time.Hour)
	data := make(map[int64]*monitorSnapshotGroupData, len(monitored))
	for _, group := range monitored {
		data[group.ExternalID] = &monitorSnapshotGroupData{}
	}
	published := map[string]bool{}
	sourceKey := monitorSourceKey(station)
	for day := currentDay; !day.Before(oldestDay); day = day.Add(-24 * time.Hour) {
		for _, group := range monitored {
			payload, err := s.loadOrRefreshMonitorDay(ctx, station, apiKey, group.ExternalID, day, now)
			if err != nil {
				return err
			}
			aggregate := data[group.ExternalID]
			aggregate.usage = append(aggregate.usage, usageMonitorEvents(payload.Usage)...)
			aggregate.failures = append(aggregate.failures, errorMonitorEvents(payload.Errors, group.ExternalID)...)
			for _, sample := range append(groupMetricUsageSamples(payload.Usage), groupMetricErrorSamples(payload.Errors, group.ExternalID)...) {
				if !sample.createdAt.Before(metricsSince) {
					aggregate.samples = append(aggregate.samples, sample)
				}
			}
		}
		for _, preset := range monitorSnapshotRanges {
			if published[preset] {
				continue
			}
			_, since, until := publicGroupMonitorWindowAt(now, preset)
			required := since
			if metricsSince.Before(required) {
				required = metricsSince
			}
			if day.After(required.UTC().Truncate(24 * time.Hour)) {
				continue
			}
			view := PublicGroupMonitor{StationID: station.ID, StationName: station.Name, Range: preset, UpdatedAt: now, Groups: []PublicGroupMonitorGroup{}, SyncStatus: "ready"}
			for _, group := range monitored {
				aggregate := data[group.ExternalID]
				item := buildPublicGroupMonitorGroupForWindow(group, aggregate.usage, aggregate.failures, true, now, since, until)
				item.RecentMetrics = buildGroupRecentMetrics(aggregate.samples, now, hours, limit, true)
				view.Groups = append(view.Groups, item)
			}
			summarizeMonitorSnapshot(&view)
			payload, err := json.Marshal(view)
			if err != nil {
				return err
			}
			if err := s.stations.SaveGroupMonitorSnapshot(ctx, &storage.RelayGroupMonitorSnapshot{RelayStationID: station.ID, RangeName: preset, SourceKey: sourceKey, Payload: payload, UpdatedAt: now.UTC()}); err != nil {
				return err
			}
			published[preset] = true
		}
	}
	return s.stations.PruneGroupMonitorDays(ctx, station.ID, oldestDay.Add(-24*time.Hour))
}

func (s *Service) loadOrRefreshMonitorDay(ctx context.Context, station *storage.RelayStation, apiKey string, groupID int64, day, now time.Time) (monitorDayPayload, error) {
	until := day.Add(24 * time.Hour)
	if now.Before(until) {
		until = now.UTC()
	}
	row, err := s.stations.GetGroupMonitorDay(ctx, station.ID, groupID, day)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return monitorDayPayload{}, err
	}
	recheck := 24 * time.Hour
	if day.Equal(now.UTC().Truncate(24 * time.Hour).Add(-24 * time.Hour)) {
		recheck = 5 * time.Minute
	}
	if err == nil && row.SourceKey == monitorSourceKey(station) && !row.CollectedUntil.Before(until) && now.Sub(row.UpdatedAt) < recheck {
		reader, err := gzip.NewReader(bytes.NewReader(row.Payload))
		if err == nil {
			var payload monitorDayPayload
			decodeErr := json.NewDecoder(reader).Decode(&payload)
			_ = reader.Close()
			if decodeErr == nil {
				return payload, nil
			}
		}
	}
	var payload monitorDayPayload
	var usageErr, errorErr error
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		payload.Usage, usageErr = fetchGroupMonitorUsagePages(ctx, s, station.BaseURL, apiKey, groupID, day, until)
	}()
	go func() {
		defer wait.Done()
		payload.Errors, errorErr = fetchGroupMonitorErrorPages(ctx, s, station.BaseURL, apiKey, groupID, day, until)
	}()
	wait.Wait()
	if err := errors.Join(usageErr, errorErr); err != nil {
		return payload, err
	}
	// 上游用量按自然日过滤，本地再精确裁剪结束边界并去除分页重复。
	usage := payload.Usage[:0]
	seen := map[int64]bool{}
	for _, item := range payload.Usage {
		created, ok := parseMonitorTime(item.CreatedAt)
		if !ok || created.Before(day) || !created.Before(until) || (item.ID > 0 && seen[item.ID]) || (item.GroupID != 0 && item.GroupID != groupID) {
			continue
		}
		if item.ID > 0 {
			seen[item.ID] = true
		}
		item.AccountName, item.UserName, item.UserEmail, item.IPAddress = "", "", "", ""
		item.AccountID, item.UserID = 0, 0
		usage = append(usage, item)
	}
	payload.Usage = usage
	failures := payload.Errors[:0]
	seen = map[int64]bool{}
	for _, item := range payload.Errors {
		created, ok := parseMonitorTime(item.CreatedAt)
		if !ok || created.Before(day) || !created.Before(until) || (item.ID > 0 && seen[item.ID]) || (item.GroupID != 0 && item.GroupID != groupID) {
			continue
		}
		if item.ID > 0 {
			seen[item.ID] = true
		}
		failures = append(failures, item)
	}
	payload.Errors = failures
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if err := json.NewEncoder(writer).Encode(payload); err != nil {
		_ = writer.Close()
		return payload, err
	}
	if err := writer.Close(); err != nil {
		return payload, err
	}
	err = s.stations.SaveGroupMonitorDay(ctx, &storage.RelayGroupMonitorDay{RelayStationID: station.ID, GroupExternalID: groupID, Day: day, SourceKey: monitorSourceKey(station), Payload: buffer.Bytes(), CollectedUntil: until, UpdatedAt: now.UTC()})
	return payload, err
}
