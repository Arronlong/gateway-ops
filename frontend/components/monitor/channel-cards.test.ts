import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"

const source = readFileSync(new URL("./channel-cards.tsx", import.meta.url), "utf8")
const metricsSource = readFileSync(new URL("./recent-request-metrics.tsx", import.meta.url), "utf8")

test("\u6e20\u9053\u548c\u4e2d\u8f6c\u7ad9\u56db\u9879\u8bc4\u5206\u4f7f\u7528\u7cfb\u7edf\u7edf\u8ba1\u65f6\u957f\u4e0e\u7edf\u4e00\u8bf4\u660e", () => {
  const relaySource = readFileSync(new URL("../../app/relay-stations-page.tsx", import.meta.url), "utf8")
  assert.match(source, /latencyHealthScore\([^\n]*windowHours: uiSettings\.recent_metrics_window_hours/)
  assert.match(relaySource, /latencyHealthScore\([^\n]*windowHours: metricsWindowHours/)
  for (const page of [source, relaySource]) {
    assert.equal((page.match(/const scoreTitle = healthScoreDescription\(/g) ?? []).length, 2)
    assert.doesNotMatch(page, /healthScore \? healthScoreClass\(/)
  }
})

test("\u6e20\u9053\u753b\u518c\u548c\u5217\u8868\u6307\u6807\u5747\u4f7f\u7528\u7cfb\u7edf\u914d\u7f6e\u7684\u7edf\u8ba1\u65f6\u957f", () => {
  const configuredMetricCalls = source.match(/<RecentRequestMetrics\b[^>]*windowHours=\{uiSettings\.recent_metrics_window_hours\}/g) ?? []
  assert.equal(configuredMetricCalls.length, 2)
})

test("\u73af\u5f62\u6307\u6807\u6807\u9898\u52a8\u6001\u663e\u793a\u914d\u7f6e\u7684\u5c0f\u65f6\u6570", () => {
  const dynamicLabels = metricsSource.match(/label=\{`\$\{metrics\.windowHours\}h [^`]+`/g) ?? []
  assert.equal(dynamicLabels.length, 2)
  assert.doesNotMatch(metricsSource, /label="1h /)
})

test("\u6e20\u9053\u641c\u7d22\u4f7f\u7528\u53ef\u4e2d\u65ad\u66f4\u65b0\u4e14\u4e0d\u8986\u76d6\u8f93\u5165\u8349\u7a3f", () => {
  assert.match(source, /startTransition\(\(\) => onValueChange\(draft\)\)/)
  assert.doesNotMatch(source, /setDraft\(value\)/)
  assert.match(source, /channelSearchResetVersion/)
  assert.match(source, /if \(lastResetVersion\.current === resetVersion\) return/)
})
