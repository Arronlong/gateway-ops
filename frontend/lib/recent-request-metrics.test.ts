import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22
import { recentRequestMetrics } from "./recent-request-metrics.ts"
const now = Date.parse("2026-10-14T08:00:00Z")
const sample = (minutes: number, extra = {}) => ({ created_at: new Date(now - minutes * 60000).toISOString(), ...extra })
test("一小时窗口排除旧记录、无效和未来时间，不补齐更早记录", () => {
  const result = recentRequestMetrics([sample(61), sample(60), sample(2), sample(-1), {created_at: "invalid"}], now)
  assert.equal(result.sampleCount, 2)
})
test("先选最新60次，再按指标排除未知状态", () => {
  const rows = Array.from({length: 80}, (_, i) => sample(i / 2, {success_known: i !== 0, success: i < 30, cache_known: true, cache_read_tokens: i < 15 ? 4 : 0}))
  const result = recentRequestMetrics(rows.reverse(), now)
  assert.equal(result.sampleCount, 60)
  assert.equal(result.successCount, 29)
  assert.equal(result.successKnownCount, 59)
  assert.equal(result.cacheHitRate, 0.25)
  assert.equal(result.cacheReadTokens, 60)
})
test("未知状态和已知零命中率分别处理", () => {
  assert.equal(recentRequestMetrics([sample(1, {input_tokens: 12, cache_read_tokens: 0})], now).cacheHitRate, null)
  assert.equal(recentRequestMetrics([sample(1, {cache_known: true, cache_read_tokens: 0})], now).cacheHitRate, 0)
  assert.equal(recentRequestMetrics([sample(1, {success_known: false, success: true, status_code: 200})], now).successRate, null)
})
test("中转调用的 HTTP 状态和缓存字段独立统计", () => {
  const result = recentRequestMetrics([sample(1,{status_code: 200,cache_known:true,cache_read_tokens:5}),sample(2,{status_code:500,cache_known:true}),sample(3,{status_code:0})], now)
  assert.equal(result.successRate, 0.5)
  assert.equal(result.cacheHitRate, 0.5)
  assert.equal(result.sampleCount, 3)
  assert.equal(recentRequestMetrics([], now).successRate, null)
})
