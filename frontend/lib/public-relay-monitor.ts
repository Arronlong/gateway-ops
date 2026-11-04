import type { PublicGroupMonitorView, PublicGroupRecentMetrics } from "./api-types"
import type { RecentRequestMetricValues } from "./recent-request-metrics"
// @ts-expect-error Node 22 原生 TypeScript 测试需要显式扩展名。
import { serializeTimeRange, timeRangeKey, type TimeRangeSelection } from "./time-range.ts"

export const publicRelayMonitorRanges = [
  { value: "today", label: "今天" },
  { value: "24h", label: "24 小时" },
  { value: "7d", label: "7 天" },
  { value: "30d", label: "30 天" },
] as const

export type PublicRelayMonitorRangePreset = typeof publicRelayMonitorRanges[number]["value"]
export type PublicRelayMonitorRange = PublicRelayMonitorRangePreset | "custom"
export const defaultPublicRelayMonitorRange: PublicRelayMonitorRangePreset = "24h"
export const publicRelayMonitorCacheVersion = 3

export function publicGroupRecentMetrics(metrics: PublicGroupRecentMetrics | undefined): RecentRequestMetricValues {
  return {
    windowHours: metrics?.window_hours ?? 1, sampleLimit: metrics?.sample_limit ?? 60, sampleCount: metrics?.sample_count ?? 0,
    latestAt: metrics?.latest_at ?? null, cacheHitRate: metrics?.cache_hit_rate ?? null, successRate: metrics?.success_rate ?? null,
    tps: metrics?.tps ?? null, tpsScore: metrics?.tps_score ?? null, successCount: metrics?.success_count ?? 0,
    successAttemptCount: metrics?.success_attempt_count ?? 0, cacheInputTokens: metrics?.cache_input_tokens ?? 0,
    cacheReadTokens: metrics?.cache_read_tokens ?? 0, cacheCreationTokens: metrics?.cache_creation_tokens ?? 0,
    tpsOutputTokens: metrics?.tps_output_tokens ?? 0, tpsDurationSeconds: metrics?.tps_duration_seconds ?? 0, tpsModels: metrics?.tps_models ?? [],
  }
}

export function publicRelayMonitorPath(stationID: number, range: TimeRangeSelection<PublicRelayMonitorRangePreset>) {
  const query = serializeTimeRange(range)
  return query ? `/public/relay-stations/${stationID}/group-monitor?${query}` : null
}

export function publicRelayMonitorCacheKey(stationID: number, range: TimeRangeSelection<PublicRelayMonitorRangePreset>) {
  return `gatewayops:group-monitor:v${publicRelayMonitorCacheVersion}:${stationID}:${timeRangeKey(range)}`
}

export function isPublicRelayMonitorViewForSelection(
  view: (Pick<PublicGroupMonitorView, "station_id"> & Partial<Pick<PublicGroupMonitorView, "range">>) | null | undefined,
  stationID: number,
  range: TimeRangeSelection<PublicRelayMonitorRangePreset>,
) {
  return view?.station_id === stationID && (view.range ?? defaultPublicRelayMonitorRange) === (typeof range === "string" ? range : "custom")
}

export interface PublicRelayMonitorStationOption {
  id: number
  name: string
}

export type PublicRelayMonitorSelection =
  | { mode: "selected"; stationID: number }
  | { mode: "auto"; stationID: number }
  | { mode: "choose"; stationID: null }
  | { mode: "empty"; stationID: null }

export function resolvePublicRelayMonitorSelection(
  stations: PublicRelayMonitorStationOption[],
  routeStationID: number | null,
): PublicRelayMonitorSelection {
  if (routeStationID != null && Number.isInteger(routeStationID) && routeStationID > 0) {
    return { mode: "selected", stationID: routeStationID }
  }
  if (stations.length === 0) {
    return { mode: "empty", stationID: null }
  }
  if (stations.length === 1) {
    return { mode: "auto", stationID: stations[0].id }
  }
  return { mode: "choose", stationID: null }
}
