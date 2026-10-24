# 定时任务执行记录实现计划

**目标：** 按已确认方案提供设置页入口、分页执行记录与单次日志。

**架构：** Go/GORM 新增 task_runs 持久化表；调度器通过 context 隔离单次日志；Gin 鉴权接口提供列表和详情；React Sheet 复用现有组件。

- [x] 先写失败测试，实现持久化、统计、分页与任务隔离。
- [x] 包装全部 cron 入口，记录部分失败、超时、崩溃和服务中断；日志限长并脱敏。
- [x] 添加鉴权 GET /api/tasks/:task/runs 与 /api/tasks/:task/runs/:id 及测试。
- [x] 添加执行记录抽屉，统计、分页、日志详情与自动刷新；将开关标签更正为已启用。
- [x] 运行后端测试、前端测试、类型检查与构建，检查编码和改动边界。

保留全部现有未提交改动；不补造上线前的执行数据；鹈鹕全局记录表示每30秒的调度检查，日志内展示实际账号执行。

## 实现和验证

- task_runs 独立持久化调度记录，重启后保留；未完成记录恢复为中断失败。
- 完成写入失败先重试3次，再由后台队列每5秒补写；结束时间不随重试改变。正常关停时等待作业和补写线程结束，并在5秒总时限内完成最后补写。
- 执行日志通过context关联，完成时持久化；单条64KiB上限，敏感认证信息脱敏。
- 调度器包含渠道同步、中转站倍率与快照、每日余额、清理、鹈鹕调度检查和兼容扫描。
- Go全量测试通过；前端176项测试、TypeScript检查、Vite构建通过。
- 真实PostgreSQL独立schema事务回滚测试通过，不写入现有业务表。
- 无头Chrome配合模拟API验证桌面与移动端、分页、日志、空态、错误态和关闭抽屉后停止轮询。

## 复现

```powershell
# 后端：在 backend 目录运行
 go test ./...
# 设置 GATEWAYOPS_TEST_DSN 后运行真实数据库集成测试
 go test ./internal/storage -run TestTaskRunsPostgresLifecycle -count=1
# 前端：在 frontend 目录运行
 node --experimental-strip-types --test components/system/task-history.test.ts
 node node_modules/typescript/bin/tsc --noEmit --incremental false
 node node_modules/vite/bin/vite.js build
 node node_modules/vite/bin/vite.js preview --port 18310 --strictPort --host 127.0.0.1
# 根目录：PLAYWRIGHT_MODULE 指向本地playwright包时可运行浏览器回归
 node tools/task_history_e2e.cjs
```
