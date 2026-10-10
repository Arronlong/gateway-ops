import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22 native TypeScript tests require the explicit .ts extension.
import { latencyGranularityOptionsForRange, normalizeLatencyGranularity } from "./latency.ts"

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
