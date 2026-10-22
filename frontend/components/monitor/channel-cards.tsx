"use client"

import { memo, startTransition, useEffect, useMemo, useRef, useState } from "react"
import type { CSSProperties } from "react"
import { toast } from "sonner"
import {
  ArrowDown,
  ArrowUp,
  CircleAlert,
  CheckCircle2,
  ChevronDown,
  Columns3,
  LayoutGrid,
  List,
  Loader2,
  LogIn,
  Pause,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  RotateCcw,
  Search,
  SlidersHorizontal,
  Trash2,
  XCircle,
} from "lucide-react"
import { Card, CardContent, CardTitle } from "@/components/ui/card"
import { TableActionButton } from "@/components/ui/table-action-button"
import { ListPanelHeader } from "@/components/ui/list-panel"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { useConfirm } from "@/components/ui/confirm-dialog"
import { useChannels, useChannelConsumptionBuckets, useChannelLatencyBuckets, useChannelLatencyTrends, useChannelMetrics, useChannelModelCatalog, useChannelRates, useChannelRelayAssociations, useRelayLatencyTrends } from "@/lib/queries"
import { apiFetch } from "@/lib/api"
import { useTriggerRefresh } from "@/lib/refresh-context"
import { channelTypeLabel, money, relativeTime } from "@/lib/format"
import { cn } from "@/lib/utils"
import { syncChannelStream, testLoginStream, type ProgressEvent } from "@/lib/sync-stream"
import { RecentRequestMetrics } from "@/components/monitor/recent-request-metrics"
import { recentRequestMetrics } from "@/lib/recent-request-metrics"
import type { Channel, ChannelAccount, ChannelType, ChannelUsageSample, LatencyBucketGranularity, RateBoundAccount, RateSnapshot, RelayUsageRange, SystemSettings } from "@/lib/api-types"
import { ChannelFormDialog } from "@/components/monitor/channel-form-dialog"
import { linkedStationsByChannel as buildLinkedStationsByChannel, relayAccountComparisonKey, relayComparisonsByChannel } from "@/lib/channel-relay-association"
import { RateChangeDot, RateTooltipBody } from "@/components/monitor/rate-change"
import { LatencyStabilityLane } from "@/components/monitor/latency-stability-lane"
import { ChannelConsumptionTrend } from "@/components/monitor/channel-consumption-trend"
import { latencyHealthScore, outputTokenSpeed, p50, p95 } from "@/lib/latency"
import { channelSectionVisible, type ChannelSection } from "@/lib/system-settings"
import { channelMatchesSearch } from "@/lib/channel-search"

type Status = "healthy" | "low" | "failed" | "idle"
type MonitorStatusFilter = "enabled" | "disabled" | "all"
type ManagementModeFilter = "auto" | "manual" | "all"
type AccountCountFilter = "multi" | "single" | "all"
type ChannelTypeFilter = "all" | ChannelType
type AssociationFilter = "all" | "unlinked" | `station:${number}`
type ChannelSortKey = "score" | "cache_hit_rate" | "success_rate" | "ratio_change" | "recent_call" | "updated" | "output_speed_p50" | "output_speed_p95" | "first_latency_p50" | "first_latency_p95" | "duration_latency_p50" | "duration_latency_p95" | "balance" | "consumption" | "usage_cost" | "name" | "account_count" | "status"
type SortDirection = "asc" | "desc"
type ChannelViewMode = "gallery" | "list"
type ChannelColumnKey = "score" | "type" | "status" | "management" | "accounts" | "balance" | "consumption" | "usage_cost" | "recent" | "stability" | "consumption_trend" | "updated" | "remark"
type ChannelColumnDefinition = { key: ChannelColumnKey; label: string; template: string; minWidth: number }

const channelColumnDefinitions: ChannelColumnDefinition[] = [
  { key: "score", label: "综合评分", template: "104px", minWidth: 104 },
  { key: "type", label: "站点类型", template: "104px", minWidth: 104 },
  { key: "status", label: "状态", template: "96px", minWidth: 96 },
  { key: "management", label: "管理方式", template: "104px", minWidth: 104 },
  { key: "accounts", label: "账号", template: "minmax(160px, .8fr)", minWidth: 160 },
  { key: "balance", label: "余额", template: "132px", minWidth: 132 },
  { key: "consumption", label: "区间余额消耗", template: "132px", minWidth: 132 },
  { key: "usage_cost", label: "区间调用消耗", template: "148px", minWidth: 148 },
  { key: "recent", label: "最近调用", template: "220px", minWidth: 220 },
  { key: "stability", label: "时间稳定性", template: "260px", minWidth: 260 },
  { key: "consumption_trend", label: "调用消耗趋势", template: "220px", minWidth: 220 },
  { key: "updated", label: "上次更新", template: "128px", minWidth: 128 },
  { key: "remark", label: "渠道备注", template: "minmax(180px, .8fr)", minWidth: 180 },
]

const columnVisibilitySections: Partial<Record<ChannelColumnKey, ChannelSection>> = {
  recent: "recent",
  stability: "stability",
  consumption_trend: "consumption_trend",
}

const defaultChannelColumns = channelColumnDefinitions.map((column) => column.key)

function usePersistedChannelViewMode() {
  const [mode, setMode] = useState<ChannelViewMode>(() => {
    try {
      const stored = window.localStorage.getItem("gatewayops_channel_view_mode")
      return stored === "list" || stored === "gallery" ? stored : "gallery"
    } catch {
      return "gallery"
    }
  })
  useEffect(() => {
    try { window.localStorage.setItem("gatewayops_channel_view_mode", mode) } catch { /* keep current session state */ }
  }, [mode])
  return [mode, setMode] as const
}

function usePersistedChannelColumns() {
  const [visibleColumns, setVisibleColumns] = useState<Set<ChannelColumnKey>>(() => {
    try {
      const raw = window.localStorage.getItem("gatewayops_channel_list_columns")
      if (raw) {
        const parsed = JSON.parse(raw) as unknown
        if (Array.isArray(parsed)) {
          const valid = parsed.filter((value): value is ChannelColumnKey => defaultChannelColumns.includes(value as ChannelColumnKey))
          if (valid.length) return new Set(valid)
        }
      }
    } catch {
      // Fall back to the default set.
    }
    return new Set(defaultChannelColumns)
  })
  useEffect(() => {
    try {
      window.localStorage.setItem("gatewayops_channel_list_columns", JSON.stringify(defaultChannelColumns.filter((key) => visibleColumns.has(key))))
    } catch {
      // Column visibility still works for the current session.
    }
  }, [visibleColumns])
  return [visibleColumns, setVisibleColumns] as const
}

function channelListGridStyle(visibleColumns: Set<ChannelColumnKey>): CSSProperties {
  const visible = channelColumnDefinitions.filter((column) => visibleColumns.has(column.key))
  const template = ["minmax(220px, 1.15fr)", ...visible.map((column) => column.template), "220px"].join(" ")
  const minWidth = 220 + 220 + visible.reduce((sum, column) => sum + column.minWidth, 0)
  return {
    "--channel-list-grid-template": template,
    "--channel-list-grid-min-width": `${minWidth}px`,
  } as CSSProperties
}

const channelSortOptions: Array<{ value: ChannelSortKey; label: string }> = [
  { value: "score", label: "综合评分" },
  { value: "cache_hit_rate", label: "缓存命中率" },
  { value: "success_rate", label: "重试感知成功率" },
  { value: "ratio_change", label: "最近倍率变动" },
  { value: "recent_call", label: "最近调用时间" },
  { value: "updated", label: "上次更新时间" },
  { value: "output_speed_p50", label: "Token 速度 P50" },
  { value: "output_speed_p95", label: "Token 速度 P95" },
  { value: "first_latency_p50", label: "首字耗时 P50" },
  { value: "first_latency_p95", label: "首字耗时 P95" },
  { value: "duration_latency_p50", label: "总耗时 P50" },
  { value: "duration_latency_p95", label: "总耗时 P95" },
  { value: "balance", label: "余额" },
  { value: "consumption", label: "区间余额消耗" },
  { value: "usage_cost", label: "区间调用消耗" },
  { value: "name", label: "渠道名称" },
  { value: "account_count", label: "账号数量" },
  { value: "status", label: "状态优先级" },
]

const monitorStatusLabels: Record<MonitorStatusFilter, string> = { enabled: "监控中", disabled: "已暂停", all: "All" }
const managementModeLabels: Record<ManagementModeFilter, string> = { all: "All", auto: "自动读取", manual: "手动管理" }
const accountCountLabels: Record<AccountCountFilter, string> = { all: "All", multi: "多账号渠道", single: "单账号渠道" }
const channelTypeLabels: Record<ChannelTypeFilter, string> = { all: "All", newapi: "New API", sub2api: "Sub2API" }

function ActiveFilterSubTrigger({ label, value }: { label: string; value?: string | null }) {
  const active = Boolean(value)
  return (
    <DropdownMenuSubTrigger className={cn(active && "text-brand")}>
      <span className="flex min-w-0 flex-1 items-center justify-between gap-2">
        <span className="shrink-0">{label}</span>
        {active ? (
          <span className="max-w-24 truncate rounded-full bg-brand/10 px-1.5 py-0.5 text-[10px] font-semibold text-brand" title={value ?? undefined}>
            {value}
          </span>
        ) : null}
      </span>
    </DropdownMenuSubTrigger>
  )
}

function defaultSortDirection(key: ChannelSortKey): SortDirection {
  return key === "name" || key.startsWith("first_latency") || key.startsWith("duration_latency") ? "asc" : "desc"
}

function statusOf(c: Channel, balance?: number | null): Status {
  if (c.last_error) return "failed"
  const current = balance ?? c.last_balance
  if (current == null) return "idle"
  if (c.balance_threshold > 0 && current <= c.balance_threshold) return "low"
  return "healthy"
}

const statusMap: Record<Status, { label: string; cls: string }> = {
  healthy: { label: "正常", cls: "text-success bg-success/10" },
  low: { label: "低余额", cls: "text-warning bg-warning/10" },
  failed: { label: "异常", cls: "text-danger bg-danger/10" },
  idle: { label: "尚未采集", cls: "text-muted-foreground bg-muted/40" },
}

function managementErrorText(message: string) {
  return message.replace("token 已失效", "渠道管理凭据失效")
}

function healthScoreClass(score: number) {
  if (score >= 85) return "bg-emerald-500/10 text-emerald-700 ring-emerald-500/20 dark:text-emerald-400"
  if (score >= 70) return "bg-amber-400/15 text-amber-700 ring-amber-500/30 dark:text-amber-400"
  return "bg-red-500/10 text-red-700 ring-red-500/20 dark:text-red-400"
}

function Row({ label, children, className }: { label: string; children: React.ReactNode; className?: string }) {
  return (
    <div className={cn("flex min-w-0 items-center justify-between gap-3 bg-card px-3 py-2.5", className)}>
      <span className="shrink-0 text-[11px] font-medium text-muted-foreground">{label}</span>
      <div className="min-w-0 truncate text-right text-xs font-semibold text-foreground">{children}</div>
    </div>
  )
}

function siteHost(value: string) {
  try {
    return new URL(value).host
  } catch {
    return value
  }
}

function cny(value: number | null | undefined) {
  if (value == null || !Number.isFinite(value)) return "—"
  return `¥${value.toLocaleString("zh-CN", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
}

function charge(value: number | null | undefined) {
  if (value == null || !Number.isFinite(value)) return "—"
  return `$${value.toLocaleString("en-US", { minimumFractionDigits: 6, maximumFractionDigits: 6 })}`
}

function balanceTone(channel: Channel): string {
  if (channel.last_balance == null || !Number.isFinite(channel.last_balance)) return "text-muted-foreground"
  if (channel.last_balance <= 0) return "text-danger"
  if (channel.balance_threshold > 0 && channel.last_balance <= channel.balance_threshold) return "text-warning"
  return "text-success"
}

function channelAccounts(channel: Channel): ChannelAccount[] {
  if (channel.accounts?.length > 0) return channel.accounts
  return [{
    id: 0,
    is_primary: true,
    username: channel.username,
    credential_mode: channel.credential_mode,
    turnstile_enabled: channel.turnstile_enabled,
    captcha_config_id: channel.captcha_config_id,
    last_balance: channel.last_balance,
    last_balance_at: channel.last_balance_at,
    last_error: channel.last_error,
    created_at: channel.created_at,
    updated_at: channel.updated_at,
  }]
}

function accountName(account: ChannelAccount, index: number) {
  return account.username.trim() || `账号 ${index + 1}`
}

function AccountPreview({ accounts }: { accounts: ChannelAccount[] }) {
  const first = accounts[0]
  if (!first) return <span>未填写</span>
  const extra = accounts.slice(1)
  return (
    <span className="inline-flex min-w-0 items-center justify-end gap-1.5">
      <span className="truncate" title={accountName(first, 0)}>{accountName(first, 0)}</span>
      {extra.length > 0 ? (
        <Tooltip delayDuration={150}>
          <TooltipTrigger asChild>
            <button type="button" className="shrink-0 rounded border border-border bg-muted/60 px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground transition-colors hover:border-brand/40 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" aria-label={`查看其余 ${extra.length} 个账号`}>
              +{extra.length} 更多
            </button>
          </TooltipTrigger>
          <TooltipContent side="top" className="max-w-xs p-2 text-xs">
            <p className="mb-1.5 font-medium">其余账号</p>
            <ul className="space-y-1.5">
              {extra.map((account, index) => (
                <li key={account.id} className="flex min-w-0 items-center justify-between gap-4">
                  <span className="truncate">{accountName(account, index + 1)}</span>
                  <span className={cn("shrink-0 font-mono tabular-nums", account.last_error && "text-warning")}>{money(account.last_balance)}</span>
                </li>
              ))}
            </ul>
          </TooltipContent>
        </Tooltip>
      ) : null}
    </span>
  )
}

function AccountBalance({ accounts, total }: { accounts: ChannelAccount[]; total: number | null | undefined }) {
  if (accounts.length <= 1) return <>{money(total)}</>
  return (
    <Tooltip delayDuration={150}>
      <TooltipTrigger asChild>
        <span className="block truncate" tabIndex={0} aria-label="查看各账号余额">{money(total)}</span>
      </TooltipTrigger>
      <TooltipContent side="top" className="max-w-xs p-2 text-xs">
        <p className="mb-1.5 font-medium">账号余额明细</p>
        <ul className="space-y-1.5">
          {accounts.map((account, index) => (
            <li key={account.id} className="flex min-w-0 items-center justify-between gap-4">
              <span className="truncate">{accountName(account, index)}</span>
              <span className={cn("shrink-0 font-mono tabular-nums", account.last_error && "text-warning")}>{money(account.last_balance)}</span>
            </li>
          ))}
        </ul>
        <div className="mt-2 flex items-center justify-between border-t border-background/20 pt-1.5 font-medium">
          <span>合计</span><span className="font-mono tabular-nums">{money(total)}</span>
        </div>
      </TooltipContent>
    </Tooltip>
  )
}

const usageRanges: { value: RelayUsageRange; label: string }[] = [
  { value: "all", label: "All" },
  { value: "today", label: "今天" },
  { value: "24h", label: "24 小时" },
  { value: "7d", label: "7 天" },
  { value: "30d", label: "30 天" },
]

/** ratioTone 按倍率给 chip 上色，与 ChannelRatesPanel 共用同一套规则。 */
function ratioTone(r: number): string {
  if (r < 1) return "bg-brand/10 text-brand ring-brand/20"
  if (r > 1.2) return "bg-warning/10 text-warning ring-warning/20"
  return "bg-muted text-foreground ring-border"
}

function formatSampleTime(value: string | undefined) {
  if (!value) return "-"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return "-"
  return date.toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  })
}

/** 最近调用区块标题：展示最后一次调用时间，超过 1 小时标红提示数据陈旧。 */
function RecentLatencyTitle({ samples, compact = false }: { samples: ChannelUsageSample[]; compact?: boolean }) {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 60_000)
    return () => window.clearInterval(timer)
  }, [])
  const latestSample = useMemo(() => samples.reduce<ChannelUsageSample | null>((latest, sample) => {
    if (!latest || new Date(sample.created_at).getTime() > new Date(latest.created_at).getTime()) return sample
    return latest
  }, null), [samples])
  const latestSampleIsStale = latestSample
    ? now - new Date(latestSample.created_at).getTime() > 60 * 60 * 1000
    : false

  if (compact) {
    return latestSample ? <p className={cn("mt-0.5 truncate text-right font-mono text-[9px] tabular-nums", latestSampleIsStale ? "font-semibold text-danger" : "text-muted-foreground")} title={`最后调用 ${formatSampleTime(latestSample.created_at)}`}>最后 {formatSampleTime(latestSample.created_at)}</p> : null
  }

  return (
    <>
      最近调用延时
      {latestSample ? <span className={cn("ml-1 inline-flex items-center gap-1 font-mono text-[11px] font-medium tabular-nums", latestSampleIsStale ? "text-danger" : "text-foreground/80")}><span aria-hidden="true">·</span><span>最后调用</span><time dateTime={latestSample.created_at} className={cn("text-xs font-semibold", latestSampleIsStale ? "text-danger" : "text-foreground")}>{formatSampleTime(latestSample.created_at)}</time></span> : null}
    </>
  )
}

/** InlineRates 在渠道卡片内部展示当前所有分组倍率，默认收起。 */
function InlineRates({ channelID, showRateChanges, defaultExpanded }: { channelID: number; showRateChanges: boolean; defaultExpanded: boolean }) {
  const { data, loading } = useChannelRates(channelID)
  const rates = [...(data ?? [])].sort((a, b) => a.ratio - b.ratio)
  const [expanded, setExpanded] = useState(defaultExpanded)
  useEffect(() => setExpanded(defaultExpanded), [defaultExpanded])

  if (loading) return null
  if (rates.length === 0) return null

  return (
    <div className="py-3">
      <div className="mb-1.5 flex items-center justify-between">
        <p className="text-[11px] text-muted-foreground">
          {rates.length} 个分组
        </p>
        <button
          type="button"
          onClick={() => setExpanded((v) => !v)}
          className="inline-flex items-center gap-0.5 text-[11px] text-muted-foreground hover:text-foreground"
          aria-expanded={expanded}
        >
          {expanded ? "收起" : "展开"}
          <ChevronDown className={cn("size-3 transition-transform duration-200", expanded && "rotate-180")} />
        </button>
      </div>

      {expanded ? <div className="relative">
        <div className="flex flex-wrap gap-1.5 pt-1.5">
          {rates.map((r) => {
            return (
              <Tooltip key={r.id} delayDuration={150}>
                <TooltipTrigger asChild>
                  <span
                    className={cn(
                      "relative inline-flex cursor-default items-center gap-1 rounded px-1.5 py-0.5 text-[11px] ring-1 ring-inset transition-colors hover:bg-muted/60",
                      ratioTone(r.ratio),
                    )}
                  >
                    {showRateChanges ? <RateChangeDot change={r.latest_ratio_change} className="absolute -right-0.5 -top-0.5" /> : null}
                    <span className="font-medium">{r.model_name}</span>
                    <span className="font-semibold tabular-nums">{r.ratio.toFixed(3)}</span>
                  </span>
                </TooltipTrigger>
                <TooltipContent side="top" className="max-w-xs text-xs">
                  <RateTooltipBody rate={r} showChange={showRateChanges} />
                </TooltipContent>
              </Tooltip>
            )
          })}
        </div>
      </div> : null}
    </div>
  )
}

function BoundAccountsPreview({ accounts }: { accounts?: RateBoundAccount[] }) {
  const boundAccounts = accounts ?? []
  const count = boundAccounts.length

  return (
    <Tooltip delayDuration={150}>
      <TooltipTrigger asChild>
        <button
          type="button"
          className={cn(
            "inline-flex min-h-5 min-w-0 items-center gap-1 rounded-md border px-1.5 py-0.5 text-left text-[10px] font-medium tabular-nums transition-colors hover:border-brand/40 hover:bg-brand/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
            count > 0 ? "border-brand/20 bg-brand/10 text-brand" : "border-border bg-muted/60 text-muted-foreground",
          )}
          aria-label={`查看已绑定的 ${count} 个账号`}
        >
          <span className="shrink-0">{count} 个账号</span>
        </button>
      </TooltipTrigger>
      <TooltipContent side="top" align="start" className="max-w-sm p-2 text-xs">
        {count === 0 ? (
          <p>该分组暂无绑定账号</p>
        ) : (
          <div className="min-w-52">
            <p className="mb-1.5 font-medium">已绑定账号 · {count} 个</p>
            <ul className="max-h-56 space-y-1.5 overflow-y-auto pr-1">
              {boundAccounts.map((account) => (
                <li key={`${account.relay_station_id}-${account.relay_account_external_id}`} className="flex min-w-0 items-start justify-between gap-3">
                  <span className="min-w-0 break-words leading-4">{account.relay_station_name} · {account.relay_account_name}</span>
                  <span className="shrink-0 font-mono text-[10px] text-background/70">#{account.relay_account_external_id}</span>
                </li>
              ))}
            </ul>
          </div>
        )}
      </TooltipContent>
    </Tooltip>
  )
}

function ManualRatesEditor({ channelID, showRateChanges, defaultExpanded }: { channelID: number; showRateChanges: boolean; defaultExpanded: boolean }) {
  const { data, loading, refetch } = useChannelRates(channelID)
  const [editingID, setEditingID] = useState<number | null>(null)
  const [name, setName] = useState("")
  const [ratio, setRatio] = useState("1")
  const [saving, setSaving] = useState(false)
  const [expanded, setExpanded] = useState(defaultExpanded)
  useEffect(() => setExpanded(defaultExpanded), [defaultExpanded])

  function reset() {
    setEditingID(null)
    setName("")
    setRatio("1")
  }

  function beginEdit(rate: RateSnapshot) {
    if (rate.source === "relay_account") return
    setEditingID(rate.id)
    setName(rate.model_name)
    setRatio(String(rate.ratio))
  }

  async function save() {
    const trimmed = name.trim()
    const value = Number(ratio)
    if (!trimmed) return toast.error("请填写分组名称")
    if (!Number.isFinite(value) || value < 0) return toast.error("倍率必须是非负数字")
    setSaving(true)
    try {
      await apiFetch(`/channels/${channelID}/rates${editingID ? `/${editingID}` : ""}`, {
        method: editingID ? "PUT" : "POST",
        body: JSON.stringify({ model_name: trimmed, ratio: value, completion_ratio: value }),
      })
      toast.success(editingID ? "分组已更新" : "分组已创建")
      reset()
      await refetch()
    } catch (error) {
      toast.error((error as Error).message || "保存分组失败")
    } finally {
      setSaving(false)
    }
  }

  async function remove(rate: RateSnapshot) {
    if (!window.confirm(`删除分组“${rate.model_name}”？`)) return
    try {
      await apiFetch(`/channels/${channelID}/rates/${rate.id}`, { method: "DELETE" })
      toast.success("分组已删除")
      if (editingID === rate.id) reset()
      await refetch()
    } catch (error) {
      toast.error((error as Error).message || "删除分组失败")
    }
  }

  return (
    <div className="mt-3 flex-1 border-t border-border pt-3">
      <button type="button" className="mb-2.5 flex w-full items-center justify-between gap-2 text-left" onClick={() => setExpanded((value) => !value)} aria-expanded={expanded}>
        <div className="flex min-w-0 items-center gap-2">
          <p className="text-xs font-semibold text-foreground">渠道分组</p>
          <span className="rounded-full bg-brand/10 px-1.5 py-0.5 text-[10px] font-medium text-brand">成本绑定</span>
        </div>
        <span className="inline-flex shrink-0 items-center gap-1 text-[10px] tabular-nums text-muted-foreground">{(data ?? []).length} 个<ChevronDown className={cn("size-3 transition-transform duration-200", expanded && "rotate-180")} /></span>
      </button>
      {expanded ? <>
      <div className="rounded-lg border border-border bg-muted/20 p-2">
        <div className="mb-1.5 flex items-center justify-between px-0.5">
          <span className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">{editingID ? "编辑分组" : "新增分组"}</span>
          {editingID ? <button type="button" className="min-h-11 px-1 text-[11px] text-muted-foreground transition-colors hover:text-foreground" onClick={reset}>取消</button> : null}
        </div>
        <div className="grid grid-cols-[minmax(0,1fr)_5.25rem_auto] gap-1.5">
          <input aria-label="分组名称" value={name} onChange={(event) => setName(event.target.value)} placeholder="例如：Claude 主力" className="h-10 min-w-0 rounded-md border border-input bg-background px-2.5 text-xs outline-none transition-shadow placeholder:text-muted-foreground/60 focus-visible:ring-2 focus-visible:ring-ring" />
          <div className="relative">
            <input aria-label="倍率" value={ratio} onChange={(event) => setRatio(event.target.value)} inputMode="decimal" placeholder="倍率" className="h-10 w-full rounded-md border border-input bg-background px-2.5 pr-6 text-xs tabular-nums outline-none transition-shadow placeholder:text-muted-foreground/60 focus-visible:ring-2 focus-visible:ring-ring" />
            <span className="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 text-[10px] text-muted-foreground">×</span>
          </div>
          <Button type="button" size="sm" className="h-10 min-w-16 gap-1 px-2 text-xs" disabled={saving} onClick={() => void save()}>{editingID ? <CheckCircle2 className="size-3.5" /> : <Plus className="size-3.5" />}{editingID ? "保存" : "添加"}</Button>
        </div>
      </div>
      {loading ? <p className="mt-2 px-1 text-[11px] text-muted-foreground">加载分组中…</p> : (data ?? []).length === 0 ? <p className="mt-2 px-1 text-[11px] text-muted-foreground">暂无渠道分组</p> : (
        <div className={cn("mt-2 grid grid-cols-1 gap-2 @sm:grid-cols-2", (data ?? []).length > 6 && "max-h-72 overflow-y-auto pr-1")}>
          {(data ?? []).map((rate) => (
            <div key={rate.id} className="group min-w-0 rounded-lg border border-border/80 bg-card p-2 transition-colors hover:border-brand/40 hover:bg-muted/20">
              <div className="flex min-w-0 items-start gap-1.5">
                <Tooltip delayDuration={150}>
                  <TooltipTrigger asChild>
                    <span className="inline-flex min-w-0 flex-1 items-center gap-1.5 rounded-sm pt-0.5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" tabIndex={0}>
                      <span className="min-w-0 truncate text-xs font-medium" title={rate.model_name}>{rate.model_name}</span>
                      {showRateChanges ? <RateChangeDot change={rate.latest_ratio_change} /> : null}
                    </span>
                  </TooltipTrigger>
                  <TooltipContent side="top" className="max-w-xs text-xs">
                    <RateTooltipBody rate={rate} showChange={showRateChanges} />
                  </TooltipContent>
                </Tooltip>
                {rate.source === "relay_account" ? null : <div className="-mr-1 -mt-1 flex shrink-0 items-center"><Button type="button" variant="ghost" size="icon" aria-label={`编辑 ${rate.model_name}`} className="size-9 text-muted-foreground opacity-70 transition-opacity hover:text-foreground group-hover:opacity-100" onClick={() => beginEdit(rate)}><Pencil className="size-3.5" /></Button><Button type="button" variant="ghost" size="icon" aria-label={`删除 ${rate.model_name}`} className="size-9 text-muted-foreground opacity-70 transition-opacity hover:text-destructive group-hover:opacity-100" onClick={() => void remove(rate)}><Trash2 className="size-3.5" /></Button></div>}
              </div>
              <div className="mt-1.5 flex min-w-0 flex-wrap items-center gap-1.5">
                <span className="shrink-0 rounded bg-muted px-1.5 py-0.5 font-mono text-[10px] font-medium tabular-nums text-foreground">{rate.ratio.toFixed(3)}×</span>
                {rate.source === "relay_account" ? <span className="shrink-0 rounded bg-sky-500/10 px-1.5 py-0.5 text-[10px] font-medium text-sky-700 dark:text-sky-400">自动关联</span> : null}
                <BoundAccountsPreview accounts={rate.bound_accounts} />
              </div>
            </div>
          ))}
        </div>
      )}
      </> : null}
    </div>
  )
}

interface ChannelSyncState {
  running: boolean
  events: ProgressEvent[]
  latest: ProgressEvent | null
  finalOk: boolean | null
  fading: boolean
}

function emptySyncState(): ChannelSyncState {
  return { running: false, events: [], latest: null, finalOk: null, fading: false }
}

const stageLabel: Record<ProgressEvent["stage"], string> = {
  captcha: "打码",
  session: "会话",
  login: "登录",
  balance: "余额",
  rates: "倍率",
  done: "完成",
  error: "失败",
}

const stageOrder: Record<ProgressEvent["stage"], number> = {
  captcha: 1,
  session: 2,
  login: 3,
  balance: 4,
  rates: 5,
  done: 9,
  error: 9,
}

/** 按 stage 去重，每个 stage 只留最后一条事件（"在做中→完成"会被覆盖成完成态）。 */
function deriveSteps(events: ProgressEvent[]): ProgressEvent[] {
  const byStage = new Map<ProgressEvent["stage"], ProgressEvent>()
  for (const ev of events) byStage.set(ev.stage, ev)
  return [...byStage.values()].sort((a, b) => stageOrder[a.stage] - stageOrder[b.stage])
}

function SyncProgressStrip({ state }: { state: ChannelSyncState }) {
  if (!state.running && state.latest == null) return null
  const steps = deriveSteps(state.events)

  return (
    <div
      className={cn(
        "mt-3 rounded-lg border border-border bg-muted/30 px-3 py-2.5",
        // 入场：上方滑入 + 淡入
        "animate-in fade-in slide-in-from-top-1 duration-300",
        // 出场：和 scheduleHide 里的 500ms 对齐
        "transition-all duration-500 ease-out",
        state.fading ? "-translate-y-0.5 opacity-0" : "opacity-100",
      )}
    >
      {steps.length === 0 ? (
        <div className="flex items-center gap-2 text-xs">
          <Loader2 className="size-3.5 shrink-0 animate-spin text-muted-foreground" />
          <span className="text-foreground/80">{"准备中…"}</span>
        </div>
      ) : (
        <ul className="space-y-1.5">
          {steps.map((ev) => {
            // 终止态：stage=done 或 error；显式 ok=true / false 也算
            const failed = ev.stage === "error" || ev.ok === false
            const succeeded = ev.stage === "done" || ev.ok === true
            const running = !failed && !succeeded
            const Icon = running ? Loader2 : failed ? XCircle : CheckCircle2
            const tone = running ? "text-muted-foreground" : failed ? "text-danger" : "text-success"
            return (
              <li
                key={ev.stage}
                className="flex items-center gap-2 text-xs animate-in fade-in duration-200"
              >
                <Icon
                  className={cn("size-3.5 shrink-0", tone, running && "animate-spin")}
                />
                <span className="w-9 shrink-0 text-[11px] text-muted-foreground">
                  {stageLabel[ev.stage]}
                </span>
                <span
                  className={cn(
                    "truncate",
                    failed ? "text-danger" : running ? "text-foreground/80" : "text-foreground",
                  )}
                >
                  {ev.message}
                </span>
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}

const DebouncedChannelSearchInput = memo(function DebouncedChannelSearchInput({ initialValue, resetVersion, onValueChange }: { initialValue: string; resetVersion: number; onValueChange: (value: string) => void }) {
  const [draft, setDraft] = useState(initialValue)
  const lastSubmittedValue = useRef(initialValue)
  const lastResetVersion = useRef(resetVersion)

  useEffect(() => {
    if (lastResetVersion.current === resetVersion) return
    lastResetVersion.current = resetVersion
    lastSubmittedValue.current = ""
    setDraft("")
  }, [resetVersion])

  useEffect(() => {
    if (draft === lastSubmittedValue.current) return
    const timer = window.setTimeout(() => {
      lastSubmittedValue.current = draft
      startTransition(() => onValueChange(draft))
    }, 80)
    return () => window.clearTimeout(timer)
  }, [draft, onValueChange])

  return <div className="relative min-w-0 flex-1 basis-64"><Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" aria-hidden="true" /><Input value={draft} onChange={(event) => setDraft(event.target.value)} placeholder="搜索渠道 / 模型 / 渠道备注 / 域名" aria-label="按渠道名称、模型、渠道备注或站点地址搜索" className="h-11 w-full pl-8 text-xs sm:h-9" /></div>
})

export function ChannelCards({ usageRange = "today", latencyGranularity = "auto", uiSettings }: { usageRange?: RelayUsageRange; latencyGranularity?: LatencyBucketGranularity; uiSettings: SystemSettings }) {
  const { data: channels, loading, error, refetch } = useChannels()
  const associations = useChannelRelayAssociations()
  const modelCatalog = useChannelModelCatalog()
  const [monitorStatus, setMonitorStatus] = useState<MonitorStatusFilter>("enabled")
  const [managementMode, setManagementMode] = useState<ManagementModeFilter>("all")
  const [accountCount, setAccountCount] = useState<AccountCountFilter>("all")
  const [channelType, setChannelType] = useState<ChannelTypeFilter>("all")
  const [associationFilter, setAssociationFilter] = useState<AssociationFilter>("all")
  const [searchQuery, setSearchQuery] = useState("")
  const [channelSearchResetVersion, setChannelSearchResetVersion] = useState(0)
  const [viewMode, setViewMode] = usePersistedChannelViewMode()
  const [preferredColumns, setVisibleColumns] = usePersistedChannelColumns()
  const visibleColumns = useMemo(() => new Set([...preferredColumns].filter((column) => {
    const section = columnVisibilitySections[column]
    return !section || channelSectionVisible(uiSettings, section)
  })), [preferredColumns, uiSettings])
  const recentLatencyLimit = 60
  const listGridStyle = useMemo(() => channelListGridStyle(visibleColumns), [visibleColumns])
  const [sortKey, setSortKey] = useState<ChannelSortKey>("score")
  const [sortDirection, setSortDirection] = useState<SortDirection>("desc")
  const activeFilterCount = Number(monitorStatus !== "all") + Number(managementMode !== "all") + Number(accountCount !== "all") + Number(channelType !== "all") + Number(associationFilter !== "all")
  const activeAssociationLabel = associationFilter === "unlinked"
    ? "未关联"
    : associationFilter.startsWith("station:")
      ? (associations.data?.stations ?? []).find((station) => station.id === Number(associationFilter.slice("station:".length)))?.name ?? "指定中转站"
      : null
  const hasActiveFilters = activeFilterCount > 0 || searchQuery.trim() !== ""
  const usageRangeLabel = usageRanges.find((item) => item.value === usageRange)?.label ?? usageRange
  const metrics = useChannelMetrics(usageRange)
  const displayBucketCount = viewMode === "list" ? 30 : 60
  const recentVisible = channelSectionVisible(uiSettings, "recent") && (viewMode === "gallery" || visibleColumns.has("recent"))
  const stabilityVisible = channelSectionVisible(uiSettings, "stability") && (viewMode === "gallery" || visibleColumns.has("stability"))
  const consumptionVisible = channelSectionVisible(uiSettings, "consumption_trend") && (viewMode === "gallery" || visibleColumns.has("consumption_trend"))
  const latencyTrends = useChannelLatencyTrends(60, true)
  const relayLatencyTrends = useRelayLatencyTrends(60, recentVisible)
  const scoreLatencyBuckets = useChannelLatencyBuckets(usageRange, "auto", 60, true)
  const listAutoLatencyBuckets = useChannelLatencyBuckets(usageRange, "auto", 30, viewMode === "list" && latencyGranularity === "auto" && stabilityVisible)
  const fixedLatencyBuckets = useChannelLatencyBuckets(usageRange, latencyGranularity, displayBucketCount, latencyGranularity !== "auto" && stabilityVisible)
  const latencyBuckets = latencyGranularity === "auto" ? (viewMode === "list" ? listAutoLatencyBuckets : scoreLatencyBuckets) : fixedLatencyBuckets
  const consumptionBuckets = useChannelConsumptionBuckets(usageRange, latencyGranularity, displayBucketCount, consumptionVisible)
  const metricByChannel = useMemo(() => new Map((metrics.data ?? []).map((item) => [item.channel_id, item])), [metrics.data])
  const latencyByChannel = useMemo(() => new Map((latencyTrends.data ?? []).map((item) => [item.channel_id, item.samples])), [latencyTrends.data])
  const recentMetricsByChannel = useMemo(() => new Map(Array.from(latencyByChannel, ([channelID, samples]) => [channelID, recentRequestMetrics(samples, Date.now(), uiSettings.recent_metrics_window_hours)])), [latencyByChannel, uiSettings.recent_metrics_window_hours])
  const relayRecentSamplesByAccount = useMemo(() => new Map((relayLatencyTrends.data ?? []).map((item) => [relayAccountComparisonKey(item.station_id, item.account_id), item.samples])), [relayLatencyTrends.data])
  const latencyBucketsByChannel = useMemo(() => new Map((latencyBuckets.data ?? []).map((item) => [item.channel_id, item.buckets])), [latencyBuckets.data])
  const scoreLatencyBucketsByChannel = useMemo(() => new Map((scoreLatencyBuckets.data ?? []).map((item) => [item.channel_id, item.buckets])), [scoreLatencyBuckets.data])
  const consumptionBucketsByChannel = useMemo(() => new Map((consumptionBuckets.data ?? []).map((item) => [item.channel_id, item.buckets])), [consumptionBuckets.data])
  const relayLinksByChannel = useMemo(() => relayComparisonsByChannel(associations.data), [associations.data])
  const healthScoreByChannel = useMemo(() => {
    const scores = new Map<number, ReturnType<typeof latencyHealthScore>>()
    for (const channel of channels ?? []) {
      scores.set(channel.id, latencyHealthScore(latencyByChannel.get(channel.id) ?? [], scoreLatencyBucketsByChannel.get(channel.id) ?? []))
    }
    return scores
  }, [channels, latencyByChannel, scoreLatencyBucketsByChannel])
  const latestCallByChannel = useMemo(() => {
    const latest = new Map<number, number>()
    for (const series of latencyTrends.data ?? []) {
      for (const sample of series.samples) {
        const at = Date.parse(sample.created_at)
        if (!Number.isFinite(at)) continue
        if (at > (latest.get(series.channel_id) ?? 0)) latest.set(series.channel_id, at)
      }
    }
    return latest
  }, [latencyTrends.data])
  const latencySortMetricsByChannel = useMemo(() => {
    const metrics = new Map<number, { outputSpeedP50: number | null; outputSpeedP95: number | null; firstLatencyP50: number | null; firstLatencyP95: number | null; durationP50: number | null; durationP95: number | null }>()
    for (const [channelID, samples] of latencyByChannel) {
      const speeds = samples.map((sample) => outputTokenSpeed(sample.output_tokens, sample.duration_ms))
      const firstLatencies = samples.map((sample) => sample.first_token_ms)
      const durations = samples.map((sample) => sample.duration_ms)
      metrics.set(channelID, {
        outputSpeedP50: p50(speeds),
        outputSpeedP95: p95(speeds),
        firstLatencyP50: p50(firstLatencies),
        firstLatencyP95: p95(firstLatencies),
        durationP50: p50(durations),
        durationP95: p95(durations),
      })
    }
    return metrics
  }, [latencyByChannel])
  const linkedStationsByChannel = useMemo(() => buildLinkedStationsByChannel(associations.data), [associations.data])
  const associatedModelsByChannel = useMemo(() => new Map((modelCatalog.data ?? []).map((item) => [item.channel_id, item.models])), [modelCatalog.data])
  const filteredChannels = useMemo(() => {
    if (!channels) return []
    const statusPriority: Record<Status, number> = { failed: 4, low: 3, idle: 2, healthy: 1 }
    const timestamp = (value: string | null | undefined) => {
      if (!value) return null
      const parsed = Date.parse(value)
      return Number.isFinite(parsed) ? parsed : null
    }
    const finite = (value: number | null | undefined) => value != null && Number.isFinite(value) ? value : null
    const currentBalance = (channel: Channel) => {
      const metric = metricByChannel.get(channel.id)
      return channel.balance_mode === "manual" ? finite(metric?.current_balance ?? channel.manual_balance) : finite(channel.last_balance)
    }
    const compareNumber = (left: number | null, right: number | null) => {
      if (left == null && right == null) return 0
      if (left == null) return 1
      if (right == null) return -1
      const result = left - right
      return sortDirection === "asc" ? result : -result
    }

    return channels.filter((channel) => {
      const statusMatches = monitorStatus === "all" || channel.monitor_enabled === (monitorStatus === "enabled")
      const managementMatches = managementMode === "all" || channel.balance_mode === managementMode
      const isMultiAccount = (channel.accounts?.length ?? 0) > 1
      const accountCountMatches = accountCount === "all" || isMultiAccount === (accountCount === "multi")
      const typeMatches = channelType === "all" || channel.type === channelType
      const linkedStations = linkedStationsByChannel.get(channel.id) ?? new Set<number>()
      const associationMatches = associationFilter === "all"
        || associationFilter === "unlinked" && linkedStations.size === 0
        || associationFilter.startsWith("station:") && linkedStations.has(Number(associationFilter.slice("station:".length)))
      const searchMatches = channelMatchesSearch(channel, searchQuery, associatedModelsByChannel.get(channel.id))
      return statusMatches && managementMatches && accountCountMatches && typeMatches && associationMatches && searchMatches
    }).sort((left, right) => {
      let result = 0
      switch (sortKey) {
        case "score":
          result = compareNumber(healthScoreByChannel.get(left.id)?.score ?? null, healthScoreByChannel.get(right.id)?.score ?? null)
          break
        case "cache_hit_rate":
          result = compareNumber(recentMetricsByChannel.get(left.id)?.cacheHitRate ?? null, recentMetricsByChannel.get(right.id)?.cacheHitRate ?? null)
          break
        case "success_rate":
          result = compareNumber(recentMetricsByChannel.get(left.id)?.successRate ?? null, recentMetricsByChannel.get(right.id)?.successRate ?? null)
          break
        case "recent_call":
          result = compareNumber(latestCallByChannel.get(left.id) ?? null, latestCallByChannel.get(right.id) ?? null)
          break
        case "updated":
          result = compareNumber(timestamp(left.last_balance_at ?? left.updated_at), timestamp(right.last_balance_at ?? right.updated_at))
          break
        case "output_speed_p50":
          result = compareNumber(latencySortMetricsByChannel.get(left.id)?.outputSpeedP50 ?? null, latencySortMetricsByChannel.get(right.id)?.outputSpeedP50 ?? null)
          break
        case "output_speed_p95":
          result = compareNumber(latencySortMetricsByChannel.get(left.id)?.outputSpeedP95 ?? null, latencySortMetricsByChannel.get(right.id)?.outputSpeedP95 ?? null)
          break
        case "first_latency_p50":
          result = compareNumber(latencySortMetricsByChannel.get(left.id)?.firstLatencyP50 ?? null, latencySortMetricsByChannel.get(right.id)?.firstLatencyP50 ?? null)
          break
        case "first_latency_p95":
          result = compareNumber(latencySortMetricsByChannel.get(left.id)?.firstLatencyP95 ?? null, latencySortMetricsByChannel.get(right.id)?.firstLatencyP95 ?? null)
          break
        case "duration_latency_p50":
          result = compareNumber(latencySortMetricsByChannel.get(left.id)?.durationP50 ?? null, latencySortMetricsByChannel.get(right.id)?.durationP50 ?? null)
          break
        case "duration_latency_p95":
          result = compareNumber(latencySortMetricsByChannel.get(left.id)?.durationP95 ?? null, latencySortMetricsByChannel.get(right.id)?.durationP95 ?? null)
          break
        case "balance":
          result = compareNumber(currentBalance(left), currentBalance(right))
          break
        case "consumption":
          result = compareNumber(finite(metricByChannel.get(left.id)?.consumption_amount), finite(metricByChannel.get(right.id)?.consumption_amount))
          break
        case "usage_cost":
          result = compareNumber(finite(metricByChannel.get(left.id)?.usage_cost_amount), finite(metricByChannel.get(right.id)?.usage_cost_amount))
          break
        case "name":
          result = left.name.localeCompare(right.name, "zh-CN", { numeric: true, sensitivity: "base" }) * (sortDirection === "asc" ? 1 : -1)
          break
        case "account_count":
          result = compareNumber(Math.max(1, left.accounts?.length ?? 0), Math.max(1, right.accounts?.length ?? 0))
          break
        case "status":
          result = compareNumber(statusPriority[statusOf(left, currentBalance(left))], statusPriority[statusOf(right, currentBalance(right))])
          break
        case "ratio_change":
        default:
          result = compareNumber(timestamp(left.latest_ratio_changed_at), timestamp(right.latest_ratio_changed_at))
          break
      }
      return result || left.id - right.id
    })
  }, [accountCount, associatedModelsByChannel, associationFilter, channelType, channels, healthScoreByChannel, latestCallByChannel, latencySortMetricsByChannel, linkedStationsByChannel, managementMode, metricByChannel, monitorStatus, recentMetricsByChannel, searchQuery, sortDirection, sortKey])
  const refresh = useTriggerRefresh()
  const { confirm, dialog: confirmDialog } = useConfirm()
  const [editing, setEditing] = useState<Channel | null>(null)
  const [creating, setCreating] = useState(false)
  const [busyAction, setBusyAction] = useState<string | null>(null)
  // 每个渠道当前 sync 进度（最新一条事件） + 历史事件
  const [syncState, setSyncState] = useState<Record<number, ChannelSyncState>>({})

  // 成功后自动消失需要的两段定时器：先 5s 显示，再 500ms 过渡（与 strip 的 transition-opacity duration-500 对齐）。
  const hideTimers = useRef<Map<number, ReturnType<typeof setTimeout>>>(new Map())

  useEffect(() => {
    const timers = hideTimers.current
    return () => {
      timers.forEach((t) => clearTimeout(t))
      timers.clear()
    }
  }, [])

  function clearHideTimer(id: number) {
    const t = hideTimers.current.get(id)
    if (t != null) {
      clearTimeout(t)
      hideTimers.current.delete(id)
    }
  }

  function scheduleHide(id: number) {
    clearHideTimer(id)
    const t1 = setTimeout(() => {
      patchSync(id, (prev) => ({ ...prev, fading: true }))
      const t2 = setTimeout(() => {
        setSyncState((s) => {
          const { [id]: _gone, ...rest } = s
          void _gone
          return rest
        })
        hideTimers.current.delete(id)
      }, 500)
      hideTimers.current.set(id, t2)
    }, 5000)
    hideTimers.current.set(id, t1)
  }

  function patchSync(id: number, fn: (prev: ChannelSyncState) => ChannelSyncState) {
    setSyncState((s) => ({ ...s, [id]: fn(s[id] ?? emptySyncState()) }))
  }

  async function startStream(channel: Channel, action: "sync" | "test-login") {
    clearHideTimer(channel.id)
    patchSync(channel.id, () => ({
      running: true,
      events: [],
      latest: null,
      finalOk: null,
      fading: false,
    }))
    let sawError = false
    const stream = action === "sync" ? syncChannelStream : testLoginStream
    try {
      await stream(channel.id, {
        onEvent: (ev) => {
          if (ev.stage === "error" || ev.ok === false) sawError = true
          patchSync(channel.id, (prev) => ({
            ...prev,
            events: [...prev.events, ev],
            latest: ev,
          }))
        },
      })
      const ok = !sawError
      patchSync(channel.id, (prev) => ({
        ...prev,
        running: false,
        finalOk: ok,
      }))
      if (ok) scheduleHide(channel.id)
    } catch (e) {
      const err = e as Error
      const failureLabel = action === "sync" ? "同步失败" : "测试登录失败"
      patchSync(channel.id, (prev) => ({
        ...prev,
        running: false,
        finalOk: false,
        latest: {
          stage: "error",
          message: err.message || failureLabel,
          time: new Date().toISOString(),
        },
      }))
      // 失败保留，不调度自动隐藏
    } finally {
      refresh()
    }
  }

  async function withBusy(key: string, fn: () => Promise<unknown>) {
    setBusyAction(key)
    try {
      await fn()
      refresh()
    } catch (e) {
      const err = e as Error
      toast.error(err.message || "操作失败")
    } finally {
      setBusyAction(null)
    }
  }

  async function syncAll() {

    setBusyAction("sync-all")
    try {
      const result = await apiFetch<{ total: number; succeeded: number; failed: number }>("/channels/sync-all", { method: "POST" })
      toast.success(`已同步 ${result.succeeded}/${result.total} 个渠道${result.failed > 0 ? `，${result.failed} 个失败` : ""}`)
      refresh()
    } catch (e) {
      toast.error((e as Error).message || "批量同步失败")
    } finally {
      setBusyAction(null)
    }
  }

  function resetFilters() {
    setMonitorStatus("all")
    setManagementMode("all")
    setAccountCount("all")
    setChannelType("all")
    setAssociationFilter("all")
    setSearchQuery("")
    setChannelSearchResetVersion((value) => value + 1)
  }

  return (
    <section>
      <Card className="gap-0 overflow-hidden border border-border py-0 shadow-none">
      <ListPanelHeader
          secondaryPlacement="inline"
          actionsPlacement="secondary"
          title={<CardTitle role="heading" aria-level={2} className="flex items-center gap-2 text-sm font-semibold"><LayoutGrid className="size-4 text-brand" />{"渠道列表"}</CardTitle>}
          description={<p className="mt-1 truncate text-xs text-muted-foreground">{"监控状态、当前余额与最近同步结果"}</p>}
actions={<>
<Button
            size="sm"
            variant="outline"
            className="h-11 gap-1.5 text-xs sm:h-9"
            disabled={busyAction === "sync-all" || channels == null || channels.length === 0}
            onClick={() => void syncAll()}
          >
            <RefreshCw className={cn("size-3.5", busyAction === "sync-all" && "animate-spin")} />
            {"同步全部"}
          </Button>
<Button
            size="sm"
            className="h-11 gap-1.5 text-xs sm:h-9"
            onClick={() => {
              setEditing(null)
              setCreating(true)
            }}
          >
            <Plus className="size-3.5" />
            {"新增"}
          </Button>
</>}
          secondary={<>
<DebouncedChannelSearchInput initialValue={searchQuery} resetVersion={channelSearchResetVersion} onValueChange={setSearchQuery} />
<DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button type="button" size="sm" variant="outline" className="h-11 gap-1.5 text-xs sm:h-9">
                <SlidersHorizontal className="size-3.5" />
                筛选
                {activeFilterCount > 0 ? <span className="rounded-full bg-brand/10 px-1.5 py-0.5 text-[10px] font-semibold text-brand">{activeFilterCount}</span> : null}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-64">
              <DropdownMenuLabel className="text-xs text-muted-foreground">渠道筛选</DropdownMenuLabel>
              <DropdownMenuSeparator />

              <DropdownMenuSub>
                <ActiveFilterSubTrigger label="监控状态" value={monitorStatus !== "all" ? monitorStatusLabels[monitorStatus] : null} />
                <DropdownMenuSubContent className="w-44">
                  <DropdownMenuRadioGroup value={monitorStatus} onValueChange={(value) => setMonitorStatus(value as MonitorStatusFilter)}>
                    {(["enabled", "disabled", "all"] as MonitorStatusFilter[]).map((value) => <DropdownMenuRadioItem key={value} value={value}>{monitorStatusLabels[value]}</DropdownMenuRadioItem>)}
                  </DropdownMenuRadioGroup>
                </DropdownMenuSubContent>
              </DropdownMenuSub>

              <DropdownMenuSub>
                <ActiveFilterSubTrigger label="管理方式" value={managementMode !== "all" ? managementModeLabels[managementMode] : null} />
                <DropdownMenuSubContent className="w-44">
                  <DropdownMenuRadioGroup value={managementMode} onValueChange={(value) => setManagementMode(value as ManagementModeFilter)}>
                    {(["all", "auto", "manual"] as ManagementModeFilter[]).map((value) => <DropdownMenuRadioItem key={value} value={value}>{managementModeLabels[value]}</DropdownMenuRadioItem>)}
                  </DropdownMenuRadioGroup>
                </DropdownMenuSubContent>
              </DropdownMenuSub>

              <DropdownMenuSub>
                <ActiveFilterSubTrigger label="账号数量" value={accountCount !== "all" ? accountCountLabels[accountCount] : null} />
                <DropdownMenuSubContent className="w-44">
                  <DropdownMenuRadioGroup value={accountCount} onValueChange={(value) => setAccountCount(value as AccountCountFilter)}>
                    {(["all", "multi", "single"] as AccountCountFilter[]).map((value) => <DropdownMenuRadioItem key={value} value={value}>{accountCountLabels[value]}</DropdownMenuRadioItem>)}
                  </DropdownMenuRadioGroup>
                </DropdownMenuSubContent>
              </DropdownMenuSub>

              <DropdownMenuSub>
                <ActiveFilterSubTrigger label="站点类型" value={channelType !== "all" ? channelTypeLabels[channelType] : null} />
                <DropdownMenuSubContent className="w-44">
                  <DropdownMenuRadioGroup value={channelType} onValueChange={(value) => setChannelType(value as ChannelTypeFilter)}>
                    {(["all", "newapi", "sub2api"] as ChannelTypeFilter[]).map((value) => <DropdownMenuRadioItem key={value} value={value}>{channelTypeLabels[value]}</DropdownMenuRadioItem>)}
                  </DropdownMenuRadioGroup>
                </DropdownMenuSubContent>
              </DropdownMenuSub>

              <DropdownMenuSub>
                <ActiveFilterSubTrigger label="关联" value={activeAssociationLabel} />
                <DropdownMenuSubContent className="w-48">
                  <DropdownMenuRadioGroup value={associationFilter} onValueChange={(value) => setAssociationFilter(value as AssociationFilter)}>
                    <DropdownMenuRadioItem value="all">All</DropdownMenuRadioItem>
                    <DropdownMenuRadioItem value="unlinked">未关联</DropdownMenuRadioItem>
                    {(associations.data?.stations ?? []).map((station) => (
                      <DropdownMenuRadioItem key={station.id} value={`station:${station.id}`}>{station.name}</DropdownMenuRadioItem>
                    ))}
                  </DropdownMenuRadioGroup>
                </DropdownMenuSubContent>
              </DropdownMenuSub>

              <DropdownMenuSeparator />
              <DropdownMenuItem disabled={!hasActiveFilters} onSelect={resetFilters}>
                <RotateCcw className="size-3.5" />
                清除筛选与搜索
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
<DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button type="button" size="sm" variant="outline" className="h-11 gap-1.5 text-xs sm:h-9">
                排序：{channelSortOptions.find((item) => item.value === sortKey)?.label}
                {sortDirection === "desc" ? <ArrowDown className="size-3.5" /> : <ArrowUp className="size-3.5" />}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-56">
              <DropdownMenuLabel className="text-xs text-muted-foreground">排序维度</DropdownMenuLabel>
              <DropdownMenuRadioGroup
                value={sortKey}
                onValueChange={(value) => {
                  const next = value as ChannelSortKey
                  setSortKey(next)
                  setSortDirection(defaultSortDirection(next))
                }}
              >
                {channelSortOptions.map((item) => <DropdownMenuRadioItem key={item.value} value={item.value}>{item.label}</DropdownMenuRadioItem>)}
              </DropdownMenuRadioGroup>
              <DropdownMenuSeparator />
              <DropdownMenuLabel className="text-xs text-muted-foreground">排序方向</DropdownMenuLabel>
              <DropdownMenuRadioGroup value={sortDirection} onValueChange={(value) => setSortDirection(value as SortDirection)}>
                <DropdownMenuRadioItem value="desc"><ArrowDown className="size-3.5" />降序（数值高 → 低 / 时间新 → 旧）</DropdownMenuRadioItem>
                <DropdownMenuRadioItem value="asc"><ArrowUp className="size-3.5" />升序（数值低 → 高 / 时间旧 → 新）</DropdownMenuRadioItem>
              </DropdownMenuRadioGroup>
            </DropdownMenuContent>
          </DropdownMenu>
<div className="inline-flex items-center rounded-md border border-border bg-muted/20 p-0.5" aria-label="渠道列表展示模式">
            <Button type="button" size="sm" variant={viewMode === "gallery" ? "secondary" : "ghost"} className="h-8 gap-1.5 px-2.5 text-xs shadow-none" aria-pressed={viewMode === "gallery"} onClick={() => setViewMode("gallery")}><LayoutGrid className="size-3.5" />画册</Button>
            <Button type="button" size="sm" variant={viewMode === "list" ? "secondary" : "ghost"} className="h-8 gap-1.5 px-2.5 text-xs shadow-none" aria-pressed={viewMode === "list"} onClick={() => setViewMode("list")}><List className="size-3.5" />列表</Button>
          </div>
{viewMode === "list" ? <DropdownMenu>
            <DropdownMenuTrigger asChild><Button type="button" size="sm" variant="outline" className="h-11 gap-1.5 text-xs sm:h-9"><Columns3 className="size-3.5" />列设置</Button></DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-52">
              <DropdownMenuLabel>渠道列表列</DropdownMenuLabel>
              <DropdownMenuSeparator />
              {channelColumnDefinitions.map((column) => <DropdownMenuCheckboxItem key={column.key} checked={preferredColumns.has(column.key)} disabled={Boolean(columnVisibilitySections[column.key] && !channelSectionVisible(uiSettings, columnVisibilitySections[column.key]!))} onSelect={(event) => event.preventDefault()} onCheckedChange={(checked) => setVisibleColumns((current) => { const next = new Set(current); if (checked === true) next.add(column.key); else if (next.size > 1) next.delete(column.key); return next })}>{column.label}</DropdownMenuCheckboxItem>)}
              <DropdownMenuSeparator />
              <DropdownMenuItem onSelect={() => setVisibleColumns(new Set(defaultChannelColumns))}>恢复默认列</DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu> : null}
</>}
          meta={<><span className="whitespace-nowrap text-xs text-muted-foreground">
            {filteredChannels.length}{filteredChannels.length === channels?.length ? " 个渠道" : ` / ${channels?.length ?? 0} 个渠道`}
          </span></>}
        ></ListPanelHeader>

      <CardContent className="border-t border-border p-0">
      {error ? (
        <Alert variant="destructive" className="rounded-none border-0">
          <CircleAlert />
          <AlertTitle>{"渠道信息读取失败"}</AlertTitle>
          <AlertDescription>
            <p>{error}</p>
            <Button type="button" size="sm" variant="outline" className="mt-2 h-11 sm:h-9" onClick={refetch}>
              <RefreshCw className="size-3.5" />
              {"重新读取"}
            </Button>
          </AlertDescription>
        </Alert>
      ) : loading ? (
        <p className="px-4 py-8 text-center text-sm text-muted-foreground">
          {"加载中…"}
        </p>
      ) : !channels || channels.length === 0 ? (
        <div className="px-4 py-10 text-center">
          <p className="text-sm text-muted-foreground">{"还没有任何渠道。"}</p>
          <Button
            size="sm"
            className="mt-3 h-11 gap-1.5 sm:h-9"
            onClick={() => {
              setEditing(null)
              setCreating(true)
            }}
          >
            <Plus className="size-3.5" />
            {"添加第一个渠道"}
          </Button>
        </div>
      ) : filteredChannels.length === 0 ? (
        <div className="px-4 py-10 text-center">
          <p className="text-sm text-muted-foreground">当前筛选没有渠道。</p>
          <Button type="button" size="sm" variant="outline" className="mt-3 h-11 sm:h-9" onClick={resetFilters}>
            清除筛选
          </Button>
        </div>
      ) : viewMode === "gallery" ? (
        <div className="grid grid-cols-1 items-stretch gap-3 p-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-3">
          {filteredChannels.map((c) => {
            const metric = metricByChannel.get(c.id)
            const currentBalance = c.balance_mode === "manual" ? (metric?.current_balance ?? c.manual_balance) : c.last_balance
			const accounts = channelAccounts(c)
            const status = statusOf(c, currentBalance)
            const meta = statusMap[status]
            const healthScore = healthScoreByChannel.get(c.id)
            const scoreTitle = healthScore
              ? `综合评分 ${healthScore.score} · 最近成功率 ${healthScore.recentSuccess ?? "-"}% · 最近延迟分 ${healthScore.recentLatency ?? "-"} · 时间稳定性分 ${healthScore.stability ?? "-"}`
              : "暂无足够调用数据计算综合评分"
            return (
              <Card key={c.id} className="@container flex h-full flex-col gap-0 border border-border p-4 shadow-none">
                <div className="flex flex-wrap items-center gap-2 border-b border-border pb-3">
                  <a
                    href={c.site_url}
                    target="_blank"
                    rel="noreferrer"
                    className="text-sm font-semibold text-foreground hover:text-brand hover:underline"
                  >
                    {c.name}
                  </a>
                  {channelSectionVisible(uiSettings, "rate_changes") && c.latest_ratio_changed_at ? (
                    <span className="text-[10px] text-muted-foreground" title="最近一次分组倍率变动">
                      {relativeTime(c.latest_ratio_changed_at)}
                    </span>
                  ) : null}
                  <div className="ml-auto flex shrink-0 items-center gap-2">
                    <span
                      className={cn(
                        "inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-semibold ring-1 ring-inset",
                        healthScore ? healthScoreClass(healthScore.score) : "bg-muted/50 text-muted-foreground ring-border",
                      )}
                      title={scoreTitle}
                    >
                      评分 {healthScore?.score ?? "--"}
                    </span>
                    {accounts.length > 1 ? (
                      <span className="inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-medium ring-1 ring-inset bg-cyan-500/10 text-cyan-700 ring-cyan-500/20 dark:text-cyan-400">
                        多账号渠道
                      </span>
                    ) : null}
                    <span className={cn("inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-medium ring-1 ring-inset", c.balance_mode === "manual" ? "bg-amber-400/15 text-amber-700 ring-amber-500/30 dark:text-amber-400" : "bg-sky-500/10 text-sky-700 ring-sky-500/20 dark:text-sky-400")}>
                      {c.balance_mode === "manual" ? "手动管理" : "自动读取"}
                    </span>
                    <span
                      className={cn(
                        "inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-medium ring-1 ring-inset",
                        c.type === "sub2api"
                          ? "bg-emerald-500/10 text-emerald-700 ring-emerald-500/20 dark:text-emerald-400"
                          : c.type === "newapi"
                            ? "bg-brand/10 text-brand ring-brand/20"
                            : "bg-foreground/5 text-foreground ring-border",
                      )}
                    >
                      {channelTypeLabel(c.type)}
                    </span>
                    {!c.monitor_enabled ? (
                      <span className="inline-flex items-center rounded bg-muted/60 px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground">
                        {"已暂停"}
                      </span>
                    ) : null}
                  </div>
                </div>

                <div className="mt-3 grid grid-cols-2 gap-px overflow-hidden rounded-md border border-border bg-border">
                  <Row label="站点">
                    <a href={c.site_url} target="_blank" rel="noreferrer" title={c.site_url} className="hover:text-brand hover:underline">
                      {siteHost(c.site_url)}
                    </a>
                  </Row>
				  {c.balance_mode !== "manual" ? <Row label="账号"><AccountPreview accounts={accounts} /></Row> : null}
                  <Row label="管理">{c.balance_mode === "manual" ? "手动余额" : "自动读取"}</Row>
                  {c.balance_mode !== "manual" ? <Row label="凭据">{c.credential_mode === "token" ? "Token" : "账号密码"}</Row> : null}
				  <Row label="余额"><span className={cn("font-mono text-base font-bold tabular-nums", balanceTone({ ...c, last_balance: currentBalance }))}>{metrics.loading && c.balance_mode === "manual" ? "读取中…" : c.balance_mode === "auto" ? <AccountBalance accounts={accounts} total={currentBalance} /> : money(currentBalance)}</span></Row>
                  <Row label="区间余额消耗"><span className="font-mono font-bold tabular-nums text-warning" title={`${usageRangeLabel}余额下降累计，充值不计入`}>{metrics.loading && !metric ? "读取中…" : money(metric?.consumption_amount)}</span></Row>
                  <Row label="累计充值"><span className="font-mono tabular-nums">{metrics.loading && !metric ? "读取中…" : cny(metric?.cumulative_recharge_amount)}</span></Row>
                  <Row label="区间调用消耗"><span className="font-mono tabular-nums" title={metric ? `${usageRangeLabel} · 渠道自身 usage/log · ${metric.usage_request_count} 次请求` : metrics.error ?? ""}>{metrics.loading && !metric ? "读取中…" : charge(metric?.usage_cost_amount)}</span></Row>
                  <Row label="阈值">{c.balance_threshold > 0 ? money(c.balance_threshold) : "未设置"}</Row>
                  <Row label="状态">
                    <span className={cn("inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-medium", meta.cls)}>
                      {meta.label}
                    </span>
                  </Row>
                  <Row label="渠道备注"><span title={c.remark || "未填写渠道备注"}>{c.remark || "未填写"}</span></Row>
                  <Row label="上次更新">{relativeTime(c.last_balance_at ?? c.updated_at)}</Row>
                  {c.last_error ? (
                    <Row label="管理采集异常" className="col-span-full">
                      <p className="truncate text-[11px] font-medium text-danger" title={managementErrorText(c.last_error)}>
                        {managementErrorText(c.last_error).length > 80 ? managementErrorText(c.last_error).slice(0, 80) + "…" : managementErrorText(c.last_error)}
                      </p>
                    </Row>
                  ) : null}
                </div>

                <RecentRequestMetrics samples={latencyByChannel.get(c.id) ?? []} loading={latencyTrends.loading} windowHours={uiSettings.recent_metrics_window_hours} />
                {channelSectionVisible(uiSettings, "recent") ? <LatencyStabilityLane
                  buckets={latencyBucketsByChannel.get(c.id) ?? []}
                  recentSamples={(latencyByChannel.get(c.id) ?? []).slice(0, recentLatencyLimit)}
                  loading={latencyBuckets.loading}
                  title={<RecentLatencyTitle samples={(latencyByChannel.get(c.id) ?? []).slice(0, recentLatencyLimit)} />}
                  headerRight={<span className="font-mono text-[10px] tabular-nums text-muted-foreground">最近 {Math.min(latencyByChannel.get(c.id)?.length ?? 0, recentLatencyLimit)} / {recentLatencyLimit} 条</span>}
                  layout="fill"
                  granularity={latencyGranularity}
                  relayLinks={relayLinksByChannel.get(c.id) ?? []}
                  relayRecentSamplesByAccount={relayRecentSamplesByAccount}
                  range={usageRange}
                  bucketCount={displayBucketCount}
                  emptyLabel={`${usageRangeLabel}暂无调用数据`}
                  loadSamples={(bucket) => apiFetch<ChannelUsageSample[]>("/channels/" + c.id + "/latency-samples?start=" + encodeURIComponent(bucket.start) + "&end=" + encodeURIComponent(bucket.end) + "&limit=30")}
                /> : null}
                {channelSectionVisible(uiSettings, "stability") ? <LatencyStabilityLane
                  buckets={latencyBucketsByChannel.get(c.id) ?? []}
                  loading={latencyBuckets.loading}
                  layout={latencyGranularity === "auto" ? "fill" : "fixed"}
                  granularity={latencyGranularity}
                  relayLinks={relayLinksByChannel.get(c.id) ?? []}
                  range={usageRange}
                  bucketCount={displayBucketCount}
                  emptyLabel={`${usageRangeLabel}暂无调用数据`}
                  loadSamples={(bucket) => apiFetch<ChannelUsageSample[]>("/channels/" + c.id + "/latency-samples?start=" + encodeURIComponent(bucket.start) + "&end=" + encodeURIComponent(bucket.end) + "&limit=30")}
                /> : null}
                {channelSectionVisible(uiSettings, "consumption_trend") ? <ChannelConsumptionTrend buckets={consumptionBucketsByChannel.get(c.id) ?? []} loading={consumptionBuckets.loading} granularity={latencyGranularity} relayLinks={relayLinksByChannel.get(c.id) ?? []} range={usageRange} bucketCount={displayBucketCount} emptyLabel={`${usageRangeLabel}暂无消费数据`} /> : null}
                {channelSectionVisible(uiSettings, "groups") ? (c.balance_mode !== "manual" ? <div className="mt-3 flex-1 border-t border-border"><InlineRates channelID={c.id} showRateChanges={channelSectionVisible(uiSettings, "rate_changes")} defaultExpanded={uiSettings.channels_groups_expanded} /></div> : <ManualRatesEditor channelID={c.id} showRateChanges={channelSectionVisible(uiSettings, "rate_changes")} defaultExpanded={uiSettings.channels_groups_expanded} />) : null}

				<div className="mt-3 grid grid-cols-3 gap-2">
				  <Button
                    variant="outline"
                    size="sm"
                    className="h-11 gap-1 text-xs sm:h-9"
                    disabled={!!syncState[c.id]?.running}
                    onClick={() => startStream(c, "sync")}
                  >
                    <RefreshCw
                      className={cn("size-3", syncState[c.id]?.running && "animate-spin")}
                    />
                    {c.balance_mode === "manual" ? "计算余额" : "同步"}
                  </Button>
                  {c.balance_mode !== "manual" ? <Button
                    variant="outline"
                    size="sm"
                    className="h-11 gap-1 text-xs sm:h-9"
                    disabled={!!syncState[c.id]?.running}
                    onClick={() => startStream(c, "test-login")}
                  >
                    <LogIn className="size-3" />
                    {"测试登录"}
                  </Button> : <Button variant="outline" size="sm" className="h-11 gap-1 text-xs sm:h-9" disabled>无需登录</Button>}
                  <Button
                    variant="outline"
                    size="sm"
                    className="h-11 gap-1 text-xs sm:h-9"
                    onClick={() => {
                      setEditing(c)
                      setCreating(true)
                    }}
                  >
					<Pencil className="size-3" />
					{c.balance_mode === "auto" ? "渠道 / 账号" : "编辑"}
                  </Button>
                </div>

                <SyncProgressStrip state={syncState[c.id] ?? emptySyncState()} />

                <div className="mt-3 flex items-center justify-between gap-2 border-t border-border pt-2.5">
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-11 gap-1 text-xs text-muted-foreground sm:h-9"
                    disabled={busyAction === `toggle-${c.id}`}
                    onClick={() =>
                      withBusy(`toggle-${c.id}`, () =>
                        apiFetch(`/channels/${c.id}/${c.monitor_enabled ? "disable" : "enable"}`, {
                          method: "POST",
                        }),
                      )
                    }
                  >
                    {c.monitor_enabled ? (
                      <Pause className="size-3" />
                    ) : (
                      <Play className="size-3" />
                    )}
                    {c.monitor_enabled ? "暂停监控" : "恢复监控"}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-11 gap-1 text-xs text-destructive hover:bg-destructive/10 hover:text-destructive sm:h-9"
                    disabled={busyAction === `delete-${c.id}`}
                    onClick={async () => {
                      const ok = await confirm({
                        title: `删除渠道 ${c.name}？`,
                        description: "删除后该渠道的余额历史、倍率快照与登录凭据都将一并清除，且无法恢复。",
                        confirmLabel: "删除",
                        destructive: true,
                      })
                      if (!ok) return
                      void withBusy(`delete-${c.id}`, () =>
                        apiFetch(`/channels/${c.id}`, { method: "DELETE" }),
                      )
                    }}
                  >
                    <Trash2 className="size-3" />
                    {"删除"}
                  </Button>
                </div>
              </Card>
            )
          })}
        </div>
      ) : (
        <div className="relative isolate max-h-[min(72vh,760px)] overflow-auto bg-card">
          <div style={listGridStyle} className="sticky top-0 z-40 hidden border-b border-border bg-muted text-[11px] font-medium text-foreground/90 lg:grid lg:grid-cols-[var(--channel-list-grid-template)] lg:min-w-[var(--channel-list-grid-min-width)] lg:[&>*]:px-4 lg:[&>*]:py-2">
            <span className="sticky left-0 z-50 flex items-center bg-muted">渠道</span>
            {visibleColumns.has("score") ? <span>综合评分</span> : null}
            {visibleColumns.has("type") ? <span>站点类型</span> : null}
            {visibleColumns.has("status") ? <span>状态</span> : null}
            {visibleColumns.has("management") ? <span>管理方式</span> : null}
            {visibleColumns.has("accounts") ? <span>账号</span> : null}
            {visibleColumns.has("balance") ? <span>余额</span> : null}
            {visibleColumns.has("consumption") ? <span>区间余额消耗</span> : null}
            {visibleColumns.has("usage_cost") ? <span>区间调用消耗</span> : null}
            {visibleColumns.has("recent") ? <span>最近调用</span> : null}
            {visibleColumns.has("stability") ? <span>时间稳定性</span> : null}
            {visibleColumns.has("consumption_trend") ? <span>调用消耗趋势</span> : null}
            {visibleColumns.has("updated") ? <span>上次更新</span> : null}
            {visibleColumns.has("remark") ? <span>渠道备注</span> : null}
            <span className="sticky right-0 z-50 flex items-center justify-end bg-muted">操作</span>
          </div>
          {filteredChannels.map((c) => {
            const metric = metricByChannel.get(c.id)
            const currentBalance = c.balance_mode === "manual" ? (metric?.current_balance ?? c.manual_balance) : c.last_balance
            const accounts = channelAccounts(c)
            const status = statusOf(c, currentBalance)
            const meta = statusMap[status]
            const displayStatus = c.monitor_enabled ? meta : { label: "已暂停", cls: "text-muted-foreground bg-muted/60" }
            const healthScore = healthScoreByChannel.get(c.id)
            const scoreTitle = healthScore
              ? `综合评分 ${healthScore.score} · 最近成功率 ${healthScore.recentSuccess ?? "-"}% · 最近延迟分 ${healthScore.recentLatency ?? "-"} · 时间稳定性分 ${healthScore.stability ?? "-"}`
              : "暂无足够调用数据计算综合评分"
            return <div key={c.id} style={listGridStyle} className="grid gap-3 border-b border-border px-4 py-3 last:border-b-0 lg:grid-cols-[var(--channel-list-grid-template)] lg:min-w-[var(--channel-list-grid-min-width)] lg:items-center lg:gap-0 lg:p-0 lg:[&>*]:px-4 lg:[&>*]:py-2.5">
              <div className="min-w-0 lg:sticky lg:left-0 lg:z-20 lg:flex lg:self-stretch lg:flex-col lg:justify-center lg:bg-card">
                <div className="flex min-w-0 items-center gap-2">
                  <a href={c.site_url} target="_blank" rel="noreferrer" className="min-w-0 truncate text-sm font-semibold text-foreground hover:text-brand hover:underline" title={c.name}>{c.name}</a>
                  {accounts.length > 1 ? <span className="shrink-0 rounded bg-cyan-500/10 px-1.5 py-0.5 text-[10px] font-medium text-cyan-700 dark:text-cyan-400">多账号</span> : null}
                </div>
                <a href={c.site_url} target="_blank" rel="noreferrer" className="mt-1 truncate text-[11px] text-muted-foreground hover:text-brand" title={c.site_url}>{siteHost(c.site_url)}</a>
                {c.last_error ? <p className="mt-1 truncate text-[10px] font-medium text-danger" title={managementErrorText(c.last_error)}>{managementErrorText(c.last_error)}</p> : null}
              </div>

              {visibleColumns.has("score") ? <div className="flex items-center justify-between gap-2 lg:block"><span className="text-[11px] text-muted-foreground lg:hidden">综合评分</span><span className={cn("inline-flex items-center rounded px-2 py-1 font-mono text-xs font-semibold tabular-nums ring-1 ring-inset", healthScore ? healthScoreClass(healthScore.score) : "bg-muted/50 text-muted-foreground ring-border")} title={scoreTitle}>{healthScore?.score ?? "--"}</span></div> : null}
              {visibleColumns.has("type") ? <div className="flex items-center justify-between gap-2 lg:block"><span className="text-[11px] text-muted-foreground lg:hidden">站点类型</span><span className={cn("inline-flex rounded px-2 py-1 text-[11px] font-medium ring-1 ring-inset", c.type === "sub2api" ? "bg-emerald-500/10 text-emerald-700 ring-emerald-500/20 dark:text-emerald-400" : "bg-brand/10 text-brand ring-brand/20")}>{channelTypeLabel(c.type)}</span></div> : null}
              {visibleColumns.has("status") ? <div className="flex items-center justify-between gap-2 lg:block"><span className="text-[11px] text-muted-foreground lg:hidden">状态</span><span className={cn("inline-flex rounded px-2 py-1 text-[11px] font-medium", displayStatus.cls)}>{displayStatus.label}</span></div> : null}
              {visibleColumns.has("management") ? <div className="flex items-center justify-between gap-2 lg:block"><span className="text-[11px] text-muted-foreground lg:hidden">管理方式</span><span className={cn("inline-flex rounded px-2 py-1 text-[11px] font-medium ring-1 ring-inset", c.balance_mode === "manual" ? "bg-amber-400/15 text-amber-700 ring-amber-500/30 dark:text-amber-400" : "bg-sky-500/10 text-sky-700 ring-sky-500/20 dark:text-sky-400")}>{c.balance_mode === "manual" ? "手动管理" : "自动读取"}</span></div> : null}
              {visibleColumns.has("accounts") ? <div className="min-w-0"><p className="mb-1 text-[11px] text-muted-foreground lg:hidden">账号</p>{c.balance_mode === "manual" ? <span className="text-xs text-muted-foreground">手动渠道</span> : <div className="min-w-0 text-xs"><AccountPreview accounts={accounts} /><p className="mt-1 text-[10px] text-muted-foreground">{accounts.length} 个账号</p></div>}</div> : null}
              {visibleColumns.has("balance") ? <div className="flex items-center justify-between gap-2 lg:block"><span className="text-[11px] text-muted-foreground lg:hidden">余额</span><span className={cn("font-mono text-sm font-semibold tabular-nums", balanceTone({ ...c, last_balance: currentBalance }))}>{metrics.loading && c.balance_mode === "manual" ? "读取中…" : c.balance_mode === "auto" ? <AccountBalance accounts={accounts} total={currentBalance} /> : money(currentBalance)}</span></div> : null}
              {visibleColumns.has("consumption") ? <div className="flex items-center justify-between gap-2 lg:block"><span className="text-[11px] text-muted-foreground lg:hidden">区间余额消耗</span><span className="font-mono text-sm font-semibold tabular-nums text-warning">{metrics.loading && !metric ? "读取中…" : money(metric?.consumption_amount)}</span></div> : null}
              {visibleColumns.has("usage_cost") ? <div className="flex items-center justify-between gap-2 lg:block"><span className="text-[11px] text-muted-foreground lg:hidden">区间调用消耗</span><span className="font-mono text-xs font-semibold tabular-nums" title={metric ? `渠道自身 usage/log · ${metric.usage_request_count} 次请求` : metrics.error ?? ""}>{metrics.loading && !metric ? "读取中…" : charge(metric?.usage_cost_amount)}</span></div> : null}
              {channelSectionVisible(uiSettings, "recent") && visibleColumns.has("recent") ? <div className="min-w-0"><p className="mb-1 text-[11px] text-muted-foreground lg:hidden">最近调用</p><LatencyStabilityLane buckets={latencyBucketsByChannel.get(c.id) ?? []} recentSamples={(latencyByChannel.get(c.id) ?? []).slice(0, recentLatencyLimit)} loading={latencyBuckets.loading} layout="fill" compact granularity={latencyGranularity} relayLinks={relayLinksByChannel.get(c.id) ?? []} relayRecentSamplesByAccount={relayRecentSamplesByAccount} range={usageRange} bucketCount={displayBucketCount} emptyLabel={`${usageRangeLabel}暂无调用数据`} loadSamples={(bucket) => apiFetch<ChannelUsageSample[]>("/channels/" + c.id + "/latency-samples?start=" + encodeURIComponent(bucket.start) + "&end=" + encodeURIComponent(bucket.end) + "&limit=30")} /><RecentLatencyTitle samples={(latencyByChannel.get(c.id) ?? []).slice(0, recentLatencyLimit)} compact /><RecentRequestMetrics samples={latencyByChannel.get(c.id) ?? []} loading={latencyTrends.loading} windowHours={uiSettings.recent_metrics_window_hours} /></div> : null}
              {channelSectionVisible(uiSettings, "stability") && visibleColumns.has("stability") ? <div className="min-w-0"><p className="mb-1 text-[11px] text-muted-foreground lg:hidden">时间稳定性</p><LatencyStabilityLane buckets={latencyBucketsByChannel.get(c.id) ?? []} loading={latencyBuckets.loading} layout="fill" compact granularity={latencyGranularity} relayLinks={relayLinksByChannel.get(c.id) ?? []} range={usageRange} bucketCount={displayBucketCount} emptyLabel={`${usageRangeLabel}暂无调用数据`} loadSamples={(bucket) => apiFetch<ChannelUsageSample[]>("/channels/" + c.id + "/latency-samples?start=" + encodeURIComponent(bucket.start) + "&end=" + encodeURIComponent(bucket.end) + "&limit=30")} /></div> : null}
              {channelSectionVisible(uiSettings, "consumption_trend") && visibleColumns.has("consumption_trend") ? <div className="min-w-0"><p className="mb-1 text-[11px] text-muted-foreground lg:hidden">调用消耗趋势</p><ChannelConsumptionTrend buckets={consumptionBucketsByChannel.get(c.id) ?? []} loading={consumptionBuckets.loading} granularity={latencyGranularity} compact relayLinks={relayLinksByChannel.get(c.id) ?? []} range={usageRange} bucketCount={displayBucketCount} emptyLabel={`${usageRangeLabel}暂无消费数据`} /></div> : null}
              {visibleColumns.has("updated") ? <div className="flex items-center justify-between gap-2 text-xs lg:block"><span className="text-[11px] text-muted-foreground lg:hidden">上次更新</span><span className="text-muted-foreground" title={c.last_balance_at ?? c.updated_at}>{relativeTime(c.last_balance_at ?? c.updated_at)}</span></div> : null}
              {visibleColumns.has("remark") ? <div className="min-w-0"><p className="mb-1 text-[11px] text-muted-foreground lg:hidden">渠道备注</p><p className="line-clamp-2 text-xs text-muted-foreground" title={c.remark || "未填写渠道备注"}>{c.remark || "未填写"}</p></div> : null}

              <div className="flex min-w-0 flex-nowrap justify-start gap-1.5 lg:sticky lg:right-0 lg:z-30 lg:self-stretch lg:items-center lg:justify-end lg:bg-card">
                <Tooltip delayDuration={200}><TooltipTrigger asChild><TableActionButton type="button" className="shrink-0" aria-label={c.balance_mode === "manual" ? "计算余额" : "同步渠道"} disabled={!!syncState[c.id]?.running} onClick={() => startStream(c, "sync")}><RefreshCw className={cn("size-3.5", syncState[c.id]?.running && "animate-spin")} /></TableActionButton></TooltipTrigger><TooltipContent side="top">{c.balance_mode === "manual" ? "计算余额" : "同步"}</TooltipContent></Tooltip>
                <Tooltip delayDuration={200}><TooltipTrigger asChild><TableActionButton type="button" className="shrink-0" aria-label="测试登录" disabled={c.balance_mode === "manual" || !!syncState[c.id]?.running} onClick={() => startStream(c, "test-login")}><LogIn className="size-3.5" /></TableActionButton></TooltipTrigger><TooltipContent side="top">{c.balance_mode === "manual" ? "手动渠道无需登录" : "测试登录"}</TooltipContent></Tooltip>
                <Tooltip delayDuration={200}><TooltipTrigger asChild><TableActionButton type="button" className="shrink-0" aria-label="编辑渠道" onClick={() => { setEditing(c); setCreating(true) }}><Pencil className="size-3.5" /></TableActionButton></TooltipTrigger><TooltipContent side="top">{c.balance_mode === "auto" ? "渠道 / 账号" : "编辑"}</TooltipContent></Tooltip>
                <Tooltip delayDuration={200}><TooltipTrigger asChild><TableActionButton type="button" className="shrink-0" aria-label={c.monitor_enabled ? "暂停监控" : "恢复监控"} disabled={busyAction === `toggle-${c.id}`} onClick={() => withBusy(`toggle-${c.id}`, () => apiFetch(`/channels/${c.id}/${c.monitor_enabled ? "disable" : "enable"}`, { method: "POST" }))}>{c.monitor_enabled ? <Pause className="size-3.5" /> : <Play className="size-3.5" />}</TableActionButton></TooltipTrigger><TooltipContent side="top">{c.monitor_enabled ? "暂停监控" : "恢复监控"}</TooltipContent></Tooltip>
                <Tooltip delayDuration={200}><TooltipTrigger asChild><TableActionButton destructive type="button" className="shrink-0" aria-label="删除渠道" disabled={busyAction === `delete-${c.id}`} onClick={async () => { const ok = await confirm({ title: `删除渠道 ${c.name}？`, description: "删除后该渠道的余额历史、倍率快照与登录凭据都将一并清除，且无法恢复。", confirmLabel: "删除", destructive: true }); if (!ok) return; void withBusy(`delete-${c.id}`, () => apiFetch(`/channels/${c.id}`, { method: "DELETE" })) }}><Trash2 className="size-3.5" /></TableActionButton></TooltipTrigger><TooltipContent side="top">删除</TooltipContent></Tooltip>
              </div>
            </div>
          })}
        </div>
      )}
      </CardContent>
      </Card>

      <ChannelFormDialog
        open={creating}
        onOpenChange={(v) => {
          setCreating(v)
          if (!v) setEditing(null)
        }}
        channel={editing}
        associationSnapshot={associations.data ?? null}
        onAssociationChanged={associations.refetch}
      />

      {confirmDialog}
    </section>
  )
}
