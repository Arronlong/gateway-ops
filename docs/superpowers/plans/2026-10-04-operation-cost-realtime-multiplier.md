# 成本管理实时倍率实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 在成本管理中增加以中转站调用成本为主口径的实时倍率，同时展示渠道余额消耗作为对账指标，并提供按中转站拆分的收入、成本和倍率。

**架构：** 后端在现有 `/operations/summary` 汇总链路中复用每个中转站的 `UsageStats`，将用户收入与已知账号调用成本汇总为主指标；同时读取渠道余额消耗和当前余额，但不把两套口径相加，避免同一请求重复计成本。前端在成本管理页展示总指标、口径说明和中转站明细。

**技术栈：** Go、Gin、GORM、React、TypeScript、现有 API 查询与 UI 组件。

---

### 任务 1：锁定实时成本计算契约

**文件：**
- 修改：`backend/internal/api/operations_test.go`
- 创建：`backend/internal/api/operation_costs.go`
- 测试：`backend/internal/api/operations_test.go`

- [ ] **步骤 1：编写失败的测试**

覆盖以下行为：正成本返回收入除以成本的倍率；零成本返回空倍率；中转站成本汇总只累加已知账号成本，并在账号成本未知或读取失败时标记不完整。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/api -run 'Test(OperationMultiplier|RelayCostSummary)'`

预期：FAIL，原因是实时成本计算类型或函数尚不存在。

- [ ] **步骤 3：编写最少实现代码**

新增纯函数和 JSON 结构：`operationMultiplier`、`relayStationCostView`、`operationCostSummary`；倍率仅在成本大于零时返回，成本完整性由失败账号数和未知成本账号数共同决定。

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/api -run 'Test(OperationMultiplier|RelayCostSummary)'`

预期：PASS。

### 任务 2：扩展成本管理汇总 API

**文件：**
- 修改：`backend/internal/api/operations.go`
- 修改：`backend/internal/api/operations_test.go`
- 修改：`frontend/lib/api-types.ts`
- 修改：`frontend/lib/queries.ts`（仅在类型或查询契约需要时）

- [ ] **步骤 1：编写失败的 API 契约测试**

验证 `operationSummaryView` 序列化出 `realtime_multiplier`、`relay_cost_amount`、`relay_cost_complete`、`relay_failed_accounts`、`channel_consumption_amount`、`channel_balance_amount`、`channel_cost_variance` 和 `relay_stations` 字段，并且分母使用中转站调用成本而不是渠道消耗。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/api -run 'TestOperationsSummary'`

预期：FAIL，原因是响应结构缺少新增字段或汇总逻辑尚未接入。

- [ ] **步骤 3：实现汇总逻辑**

在 `operationsSummary` 中：
1. 列出渠道并计算当前渠道余额；
2. 用 `Rates.ChannelConsumptionTotals(since)` 汇总期间渠道余额消耗；
3. 对每个中转站调用 `Relay.UsageStats`，累加 `UserCharge` 和 `AccountCostKnown` 的 `AccountCost`；
4. 用 `user_charge / relay_cost` 计算总实时倍率；
5. 计算 `channel_consumption - relay_cost` 作为对账差异，不将两者相加；
6. 对失败或未知成本情况返回完整性字段和中转站明细，不伪造倍率。

- [ ] **步骤 4：更新 TypeScript 类型**

让 `OperationSummary` 与后端响应一一对应，倍率和不可靠的金额字段使用可空类型，保留现有账本和本地号池字段。

- [ ] **步骤 5：运行后端测试**

运行：`go test ./internal/api ./internal/relay ./internal/storage`

预期：PASS。

### 任务 3：成本管理页面展示总指标与中转站明细

**文件：**
- 修改：`frontend/components/operations/summary-cards.tsx`
- 修改：`frontend/app/operations-costs-page.tsx`
- 修改：`frontend/lib/api-types.ts`

- [ ] **步骤 1：编写失败的前端纯函数测试**

验证倍率格式化在无成本时显示占位符，且渠道消耗和中转站调用成本使用不同标签，防止用户误认为两者应相加。

- [ ] **步骤 2：运行测试验证失败**

运行：`pnpm exec vitest run frontend/lib/operation-costs.test.ts`

预期：FAIL，原因是格式化函数或测试文件尚不存在。

- [ ] **步骤 3：实现页面**

在现有四张经营卡片下增加：实时倍率、中转站调用成本、当前渠道余额、渠道实际消耗、渠道对账差异；增加说明条，明确实时倍率分母为中转站调用成本，渠道消耗仅用于对账。新增中转站表格展示收入、调用成本、实时倍率、完整性和失败账号数。

- [ ] **步骤 4：运行前端验证**

运行：`pnpm exec vitest run frontend/lib/operation-costs.test.ts` 与 `pnpm run build`。

预期：测试通过，生产构建退出码为 0。

### 任务 4：全链路回归与交付检查

**文件：**
- 修改：任务 1-3 中的文件

- [ ] **步骤 1：运行完整后端测试**

运行：`go test ./...`

- [ ] **步骤 2：运行完整前端检查**

运行：`pnpm run lint` 与 `pnpm run build`。

- [ ] **步骤 3：检查交付文件编码和链路**

检查新增或修改文件中的连续问号、Unicode 替换字符、关键中文文案；确认后端 JSON 字段、TypeScript 类型、查询响应和页面展示字段名称完全一致。

- [ ] **步骤 4：查看差异**

运行：`git diff -- backend/internal/api/operations.go backend/internal/api/operation_costs.go backend/internal/api/operations_test.go frontend/lib/api-types.ts frontend/components/operations/summary-cards.tsx frontend/app/operations-costs-page.tsx`

确认不包含与本功能无关的回退或覆盖。
