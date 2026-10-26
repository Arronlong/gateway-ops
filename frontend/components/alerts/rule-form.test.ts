import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 的原生测试运行器需要显式扩展名。
import { initialRuleForm, rulePayload, targetChanged } from "./rule-form.ts"

test("编辑保留订阅渠道、范围、停用状态和零恢复开关", () => {
  const form = initialRuleForm({ name: "消耗保护", target_type: "relay_user", metric: "user_spend", enabled: false, severity: "critical", channel_ids: [3], scope: '{"station_ids":[7],"target_ids":["42"]}', condition: '{"threshold":10,"window_seconds":900}', action: "{}", recovery: '{"enabled":false}', cooldown_seconds: 3600 } as never)
  const payload = rulePayload(form)
  assert.deepEqual(payload.channel_ids, [3])
  assert.deepEqual(payload.scope, { station_ids: [7], target_ids: ["42"] })
  assert.equal(payload.enabled, false)
  assert.equal(payload.recovery.enabled, false)
  assert.equal(payload.cooldown_seconds, 3600)
})

test("切换目标不会遗留不兼容的指标和动作", () => {
  const form = targetChanged({ ...initialRuleForm(), action: "disable_account_scheduling" }, "relay_user")
  assert.equal(form.metric, "user_spend")
  assert.equal(form.action, "none")
})

test("渠道对象使用渠道指标并保存动态筛选条件", () => {
  const form = targetChanged(initialRuleForm(), "channel")
  assert.equal(form.metric, "balance_low")
  const payload = rulePayload({
    ...form,
    name: "低余额渠道",
    filterLogic: "all",
    filters: [
      { field: "score", operator: "gte", value: "80" },
      { field: "monitor_enabled", operator: "eq", value: "true" },
    ],
  })
  assert.deepEqual(payload.scope, {
    station_ids: [],
    target_ids: [],
    logic: "all",
    filters: [
      { field: "score", operator: "gte", value: "80" },
      { field: "monitor_enabled", operator: "eq", value: "true" },
    ],
  })
})

test("用户余额指标允许零余额阈值且保留恢复滞回", () => {
  const form = { ...initialRuleForm(), name: "用户余额", targetType: "relay_user" as const, metric: "user_balance_low" as const, threshold: "0", recoveryThreshold: "5" }
  const payload = rulePayload(form)
  assert.equal(payload.condition.threshold, 0)
  assert.equal(payload.condition.recovery_threshold, 5)
})

test("metrics without request counts ignore the minimum requests field", () => {
  for (const metric of ["channel_rate_change", "user_balance_low"] as const) {
    const payload = rulePayload({ ...initialRuleForm(), name: metric, targetType: metric === "user_balance_low" ? "relay_user" : "channel", metric, minRequests: "invalid" })
    assert.equal(payload.condition.min_requests, 0)
  }
})

test("incomplete scope filters cannot silently widen a rule", () => {
  const form = { ...initialRuleForm(), name: "scoped", targetType: "channel" as const, metric: "balance_low" as const, filters: [{ field: "name", operator: "contains", value: " " }] }
  assert.throws(() => rulePayload(form))
})

test("次数条件、跨站 ID 和恢复滞回进行保存前校验", () => {
  const form = { ...initialRuleForm(), name: "测试" }
  assert.throws(() => rulePayload({ ...form, trigger: "4", hitWindow: "3" }))
  assert.throws(() => rulePayload({ ...form, targetIDs: "42" }))
  assert.throws(() => rulePayload({ ...form, recoveryThreshold: "30" }))
  assert.throws(() => rulePayload({ ...form, threshold: "" }))
  assert.throws(() => rulePayload({ ...form, stationID: "7", targetIDs: "someone@example.com", action: "disable_account_scheduling" }))
})
