import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22 原生 TypeScript 测试需要显式扩展名。
import { defaultSystemSettings, pelicanTimezoneOptions, pelicanWeekdayOptions } from "./system-settings.ts"
// @ts-expect-error Node 22 原生 TypeScript 测试需要显式扩展名。
import { addPelicanPlan, defaultPelicanPlan, hasConfiguredPelicanPlan, normalizePelicanPlans, pelicanPlanValidationIssues, removePelicanPlan, updatePelicanPlan } from "./pelican.ts"

test("运营管理默认显示", () => {
  assert.equal(defaultSystemSettings.operations_management_enabled, true)
})

test("测智页面展示和任务执行默认分别开启", () => {
  assert.equal(defaultSystemSettings.pelican_page_enabled, true)
  assert.equal(defaultSystemSettings.pelican_tasks_enabled, true)
})

test("测智作品保留数量默认沿用三十份", () => {
  assert.equal(defaultSystemSettings.pelican_history_count, 30)
})

test("自动测智允许时段默认覆盖一周七天", () => {
  assert.deepEqual(defaultSystemSettings.pelican_active_weekdays, [0, 1, 2, 3, 4, 5, 6])
  assert.deepEqual(pelicanWeekdayOptions.map(day => day.value), [1, 2, 3, 4, 5, 6, 0])
})

test("自动测智有效时段默认沿用当前系统时区", () => {
  assert.equal(defaultSystemSettings.pelican_active_timezone, "Asia/Shanghai")
  assert.ok(pelicanTimezoneOptions.some(option => option.value === defaultSystemSettings.pelican_active_timezone))
})

test("监控指标统计时长默认为一小时", () => {
  assert.equal(defaultSystemSettings.recent_metrics_window_hours, 1)
})

test("测智默认为一个独立方案并使用现有默认值", () => {
  assert.deepEqual(defaultSystemSettings.pelican_plans, [defaultPelicanPlan()])
  assert.equal(defaultSystemSettings.pelican_model, "")
  assert.match(defaultSystemSettings.pelican_prompt, /SVG/)
  assert.equal(defaultSystemSettings.pelican_reasoning_effort, "medium")
  assert.equal(defaultSystemSettings.pelican_parallel_count, 1)
  assert.equal(defaultSystemSettings.pelican_cron_expression, "*/30 * * * *")
})

test("旧设置迁移为一个方案且不按旧并行份数复制", () => {
  const plans = normalizePelicanPlans({
    pelican_model: "first\nsecond",
    pelican_prompt: "legacy prompt",
    pelican_reasoning_effort: "high",
    pelican_parallel_count: 8,
  })

  assert.deepEqual(plans, [{ model: "first\nsecond", prompt: "legacy prompt", reasoning_effort: "high" }])
})

test("新方案优先于旧字段且空列表回退为一个默认方案", () => {
  assert.deepEqual(normalizePelicanPlans({
    pelican_plans: [{ model: "new", prompt: "new prompt", reasoning_effort: "low" }],
    pelican_model: "legacy",
    pelican_prompt: "legacy prompt",
    pelican_reasoning_effort: "high",
  }), [{ model: "new", prompt: "new prompt", reasoning_effort: "low" }])
  assert.deepEqual(normalizePelicanPlans({ pelican_plans: [], pelican_model: "legacy" }), [defaultPelicanPlan()])
})

test("方案归一化最多保留八项并补充缺失字段", () => {
  const plans = normalizePelicanPlans({
    pelican_plans: [{ model: "model", prompt: "prompt" }, ...Array.from({ length: 9 }, () => ({}))],
  })

  assert.equal(plans.length, 8)
  assert.deepEqual(plans[0], { model: "model", prompt: "prompt", reasoning_effort: "medium" })
  assert.deepEqual(plans[1], normalizePelicanPlans({})[0])
})

test("单独编辑方案不修改其他项或原始对象", () => {
  const plans = [defaultPelicanPlan(), { model: "second", prompt: "second prompt", reasoning_effort: "high" }]
  const updated = updatePelicanPlan(plans, 0, { model: "first" })

  assert.deepEqual(updated[0], { ...plans[0], model: "first" })
  assert.deepEqual(updated[1], plans[1])
  assert.equal(plans[0].model, "")
  assert.notEqual(updated, plans)
})

test("方案增删保持一至八项限制", () => {
  const one = [defaultPelicanPlan()]
  assert.equal(removePelicanPlan(one, 0), one)
  assert.equal(addPelicanPlan(Array.from({ length: 8 }, () => defaultPelicanPlan())).length, 8)
  assert.equal(addPelicanPlan(one).length, 2)
  assert.equal(removePelicanPlan(addPelicanPlan(one), 1).length, 1)
})

test("单个默认方案允许未配置而多方案必须完整填写", () => {
  assert.deepEqual(pelicanPlanValidationIssues([defaultPelicanPlan()]), [])
  assert.ok(pelicanPlanValidationIssues([defaultPelicanPlan(), defaultPelicanPlan()]).some(issue => issue.field === "model"))
  assert.ok(pelicanPlanValidationIssues([{ ...defaultPelicanPlan(), prompt: "  " }]).some(issue => issue.field === "prompt"))
  assert.ok(pelicanPlanValidationIssues([{ ...defaultPelicanPlan(), model: "m".repeat(257) }]).some(issue => issue.field === "model"))
  assert.ok(pelicanPlanValidationIssues([{ ...defaultPelicanPlan(), prompt: "p".repeat(32001) }]).some(issue => issue.field === "prompt"))
  assert.ok(pelicanPlanValidationIssues([{ ...defaultPelicanPlan(), reasoning_effort: "invalid" }]).some(issue => issue.field === "reasoning_effort"))
})

test("手动测试从任一已配置方案判断并兼容旧响应", () => {
  assert.equal(hasConfiguredPelicanPlan({ pelican_plans: [defaultPelicanPlan(), { ...defaultPelicanPlan(), model: "model-c" }] }), true)
  assert.equal(hasConfiguredPelicanPlan({ pelican_plans: [{ ...defaultPelicanPlan(), model: " , " }] }), false)
  assert.equal(hasConfiguredPelicanPlan({ pelican_model: "legacy-model" }), true)
  assert.equal(hasConfiguredPelicanPlan({ pelican_model: "  " }), false)
})

test("模型列表兼容中文分隔符且校验与后端字节上限一致", () => {
  assert.equal(hasConfiguredPelicanPlan({ pelican_plans: [{ ...defaultPelicanPlan(), model: "  ，；model-a，model-b" }] }), true)
  assert.ok(pelicanPlanValidationIssues([{ ...defaultPelicanPlan(), model: "模型".repeat(86) }]).some(issue => issue.field === "model"))
  assert.ok(pelicanPlanValidationIssues([{ ...defaultPelicanPlan(), prompt: "题".repeat(11000) }]).some(issue => issue.field === "prompt"))
})
