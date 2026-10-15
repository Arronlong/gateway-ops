package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

// ReadAutomationState 是协议适配边界。预警引擎只使用归一化状态，不依赖远端端点。
// 新增站点协议时在此层提供能力和状态映射，不应在规则判定中拼接接口地址。
func (s *Service) ReadAutomationState(ctx context.Context, stationID uint, targetType, targetID string) (bool, string, error) {
	id, err := strconv.ParseInt(targetID, 10, 64)
	if err != nil || id <= 0 {
		return false, "", errors.New("目标必须是有效的远端稳定 ID，不能使用邮箱")
	}
	station, err := s.stations.FindByID(stationID)
	if err != nil {
		return false, "", err
	}
	key, err := s.cipher.Decrypt(station.APIKeyCipher)
	if err != nil {
		return false, "", err
	}
	resource := "accounts"
	if targetType == "relay_user" {
		resource = "users"
	} else if targetType != "relay_account" {
		return false, "", errors.New("当前协议不支持此自动化目标")
	}
	var remote struct {
		Schedulable *bool           `json:"schedulable"`
		Status      string          `json:"status"`
		UpdatedAt   json.RawMessage `json:"updated_at"`
	}
	if err := s.get(ctx, fmt.Sprintf("%s/api/v1/admin/%s/%d", station.BaseURL, resource, id), key, &remote); err != nil {
		return false, "", err
	}
	if targetType == "relay_account" {
		if remote.Schedulable == nil {
			return false, "", errors.New("远端未提供调度状态，禁止自动化")
		}
		return *remote.Schedulable, string(remote.UpdatedAt), nil
	}
	if remote.Status != "active" && remote.Status != "disabled" {
		return false, "", errors.New("远端用户状态不可用于自动化")
	}
	return remote.Status == "active", string(remote.UpdatedAt), nil
}

func (s *Service) SetAutomationState(ctx context.Context, stationID uint, targetType, targetID string, enabled bool) error {
	id, err := strconv.ParseInt(targetID, 10, 64)
	if err != nil || id <= 0 {
		return errors.New("自动化目标 ID 无效")
	}
	station, err := s.stations.FindByID(stationID)
	if err != nil {
		return err
	}
	if targetType == "relay_account" {
		account, err := s.stations.FindAccountByExternalID(stationID, id)
		if err != nil {
			return err
		}
		// 使用刚读取的远端状态，避免本地旧快照导致错误跳过。
		account.Schedulable = !enabled
		groups, err := s.accountCurrentGroups(stationID, account.ID)
		if err != nil {
			return err
		}
		return s.setScheduling(ctx, station, *account, groups, enabled, "alert")
	}
	if targetType != "relay_user" {
		return errors.New("当前协议不支持此自动化目标")
	}
	key, err := s.cipher.Decrypt(station.APIKeyCipher)
	if err != nil {
		return err
	}
	status := "disabled"
	if enabled {
		status = "active"
	}
	return s.put(ctx, fmt.Sprintf("%s/api/v1/admin/users/%d", station.BaseURL, id), key, map[string]any{"status": status})
}

// SyncAlertUsage 只补齐增量用量，不执行完整快照、主动测速或三十天回填。
func (s *Service) SyncAlertUsage(ctx context.Context, stationID uint) error {
	station, err := s.stations.FindByID(stationID)
	if err != nil {
		return err
	}
	key, err := s.cipher.Decrypt(station.APIKeyCipher)
	if err != nil {
		return err
	}
	_, err, _ = s.usageBackfillGroup.Do(strconv.FormatUint(uint64(stationID), 10), func() (any, error) {
		now := time.Now().UTC()
		state, err := s.stations.UsageSyncState(stationID)
		if err != nil {
			return nil, err
		}
		start := now.Add(-2 * time.Hour)
		if state.ForwardSyncedAt != nil {
			start = state.ForwardSyncedAt.Add(-time.Minute)
		}
		if start.Before(now.Add(-24 * time.Hour)) {
			start = now.Add(-24 * time.Hour)
		}
		if err := s.syncUsageRange(ctx, *station, key, start, now); err != nil {
			return nil, err
		}
		state.ForwardSyncedAt = &now
		if state.BackfilledAt == nil {
			state.BackfilledAt = &start
		}
		return nil, s.stations.SaveUsageSyncState(state)
	})
	return err
}

// SyncAlertRates 只读取上游已声明的成本快照，不启动付费请求或主动探测。
func (s *Service) SyncAlertRates(ctx context.Context, stationID uint) error {
	station, err := s.stations.FindByID(stationID)
	if err != nil {
		return err
	}
	key, err := s.cipher.Decrypt(station.APIKeyCipher)
	if err != nil {
		return err
	}
	accounts, err := s.fetchAccounts(ctx, station.BaseURL, key)
	if err != nil {
		return err
	}
	updates := make([]storage.RelayAccountRateUpdate, 0, len(accounts))
	for _, account := range accounts {
		value, observed := upstreamProbeCost(account.Extra)
		if value == nil {
			continue
		}
		updates = append(updates, storage.RelayAccountRateUpdate{ExternalID: account.ID, RateMultiplier: value, RateObservedAt: observed, RateSource: "upstream_probe"})
	}
	return s.stations.UpdateAccountRates(stationID, updates, time.Now().UTC())
}

func (s *Service) SyncAlertStations(ctx context.Context, kind string, stationIDs []uint) error {
	stations, err := s.stations.List()
	if err != nil {
		return err
	}
	var errs []error
	for _, station := range stations {
		match := len(stationIDs) == 0
		for _, id := range stationIDs {
			if station.ID == id {
				match = true
			}
		}
		if !match {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if kind == "rate" {
			err = s.SyncAlertRates(ctx, station.ID)
		} else {
			err = s.SyncAlertUsage(ctx, station.ID)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("station %d: %w", station.ID, err))
		}
	}
	return errors.Join(errs...)
}

func (s *Service) MarkAlertManualIntervention(targetType string, stationID uint, targetID int64) error {
	return s.stations.MarkAlertManualIntervention(targetType, stationID, strconv.FormatInt(targetID, 10))
}
