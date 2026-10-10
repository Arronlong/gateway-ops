# 分组监控时间筛选实现计划

**目标：** 新增平铺的今天、24小时、7天、30天筛选，默认选中24小时。

**架构：** 前端将range参数传给公开分组监控接口，后端按同一时间窗口过滤调用并重建60个时间桶。沿用最近60次调用的状态统计口径，今天、7天、30天使用与渠道管理相同的日历区间。服务端与浏览器缓存都按站点与时间范围隔离，快速切换时丢弃旧请求结果。

**技术栈：** Go、Gin、React、TypeScript、Node原生测试。

## 执行步骤

- [x] 添加并运行后端时间窗口、边界过滤、缓存隔离和上游参数测试，确认实现前失败。
- [x] 添加并运行前端默认值、筛选选项、接口路径和缓存键测试，确认实现前失败。
- [x] 修改 `backend/internal/relay/group_monitor.go` 、 `backend/internal/relay/service.go` 和 `backend/internal/api/public_relay_monitor.go`，传递时间范围、过滤样本和隔离缓存。
- [x] 修改 `frontend/lib/public-relay-monitor.ts`、 `frontend/lib/api-types.ts` 和 `frontend/app/public-relay-monitor-page.tsx`，增加按钮、默认选项、缓存隔离和请求防串数。
- [x] 运行 `go test ./...`、 `node --experimental-strip-types --test lib/*.test.ts`、 `pnpm exec tsc --noEmit` 和 `pnpm build`。
- [x] 检查UTF-8编码、中文字段、替换字符及全链路。不提交现有其他变更，不主动重启服务。

## 验证记录

- 后端全量测试通过。
- 前端28项单元测试通过，分组监控相关文件的TypeScript检查和生产构建通过。
- 全局TypeScript检查仍有本地号池页面的既有expires_at类型错误，未修改该无关页面。
- UTF-8编码和前后端筛选参数、缓存、聚合链路检查通过。
- 未进行浏览器端实时验收，未重启服务。
