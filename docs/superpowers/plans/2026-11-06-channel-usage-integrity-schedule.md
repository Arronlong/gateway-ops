# 全渠道用量核对与自动修复实现计划

> **执行方式：** 当前会话内按任务顺序实现；每个独立功能单元验证后单独提交。
> **目标：** 移除渠道卡片自动条数校对，新增默认启用、北京时间每日 07:00 执行的全渠道用量对比修复任务，并保留单渠道手动触发。

**架构：** monitor 服务逐渠道复用既有 `RefetchUsageHistoryWithReport`，汇总任务结果；scheduler 使用独立 `Asia/Shanghai` cron，并将手动触发和每日触发统一到相同的任务跟踪入口。配置保存在现有 settings 键值表，前端在后台任务卡片编辑并手动执行。

**技术栈：** Go、Gin、robfig/cron、GORM、React/TypeScript、Node.js node:test。

---

## 文件结构与职责

- `frontend/components/monitor/channel-usage-history.tsx`：移除自动校对查询与卡片状态，不改单渠道手动弹窗和修复流。
- `frontend/components/monitor/channel-usage-history.test.ts`：锁定自动查询移除和手动入口保留。
- `backend/internal/monitor/usage_history_all.go`：定义全渠道处理结果、范围窗口和逐渠道复用逻辑。
- `backend/internal/monitor/usage_history_all_test.go`：覆盖范围计算、跳过和逐渠道失败继续。
- `backend/internal/storage/model.go`、`backend/internal/storage/settings.go`：任务设置字段、持久化键、兼容默认值及校验。
- `backend/internal/storage/settings_test.go`：新设置默认值与读写回归。
- `backend/internal/scheduler/scheduler.go`、`backend/internal/scheduler/task_runs.go`：独立北京时间 cron 生命周期和任务名。
- `backend/internal/scheduler/channel_usage_integrity.go`：任务注册、原子防重、手动触发入口。
- `backend/internal/scheduler/channel_usage_integrity_test.go`：时区、每日时刻、启停更新及重复触发测试。
- `backend/internal/api/sync_settings.go`、`backend/internal/api/task_runs.go`：设置验证与仅对新任务开放的手动执行接口。
- `backend/internal/api/channel_usage_integrity_test.go`：设置及手动触发 API 测试。
- `frontend/lib/api-types.ts`、`frontend/lib/system-settings.ts`：同步设置类型和客户端默认值。
- `frontend/app/system-settings-page.tsx`：每日任务开关、范围、时间、手动触发和历史入口。
- `frontend/app/system-settings-page.test.ts`：后台任务控件、默认值及触发入口回归。

## 任务 1：移除渠道卡片自动条数校对

- [ ] **步骤 1：先增加回归测试**，断言 `ChannelUsageHistory` 不再调用 `useChannelUsageIntegrity` 或渲染自动状态，同时保留 `refetchUsageHistoryStream`、范围筛选与“开始核对并补齐”按钮。
- [ ] **步骤 2：运行测试确认失败**：`node --experimental-strip-types --test frontend/components/monitor/channel-usage-history.test.ts`；预期因自动 hook 仍存在而失败。
- [ ] **步骤 3：最小修改组件**：移除自动校对 hook、仅服务于自动状态的 timer/label/error 展示；保留手动结果、报告、SSE 操作及余额模式判断。
- [ ] **步骤 4：重跑定向测试与现有卡片测试**：`node --experimental-strip-types --test frontend/components/monitor/channel-usage-history.test.ts frontend/components/monitor/channel-cards.test.ts`；预期通过。
- [ ] **步骤 5：审查 diff 后提交该独立 UI 行为变更**，提交仅包含该组件及其测试。

## 任务 2：后端全渠道核对与范围计算

- [ ] **步骤 1：先为范围窗口编写失败测试**：固定传入 `now`，验证 today 按上海时区零点起算、24h/7d 为滚动窗口、all 的起点为零值，且窗口结束固定为 `now`。
- [ ] **步骤 2：运行 `go test ./internal/monitor -run TestUsageHistoryAllWindow`**（在 `backend` 目录）；预期因窗口函数未实现而失败。
- [ ] **步骤 3：实现窗口函数和全渠道编排**：加载 `s.channels.List()` 的全部渠道；手动余额或无上游历史能力的渠道计为 skipped 并写任务日志；其他渠道逐一调用 `RefetchUsageHistoryWithReport(ctx, &channel, since, until)`；记录每个渠道的错误并继续，汇总 total/succeeded/failed/skipped；遵守 context cancellation。
- [ ] **步骤 4：添加全渠道执行测试**，验证一条失败不阻断后续渠道、manual 渠道跳过、结果计数和错误摘要。
- [ ] **步骤 5：运行 `go test ./internal/monitor -run TestUsageHistoryAll`**；预期全部通过并提交后端编排单元。

## 任务 3：任务设置读写和验证

- [ ] **步骤 1：扩展 storage 测试**，验证首次读取默认 enabled=true、range=24h、time=07:00；保存更改后重新读取值一致。
- [ ] **步骤 2：运行 `go test ./internal/storage -run TestSyncSettingsUsageIntegrity`**；预期因字段/存储键缺失失败。
- [ ] **步骤 3：在 `SyncSettings` 增加 `channel_usage_integrity_enabled`、`channel_usage_integrity_range`、`channel_usage_integrity_time`；读取无配置时填默认值，保存时只接受 `today|24h|7d|all` 和有效 `HH:mm`，其余返回校验错误。**
- [ ] **步骤 4：扩展 `syncSettingsInput` 并增加 API 测试**，验证 GET 默认、PUT 正常更新及非法范围/时间返回 400。
- [ ] **步骤 5：运行 `go test ./internal/storage ./internal/api -run 'SyncSettings|UsageIntegrity'`**；通过后提交配置/API 单元。

## 任务 4：北京时间定时任务与共用手动触发

- [ ] **步骤 1：先编写 scheduler 测试**，验证任务 cron 使用 `Asia/Shanghai`、07:00 下一次触发时间正确；修改设置会替换旧项，关闭时移除；运行中再次触发被拒绝。
- [ ] **步骤 2：运行 `go test ./internal/scheduler -run TestChannelUsageIntegritySchedule`**；预期因调度器入口缺失失败。
- [ ] **步骤 3：新增独立 `cron.New(cron.WithSeconds(), cron.WithLocation(loc))` 实例，仅服务本任务；在 Start/Stop 管理生命周期；更新设置时移除并按 `0 分 时 * * *` 注册任务；用 `atomic.Bool` 防止手动和定时运行重叠；两条路径都调用同一个 `track(..., "channel_usage_integrity", ..., fn)` 并使用保存的范围。**
- [ ] **步骤 4：在任务历史 API 增加 `POST /tasks/channel_usage_integrity/run`，只允许该任务标识；冲突时返回 409，接受时返回 202。测试成功、并发冲突和未知任务拒绝。
- [ ] **步骤 5：运行 `go test ./internal/scheduler ./internal/api -run 'ChannelUsageIntegrity|TaskRun'`**；通过后提交调度和手动触发单元。

## 任务 5：系统设置界面

- [ ] **步骤 1：先添加前端测试**，锁定控件默认值、范围选项、保存字段映射、手动 POST 路径及任务历史按钮。
- [ ] **步骤 2：运行 `node --experimental-strip-types --test frontend/app/system-settings-page.test.ts`**；预期在控件缺失时失败。
- [ ] **步骤 3：同步 `SyncSettings` 客户端类型与默认值；在后台任务区域新增卡片，提供启用开关、今天/24 小时/7 天/All 下拉框、`input type=time`（北京时间提示）、运行按钮和 `TaskHistoryButton`；保存复用现有 `/sync-settings` PUT，手动运行禁用重复点击并在完成/冲突时提示与刷新历史。**
- [ ] **步骤 4：运行前端定向测试、TypeScript 检查和 `pnpm build`**；通过后提交设置 UI 单元。

## 全量验证与交付

- [ ] 在 `backend` 执行 `go test ./internal/monitor ./internal/storage ./internal/scheduler ./internal/api`。
- [ ] 在 `frontend` 执行新增定向 Node 测试、项目 TypeScript 检查及 `pnpm build`。
- [ ] 审查工作区差异，确认用户既有改动未混入提交；逐条核对规格验收标准。
- [ ] 使用 `C:/Users/cclsu/Desktop/gatewayops/start.bat` 重启当前服务；有界等待并检查 `http://localhost:3010/` 与 `http://127.0.0.1:8418/healthz`。
- [ ] 汇报提交、测试证据、服务健康状态及任何仍存在的限制；不推送、不合并、不删除分支。
## 计划自检记录

- 已覆盖渠道页、监控编排、设置持久化、调度器、API、前端后台任务和最终验证。
- 已明确测试先行、失败确认、最小实现、定向验证和独立提交边界。
- 已明确 Asia/Shanghai、默认 24 小时、默认 07:00、手动触发和单渠道流程保留。


## 实施完成与验证记录（2026-11-06）

- 渠道页自动条数校对已移除，单渠道手动流程保留。
- 全渠道编排、配置持久化、独立北京时间调度、后台手动触发与执行记录均已完成。
- 审查发现 All 截止时间漂移，已改为整批一次计算安全截止，并以两个渠道的真实重拉入口和本地上游替身验证共享窗口。
- `go test ./...`：后端全量通过。
- Node.js 前端全量回归：45 个文件、268 项测试通过。
- `tools/usage_integrity_schedule_e2e.cjs`：1440 和 390 像素宽度均通过；在非北京浏览器时区下验证默认 07:00、范围保存、关闭定时任务后手动执行、执行记录、画册与列表的单渠道修复以及零自动条数校对请求。
- 浏览器验收使用本地真实页面与 API 替身；后端集成测试使用临时 SQLite 和 HTTP 替身，未触发真实付费模型请求或真实全渠道修复。
