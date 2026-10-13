import type { ChannelUsageSample, LatencyBucket, LatencyBucketGranularity, RelayLatencySample, RelayUsageRange } from "@/lib/api-types"

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
  score: number
  recentSuccess: number | null
  recentLatency: number | null
  stability: number | null
  sampleCount: number
  bucketRequestCount: number
}

type HealthSample = RelayLatencySample | ChannelUsageSample

function average(values: number[]) {
  if (values.length === 0) return null
  return values.reduce((sum, value) => sum + value, 0) / values.length
}

function sampleSuccessScore(sample: HealthSample): number | null {
  if ("success_known" in sample) {
    if (!sample.success_known) return null
    return sample.success ? 100 : 0
  }
  const status = sample.status_code
  if (status == null) return null
  return status === 0 || (status >= 200 && status < 400) ? 100 : 0
}

function sampleLatencyScore(sample: HealthSample) {
  const values: number[] = []
  if (sample.first_token_ms != null) values.push(latencyPerformanceScore(sample.first_token_ms, "first"))
  if (sample.duration_ms != null) values.push(latencyPerformanceScore(sample.duration_ms, "duration"))
  return average(values)
}

export function latencyHealthScore(samples: HealthSample[], buckets: LatencyBucket[]): LatencyHealthScore | null {
  const recent = samples.slice(0, 60)
  const sampleCount = recent.length
  const bucketRequestCount = buckets.reduce((sum, bucket) => sum + Math.max(0, bucket.request_count), 0)
  if (sampleCount === 0 && bucketRequestCount === 0) return null

  const recentSuccess = average(recent.map(sampleSuccessScore).filter((value): value is number => value != null))
  const recentLatency = average(recent.map(sampleLatencyScore).filter((value): value is number => value != null))

  let stability: number | null = null
  if (bucketRequestCount > 0) {
    let weighted = 0
    let weightedCount = 0
    for (const bucket of buckets) {
      const count = Math.max(0, bucket.request_count)
      if (count === 0) continue

      const components: number[] = []
      if (bucket.success_rate != null) {
        components.push(Math.max(0, Math.min(100, bucket.success_rate * 100)))
      }
      const tailLatency = average([
        bucket.first_token_p95_ms == null ? null : latencyPerformanceScore(bucket.first_token_p95_ms, "first"),
        bucket.duration_p95_ms == null ? null : latencyPerformanceScore(bucket.duration_p95_ms, "duration"),
      ].filter((value): value is number => value != null))
      if (tailLatency != null) components.push(tailLatency)

      const bucketScore = average(components)
      if (bucketScore == null) continue
      weighted += bucketScore * count
      weightedCount += count
    }
    if (weightedCount > 0) stability = weighted / weightedCount
  }

  const parts: Array<{ value: number | null; weight: number }> = [
    { value: recentSuccess, weight: 0.4 },
    { value: recentLatency, weight: 0.3 },
    { value: stability, weight: 0.3 },
  ]
  const available = parts.filter((part): part is { value: number; weight: number } => part.value != null)
  const weight = available.reduce((sum, part) => sum + part.weight, 0)
  if (weight === 0) return null
  const score = available.reduce((sum, part) => sum + part.value * part.weight, 0) / weight

  return {
    score: Math.round(Math.max(0, Math.min(100, score))),
    recentSuccess: recentSuccess == null ? null : Math.round(recentSuccess),
    recentLatency: recentLatency == null ? null : Math.round(recentLatency),
    stability: stability == null ? null : Math.round(stability),
    sampleCount,
    bucketRequestCount,
  }
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

