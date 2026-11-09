# 用户长期未使用实现计划

**目标：** 新增默认 30 天未使用的用户预警，复用通用处理选项。

**架构：** 由用户快照计算未使用天数，复用阈值评估和现有生命周期。用显式能力标记区分从未使用和接口不支持。

**技术栈：** Go、React、TypeScript、Node 原生测试。

- [x] 补充 `backend/internal/alert/user_inactive_test.go` 和 `frontend/components/alerts/rule-form.test.ts`，验证默认值、边界、缺失数据、动态范围和通用停用动作。先运行确认失败。
- [x] 完成 `types.go`、`validation.go`、`conditions.go`、`collector.go` 和独立的 `user_inactivity.go`，接入用户快照能力。
- [x] 完成前端类型、场景切换默认值、天数控件、摘要和通知展示，不改动处理选项。
- [x] 运行 `go test ./internal/alert ./internal/relay`、前端定向测试和 `tsc --noEmit`。
- [x] 使用现有 3010 服务完成浏览器验收，运行全量回归和前端构建。
- [x] 检查编码和差异，仅提交本功能。

验证记录：后端 `go test ./...` 通过；前端 263 项测试、类型检查和生产构建通过。实际页面验证 1440 和 390 宽度、通用处理选项及拦截的保存请求通过。既有启动脚本重启后前后端与代理健康检查均为 200；真实只读试算返回 10 个有效对象，未修改远端用户状态。
