# 顶部监控下拉与多站分组监控实现计划

> **面向 AI 代理的工作者：** 在当前工作区内执行；仓库已有其他未提交改动，本任务不得提交或覆盖无关文件。

**目标：** 将“分组监控”和“服务状态”组织到顶部“监控”下拉菜单中，并让分组监控正确支持多个中转站。

**架构：** 顶部导航只负责打开统一入口 `/public/relay-monitor`，不读取或猜测具体中转站。公开分组监控页面通过只返回 `id/name` 的安全站点列表识别可用中转站：单站自动进入，多站展示站点选择器；原有 `/public/relay-monitor/:stationID` 直链继续保留。

**技术栈：** Go + Gin、React 19、React Router、Radix Select/DropdownMenu、TypeScript、Vite。

---

### 任务 1：顶部导航

**文件：**
- 修改：`frontend/components/monitor/monitor-header.tsx`

- [x] 顶部使用“监控”下拉菜单。
- [x] 下拉项固定顺序：1. 分组监控；2. 服务状态。
- [x] 两项都使用新标签页打开。
- [x] 分组监控固定打开 `/public/relay-monitor`，不默认选择第一个站。
- [x] 移动端菜单保持相同结构和顺序。

### 任务 2：公开中转站选项

**文件：**
- 修改：`backend/internal/api/public_relay_monitor.go`
- 新建：`backend/internal/api/public_relay_monitor_test.go`

- [x] 新增 `GET /api/public/relay-stations`。
- [x] 响应仅包含 `id` 和 `name`，不暴露 BaseURL、API Key 或其他配置。
- [x] 添加测试验证公开字段边界。

### 任务 3：多站分组监控页面

**文件：**
- 修改：`frontend/src/main.tsx`
- 修改：`frontend/app/public-relay-monitor-page.tsx`
- 新建：`frontend/lib/public-relay-monitor.ts`
- 新建：`frontend/lib/public-relay-monitor.test.ts`

- [x] 新增无参数路由 `/public/relay-monitor`。
- [x] 单站时自动跳转到对应 `/:stationID`。
- [x] 多站时不默认选择，页面展示中转站选择器。
- [x] 切换站点时更新 URL 并重新加载对应监控数据。
- [x] 保留原有具体站点公开直链。
- [x] 添加单站、多站、显式站点和空列表的选择逻辑测试。

### 任务 4：验证

- [x] `go test ./...`
- [x] `node --test lib/public-relay-monitor.test.ts`
- [x] `pnpm build`
- [x] `git diff --check`
- [x] 确认本次修改文件没有新增 TypeScript 错误；仓库现存的全量 `tsc` 错误位于正在开发的 `relay-stations-page.tsx`。
