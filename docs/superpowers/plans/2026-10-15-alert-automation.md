# 预警中心与自动化处理实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）来跟踪进度。

**目标：** 在现有 GatewayOps 中增加以中转站账号和用户为目标的预警规则、自动化动作、自动恢复、通知渠道关联和预警中心页面。

**架构：** 新增 `internal/alert` 作为规则领域层，读取中转站用量、账号快照和渠道余额，维护规则目标状态与事件生命周期。scheduler 每分钟执行本地评估，采集任务按倍率、余额、用量/IP 和稳定性分别控制新鲜度；动作层调用现有 relay 服务完成账号停排、用户启停，并通过现有 notify dispatcher 发送内置消息。前端保留现有 GatewayOps 外壳，在通知页面中增加指标卡和四个页签。

**技术栈：** Go 1.23、Gin、GORM/PostgreSQL、robfig/cron、React 19、TypeScript、React Router、Tailwind、现有通知渠道和中转站服务。

---

## 文件清单与职责

**后端新增：**

- `backend/internal/alert/types.go`：规则、条件、动作、状态和统一指标类型。
- `backend/internal/alert/conditions.go`：固定阈值、倍率变化、基线、窗口统计和 IP 规则计算。
- `backend/internal/alert/state.go`：`normal/pending/firing/recovering/recovered/unknown` 状态机。
- `backend/internal/alert/service.go`：规则 CRUD、全量评估、事件生成和状态持久化编排。
- `backend/internal/alert/actions.go`：账号停排、用户启停、自动恢复保护和动作重试。
- `backend/internal/storage/alerts.go`：预警规则、状态、事件、动作日志、指标样本和通知渠道关联的存储方法。
- `backend/internal/api/alerts.go`：预警规则、事件、汇总、设置和测试评估 API。

**后端修改：**

- `backend/internal/storage/model.go`：新增 GORM 模型；扩展 `RelayUsageEvent` 的稳定用户标识和客户端 IP。
- `backend/internal/storage/storage.go`：加入预警模型迁移与历史字段兼容处理。
- `backend/internal/storage/settings.go`：加入预警开关和采集频率设置。
- `backend/internal/storage/relay_usage.go`：支持按账号、用户、窗口、IP 和状态聚合查询。
- `backend/internal/relay/service.go`：暴露中转站账号和用户动作给预警服务复用。
- `backend/internal/scheduler/scheduler.go`：注册预警评估与按新鲜度补采集任务。
- `backend/internal/api/api.go`：注入预警服务并注册路由。
- `backend/cmd/server/main.go`：组装预警存储、服务和 scheduler 依赖。

**前端新增：**

- `frontend/components/alerts/alert-rule-dialog.tsx`：四步规则编辑器。
- `frontend/components/alerts/alert-event-list.tsx`：事件列表和筛选。
- `frontend/components/alerts/alert-rule-list.tsx`：订阅规则列表。
- `frontend/components/alerts/alert-channel-list.tsx`：复用通知渠道编辑和测试发送。
- `frontend/components/alerts/alert-delivery-list.tsx`：通知发送记录。
- `frontend/components/alerts/alert-event-detail-dialog.tsx`：事件样本、动作和恢复详情。
- `frontend/components/alerts/alert-overview.tsx`：四个指标卡。

**前端修改：**

- `frontend/app/notifications-page.tsx`：改为预警中心页面，保留现有通知渠道和发送记录能力。
- `frontend/components/monitor/monitor-header.tsx`：将入口名称显示为“预警中心”，保留 `/notifications` 路由兼容。
- `frontend/lib/api-types.ts`：增加预警 API 类型。
- `frontend/lib/queries.ts`：增加规则、事件、汇总、设置和动作查询。
- `frontend/src/main.tsx`：增加 `/alerts` 页面入口，并将 `/notifications` 指向兼容入口。

**测试新增或修改：**

- `backend/internal/alert/conditions_test.go`
- `backend/internal/alert/state_test.go`
- `backend/internal/alert/service_test.go`
- `backend/internal/alert/actions_test.go`
- `backend/internal/storage/alerts_test.go`
- `backend/internal/api/alerts_test.go`
- `frontend/components/alerts/alert-rule-dialog.test.ts`
- `frontend/components/alerts/alert-page-completeness.test.ts`

### 任务 1：建立预警领域类型和持久化模型

**文件：**

- 创建：`backend/internal/alert/types.go`
- 修改：`backend/internal/storage/model.go`
- 修改：`backend/internal/storage/storage.go`
- 修改：`backend/internal/storage/settings.go`
- 创建：`backend/internal/storage/alerts.go`
- 创建：`backend/internal/storage/alerts_test.go`

- [ ] **步骤 1：编写存储模型测试**

覆盖以下行为：

```go
func TestAlertRuleStateUniqueTarget(t *testing.T) {
    // 同一 rule + station + target 只能有一个状态行。
}

func TestAlertEventUpsertOpenEvent(t *testing.T) {
    // 同一去重键重复写入时更新现有未恢复事件，而不是创建第二条。
}

func TestAlertSettingsDefaults(t *testing.T) {
    // 评估 60 秒、倍率/用量/IP 60 秒、余额 300 秒。
}
```

- [ ] **步骤 2：运行测试确认模型接口尚不存在**

运行：

```text
go test ./internal/storage ./internal/alert
```

预期：编译失败，原因是预警模型、存储方法和 `internal/alert` 包尚未创建。

- [ ] **步骤 3：增加类型和 GORM 模型**

`types.go` 定义稳定枚举和配置结构：

```go
type TargetType string
const (
    TargetRelayAccount TargetType = "relay_account"
    TargetRelayUser    TargetType = "relay_user"
)

type MetricKind string
const (
    MetricRateChange       MetricKind = "rate_change"
    MetricBalanceLow       MetricKind = "balance_low"
    MetricAccountSpend     MetricKind = "account_spend"
    MetricAccountStability MetricKind = "account_stability"
    MetricUserSpend        MetricKind = "user_spend"
    MetricUserMultiIP      MetricKind = "user_multi_ip"
)

type RuleSpec struct {
    Metric MetricKind
    Window time.Duration
    Threshold float64
    RecoveryThreshold *float64
    MinRequests int64
    TriggerConsecutive int
    RecoveryConsecutive int
    BaselineMultiplier *float64
    TrustedIPs []string
}

type AlertMessageData struct {
    Event AlertEvent
    Rule AlertRule
    ActionResults []AlertActionLog
    Recovered bool
}

type AlertState string
const (
    StateNormal     AlertState = "normal"
    StatePending    AlertState = "pending"
    StateFiring     AlertState = "firing"
    StateRecovering AlertState = "recovering"
    StateRecovered  AlertState = "recovered"
    StateUnknown    AlertState = "unknown"
)
```

`storage/model.go` 增加 `AlertRule`、`AlertRuleState`、`AlertEvent`、`AlertActionLog`、`AlertMetricSample` 和 `AlertRuleNotificationChannel`。规则的 `ConditionJSON`、`ActionJSON`、`RecoveryJSON` 使用 text 保存，稳定的目标类型、指标类型、启用状态和严重级别单独建列并加索引。

扩展 `RelayUsageEvent`：新增 `UserExternalID string` 和 `IPAddress string`，保留 `UserEmail` 作为展示和旧数据回退字段。

- [ ] **步骤 4：增加存储方法**

在 `storage/alerts.go` 实现：

```go
ListAlertRules(filter AlertRuleFilter) ([]AlertRule, error)
FindAlertRule(id uint) (*AlertRule, error)
CreateAlertRule(rule *AlertRule) error
UpdateAlertRule(rule *AlertRule) error
DeleteAlertRule(id uint) error
GetOrCreateAlertRuleState(key AlertStateKey) (*AlertRuleState, error)
SaveAlertRuleState(state *AlertRuleState) error
UpsertOpenAlertEvent(event *AlertEvent) error
RecoverAlertEvent(id uint, at time.Time, summary string) error
ListAlertEvents(filter AlertEventFilter) ([]AlertEvent, error)
ListAlertActionLogs(eventID uint) ([]AlertActionLog, error)
SaveAlertMetricSample(sample *AlertMetricSample) error
```

所有状态和事件写入使用事务；活动事件使用 `rule_id + target_type + station_id + target_external_id` 去重。

- [ ] **步骤 5：增加预警设置默认值和迁移**

在 `storage/settings.go` 加入：

```go
SettingAlertsEnabled
SettingAlertsEvaluationIntervalSeconds
SettingAlertsRateIntervalSeconds
SettingAlertsUsageIntervalSeconds
SettingAlertsBalanceIntervalSeconds
```

默认值分别为 `true`、`60`、`60`、`60`、`300`。低于 30 秒的评估或采集间隔拒绝保存，余额间隔最低 60 秒。

- [ ] **步骤 6：运行存储测试并提交**

运行：

```text
go test ./internal/storage ./internal/alert
```

预期：存储模型、唯一键、默认设置和事件 upsert 测试通过。

```text
git add backend/internal/alert backend/internal/storage docs/superpowers/plans/2026-10-15-alert-automation.md
git commit -m "feat(预警): 增加规则与事件持久化模型"
```

### 任务 2：实现条件计算和状态机

**文件：**

- 创建：`backend/internal/alert/conditions.go`
- 创建：`backend/internal/alert/conditions_test.go`
- 创建：`backend/internal/alert/state.go`
- 创建：`backend/internal/alert/state_test.go`

- [ ] **步骤 1：编写失败测试**

测试固定阈值、倍率变化、滑动窗口、基线倍数、最少请求数、IP 去重和滞回：

```go
func TestEvaluateRateChangeUsesAbsolutePercentage(t *testing.T) {}
func TestEvaluateSpendWindowRequiresMinimumRequests(t *testing.T) {}
func TestEvaluateSpendBaselineNeedsAmountAndMultiplier(t *testing.T) {}
func TestEvaluateMultiIPIgnoresTrustedIPs(t *testing.T) {}
func TestStateFiresAfterThreeConsecutiveViolations(t *testing.T) {}
func TestStateRecoversAfterThreeHealthyEvaluations(t *testing.T) {}
func TestUnknownDataDoesNotFireBusinessAlert(t *testing.T) {}
```

- [ ] **步骤 2：运行指定测试确认失败**

运行：

```text
go test ./internal/alert -run 'TestEvaluate|TestState' -count=1
```

预期：失败，原因是计算函数和状态机尚未实现。

- [ ] **步骤 3：实现统一观测值和条件评估**

定义不依赖数据库的输入：

```go
type Observation struct {
    ObservedAt time.Time
    Value      float64
    RequestCount int64
    ErrorCount int64
    DistinctIPs int
    Valid      bool
    Reason     string
}

type ConditionResult struct {
    Violated bool
    Unknown  bool
    Actual   float64
    Summary  string
}

func EvaluateCondition(rule RuleSpec, observation Observation, baseline *Baseline) ConditionResult
```

所有窗口使用 UTC 时间和 `[now-window, now]` 范围；百分比变化使用绝对值；旧值缺失、过期或能力不支持返回 `Unknown`；稳定性 P95 使用排序后的 nearest-rank 计算。

- [ ] **步骤 4：实现状态转换**

```go
type StateInput struct {
    Previous AlertRuleState
    Condition ConditionResult
    TriggerConsecutive int
    RecoveryConsecutive int
    Now time.Time
}

type StateTransition struct {
    Next AlertRuleState
    OpenEvent bool
    RecoverEvent bool
}

func Transition(input StateInput) StateTransition
```

`unknown` 不增加触发或恢复计数；`pending` 达到触发次数后转 `firing`；`firing` 满足恢复条件后转 `recovering`，连续达到恢复次数后转 `recovered`。

- [ ] **步骤 5：运行测试并提交**

运行：

```text
go test ./internal/alert -run 'TestEvaluate|TestState' -count=1
```

预期：全部通过。

```text
git add backend/internal/alert
git commit -m "feat(预警): 增加阈值计算与状态机"
```

### 任务 3：接入中转站数据和指标采集

**文件：**

- 修改：`backend/internal/storage/relay_usage.go`
- 修改：`backend/internal/relay/service.go`
- 创建：`backend/internal/alert/collector.go`
- 创建：`backend/internal/alert/collector_test.go`

- [ ] **步骤 1：编写采集聚合测试**

测试账号、用户、窗口、状态码、延迟、IP、关联渠道余额和数据过期：

```go
func TestCollectAccountSpendByWindow(t *testing.T) {}
func TestCollectUserSpendUsesStableExternalID(t *testing.T) {}
func TestCollectStabilityUsesErrorRateAndP95(t *testing.T) {}
func TestCollectUserIPsExcludesTrustedIPs(t *testing.T) {}
func TestCollectBalanceUsesLowestFreshLinkedChannelBalance(t *testing.T) {}
func TestCollectMarksExpiredSourceUnknown(t *testing.T) {}
```

- [ ] **步骤 2：实现存储聚合查询**

在 `relay_usage.go` 增加按中转站、账号、用户和时间窗口读取事件的方法：

```go
ListUsageEventsByAccountWindow(stationID uint, accountExternalID int64, since, until time.Time) ([]RelayUsageEvent, error)
ListUsageEventsByUserWindow(stationID uint, userKey string, since, until time.Time) ([]RelayUsageEvent, error)
```

采集器在 Go 层计算金额、错误率、P95 和 IP 集合，避免不同数据库对 P95 和 JSON 字段的行为差异。

- [ ] **步骤 3：实现 `MetricCollector`**

```go
type MetricCollector interface {
    Collect(ctx context.Context, rule AlertRule, now time.Time) ([]TargetObservation, error)
}

type TargetObservation struct {
    StationID uint
    TargetExternalID string
    TargetName string
    Metric MetricKind
    Observation Observation
    Baseline *Baseline
}
```

`collector.go` 读取现有 `RelayAccount`、关联渠道账号和 `RelayUsageEvent`。余额使用最低有效关联余额；没有有效数据返回 `Valid=false` 和原因。账号倍率使用 `RateMultiplier` 与 `RateObservedAt`，每次有效变化写入 `AlertMetricSample`。

- [ ] **步骤 4：补齐 sub2api 字段映射并保留 newapi 扩展口**

在 relay usage 同步的标准化位置写入 `UserExternalID` 和 `IPAddress`。已有 Sub2API 响应使用远端用户 ID和请求 IP；无法提供稳定用户 ID时回退邮箱。新增适配能力枚举：

```go
type AlertCapability string
const (
    CapabilityUserUsage AlertCapability = "user_usage"
    CapabilityUserIP AlertCapability = "user_ip"
    CapabilityLatency AlertCapability = "latency"
    CapabilityAccountScheduling AlertCapability = "account_scheduling"
    CapabilityUserStatus AlertCapability = "user_status"
)
```

newapi 尚未实现的能力返回 `unsupported`，采集器将对应观测标记为 `unknown`。

- [ ] **步骤 5：运行采集测试并提交**

运行：

```text
go test ./internal/alert ./internal/storage ./internal/relay -run 'TestCollect|TestUsage' -count=1
```

预期：聚合、字段兼容和数据过期测试通过。

```text
git add backend/internal/alert backend/internal/storage backend/internal/relay
git commit -m "feat(预警): 接入账号用户指标采集"
```

### 任务 4：实现规则服务、自动化动作和通知

**文件：**

- 创建：`backend/internal/alert/service.go`
- 创建：`backend/internal/alert/actions.go`
- 创建：`backend/internal/alert/service_test.go`
- 创建：`backend/internal/alert/actions_test.go`
- 修改：`backend/internal/relay/service.go`
- 修改：`backend/internal/notify/dispatcher.go`
- 修改：`backend/internal/notify/format.go`

- [ ] **步骤 1：编写服务和动作失败测试**

```go
func TestEvaluateOpensOneEventAndPersistsState(t *testing.T) {}
func TestEvaluateDoesNotDuplicateFiringEvent(t *testing.T) {}
func TestEvaluateCooldownSuppressesRepeatedNotification(t *testing.T) {}
func TestAutoDisableIsIdempotent(t *testing.T) {}
func TestAutoRecoveryDoesNotOverrideManualChange(t *testing.T) {}
func TestFailedActionIsLoggedAndDoesNotBlockNotification(t *testing.T) {}
```

- [ ] **步骤 2：实现服务依赖和评估入口**

```go
type Service struct {
    rules *storage.Alerts
    stations *storage.RelayStations
    collector MetricCollector
    relay RelayActions
    dispatcher *notify.Dispatcher
    settings *storage.Settings
    log *slog.Logger
}

func (s *Service) EvaluateAll(ctx context.Context, now time.Time) error
func (s *Service) TestRule(ctx context.Context, ruleID uint, now time.Time) ([]PreviewTarget, error)
```

`EvaluateAll` 按启用规则遍历目标，执行条件评估、状态转换、事件 upsert、动作和通知。单条规则或单个目标失败只记录错误，不中断其他目标。

- [ ] **步骤 3：实现动作接口和安全恢复**

```go
type RelayActions interface {
    SetSchedulable(ctx context.Context, stationID uint, accountExternalID int64, schedulable bool) error
    UpdateUserStatus(ctx context.Context, stationID uint, userID int64, status string) error
}
```

动作执行前读取当前状态，写入 `AlertActionLog` 的 before 状态；成功后写入 after 状态和自动动作标识。恢复时比较当前状态和自动设置后的状态，发现人工介入则只记录跳过恢复。

- [ ] **步骤 4：增加内置预警消息格式**

在 `notify/format.go` 增加：

```go
func BuildAlertMessage(event AlertMessageData) Message
```

消息包含规则、目标、实际值、阈值、窗口、动作结果和恢复信息。dispatcher 增加按规则绑定通知渠道发送的方法，继续复用现有重试、去抖和发送日志。

- [ ] **步骤 5：运行服务测试并提交**

运行：

```text
go test ./internal/alert ./internal/notify ./internal/relay -run 'TestEvaluate|TestAuto|TestAlert' -count=1
```

预期：状态持久化、冷却、动作幂等、人工介入和消息格式测试通过。

```text
git add backend/internal/alert backend/internal/notify backend/internal/relay
git commit -m "feat(预警): 增加自动化动作与通知"
```

### 任务 5：接入 scheduler、设置和 API

**文件：**

- 修改：`backend/internal/scheduler/scheduler.go`
- 修改：`backend/internal/api/api.go`
- 创建：`backend/internal/api/alerts.go`
- 修改：`backend/cmd/server/main.go`
- 创建：`backend/internal/api/alerts_test.go`

- [ ] **步骤 1：编写 API 失败测试**

覆盖：规则 CRUD、非法条件、规则测试不执行动作、事件筛选、事件详情、确认、恢复、汇总和设置校验。

```go
func TestAlertsCreateRuleValidatesCondition(t *testing.T) {}
func TestAlertsTestRuleDoesNotExecuteAction(t *testing.T) {}
func TestAlertsEventsFilterByStatusAndTarget(t *testing.T) {}
func TestAlertsUpdateSettingsRejectsTooShortInterval(t *testing.T) {}
```

- [ ] **步骤 2：注册依赖和路由**

在 `api.Deps` 增加 `Alerts *alert.Service`，在 `main.go` 按依赖顺序创建 storage、collector、alert service，并注册：

```text
GET/POST   /api/alerts/rules
GET/PUT/DELETE /api/alerts/rules/:id
POST       /api/alerts/rules/:id/test
GET        /api/alerts/events
GET        /api/alerts/events/:id
POST       /api/alerts/events/:id/ack
POST       /api/alerts/events/:id/recover
GET        /api/alerts/summary
GET/PUT    /api/alerts/settings
```

handler 使用现有 `fail`、`uintParam` 和 JSON 绑定风格；规则 JSON 在 API 层解析并由 alert service 再次校验。

- [ ] **步骤 3：注册每分钟评估任务**

在 scheduler 中增加 `alertEvaluationID` 和运行锁，使用 `@every 1m` 调用 `AlertService.EvaluateAll`。采集刷新任务按照预警设置的倍率、用量/IP 和余额间隔触发已有 monitor/relay 同步，运行锁防止同类任务重叠。

```go
func (s *Scheduler) runAlertEvaluation() {
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()
    if err := s.alerts.EvaluateAll(ctx, time.Now().UTC()); err != nil {
        s.log.Warn("alert evaluation failed", "err", err)
    }
}
```

- [ ] **步骤 4：运行 API 和 scheduler 测试并提交**

运行：

```text
go test ./internal/api ./internal/scheduler ./internal/alert -count=1
```

预期：API 返回结构、输入校验和调度运行锁测试通过。

```text
git add backend/internal/api backend/internal/scheduler backend/cmd/server/main.go
git commit -m "feat(预警): 注册调度与管理接口"
```

### 任务 6：实现预警中心前端

**文件：**

- 创建：`frontend/components/alerts/alert-overview.tsx`
- 创建：`frontend/components/alerts/alert-rule-dialog.tsx`
- 创建：`frontend/components/alerts/alert-event-list.tsx`
- 创建：`frontend/components/alerts/alert-rule-list.tsx`
- 创建：`frontend/components/alerts/alert-channel-list.tsx`
- 创建：`frontend/components/alerts/alert-delivery-list.tsx`
- 创建：`frontend/components/alerts/alert-event-detail-dialog.tsx`
- 修改：`frontend/app/notifications-page.tsx`
- 修改：`frontend/lib/api-types.ts`
- 修改：`frontend/lib/queries.ts`
- 修改：`frontend/components/monitor/monitor-header.tsx`
- 修改：`frontend/src/main.tsx`
- 创建：`frontend/components/alerts/alert-page-completeness.test.ts`

- [ ] **步骤 1：编写页面静态测试**

```ts
test("预警中心保留四个页签和指标区域", () => {
  const source = read("../../app/notifications-page.tsx")
  assert.match(source, /预警事件/)
  assert.match(source, /订阅规则/)
  assert.match(source, /通知渠道/)
  assert.match(source, /发送记录/)
})
```

- [ ] **步骤 2：增加 API 类型和查询 hooks**

增加 `AlertRule`、`AlertEvent`、`AlertActionLog`、`AlertSummary`、`AlertSettings` 和条件配置类型；在 `queries.ts` 增加规则、事件、汇总、设置 hooks，变更成功后使用现有 refresh context 刷新相关页面。

- [ ] **步骤 3：实现指标和四个页签**

沿用已经确认的页面结构：标题第一行，指标第二行，第三行使用方案 A 的横向四个页签。通知渠道页签复用 `NotificationStatus` 和 `NotificationFormDialog`，发送记录页签复用 `NotificationLogs` 的展示和刷新逻辑。

- [ ] **步骤 4：实现规则编辑器**

规则弹窗按四步显示：目标、条件、动作与恢复、通知。根据目标和指标动态显示字段；恢复选择“无自动恢复”时隐藏恢复阈值；适配器能力不支持时禁用对应动作并解释原因。保存前在前端校验时间窗口、阈值、连续次数和冷却时间，后端返回的错误仍显示在表单中。

- [ ] **步骤 5：实现事件详情和操作**

事件列表支持状态、严重级别、目标类型和中转站筛选；详情弹窗展示实际值、阈值、样本、动作日志和通知日志。只有未恢复事件显示确认和手工恢复按钮，手工恢复不会调用自动化动作。

- [ ] **步骤 6：运行前端测试、类型检查和构建并提交**

运行：

```text
pnpm exec tsc --noEmit
    node --experimental-strip-types --test components/alerts/*.test.ts components/ui/*.test.ts
pnpm build
```

预期：新增静态测试通过，TypeScript 无错误，Vite 构建成功。

```text
git add frontend
git commit -m "feat(预警): 增加预警中心页面"
```

### 任务 7：集成验证、兼容性和清理

**文件：**

- 修改：`backend/internal/connector/connector.go` 或新增适配能力文件（仅在统一能力接口需要时）
- 修改：相关 README 或配置示例文件，补充预警设置说明
- 修改：现有 UI 完整性测试（仅在页面入口名称变更导致断言需要更新时）

- [ ] **步骤 1：运行后端全量测试**

```text
go test ./...
```

预期：所有已有和新增测试通过。

- [ ] **步骤 2：运行前端全量检查**

```text
pnpm exec tsc --noEmit
pnpm lint
pnpm build
```

预期：类型检查、lint 和构建均退出码为 0。

- [ ] **步骤 3：检查兼容性行为**

验证以下行为：

- 旧 `/notifications` 路径仍可打开预警中心；
- 旧通知渠道可以继续测试发送；
- 旧 `NotificationChannel.Subscriptions` 不被破坏；
- 没有中转站或没有关联渠道账号时页面显示数据不足，不创建误报；
- newapi 未支持的能力显示禁用状态，不执行伪造动作；
- 自动恢复不会覆盖管理员手工修改；
- 活动事件不会重复创建。

- [ ] **步骤 4：运行交付前差异检查**

```text
git diff --check HEAD~7..HEAD
git status --short
```

预期：没有空白错误；工作区只包含明确的交付文件或为空；当前分支为 `codex/feature-alert-automation`。

- [ ] **步骤 5：最终提交**

```text
git add backend frontend docs
git commit -m "feat(预警): 完成预警中心与自动化处理"
```
