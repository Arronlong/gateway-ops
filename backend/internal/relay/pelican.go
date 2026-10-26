package relay

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
	"github.com/guliansheng/gateway-ops/internal/tasklog"
	"gorm.io/gorm"
)

const DefaultPelicanPrompt = storage.DefaultPelicanPrompt
const pelicanContract = "\n\n直接返回独立 HTML，不使用 Markdown 代码块或外部依赖。只输出 HTML，不要解释。"
const pelicanTimeout = 10 * time.Minute

var ErrPelicanRunning = errors.New("该账号正在进行鹈鹕测智")
var ErrPelicanUnsupportedStation = errors.New("鹈鹕测智仅支持 Sub2API 中转站")
var pelicanStart = regexp.MustCompile(`(?i)<(?:!doctype\s+html|html|svg)[\s>]`)

func validatePelicanStation(station *storage.RelayStation) error {
	if station != nil && station.Type == string(storage.ChannelTypeNewAPI) {
		return ErrPelicanUnsupportedStation
	}
	return nil
}

func applyPelicanSettings(config *storage.PelicanConfig, settings storage.PelicanSettings) {
	plans := settings.Plans
	if len(plans) == 0 {
		plans = []storage.PelicanTestPlan{{Model: settings.Model, Prompt: settings.Prompt, ReasoningEffort: settings.ReasoningEffort}}
	}
	if len(plans) > 0 {
		config.Model = plans[0].Model
		config.Prompt = plans[0].Prompt
		config.ReasoningEffort = plans[0].ReasoningEffort
	}
	config.ParallelCount = 1
	config.CronExpression = settings.CronExpression
}

func pelicanExecutionPlans(config storage.PelicanConfig, configured [][]storage.PelicanTestPlan) []storage.PelicanTestPlan {
	if len(configured) > 0 && len(configured[0]) > 0 {
		return append([]storage.PelicanTestPlan(nil), configured[0]...)
	}
	return []storage.PelicanTestPlan{{Model: config.Model, Prompt: config.Prompt, ReasoningEffort: config.ReasoningEffort}}
}

func pelicanManualRunPlans(configured []storage.PelicanTestPlan, requested []storage.PelicanTestPlan) ([]storage.PelicanTestPlan, error) {
	plans := storage.ConfiguredPelicanPlans(configured)
	if len(plans) == 0 {
		return nil, errors.New("请先在系统设置中配置鹈鹕测智模型")
	}
	if len(requested) == 0 {
		return plans, nil
	}
	if len(requested) != 1 {
		return nil, errors.New("一次手动测智只能指定一个方案")
	}
	plan := requested[0]
	plan.Model = strings.TrimSpace(plan.Model)
	if plan.Model == "" || len(plan.Model) > 256 {
		return nil, errors.New("请选择有效的测智模型")
	}
	modelConfigured := false
	for _, configuredPlan := range plans {
		for _, model := range storage.PelicanModelPriorityList(configuredPlan.Model) {
			if model == plan.Model {
				modelConfigured = true
				break
			}
		}
		if modelConfigured {
			break
		}
	}
	if !modelConfigured {
		return nil, errors.New("所选模型不在系统设置的测智模型列表中")
	}
	if strings.TrimSpace(plan.Prompt) == "" || len(plan.Prompt) > 32000 {
		return nil, errors.New("测智提示词不能为空且不能超过 32000 字节")
	}
	switch plan.ReasoningEffort {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra":
	default:
		return nil, errors.New("思考强度无效")
	}
	return []storage.PelicanTestPlan{plan}, nil
}

func (s *Service) pelicanSettings() (storage.PelicanSettings, error) {
	if s.settings == nil {
		return storage.PelicanSettings{}, errors.New("system settings unavailable")
	}
	return s.settings.PelicanSettings()
}
func extractPelicanArtwork(text string) (string, error) {
	start := pelicanStart.FindStringIndex(text)
	if start == nil {
		return "", errors.New("模型未返回 HTML/SVG 作品")
	}
	html := text[start[0]:]
	ending := "</html>"
	if strings.HasPrefix(strings.ToLower(html), "<svg") {
		ending = "</svg>"
	}
	end := strings.LastIndex(strings.ToLower(html), ending)
	if end < 0 {
		return "", errors.New("HTML/SVG 输出不完整")
	}
	html = html[:end+len(ending)]
	if len(html) > 2<<20 {
		return "", errors.New("作品超过 2 MiB 保存上限")
	}
	return html, nil
}
func (s *Service) SavePelicanConfig(stationID uint, accountID int64, config storage.PelicanConfig) (*storage.PelicanConfig, error) {
	station, err := s.stations.FindByID(stationID)
	if err != nil {
		return nil, err
	}
	if err = validatePelicanStation(station); err != nil {
		return nil, err
	}
	settings, err := s.pelicanSettings()
	if err != nil {
		return nil, err
	}
	if config.Enabled && len(storage.ConfiguredPelicanPlans(settings.Plans)) == 0 {
		return nil, errors.New("\u8bf7\u5148\u5728\u7cfb\u7edf\u8bbe\u7f6e\u4e2d\u914d\u7f6e\u9e48\u9e55\u6d4b\u667a\u6a21\u578b")
	}
	applyPelicanSettings(&config, settings)
	config.ID = 0
	config.RelayStationID = stationID
	config.AccountExternalID = accountID
	config.LeaseUntil = nil
	config.CreatedAt = time.Time{}
	config.UpdatedAt = time.Time{}
	config.NextRunAt, err = settings.NextRun(time.Now())
	if err != nil {
		return nil, err
	}
	if err := s.stations.SavePelicanConfig(&config); err != nil {
		return nil, err
	}
	return s.stations.PelicanConfig(stationID, accountID)
}
func (s *Service) generatePelican(ctx context.Context, station storage.RelayStation, key string, config storage.PelicanConfig) (string, error) {
	target, err := s.preparePelican(ctx, station, key, config)
	if err != nil {
		return "", err
	}
	defer target.close()
	return target.generate(ctx, config)
}
func (s *Service) StartPelican(ctx context.Context, stationID uint, accountID int64, requested ...storage.PelicanTestPlan) (*storage.PelicanConfig, error) {
	station, err := s.stations.FindByID(stationID)
	if err != nil {
		return nil, err
	}
	if err = validatePelicanStation(station); err != nil {
		return nil, err
	}
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
	config, err := s.stations.PelicanConfig(stationID, accountID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		config, err = s.SavePelicanConfig(stationID, accountID, storage.PelicanConfig{Enabled: false})
	}
	if err != nil {
		return nil, err
	}
	applyPelicanSettings(config, settings)
	if _, err = s.stations.FindAccountByExternalID(stationID, accountID); err != nil {
		return nil, err
	}
	until := time.Now().UTC().Add(15 * time.Minute).Truncate(time.Microsecond)
	claimed, err := s.stations.ClaimPelican(config.ID, time.Now(), until, config.NextRunAt, false)
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, ErrPelicanRunning
	}
	config.LeaseUntil = &until
	ready := make(chan error, 1)
	go s.runPelicanBatchReady(context.Background(), *config, until, time.Time{}, "manual", ready, plans)
	if err := <-ready; err != nil {
		return nil, err
	}
	return config, nil
}
func (s *Service) RunDuePelicans(ctx context.Context) error {
	return s.runDuePelicansAt(ctx, time.Now())
}

func (s *Service) runDuePelicansAt(ctx context.Context, now time.Time) error {
	settings, err := s.pelicanSettings()
	if err != nil {
		return err
	}
	if !settings.TasksEnabled {
		tasklog.Add(ctx, "INFO", "未触发自动测试：鹈鹕测智任务执行已关闭")
		return nil
	}
	plans := storage.ConfiguredPelicanPlans(settings.Plans)
	if len(plans) == 0 || !settings.AutomaticRunAllowed(now) {
		tasklog.Add(ctx, "INFO", "未触发自动测试：模型未配置或不在允许执行时段")
		return nil
	}
	configs, err := s.stations.DuePelicanConfigs(now)
	if err != nil {
		return err
	}
	tasklog.Add(ctx, "INFO", "本次待调度账号：%d", len(configs))
	var jobs sync.WaitGroup
	defer jobs.Wait()
	for _, config := range configs {
		applyPelicanSettings(&config, settings)
		if err := ctx.Err(); err != nil {
			return err
		}
		next, err := settings.NextRun(now)
		if err != nil {
			return err
		}
		until := now.UTC().Add(15 * time.Minute).Truncate(time.Microsecond)
		claimed, err := s.stations.ClaimPelican(config.ID, now, until, config.NextRunAt, true)
		if err != nil {
			return err
		}
		if !claimed {
			continue
		}
		if !settings.AutomaticRunAllowed(config.NextRunAt) {
			if err := s.stations.CompletePelican(config.ID, until, config.NextRunAt, next); err != nil {
				return err
			}
			continue
		}
		jobs.Add(1)
		go func(config storage.PelicanConfig, next time.Time) {
			defer jobs.Done()
			s.runPelicanBatch(ctx, config, until, next, "scheduled", plans)
		}(config, next)
	}
	return s.runDueGroupPelicans(ctx, now, settings)
}
func (s *Service) savePelicanFailures(config storage.PelicanConfig, trigger, accountName string, cause error, configured ...[]storage.PelicanTestPlan) bool {
	saved := true
	if accountName == "" {
		accountName = fmt.Sprintf("账号 #%d", config.AccountExternalID)
	}
	plans := pelicanExecutionPlans(config, configured)
	errorText := redactTestText(cause.Error(), "")
	for _, plan := range plans {
		started := time.Now().UTC()
		result := storage.PelicanResult{RelayStationID: config.RelayStationID, AccountExternalID: config.AccountExternalID, AccountName: accountName, Model: plan.Model, Prompt: plan.Prompt, ReasoningEffort: plan.ReasoningEffort, Trigger: trigger, Status: "failed", Error: errorText, StartedAt: started, FinishedAt: started, DurationMS: 0}
		if err := s.stations.SavePelicanResult(&result); err != nil {
			saved = false
			slog.Error("save pelican failure result failed", "station", config.RelayStationID, "account", config.AccountExternalID, "error", err)
		}
	}
	return saved
}

func (s *Service) runPelicanBatch(parent context.Context, config storage.PelicanConfig, until, next time.Time, trigger string, configured ...[]storage.PelicanTestPlan) {
	s.runPelicanBatchReady(parent, config, until, next, trigger, nil, configured...)
}

func (s *Service) runPelicanBatchReady(parent context.Context, config storage.PelicanConfig, until, next time.Time, trigger string, ready chan error, configured ...[]storage.PelicanTestPlan) {
	var readyOnce sync.Once
	notify := func(err error) {
		if ready != nil {
			readyOnce.Do(func() { ready <- err })
		}
	}
	defer func() { notify(errors.New("创建测智任务失败")) }()
	plans := pelicanExecutionPlans(config, configured)
	tasklog.Add(parent, "INFO", "开始测试：中转站 #%d，账号 #%d", config.RelayStationID, config.AccountExternalID)
	var saved atomic.Bool
	defer func() {
		if !saved.Load() {
			return
		}
		var err error
		if trigger == "scheduled" && !next.IsZero() {
			err = s.stations.CompletePelican(config.ID, until, config.NextRunAt, next)
		} else {
			err = s.stations.FinishPelican(config.ID, until)
		}
		if err != nil {
			slog.Error("finish pelican lease failed", "station", config.RelayStationID, "account", config.AccountExternalID, "error", err)
			tasklog.Add(parent, "ERROR", "finish pelican lease failed: %v", err)
		}
	}()
	ctx, cancel := context.WithTimeout(parent, pelicanTimeout)
	defer cancel()
	station, err := s.stations.FindByID(config.RelayStationID)
	if err != nil {
		slog.Error("load pelican station failed", "station", config.RelayStationID, "account", config.AccountExternalID, "error", err)
		tasklog.Add(parent, "ERROR", "load pelican station failed: %v", err)
		return
	}
	account, err := s.stations.FindAccountByExternalID(config.RelayStationID, config.AccountExternalID)
	if err != nil {
		slog.Error("load pelican account failed", "station", config.RelayStationID, "account", config.AccountExternalID, "error", err)
		tasklog.Add(parent, "ERROR", "load pelican account failed: %v", err)
		saved.Store(s.savePelicanFailures(config, trigger, "", err, plans))
		return
	}
	results := make([]storage.PelicanResult, len(plans))
	for i, plan := range plans {
		started := time.Now().UTC()
		results[i] = storage.PelicanResult{RelayStationID: config.RelayStationID, AccountExternalID: config.AccountExternalID, AccountName: account.Name, Model: plan.Model, Prompt: plan.Prompt, ReasoningEffort: plan.ReasoningEffort, Trigger: trigger, Status: "running", StartedAt: started, FinishedAt: started}
		if err := s.stations.SavePelicanResult(&results[i]); err != nil {
			for j := 0; j < i; j++ {
				results[j].Status, results[j].Error, results[j].FinishedAt = "failed", redactTestText(err.Error(), ""), time.Now().UTC()
				if saveErr := s.stations.SavePelicanResult(&results[j]); saveErr != nil {
					slog.Error("finalize pelican placeholder failed", "error", saveErr)
				}
			}
			if finishErr := s.stations.FinishPelican(config.ID, until); finishErr != nil {
				slog.Error("release pelican lease failed", "error", finishErr)
			}
			notify(err)
			return
		}
	}
	notify(nil)
	key, err := s.cipher.Decrypt(station.APIKeyCipher)
	if err != nil {
		tasklog.Add(parent, "ERROR", "账号 #%d 执行失败：%v", config.AccountExternalID, err)
		saved.Store(true)
		for i := range results {
			results[i].Status, results[i].Error, results[i].FinishedAt = "failed", redactTestText(err.Error(), ""), time.Now().UTC()
			if saveErr := s.stations.SavePelicanResult(&results[i]); saveErr != nil {
				saved.Store(false)
			}
		}
		return
	}
	saved.Store(true)
	var keyInfoOnce sync.Once
	var keyInfo pelicanKeyGroup
	var calls sync.WaitGroup
	for i, plan := range plans {
		calls.Add(1)
		go func(plan storage.PelicanTestPlan, result storage.PelicanResult) {
			defer calls.Done()
			planConfig := config
			planConfig.Model, planConfig.Prompt, planConfig.ReasoningEffort, planConfig.ParallelCount = plan.Model, plan.Prompt, plan.ReasoningEffort, 1
			result.Status = "success"
			target, err := s.preparePelican(ctx, *station, key, planConfig)
			var html string
			if err == nil {
				result.Model = target.model
				keyInfoOnce.Do(func() { keyInfo = s.pelicanKeyGroup(ctx, target, config.RelayStationID, config.AccountExternalID) })
				result.UpstreamGroupName = keyInfo.Name
				result.UpstreamGroupMultiplier = keyInfo.Multiplier
				html, err = target.generate(ctx, planConfig)
				target.close()
			}
			result.FinishedAt = time.Now().UTC()
			result.DurationMS = result.FinishedAt.Sub(result.StartedAt).Milliseconds()
			if err != nil {
				result.Status = "failed"
				result.Error = redactTestText(err.Error(), key)
				tasklog.Add(parent, "ERROR", "账号 #%d 生成失败：%s", config.AccountExternalID, result.Error)
			} else {
				result.HTML = html
				tasklog.Add(parent, "INFO", "账号 #%d 生成成功，耗时 %d ms", config.AccountExternalID, result.DurationMS)
			}
			if err := s.stations.SavePelicanResult(&result); err != nil {
				saved.Store(false)
				slog.Error("save pelican result failed", "station", config.RelayStationID, "account", config.AccountExternalID, "error", err)
				tasklog.Add(parent, "ERROR", "save pelican result failed: %v", err)
			}
		}(plan, results[i])
	}
	calls.Wait()
}
