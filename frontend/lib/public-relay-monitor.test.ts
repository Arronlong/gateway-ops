import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"
// @ts-expect-error Node 22 native TypeScript tests require the explicit .ts extension.
import { defaultPublicRelayMonitorRange, isPublicRelayMonitorViewForSelection, publicRelayMonitorCacheKey, publicRelayMonitorPath, publicRelayMonitorRanges, resolvePublicRelayMonitorSelection, publicGroupRecentMetrics } from "./public-relay-monitor.ts"

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
  assert.match(current, /^gatewayops:group-monitor:v3:/)
})

test("group aggregate values reuse channel ring inputs without averaging account percentages", () => {
  const metrics = publicGroupRecentMetrics({
    window_hours: 6, sample_limit: 200, sample_count: 80, latest_at: "2026-10-27T02:00:00Z",
    cache_hit_rate: 0.8, success_rate: 0.5, tps: 20, tps_score: 40,
    success_count: 40, success_attempt_count: 80, cache_input_tokens: 20, cache_read_tokens: 80, cache_creation_tokens: 0,
    tps_output_tokens: 200, tps_duration_seconds: 10, tps_models: ["text"],
  })
  assert.equal(metrics.windowHours, 6)
  assert.equal(metrics.sampleLimit, 200)
  assert.equal(metrics.sampleCount, 80)
  assert.equal(metrics.cacheHitRate, 0.8)
  assert.equal(metrics.successRate, 0.5)
  assert.equal(metrics.tps, 20)
  assert.equal(metrics.tpsScore, 40)
  assert.equal(metrics.successAttemptCount, 80)
  assert.deepEqual(metrics.tpsModels, ["text"])
})

test("missing public group aggregates stay unknown instead of displaying fabricated zeroes", () => {
  const metrics = publicGroupRecentMetrics(undefined)
  assert.equal(metrics.cacheHitRate, null)
  assert.equal(metrics.successRate, null)
  assert.equal(metrics.tps, null)
  assert.equal(metrics.sampleCount, 0)
  assert.deepEqual(metrics.tpsModels, [])
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
  assert.match(page, /function RecentSampleTooltip\(\{ sample, primarySamples, relay, channel \}/)
  assert.match(page, /formatLatency\(sample\.first_token_ms\)/)
  assert.match(page, /formatLatency\(sample\.duration_ms\)/)
  assert.match(page, /matchRelatedRecentCall/)
  assert.doesNotMatch(page, /channel\.links\.length > 0 && bucket/)
  assert.match(page, /RecentSampleTooltip sample=\{sample\} primarySamples=\{primarySamples\} relay=\{relay\} channel=\{channel\}/)
})

test("recent-call comparison uses decoded data labels and the same three metrics as stability", () => {
  const page = readFileSync(new URL("../components/monitor/latency-stability-lane.tsx", import.meta.url), "utf8")
  const pair = page.slice(page.indexOf("function RecentCallPair"), page.indexOf("function RecentSampleTooltip"))
  const tooltip = page.slice(page.indexOf("function RecentSampleTooltip"), page.indexOf("function bucketForSample"))

  assert.match(tooltip, /const valueOrder = relayPairs\.length > 0/)
  assert.match(tooltip, /primaryLabel: "\\u6e20\\u9053\\u6570\\u636e"/)
  assert.match(tooltip, /counterpartLabel: "\\u4e2d\\u8f6c\\u7ad9\\u6570\\u636e"/)
  assert.match(tooltip, /primaryLabel: "\\u4e2d\\u8f6c\\u7ad9\\u6570\\u636e"/)
  assert.match(tooltip, /counterpartLabel: "\\u6e20\\u9053\\u6570\\u636e"/)
  assert.match(tooltip, /flex items-start justify-between gap-3[\s\S]*formatSampleTime\(sample\.created_at\)[\s\S]*RecentValueOrder/)
  assert.match(tooltip, /w-\[min\(20rem,calc\(100vw-32px\)\)\]/)
  assert.doesNotMatch(pair, /\\u6570\\u503c\\u987a\\u5e8f|primaryLabel|counterpartLabel/)
  assert.deepEqual(Array.from(pair.matchAll(/\{ label: [^,]+, metric: "([^"]+)" \}/g), (match) => match[1]), ["first", "duration", "speed"])
  assert.doesNotMatch(pair, /metric: "time"|metric: "model"|metric: "tokens"/)
  assert.match(pair, /grid-cols-3/)
  assert.match(pair, /grid grid-rows-2 gap-0\.5/)
  assert.match(pair, /RecentCallValue sample=\{primary\} metric=\{row\.metric\} \/><RecentCallValue sample=\{counterpart\} metric=\{row\.metric\}/)
  assert.doesNotMatch(pair, /grid-cols-\[68px_1fr_1fr\]/)
})

test("recent-call fallback keeps three core metrics in two rows", () => {
  const page = readFileSync(new URL("../components/monitor/latency-stability-lane.tsx", import.meta.url), "utf8")
  const tooltip = page.slice(page.indexOf("function RecentSampleTooltip"), page.indexOf("function bucketForSample"))

  assert.match(tooltip, /const hasComparison = relayPairs\.length > 0 \|\| channelPairs\.length > 0/)
  assert.match(tooltip, /hasComparison \? "w-\[min\(20rem,calc\(100vw-32px\)\)\]" : "w-fit max-w-\[calc\(100vw-32px\)\]"/)
  assert.match(tooltip, /grid grid-cols-3 gap-px overflow-hidden/)
  assert.match(tooltip, /block font-sans text-\[10px\] text-muted-foreground/)
  assert.match(tooltip, /mt-1 block whitespace-nowrap font-semibold leading-4/)
  assert.doesNotMatch(tooltip, /totalTokens\(sample\)/)
})

test("recent calls remain clickable when they are outside the active time buckets", () => {
  const page = readFileSync(new URL("../components/monitor/latency-stability-lane.tsx", import.meta.url), "utf8")
  const cell = page.slice(page.indexOf("function RecentSampleCell"), page.indexOf("interface RelayCompareConfig"))
  const lane = page.slice(page.indexOf("export function LatencyStabilityLane"))

  assert.match(cell, /onOpen: \(sample: LatencyDetailSample, bucket: LatencyBucket \| null\) => void/)
  assert.match(cell, /onClick=\{\(\) => onOpen\(sample, bucket\)\}/)
  assert.doesNotMatch(cell, /if \(bucket\) onOpen/)
  assert.match(page, /function recentSampleBucket\(sample: LatencyDetailSample\)/)
  assert.match(lane, /function openRecentSample\(sample: LatencyDetailSample, bucket: LatencyBucket \| null\)/)
  assert.match(lane, /if \(bucket\) \{[\s\S]*void openBucket\(bucket\)[\s\S]*return[\s\S]*\}/)
  assert.match(lane, /setSelected\(recentSampleBucket\(sample\)\)/)
  assert.match(lane, /setSamples\(\[sample\]\)/)
})

test("call detail percentile values use two fixed rows", () => {
  const page = readFileSync(new URL("../components/monitor/latency-stability-lane.tsx", import.meta.url), "utf8")
  const detailValue = page.slice(page.indexOf("function DetailPercentileValue"), page.indexOf("function InlineLatencyDetails"))

  assert.match(detailValue, /grid grid-rows-2/)
  assert.match(detailValue, /grid-cols-\[auto_auto\]/)
  assert.match(detailValue, /justify-start/)
  assert.doesNotMatch(detailValue, /justify-between/)
  assert.match(detailValue, />P50</)
  assert.match(detailValue, />P95</)
  assert.match(page, /<DetailPercentileValue p50=\{selected\.first_token_p50_ms\} p95=\{selected\.first_token_p95_ms\} format=\{formatLatency\} \/>/)
  assert.match(page, /<DetailPercentileValue p50=\{selected\.duration_p50_ms\} p95=\{selected\.duration_p95_ms\} format=\{formatLatency\} \/>/)
  assert.match(page, /<DetailPercentileValue p50=\{selected\.output_token_speed_p50\} p95=\{selected\.output_token_speed_p95\} format=\{formatOutputTokenSpeed\} \/>/)
})

test("call detail metric panel aligns with the detail list width", () => {
  const page = readFileSync(new URL("../components/monitor/latency-stability-lane.tsx", import.meta.url), "utf8")
  const dialog = page.slice(page.indexOf("<Dialog open="), page.indexOf("</Dialog>"))

  assert.match(dialog, /mx-3 mt-3 grid grid-cols-2/)
  assert.match(dialog, /sm:grid-cols-\[104px_repeat\(3,minmax\(0,1fr\)\)\]/)
  assert.match(dialog, /overflow-hidden rounded-lg border border-border/)
  assert.doesNotMatch(dialog, /mx-auto|sm:w-fit|w-\[calc\(100%-24px\)\]/)
  assert.doesNotMatch(dialog, /grid grid-cols-2 gap-px border-b border-border bg-border sm:grid-cols-4/)
})
