# 本地号池订阅筛选与到期排序实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 从中转站账号 `credentials.plan_type` 和 `credentials.expires_at` 读取订阅信息，在本地号池单独账号列表中提供订阅筛选、到期时间显示与排序，并统一筛选菜单交互。

**架构：** 同步中转站账号时从 credentials 原始字段解析订阅类型与到期时间，保存到本地 `relay_accounts` 快照；号池 overview API 将字段返回给前端。前端单独账号视图使用渠道管理风格的筛选菜单，筛选条件集中管理，排序使用统一排序菜单，缺失日期排在末尾。

**技术栈：** Go、Gin、GORM、PostgreSQL、React、TypeScript、Radix DropdownMenu、Vite。

---

## 文件清单

- 修改：`backend/internal/relay/service.go` —— 解析 credentials.plan_type / credentials.expires_at，并在同步和探测更新时写入本地快照。
- 修改：`backend/internal/relay/service_test.go` —— 覆盖 credentials 字段优先级、日期解析和快照字段构造。
- 修改：`backend/internal/storage/model.go` —— 增加本地账号到期时间字段。
- 修改：`backend/internal/storage/relay_stations.go` —— 探测更新同步订阅与到期时间。
- 修改：`backend/internal/api/relay_stations.go` —— overview 响应增加订阅和到期时间字段。
- 修改：`backend/migrations/0001_init.sql` —— 为已有数据库增加到期时间列。
- 修改：`frontend/lib/api-types.ts` —— 扩展 `RelayAccountView` 类型。
- 修改：`frontend/app/local-pool-page.tsx` —— 增加订阅筛选、到期时间列、排序和渠道管理风格筛选菜单。

## 任务 1：先写后端解析测试

- [ ] 为 `remoteAccountPlan` 增加测试，验证 `credentials.plan_type` 优先于旧的 `extra` 字段，返回值保留可展示的订阅名称规范。
- [ ] 为 `remoteAccountExpiresAt` 增加测试，验证 RFC3339 字符串、Unix 秒/毫秒数字和空值。
- [ ] 运行 `go test ./internal/relay -run 'TestRemoteAccount(Plan|ExpiresAt)'`，确认新测试在实现前失败。

## 任务 2：实现后端字段解析与存储

- [ ] 在 `remoteAccountCredentials` 增加原始字段解析辅助函数，不改变现有 credentials.Raw 的兼容行为。
- [ ] 在 `storage.RelayAccount` 增加 `ExpiresAt *time.Time`，并在同步快照、单账号探测更新中写入。
- [ ] 在 `0001_init.sql` 增加幂等的 `expires_at` 列；启动时 GORM AutoMigrate 负责现有数据库升级。
- [ ] 在 API `relayAccountView` 增加 `ExpiresAt`，从风险账号快照映射返回。
- [ ] 运行后端定向测试并确认通过。

## 任务 3：增加前端字段与纯数据处理

- [ ] 扩展 `RelayAccountView` 的 `account_plan` 和 `expires_at` 类型。
- [ ] 移除本地号池根据账号名/分组名猜套餐的逻辑，直接使用接口返回的 `account_plan`，统一首字母大写展示，空值显示“未识别”。
- [ ] 增加到期时间解析与排序比较函数：有效日期按时间排序，缺失/非法日期固定排在末尾，ID 作为稳定排序兜底。

## 任务 4：改造本地号池筛选交互

- [ ] 将单独账号视图的订阅、平台、状态下拉框集中到 `SlidersHorizontal` 筛选菜单中，使用渠道管理已有的子菜单、当前值徽标和重置入口模式。
- [ ] 订阅选项动态来自当前 OAuth 账号，支持 Team、K12、Pro、Plus 等接口实际返回值。
- [ ] 增加排序菜单，提供“到期时间（近到远）”“到期时间（远到近）”以及现有常用排序项。
- [ ] 在单独账号表格显示订阅类型和到期时间，宽度与横向滚动保持兼容。
- [ ] 聚合视图继续按订阅类型聚合，不受单独账号排序影响。

## 任务 5：验证

- [ ] 运行 `gofmt` 检查修改过的 Go 文件。
- [ ] 运行 `go test ./...`。
- [ ] 运行前端 `pnpm lint`。
- [ ] 运行前端 `pnpm build`。
- [ ] 检查变更文件中的 `???`、U+FFFD，并抽样确认关键中文文案与字段链路。