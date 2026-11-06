import type { LatencyBucket, LatencyBucketGranularity, RelayUsageRangePreset } from "@/lib/api-types"
import type { TimeRangeSelection } from "@/lib/time-range"

// @ts-expect-error Node 22 原生 TypeScript 测试需要显式扩展名。
import { recentRequestMetrics, recentRequestWindow, type RecentRequestSample } from "./recent-request-metrics.ts"

export type LatencyMetric = "first" | "duration"
export type LatencyTone = "good" | "warn" | "bad"

export const latencyThresholds = {
  first: { warn: 10_000, bad: 30_000 },
  duration: { warn: 60_000, bad: 180_000 },
} as const

export function latencyTone(value: number | null | undefined, metric: LatencyMetric): LatencyTone {
  if (value == null || !Number.isFinite(value) || value < 0) return "bad"
  const thresholds = latencyThresholds[metric]
  if (value >= thresholds.bad) return "bad"
  if (value >= thresholds.warn) return "warn"
  return "good"
}

export function latencyBucketTones(bucket: LatencyBucket): Array<LatencyTone | null> {
  if (bucket.request_count <= 0) return [null, null, null, null]
  const rate = bucket.success_rate
  const outcome: LatencyTone | null = rate == null || !Number.isFinite(rate) || rate < 0 || rate > 1
    ? null
    : rate === 0 ? "bad" : rate < 1 ? "warn" : "good"
  const tone = (value: number | null | undefined, metric: LatencyMetric): LatencyTone | null => {
    const measured = value == null || !Number.isFinite(value) || value < 0 ? null : latencyTone(value, metric)
    // 有延迟时与浮层使用同一阈值；缺失延迟才回退到请求结果。
    return measured ?? outcome
  }
  return [
    tone(bucket.first_token_p50_ms, "first"),
    tone(bucket.first_token_p95_ms, "first"),
    tone(bucket.duration_p50_ms, "duration"),
    tone(bucket.duration_p95_ms, "duration"),
  ]
}

export const latencyColor: Record<LatencyTone, string> = {
  good: "#10b981",
  warn: "#f59e0b",
  bad: "#ef4444",
}

export const latencyTextClass: Record<LatencyTone, string> = {
  good: "text-emerald-600 dark:text-emerald-400",
  warn: "text-amber-600 dark:text-amber-400",
  bad: "text-red-600 dark:text-red-400",
}

export const latencyTooltipTextClass: Record<LatencyTone, string> = {
  good: "text-emerald-700",
  warn: "text-amber-700",
  bad: "text-red-700",
}

function latencyPerformanceScore(value: number | null | undefined, metric: LatencyMetric) {
  if (value == null || !Number.isFinite(value) || value < 0) return 0
  const { warn, bad } = latencyThresholds[metric]
  if (value <= warn) return 100
  if (value >= bad) return 0
  return Math.max(0, Math.min(100, 100 * (bad - value) / (bad - warn)))
}

export interface LatencyHealthScore {
  score: number | null
  recentLatency: number | null
  stability: number | null
  cacheHit: number | null
  sampleCount: number
  sampleLimit: number
  measuredCount: number
  windowHours: number
  missingMetrics: string[]
  lowConfidence: boolean
}

type HealthSample = RecentRequestSample & {
  first_token_ms?: number | null
  duration_ms?: number | null
}

function average(values: number[]) {
  if (values.length === 0) return null
  return values.reduce((sum, value) => sum + value, 0) / values.length
}

function validLatency(value: number | null | undefined): value is number {
  return value != null && Number.isFinite(value) && value > 0
}

function sampleLatencyScore(sample: HealthSample) {
  const values: number[] = []
  if (validLatency(sample.first_token_ms)) values.push(latencyPerformanceScore(sample.first_token_ms, "first"))
  if (validLatency(sample.duration_ms)) values.push(latencyPerformanceScore(sample.duration_ms, "duration"))
  return average(values)
}

function latencyConsistency(values: number[]) {
  if (values.length < 2) return null
  const mean = average(values)!
  const deviation = Math.sqrt(average(values.map((value) => (value - mean) ** 2))!)
  return 100 / (1 + deviation / mean)
}

function requestStability(samples: readonly HealthSample[]) {
  return average([
    latencyConsistency(samples.map((sample) => sample.first_token_ms).filter(validLatency)),
    latencyConsistency(samples.map((sample) => sample.duration_ms).filter(validLatency)),
  ].filter((value): value is number => value != null))
}

function healthMetricSample(sample: HealthSample) {
  const requestType=sample.request_type?.trim().toLowerCase()??""
  return sample.in_progress!==true && sample.attempt_status!=="in_progress"
    && sample.is_auxiliary!==true && sample.is_probe!==true && sample.is_cyber!==true
    && !/(?:^|[_-])(auxiliary|probe|cyber)(?:$|[_-])/.test(requestType)
}

export function latencyHealthScore(samples: readonly HealthSample[], options: { now?: number; windowHours?: number; sampleLimit?: number } = {}): LatencyHealthScore | null {
  const {now = Date.now(), windowHours = 1, sampleLimit = 60} = options
  const selected = recentRequestWindow(samples, now, windowHours, sampleLimit)
  if (selected.samples.length === 0) return null
  const metricSamples = selected.samples.filter(healthMetricSample)
  const metrics = recentRequestMetrics(metricSamples, now, selected.windowHours, selected.sampleLimit)
  const measured = metricSamples.filter(sample => validLatency(sample.first_token_ms) || validLatency(sample.duration_ms))
  const recentLatency = average(measured.map(sampleLatencyScore).filter((value): value is number => value != null))
  const stability = requestStability(measured)
  const cacheHit = metrics.cacheHitRate == null ? null : metrics.cacheHitRate * 100
  const parts = [
    { name: "稳定性", value: stability },
    { name: "缓存命中", value: cacheHit },
    { name: "延迟", value: recentLatency },
  ]
  const missingMetrics = parts.filter(({value}) => value == null).map(({name}) => name)
  const score = missingMetrics.length > 0 ? null : Math.round(parts.reduce((sum, {value}) => sum + value!, 0) / parts.length)
  return {
    score,
    recentLatency,
    stability,
    cacheHit,
    sampleCount: selected.samples.length,
    sampleLimit: selected.sampleLimit,
    measuredCount: measured.length,
    windowHours: selected.windowHours,
    missingMetrics,
    lowConfidence: measured.length < 10,
  }
}

export function healthScoreDescription(score: LatencyHealthScore | null) {
  if (!score) return "当前统计时间窗内暂无调用样本"
  const value = (part: number | null) => part == null ? "未知" : part.toFixed(1)
  const result = score.score == null ? `数据不足：${score.missingMetrics.join("、")}` : `综合评分 ${score.score}`
  const confidence = score.lowConfidence ? " · 少于 10 次，低置信度" : ""
  return `${result} · ${score.windowHours}h \u6700\u8fd1 ${score.sampleCount}/${score.sampleLimit} \u6b21\uff08\u6709\u6548\u8017\u65f6 ${score.measuredCount} \u6b21\uff09${confidence}
\u7a33\u5b9a\u6027 ${value(score.stability)} · \u7f13\u5b58\u547d\u4e2d ${value(score.cacheHit)} · \u5ef6\u8fdf ${value(score.recentLatency)}
\u4e09\u9879\u5404\u5360 1/3\uff0c\u7f3a\u9879\u4e0d\u91cd\u65b0\u5206\u914d\u6743\u91cd\u3002\u7f13\u5b58\u4f7f\u7528 Token \u52a0\u6743\u6bd4\u4f8b\uff1b\u7a33\u5b9a\u6027\u548c\u5ef6\u8fdf\u76f4\u63a5\u4f7f\u7528\u5b9e\u6d4b\u8017\u65f6\u3002
\u7a33\u5b9a\u6027 = 100 \u00f7 (1 + \u6807\u51c6\u5dee/\u5747\u503c)\uff0c\u5206\u522b\u8ba1\u7b97\u9996\u5b57\u548c\u603b\u8017\u65f6\u540e\u53d6\u5e73\u5747\u3002`
}

export function formatLatency(value: number | null | undefined) {
  if (value == null || !Number.isFinite(value) || value < 0) return "-"
  const milliseconds = Math.round(value)
  if (milliseconds < 1_000) return `${milliseconds}ms`
  const seconds = milliseconds / 1_000
  if (seconds < 60) return `${seconds.toFixed(seconds < 10 ? 1 : 0)}秒`
  const minutes = Math.floor(seconds / 60)
  return `${minutes}分 ${String(Math.round(seconds % 60)).padStart(2, "0")}秒`
}

export function outputTokenSpeed(outputTokens: number | null | undefined, durationMS: number | null | undefined) {
  if (outputTokens == null || durationMS == null || outputTokens <= 0 || durationMS <= 0) return null
  return outputTokens * 1000 / durationMS
}

export function percentile(values: Array<number | null | undefined>, rank: number) {
  if (!Number.isFinite(rank) || rank < 0 || rank > 1) return null
  const sorted = values.filter((value): value is number => value != null && Number.isFinite(value) && value >= 0).sort((left, right) => left - right)
  if (sorted.length === 0) return null
  const position = rank * (sorted.length - 1)
  const lower = Math.floor(position)
  const upper = Math.ceil(position)
  if (lower === upper) return sorted[lower]
  const weight = position - lower
  return sorted[lower] * (1 - weight) + sorted[upper] * weight
}

export function p50(values: Array<number | null | undefined>) { return percentile(values, 0.5) }

export function p95(values: Array<number | null | undefined>) { return percentile(values, 0.95) }

export function formatOutputTokenSpeed(value: number | null | undefined) {
  if (value == null || !Number.isFinite(value) || value < 0) return "未提供"
  return `${Number(value.toFixed(1)).toLocaleString("zh-CN")} tok/s`
}

export const latencyBucketGranularityOptions: Array<{ value: LatencyBucketGranularity; label: string }> = [
  { value: "auto", label: "自动" },
  { value: "5", label: "5 分钟" },
  { value: "10", label: "10 分钟" },
  { value: "30", label: "30 分钟" },
  { value: "60", label: "1 小时" },
  { value: "180", label: "3 小时" },
  { value: "360", label: "6 小时" },
  { value: "720", label: "12 小时" },
]

export function latencyGranularityOptionsForRange(range: TimeRangeSelection<RelayUsageRangePreset>) {
  if (typeof range !== "string" || range === "today" || range === "24h") return latencyBucketGranularityOptions
  return latencyBucketGranularityOptions.filter((item) => item.value === "auto" || Number(item.value) >= 60)
}

export function normalizeLatencyGranularity(range: TimeRangeSelection<RelayUsageRangePreset>, granularity: LatencyBucketGranularity) {
  return latencyGranularityOptionsForRange(range).some((item) => item.value === granularity) ? granularity : "60"
}

export function latencyBucketGranularityLabel(granularity: LatencyBucketGranularity) {
  if (granularity === "auto") return "自动"
  return latencyBucketGranularityOptions.find((item) => item.value === granularity)?.label ?? `${granularity} 分钟`
}

export function latencyBucketQuery(granularity: LatencyBucketGranularity, targetCount: number) {
  const query = new URLSearchParams()
  if (granularity === "auto") {
    query.set("bucket_mode", "auto")
    query.set("bucket_count", String(targetCount))
  } else {
    query.set("bucket_mode", "fixed")
    query.set("bucket_minutes", granularity)
  }
  return query
}

