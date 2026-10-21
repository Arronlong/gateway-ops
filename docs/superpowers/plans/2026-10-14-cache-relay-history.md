# 缓存指标和测试历史实现计划

更新说明（2026-10-15）：本文的缓存指标部分保留；链路测试历史部分已由账号鹈鹕测智替换，见 `2026-10-15-account-pelican` 设计与计划。以下旧测试历史描述仅作为过程记录，不代表当前实现。

目标：在主线隔离工作树中完成两项功能并验证。按当前会话直接执行，不要求用户再次选择。

- [x] 编写并运行 `frontend/lib/recent-request-metrics.test.ts`：验证一小时边界、最近 60 条、未知状态、未命中与无数据区别。运行 `node --experimental-strip-types --test lib/recent-request-metrics.test.ts`。
- [x] 实现 `frontend/lib/recent-request-metrics.ts` 和 `frontend/components/monitor/recent-request-metrics.tsx`；接入渠道卡片、渠道列表、中转账号卡片和列表。
- [x] 在连接器、同步和用量模型传递 `cache_known`；NewAPI 解析缓存字段和消费/错误类型。编写连接器测试验证分页和日志身份。
- [x] 在 `backend/internal/storage/relay_test_runs.go` 建立批次模型、事务写入、最新批次和 30 条历史读取；在 `storage.go` 注册迁移，删除站点时清理批次。
- [x] 在 `backend/internal/relay/test_history.go` 封装已有账号/分组测试，归档成功和失败；在 API 注册最新和历史查询。测试 SSE 失败、空响应、脱敏和保留数量。
- [x] 实现 `frontend/components/relay/test-history.tsx` 和 `frontend/lib/relay-test-report.ts`，站点列表显示最新结果，弹窗时间轴和动态 SVG。测试后刷新历史，快照无数据时也可读取。
- [x] 运行 Go 全量测试、前端 Node 测试、TypeScript 类型检查和 Vite 构建；自审时间窗口和数据隔离。所有交付文件检查 UTF-8、连续问号和替换字符。

## 验证记录

- `go test ./...`：后端全量通过。
- `node --experimental-strip-types --test lib/*.test.ts components/monitor/*.test.ts`：88 项通过，0 失败。
- `pnpm exec tsc --noEmit`：通过。
- `pnpm build`：生产构建通过。
- Playwright 本地演示数据：圆环比例、30 条历史、SVG 动画和下载、测试后刷新、空历史、桌面和移动端展示通过。旧账号快照为空时，仍使用最新本地用量。
- 数据库使用 sqlmock 验证站点锁顺序、事务保存与裁剪、回滚、历史隔离和 JSON 解码。本次验证未连接真实上游。
