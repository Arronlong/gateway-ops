import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 原生 TypeScript 测试需要显式扩展名。
import { duplicateStatisticsChannels } from "./channel-statistics.ts"

const base = {id: 1, name: "渠道 A", type: "newapi", site_url: "https://upstream.example/", balance_mode: "auto", credential_mode: "password", username: "user@example.com"}

test("同站点相同登录账号才提示可能重复统计，暂停监控不影响识别", () => {
  const other = {...base, id: 2, name: "渠道 B", site_url: "https://upstream.example", monitor_enabled: false}
  assert.deepEqual(duplicateStatisticsChannels(base, [base, other]).map((item: typeof base) => item.id), [2])
  assert.equal(duplicateStatisticsChannels(base, [{...other, username: "another@example.com"}]).length, 0)
  assert.equal(duplicateStatisticsChannels(base, [{...other, site_url: "https://elsewhere.example"}]).length, 0)
  assert.equal(duplicateStatisticsChannels(base, [{...other, site_url: "https://upstream.example/another"}]).length, 0)
})

test("排除汇总和手动余额不会产生共享自动账号提醒", () => {
  const other = {...base, id: 2}
  assert.equal(duplicateStatisticsChannels({...base, exclude_from_statistics: true}, [other]).length, 0)
  assert.equal(duplicateStatisticsChannels(base, [{...other, exclude_from_statistics: true}]).length, 0)
  assert.equal(duplicateStatisticsChannels({...base, balance_mode: "manual"}, [other]).length, 0)
  assert.equal(duplicateStatisticsChannels({...base, username: ""}, [{...other, username: ""}]).length, 0)
})

test("Token 展示名称不能当账号身份，NewAPI 用实际 User ID 比较", () => {
  const token = {...base, credential_mode: "token", newapi_user_id: "42"}
  assert.equal(duplicateStatisticsChannels(token, [{...token, id: 2, username: "different label"}]).length, 1)
  assert.equal(duplicateStatisticsChannels(token, [{...token, id: 2, newapi_user_id: "43"}]).length, 0)
  assert.equal(duplicateStatisticsChannels({...base, type: "sub2api", credential_mode: "token"}, [{...base, id: 2, type: "sub2api", credential_mode: "token"}]).length, 0)
})

test("附加账号同样参与重复检测，同一渠道只提醒一次", () => {
  const other = {...base, id: 2, username: "different", accounts: [{credential_mode: "password", username: base.username}]}
  assert.equal(duplicateStatisticsChannels(base, [other, {...other, id: 3, exclude_from_statistics: true}]).length, 1)
})
