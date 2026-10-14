package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

func accountHistoryCall(result AccountTestResult, err error) storage.RelayTestCall {
	success := err == nil && result.StatusCode >= 200 && result.StatusCode < 300 && strings.TrimSpace(result.Output) != ""
	hasStream, completed := false, false
	for _, line := range strings.Split(result.Output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		hasStream = true
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		var event struct {
			Type    string `json:"type"`
			Success *bool  `json:"success"`
		}
		if json.Unmarshal([]byte(payload), &event) != nil {
			continue
		}
		if event.Type == "error" {
			success = false
		}
		if event.Type == "test_complete" && event.Success != nil {
			completed = true
			success = success && *event.Success
		}
	}
	if hasStream && !completed {
		success = false
	}
	if !hasStream {
		var data struct {
			Success *bool           `json:"success"`
			Code    int             `json:"code"`
			Error   json.RawMessage `json:"error"`
		}
		if json.Unmarshal([]byte(result.Output), &data) == nil {
			if data.Success != nil {
				success = success && *data.Success
			}
			if data.Code != 0 || (len(data.Error) > 0 && string(data.Error) != "null") {
				success = false
			}
		}
	}
	output := result.Output
	if output == "" && err != nil {
		output = err.Error()
	}
	return storage.RelayTestCall{Index: 1, Success: success, StatusCode: result.StatusCode, DurationMS: result.DurationMS, Output: output}
}
func summarizeTestRun(run *storage.RelayTestRun) {
	run.Succeeded, run.Failed = 0, 0
	for _, call := range run.Results {
		if call.Success {
			run.Succeeded++
		} else {
			run.Failed++
		}
	}
}
func (s *Service) persistTestRun(run *storage.RelayTestRun, upstreamErr error) error {
	run.FinishedAt = time.Now().UTC()
	run.DurationMS = run.FinishedAt.Sub(run.StartedAt).Milliseconds()
	var secret string
	if station, err := s.stations.FindByID(run.RelayStationID); err == nil && s.cipher != nil {
		secret, _ = s.cipher.Decrypt(station.APIKeyCipher)
	}
	if upstreamErr != nil {
		run.Error = redactTestText(upstreamErr.Error(), secret)
	}
	for index := range run.Results {
		run.Results[index].Output = redactTestText(run.Results[index].Output, secret)
	}
	summarizeTestRun(run)
	if err := s.stations.SaveTestRun(run); err != nil {
		return errors.Join(upstreamErr, fmt.Errorf("保存测试记录失败: %w", err))
	}
	return upstreamErr
}
func (s *Service) TestAccount(ctx context.Context, stationID uint, accountID int64, modelID, mode string) (AccountTestResult, error) {
	station, err := s.stations.FindByID(stationID)
	if err != nil {
		return AccountTestResult{}, err
	}
	run := storage.RelayTestRun{RelayStationID: stationID, StationName: station.Name, TargetType: "account", TargetID: accountID, TargetName: fmt.Sprintf("账号 #%d", accountID), Model: modelID, Mode: mode, Requested: 1, StartedAt: time.Now().UTC()}
	if account, findErr := s.stations.FindAccountByExternalID(stationID, accountID); findErr == nil {
		run.TargetName = account.Name
	}
	result, callErr := s.testAccount(ctx, stationID, accountID, modelID, mode)
	call := accountHistoryCall(result, callErr)
	if call.DurationMS == 0 {
		call.DurationMS = time.Since(run.StartedAt).Milliseconds()
	}
	run.Results = []storage.RelayTestCall{call}
	return result, s.persistTestRun(&run, callErr)
}
func (s *Service) TestGroup(ctx context.Context, stationID uint, groupID int64, model string, count int) (GroupTestResult, error) {
	station, err := s.stations.FindByID(stationID)
	if err != nil {
		return GroupTestResult{}, err
	}
	run := storage.RelayTestRun{RelayStationID: stationID, StationName: station.Name, TargetType: "group", TargetID: groupID, TargetName: fmt.Sprintf("分组 #%d", groupID), Model: model, Mode: "regular", Requested: count, StartedAt: time.Now().UTC(), Results: make([]storage.RelayTestCall, 0)}
	if group, findErr := s.stations.FindGroupByExternalID(stationID, groupID); findErr == nil {
		run.TargetName = group.Name
	}
	result, callErr := s.testGroup(ctx, stationID, groupID, model, count)
	for _, call := range result.Results {
		run.Results = append(run.Results, storage.RelayTestCall{Index: call.Index, Success: call.Success, StatusCode: call.StatusCode, DurationMS: call.DurationMS, Output: call.Output})
	}
	if len(run.Results) == 0 && callErr != nil {
		run.SetupError = true
	}
	return result, s.persistTestRun(&run, callErr)
}
