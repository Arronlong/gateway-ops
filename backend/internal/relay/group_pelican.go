package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
	"github.com/guliansheng/gateway-ops/internal/tasklog"
	"gorm.io/gorm"
)

func (s *Service) SaveGroupPelicanConfig(stationID uint, groupID int64, enabled bool) (*storage.GroupPelicanConfig, error) {
	settings, err := s.pelicanSettings()
	if err != nil {
		return nil, err
	}
	next, err := settings.NextRun(time.Now())
	if err != nil {
		return nil, err
	}
	config := storage.GroupPelicanConfig{RelayStationID: stationID, GroupExternalID: groupID, Enabled: enabled, CronExpression: settings.CronExpression, NextRunAt: next}
	if err := s.stations.SaveGroupPelicanConfig(&config); err != nil {
		return nil, err
	}
	return s.stations.GroupPelicanConfig(stationID, groupID)
}
func (s *Service) StartGroupPelican(ctx context.Context, stationID uint, groupID int64, requested ...storage.PelicanTestPlan) (*storage.GroupPelicanConfig, error) {
	settings, err := s.pelicanSettings()
	if err != nil {
		return nil, err
	}
	if !settings.TasksEnabled {
		return nil, errors.New("鹈鹕测智任务执行已关闭，请在系统设置中开启")
	}
	plans, err := pelicanManualRunPlans(settings.Plans, requested)
	if err != nil {
		return nil, err
	}
	if _, err := s.stations.FindByID(stationID); err != nil {
		return nil, err
	}
	if _, err := s.stations.FindGroupByExternalID(stationID, groupID); err != nil {
		return nil, err
	}
	config, err := s.stations.GroupPelicanConfig(stationID, groupID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		config, err = s.SaveGroupPelicanConfig(stationID, groupID, false)
	}
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	until := now.Add(15 * time.Minute).Truncate(time.Microsecond)
	claimed, err := s.stations.ClaimGroupPelican(config.ID, now, until, config.NextRunAt, false)
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, ErrPelicanRunning
	}
	config.LeaseUntil = &until
	ready := make(chan error, 1)
	go s.runGroupPelicanBatchReady(context.Background(), *config, until, time.Time{}, "manual", plans, ready)
	if err := <-ready; err != nil {
		return nil, err
	}
	return config, nil
}

func (s *Service) runDueGroupPelicans(ctx context.Context, now time.Time, settings storage.PelicanSettings) error {
	if !settings.TasksEnabled || !settings.AutomaticRunAllowed(now) {
		return nil
	}
	plans := storage.ConfiguredPelicanPlans(settings.Plans)
	if len(plans) == 0 {
		return nil
	}
	configs, err := s.stations.DueGroupPelicanConfigs(now)
	if err != nil {
		return err
	}
	var jobs sync.WaitGroup
	defer jobs.Wait()
	for _, config := range configs {
		if err := ctx.Err(); err != nil {
			return err
		}
		next, err := settings.NextRun(now)
		if err != nil {
			return err
		}
		until := now.UTC().Add(15 * time.Minute).Truncate(time.Microsecond)
		claimed, err := s.stations.ClaimGroupPelican(config.ID, now, until, config.NextRunAt, true)
		if err != nil {
			return err
		}
		if !claimed {
			continue
		}
		if !settings.AutomaticRunAllowed(config.NextRunAt) {
			if err := s.stations.FinishGroupPelican(config.ID, until, config.NextRunAt, next); err != nil {
				return err
			}
			continue
		}
		jobs.Add(1)
		go func(config storage.GroupPelicanConfig, next time.Time) {
			defer jobs.Done()
			s.runGroupPelicanBatch(ctx, config, until, next, "scheduled", plans)
		}(config, next)
	}
	return nil
}

func (s *Service) groupPelicanModels(ctx context.Context, station storage.RelayStation, key string) ([]string, error) {
	endpoint, err := pelicanUpstreamURL(station.BaseURL, "models")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	client := *s.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 2<<20 {
		return nil, errors.New("分组模型目录过大")
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("读取分组模型失败（HTTP %d）", response.StatusCode)
	}
	models := extractAccountModelIDs(json.RawMessage(data))
	if len(models) == 0 {
		return nil, errors.New("该分组没有可用模型")
	}
	return models, nil
}

func (s *Service) runGroupPelicanBatch(parent context.Context, config storage.GroupPelicanConfig, until, next time.Time, trigger string, plans []storage.PelicanTestPlan) {
	s.runGroupPelicanBatchReady(parent, config, until, next, trigger, plans, nil)
}

func (s *Service) runGroupPelicanBatchReady(parent context.Context, config storage.GroupPelicanConfig, until, next time.Time, trigger string, plans []storage.PelicanTestPlan, ready chan error) {
	ctx, cancel := context.WithTimeout(parent, pelicanTimeout)
	defer cancel()
	var saveFailed atomic.Bool
	defer func() {
		if saveFailed.Load() {
			return
		}
		if err := s.stations.FinishGroupPelican(config.ID, until, config.NextRunAt, next); err != nil {
			slog.Error("finish group pelican lease failed", "station", config.RelayStationID, "group", config.GroupExternalID, "error", err)
		}
	}()
	group, preflight := s.stations.FindGroupByExternalID(config.RelayStationID, config.GroupExternalID)
	name := fmt.Sprintf("分组 #%d", config.GroupExternalID)
	if group != nil && group.Name != "" {
		name = group.Name
	}
	results := make([]storage.GroupPelicanResult, len(plans))
	for i, plan := range plans {
		started := time.Now().UTC()
		results[i] = storage.GroupPelicanResult{RelayStationID: config.RelayStationID, GroupExternalID: config.GroupExternalID, GroupName: name, Model: plan.Model, Prompt: plan.Prompt, ReasoningEffort: plan.ReasoningEffort, Trigger: trigger, Status: "running", StartedAt: started, FinishedAt: started}
		if err := s.stations.SaveGroupPelicanResult(&results[i]); err != nil {
			for j := 0; j < i; j++ {
				results[j].Status, results[j].Error, results[j].FinishedAt = "failed", redactTestText(err.Error(), ""), time.Now().UTC()
				if saveErr := s.stations.SaveGroupPelicanResult(&results[j]); saveErr != nil {
					saveFailed.Store(true)
				}
			}
			if ready != nil {
				ready <- err
			}
			return
		}
	}
	if ready != nil {
		ready <- nil
	}
	var station *storage.RelayStation
	var key string
	var models []string
	if preflight == nil {
		station, key, preflight = s.groupGatewayCredential(ctx, config.RelayStationID, config.GroupExternalID, true)
	}
	if preflight == nil {
		models, preflight = s.groupPelicanModels(ctx, *station, key)
	}
	secrets := []string{key}
	if station != nil {
		if admin, err := s.cipher.Decrypt(station.APIKeyCipher); err == nil {
			secrets = append(secrets, admin)
		}
	}
	var jobs sync.WaitGroup
	for i, plan := range plans {
		jobs.Add(1)
		go func(plan storage.PelicanTestPlan, result storage.GroupPelicanResult) {
			defer jobs.Done()
			result.Status = "success"
			err := preflight
			if err == nil {
				result.Model, err = selectPelicanModel(plan.Model, models)
				if err == nil {
					endpoint, e := pelicanUpstreamURL(station.BaseURL, "chat/completions")
					err = e
					if err == nil {
						target := &pelicanTarget{client: *s.client, endpoint: endpoint, model: result.Model, token: key, chat: true, headers: make(http.Header), secrets: secrets}
						target.client.Timeout = pelicanTimeout
						target.client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
						result.UpstreamGroupName = name
						rate := group.RateMultiplier
						result.UpstreamGroupMultiplier = &rate
						result.HTML, err = target.generate(ctx, storage.PelicanConfig{Model: result.Model, Prompt: plan.Prompt, ReasoningEffort: plan.ReasoningEffort})
					}
				}
			}
			result.FinishedAt = time.Now().UTC()
			result.DurationMS = result.FinishedAt.Sub(result.StartedAt).Milliseconds()
			if err != nil {
				result.Status = "failed"
				result.Error = err.Error()
				for _, secret := range secrets {
					if strings.TrimSpace(secret) != "" {
						result.Error = strings.ReplaceAll(result.Error, secret, "[redacted]")
					}
				}
				result.Error = redactTestText(result.Error, "")
			}
			if err := s.stations.SaveGroupPelicanResult(&result); err != nil {
				saveFailed.Store(true)
				tasklog.Add(parent, "ERROR", "保存分组测智结果失败：%v", err)
			}
		}(plan, results[i])
	}
	jobs.Wait()
}
