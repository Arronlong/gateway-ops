# 分组监控最近调用与时间稳定性实现计划

> **面向 AI 代理的工作者：** 本计划按测试先行执行；每个步骤完成后运行对应验证命令，不覆盖工作区已有的其他未提交改动。

**目标：** 让分组监控同时展示最近调用和时间稳定性，并使用与渠道管理一致的 P50/P95 四分位状态图、悬浮统计和卡片内调用明细。

**架构：** 后端在现有公开分组监控响应中基于安全的分组调用事件生成时间桶四分位数据；前端复用 `LatencyStabilityLane` 的四格图和 Tooltip，将分组调用适配为该组件支持的样本类型。分组明细不再弹窗，而是在当前卡片内展开。

**技术栈：** Go、Gin、React、TypeScript、Vite、Tailwind CSS。

---

### 任务 1：增加分组监控时间桶四分位聚合

**文件：**
- 修改：`backend/internal/relay/group_monitor.go`
- 测试：`backend/internal/relay/group_monitor_test.go`

- [ ] 编写失败测试，覆盖时间窗口、请求数、成功率以及首字/总耗时 P50/P95。
- [ ] 运行 `go test ./internal/relay -run TestBuildPublicGroupMonitorLatencyBuckets -count=1` 确认测试因聚合函数不存在而失败。
- [ ] 实现分组监控公开时间桶类型和聚合函数，并将结果挂载到分组响应。
- [ ] 重新运行上述测试，确认通过。

### 任务 2：把分组监控接入四格状态图和行内明细

**文件：**
- 修改：`frontend/lib/api-types.ts`
- 修改：`frontend/components/monitor/latency-stability-lane.tsx`
- 修改：`frontend/app/public-relay-monitor-page.tsx`

- [ ] 扩展分组监控 API 类型，支持时间桶和分组调用作为明细样本。
- [ ] 让延时泳道支持分组调用样本和 `inline` 明细呈现，默认渠道管理仍使用原有弹窗行为。
- [ ] 分组卡片增加“最近调用”和“时间稳定性”两条泳道，悬浮展示 P50/P95，点击在卡片内展开明细。

### 任务 3：验证交付链路

**文件：**
- 修改：无

- [ ] 运行 Go 相关测试。
- [ ] 运行前端 TypeScript 构建和现有前端测试脚本。
- [ ] 检查交付文件中的乱码、替换字符和关键字段，并核对工作区差异只包含本需求改动。
