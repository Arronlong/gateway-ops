# New API 中转站管理实现计划

**目标：** 实现已批准的完整 New API 中转站管理方案。

**架构：** 独立的 New API 管理适配器输出现有 relay 服务使用的快照和用量结构。站点类型和加密凭据决定管理请求分发。

**技术栈：** Go、GORM、PostgreSQL、React、TypeScript。

- [ ] 在 `backend/internal/relay/newapi_admin_test.go` 编写失败测试，运行 `go test ./internal/relay` 确认功能缺失。
- [ ] 扩展 `backend/internal/storage/model.go` 并增加 `backend/migrations/0005_newapi_relay_stations.sql`，兼容旧站点。
- [ ] 实现 `backend/internal/relay/newapi_admin.go`：凭据、会话、管理接口、分页、字段映射。
- [ ] 接入 `backend/internal/relay/service.go` 与 `risk.go`：配置、同步、账号分组、调度、测试、复制。
- [ ] 扩展 `backend/internal/api/relay_stations.go` 与 `frontend/lib/api-types.ts`，隐藏敏感凭据。
- [ ] 修改 `frontend/app/relay-stations-page.tsx`，增加站点类型、凭据模式、能力说明。
- [ ] 更新 `README.md`，运行 `go test ./...`、前端 Node 测试、`pnpm exec tsc --noEmit` 和 `pnpm build`。
- [ ] 审查差异，检查中文编码和 `git diff --check`，提交功能变更。
