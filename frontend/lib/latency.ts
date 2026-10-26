import type { LatencyBucket, LatencyBucketGranularity, RelayUsageRange } from "@/lib/api-types"

// @ts-expect-error Node 22 原生 TypeScript 测试需要显式扩展名。
import { recentRequestAttemptOutcome, recentRequestMetrics, recentRequestWindow, type RecentRequestSample } from "./recent-request-metrics.ts"

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
    if (outcome === "bad" || measured === "bad") return "bad"
    if (outcome === "warn" || measured === "warn") return "warn"
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
  recentSuccess: number | null
  recentLatency: number | null
  stability: number | null
  cacheHit: number | null
  sampleCount: number
  sampleLimit: number
  attemptCount: number
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

function requestStability(attempts: Array<{ sample: HealthSample; outcome: boolean }>) {
  if (attempts.length < 2) return null
  const successful = attempts.filter(({outcome}) => outcome).map(({sample}) => sample)
  const completed = attempts.filter(({sample}) => sample.in_progress !== true && sample.attempt_status !== "in_progress")
  const consistency = average([
    latencyConsistency(successful.map((sample) => sample.first_token_ms).filter(validLatency)),
    latencyConsistency(successful.map((sample) => sample.duration_ms).filter(validLatency)),
  ].filter((value): value is number => value != null))
  let run = 0, longestRun = 0
  for (const {sample, outcome} of attempts) {
    if (sample.in_progress === true || sample.attempt_status === "in_progress") {
      run = 0
      continue
    }
    run = outcome ? 0 : run + 1
    longestRun = Math.max(longestRun, run)
  }
  if (consistency == null) return completed.length >= 2 && completed.every(({outcome}) => !outcome) ? 0 : null
  const continuity = 1 - Math.max(0, longestRun - 1) / (attempts.length - 1)
  return consistency * Math.max(0, continuity)
}

export function latencyHealthScore(samples: readonly HealthSample[], options: { now?: number; windowHours?: number; sampleLimit?: number } = {}): LatencyHealthScore | null {
  const {now = Date.now(), windowHours = 1, sampleLimit = 60} = options
  const selected = recentRequestWindow(samples, now, windowHours, sampleLimit)
  if (selected.samples.length === 0) return null
  const metrics = recentRequestMetrics(selected.samples, now, selected.windowHours, selected.sampleLimit)
  const attempts = selected.samples.map((sample) => ({sample, outcome: recentRequestAttemptOutcome(sample)}))
    .filter((attempt): attempt is {sample: HealthSample; outcome: boolean} => attempt.outcome != null)
  const successful = attempts.filter(({outcome}) => outcome).map(({sample}) => sample)
  const recentSuccess = metrics.successRate == null ? null : metrics.successRate * 100
  const recentLatency = average(successful.map(sampleLatencyScore).filter((value): value is number => value != null))
  const stability = requestStability(attempts)
  const cacheHit = metrics.cacheHitRate == null ? null : metrics.cacheHitRate * 100
  const parts = [
    { name: "成功率", value: recentSuccess },
    { name: "稳定性", value: stability },
    { name: "缓存命中", value: cacheHit },
    { name: "延迟", value: recentLatency },
  ]
  const missingMetrics = parts.filter(({value}) => value == null).map(({name}) => name)
  const score = missingMetrics.length > 0 ? null : Math.round(parts.reduce((sum, {value}) => sum + value! * 0.25, 0))
  return {
    score,
    recentSuccess,
    recentLatency,
    stability,
    cacheHit,
    sampleCount: selected.samples.length,
    sampleLimit: selected.sampleLimit,
    attemptCount: attempts.length,
    windowHours: selected.windowHours,
    missingMetrics,
    lowConfidence: attempts.length < 10,
  }
}

export function healthScoreDescription(score: LatencyHealthScore | null) {
  if (!score) return "当前统计时间窗内暂无调用样本"
  const value = (part: number | null) => part == null ? "未知" : part.toFixed(1)
  const result = score.score == null ? `数据不足：${score.missingMetrics.join("、")}` : `综合评分 ${score.score}`
  const confidence = score.lowConfidence ? " · 少于 10 次，低置信度" : ""
  return `${result} · ${score.windowHours}h 最近 ${score.sampleCount}/${score.sampleLimit} 次（有效尝试 ${score.attemptCount} 次）${confidence}
成功率 ${value(score.recentSuccess)} · 稳定性 ${value(score.stability)} · 缓存命中 ${value(score.cacheHit)} · 延迟 ${value(score.recentLatency)}
四项各占 25%，缺项不重新分配权重。成功率和缓存与环形指标同口径；稳定性仅评估相对延迟波动与连续失败。
波动分 = 100 ÷ (1 + 标准差/均值)；连续失败系数 = 1 - max(0, 最长连败-1) ÷ (尝试次数-1)。稳定性 = 波动分 × 连续失败系数；延迟仅评估成功请求。`
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

export function latencyGranularityOptionsForRange(range: RelayUsageRange) {
  if (range === "today" || range === "24h") return latencyBucketGranularityOptions
  return latencyBucketGranularityOptions.filter((item) => item.value === "auto" || Number(item.value) >= 60)
}

export function normalizeLatencyGranularity(range: RelayUsageRange, granularity: LatencyBucketGranularity) {
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

