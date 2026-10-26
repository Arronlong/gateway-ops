import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22
import { recentRequestMetrics } from "./recent-request-metrics.ts"

test("TPS 按配置样本的总输出与总耗时加权计算", () => {
  const result = recentRequestMetrics([
    sample(1, {model: "model-a", output_tokens: 40, duration_ms: 2000}),
    sample(2, {model: "model-b", output_tokens: 90, duration_ms: 3000}),
    sample(3, {model: "image-model", request_type: "image", output_tokens: 1000, duration_ms: 1000}),
    sample(4, {model: "probe-model", request_type: "probe", output_tokens: 1000, duration_ms: 1000}),
  ], now, 1, 4)
  assert.equal(result.tps, 26)
  assert.equal(result.tpsScore, 52)
  assert.deepEqual(result.tpsModels, ["model-a", "model-b"])
})

test("TPS 无有效文字计量时不可用且独立评分最高百分", () => {
  assert.equal(recentRequestMetrics([sample(1, {request_type: "image", output_tokens: 100, duration_ms: 1000})], now).tps, null)
  assert.equal(recentRequestMetrics([sample(1, {output_tokens: 0, duration_ms: 1000})], now).tpsScore, 0)
  assert.equal(recentRequestMetrics([sample(1, {output_tokens: 25, duration_ms: 1000})], now).tpsScore, 50)
  assert.equal(recentRequestMetrics([sample(1, {output_tokens: 50, duration_ms: 1000})], now).tpsScore, 100)
  assert.equal(recentRequestMetrics([sample(1, {output_tokens: 100, duration_ms: 1000})], now).tpsScore, 100)
})

test("TPS 与缓存及成功率共用统计时长与最近数量", () => {
  const result = recentRequestMetrics([
    sample(90, {output_tokens: 1000, duration_ms: 1000}),
    sample(4, {output_tokens: 1000, duration_ms: 1000}),
    sample(3, {output_tokens: 100, duration_ms: 1000}),
    sample(2, {output_tokens: 80, duration_ms: 2000}),
  ], now, 1, 2)
  assert.equal(result.sampleCount, 2)
  assert.equal(result.tps, 60)
  assert.deepEqual(result.tpsModels, [])
})

test("TPS 保留省略零输出的耗时并排除无效或未完成计量", () => {
  const completed = sample(1, {model: "text-model", output_tokens: 100, duration_ms: 1000})
  assert.equal(recentRequestMetrics([completed, sample(2, {duration_ms: 99000})], now).tps, 1)
  assert.equal(recentRequestMetrics([sample(1, {duration_ms: 1000})], now).tps, 0)
  const invalid = [
    sample(2, {output_tokens: 0, duration_ms: 99000, in_progress: true}),
    sample(3, {output_tokens: 0, duration_ms: 99000, attempt_status: "in_progress"}),
    ...[0, -1, null, NaN, Infinity].map(duration_ms => sample(4, {output_tokens: 100, duration_ms})),
    ...[-1, NaN, Infinity].map(output_tokens => sample(5, {output_tokens, duration_ms: 1000})),
  ]
  assert.equal(recentRequestMetrics([completed, ...invalid], now).tps, 100)
})

test("TPS 在渠道仅提供模型名时排除图片补计量", () => {
  const imageModels = ["gpt-image-1", "GROK-IMAGINE", "dall-e-3", "imagen-4", "seedream-4", "image-generation", "flux-dev"]
  for (const model of imageModels) {
    const result = recentRequestMetrics([sample(1, {model, output_tokens: 1000, duration_ms: 1000})], now)
    assert.equal(result.tps, null, model)
    assert.equal(result.tpsScore, null, model)
  }
})

test("配置数量同时限制最近样本和统计分母", () => {
  const rows = Array.from({length: 201}, (_, i) => sample(i / 5 + 1, {input_tokens: 10, status_code: i < 20 ? 200 : 500}))
  const result = recentRequestMetrics(rows.reverse(), now, 1, 20)
  assert.equal(result.sampleCount, 20)
  assert.equal(result.sampleLimit, 20)
  assert.equal(result.successAttemptCount, 20)
  assert.equal(result.successRate, 1)
  assert.equal(result.cacheInputTokens, 200)
  assert.equal(recentRequestMetrics(rows, now, 1, 200).sampleCount, 200)
  assert.equal(recentRequestMetrics(rows, now, 1, 201).sampleLimit, 200)
  for (const limit of [0, -1, 1.5, NaN]) assert.equal(recentRequestMetrics(rows, now, 1, limit).sampleLimit, 60)
})

const now = Date.parse("2026-10-14T08:00:00Z")
const sample = (minutes: number, extra = {}) => ({created_at: new Date(now - minutes * 60000).toISOString(), ...extra})

test("selects the one-hour window before the 45-second cutoff", () => {
  const result = recentRequestMetrics([sample(61), sample(60), sample(2), sample(0), sample(-1), {created_at: "invalid"}], now)
  assert.equal(result.sampleCount, 2)
})

test("uses the configured integer hour window up to 24 hours", () => {
  const result = recentRequestMetrics([sample(60 * 24 - 1), sample(60 * 24 + 1)], now, 24)
  assert.equal(result.windowHours, 24)
  assert.equal(result.sampleCount, 1)
})

test("limits the selected records to the newest 60 before calculating weighted tokens", () => {
  const rows = Array.from({length: 80}, (_, i) => sample(i / 2 + 1, {
    input_tokens: 10,
    cache_read_tokens: i < 15 ? 4 : 0,
    cache_creation_tokens: 6,
    success_known: i !== 0,
    success: i < 30,
  }))
  const result = recentRequestMetrics(rows.reverse(), now)
  assert.equal(result.sampleCount, 60)
  assert.equal(result.successCount, 29)
  assert.equal(result.successAttemptCount, 59)
  assert.equal(result.cacheReadTokens, 60)
  assert.equal(result.cacheInputTokens, 600)
  assert.equal(result.cacheCreationTokens, 360)
  assert.equal(result.cacheTokenTotal, 1020)
  assert.equal(result.cacheHitRate, 60 / 1020)
})

test("counts ordinary input without cache fields and shows unknown only without tokens", () => {
  assert.equal(recentRequestMetrics([sample(1, {input_tokens: 12})], now).cacheHitRate, 0)
  assert.equal(recentRequestMetrics([sample(1, {cache_known: true, input_tokens: 12, cache_read_tokens: 0})], now).cacheHitRate, 0)
  assert.equal(recentRequestMetrics([sample(1, {cache_known: true, input_tokens: 0, cache_read_tokens: 0})], now).cacheHitRate, null)
})

test("uses cache creation detail fields when the aggregate field is absent", () => {
  const result = recentRequestMetrics([sample(1, {input_tokens: 100, cache_read_tokens: 20, cache_creation_5m_tokens: 5, cache_creation_1h_tokens: 5})], now)
  assert.equal(result.cacheCreationTokens, 10)
  assert.equal(result.cacheHitRate, 20 / 130)
})

test("counts retries as separate attempts and excludes auxiliary, probe, and cyber calls", () => {
  const result = recentRequestMetrics([
    sample(1, {status_code: 500, request_type: "sync"}),
    sample(2, {status_code: 200, request_type: "sync"}),
    sample(3, {status_code: 200, request_type: "auxiliary"}),
    sample(4, {status_code: 500, request_type: "probe"}),
    sample(5, {status_code: 200, request_type: "cyber"}),
    sample(6, {status_code: 0, request_type: "text", in_progress: true}),
    sample(7, {status_code: 202, request_type: "image", in_progress: true}),
    sample(8, {status_code: 200, request_type: "sync", is_cyber: true}),
  ], now)
  assert.equal(result.successCount, 1)
  assert.equal(result.successAttemptCount, 4)
  assert.equal(result.successRate, 0.25)
})

test("keeps HTTP success rate independent from token cache rate", () => {
  const result = recentRequestMetrics([
    sample(1, {status_code: 200, input_tokens: 10, cache_read_tokens: 5}),
    sample(2, {status_code: 500, input_tokens: 10}),
    sample(3, {status_code: 0}),
  ], now)
  assert.equal(result.successRate, 0.5)
  assert.equal(result.successAttemptCount, 2)
  assert.equal(result.cacheHitRate, 5 / 25)
  assert.equal(result.sampleCount, 3)
  assert.equal(recentRequestMetrics([], now).successRate, null)
})
