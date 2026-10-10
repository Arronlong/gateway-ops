import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"
// @ts-expect-error Node 22 native TypeScript tests require the explicit .ts extension.
import { defaultPublicRelayMonitorRange, isPublicRelayMonitorViewForSelection, publicRelayMonitorCacheKey, publicRelayMonitorPath, publicRelayMonitorRanges, resolvePublicRelayMonitorSelection } from "./public-relay-monitor.ts"

test("single station auto-selects from the generic monitor entry", () => {
  assert.deepEqual(
    resolvePublicRelayMonitorSelection([{ id: 3, name: "A" }], null),
    { mode: "auto", stationID: 3 },
  )
})

test("multiple stations require an explicit choice from the monitor page", () => {
  assert.deepEqual(
    resolvePublicRelayMonitorSelection([{ id: 3, name: "A" }, { id: 8, name: "B" }], null),
    { mode: "choose", stationID: null },
  )
})

test("an explicit route station remains selected even with multiple stations", () => {
  assert.deepEqual(
    resolvePublicRelayMonitorSelection([{ id: 3, name: "A" }, { id: 8, name: "B" }], 8),
    { mode: "selected", stationID: 8 },
  )
})

test("no stations reports an empty state", () => {
  assert.deepEqual(resolvePublicRelayMonitorSelection([], null), { mode: "empty", stationID: null })
})

test("monitor ranges default to 24h and expose the flat range options", () => {
  assert.equal(defaultPublicRelayMonitorRange, "24h")
  assert.deepEqual(publicRelayMonitorRanges.map((item) => item.value), ["today", "24h", "7d", "30d"])
})

test("monitor requests include the selected station and time range", () => {
  assert.equal(publicRelayMonitorPath(3, "24h"), "/public/relay-stations/3/group-monitor?range=24h")
  assert.equal(publicRelayMonitorPath(3, "7d"), "/public/relay-stations/3/group-monitor?range=7d")
})

test("browser cache keys isolate ranges and stations and retire the old schema", () => {
  const current = publicRelayMonitorCacheKey(3, "24h")
  assert.notEqual(current, publicRelayMonitorCacheKey(3, "7d"))
  assert.notEqual(current, publicRelayMonitorCacheKey(8, "24h"))
  assert.match(current, /^gatewayops:group-monitor:v2:/)
})

test("only matching station and range payloads are accepted", () => {
  assert.equal(isPublicRelayMonitorViewForSelection({ station_id: 3, range: "24h" }, 3, "24h"), true)
  assert.equal(isPublicRelayMonitorViewForSelection({ station_id: 3, range: "7d" }, 3, "24h"), false)
  assert.equal(isPublicRelayMonitorViewForSelection({ station_id: 8, range: "24h" }, 3, "24h"), false)
  assert.equal(isPublicRelayMonitorViewForSelection(null, 3, "24h"), false)
  assert.equal(isPublicRelayMonitorViewForSelection(undefined, 3, "24h"), false)
})

test("legacy monitor responses without range retain valid data for the default 24h view", () => {
  assert.equal(isPublicRelayMonitorViewForSelection({ station_id: 3 }, 3, "24h"), true)
  assert.equal(isPublicRelayMonitorViewForSelection({ station_id: 3, range: undefined }, 3, "24h"), true)
})

test("legacy monitor responses must never be relabelled as another station or time range", () => {
  assert.equal(isPublicRelayMonitorViewForSelection({ station_id: 8 }, 3, "24h"), false)
  for (const range of ["today", "7d", "30d"] as const) {
    assert.equal(isPublicRelayMonitorViewForSelection({ station_id: 3 }, 3, range), false)
  }
})

test("public monitor cards do not inject failure-summary explanations into recent calls", () => {
  const page = readFileSync(new URL("../app/public-relay-monitor-page.tsx", import.meta.url), "utf8")
  assert.doesNotMatch(page, /failure_summary/)
  assert.match(page, /formatTime\(group\.latest_call_at\)/)
  assert.match(page, /recentSamples=\{group\.calls\}/)
})

test("public recent-call tooltips use each call latency instead of only its bucket", () => {
  const page = readFileSync(new URL("../components/monitor/latency-stability-lane.tsx", import.meta.url), "utf8")
  assert.match(page, /function RecentSampleTooltip\(\{ sample, bucket \}/)
  assert.match(page, /formatLatency\(sample\.first_token_ms\)/)
  assert.match(page, /formatLatency\(sample\.duration_ms\)/)
  assert.match(page, /当前调用延迟；同时对比所属时间段的 P50\/P95/)
})
