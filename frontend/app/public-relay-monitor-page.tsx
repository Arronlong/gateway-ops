"use client"
import { ProductIcon, useProductPageTitle } from "@/components/product-branding"

import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { Activity, Ban, CheckCircle2, CircleHelp, CircleX, Clock3, Info, RefreshCw, TriangleAlert } from "lucide-react"
import { useNavigate, useParams } from "react-router-dom"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { apiFetch } from "@/lib/api"
import type { LatencyBucket, PublicGroupMonitorGroup, PublicGroupMonitorStatus, PublicGroupMonitorView } from "@/lib/api-types"
import { defaultPublicRelayMonitorRange, isPublicRelayMonitorViewForSelection, publicRelayMonitorCacheKey, publicRelayMonitorCacheVersion, publicRelayMonitorPath, publicRelayMonitorRanges, resolvePublicRelayMonitorSelection, publicGroupRecentMetrics, type PublicRelayMonitorRange, type PublicRelayMonitorStationOption } from "@/lib/public-relay-monitor"
import { cn } from "@/lib/utils"
import { PlatformBadge } from "@/components/relay/platform-badge"
import { PublicGroupPelicanResults } from "@/components/relay/pelican-target-panel"
import { useGroupPelicanGallery, type QueryState } from "@/lib/queries"
import type { PelicanGallery } from "@/lib/api-types"
import { LatencyStabilityLane } from "@/components/monitor/latency-stability-lane"
import { RecentRequestMetricsDisplay } from "@/components/monitor/recent-request-metrics"

const refreshIntervalSeconds = 30
const refreshIntervalMS = refreshIntervalSeconds * 1_000
const monitorBrowserCacheVersion = publicRelayMonitorCacheVersion

type MonitorBrowserCache = {
  version: number
  data: PublicGroupMonitorView
}

function readMonitorBrowserCache(stationID: number, range: PublicRelayMonitorRange): PublicGroupMonitorView | null {
  if (typeof window === "undefined" || !Number.isInteger(stationID) || stationID <= 0) return null
  try {
    const raw = window.localStorage.getItem(publicRelayMonitorCacheKey(stationID, range))
    if (!raw) return null
    const cached = JSON.parse(raw) as MonitorBrowserCache
    if (cached.version !== monitorBrowserCacheVersion || !isPublicRelayMonitorViewForSelection(cached.data, stationID, range) || !Array.isArray(cached.data?.groups)) return null
    return cached.data
  } catch {
    return null
  }
}

function writeMonitorBrowserCache(stationID: number, range: PublicRelayMonitorRange, data: PublicGroupMonitorView) {
  if (typeof window === "undefined") return
  try {
    window.localStorage.setItem(publicRelayMonitorCacheKey(stationID, range), JSON.stringify({ version: monitorBrowserCacheVersion, data } satisfies MonitorBrowserCache))
  } catch {
    // Storage can be unavailable in private browsing; live refresh remains available.
  }
}

const statusMeta: Record<PublicGroupMonitorStatus, { label: string; className: string; pillClassName: string; icon: typeof CheckCircle2 }> = {
  available: { label: "可用", className: "text-success", pillClassName: "border-success/25 bg-success/10", icon: CheckCircle2 },
  degraded: { label: "不稳定", className: "text-warning", pillClassName: "border-warning/25 bg-warning/10", icon: TriangleAlert },
  unavailable: { label: "不可用", className: "text-danger", pillClassName: "border-danger/25 bg-danger/10", icon: CircleX },
  idle: { label: "暂无调用", className: "text-muted-foreground", pillClassName: "border-border bg-muted/60", icon: Clock3 },
  disabled: { label: "已停用", className: "text-muted-foreground", pillClassName: "border-border bg-muted/60", icon: Ban },
  unknown: { label: "状态未知", className: "text-warning", pillClassName: "border-warning/25 bg-warning/10", icon: CircleHelp },
}

function formatTime(value: string | null | undefined) {
  if (!value) return "暂无"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return "暂无"
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  }).format(date)
}

function Status({ status }: { status: PublicGroupMonitorStatus }) {
  const meta = statusMeta[status]
  const Icon = meta.icon
  return <span className={cn("inline-flex items-center gap-1.5 rounded-md border px-2.5 py-1.5 text-xs font-semibold", meta.className, meta.pillClassName)}><Icon className="size-3.5" />{meta.label}</span>
}

function successRate(group: PublicGroupMonitorGroup) {
  const total = group.success_count + group.failure_count
  return total > 0 ? `${((group.success_count / total) * 100).toFixed(1)}%` : "-"
}

function MonitorRow({ group, range, stationID, pelican }: { group: PublicGroupMonitorGroup; range: PublicRelayMonitorRange; stationID:number; pelican:QueryState<PelicanGallery> }) {
  return <article className="group overflow-hidden rounded-[5px] border border-border/80 bg-card shadow-sm transition-[border-color,box-shadow] duration-200 hover:border-brand/40 hover:shadow-md">
    <div className="p-4 sm:p-5">
      <div className="flex min-w-0 flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 flex-1 basis-36">
          <div className="flex min-w-0 flex-wrap items-center gap-2.5">
            <h2 className="min-w-0 truncate text-base font-semibold text-foreground" title={group.name}>{group.name}</h2>
            <span className="inline-flex shrink-0 items-center rounded-md border border-brand/20 bg-brand/5 px-2 py-1 font-mono text-[11px] font-semibold tabular-nums text-brand">倍率 {group.rate_multiplier.toFixed(3)}</span>
            <PlatformBadge platform={group.platform} />
          </div>
          <p className="mt-2 line-clamp-2 text-xs leading-5 text-muted-foreground" title={group.description || "暂无描述"}>{group.description || "暂无描述"}</p>
        </div>
        <RecentRequestMetricsDisplay metrics={publicGroupRecentMetrics(group.recent_metrics)} showSampleCount={false} />
        <div className="flex shrink-0 items-center gap-2">
          <Status status={group.status} />
          <span className="inline-flex items-center rounded-md border border-brand/20 bg-brand/5 px-2.5 py-1.5 font-mono text-xs font-semibold tabular-nums text-brand">成功率 {successRate(group)}</span>
        </div>
      </div>

      <div className="mt-4 rounded-md bg-muted/35 px-2 py-1.5">
        <LatencyStabilityLane
          buckets={group.latency_buckets ?? []}
          recentSamples={group.calls}
          range={range}
          loadSamples={(bucket: LatencyBucket) => {
            const start = Date.parse(bucket.start)
            const end = Date.parse(bucket.end)
            return Promise.resolve(group.calls.filter((call) => {
              const timestamp = Date.parse(call.created_at)
              return Number.isFinite(start) && Number.isFinite(end) && Number.isFinite(timestamp) && start <= timestamp && timestamp < end
            }))
          }}
          layout="fill"
          detailMode="inline"
          emptyLabel="暂无调用记录"
          title="最近调用"
          headerRight={<time className="shrink-0 font-mono tabular-nums text-foreground">{formatTime(group.latest_call_at)}</time>}
        />
        <LatencyStabilityLane
          buckets={group.latency_buckets ?? []}
          loadSamples={(bucket: LatencyBucket) => {
            const start = Date.parse(bucket.start)
            const end = Date.parse(bucket.end)
            return Promise.resolve(group.calls.filter((call) => {
              const timestamp = Date.parse(call.created_at)
              return Number.isFinite(start) && Number.isFinite(end) && Number.isFinite(timestamp) && start <= timestamp && timestamp < end
            }))
          }}
          layout="fill"
          detailMode="inline"
          emptyLabel="暂无时间稳定性数据"
          title="时间稳定性"
          range={range}
        />
      </div>

      <PublicGroupPelicanResults stationID={stationID} groupID={group.external_id} query={pelican}/>
    </div>
  </article>
}

function LoadingCards() {
  return <div className="grid min-h-[420px] gap-3 min-[1150px]:grid-cols-2">{Array.from({ length: 6 }).map((_, index) => <div key={index} className="overflow-hidden rounded-[5px] border border-border/80 bg-card p-4 shadow-sm sm:p-5"><div className="flex items-start justify-between gap-3"><div><Skeleton className="h-4 w-32" /><Skeleton className="mt-2 h-3 w-24" /></div><div className="flex gap-2"><Skeleton className="h-7 w-16 rounded-md" /><Skeleton className="h-7 w-20 rounded-md" /></div></div><div className="mt-5 border-t border-border/70 pt-3.5"><Skeleton className="mb-2 h-3 w-24" /><Skeleton className="h-10 w-full rounded-md" /></div></div>)}</div>
}

export default function PublicRelayMonitorPage() {
  const { stationID } = useParams()
  const navigate = useNavigate()
  const parsedStationID = Number(stationID)
  const routeStationID = Number.isInteger(parsedStationID) && parsedStationID > 0 ? parsedStationID : null
  const [stations, setStations] = useState<PublicRelayMonitorStationOption[]>([])
  const [stationsLoading, setStationsLoading] = useState(true)
  const [stationsError, setStationsError] = useState<string | null>(null)
  const selection = useMemo(() => resolvePublicRelayMonitorSelection(stations, routeStationID), [routeStationID, stations])
  const numericStationID = selection.mode === "selected" ? selection.stationID : 0
  const pelican = useGroupPelicanGallery(numericStationID || null,true)
  const pelicanRefetch=useRef(pelican.refetch)
  pelicanRefetch.current=pelican.refetch
  const [range, setRange] = useState<PublicRelayMonitorRange>(defaultPublicRelayMonitorRange)
  const initialCache = useMemo(() => readMonitorBrowserCache(numericStationID, range), [numericStationID, range])
  const requestIDRef = useRef(0)
  const [data, setData] = useState<PublicGroupMonitorView | null>(initialCache)
  const [loading, setLoading] = useState(!initialCache)
  const [refreshing, setRefreshing] = useState(false)
  const [countdown, setCountdown] = useState(refreshIntervalSeconds)
  const [error, setError] = useState<string | null>(null)

  const loadStations = useCallback(async () => {
    setStationsLoading(true)
    try {
      const next = await apiFetch<PublicRelayMonitorStationOption[]>("/public/relay-stations", { skipAuthErrorHandler: true })
      setStations(next)
      setStationsError(null)
    } catch (requestError) {
      setStationsError(requestError instanceof Error ? requestError.message : "中转站列表读取失败")
    } finally {
      setStationsLoading(false)
    }
  }, [])

  const load = useCallback(async (silent = false) => {
    if (!Number.isInteger(numericStationID) || numericStationID <= 0) {
      setLoading(false)
      return
    }
    const requestID = ++requestIDRef.current
    if (silent) setRefreshing(true)
    try {
      const next = await apiFetch<PublicGroupMonitorView>(publicRelayMonitorPath(numericStationID, range), { skipAuthErrorHandler: true })
      if (requestID !== requestIDRef.current) return
      if (!isPublicRelayMonitorViewForSelection(next, numericStationID, range)) {
        throw new Error(next?.station_id === numericStationID && next.range == null
          ? "当前后端服务尚未更新，24小时数据仍可查看；其他时间范围需要重启后端后生效。"
          : "监控数据的时间范围不匹配")
      }
      setData(next)
      if(!document.hidden)void pelicanRefetch.current().catch(()=>{})
      writeMonitorBrowserCache(numericStationID, range, next)
      setError(null)
    } catch (requestError) {
      if (requestID !== requestIDRef.current) return
      setError(requestError instanceof Error ? requestError.message : "监控数据读取失败")
    } finally {
      if (requestID === requestIDRef.current) {
        setLoading(false)
        setRefreshing(false)
        setCountdown(refreshIntervalSeconds)
      }
    }
  }, [numericStationID, range])

  useEffect(() => {
    void loadStations()
  }, [loadStations])

  useEffect(() => {
    if (!stationsLoading && selection.mode === "auto") {
      navigate(`/public/relay-monitor/${selection.stationID}`, { replace: true })
    }
  }, [navigate, selection, stationsLoading])

  useEffect(() => {
    requestIDRef.current++
    setRefreshing(false)
    if (!numericStationID) {
      setData(null)
      setLoading(false)
      setError(null)
      return
    }
    const cached = readMonitorBrowserCache(numericStationID, range)
    setData(cached)
    setLoading(!cached)
    setError(null)
    setCountdown(refreshIntervalSeconds)
  }, [numericStationID, range])

  useEffect(() => {
    if (!numericStationID) return
    void load(Boolean(initialCache))
    const timer = window.setInterval(() => void load(true), refreshIntervalMS)
    const countdownTimer = window.setInterval(() => setCountdown((current) => current <= 1 ? refreshIntervalSeconds : current - 1), 1_000)
    return () => {
      requestIDRef.current++
      window.clearInterval(timer)
      window.clearInterval(countdownTimer)
    }
  }, [initialCache, load, numericStationID])

  useProductPageTitle(data?.station_name ? `${data.station_name} · 分组调用监控` : "分组调用监控")

  const summary = data?.summary
  const selectedStationName = stations.find((station) => station.id === routeStationID)?.name
  return <main className="min-h-screen bg-background text-foreground">
    <div className="relative mx-auto w-full max-w-[1440px] px-4 py-6 sm:px-6 lg:px-8 lg:py-8">
      {refreshing && data ? <div className="pointer-events-none absolute inset-x-0 top-3 z-50 flex justify-center"><span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground"><RefreshCw className="size-3.5 animate-spin text-brand" />正在刷新分组监控</span></div> : null}
      <header className="flex min-h-16 flex-wrap items-start justify-between gap-4 border-b border-border pb-5">
        <div className="min-w-0"><h1 className="flex items-center gap-2 text-xl font-bold sm:text-2xl"><ProductIcon className="size-5 sm:size-6"/><span className="truncate">{data?.station_name || selectedStationName || "分组调用监控"}</span></h1><p className="mt-1.5 text-sm text-muted-foreground">公开分组的近期调用结果与响应耗时</p></div>
        <div className="flex flex-wrap items-center justify-end gap-3">
          {stations.length > 1 ? <Select value={routeStationID ? String(routeStationID) : undefined} onValueChange={(value) => navigate(`/public/relay-monitor/${value}`)}><SelectTrigger className="h-11 min-w-44 sm:h-9"><SelectValue placeholder="选择中转站" /></SelectTrigger><SelectContent>{stations.map((station) => <SelectItem key={station.id} value={String(station.id)}>{station.name}</SelectItem>)}</SelectContent></Select> : null}
          {numericStationID ? <><span className="whitespace-nowrap font-mono text-xs tabular-nums text-muted-foreground">{refreshing ? "刷新中" : `${countdown} 秒后刷新`}</span><span className="hidden text-xs text-muted-foreground sm:inline">更新于 {formatTime(data?.updated_at)}</span></> : null}
          <Button type="button" variant="outline" size="icon" className="size-11 sm:size-9" aria-label="刷新监控数据" disabled={refreshing || !numericStationID} onClick={() => void load(true)}><RefreshCw className={cn("size-4", refreshing && "animate-spin")} /></Button>
        </div>
      </header>

      <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
        <div className="inline-flex flex-wrap rounded-md border border-border bg-muted/30 p-0.5" role="group" aria-label="分组监控时间范围">
          {publicRelayMonitorRanges.map((item) => (
            <button
              key={item.value}
              type="button"
              aria-pressed={range === item.value}
              onClick={() => {
                if (range === item.value) return
                requestIDRef.current++
                setData(null)
                setLoading(true)
                setRefreshing(false)
                setError(null)
                setRange(item.value)
              }}
              className={cn("h-11 rounded px-4 text-xs transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:h-8", range === item.value ? "bg-background font-medium text-foreground shadow-xs" : "text-muted-foreground hover:text-foreground")}
            >
              {item.label}
            </button>
          ))}
        </div>
        <p className="text-[11px] text-muted-foreground">每个分组展示所选时间范围内最近 60 次调用</p>
      </div>

      {summary ? <section className="grid grid-cols-2 border-b border-border sm:grid-cols-4" aria-label="分组状态统计"><div className="py-4 sm:px-4 sm:first:pl-0"><p className="text-[11px] text-muted-foreground">监控分组</p><p className="mt-1 font-mono text-xl font-bold tabular-nums">{summary.total}</p></div><div className="py-4 sm:px-4"><p className="text-[11px] text-muted-foreground">可用</p><p className="mt-1 font-mono text-xl font-bold tabular-nums text-success">{summary.available}</p></div><div className="py-4 sm:px-4"><p className="text-[11px] text-muted-foreground">不稳定</p><p className="mt-1 font-mono text-xl font-bold tabular-nums text-warning">{summary.degraded}</p></div><div className="py-4 sm:px-4"><p className="text-[11px] text-muted-foreground">不可用</p><p className="mt-1 font-mono text-xl font-bold tabular-nums text-danger">{summary.unavailable}</p></div></section> : null}

      <div className="mt-4 flex items-center gap-3 rounded-lg border border-brand/20 bg-brand/5 px-3.5 py-2.5 text-xs text-foreground" role="note"><span className="flex size-7 shrink-0 items-center justify-center rounded-md bg-brand/10 text-brand"><Info className="size-4" aria-hidden="true" /></span><p className="min-w-0 leading-5"><strong className="font-semibold text-brand">监控说明</strong><span className="mx-2 text-muted-foreground/60" aria-hidden="true">·</span><span className="font-semibold text-warning">仅供趋势参考；状态基于最近调用，不代表当前实际可用性。</span></p></div>

      <section className="mt-5" aria-label="监控分组列表">
        {stationsError && !routeStationID ? <Alert variant="destructive" className="mb-4"><TriangleAlert /><AlertTitle>中转站列表读取失败</AlertTitle><AlertDescription>{stationsError}</AlertDescription></Alert> : null}
        {error ? <Alert variant="destructive" className="mb-4"><TriangleAlert /><AlertTitle>数据更新失败</AlertTitle><AlertDescription>{error}</AlertDescription></Alert> : null}
        {stationsLoading && !routeStationID ? <LoadingCards /> : selection.mode === "auto" ? <LoadingCards /> : selection.mode === "choose" ? <div className="flex min-h-[420px] flex-col items-center justify-center border border-dashed border-border px-6 text-center"><Activity className="size-8 text-brand" /><p className="mt-3 text-sm font-semibold text-foreground">请选择要查看的中转站</p><p className="mt-1 text-xs text-muted-foreground">检测到多个中转站，请使用右上角的站点选择器进入对应的分组监控。</p></div> : selection.mode === "empty" && !stationsError ? <div className="flex min-h-[420px] items-center justify-center border border-dashed border-border px-6 text-center text-sm text-muted-foreground">当前还没有配置中转站</div> : loading && !data ? <LoadingCards /> : data ? data.groups.length ? <div className="grid min-h-[420px] gap-3 min-[1150px]:grid-cols-2">{data.groups.map((group) => <MonitorRow key={`${numericStationID}:${range}:${group.external_id}`} group={group} range={range} stationID={numericStationID} pelican={pelican} />)}</div> : <div className="flex min-h-[420px] items-center justify-center border border-border px-6 text-center text-sm text-muted-foreground">当前没有开启监控的分组</div> : <div className="flex min-h-[420px] items-center justify-center border border-border px-6 text-sm text-muted-foreground">暂无监控数据</div>}
      </section>
      <span className="sr-only" aria-live="polite">{refreshing ? "正在刷新监控数据" : error ? "监控数据刷新失败" : "监控数据已更新"}</span>
    </div>
  </main>
}
