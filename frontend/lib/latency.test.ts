import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22 native TypeScript tests require the explicit .ts extension.
import { formatOutputTokenSpeed, latencyBucketTones, latencyGranularityOptionsForRange, normalizeLatencyGranularity, outputTokenSpeed, p50, p95 } from "./latency.ts"

test("short ranges keep the full latency granularity options", () => {
  assert.deepEqual(
    latencyGranularityOptionsForRange("today").map((item) => item.value),
    ["auto", "5", "10", "30", "60", "180", "360", "720"],
  )
  assert.deepEqual(
    latencyGranularityOptionsForRange("24h").map((item) => item.value),
    ["auto", "5", "10", "30", "60", "180", "360", "720"],
  )
})

test("long ranges keep automatic and hourly-or-coarser granularities", () => {
  assert.deepEqual(
    latencyGranularityOptionsForRange("7d").map((item) => item.value),
    ["auto", "60", "180", "360", "720"],
  )
  assert.equal(normalizeLatencyGranularity("30d", "30"), "60")
  assert.equal(normalizeLatencyGranularity("all", "5"), "60")
  assert.equal(normalizeLatencyGranularity("7d", "auto"), "auto")
})

test("output token speed divides by total duration", () => {
  assert.equal(outputTokenSpeed(900, 10000), 90)
  assert.equal(outputTokenSpeed(1200, 7000), 1200 * 1000 / 7000)
  assert.equal(outputTokenSpeed(900, null), null)
  assert.equal(outputTokenSpeed(900, 0), null)
  assert.equal(formatOutputTokenSpeed(100), "100 tok/s")
})

test("p50 uses the median of available finite values", () => {
  assert.equal(p50([1, 3, 2]), 2)
  assert.equal(p50([1, 2, 3, 4]), 2.5)
  assert.equal(p50([null, 4, Number.NaN, 2]), 3)
  assert.equal(p50([null, Number.POSITIVE_INFINITY]), null)
})

test("p95 uses the same linear interpolation as backend latency buckets", () => {
  assert.equal(p95([10, 20, 30, 40, 100]), 88)
  assert.equal(p95([null, 0, 100]), 95)
  assert.equal(p95([]), null)
})

test("\u65f6\u95f4\u6876\u7684\u8bf7\u6c42\u7ed3\u679c\u4e0d\u4f1a\u56e0\u5ef6\u8fdf\u7f3a\u5931\u800c\u663e\u793a\u6210\u65e0\u8c03\u7528", () => {
  const bucket = {
    start: "2026-10-15T10:12:00Z",
    end: "2026-10-15T10:36:00Z",
    request_count: 12,
    success_rate: 0,
    first_token_p50_ms: null,
    first_token_p95_ms: null,
    duration_p50_ms: null,
    duration_p95_ms: null,
    output_token_speed_p50: null,
    output_token_speed_p95: null,
  }
  assert.deepEqual(latencyBucketTones(bucket), ["bad", "bad", "bad", "bad"])
  assert.deepEqual(latencyBucketTones({ ...bucket, success_rate: 1 }), ["good", "good", "good", "good"])
  assert.deepEqual(latencyBucketTones({ ...bucket, success_rate: 0.5 }), ["warn", "warn", "warn", "warn"])
  assert.deepEqual(latencyBucketTones({ ...bucket, request_count: 0 }), [null, null, null, null])
  assert.deepEqual(latencyBucketTones({ ...bucket, success_rate: null }), [null, null, null, null])
})

test("\u65f6\u95f4\u6876\u540c\u65f6\u8003\u8651\u8c03\u7528\u5931\u8d25\u548c\u9ad8\u5ef6\u8fdf", () => {
  const bucket = {
    start: "2026-10-15T10:12:00Z",
    end: "2026-10-15T10:36:00Z",
    request_count: 12,
    success_rate: 1,
    first_token_p50_ms: 100,
    first_token_p95_ms: 10_000,
    duration_p50_ms: 60_000,
    duration_p95_ms: 180_000,
    output_token_speed_p50: null,
    output_token_speed_p95: null,
  }
  assert.deepEqual(latencyBucketTones(bucket), ["good", "warn", "warn", "bad"])
  assert.deepEqual(latencyBucketTones({ ...bucket, success_rate: 0 }), ["bad", "bad", "bad", "bad"])
  assert.deepEqual(latencyBucketTones({ ...bucket, success_rate: null }), ["good", "warn", "warn", "bad"])
})
