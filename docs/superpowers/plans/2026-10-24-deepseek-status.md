# DeepSeek 服务状态实现与验收计划

**目标：** 在原有服务状态页增加可切换的 DeepSeek 官方状态。

**架构：** Go 适配 Flashduty 公开接口到现有状态模型；React 复用既有显示。

**技术栈：** Go、Gin、httptest、React、TypeScript、Vite。

## 文件职责

- `C:/Users/cclsu/Desktop/gateway-ops/backend/internal/api/deepseek_service_status.go`：接口适配与统一状态映射。
- `C:/Users/cclsu/Desktop/gateway-ops/backend/internal/api/deepseek_service_status_test.go`：官方数据格式、故障、维护、历史和失败边界。
- `C:/Users/cclsu/Desktop/gateway-ops/backend/internal/api/public_service_status.go`：并发数据源注册。
- `C:/Users/cclsu/Desktop/gateway-ops/frontend/app/public-service-status-page.tsx`：图标、来源、分组展开和历史缺失显示。
- `C:/Users/cclsu/Desktop/gateway-ops/frontend/lib/api-types.ts`：明确 DeepSeek 服务标识。

## 单一功能交付步骤

- [x] 先用定向测试确认适配器缺失的失败。
- [x] 实现当前状态、历史、分组和事件映射，确认定向测试通过。
- [x] 将 DeepSeek 加入并发数据源，复用标签、图标和刷新。
- [x] 测试复现历史失败误报健康、计划维护和同事件数据优先级问题。
- [x] 修正边界并重跑定向测试。
- [x] 执行 `go test ./...`、`go vet ./internal/api`、`pnpm exec tsc --noEmit` 和 `pnpm run build`。
- [x] 完成浏览器页面验收、编码检查和差异审查，保留无关用户改动。
- [x] 所有必要验证通过后创建单一功能提交，不推送。

## 最终验证记录

- 后端全量测试和 `go vet ./internal/api` 通过。
- `pnpm exec tsc --noEmit` 和 `pnpm run build` 通过；构建仍有既有的大体积包提示。
- 显式开启的官方接口测试通过：7 个组件、5 个分组、14 条历史事件。默认测试不请求外网。
- 浏览器使用新适配器的真实官方响应，仅在测试上下文替换接口返回。标签切换、分组展开、历史提示和移动端检查通过，无脚本错误或页面横向溢出。
- 当前执行策略拒绝后端重载，未绕过限制，未改变既有服务。日常页面需后端重启后才加载新功能。
- ESLint 未安装，现有 `pnpm run lint` 无法运行；`go test -race` 以 `0xc0000139` 退出，不将两者宣称为通过。
- 已保留无关的 `frontend/prototypes/` 用户文件，不纳入提交。
