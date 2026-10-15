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

test("次数条件、跨站 ID 和恢复滞回进行保存前校验", () => {
  const form = { ...initialRuleForm(), name: "测试" }
  assert.throws(() => rulePayload({ ...form, trigger: "4", hitWindow: "3" }))
  assert.throws(() => rulePayload({ ...form, targetIDs: "42" }))
  assert.throws(() => rulePayload({ ...form, recoveryThreshold: "30" }))
  assert.throws(() => rulePayload({ ...form, threshold: "" }))
  assert.throws(() => rulePayload({ ...form, stationID: "7", targetIDs: "someone@example.com", action: "disable_account_scheduling" }))
})
