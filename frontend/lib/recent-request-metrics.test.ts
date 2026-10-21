import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22
import { recentRequestMetrics } from "./recent-request-metrics.ts"

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
