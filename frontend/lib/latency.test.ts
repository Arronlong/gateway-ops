import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22 native TypeScript tests require the explicit .ts extension.
import { formatOutputTokenSpeed, latencyGranularityOptionsForRange, normalizeLatencyGranularity, outputTokenSpeed, p50, p95 } from "./latency.ts"

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
