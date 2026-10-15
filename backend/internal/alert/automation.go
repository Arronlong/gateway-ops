package alert

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

// AutomationAdapter 是站点能力边界。协议实现必须实时读取状态，不能以缓存代替。
type AutomationAdapter interface {
	ReadAutomationState(context.Context, uint, string, string) (enabled bool, fingerprint string, err error)
	SetAutomationState(context.Context, uint, string, string, bool) error
}

func saveOwnership(repo *storage.Alerts, state *storage.AlertRuleState, ownership storage.AlertAutomationOwnership) error {
	data, err := json.Marshal(ownership)
	if err != nil {
		return err
	}
	state.AutomaticActionJSON = string(data)
	return repo.SaveState(state)
}

func (s *Service) runFiringAction(ctx context.Context, rule storage.AlertRule, event *storage.AlertEvent, state *storage.AlertRuleState, now time.Time) error {
	var config ActionConfig
	if err := json.Unmarshal([]byte(rule.ActionJSON), &config); err != nil {
		return err
	}
	for _, spec := range config.Actions {
		if !spec.Enabled {
			continue
		}
		desired := spec.Kind == ActionEnableAccountScheduling || spec.Kind == ActionEnableUser
		// 停用约束与远端执行结果分开记录，读取失败也不能让其他规则提前启用。
		if !desired && !state.ManualIntervened {
			var ownership storage.AlertAutomationOwnership
			if err := json.Unmarshal([]byte(state.AutomaticActionJSON), &ownership); err != nil {
				return err
			}
			ownership.EventID, ownership.Hold = event.ID, true
			if err := saveOwnership(s.rules, state, ownership); err != nil {
				return err
			}
		}
		logs, err := s.rules.ListActionLogs(event.ID)
		if err != nil {
			return err
		}
		// 在远端调用前持久化执行占位；不重放崩溃或超时后结果不确定的写请求。
		for _, entry := range logs {
			if entry.Phase == "firing" {
				return nil
			}
		}
		entry := &storage.AlertActionLog{EventID: event.ID, Action: string(spec.Kind), Phase: "firing", Status: "prepared", Automatic: true, Attempt: 1, CreatedAt: now}
		if err := s.rules.SaveActionLog(entry); err != nil {
			return err
		}
		finish := func(status string, actionErr error) error {
			entry.Status = status
			entry.Success = status == "succeeded"
			if actionErr != nil {
				entry.ErrorMessage = actionErr.Error()
			}
			if err := s.rules.UpdateActionLog(entry); err != nil {
				return err
			}
			return actionErr
		}
		adapter, ok := s.actions.(AutomationAdapter)
		if !ok {
			return finish("skipped", errors.New("站点适配器未声明自动化能力"))
		}
		if state.ManualIntervened {
			return finish("skipped", errors.New("此事件发生后已有人工操作，禁止再次自动变更"))
		}
		before, _, err := adapter.ReadAutomationState(ctx, targetStation(state), rule.TargetType, state.TargetExternalID)
		if err != nil {
			return finish("failed", err)
		}
		ownership := storage.AlertAutomationOwnership{EventID: event.ID, Before: before, After: desired, Hold: !desired}
		entry.BeforeJSON = fmt.Sprintf(`{"enabled":%t}`, before)
		if desired {
			held, err := s.rules.OtherAutomationHold(*state)
			if err != nil {
				return finish("failed", err)
			}
			if held {
				return finish("skipped", errors.New("其他规则仍在停用此目标，禁止自动启用"))
			}
		}
		if err := saveOwnership(s.rules, state, ownership); err != nil {
			return finish("failed", err)
		}
		if before == desired {
			return finish("skipped", errors.New("目标已处于期望状态，未取得自动恢复所有权"))
		}
		if err := adapter.SetAutomationState(ctx, state.RelayStationID, rule.TargetType, state.TargetExternalID, desired); err != nil {
			return finish("uncertain", err)
		}
		after, fingerprint, err := adapter.ReadAutomationState(ctx, state.RelayStationID, rule.TargetType, state.TargetExternalID)
		if err != nil {
			return finish("uncertain", err)
		}
		if after != desired {
			return finish("uncertain", errors.New("写入后的远端状态与期望不符"))
		}
		ownership.Owned = !desired
		ownership.Fingerprint = fingerprint
		if err := saveOwnership(s.rules, state, ownership); err != nil {
			return finish("uncertain", err)
		}
		entry.AfterJSON = fmt.Sprintf(`{"enabled":%t}`, after)
		return finish("succeeded", nil)
	}
	return nil
}

func targetStation(state *storage.AlertRuleState) uint { return state.RelayStationID }

func (s *Service) runRecoveryAction(ctx context.Context, rule storage.AlertRule, event *storage.AlertEvent, state *storage.AlertRuleState, recovery RecoverySpec, now time.Time) (bool, error) {
	var ownership storage.AlertAutomationOwnership
	if err := json.Unmarshal([]byte(state.AutomaticActionJSON), &ownership); err != nil {
		return false, err
	}
	if !recovery.Enabled {
		return true, nil
	}
	// 本规则已恢复，不再阻挡其他规则；仅动作原所有者可以恢复目标。
	ownership.Hold = false
	if err := saveOwnership(s.rules, state, ownership); err != nil {
		return false, err
	}
	if !ownership.Owned || state.ManualIntervened {
		return true, nil
	}
	held, err := s.rules.OtherAutomationHold(*state)
	if err != nil || held {
		return false, err
	}
	logs, err := s.rules.ListActionLogs(event.ID)
	if err != nil {
		return false, err
	}
	for _, log := range logs {
		if log.Phase == "recovery" {
			return true, nil
		}
	}
	kind := ActionEnableAccountScheduling
	if rule.TargetType == string(TargetRelayUser) {
		kind = ActionEnableUser
	}
	entry := &storage.AlertActionLog{EventID: event.ID, Action: string(kind), Phase: "recovery", Status: "prepared", Automatic: true, Attempt: 1, CreatedAt: now}
	if err := s.rules.SaveActionLog(entry); err != nil {
		return false, err
	}
	finish := func(status string, reason string) (bool, error) {
		entry.Status = status
		entry.Success = status == "succeeded"
		entry.ErrorMessage = reason
		ownership.Owned = false
		if err := saveOwnership(s.rules, state, ownership); err != nil {
			return false, err
		}
		return true, s.rules.UpdateActionLog(entry)
	}
	adapter, ok := s.actions.(AutomationAdapter)
	if !ok {
		return finish("skipped", "适配器未提供自动恢复能力")
	}
	current, fingerprint, err := adapter.ReadAutomationState(ctx, state.RelayStationID, rule.TargetType, state.TargetExternalID)
	if err != nil {
		return finish("skipped", "无法核对远端状态："+err.Error())
	}
	if current != ownership.After || fingerprint != ownership.Fingerprint {
		return finish("skipped", "远端状态已被其他操作修改，保留人工决定")
	}
	if strings.TrimSpace(fingerprint) == "" || fingerprint == "null" {
		return finish("skipped", "远端未提供状态版本，无法安全判断人工干预，请手动恢复")
	}
	entry.BeforeJSON = fmt.Sprintf(`{"enabled":%t}`, current)
	if err := adapter.SetAutomationState(ctx, state.RelayStationID, rule.TargetType, state.TargetExternalID, ownership.Before); err != nil {
		return finish("uncertain", err.Error())
	}
	after, _, err := adapter.ReadAutomationState(ctx, state.RelayStationID, rule.TargetType, state.TargetExternalID)
	if err != nil || after != ownership.Before {
		return finish("uncertain", "恢复请求已发出，但未确认最终状态，请人工核对")
	}
	entry.AfterJSON = fmt.Sprintf(`{"enabled":%t}`, after)
	return finish("succeeded", "")
}
