"use client"

import { useCallback, useContext, useEffect, useMemo, useRef, useState } from "react"
import { SyncSettingsContext, SystemSettingsContext } from "@/lib/settings-context"
import { apiFetch } from "@/lib/api"
import { createSharedRequestRegistry } from "@/lib/shared-request"
import { scheduleQueryUpdate } from "@/lib/query-transition"
import { normalizeRecentMetricsLimit } from "@/lib/recent-request-metrics"
import { latencyBucketQuery } from "@/lib/latency"
import { useRefreshTick } from "@/lib/refresh-context"
import type {
  BalanceTrendPoint,
  CaptchaConfig,
  Channel,
  ChannelModelCatalogItem,
  ChannelRelayAssociationSnapshot,
  ChannelMetric,
  ChannelSummary,
  ChannelLatencyTrend,
  ChannelLatencyBucketSeries,
  ChannelConsumptionBucketSeries,
  RelayAccountLatencyBucketSeries,
  RelayLatencyTrend,
  RelayAccountConsumptionBucketSeries,
  RelayAccountModelCatalogItem,
  DashboardSummary,
  NotificationChannel,
  NotificationLog,
  AlertRule,
  AlertEvent,
  AlertSummary,
  RateChangeLog,
  RateSnapshot,
  RelayOverview,
  RelayUsageView,
  RelayDailyConsumptionPoint,
  RelayRecentUsage,
  RelayUserManagementPage,
  RelayUserSortKey,
  RelayUserRiskLevel,
  RelayUserBalanceHistory,
  RelayStation,
  PelicanGallery,
  PelicanLibraryPage,
  DashboardRange,
  RelayUsageRange,
  LatencyBucketGranularity,
  SyncSettings,
  SystemSettings,
  LocalAccountListResponse,
  OperationRange,
  OperationLedgerEntry,
  OperationLinkStation,
  OperationSummary,
} from "@/lib/api-types"

export interface QueryState<T> {
  data: T | null
  loading: boolean
  refreshing: boolean
  error: string | null
  refetch: () => Promise<void>
}

/**
 * In-flight 请求去重：同一个 URL 在同一个 tick 内只发一次，所有 useApi 共享 Promise。
 *
 * 为什么需要：useDashboardSummary() 在 5 个组件里都被调用，没去重的话每次 mount /
 * refresh 都会发 5 个相同请求。开发环境叠加 StrictMode 翻倍后会更夸张。
 */
const inflight = createSharedRequestRegistry<unknown>()

/** Cache 已完成的响应一小段时间，便于同一帧内挂载的多个组件共享结果（即使第一次的 Promise 已经 resolve）。 */
interface CacheEntry {
  data: unknown
  expiresAt: number
}
const cache = new Map<string, CacheEntry>()
const CACHE_TTL_MS = 800
const RELAY_OVERVIEW_CACHE_TTL_MS = 30_000

export function cancelInFlightQueries() {
  inflight.cancelAll()
}

interface ApiState<T> {
  path: string | null
  data: T | null
  loading: boolean
  refreshing: boolean
  error: string | null
}

function cacheKey(path: string, tick: number, bump: number) {
  return `${path}#${tick}#${bump}`
}

function readCached<T>(key: string) {
  const cached = cache.get(key)
  if (!cached) return null
  if (cached.expiresAt <= Date.now()) {
    cache.delete(key)
    return null
  }
  return cached.data as T
}

function fetchShared<T>(path: string, key: string, cancelOnNavigation = true, cacheTTL = CACHE_TTL_MS) {
  const cached = readCached<T>(key)
  if (cached !== null) return { promise: Promise.resolve(cached), release: () => {} }

  const request = inflight.acquire(key, (signal) =>
    apiFetch<T>(path, { signal }).then((data) => {
      cache.set(key, { data, expiresAt: Date.now() + cacheTTL })
      return data
    }),
    { cancelOnNavigation },
  )
  return { promise: request.promise as Promise<T>, release: request.release }
}

/**
 * useApi 通用数据获取 hook（stale-while-revalidate）。
 * - 首次加载：loading = true，组件显示加载占位
 * - 后续刷新（refresh tick / refetch）：保留旧 data，refreshing=true 供表格显示刷新反馈
 * - 请求路径变化：清空旧 data，避免把上一条查询的结果展示成新查询
 * - 同 URL + 同 tick 的并发调用共享一次请求
 */
function useApi<T>(path: string | null, cancelOnNavigation = true, cacheTTL = CACHE_TTL_MS): QueryState<T> {
  const [state, setState] = useState<ApiState<T>>({
    path,
    data: null,
    loading: path !== null,
    refreshing: path !== null,
    error: null,
  })
  const [bump, setBump] = useState(0)
  const globalTick = useRefreshTick()
  const [requestTick, setRequestTick] = useState(globalTick)
  const requestPending = useRef(false)
  const previousTick = useRef(globalTick)
  const bumpRef = useRef(0)
  const refetchWaiters = useRef<Array<{ path: string; bump: number; resolve: () => void; reject: (error: unknown) => void }>>([])

  useEffect(() => () => {
    const waiters = refetchWaiters.current.splice(0)
    waiters.forEach(({ resolve }) => resolve())
  }, [])

  useEffect(() => {
    if (previousTick.current === globalTick) return
    previousTick.current = globalTick
    // 自动轮询只在当前读取完成后发起，避免长请求被每轮刷新取消而永远无法展示。
    if (!requestPending.current) setRequestTick(globalTick)
  }, [globalTick])

  useEffect(() => {
    const obsoleteWaiters = refetchWaiters.current.filter((waiter) => waiter.path !== path)
    refetchWaiters.current = refetchWaiters.current.filter((waiter) => waiter.path === path)
    obsoleteWaiters.forEach(({ resolve }) => resolve())
    if (path === null) {
      setState({ path: null, data: null, loading: false, refreshing: false, error: null })
      return
    }
    const requestBump = bump
    let cancelled = false
    requestPending.current = true
    setState((previous) => {
      if (previous.path !== path) {
        const cached = readCached<T>(cacheKey(path, requestTick, 0))
        return { path, data: cached, loading: cached === null, refreshing: true, error: null }
      }
      if (previous.data === null) {
        return { ...previous, loading: true, refreshing: true, error: null }
      }
      return { ...previous, loading: false, refreshing: true, error: null }
    })
    const request = fetchShared<T>(path, cacheKey(path, requestTick, bump), cancelOnNavigation, cacheTTL)
    request.promise
      .then((d) => {
        if (cancelled) return
        requestPending.current = false
        scheduleQueryUpdate(() => {
          setState({ path, data: d, loading: false, refreshing: false, error: null })
        })
        const waiters = refetchWaiters.current.filter((waiter) => waiter.path === path && waiter.bump <= requestBump)
        refetchWaiters.current = refetchWaiters.current.filter((waiter) => waiter.path !== path || waiter.bump > requestBump)
        waiters.forEach(({ resolve }) => resolve())
      })
      .catch((e: Error) => {
        if (cancelled) return
        requestPending.current = false
        if (e.name === "AbortError") return
        scheduleQueryUpdate(() => {
          setState((previous) => (
            previous.path === path
              ? { ...previous, loading: false, refreshing: false, error: e.message }
              : previous
          ))
        })
        const waiters = refetchWaiters.current.filter((waiter) => waiter.path === path && waiter.bump <= requestBump)
        refetchWaiters.current = refetchWaiters.current.filter((waiter) => waiter.path !== path || waiter.bump > requestBump)
        waiters.forEach(({ reject }) => reject(e))
      })
    return () => {
      cancelled = true
      requestPending.current = false
      request.release()
    }
  }, [path, bump, requestTick, cancelOnNavigation])

  const current = state.path === path
    ? state
    : (() => {
      const cached = path === null ? null : readCached<T>(cacheKey(path, requestTick, 0))
      return { path, data: cached, loading: path !== null && cached === null, refreshing: path !== null, error: null }
    })()

  return {
    data: current.data,
    loading: current.loading,
    refreshing: current.refreshing,
    error: current.error,
    refetch: useCallback(() => {
      if (path === null) return Promise.resolve()
      const nextBump = bumpRef.current + 1
      bumpRef.current = nextBump
      return new Promise<void>((resolve, reject) => {
        refetchWaiters.current.push({ path, bump: nextBump, resolve, reject })
        setBump(nextBump)
      })
    }, [path]),
  }
}

export function useDashboardSummary(range: RelayUsageRange = "today", enabled = true) {
  return useApi<DashboardSummary>(enabled ? `/dashboard/summary?range=${range}` : null)
}

export type BalanceTrendRange = DashboardRange

export function useBalanceTrend(range: BalanceTrendRange = "7d") {
  return useApi<BalanceTrendPoint[]>(`/dashboard/balance-trend?range=${range}`)
}

export function useChannels(cancelOnNavigation = true) {
  return useApi<Channel[]>("/channels", cancelOnNavigation)
}

export function useChannelRelayAssociations() {
  return useApi<ChannelRelayAssociationSnapshot>("/channels/associations")
}

export function useChannelModelCatalog() {
  return useApi<ChannelModelCatalogItem[]>("/channels/model-catalog")
}

export function useChannelSummary(range: RelayUsageRange = "today", enabled = true) {
  return useApi<ChannelSummary>(enabled ? `/channels/summary?range=${range}` : null)
}

export function useChannelMetrics(range: RelayUsageRange = "today") {
  return useApi<ChannelMetric[]>(`/channels/metrics?range=${range}`)
}

export function useChannelLatencyTrends(limit = 60, enabled = true) {
  return useApi<ChannelLatencyTrend[]>(enabled ? `/channels/latency-trends?limit=${normalizeRecentMetricsLimit(limit)}` : null)
}

export function useRelayLatencyTrends(limit = 60, enabled = true) {
  return useApi<RelayLatencyTrend[]>(enabled ? `/relay-stations/latency-trends?limit=${normalizeRecentMetricsLimit(limit)}` : null)
}

export function useChannelLatencyBuckets(range: RelayUsageRange = "today", granularity: LatencyBucketGranularity = "auto", targetCount = 60, enabled = true) {
  const normalized = range === "all" ? "30d" : range
  const query = latencyBucketQuery(granularity, targetCount)
  query.set("range", normalized)
  return useApi<ChannelLatencyBucketSeries[]>(enabled ? `/channels/latency-buckets?${query.toString()}` : null)
}

export function useChannelConsumptionBuckets(range: RelayUsageRange = "today", granularity: LatencyBucketGranularity = "auto", targetCount = 60, enabled = true) {
  const normalized = range === "all" ? "30d" : range
  const query = latencyBucketQuery(granularity, targetCount)
  query.set("range", normalized)
  return useApi<ChannelConsumptionBucketSeries[]>(enabled ? `/channels/consumption-buckets?${query.toString()}` : null)
}

export function useRelayLatencyBuckets(stationID: number | null, range: RelayUsageRange = "today", granularity: LatencyBucketGranularity = "auto", targetCount = 30) {
  const normalized = range === "all" ? "30d" : range
  const query = latencyBucketQuery(granularity, targetCount)
  query.set("range", normalized)
  return useApi<RelayAccountLatencyBucketSeries[]>(stationID == null ? null : `/relay-stations/${stationID}/latency-buckets?${query.toString()}`)
}

export function useRelayAccountConsumptionBuckets(stationID: number | null, range: RelayUsageRange = "today", granularity: LatencyBucketGranularity = "auto", targetCount = 30) {
  const normalized = range === "all" ? "30d" : range
  const query = latencyBucketQuery(granularity, targetCount)
  query.set("range", normalized)
  return useApi<RelayAccountConsumptionBucketSeries[]>(stationID == null ? null : `/relay-stations/${stationID}/consumption-buckets?${query.toString()}`)
}

export function useChannelRates(channelID: number | null) {
  return useApi<RateSnapshot[]>(channelID == null ? null : `/channels/${channelID}/rates`)
}

export function useRateChanges(limit = 20, channelID?: number, ratioOnly = false) {
  const q = new URLSearchParams()
  q.set("limit", String(limit))
  if (channelID != null) q.set("channel_id", String(channelID))
  if (ratioOnly) q.set("ratio_only", "true")
  return useApi<RateChangeLog[]>(`/rate-changes?${q.toString()}`)
}

export function useLatestRatioChanges(channelID: number | null) {
  const path = channelID == null
    ? null
    : `/rate-changes?channel_id=${channelID}&latest_per_group=true`
  return useApi<RateChangeLog[]>(path)
}

export function useNotificationChannels() {
  return useApi<NotificationChannel[]>("/notifications/channels")
}

export function useNotificationLogs(limit = 20) {
  return useApi<NotificationLog[]>(`/notifications/logs?limit=${limit}`)
}

export function useAlertSummary() {
  return useApi<AlertSummary>("/alerts/summary")
}

export function useAlertRules() {
  return useApi<AlertRule[]>("/alerts/rules")
}

export function useAlertEvents(state?: string, filters: { target?: string; severity?: string; station?: string; offset?: number; from?: string } = {}) {
  const query = new URLSearchParams({ limit: "50", offset: String(filters.offset ?? 0) })
  if (state) query.set("state", state)
  if (filters.target) query.set("target_type", filters.target)
  if (filters.severity) query.set("severity", filters.severity)
  if (filters.station) query.set("station_id", filters.station)
  if (filters.from) query.set("from", filters.from)
  return useApi<AlertEvent[]>(`/alerts/events?${query.toString()}`)
}

export function useCaptchaConfigs() {
  return useApi<CaptchaConfig[]>("/captcha-configs")
}

export function usePelicanGallery(stationID: number | null) {
  return useApi<PelicanGallery>(stationID == null ? null : `/relay-stations/${stationID}/pelican`)
}
export function usePelicanSources(sourceID="all") {
 return useApi<import("./api-types").PelicanSourcesGallery>(`/relay-stations/pelican-sources${sourceID==="all"?"":`?source_id=${encodeURIComponent(sourceID)}`}`)
}
export function usePoolPelicanGallery(stationID:number|null) {
 return useApi<PelicanGallery>(stationID==null?null:`/relay-stations/${stationID}/pelican?account_type=oauth`)
}
export function useGroupPelicanGallery(stationID:number|null,readOnly=false):QueryState<PelicanGallery> {
 const query=useApi<import("./api-types").GroupPelicanGallery>(stationID==null?null:`${readOnly?"/public":""}/relay-stations/${stationID}/group-pelican?limit=3`)
 const data=useMemo(()=>query.data?{page_enabled:query.data.page_enabled,configs:query.data.configs.map(config=>({...config,account_external_id:config.group_external_id})),results:query.data.results}:null,[query.data])
 return {...query,data}
}
export function useChannelPelicanGallery(enabled = true) {
  const query=useApi<import("./api-types").ChannelPelicanGallery>(enabled ? "/channels/pelican" : null)
  const anyRunning=enabled&&query.data?.groups.some(group=>group.targets.some(target=>target.config.lease_until&&Date.parse(target.config.lease_until)>Date.now()))
  const refetch=useRef(query.refetch)
  refetch.current=query.refetch
  useEffect(()=>{
    if(!anyRunning)return
    const timer=window.setInterval(()=>{if(!document.hidden)void refetch.current().catch(()=>{})},5000)
    return()=>window.clearInterval(timer)
  },[anyRunning])
  return query
}

export function usePelicanLibrary(path: string | null) {
  return useApi<PelicanLibraryPage>(path)
}
export function useRelayStations() {
  return useApi<RelayStation[]>("/relay-stations")
}

export function useRelayOverview(stationID: number | null) {
  return useApi<RelayOverview>(stationID == null ? null : `/relay-stations/${stationID}`, true, RELAY_OVERVIEW_CACHE_TTL_MS)
}

export function usePrefetchRelayOverviews(stationIDs: readonly number[]) {
  const globalTick = useRefreshTick()
  const stationKey = stationIDs.join(",")

  useEffect(() => {
    if (stationIDs.length === 0) return
    const requests = stationIDs.map((stationID) => fetchShared<RelayOverview>(
      `/relay-stations/${stationID}`,
      cacheKey(`/relay-stations/${stationID}`, globalTick, 0),
      false,
      RELAY_OVERVIEW_CACHE_TTL_MS,
    ))
    requests.forEach(({ promise }) => { void promise.catch(() => {}) })
    return () => requests.forEach(({ release }) => release())
  }, [globalTick, stationKey])
}

export function useRelayAccountModelCatalog(stationID: number | null, enabled = true) {
  return useApi<RelayAccountModelCatalogItem[]>(stationID == null || !enabled ? null : `/relay-stations/${stationID}/account-models`)
}

export function useRelayUsage(stationID: number | null, range: RelayUsageRange = "today") {
  return useApi<RelayUsageView>(stationID == null ? null : `/relay-stations/${stationID}/usage?range=${range}`)
}

export function useRelayDailyConsumption(stationID: number | null, range: RelayUsageRange = "today") {
  return useApi<RelayDailyConsumptionPoint[]>(stationID == null ? null : `/relay-stations/${stationID}/usage/daily?range=${range}`)
}

export function useRelayRecentUsage(stationID: number | null, range: RelayUsageRange = "today") {
  return useApi<RelayRecentUsage[]>(stationID == null ? null : `/relay-stations/${stationID}/usage/recent?range=${range}`)
}

export function useRelayUsers(stationID: number | null, options: {
  page: number
  pageSize: number
  search: string
  range: RelayUsageRange
  sortBy: RelayUserSortKey
  sortOrder: "asc" | "desc"
  riskLevel: RelayUserRiskLevel
  registrationIP: string
}) {
  if (stationID == null) return useApi<RelayUserManagementPage>(null)
  const query = new URLSearchParams({
    page: String(options.page),
    page_size: String(options.pageSize),
    range: options.range,
    sort_by: options.sortBy,
    sort_order: options.sortOrder,
    risk_level: options.riskLevel,
    registration_ip: options.registrationIP,
  })
  if (options.search.trim()) query.set("search", options.search.trim())
  return useApi<RelayUserManagementPage>(`/relay-stations/${stationID}/users?${query.toString()}`)
}

export function useRelayUserBalanceHistory(stationID: number | null, userID: number | null, page = 1, type = "") {
  const query = type ? `&type=${encodeURIComponent(type)}` : ""
  return useApi<RelayUserBalanceHistory>(stationID == null || userID == null ? null : `/relay-stations/${stationID}/users/${userID}/balance-history?page=${page}&page_size=15${query}`)
}

export function useSyncSettings() {
  const settings = useContext(SyncSettingsContext)
  if (!settings) throw new Error("useSyncSettings requires SettingsProvider")
  return settings
}

export function useSystemSettings() {
  const settings = useContext(SystemSettingsContext)
  if (!settings) throw new Error("useSystemSettings requires SettingsProvider")
  return settings
}

export function useSyncSettingsQuery() {
  return useApi<SyncSettings>("/sync-settings", false)
}

export function useSystemSettingsQuery() {
  const query = useApi<SystemSettings>("/system-settings", false)
  return { ...query, data: query.data ? { ...query.data, recent_metrics_limit: normalizeRecentMetricsLimit(query.data.recent_metrics_limit) } : null }
}

export function useOperationSummary(range: OperationRange = "today") {
  return useApi<OperationSummary>(`/operations/summary?range=${range}`)
}

export function useOperationLedger(range: OperationRange = "30d", filters: { direction?: string; category?: string; q?: string } = {}) {
  const query = new URLSearchParams({ range })
  if (filters.direction && filters.direction !== "all") query.set("direction", filters.direction)
  if (filters.category && filters.category !== "all") query.set("category", filters.category)
  if (filters.q?.trim()) query.set("q", filters.q.trim())
  return useApi<OperationLedgerEntry[]>(`/operations/ledger?${query.toString()}`)
}

export function useLocalAccounts(filters: { status?: string; platform?: string; q?: string } = {}) {
  const query = new URLSearchParams()
  if (filters.status) query.set("status", filters.status)
  if (filters.platform) query.set("platform", filters.platform)
  if (filters.q) query.set("q", filters.q)
  const suffix = query.toString() ? `?${query.toString()}` : ""
  return useApi<LocalAccountListResponse>(`/operations/local-accounts${suffix}`)
}

export function useOperationLinkOptions() {
  return useApi<OperationLinkStation[]>("/operations/link-options")
}
