# 时间筛选自动生效与紧凑布局实现计划

## 已确认的设计

- 预设即选即用；自定义放入弹层，有效的日期变更自动查询，不增加应用按钮。
- 打开弹层只编辑草稿，不替换当前查询；不完整或无效草稿不提交，并显示校验提示。
- 按钮组内部不换行，极窄空间组内横向滚动；弹层不超出视口。
- 本地号池使用 RelayUsageRangePreset，补齐 ALL，沿用现有 range=all 接口。
- 本轮不实施后台预同步改造，不重启服务，不触发真实付费请求。

## 功能单元一：共享时间控件

文件：`frontend/components/ui/time-range-filter.tsx`、`tools/time_range_e2e.cjs`。

- [x] 先在浏览器测试中断言弹层、打开不查询、无效草稿不替换已生效区间、有效区间自动请求及按钮同行。
- [x] 运行 `node tools/time_range_e2e.cjs`，用 `TIME_RANGE_CASE=monitor`、`TIME_RANGE_WIDTHS=390` 定向确认红灯。
- [x] 复用 Popover，以独立的 `draft` 保存日期：

```tsx
const next = { ...current, ...patch }
setDraft(next)
if (serializeTimeRange(next)) onChange(next)
```

- [x] 完成 1440、768、390 宽度下全部八个页面的请求与布局验证，通过后独立提交。

## 功能单元二：本地号池 ALL

文件：`frontend/app/local-pool-page.tsx`、`tools/time_range_e2e.cjs`。

- [x] 先断言 ALL 可见且点击会发送 `range=all`，确认红灯。
- [x] 将页面类型改为 `RelayUsageRangePreset`，预设增加 `{ value: "all", label: "ALL" }`。
- [x] 运行本地号池定向浏览器测试和类型检查，通过后独立提交。

## 收尾验证

- [x] `node --experimental-strip-types --test frontend/lib/*.test.ts`。
- [x] `pnpm exec tsc --noEmit`。
- [x] 全部时间筛选浏览器回归、截图视觉检查与一次生产构建。
- [x] 检查 UTF-8、差异和工作区，不提交其他任务改动。

## 验证记录

- 原有浏览器基线：16 个场景通过。
- 新弹层交互和 ALL 缺失均已先确认预期失败，再完成实现。
- 最终浏览器回归：8 个页面× 1440、768、390 三种宽度，24 个场景通过，页面错误数为 0。
- 本地号池额外完成 320 宽度检查，确认 ALL 的 `range=all` 请求和布局。
- 前端 lib 单元测试 185 项通过；`pnpm exec tsc --noEmit` 和 `pnpm build` 通过。
- 已检查截图、UTF-8、关键中文标签、日期参数及差异；只读审查未发现阻塞问题。
- 未重启服务，未推送，浏览器测试全部拦截业务 API，不访问真实付费模型。
