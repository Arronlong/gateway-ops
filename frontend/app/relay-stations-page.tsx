"use client"

import { memo, startTransition, useEffect, useMemo, useRef, useState } from "react"
import type { CSSProperties, ReactNode } from "react"
import { Activity, Archive, ArrowDown, ArrowUp, ArrowUpDown, Box, Check, ChevronDown, CircleAlert, CircleDollarSign, CircleHelp, Cog, Columns3, Copy, ExternalLink, Gauge, GripVertical, History, KeyRound, Layers3, LayoutGrid, List, ListChecks, PencilLine, Play, Plus, Power, PowerOff, RefreshCw, RotateCcw, Save, Search, Server, Settings2, ShieldAlert, ShieldCheck, SlidersHorizontal, Trash2, TrendingDown, Users, X } from "lucide-react"
import { toast } from "sonner"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { TableActionButton } from "@/components/ui/table-action-button"
import { ListPanelHeader } from "@/components/ui/list-panel"
import { ListFilterMenu } from "@/components/ui/list-filter-menu"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuSeparator, DropdownMenuSub, DropdownMenuSubContent, DropdownMenuSubTrigger, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { useConfirm } from "@/components/ui/confirm-dialog"
import { apiFetch } from "@/lib/api"
import type { ChannelUsageSample, LatencyBucket, LatencyBucketGranularity, RelayAccountBatchActionResult, RelayAccountBatchCloneGroup, RelayAccountBatchCloneResult, RelayAccountConsumptionBucket, RelayLatencySample, RelayUsageRange, RelayAccountTestResult, RelayAccountView, RelayGroupOption, RelayGroupTestResult, RelayGroupView, RelayOverview, RelayRecentUsage, RelayStation, RelayStationAuthMode, RelayStationPayload, RelayStationType } from "@/lib/api-types"
import { useChannelConsumptionBuckets, useChannelLatencyBuckets, useChannelLatencyTrends, useChannelRelayAssociations, useChannels, usePrefetchRelayOverviews, useRelayAccountConsumptionBuckets, useRelayAccountModelCatalog, useRelayDailyConsumption, useRelayLatencyBuckets, useRelayOverview, useRelayRecentUsage, useRelayStations, useRelayUsage, useRelayUserBalanceHistory, useRelayUsers } from "@/lib/queries"
import { RelayAdjustmentLog } from "@/components/monitor/relay-adjustment-log"
import { UserManagement } from "@/components/relay/user-management"
import { PlatformBadge } from "@/components/relay/platform-badge"
import { LatencyStabilityLane } from "@/components/monitor/latency-stability-lane"
import { RelayDailyConsumptionTrend } from "@/components/monitor/relay-daily-consumption-trend"
import { RelayAccountConsumptionTrend } from "@/components/monitor/relay-account-consumption-trend"
import { formatLatency, latencyGranularityOptionsForRange, latencyHealthScore, latencyTextClass, latencyTone, normalizeLatencyGranularity, type LatencyHealthScore, type LatencyTone } from "@/lib/latency"
import { channelComparisonsByRelayAccount, relayAccountComparisonKey, type ChannelCompareLink } from "@/lib/channel-relay-association"
import { latestRelayLatencySamples, relayAccountDisplayBucketCount, relayAccountSearchText, type RelayAccountViewMode } from "@/lib/relay-account-view"
import { cn } from "@/lib/utils"

type StationForm = { type: RelayStationType; auth_mode: RelayStationAuthMode; name: string; base_url: string; api_key: string; username: string; password: string; admin_token: string; admin_cookie: string; admin_user_id: string; gateway_key: string }

export function createRelayStationForm(station?: RelayStation): StationForm {
  return { type: station?.type ?? "sub2api", auth_mode: station?.auth_mode ?? "password", name: station?.name ?? "", base_url: station?.base_url ?? "", username: station?.username ?? "", admin_user_id: station?.admin_user_id ?? "", api_key: "", password: "", admin_token: "", admin_cookie: "", gateway_key: "" }
}

export function buildRelayStationPayload(form: StationForm, station?: RelayStation): RelayStationPayload {
  const payload: RelayStationPayload = { type: form.type, name: form.name.trim(), base_url: form.base_url.trim() }
  if (!payload.name || !payload.base_url) throw new Error("名称和中转站地址不能为空")
  const sameType = station != null && (station.type ?? "sub2api") === form.type
  if (form.type === "sub2api") {
    if (form.api_key.trim()) payload.api_key = form.api_key.trim()
    else if (!sameType) throw new Error("请填写 Sub2API 管理员 API Key")
    return payload
  }
  payload.auth_mode = form.auth_mode
  const keepSecret = sameType && (station.auth_mode ?? "password") === form.auth_mode && station.api_key_configured
  if (form.auth_mode === "password") {
    const username = form.username.trim() || (sameType ? station.username?.trim() : "")
    if (!username) throw new Error("请填写 NewAPI 管理员用户名")
    payload.username = username
    if (form.password) payload.password = form.password
    else if (!keepSecret) throw new Error("请填写 NewAPI 管理员密码")
  } else {
    const userID = form.admin_user_id.trim() || (sameType ? station.admin_user_id?.trim() : "")
    if (!userID || !/^[1-9]\d*$/.test(userID)) throw new Error("NewAPI 管理员用户 ID 必须为正整数")
    payload.admin_user_id = userID
    const field = form.auth_mode === "access_token" ? "admin_token" : "admin_cookie"
    if (form[field].trim()) payload[field] = form[field].trim()
    else if (!keepSecret) throw new Error(form.auth_mode === "access_token" ? "请填写 NewAPI 管理员访问令牌" : "请填写 NewAPI 管理员 Cookie")
  }
  if (form.gateway_key.trim()) payload.gateway_key = form.gateway_key.trim()
  return payload
}

export function relayStationCapabilities(type: RelayStationType = "sub2api") {
  return { accountConcurrency: type === "sub2api", poolRetry: type === "sub2api", userLimits: type === "sub2api", accountCostProbe: type === "sub2api", groupTest: true, priority: true, scheduling: true, groupRate: true, userStatus: true, costBinding: true, monitoring: true }
}

export function buildRelayStationErrorMessage(type: RelayStationType, operation: "sync" | "save" | "group-models" | "group-test", detail: string) {
  if (type !== "newapi") return detail
  const unauthorized = /\b401\b|unauthori[sz]ed|unauthenticated/i.test(detail)
  const help = operation === "group-test"
    ? `NewAPI 真实分组调用${unauthorized ? "失败" : "需要可用令牌"}：系统会从当前管理员名下全量分页选择与该分组匹配的令牌；如配置网关 API Key，该 Key 也必须属于当前管理员并匹配该分组，留空可自动选择。管理员凭据用于管理接口，不是网关 API Key。`
    : `NewAPI 管理员${unauthorized ? "鉴权失败" : "接口请求失败"}：请检查管理员凭据（用户名/密码、访问令牌或 Cookie + 用户 ID）。分组模型来自管理员可访问的渠道配置，不需要 Gateway API Key；Gateway API Key 仅供真实分组调用。`
  return `${detail}\n${help}`
}

export const relayStationErrorMessage = buildRelayStationErrorMessage

export function RelayStationFields({ form, station, onChange }: { form: StationForm; station?: RelayStation; onChange: (form: StationForm) => void }) {
  const sameMode = station?.type === form.type && (station.auth_mode ?? "password") === form.auth_mode && station.api_key_configured
  const set = (key: keyof StationForm, value: string) => onChange({ ...form, [key]: value })
  const secretPlaceholder = sameMode ? "留空保留原凭据" : "请填写管理员凭据"
  return <>
    <div className="space-y-1.5"><Label htmlFor="relay-type">站点类型</Label><Select value={form.type} onValueChange={(value) => onChange({ ...createRelayStationForm(station?.type === value ? station : undefined), name: form.name, base_url: form.base_url, type: value as RelayStationType })}><SelectTrigger id="relay-type" className="w-full"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="sub2api">Sub2API</SelectItem><SelectItem value="newapi">NewAPI</SelectItem></SelectContent></Select></div>
    <div className="space-y-1.5"><Label htmlFor="relay-name">名称</Label><Input id="relay-name" autoComplete="off" value={form.name} onChange={(event) => set("name", event.target.value)} required /></div>
    <div className="space-y-1.5"><Label htmlFor="relay-url">中转站地址</Label><Input id="relay-url" type="url" autoComplete="off" value={form.base_url} onChange={(event) => set("base_url", event.target.value)} required /></div>
    {form.type === "sub2api" ? <div className="space-y-1.5"><Label htmlFor="relay-key">管理员 API Key</Label><Input id="relay-key" type="password" autoComplete="new-password" data-lpignore="true" value={form.api_key} onChange={(event) => set("api_key", event.target.value)} placeholder={station?.type === "sub2api" ? "留空保留原 API Key" : "粘贴 x-api-key"} required={station?.type !== "sub2api"} /></div> : <>
      <div className="space-y-1.5"><Label htmlFor="relay-auth-mode">管理员鉴权方式</Label><Select value={form.auth_mode} onValueChange={(value) => onChange({ ...form, auth_mode: value as RelayStationAuthMode, password: "", admin_token: "", admin_cookie: "" })}><SelectTrigger id="relay-auth-mode" className="w-full"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="password">用户名 / 密码</SelectItem><SelectItem value="access_token">访问令牌</SelectItem><SelectItem value="cookie">Cookie + 用户 ID</SelectItem></SelectContent></Select></div>
      {form.auth_mode === "password" ? <>
        <div className="space-y-1.5"><Label htmlFor="relay-username">管理员用户名</Label><Input id="relay-username" autoComplete="off" value={form.username} onChange={(event) => set("username", event.target.value)} placeholder={station?.username} required={!station?.username} /></div>
        <div className="space-y-1.5"><Label htmlFor="relay-password">管理员密码</Label><Input id="relay-password" type="password" autoComplete="new-password" value={form.password} onChange={(event) => set("password", event.target.value)} placeholder={secretPlaceholder} required={!sameMode} /></div>
      </> : <>
        <div className="space-y-1.5"><Label htmlFor={form.auth_mode === "access_token" ? "relay-admin-token" : "relay-admin-cookie"}>{form.auth_mode === "access_token" ? "管理员访问令牌" : "管理员 Cookie"}</Label><Input id={form.auth_mode === "access_token" ? "relay-admin-token" : "relay-admin-cookie"} type="password" autoComplete="new-password" value={form.auth_mode === "access_token" ? form.admin_token : form.admin_cookie} onChange={(event) => set(form.auth_mode === "access_token" ? "admin_token" : "admin_cookie", event.target.value)} placeholder={secretPlaceholder} required={!sameMode} /></div>
        <div className="space-y-1.5"><Label htmlFor="relay-admin-user-id">管理员用户 ID</Label><Input id="relay-admin-user-id" inputMode="numeric" pattern="[1-9][0-9]*" value={form.admin_user_id} onChange={(event) => set("admin_user_id", event.target.value)} required={!station?.admin_user_id} placeholder={station?.admin_user_id} /></div>
      </>}
      <div className="space-y-1.5"><Label htmlFor="relay-gateway-key">网关 API Key（可选）</Label><Input id="relay-gateway-key" type="password" autoComplete="new-password" value={form.gateway_key} onChange={(event) => set("gateway_key", event.target.value)} placeholder={station?.gateway_key_configured ? "已配置，留空保留" : "用于真实分组调用"} /></div>
      <p className="text-xs leading-5 text-muted-foreground md:col-span-2 xl:col-span-4">管理员凭据用于同步、管理和读取管理员可访问的渠道模型；真实分组测试自动选择当前管理员名下匹配该分组的令牌。可选 Gateway API Key 也必须归当前管理员所有且匹配该分组，留空时自动选择。两类凭据用途不同；编辑时秘密字段留空保留原值，切换鉴权模式时请提供新模式凭据。</p>
    </>}
  </>
}

function RelayStationEditor({ station, busy, onSave, onCancel }: { station?: RelayStation; busy: boolean; onSave: (form: StationForm) => void; onCancel: () => void }) {
  const [form, setForm] = useState<StationForm>(() => createRelayStationForm(station))

  function submit(event: React.FormEvent) {
    event.preventDefault()
    onSave(form)
  }

  return <form onSubmit={submit} autoComplete="off" className="grid gap-3 border border-border bg-card p-4 md:grid-cols-2 xl:grid-cols-4"><RelayStationFields form={form} station={station} onChange={setForm} /><div className="flex items-end gap-2"><Button type="submit" disabled={busy}>保存</Button><Button type="button" variant="outline" disabled={busy} onClick={onCancel}>取消</Button></div></form>
}

type RiskFilter = "all" | "adjustable" | "downgradable" | RelayAccountView["risk_state"]
type BatchMode = "channel_group" | "manual" | "groups" | "concurrency" | "priority" | "retry_count" | "model_type"
export function relayBatchModeForStation(type: RelayStationType, mode: BatchMode): BatchMode {
  return type === "newapi" && (mode === "concurrency" || mode === "retry_count") ? "channel_group" : mode
}
export function relayPriorityRequiresOAuth(type: RelayStationType, priority: number, accountType: string | undefined): boolean {
  return type !== "newapi" && priority === 1 && accountType?.toLowerCase() !== "oauth"
}
type SortDirection = "asc" | "desc"
type GroupSortKey = "name" | "rate"
type AccountSortKey = "score" | "recent_call" | "last_used" | "priority" | "cost" | "usage" | "name"
type SortState<K extends string> = { key: K; direction: SortDirection } | null
type RelayDetailModule = "usage" | "users" | "groups" | "accounts"
type AccountAdjustmentMode = "suggestion" | "downgrade"
type AccountAdjustmentSelection = Record<number, number[]>
type AccountAdjustmentDialogState = {
  mode: AccountAdjustmentMode
  title: string
  confirmLabel: string
  accounts: RelayAccountView[]
  totalCount: number
} | null
type BatchCloneFormRow = { id: string; name: string; api_key: string; base_url: string; model_type: string }
type BatchCloneFormGroup = { source_account_external_id: number; rows: BatchCloneFormRow[] }

const batchCloneableAccountTypes = new Set(["apikey", "upstream", "bedrock", "service_account"])

const usageRanges: { value: RelayUsageRange; label: string }[] = [
  { value: "today", label: "今天" },
  { value: "24h", label: "24 小时" },
  { value: "7d", label: "7 天" },
  { value: "30d", label: "30 天" },
  { value: "all", label: "All" },
]

const accountSortOptions: Array<{ value: AccountSortKey; label: string }> = [
  { value: "score", label: "综合评分" },
  { value: "recent_call", label: "最近调用" },
  { value: "last_used", label: "最近使用时间" },
  { value: "usage", label: "区间消费" },
  { value: "cost", label: "成本倍率" },
  { value: "priority", label: "优先级" },
  { value: "name", label: "账号名称" },
]

function defaultAccountSortDirection(key: AccountSortKey): SortDirection {
  return key === "name" || key === "priority" ? "asc" : "desc"
}
type AccountColumnKey = "score" | "platform" | "concurrency" | "capacity" | "priority" | "retry" | "schedulable" | "model" | "recent" | "stability" | "usage" | "consumption_trend" | "cost" | "groups" | "risk" | "profit" | "downgrade"

type AccountColumnDefinition = { key: AccountColumnKey; label: string; template: string; minWidth: number }

const accountColumnDefinitions: AccountColumnDefinition[] = [
  { key: "score", label: "综合评分", template: "106px", minWidth: 106 },
  { key: "platform", label: "平台", template: "132px", minWidth: 132 },
  { key: "concurrency", label: "并发数", template: "96px", minWidth: 96 },
  { key: "capacity", label: "容量", template: "120px", minWidth: 120 },
  { key: "priority", label: "优先级", template: "96px", minWidth: 96 },
  { key: "retry", label: "重试次数", template: "96px", minWidth: 96 },
  { key: "schedulable", label: "调度", template: "120px", minWidth: 120 },
  { key: "model", label: "模型类型", template: "152px", minWidth: 152 },
  { key: "recent", label: "最近调用", template: "212px", minWidth: 212 },
  { key: "stability", label: "时间稳定性", template: "260px", minWidth: 260 },
  { key: "usage", label: "区间消费", template: "minmax(172px, 0.9fr)", minWidth: 172 },
  { key: "consumption_trend", label: "消费趋势", template: "220px", minWidth: 220 },
  { key: "cost", label: "成本", template: "132px", minWidth: 132 },
  { key: "groups", label: "当前销售分组", template: "minmax(280px, 1.1fr)", minWidth: 280 },
  { key: "risk", label: "风险", template: "120px", minWidth: 120 },
  { key: "profit", label: "利润", template: "96px", minWidth: 96 },
  { key: "downgrade", label: "可降级", template: "130px", minWidth: 130 },
]

const defaultAccountColumns = accountColumnDefinitions.map((column) => column.key)

export function stationAccountColumns(type: RelayStationType = "sub2api") {
  const capabilities = relayStationCapabilities(type)
  return accountColumnDefinitions.filter((column) => !((column.key === "concurrency" || column.key === "capacity") && !capabilities.accountConcurrency) && !(column.key === "retry" && !capabilities.poolRetry))
}

export function NewAPIUserManagement({ stationID, range }: { stationID: number; range: RelayUsageRange }) {
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState("")
  const [busyID, setBusyID] = useState<number | null>(null)
  const [selected, setSelected] = useState<number[]>([])
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const users = useRelayUsers(stationID, { page, pageSize: 20, search, range, sortBy: "id", sortOrder: "desc", riskLevel: "all", registrationIP: "" })
  const rows = users.data?.items ?? []
  const selectableRows = rows.filter((user) => user.role !== "admin" && user.role !== "root")
  const allPageSelected = selectableRows.length > 0 && selectableRows.every((user) => selected.includes(user.id))
  async function updateStatus(id: number, status: "active" | "disabled") {
    setBusyID(id)
    try {
      await apiFetch(`/relay-stations/${stationID}/users/${id}/status`, { method: "PUT", body: JSON.stringify({ status }) })
      await users.refetch()
      toast.success("用户状态已更新")
    } catch (error) { toast.error(relayStationErrorMessage("newapi", "sync", error instanceof Error ? error.message : "更新用户状态失败")) }
    finally { setBusyID(null) }
  }
  async function deleteSelected() {
    if (selected.length === 0) return
    setDeleting(true)
    try {
      const result = await apiFetch<{ affected: number; skipped_admins: number; failed: number[] }>(`/relay-stations/${stationID}/users/batch-delete`, { method: "POST", body: JSON.stringify({ user_ids: selected }) })
      setSelected([])
      setDeleteDialogOpen(false)
      await users.refetch()
      const message = `已删除 ${result.affected} 个用户${result.skipped_admins ? `，跳过 ${result.skipped_admins} 个管理员` : ""}${result.failed.length ? `，${result.failed.length} 个失败` : ""}`
      if (result.failed.length) toast.warning(message)
      else toast.success(message)
    } catch (error) { toast.error(error instanceof Error ? error.message : "批量删除用户失败") }
    finally { setDeleting(false) }
  }
  return <Card className="gap-0 py-0 shadow-none"><CardHeader className="flex flex-wrap items-center justify-between gap-3 px-4 py-3"><CardTitle className="text-sm">NewAPI 用户管理</CardTitle><div className="flex min-w-0 flex-wrap items-center gap-2">{selected.length ? <Button variant="destructive" size="sm" onClick={() => setDeleteDialogOpen(true)} disabled={deleting}><Trash2 className="size-3.5" />删除 {selected.length} 个用户</Button> : null}<Input aria-label="搜索用户" placeholder="搜索用户名或邮箱" value={search} onChange={(event) => { setSearch(event.target.value); setPage(1); setSelected([]) }} /><Button variant="outline" onClick={() => void users.refetch()} disabled={users.loading || users.refreshing}>刷新</Button></div></CardHeader><CardContent className="space-y-3 border-t p-4">
    {users.error ? <Alert variant="destructive"><CircleAlert /><AlertDescription>{relayStationErrorMessage("newapi", "sync", users.error)}</AlertDescription></Alert> : null}
    <div className="overflow-x-auto"><table className="w-full text-left text-xs"><thead><tr className="border-b [&>th]:p-3"><th><Checkbox checked={allPageSelected} aria-label="选择当前页普通用户" onCheckedChange={(checked) => { const ids = selectableRows.map((user) => user.id); setSelected((current) => checked === true ? Array.from(new Set([...current, ...ids])) : current.filter((id) => !ids.includes(id))) }} /></th><th>ID</th><th>用户名 / 邮箱</th><th>角色</th><th>余额</th><th>区间消费</th><th>状态</th></tr></thead><tbody>{rows.map((user) => { const protectedUser = user.role === "admin" || user.role === "root"; return <tr key={user.id} className="border-b last:border-0 [&>td]:p-3"><td><Checkbox checked={selected.includes(user.id)} disabled={protectedUser || deleting} aria-label={`选择用户 ${user.username || user.email || user.id}`} onCheckedChange={(checked) => setSelected((current) => checked === true ? Array.from(new Set([...current, user.id])) : current.filter((id) => id !== user.id))} /></td><td>#{user.id}</td><td><p>{user.username || "-"}</p><p className="text-muted-foreground">{user.email || "-"}</p></td><td>{user.role}</td><td>{money(user.balance)}</td><td>{money(user.usage)}</td><td><div className="flex items-center gap-2"><Switch checked={user.status === "active"} aria-label={`${user.username || user.email}用户状态`} disabled={busyID != null || protectedUser} onCheckedChange={(checked) => void updateStatus(user.id, checked ? "active" : "disabled")} /><span>{user.status === "active" ? "已启用" : "已禁用"}</span></div></td></tr> })}</tbody></table></div>
    {users.loading ? <p className="text-xs text-muted-foreground">正在读取用户...</p> : !users.data?.items.length ? <p className="text-xs text-muted-foreground">暂无用户</p> : null}
    <div className="flex items-center justify-between gap-2 text-xs"><span>共 {users.data?.total ?? 0} 个用户</span><div className="flex items-center gap-2"><Button variant="outline" size="sm" disabled={page <= 1 || users.loading} onClick={() => { setSelected([]); setPage((value) => value - 1) }}>上一页</Button><span>{page} / {users.data?.pages || 1}</span><Button variant="outline" size="sm" disabled={page >= (users.data?.pages || 1) || users.loading} onClick={() => { setSelected([]); setPage((value) => value + 1) }}>下一页</Button></div></div>
  </CardContent><Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}><DialogContent><DialogHeader><DialogTitle>确认删除用户？</DialogTitle><DialogDescription>将删除已选中的 {selected.length} 个 New API 用户。管理员账号会被跳过，删除结果会显示在操作提示中。</DialogDescription></DialogHeader><DialogFooter><Button variant="outline" disabled={deleting} onClick={() => setDeleteDialogOpen(false)}>取消</Button><Button variant="destructive" disabled={deleting} onClick={() => void deleteSelected()}><Trash2 className="size-4" />{deleting ? "删除中..." : "确认删除"}</Button></DialogFooter></DialogContent></Dialog></Card>
}

function usePersistedAccountColumns(storageKey: string) {
  const [visibleColumns, setVisibleColumns] = useState<Set<AccountColumnKey>>(() => {
    try {
      const raw = window.localStorage.getItem(storageKey)
      if (raw) {
        const parsed = JSON.parse(raw) as unknown
        if (Array.isArray(parsed)) {
          const valid = parsed.filter((value): value is AccountColumnKey => defaultAccountColumns.includes(value as AccountColumnKey))
          const scoreMigrationKey = `${storageKey}:score-column-v1`
          if (!window.localStorage.getItem(scoreMigrationKey) && !valid.includes("score")) valid.unshift("score")
          if (valid.length) return new Set(valid)
        }
      }
    } catch {
      // Fall back to all columns when persisted settings are unavailable or invalid.
    }
    return new Set(defaultAccountColumns)
  })
  useEffect(() => {
    try {
      window.localStorage.setItem(storageKey, JSON.stringify(defaultAccountColumns.filter((key) => visibleColumns.has(key))))
      window.localStorage.setItem(`${storageKey}:score-column-v1`, "1")
    } catch {
      // Column visibility still works for the current session without persistence.
    }
  }, [storageKey, visibleColumns])
  return [visibleColumns, setVisibleColumns] as const
}

function accountGridStyle(visibleColumns: Set<AccountColumnKey>): CSSProperties {
  const visible = accountColumnDefinitions.filter((column) => visibleColumns.has(column.key))
  const template = ["64px", "minmax(202px, 1.1fr)", ...visible.map((column) => column.template), "minmax(256px, 0.9fr)"].join(" ")
  const minWidth = 64 + 202 + 256 + visible.reduce((sum, column) => sum + column.minWidth, 0)
  return {
    "--relay-account-grid-template": template,
    "--relay-account-grid-min-width": `${minWidth}px`,
  } as CSSProperties
}

const compactNumber = new Intl.NumberFormat("zh-CN", { notation: "compact", maximumFractionDigits: 2 })
const modelTypeHelp = "模型类型用于标记分组适用的模型范围。账号绑定模型类型后，在平台、账号类型等条件满足时，只会在设置了相同模型类型的分组之间自动调组；未绑定模型类型的账号不参与自动调组。不同分组可以设置相同模型类型，设置相同后会进入同一自动调组范围。"

function isMissingGroupAPIKeyError(error: unknown) {
  if (!error || typeof error !== "object") return false
  const candidate = error as { status?: unknown; body?: unknown }
  if (candidate.status !== 422 || !candidate.body || typeof candidate.body !== "object") return false
  return (candidate.body as { code?: unknown }).code === "group_admin_api_key_missing"
}

function ModelTypeHelp() {
  return <Tooltip delayDuration={200}><TooltipTrigger asChild><button type="button" className="cursor-help rounded text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" aria-label="查看模型类型说明"><CircleHelp className="size-3.5" /></button></TooltipTrigger><TooltipContent side="top" className="max-w-80 text-xs leading-5">{modelTypeHelp}</TooltipContent></Tooltip>
}

function usePersistedOpen(storageKey: string, defaultOpen = true) {
  const [open, setOpen] = useState(() => {
    try {
      const stored = window.localStorage.getItem(storageKey)
      return stored === "true" || stored === "false" ? stored === "true" : defaultOpen
    } catch {
      return defaultOpen
    }
  })
  useEffect(() => {
    try {
      window.localStorage.setItem(storageKey, String(open))
    } catch {
      // The disclosure still works for the current session without persistence.
    }
  }, [open, storageKey])
  return [open, setOpen] as const
}

function usePersistedDetailModule(storageKey: string, defaultModule: RelayDetailModule = "accounts") {
  const [module, setModule] = useState<RelayDetailModule>(() => {
    try {
      const stored = window.localStorage.getItem(storageKey)
      if (stored === "usage" || stored === "users" || stored === "groups" || stored === "accounts") return stored
    } catch {
      // Keep the default module when localStorage is unavailable.
    }
    return defaultModule
  })
  useEffect(() => {
    try {
      window.localStorage.setItem(storageKey, module)
    } catch {
      // The tab still works for the current session without persistence.
    }
  }, [module, storageKey])
  return [module, setModule] as const
}

function usePersistedRelayAccountViewMode() {
  const [mode, setMode] = useState<RelayAccountViewMode>(() => {
    try {
      const stored = window.localStorage.getItem("gatewayops_relay_account_view_mode")
      return stored === "gallery" || stored === "list" ? stored : "gallery"
    } catch {
      return "gallery"
    }
  })
  useEffect(() => {
    try {
      window.localStorage.setItem("gatewayops_relay_account_view_mode", mode)
    } catch {
      // The current session can still switch modes without persistence.
    }
  }, [mode])
  return [mode, setMode] as const
}

function SyncPlanHelp({ label, help }: { label: string; help: string }) {
  return <Tooltip delayDuration={200}><TooltipTrigger asChild><button type="button" className="shrink-0 rounded text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" aria-label={`查看${label}说明`}><CircleHelp className="size-3.5" /></button></TooltipTrigger><TooltipContent side="top" className="max-w-80 text-xs leading-5">{help}</TooltipContent></Tooltip>
}

function CompactPolicySelect({ label, help, value, options, formatOption, onValueChange, disabled }: { label: string; help: string; value: number; options: readonly number[]; formatOption: (value: number) => string; onValueChange: (value: number) => void; disabled?: boolean }) {
  const selectedLabel = formatOption(value)
  return <div className="flex min-w-0 flex-1 items-center gap-1.5 overflow-hidden"><span className="min-w-0 shrink truncate whitespace-nowrap text-xs font-medium">{label}：</span><Select value={String(value)} onValueChange={(nextValue) => onValueChange(Number(nextValue))} disabled={disabled}><SelectTrigger className="h-7 w-0 min-w-0 flex-1 shrink gap-0.5 overflow-hidden border-0 bg-transparent px-1 py-0 text-xs font-semibold text-brand shadow-none hover:bg-brand/10 focus-visible:ring-2 focus-visible:ring-ring [&_[data-slot=select-value]]:min-w-0 [&_[data-slot=select-value]]:truncate [&_svg]:size-3 [&_svg]:opacity-70" aria-label={`${label}：${selectedLabel}`}><SelectValue /></SelectTrigger><SelectContent>{options.map((optionValue) => <SelectItem key={optionValue} value={String(optionValue)}>{formatOption(optionValue)}</SelectItem>)}</SelectContent></Select><SyncPlanHelp label={label} help={help} /></div>
}

function SortButton({ label, active, direction, onClick, className, title }: { label: string; active: boolean; direction?: SortDirection; onClick: () => void; className?: string; title?: string }) {
  const Icon = !active ? ArrowUpDown : direction === "asc" ? ArrowUp : ArrowDown
  return <button type="button" onClick={onClick} title={title} className={cn("inline-flex cursor-pointer items-center gap-1 rounded-sm font-medium transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring", className)} aria-label={`${label}排序${active ? `，当前${direction === "asc" ? "升序" : "降序"}` : ""}`}><span>{label}</span><Icon className="size-3" /></button>
}

function accountURLHost(value: string | null | undefined) {
  const raw = value?.trim()
  if (!raw) return ""
  try {
    return new URL(raw).host
  } catch {
    return raw
  }
}

function publicFirst(groups: RelayGroupOption[]) {
  return [...groups].sort(
    (a, b) => Number(Boolean(a.is_exclusive)) - Number(Boolean(b.is_exclusive)) || a.name.localeCompare(b.name, "zh-CN"),
  )
}

function accountDowngradeGroups(account: RelayAccountView) {
  if (account.downgrade_groups?.length) return account.downgrade_groups
  return account.downgrade_group ? [account.downgrade_group] : []
}

const stateMeta: Record<RelayAccountView["risk_state"], { label: string; tone: string }> = {
  risk: { label: "亏损风险", tone: "bg-danger/10 text-danger" },
  no_safe_candidate: { label: "无安全候选", tone: "bg-yellow-400/15 text-yellow-700 dark:text-yellow-300" },
  no_profit: { label: "无盈利候选", tone: "bg-yellow-400/15 text-yellow-700 dark:text-yellow-300" },
  cost_unknown: { label: "成本未知", tone: "bg-warning/10 text-warning" },
  protected: { label: "分组安全", tone: "bg-success/10 text-success" },
  unassigned: { label: "未分配销售组", tone: "bg-muted text-muted-foreground" },
  inactive: { label: "账号未启用", tone: "bg-muted text-muted-foreground" },
}

function healthScoreClass(score: number) {
  if (score >= 85) return "bg-emerald-500/10 text-emerald-700 ring-emerald-500/20 dark:text-emerald-400"
  if (score >= 70) return "bg-amber-400/15 text-amber-700 ring-amber-500/30 dark:text-amber-400"
  return "bg-red-500/10 text-red-700 ring-red-500/20 dark:text-red-400"
}

function multiplier(value: number | null | undefined) {
  return value == null ? "-" : value.toFixed(3)
}

function profitPercent(margin: number | null | undefined, cost: number | null | undefined) {
  if (margin == null || cost == null || !Number.isFinite(margin) || !Number.isFinite(cost) || cost <= 0) return "-"
  return `${((margin / cost) * 100).toFixed(1)}%`
}

function tokenAmount(value: number | null | undefined) {
  if (value == null || !Number.isFinite(value)) return "-"
  return compactNumber.format(value)
}

function chargeAmount(value: number | null | undefined) {
  if (value == null || !Number.isFinite(value)) return "-"
  return `$${value.toLocaleString("en-US", { minimumFractionDigits: 6, maximumFractionDigits: 6 })}`
}

function capacityTone(account: RelayAccountView) {
  if (account.concurrency <= 0 || account.current_concurrency <= 0) return "border-border bg-muted text-muted-foreground"
  if (account.current_concurrency >= account.concurrency) return "border-danger/30 bg-danger/10 text-danger"
  return "border-warning/30 bg-warning/10 text-warning"
}

function GroupNames({ groups }: { groups: RelayGroupOption[] }) {
  if (groups.length === 0) return <>未关联</>
  return <>{groups.map((group, index) => <span key={group.external_id}>{index ? " · " : ""}{group.name} <strong className="font-mono text-sm font-semibold tabular-nums">{multiplier(group.rate_multiplier)}</strong></span>)}</>
}

function uniqueGroups(groups: RelayGroupOption[]) {
  const seen = new Set<number>()
  const result: RelayGroupOption[] = []
  for (const group of groups) {
    if (seen.has(group.external_id)) continue
    seen.add(group.external_id)
    result.push(group)
  }
  return result
}

function suggestedGroups(account: RelayAccountView, groupsByID: Map<number, RelayGroupOption>) {
  const fallbacks = new Map<number, RelayGroupOption>()
  for (const group of [...account.current_groups, ...(account.recommended_group ? [account.recommended_group] : []), ...accountDowngradeGroups(account)]) {
    fallbacks.set(group.external_id, group)
  }
  const groups = (account.suggested_group_ids ?? []).map((id) => groupsByID.get(id) ?? fallbacks.get(id)).filter(Boolean) as RelayGroupOption[]
  if (groups.length === 0 && account.recommended_group) return [account.recommended_group]
  return uniqueGroups(groups)
}

function adjustmentCandidateGroups(mode: AccountAdjustmentMode, account: RelayAccountView, groupsByID: Map<number, RelayGroupOption>) {
  return mode === "downgrade" ? accountDowngradeGroups(account) : suggestedGroups(account, groupsByID)
}

function groupIDsFromOptions(groups: RelayGroupOption[]) {
  return groups.map((group) => group.external_id)
}

function selectedCandidateGroups(groups: RelayGroupOption[], selectedIDs: number[]) {
  const selected = new Set(selectedIDs)
  return groups.filter((group) => selected.has(group.external_id))
}

function defaultAccountAdjustmentSelections(mode: AccountAdjustmentMode, accounts: RelayAccountView[], groupsByID: Map<number, RelayGroupOption>) {
  return Object.fromEntries(accounts.map((account) => [account.external_id, groupIDsFromOptions(adjustmentCandidateGroups(mode, account, groupsByID))])) as AccountAdjustmentSelection
}

function selectedAdjustmentAccountCount(accounts: RelayAccountView[], selections: AccountAdjustmentSelection) {
  return accounts.filter((account) => (selections[account.external_id] ?? []).length > 0).length
}

function downgradeAdjustedGroups(account: RelayAccountView, additions = accountDowngradeGroups(account)) {
  return uniqueGroups([...account.current_groups, ...additions])
}

function GroupPreviewPills({ groups, tone = "neutral" }: { groups: RelayGroupOption[]; tone?: "neutral" | "target" | "addition" }) {
  if (groups.length === 0) return <span className="text-[11px] text-muted-foreground">未关联</span>
  return <span className="flex min-w-0 flex-wrap gap-1.5">{groups.map((group) => <span key={group.external_id} className={cn("max-w-full truncate rounded border px-1.5 py-0.5 text-[11px] font-medium", tone === "target" ? "border-success/25 bg-success/10 text-success" : tone === "addition" ? "border-warning/25 bg-warning/10 text-warning" : "border-border bg-background text-foreground/75")} title={`${group.name} · 倍率 ${multiplier(group.rate_multiplier)}`}>{group.name} <span className="font-mono tabular-nums">{multiplier(group.rate_multiplier)}</span></span>)}</span>
}

function GroupCandidatePicker({ mode, account, groups, selectedIDs, onChange }: { mode: AccountAdjustmentMode; account: RelayAccountView; groups: RelayGroupOption[]; selectedIDs: number[]; onChange: (accountID: number, groupIDs: number[]) => void }) {
  const selected = new Set(selectedIDs)
  const allIDs = groupIDsFromOptions(groups)
  return <div className="mt-3 min-w-0 rounded-md border border-border bg-muted/20 p-2.5">
    <div className="mb-2 flex items-center justify-between gap-2">
      <p className="text-[11px] font-medium text-foreground">{mode === "downgrade" ? "可选新增降价分组" : "可选调整分组"}</p>
      {groups.length > 1 ? <div className="flex shrink-0 items-center gap-1"><button type="button" className="rounded px-1.5 py-0.5 text-[10px] font-medium text-brand transition-colors hover:bg-brand/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" onClick={() => onChange(account.external_id, allIDs)}>全选</button><button type="button" className="rounded px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" onClick={() => onChange(account.external_id, [])}>清空</button></div> : null}
    </div>
    <div className="grid gap-1.5 sm:grid-cols-2">
      {groups.map((group) => {
        const checked = selected.has(group.external_id)
        const id = `adjustment-${mode}-${account.external_id}-${group.external_id}`
        const nextIDs = allIDs.filter((value) => value === group.external_id ? !checked : selected.has(value))
        return <label key={group.external_id} htmlFor={id} className={cn("flex min-w-0 cursor-pointer items-center gap-2 rounded border px-2 py-1.5 text-[11px] transition-colors", checked ? mode === "downgrade" ? "border-warning/30 bg-warning/10 text-warning" : "border-success/30 bg-success/10 text-success" : "border-border bg-background text-muted-foreground hover:text-foreground")}>
          <Checkbox id={id} checked={checked} onCheckedChange={() => onChange(account.external_id, nextIDs)} />
          <span className="min-w-0 flex-1 truncate" title={group.name}>{group.name}</span>
          <span className="shrink-0 font-mono font-semibold tabular-nums">{multiplier(group.rate_multiplier)}</span>
        </label>
      })}
    </div>
  </div>
}

function AccountAdjustmentPreview({ mode, accounts, totalCount, groupsByID, selectedGroupIDs, onSelectionChange }: { mode: AccountAdjustmentMode; accounts: RelayAccountView[]; totalCount: number; groupsByID: Map<number, RelayGroupOption>; selectedGroupIDs?: AccountAdjustmentSelection; onSelectionChange?: (accountID: number, groupIDs: number[]) => void }) {
  const rows = accounts.map((account) => {
    const candidateGroups = adjustmentCandidateGroups(mode, account, groupsByID)
    const selectedIDs = selectedGroupIDs?.[account.external_id] ?? groupIDsFromOptions(candidateGroups)
    const selectedGroups = selectedCandidateGroups(candidateGroups, selectedIDs)
    const additions = mode === "downgrade" ? selectedGroups : []
    const target = mode === "downgrade" ? downgradeAdjustedGroups(account, additions) : selectedGroups
    return { account, additions, candidateGroups, selectedGroups, selectedIDs, target }
  })
  const selectedAccountCount = rows.filter((row) => row.selectedGroups.length > 0).length
  const skipped = Math.max(totalCount - selectedAccountCount, 0)
  return <div className="space-y-3 text-left text-sm">
    <p className="leading-6 text-foreground">{mode === "downgrade" ? "将为可降价账号追加勾选的安全降价分组，原有分组会保留；未勾选任何分组的账号会跳过。" : "将把可应用账号同步为勾选的建议销售分组；未勾选任何分组的账号会跳过。"}</p>
    <div className="grid grid-cols-3 gap-2 text-center"><div className="rounded-md border border-border bg-muted/40 px-2 py-2"><p className="text-[11px] text-muted-foreground">当前筛选</p><p className="mt-0.5 font-mono text-base font-semibold tabular-nums text-foreground">{totalCount}</p></div><div className="rounded-md border border-success/25 bg-success/10 px-2 py-2"><p className="text-[11px] text-success">{mode === "downgrade" ? "将降价" : "将调组"}</p><p className="mt-0.5 font-mono text-base font-semibold tabular-nums text-success">{selectedAccountCount}</p></div><div className="rounded-md border border-border bg-muted/40 px-2 py-2"><p className="text-[11px] text-muted-foreground">预计跳过</p><p className="mt-0.5 font-mono text-base font-semibold tabular-nums text-foreground">{skipped}</p></div></div>
    <div className="max-h-[min(58vh,520px)] space-y-2 overflow-y-auto pr-1">{rows.map(({ account, additions, candidateGroups, selectedGroups, selectedIDs, target }) => <div key={account.external_id} className="rounded-md border border-border bg-card p-3"><div className="flex flex-wrap items-center justify-between gap-2"><div className="min-w-0"><p className="truncate text-xs font-semibold text-foreground" title={account.name}>{account.name}</p><p className="mt-0.5 text-[11px] text-muted-foreground">#{account.external_id} · {account.platform || "-"} · {account.model_type || "未绑定模型类型"}</p></div><div className="flex shrink-0 items-center gap-1.5"><span className={cn("rounded px-2 py-1 text-[11px] font-medium", selectedGroups.length > 0 ? "bg-success/10 text-success" : "bg-warning/10 text-warning")}>{selectedGroups.length > 0 ? `已选 ${selectedGroups.length}/${candidateGroups.length}` : "将跳过"}</span><span className="rounded bg-muted px-2 py-1 font-mono text-[11px] font-medium tabular-nums text-foreground">成本 {multiplier(account.cost_multiplier)}</span></div></div><div className="mt-3 grid gap-2 sm:grid-cols-2"><div className="min-w-0"><p className="mb-1 text-[11px] font-medium text-muted-foreground">当前分组</p><GroupPreviewPills groups={account.current_groups} /></div><div className="min-w-0"><p className="mb-1 text-[11px] font-medium text-success">调整后分组</p>{selectedGroups.length > 0 ? <GroupPreviewPills groups={target} tone="target" /> : <span className="text-[11px] text-warning">未选择，确认时跳过</span>}</div></div>{mode === "downgrade" ? <div className="mt-2 min-w-0"><p className="mb-1 text-[11px] font-medium text-warning">新增降价分组</p>{selectedGroups.length > 0 ? <GroupPreviewPills groups={additions} tone="addition" /> : <span className="text-[11px] text-warning">未选择新增分组</span>}</div> : account.recommended_group ? <p className="mt-2 text-[11px] text-muted-foreground">推荐基准：<span className="font-medium text-foreground">{account.recommended_group.name}</span> <span className="font-mono tabular-nums">{multiplier(account.recommended_group.rate_multiplier)}</span></p> : null}{onSelectionChange ? <GroupCandidatePicker mode={mode} account={account} groups={candidateGroups} selectedIDs={selectedIDs} onChange={onSelectionChange} /> : null}</div>)}</div>
  </div>
}

function AccountAdjustmentSelectionDialog({ state, selections, busy, groupsByID, onSelectionChange, onCancel, onConfirm }: { state: AccountAdjustmentDialogState; selections: AccountAdjustmentSelection; busy: boolean; groupsByID: Map<number, RelayGroupOption>; onSelectionChange: (accountID: number, groupIDs: number[]) => void; onCancel: () => void; onConfirm: () => void }) {
  return <Dialog open={state != null} onOpenChange={(open) => { if (!open && !busy) onCancel() }}>
    <DialogContent className="w-[calc(100vw-2rem)] max-h-[calc(100dvh-2rem)] overflow-hidden p-0 sm:max-w-3xl">
      {state ? <><DialogHeader className="border-b border-border px-4 py-5 sm:px-6"><DialogTitle>{state.title}</DialogTitle><DialogDescription>默认全选候选分组，可按账号取消不需要应用的分组。</DialogDescription></DialogHeader><div className="px-4 py-4 sm:px-6"><AccountAdjustmentPreview mode={state.mode} accounts={state.accounts} totalCount={state.totalCount} groupsByID={groupsByID} selectedGroupIDs={selections} onSelectionChange={onSelectionChange} /></div><DialogFooter className="border-t border-border px-4 py-4 sm:px-6"><Button type="button" variant="outline" disabled={busy} onClick={onCancel}>取消</Button><Button type="button" className="gap-1.5" disabled={busy} onClick={onConfirm}><Check className="size-3.5" />{state.confirmLabel}</Button></DialogFooter></> : null}
    </DialogContent>
  </Dialog>
}

function sourceLabel(account: RelayAccountView, overview: RelayOverview) {
  if (!account.cost_source) return "未获取"
  if (account.cost_source === "manual") return `手工倍率 ${multiplier(account.cost_multiplier)}`
  if (account.cost_source === "channel_group") {
    const channel = overview.monitor_channels.find((item) => item.id === account.cost_override_channel_id)
    return `渠道 ${channel?.name ?? `#${account.cost_override_channel_id ?? "-"}`} / ${account.cost_override_group ?? "-"}`
  }
  if (account.cost_source === "auto_link") {
    const channel = overview.monitor_channels.find((item) => item.id === account.cost_override_channel_id)
    return `自动关联 ${channel?.name ?? `#${account.cost_override_channel_id ?? "-"}`} / ${account.cost_override_group ?? "-"}`
  }
  return "实时探测"
}

const latencyBarClass: Record<LatencyTone, string> = {
  good: "bg-emerald-500",
  warn: "bg-amber-400",
  bad: "bg-red-500",
}

function requestTypeLabel(value: string) {
  const normalized = value.trim().toLowerCase().replaceAll("_", "-")
  const labels: Record<string, string> = {
    stream: "流式",
    streaming: "流式",
    "non-stream": "非流式",
    nonstream: "非流式",
    sync: "同步",
    async: "异步",
    image: "图像",
    video: "视频",
  }
  return labels[normalized] ?? (normalized ? normalized.replaceAll("-", " ") : "-")
}

function requestTypeClass(value: string) {
  const normalized = value.trim().toLowerCase().replaceAll("_", "-")
  if (normalized === "stream" || normalized === "streaming") return "border-emerald-300 bg-emerald-100 text-emerald-800 dark:border-emerald-500/40 dark:bg-emerald-500/20 dark:text-emerald-300"
  if (normalized === "sync") return "border-blue-300 bg-blue-100 text-blue-800 dark:border-blue-500/40 dark:bg-blue-500/20 dark:text-blue-300"
  if (normalized === "non-stream" || normalized === "nonstream") return "border-amber-300 bg-amber-100 text-amber-800 dark:border-amber-500/40 dark:bg-amber-500/20 dark:text-amber-300"
  if (normalized === "async") return "border-violet-300 bg-violet-100 text-violet-800 dark:border-violet-500/40 dark:bg-violet-500/20 dark:text-violet-300"
  if (normalized === "image") return "border-pink-300 bg-pink-100 text-pink-800 dark:border-pink-500/40 dark:bg-pink-500/20 dark:text-pink-300"
  if (normalized === "video") return "border-orange-300 bg-orange-100 text-orange-800 dark:border-orange-500/40 dark:bg-orange-500/20 dark:text-orange-300"
  return "border-slate-300 bg-slate-100 text-slate-700 dark:border-slate-500/40 dark:bg-slate-500/20 dark:text-slate-300"
}

function money(value: number | null | undefined) {
  return `$${(value ?? 0).toFixed(6)}`
}

type AccountTestOutputView = {
  model: string
  content: string
  completed: boolean | null
  fallback: string
}

function parseAccountTestOutput(output: string): AccountTestOutputView {
  let model = ""
  let content = ""
  let completed: boolean | null = null
  let parsedEvent = false

  for (const line of output.split(/\r?\n/)) {
    const trimmed = line.trim()
    if (!trimmed.startsWith("data:")) continue
    const payload = trimmed.slice(5).trim()
    if (!payload || payload === "[DONE]") continue
    try {
      const event = JSON.parse(payload) as Record<string, unknown>
      parsedEvent = true
      if (typeof event.model === "string") model = event.model
      if (event.type === "content" && typeof event.text === "string") content += event.text
      if (event.type === "test_complete" && typeof event.success === "boolean") completed = event.success
    } catch {
      // Unknown SSE frames are retained by the formatted fallback below.
    }
  }

  let fallback = output.trim()
  if (!parsedEvent && fallback) {
    try {
      fallback = JSON.stringify(JSON.parse(fallback), null, 2)
    } catch {
      // Plain-text upstream responses are already readable as-is.
    }
  }
  return { model, content, completed, fallback }
}

const DebouncedAccountSearchInput = memo(function DebouncedAccountSearchInput({ initialValue, resetVersion, onValueChange }: { initialValue: string; resetVersion: number; onValueChange: (value: string) => void }) {
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

  return <div className="relative min-w-0 flex-1 basis-64 2xl:w-64 2xl:flex-none 2xl:basis-auto"><Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" aria-hidden="true" /><Input value={draft} onChange={(event) => setDraft(event.target.value)} placeholder="搜索账号 / 地址 / 平台 / 模型" aria-label="按账号名称、地址、平台、类型、模型或 ID 搜索" className="h-11 w-full pl-8 text-xs sm:h-9" /></div>
})

function AccountName({ account }: { account: RelayAccountView }) {
  const href = account.base_url?.trim() || ""
  const name = href
    ? <a href={href} target="_blank" rel="noopener noreferrer" className="min-w-0 truncate text-sm font-medium text-foreground underline-offset-2 hover:text-brand hover:underline" title={account.name}>{account.name}</a>
    : <span className="min-w-0 truncate text-sm font-medium text-foreground" title={account.name}>{account.name}</span>
  return <Tooltip delayDuration={250}><TooltipTrigger asChild>{name}</TooltipTrigger><TooltipContent side="top" className="max-w-sm break-all text-xs">{account.name}<span className="mt-1 block text-muted-foreground">{href ? `点击打开 ${href}` : "该账号未配置 Base URL"}</span></TooltipContent></Tooltip>
}

function RelayAccountStatRow({ label, children, className }: { label: string; children: ReactNode; className?: string }) {
  return <div className={cn("flex min-w-0 items-center justify-between gap-3 bg-card px-3 py-2.5", className)}><span className="shrink-0 text-[11px] font-medium text-muted-foreground">{label}</span><div className="min-w-0 truncate text-right text-xs font-semibold text-foreground">{children}</div></div>
}

function RiskSummary({ overview }: { overview: RelayOverview }) {
  const items = [
    { label: "远端账号", value: overview.summary.account_count, icon: Users, tone: "text-foreground", iconTone: "text-foreground", background: "bg-muted" },
    { label: "亏损风险", value: overview.summary.risk_account_count, icon: CircleAlert, tone: "text-danger", iconTone: "text-danger", background: "bg-danger/10" },
    { label: "无盈利候选", value: overview.summary.no_profit_account_count, icon: CircleAlert, tone: "text-yellow-600 dark:text-yellow-400", iconTone: "text-yellow-600 dark:text-yellow-400", background: "bg-yellow-400/10" },
    { label: "无安全候选", value: overview.summary.no_safe_candidate_count, icon: CircleAlert, tone: "text-yellow-600 dark:text-yellow-400", iconTone: "text-yellow-600 dark:text-yellow-400", background: "bg-muted" },
    { label: "成本未知", value: overview.summary.unknown_cost_count, icon: SlidersHorizontal, tone: "text-yellow-600 dark:text-yellow-400", iconTone: "text-yellow-600 dark:text-yellow-400", background: "bg-muted" },
    { label: "自动处理", value: overview.summary.auto_adjusted_count, icon: ShieldCheck, tone: "text-brand", iconTone: "text-brand", background: "bg-brand/10" },
  ]
  return <div className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">{items.map(({ label, value, icon: Icon, tone, iconTone, background }) => <Card key={label} className="border border-border p-4 shadow-none"><div className="flex items-start justify-between gap-2"><div><p className="text-xs text-muted-foreground">{label}</p><p className={cn("mt-1 text-xl font-bold tabular-nums", tone)}>{value}</p></div><span className={cn("flex size-8 shrink-0 items-center justify-center rounded-lg", background)}><Icon className={cn("size-4", iconTone)} /></span></div></Card>)}</div>
}

function UserBalanceHistoryDialog({ open, userEmail, userName, data, loading, error, onClose }: { open: boolean; userEmail: string; userName: string; data: { user: { balance?: number; created_at?: string; email?: string; username?: string }; total_recharged: number } | null; loading: boolean; error: string | null; onClose: () => void }) {
  return <Dialog open={open} onOpenChange={(value) => !value && onClose()}><DialogContent className="max-w-md"><DialogHeader><DialogTitle>用户余额</DialogTitle><DialogDescription className="flex flex-wrap items-center gap-2"><span className="min-w-0 truncate">{data?.user.email || userEmail || "-"}</span><span className="inline-flex max-w-full truncate rounded-md border border-brand/20 bg-brand/10 px-2 py-0.5 text-[11px] font-medium text-brand">{data?.user.username || userName || "-"}</span></DialogDescription></DialogHeader>{loading ? <div className="py-10 text-center text-sm text-muted-foreground">正在读取余额...</div> : error ? <p className="py-8 text-center text-sm text-danger">读取失败：{error}</p> : <div className="grid grid-cols-2 gap-3"><div className="rounded-md border border-border bg-muted/30 p-4"><p className="text-xs text-muted-foreground">当前余额</p><p className="mt-2 text-lg font-bold tabular-nums">{money(data?.user.balance)}</p></div><div className="rounded-md border border-border bg-muted/30 p-4"><p className="text-xs text-muted-foreground">累计充值</p><p className="mt-2 text-lg font-bold tabular-nums text-emerald-600 dark:text-emerald-400">{money(data?.total_recharged)}</p></div></div>}</DialogContent></Dialog>
}

function UsageTokenBreakdown({ row }: { row: RelayRecentUsage }) {
  const hasCache = row.cache_read_tokens > 0 || row.cache_creation_tokens > 0
  return <span className="space-y-2 font-mono tabular-nums">
    <span className="flex items-center gap-x-3 whitespace-nowrap">
      <span className="inline-flex items-center gap-1.5"><ArrowDown className="size-3.5 text-emerald-500" /><strong className="font-medium text-foreground">{row.input_tokens.toLocaleString("zh-CN")}</strong><span className="text-[10px] text-muted-foreground">输入</span></span>
      <span className="inline-flex items-center gap-1.5"><ArrowUp className="size-3.5 text-violet-500" /><strong className="font-medium text-foreground">{row.output_tokens.toLocaleString("zh-CN")}</strong><span className="text-[10px] text-muted-foreground">输出</span></span>
    </span>
    {hasCache ? <span className="flex flex-wrap gap-x-3 gap-y-1">
      {row.cache_read_tokens > 0 ? <span className="flex items-center gap-1.5 whitespace-nowrap"><Archive className="size-3.5 text-sky-500" /><strong className="font-medium text-foreground">{row.cache_read_tokens.toLocaleString("zh-CN")}</strong><span className="text-[10px] text-muted-foreground">缓存读</span></span> : null}
      {row.cache_creation_tokens > 0 ? <span className="flex items-center gap-1.5 whitespace-nowrap"><PencilLine className="size-3.5 text-amber-500" /><strong className="font-medium text-foreground">{row.cache_creation_tokens.toLocaleString("zh-CN")}</strong><span className="text-[10px] text-muted-foreground">缓存写</span>{row.cache_creation_1h_tokens > 0 ? <span className="rounded bg-orange-500/15 px-1 py-px text-[9px] font-semibold text-orange-600 dark:text-orange-400">1h</span> : null}{row.cache_ttl_overridden ? <span title="缓存 TTL 已由请求覆盖" className="rounded bg-rose-500/15 px-1 py-px text-[9px] font-semibold text-rose-600 dark:text-rose-400">R</span> : null}</span> : null}
    </span> : null}
  </span>
}

function RecentUsageTable({
  rows,
  loading,
  refreshing,
  error,
  onRefresh,
  range,
  groupNames,
  accountNames,
  accountURLs,
  systemCapacity,
  onUserClick,
}: {
  rows: RelayRecentUsage[];
  loading: boolean;
  refreshing: boolean;
  error: string | null;
  onRefresh: () => void;
  range: RelayUsageRange;
  groupNames: Map<number, string>;
  accountNames: Map<number, string>;
  accountURLs: Map<number, string>;
  systemCapacity?: { current: number; total: number };
  onUserClick: (userID: number, userEmail: string, userName: string) => void;
}) {
  const [open, setOpen] = usePersistedOpen("uh_relay_recent_usage_open");
  const [user, setUser] = useState("");
  const [group, setGroup] = useState("all");
  const [page, setPage] = useState(1);
  const pageSize = 100;
  const groups = useMemo(
    () => [
      ...new Map(
        rows.map((row) => [
          row.group_id,
          {
            id: row.group_id,
            name:
              row.group_name ||
              groupNames.get(row.group_id) ||
              `分组 #${row.group_id}`,
          },
        ]),
      ).values(),
    ],
    [groupNames, rows],
  );
  const filtered = rows.filter((row) => {
    const name = row.user_email || row.user_name || `用户 #${row.user_id}`;
    const keyword = user.trim().toLowerCase();
    return (
      (!keyword ||
        name.toLowerCase().includes(keyword) ||
        row.user_name.toLowerCase().includes(keyword) ||
        String(row.user_id).includes(keyword)) &&
      (group === "all" || group === String(row.group_id))
    );
  });
  const pageCount = Math.max(1, Math.ceil(filtered.length / pageSize));
  const currentPage = Math.min(page, pageCount);
  const pagedRows = filtered.slice((currentPage - 1) * pageSize, currentPage * pageSize);
  useEffect(() => { setPage(1) }, [user, group, range]);
  useEffect(() => { if (page > pageCount) setPage(pageCount) }, [page, pageCount]);
  return (
    <Card className="recent-usage-table gap-0 overflow-hidden border border-border py-0 shadow-none">
      <ListPanelHeader
          secondaryPlacement="inline"
          actionsPlacement="secondary"
          title={<CardTitle className="flex items-center gap-2 text-sm">
              <History className="size-4 text-brand" />
              最近使用记录            </CardTitle>}
          description={open ? (
              <p className="mt-1 truncate text-xs text-muted-foreground">
                按当前时间范围展示中转站使用记录，不再固定截取最近 100 条。              </p>
            ) : null}
          summary={open && systemCapacity ? <div className="flex min-w-0 items-center gap-2">
              <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-brand/10 text-brand">
                <Gauge className="size-4" />
              </span>
              <div>
                <p className="text-[10px] text-muted-foreground">系统并发</p>
                <p className="font-mono text-sm font-semibold tabular-nums text-foreground">
                  {systemCapacity.current} / {systemCapacity.total}
                </p>
              </div>
            </div> : null}
actions={open ? <>
<Button
              variant="outline"
              size="sm"
              className="h-11 sm:h-9 gap-1.5"
              disabled={loading || refreshing}
              onClick={onRefresh}
            >
              <RefreshCw
                className={cn(
                  "size-3.5",
                  (loading || refreshing) && "animate-spin",
                )}
              />
              刷新
            </Button>
</> : null}
          secondary={open ? <>
{error ? (
              <span className="text-xs text-danger">读取失败：{error}</span>
            ) : null}
<div className="relative min-w-0 flex-1 basis-64">
              <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={user}
                onChange={(event) => setUser(event.target.value)}
                placeholder="输入用户名或 ID"
                aria-label="输入用户名筛选"
                className="h-11 sm:h-9 w-full pl-8 text-xs"
              />
            </div>
<ListFilterMenu
              activeCount={Number(group !== "all")}
              onReset={() => { setUser(""); setGroup("all") }}
              groups={[{ label: "使用分组", value: group, onValueChange: setGroup, options: [{ value: "all", label: "All" }, ...groups.map((item) => ({ value: String(item.id), label: item.name }))] }]}
            />
<Button
              variant="outline"
              size="sm"
              className="h-11 sm:h-9 gap-1.5"
              onClick={() => {
                setUser("");
                setGroup("all");
              }}
            >
              <RotateCcw className="size-3.5" />
              重置
            </Button>
</> : null}
          meta={<><div className="flex shrink-0 items-center gap-2">
            {open ? (
              <span className="text-xs text-muted-foreground">
                {filtered.length} / {rows.length} 条              </span>
            ) : null}
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="size-11 sm:size-9"
              aria-label={open ? "收起最近使用记录" : "展开最近使用记录"}
              aria-expanded={open}
              onClick={() => setOpen((value) => !value)}
            >
              <ChevronDown
                className={cn(
                  "size-4 transition-transform duration-200",
                  open && "rotate-180",
                )}
              />
            </Button>
          </div></>}
        ></ListPanelHeader>
      {open ? (
        <CardContent className="border-t border-border p-0">
          <div className="relative isolate h-[520px] max-h-[520px]">
            <div className="h-full overflow-auto">
              <div className="min-w-[1684px]">
                <div className="sticky top-0 z-30 grid grid-cols-[180px_160px_180px_150px_76px_240px_170px_160px_200px_168px] gap-0 border-b border-border bg-muted text-[11px] text-foreground/90 font-medium [&>*]:px-4 [&>*]:py-2">
                  <span>用户邮箱</span>
                  <span>账户</span>
                  <span>模型</span>
                  <span>分组</span>
                  <span>类型</span>
                  <span>Token</span>
                  <span>费用</span>
                  <span>延迟</span>
                  <span>IP</span>
                  <span>时间</span>
                </div>
                {filtered.length ? (
                  pagedRows.map((row, index) => {
                    const groupLabel =
                      row.group_name ||
                      groupNames.get(row.group_id) ||
                      `分组 #${row.group_id}`;
                    const accountLabel =
                      row.account_name ||
                      accountNames.get(row.account_id) ||
                      `账号 #${row.account_id}`;
                    const accountURL = accountURLs.get(row.account_id);
                    const userName = row.user_name || `用户 #${row.user_id}`;
                    const userEmail = row.user_email || userName;
                    const firstTone = latencyTone(row.first_token_ms, "first");
                    const durationTone = latencyTone(
                      row.duration_ms,
                      "duration",
                    );
                    return (
                      <div
                        key={`${row.id}-${index}`}
                        className="grid grid-cols-[180px_160px_180px_150px_76px_240px_170px_160px_200px_168px] items-center gap-0 border-b border-border text-xs last:border-0 [&>*]:px-4 [&>*]:py-2.5 last:[&>*]:border-b-0"
                      >
                        <button
                          type="button"
                          className="flex min-w-0 self-stretch flex-col justify-center text-left text-brand transition-colors hover:text-brand/80"
                          title={`查看 ${userEmail} 的当前余额和累计充值`}
                          onClick={() =>
                            onUserClick(row.user_id, userEmail, userName)
                          }
                        >
                          <span className="block truncate font-medium underline-offset-2 hover:underline">
                            {userEmail}
                          </span>
                          <span className="mt-0.5 block truncate text-[10px] text-muted-foreground">
                            {userName} · #{row.user_id}
                          </span>
                        </button>
                        {accountURL ? (
                          <a
                            href={accountURL}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="truncate font-medium text-brand underline-offset-2 transition-colors hover:text-brand/80 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                            title={`打开 ${accountLabel}：${accountURL}`}
                          >
                            {accountLabel}
                          </a>
                        ) : (
                          <span className="truncate" title={accountLabel}>
                            {accountLabel}
                          </span>
                        )}
                        <span
                          className="truncate font-mono text-[11px]"
                          title={row.model}
                        >
                          {row.model || "-"}
                        </span>
                        <span className="min-w-0">
                          <span
                            className="inline-flex max-w-full truncate rounded-md border border-brand/20 bg-brand/10 px-2 py-1 text-[11px] font-medium text-brand"
                            title={groupLabel}
                          >
                            {groupLabel}
                          </span>
                        </span>
                        <span>
                          <span
                            className={cn(
                              "inline-flex whitespace-nowrap rounded border px-2 py-1 text-[10px] font-medium",
                              requestTypeClass(row.request_type),
                            )}
                          >
                            {requestTypeLabel(row.request_type)}
                          </span>
                        </span>
                        <UsageTokenBreakdown row={row} />
                        <span className="space-y-0.5 font-mono tabular-nums">
                          <span className="block whitespace-nowrap font-medium text-brand">
                            用户扣费 {money(row.user_charge)}
                          </span>
                          <span className="block whitespace-nowrap text-[10px] text-muted-foreground">
                            原始 {money(row.original_cost)}
                          </span>
                        </span>
                        <span className="relative space-y-1 pl-3 font-mono tabular-nums">
                          <i
                            className={cn(
                              "absolute inset-y-0 left-0 w-1 rounded-full",
                              firstTone === durationTone
                                ? latencyBarClass[firstTone]
                                : "bg-gradient-to-b from-40% to-60%",
                              firstTone === durationTone
                                ? ""
                                : firstTone === "good"
                                  ? "from-emerald-500"
                                  : firstTone === "warn"
                                    ? "from-amber-400"
                                    : "from-red-500",
                              firstTone === durationTone
                                ? ""
                                : durationTone === "good"
                                  ? "to-emerald-500"
                                  : durationTone === "warn"
                                    ? "to-amber-400"
                                    : "to-red-500",
                            )}
                          />
                          <span className="grid grid-cols-[42px_1fr] items-center gap-1 whitespace-nowrap">
                            <span className="text-muted-foreground">首字</span>
                            <strong
                              className={cn(
                                "font-medium",
                                latencyTextClass[firstTone],
                              )}
                            >
                              {formatLatency(row.first_token_ms)}
                            </strong>
                          </span>
                          <span className="grid grid-cols-[42px_1fr] items-center gap-1 whitespace-nowrap">
                            <span className="text-muted-foreground">
                              总耗时
                            </span>
                            <strong
                              className={cn(
                                "font-medium",
                                latencyTextClass[durationTone],
                              )}
                            >
                              {formatLatency(row.duration_ms)}
                            </strong>
                          </span>
                        </span>
                        <span className="min-w-0 space-y-0.5" title={row.ip_location || row.ip_address || "未记录 IP"}>
                          <span className="block truncate font-mono text-[11px] text-foreground">
                            {row.ip_address || "-"}
                          </span>
                          <span className="block truncate text-[10px] text-muted-foreground">
                            {row.ip_location || (row.ip_address ? "归属地未知" : "未记录")}
                          </span>
                        </span>
                        <span className="font-mono text-[11px] text-muted-foreground">
                          {new Date(row.created_at).toLocaleString("zh-CN")}
                        </span>
                      </div>
                    );
                  })
                ) : (
                  <p className="py-12 text-center text-sm text-muted-foreground">
                    当前筛选没有使用记录                  </p>
                )}
              </div>
            </div>
            {refreshing && rows.length > 0 ? (
              <div className="pointer-events-none absolute inset-x-0 top-10 z-40 flex justify-center">
                <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
                  <RefreshCw className="size-3.5 animate-spin text-brand" />
                  正在刷新使用记录
                </span>
              </div>
            ) : null}
          </div>
          {filtered.length > pageSize ? <div className="flex flex-wrap items-center justify-between gap-2 border-t border-border px-4 py-2.5 text-xs text-muted-foreground"><span>范围内共 {filtered.length} 条；当前显示第 {(currentPage - 1) * pageSize + 1}-{Math.min(currentPage * pageSize, filtered.length)} 条，仅分页展示，不截断数据。</span><div className="flex items-center gap-2"><Button type="button" variant="outline" size="sm" className="h-8" disabled={currentPage <= 1} onClick={() => setPage((value) => Math.max(1, value - 1))}>上一页</Button><span className="font-mono tabular-nums">{currentPage} / {pageCount}</span><Button type="button" variant="outline" size="sm" className="h-8" disabled={currentPage >= pageCount} onClick={() => setPage((value) => Math.min(pageCount, value + 1))}>下一页</Button></div></div> : null}
        </CardContent>
      ) : null}
    </Card>
  );
}

function RiskRow({
  account,
  overview,
  selected,
  busy,
  scheduling,
  probing,
  allowProbe = relayStationCapabilities(overview.station.type).accountCostProbe,
  testing,
  deleting,
  stationID,
  latencyGranularity,
  stabilityBuckets,
  stabilityLoading,
  consumptionBuckets,
  consumptionLoading,
  channelLinks,
  channelLatencyBucketsByChannel,
  channelRecentSamplesByChannel,
  channelConsumptionBucketsByChannel,
  healthScore,
  visibleColumns,
  gridStyle,
  onToggle,
  onApply,
  onAddDowngrade,
  onEditGroups,
  onSchedulableChange,
  onProbe,
  onTest,
  onDelete,
}: {
  account: RelayAccountView;
  overview: RelayOverview;
  selected: boolean;
  busy: boolean;
  scheduling: boolean;
  probing: boolean;
  allowProbe?: boolean;
  testing: boolean;
  deleting: boolean;
  stationID: number;
  latencyGranularity: LatencyBucketGranularity;
  stabilityBuckets: LatencyBucket[];
  stabilityLoading: boolean;
  consumptionBuckets: RelayAccountConsumptionBucket[];
  consumptionLoading: boolean;
  channelLinks: ChannelCompareLink[];
  channelLatencyBucketsByChannel: Map<number, LatencyBucket[]>;
  channelRecentSamplesByChannel: Map<number, ChannelUsageSample[]>;
  channelConsumptionBucketsByChannel: Map<number, { start: string; end: string; amount: number }[]>;
  healthScore: LatencyHealthScore | null;
  visibleColumns: Set<AccountColumnKey>;
  gridStyle: CSSProperties;
  onToggle: (checked: boolean) => void;
  onApply: () => void;
  onAddDowngrade: () => void;
  onEditGroups: () => void;
  onSchedulableChange: (checked: boolean) => void;
  onProbe: () => void;
  onTest: () => void;
  onDelete: () => void;
}) {
  const state = stateMeta[account.risk_state];
  const marginTone =
    account.margin == null
      ? "text-muted-foreground"
      : account.margin < 0
        ? "text-danger"
        : account.risk_state === "no_profit"
          ? "text-warning"
          : "text-success";
  const downgradeGroups = accountDowngradeGroups(account);
  const downgradeSummary = downgradeGroups
    .map((group) => `${group.name} · ${multiplier(group.rate_multiplier)}`)
    .join("、");
  const scoreTitle = healthScore
    ? `综合评分 ${healthScore.score} · 最近成功率 ${healthScore.recentSuccess ?? "-"}% · 最近延迟分 ${healthScore.recentLatency ?? "-"} · 时间稳定性分 ${healthScore.stability ?? "-"}`
    : "暂无足够调用数据计算综合评分";
  return (
    <div style={gridStyle} className="relay-account-grid grid gap-3 border-b border-border px-4 py-2.5 last:border-0 lg:items-center lg:gap-0 lg:p-0 lg:[&>*]:px-4 lg:[&>*]:py-2.5">
      <div className="fixed-column-shadow-desktop fixed-column-shadow-left lg:sticky lg:left-0 lg:z-20 lg:flex lg:self-stretch lg:items-center lg:bg-card">
        <Checkbox
          checked={selected}
          onCheckedChange={(value) => onToggle(value === true)}
          aria-label={`选择账号 ${account.name}`}
        />
      </div>
      <div className="fixed-column-shadow-desktop fixed-column-shadow-left min-w-0 lg:sticky lg:left-[64px] lg:z-20 lg:flex lg:self-stretch lg:flex-col lg:justify-center lg:bg-card">
        <div className="flex items-center gap-2">
          <AccountName account={account} />
          <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">
            {account.type || "-"}
          </span>
        </div>
        <p className="mt-1 truncate text-[11px] text-muted-foreground">
          #{account.external_id}
        </p>
      </div>
      {visibleColumns.has("score") ? <div className="flex items-center justify-between gap-2 lg:block"><span className="text-[11px] text-muted-foreground lg:hidden">综合评分</span><span className={cn("inline-flex items-center rounded px-2 py-1 font-mono text-xs font-semibold tabular-nums ring-1 ring-inset", healthScore ? healthScoreClass(healthScore.score) : "bg-muted/50 text-muted-foreground ring-border")} title={scoreTitle}>{healthScore?.score ?? "--"}</span></div> : null}
      {visibleColumns.has("platform") ? <div className="flex items-center justify-between gap-2 lg:block"><span className="text-[11px] text-muted-foreground lg:hidden">平台</span><PlatformBadge platform={account.platform} /></div> : null}
      {visibleColumns.has("concurrency") ? <div className="flex items-center justify-between gap-2 lg:block"><span className="text-[11px] text-muted-foreground lg:hidden">并发数</span><span className="font-mono text-sm font-semibold tabular-nums text-foreground">{account.concurrency}</span></div> : null}
      {visibleColumns.has("capacity") ? <div className="flex items-center justify-between gap-2 lg:block"><span className="text-[11px] text-muted-foreground lg:hidden">容量</span><span className={cn("inline-flex items-center gap-1 rounded-md border px-1.5 py-1 font-mono text-[11px] font-semibold tabular-nums", capacityTone(account))} title="实时并发占用 / 并发上限"><Gauge className="size-3" />{account.current_concurrency} / {account.concurrency}</span></div> : null}
      {visibleColumns.has("priority") ? <div className="flex items-center justify-between gap-2 lg:block"><span className="text-[11px] text-muted-foreground lg:hidden">优先级</span><span className="font-mono text-sm font-semibold tabular-nums text-foreground">{account.priority}</span></div> : null}
      {visibleColumns.has("retry") ? <div className="flex items-center justify-between gap-2 lg:block"><span className="text-[11px] text-muted-foreground lg:hidden">重试次数</span><span className={cn("font-mono text-sm font-semibold tabular-nums", account.pool_mode ? "text-foreground" : "text-muted-foreground")} title={account.pool_mode ? "同账号重试次数" : "池模式未开启，当前重试次数不生效"}>{account.pool_mode_retry_count}</span></div> : null}
      {visibleColumns.has("schedulable") ? <div className="flex min-h-11 items-center gap-2 lg:min-h-0 lg:flex-col lg:items-start lg:gap-1"><Switch checked={account.schedulable} onCheckedChange={onSchedulableChange} disabled={busy || scheduling} aria-label={`${account.name}${account.schedulable ? "关闭" : "开启"}调度`} /><span className={cn("text-[11px]", account.schedulable ? "text-success" : "text-muted-foreground")}>{account.schedulable ? "已开启" : "已关闭"}</span></div> : null}
      {visibleColumns.has("model") ? <div className="min-w-0"><p className="mb-1 text-[11px] uppercase tracking-wide text-muted-foreground lg:hidden">模型类型</p><span className={cn("inline-flex max-w-full truncate rounded-md border px-2 py-1 text-[11px] font-medium", account.model_type ? "border-brand/20 bg-brand/10 text-brand" : "border-border bg-muted text-muted-foreground")} title={account.model_type || "未绑定，不参与自动调组"}>{account.model_type || "未绑定"}</span></div> : null}
      {visibleColumns.has("recent") ? <div className="min-w-0"><p className="mb-1 text-[11px] uppercase tracking-wide text-muted-foreground lg:hidden">最近调用</p><LatencyStabilityLane buckets={stabilityBuckets} recentSamples={latestRelayLatencySamples(account.latency_samples ?? [], 30)} loading={stabilityLoading} layout="fill" compact channelLinks={channelLinks} channelLatencyBucketsByChannel={channelLatencyBucketsByChannel} channelRecentSamplesByChannel={channelRecentSamplesByChannel} relayAccountName={account.name} loadSamples={(bucket) => apiFetch<RelayLatencySample[]>("/relay-stations/" + stationID + "/accounts/" + account.external_id + "/latency-samples?start=" + encodeURIComponent(bucket.start) + "&end=" + encodeURIComponent(bucket.end) + "&limit=30")} /></div> : null}
      {visibleColumns.has("stability") ? <div className="min-w-0"><LatencyStabilityLane buckets={stabilityBuckets} loading={stabilityLoading} layout="fill" compact granularity={latencyGranularity} channelLinks={channelLinks} channelLatencyBucketsByChannel={channelLatencyBucketsByChannel} relayAccountName={account.name} loadSamples={(bucket) => apiFetch<RelayLatencySample[]>("/relay-stations/" + stationID + "/accounts/" + account.external_id + "/latency-samples?start=" + encodeURIComponent(bucket.start) + "&end=" + encodeURIComponent(bucket.end) + "&limit=30")} /></div> : null}
      {visibleColumns.has("usage") ? <div className="min-w-0"><p className="text-[11px] uppercase tracking-wide text-muted-foreground lg:hidden">区间消费</p><p className="mt-1 font-mono text-sm font-semibold leading-5 tabular-nums text-foreground lg:mt-0 lg:leading-4">{tokenAmount(account.usage_total_tokens)} Token</p><p className="mt-0.5 text-[11px] font-medium leading-4 tabular-nums text-brand lg:mt-0">用户扣费 {chargeAmount(account.user_charge_amount)}</p><p className="mt-0.5 text-[11px] font-medium leading-4 tabular-nums text-warning lg:mt-0">成本 {chargeAmount(account.account_cost_amount)}</p></div> : null}
      {visibleColumns.has("consumption_trend") ? <div className="min-w-0"><p className="mb-1 text-[11px] uppercase tracking-wide text-muted-foreground lg:hidden">消费趋势</p><RelayAccountConsumptionTrend buckets={consumptionBuckets} loading={consumptionLoading} compact channelLinks={channelLinks} channelConsumptionBucketsByChannel={channelConsumptionBucketsByChannel} relayAccountName={account.name} /></div> : null}
      {visibleColumns.has("cost") ? <div><p className="font-mono text-base font-semibold tabular-nums text-foreground">{multiplier(account.cost_multiplier)}</p><p className="mt-1 max-w-[150px] truncate text-[10px] text-muted-foreground" title={sourceLabel(account, overview)}>{sourceLabel(account, overview)}</p></div> : null}
      {visibleColumns.has("groups") ? <div className="min-w-0"><p className="text-[11px] uppercase tracking-wide text-muted-foreground lg:hidden">当前销售分组</p><p className="mt-1 line-clamp-2 text-xs text-foreground"><GroupNames groups={account.current_groups} /></p></div> : null}
      {visibleColumns.has("risk") ? <div><span className={cn("inline-flex rounded px-2 py-1 text-[11px] font-medium", state.tone)}>{state.label}</span><p className={cn("mt-1 font-mono text-sm font-semibold tabular-nums", marginTone)}>空间 {account.margin == null ? "-" : multiplier(account.margin)}</p></div> : null}
      {visibleColumns.has("profit") ? <div><p className="text-[11px] uppercase tracking-wide text-muted-foreground lg:hidden">利润</p><p className={cn("font-mono text-sm font-semibold tabular-nums", marginTone)} title="利润率 = (销售倍率 - 成本倍率) / 成本倍率">{profitPercent(account.margin, account.cost_multiplier)}</p></div> : null}
      {visibleColumns.has("downgrade") ? <div className="min-w-0"><span className="mr-2 text-[11px] text-muted-foreground lg:hidden">可降级</span><span className={cn("inline-flex rounded px-2 py-1 text-[11px] font-medium", downgradeGroups.length ? "bg-success/10 text-success" : "bg-muted text-muted-foreground")}>{downgradeGroups.length ? `${downgradeGroups.length} 个` : "否"}</span>{downgradeGroups.length ? <p className="mt-1 line-clamp-2 text-[11px] text-muted-foreground" title={downgradeSummary}>{downgradeSummary}</p> : null}</div> : null}
      <div className="fixed-column-shadow-desktop fixed-column-shadow-right flex min-w-0 flex-nowrap justify-start gap-1.5 lg:sticky lg:right-0 lg:z-30 lg:self-stretch lg:items-center lg:justify-end lg:bg-card">
        <Tooltip delayDuration={250}>
          <TooltipTrigger asChild>
            <TableActionButton
              type="button"
              className="shrink-0 text-brand"
              aria-label="测试连接"
              disabled={busy || testing || deleting}
              onClick={onTest}
            >
              <Play className={cn("size-3.5", testing && "animate-pulse")} />
            </TableActionButton>
          </TooltipTrigger>
          <TooltipContent side="top">测试连接</TooltipContent>
        </Tooltip>
        {allowProbe && account.type?.toLowerCase() === "apikey" ? (
          <Tooltip delayDuration={250}>
            <TooltipTrigger asChild>
              <TableActionButton
                type="button"
                className="shrink-0"
                aria-label="探测倍率"
                disabled={busy || probing || deleting}
                onClick={onProbe}
              >
                <RefreshCw
                  className={cn("size-3.5", probing && "animate-spin")}
                />
              </TableActionButton>
            </TooltipTrigger>
            <TooltipContent side="top">探测倍率</TooltipContent>
          </Tooltip>
        ) : null}
        <Tooltip delayDuration={250}>
          <TooltipTrigger asChild>
            <TableActionButton
              type="button"
              className="shrink-0"
              aria-label="调整分组"
              disabled={busy || probing || deleting}
              onClick={onEditGroups}
            >
              <SlidersHorizontal className="size-3.5" />
            </TableActionButton>
          </TooltipTrigger>
          <TooltipContent side="top">调整分组</TooltipContent>
        </Tooltip>
        {downgradeGroups.length ? (
          <Tooltip delayDuration={250}>
            <TooltipTrigger asChild>
              <TableActionButton
                type="button"
                className="shrink-0 border-success/30 bg-success/10 text-success hover:bg-success/20 hover:text-success"
                aria-label={`加入全部 ${downgradeGroups.length} 个可降级分组`}
                disabled={busy || probing || deleting}
                onClick={onAddDowngrade}
              >
                <Plus className="size-3.5" />
              </TableActionButton>
            </TooltipTrigger>
            <TooltipContent side="top">探测倍率</TooltipContent>
          </Tooltip>
        ) : null}
        {account.can_apply ? (
          <Tooltip delayDuration={250}>
            <TooltipTrigger asChild>
              <TableActionButton
                type="button"
                className="shrink-0 border-brand/30 bg-brand/10 text-brand hover:bg-brand/20 hover:text-brand"
                aria-label="应用建议"
                disabled={busy || probing || deleting}
                onClick={onApply}
              >
                <Check className="size-3.5" />
              </TableActionButton>
            </TooltipTrigger>
            <TooltipContent side="top">应用建议</TooltipContent>
          </Tooltip>
        ) : null}
        <Tooltip delayDuration={250}>
          <TooltipTrigger asChild>
            <TableActionButton destructive
              type="button"
              className="shrink-0"
              aria-label={`删除账号${account.name}`}
              disabled={busy || deleting}
              onClick={onDelete}
            >
              <Trash2 className={cn("size-3.5", deleting && "animate-pulse")} />
            </TableActionButton>
          </TooltipTrigger>
          <TooltipContent side="top">删除账号</TooltipContent>
        </Tooltip>
      </div>
    </div>
  );
}

export function RelayAccountGalleryCard({
  account,
  overview,
  selected,
  busy,
  scheduling,
  probing,
  allowProbe = relayStationCapabilities(overview.station.type).accountCostProbe,
  testing,
  deleting,
  stationID,
  latencyGranularity,
  stabilityBuckets,
  stabilityLoading,
  consumptionBuckets,
  consumptionLoading,
  channelLinks,
  channelLatencyBucketsByChannel,
  channelRecentSamplesByChannel,
  channelConsumptionBucketsByChannel,
  healthScore,
  onToggle,
  onApply,
  onAddDowngrade,
  onEditGroups,
  onSchedulableChange,
  onProbe,
  onTest,
  onDelete,
}: {
  account: RelayAccountView;
  overview: RelayOverview;
  selected: boolean;
  busy: boolean;
  scheduling: boolean;
  probing: boolean;
  allowProbe?: boolean;
  testing: boolean;
  deleting: boolean;
  stationID: number;
  latencyGranularity: LatencyBucketGranularity;
  stabilityBuckets: LatencyBucket[];
  stabilityLoading: boolean;
  consumptionBuckets: RelayAccountConsumptionBucket[];
  consumptionLoading: boolean;
  channelLinks: ChannelCompareLink[];
  channelLatencyBucketsByChannel: Map<number, LatencyBucket[]>;
  channelRecentSamplesByChannel: Map<number, ChannelUsageSample[]>;
  channelConsumptionBucketsByChannel: Map<number, { start: string; end: string; amount: number }[]>;
  healthScore: LatencyHealthScore | null;
  onToggle: (checked: boolean) => void;
  onApply: () => void;
  onAddDowngrade: () => void;
  onEditGroups: () => void;
  onSchedulableChange: (checked: boolean) => void;
  onProbe: () => void;
  onTest: () => void;
  onDelete: () => void;
}) {
  const state = stateMeta[account.risk_state]
  const downgradeGroups = accountDowngradeGroups(account)
  const recentCount = latestRelayLatencySamples(account.latency_samples ?? [], 60).length
  const marginTone = account.margin == null ? "text-muted-foreground" : account.margin < 0 ? "text-danger" : account.risk_state === "no_profit" ? "text-warning" : "text-success"
  const scoreTitle = healthScore
    ? `综合评分 ${healthScore.score} · 最近成功率 ${healthScore.recentSuccess ?? "-"}% · 最近延迟分 ${healthScore.recentLatency ?? "-"} · 时间稳定性分 ${healthScore.stability ?? "-"}`
    : "暂无足够调用数据计算综合评分"

  return <Card className="@container flex h-full flex-col gap-0 border border-border p-4 shadow-none">
    <div className="flex min-w-0 items-center gap-2">
      <Checkbox checked={selected} onCheckedChange={(value) => onToggle(value === true)} aria-label={`选择账号 ${account.name}`} />
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 flex-wrap items-center gap-2"><AccountName account={account} /><span className="rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">{account.type || "-"}</span></div>
      </div>
      <span className={cn("inline-flex shrink-0 items-center rounded px-2 py-1 font-mono text-xs font-semibold tabular-nums ring-1 ring-inset", healthScore ? healthScoreClass(healthScore.score) : "bg-muted/50 text-muted-foreground ring-border")} title={scoreTitle}>评分 {healthScore?.score ?? "--"}</span>
    </div>

    <div className="mt-2 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1.5 border-b border-border pb-2.5">
      <p className="min-w-0 flex-1 truncate text-[11px] text-muted-foreground">#{account.external_id} · {account.base_url ? accountURLHost(account.base_url) : "未配置地址"}</p>
      <div className="flex min-w-0 flex-wrap items-center gap-1.5">
        <PlatformBadge platform={account.platform} compact />
        <span className={cn("inline-flex rounded px-1.5 py-0.5 text-[10px] font-medium", state.tone)}>{state.label}</span>
        <span className={cn("inline-flex rounded px-1.5 py-0.5 text-[10px] font-medium", account.schedulable ? "bg-success/10 text-success" : "bg-muted text-muted-foreground")}>{account.schedulable ? "调度开启" : "调度关闭"}</span>
        {account.model_type ? <span className="inline-flex max-w-full truncate rounded border border-brand/20 bg-brand/10 px-1.5 py-0.5 text-[10px] font-medium text-brand" title={account.model_type}>{account.model_type}</span> : null}
      </div>
    </div>

    <div className="mt-3 grid grid-cols-2 gap-px overflow-hidden rounded-md border border-border bg-border text-xs">
      {relayStationCapabilities(overview.station.type).accountConcurrency ? <RelayAccountStatRow label="容量"><span className={cn("inline-flex items-center gap-1 rounded border px-1.5 py-0.5 font-mono tabular-nums", capacityTone(account))}><Gauge className="size-3" />{account.current_concurrency} / {account.concurrency}</span></RelayAccountStatRow> : null}
      <RelayAccountStatRow label={overview.station.type === "newapi" ? "优先级" : "优先级 / 重试"}><span className="font-mono tabular-nums">{account.priority}{overview.station.type !== "newapi" ? ` / ${account.pool_mode_retry_count}` : ""}</span></RelayAccountStatRow>
      <RelayAccountStatRow label="成本倍率"><span className="flex min-w-0 items-center justify-end gap-1.5"><span className="shrink-0 font-mono tabular-nums">{multiplier(account.cost_multiplier)}</span><span className="truncate text-[9px] font-normal text-muted-foreground" title={sourceLabel(account, overview)}>{sourceLabel(account, overview)}</span></span></RelayAccountStatRow>
      <RelayAccountStatRow label="区间 Token"><span className="font-mono tabular-nums">{tokenAmount(account.usage_total_tokens)}</span></RelayAccountStatRow>
      <RelayAccountStatRow label="用户消费"><span className="font-mono tabular-nums text-success">{chargeAmount(account.user_charge_amount)}</span></RelayAccountStatRow>
      <RelayAccountStatRow label="利润率"><span className={cn("font-mono tabular-nums", marginTone)}>{profitPercent(account.margin, account.cost_multiplier)}</span></RelayAccountStatRow>
      <RelayAccountStatRow label="当前销售分组" className="col-span-full"><span className="block truncate text-xs" title={account.current_groups.map((group) => `${group.name} · ${multiplier(group.rate_multiplier)}`).join(" · ")}><GroupNames groups={account.current_groups} /></span></RelayAccountStatRow>
    </div>

    <div className="mt-3 border-t border-border pt-3">
      <div className="mb-1.5 flex items-center justify-between gap-2"><span className="text-[11px] font-medium text-muted-foreground">最近调用延迟</span><span className="font-mono text-[10px] tabular-nums text-muted-foreground">最近 {recentCount} / 60 条</span></div>
      <LatencyStabilityLane buckets={stabilityBuckets} recentSamples={latestRelayLatencySamples(account.latency_samples ?? [], 60)} loading={stabilityLoading} layout="fill" compact granularity={latencyGranularity} channelLinks={channelLinks} channelLatencyBucketsByChannel={channelLatencyBucketsByChannel} channelRecentSamplesByChannel={channelRecentSamplesByChannel} relayAccountName={account.name} emptyLabel="当前范围暂无调用数据" loadSamples={(bucket) => apiFetch<RelayLatencySample[]>("/relay-stations/" + stationID + "/accounts/" + account.external_id + "/latency-samples?start=" + encodeURIComponent(bucket.start) + "&end=" + encodeURIComponent(bucket.end) + "&limit=30")} />
    </div>

    <LatencyStabilityLane buckets={stabilityBuckets} loading={stabilityLoading} layout="fill" granularity={latencyGranularity} channelLinks={channelLinks} channelLatencyBucketsByChannel={channelLatencyBucketsByChannel} relayAccountName={account.name} emptyLabel="当前范围暂无调用数据" loadSamples={(bucket) => apiFetch<RelayLatencySample[]>("/relay-stations/" + stationID + "/accounts/" + account.external_id + "/latency-samples?start=" + encodeURIComponent(bucket.start) + "&end=" + encodeURIComponent(bucket.end) + "&limit=30")} />

    <div className="mt-3 border-t border-border pt-3">
      <div className="mb-1.5 flex items-center justify-between gap-2"><span className="text-[11px] font-medium text-muted-foreground">消费趋势</span><span className="text-[10px] text-muted-foreground">60 段</span></div>
      <RelayAccountConsumptionTrend buckets={consumptionBuckets} loading={consumptionLoading} channelLinks={channelLinks} channelConsumptionBucketsByChannel={channelConsumptionBucketsByChannel} relayAccountName={account.name} />
    </div>

    <div className="mt-auto pt-3">
      <div className="flex items-center justify-between gap-2 border-t border-border pt-3">
        <div className="flex items-center gap-2"><Switch checked={account.schedulable} onCheckedChange={onSchedulableChange} disabled={busy || scheduling} aria-label={`${account.name}${account.schedulable ? "关闭" : "开启"}调度`} /><span className="text-[10px] text-muted-foreground">调度</span></div>
        <div className="flex flex-wrap items-center justify-end gap-1.5">
          <Tooltip delayDuration={250}><TooltipTrigger asChild><Button type="button" variant="outline" size="icon" className="size-8 text-brand" aria-label="测试连接" disabled={busy || testing || deleting} onClick={onTest}><Play className={cn("size-3.5", testing && "animate-pulse")} /></Button></TooltipTrigger><TooltipContent side="top">测试连接</TooltipContent></Tooltip>
          {allowProbe && account.type?.toLowerCase() === "apikey" ? <Tooltip delayDuration={250}><TooltipTrigger asChild><Button type="button" variant="outline" size="icon" className="size-8" aria-label="探测倍率" disabled={busy || probing || deleting} onClick={onProbe}><RefreshCw className={cn("size-3.5", probing && "animate-spin")} /></Button></TooltipTrigger><TooltipContent side="top">探测倍率</TooltipContent></Tooltip> : null}
          <Tooltip delayDuration={250}><TooltipTrigger asChild><Button type="button" variant="outline" size="icon" className="size-8" aria-label="调整分组" disabled={busy || probing || deleting} onClick={onEditGroups}><SlidersHorizontal className="size-3.5" /></Button></TooltipTrigger><TooltipContent side="top">调整分组</TooltipContent></Tooltip>
          {downgradeGroups.length ? <Tooltip delayDuration={250}><TooltipTrigger asChild><Button type="button" variant="outline" size="icon" className="size-8 border-success/30 bg-success/10 text-success hover:bg-success/20 hover:text-success" aria-label={`加入全部 ${downgradeGroups.length} 个可降级分组`} disabled={busy || probing || deleting} onClick={onAddDowngrade}><Plus className="size-3.5" /></Button></TooltipTrigger><TooltipContent side="top">加入可降级分组</TooltipContent></Tooltip> : null}
          {account.can_apply ? <Tooltip delayDuration={250}><TooltipTrigger asChild><Button type="button" variant="outline" size="icon" className="size-8 border-brand/30 bg-brand/10 text-brand hover:bg-brand/20 hover:text-brand" aria-label="应用建议" disabled={busy || probing || deleting} onClick={onApply}><Check className="size-3.5" /></Button></TooltipTrigger><TooltipContent side="top">应用建议</TooltipContent></Tooltip> : null}
          <Tooltip delayDuration={250}><TooltipTrigger asChild><Button type="button" variant="outline" size="icon" className="size-8 border-danger/30 text-danger hover:bg-danger/10 hover:text-danger" aria-label={`删除账号${account.name}`} disabled={busy || deleting} onClick={onDelete}><Trash2 className={cn("size-3.5", deleting && "animate-pulse")} /></Button></TooltipTrigger><TooltipContent side="top">删除账号</TooltipContent></Tooltip>
        </div>
      </div>
    </div>
  </Card>
}

type RelayGroupPatch = Partial<Pick<RelayGroupView, "name" | "description" | "rate_multiplier" | "is_exclusive" | "status" | "model_types" | "monitor_enabled">>

export function buildRelayGroupPatch(type: RelayStationType, group: RelayGroupView, values: Pick<RelayGroupView, "name" | "description" | "rate_multiplier" | "model_types">): RelayGroupPatch {
  if (type !== "newapi") return values
  return { description: values.description, model_types: values.model_types, ...(values.rate_multiplier !== group.rate_multiplier ? { rate_multiplier: values.rate_multiplier } : {}) }
}

export function buildRelayGroupUpdates(type: RelayStationType, group: RelayGroupView, values: Pick<RelayGroupView, "name" | "description" | "rate_multiplier" | "model_types">): RelayGroupPatch[] {
  if (type !== "newapi") return [values]
  const updates: RelayGroupPatch[] = []
  const local: RelayGroupPatch = {}
  if (values.description !== (group.description ?? "")) local.description = values.description
  if (JSON.stringify(values.model_types) !== JSON.stringify(group.model_types ?? [])) local.model_types = values.model_types
  if (Object.keys(local).length > 0) updates.push(local)
  if (Number.isFinite(values.rate_multiplier) && values.rate_multiplier >= 0 && values.rate_multiplier !== group.rate_multiplier) updates.push({ rate_multiplier: values.rate_multiplier })
  return updates
}

export function GroupManagementRow({ stationType = "sub2api", group, accounts, onUpdate, onTest, onDelete, testing, deleting, dragDisabled, dragging, onDragStart, onDragOver, onDrop }: { stationType?: RelayStationType; group: RelayGroupView; accounts: RelayAccountView[]; onUpdate: (groupID: number, patch: RelayGroupPatch) => Promise<void>; onTest: (group: RelayGroupView) => void; onDelete: (group: RelayGroupView) => void; testing: boolean; deleting: boolean; dragDisabled: boolean; dragging: boolean; onDragStart: (id: number) => void; onDragOver: (id: number) => void; onDrop: (id: number) => void }) {
  const [expanded, setExpanded] = useState(false)
  const [name, setName] = useState(group.name)
  const [description, setDescription] = useState(group.description ?? "")
  const [rate, setRate] = useState(String(group.rate_multiplier))
  const [modelTypes, setModelTypes] = useState((group.model_types ?? []).join(", "))
  const [busy, setBusy] = useState(false)
  const [dirty, setDirty] = useState(false)
  const active = group.status?.toLowerCase() === "active"
  const parsedRate = Number(rate)
  const parsedTypes = Array.from(new Set(modelTypes.split(",").map((value) => value.trim().toLowerCase()).filter(Boolean)))
  const changed = name.trim() !== group.name || description.trim() !== (group.description ?? "") || (Number.isFinite(parsedRate) && parsedRate !== group.rate_multiplier) || JSON.stringify(parsedTypes) !== JSON.stringify(group.model_types ?? [])
  useEffect(() => {
    if (dirty) return
    setName(group.name)
    setDescription(group.description ?? "")
    setRate(String(group.rate_multiplier))
    setModelTypes((group.model_types ?? []).join(", "))
  }, [dirty, group.name, group.description, group.rate_multiplier, group.model_types])
  useEffect(() => { if (dirty && !changed) setDirty(false) }, [changed, dirty])

  async function update(patch: RelayGroupPatch) {
    setBusy(true)
    try {
      await onUpdate(group.external_id, patch)
      return true
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "分组更新失败")
      return false
    } finally {
      setBusy(false)
    }
  }

  async function save() {
    if (stationType !== "newapi" && !name.trim()) { toast.error("分组名称不能为空"); return }
    const values = { name: name.trim(), description: description.trim(), rate_multiplier: parsedRate, model_types: parsedTypes }
    if (stationType === "newapi") {
      const updates = buildRelayGroupUpdates(stationType, group, values)
      for (const patch of updates) if (!(await update(patch))) return
      if (!Number.isFinite(parsedRate) || parsedRate < 0) { toast.error("分组倍率必须是非负数"); return }
      setDirty(false)
      return
    }
    if (!Number.isFinite(parsedRate) || parsedRate < 0) { toast.error("分组倍率必须是非负数"); return }
    if (await update(buildRelayGroupPatch(stationType, group, values))) setDirty(false)
  }

  return <div className={cn("border-b border-border last:border-0", dragging && "bg-brand/5")} onDragOver={(event) => { if (!dragDisabled) { event.preventDefault(); onDragOver(group.external_id) } }} onDrop={(event) => { event.preventDefault(); if (!dragDisabled) onDrop(group.external_id) }}>
    <div className="grid gap-3 px-4 py-2.5 lg:min-w-[1422px] lg:grid-cols-[88px_minmax(152px,.8fr)_132px_minmax(172px,1fr)_114px_minmax(152px,.9fr)_128px_120px_144px_96px_144px] lg:items-center lg:gap-0 lg:p-0 lg:[&>*]:px-4 lg:[&>*]:py-2.5">
      <div className="flex items-center gap-1"><button type="button" draggable={!dragDisabled} onDragStart={() => onDragStart(group.external_id)} aria-label={dragDisabled ? "清除临时排序后可拖拽分组" : `拖拽排序${group.name}`} className={cn("flex size-8 cursor-grab items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground active:cursor-grabbing", dragDisabled && "cursor-not-allowed opacity-40")}><GripVertical className="size-4" /></button><button type="button" aria-expanded={expanded} aria-label={`${expanded ? "收起" : "展开"}${group.name}中的账号`} onClick={() => setExpanded((value) => !value)} className="flex size-9 cursor-pointer items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"><ChevronDown className={cn("size-4 transition-transform duration-200", expanded && "rotate-180")} /></button></div>
      <div><Input value={name} onChange={(event) => { setName(event.target.value); setDirty(true) }} aria-label={`${group.name}分组名称`} className="h-10 min-w-0 text-sm" disabled={busy || stationType === "newapi"} title={stationType === "newapi" ? "NewAPI 分组名称由上游管理，暂不支持重命名" : undefined} /></div>
      <div className="flex items-center justify-between gap-3 lg:block"><span className="text-xs text-muted-foreground lg:hidden">平台</span><PlatformBadge platform={group.platform} /></div>
      <div><Input value={description} onChange={(event) => { setDescription(event.target.value); setDirty(true) }} aria-label={`${group.name}分组描述`} placeholder="暂无描述" className="h-10 min-w-0 text-sm" disabled={busy} /></div>
      <div><Input value={rate} onChange={(event) => { setRate(event.target.value); setDirty(true) }} aria-label={`${group.name}分组倍率`} inputMode="decimal" className="h-10 font-mono text-sm tabular-nums" disabled={busy} />{stationType === "newapi" ? <p className="mt-1 text-[10px] text-muted-foreground">修改上游倍率需要 root 权限</p> : null}</div>
      <div><Input value={modelTypes} onChange={(event) => { setModelTypes(event.target.value); setDirty(true) }} aria-label={`${group.name}模型类型`} placeholder="gpt, claude" className="h-10 min-w-0 text-sm" disabled={busy} /></div>
      <div className="flex min-h-11 items-center justify-between gap-3 lg:justify-start"><span className="text-xs text-muted-foreground lg:hidden">类型</span><div className="flex items-center gap-2"><Switch checked={Boolean(group.is_exclusive)} onCheckedChange={(checked) => void update({ is_exclusive: checked, monitor_enabled: !checked })} disabled={busy} aria-label={`${group.name}切换为${group.is_exclusive ? "公开" : "专属"}分组`} title="公开自动开启监控，专属自动关闭监控" /><span className={cn("text-xs font-medium", group.is_exclusive ? "text-warning" : "text-brand")}>{group.is_exclusive ? "专属" : "公开"}</span></div></div>
      <div className="flex min-h-11 items-center justify-between gap-3 lg:justify-start"><span className="text-xs text-muted-foreground lg:hidden">监控</span><div className="flex items-center gap-2"><Switch checked={group.monitor_enabled} onCheckedChange={(checked) => void update({ monitor_enabled: checked })} disabled={busy} aria-label={`${group.name}${group.monitor_enabled ? "关闭" : "开启"}公开监控`} /><span className={cn("text-xs font-medium", group.monitor_enabled ? "text-brand" : "text-muted-foreground")}>{group.monitor_enabled ? "已监控" : "未监控"}</span></div></div>
      <div className="flex min-h-11 items-center justify-between gap-3 lg:justify-start"><span className="text-xs text-muted-foreground lg:hidden">状态</span><div className="flex items-center gap-2"><Switch checked={active} onCheckedChange={(checked) => void update({ status: checked ? "active" : "inactive" })} disabled={busy || stationType === "newapi"} title={stationType === "newapi" ? "NewAPI 无独立分组状态接口" : undefined} aria-label={`${group.name}${active ? "停用" : "启用"}`} /><span className={cn("text-xs font-medium", active ? "text-success" : "text-muted-foreground")}>{active ? "已启用" : "已停用"}</span></div></div>
      <div className="flex items-center justify-between gap-2 text-xs lg:block"><span className="text-muted-foreground lg:hidden">账号</span><p className="font-mono text-sm font-semibold tabular-nums text-foreground">{accounts.length}</p></div>
      <div className="fixed-column-shadow-desktop fixed-column-shadow-right flex flex-nowrap justify-start gap-1.5 lg:sticky lg:right-0 lg:z-10 lg:self-stretch lg:items-center lg:justify-end lg:bg-card"><Tooltip delayDuration={250}><TooltipTrigger asChild><TableActionButton type="button" className="shrink-0 text-brand" aria-label={`快速测试${group.name}`} disabled={busy || testing || deleting} onClick={() => onTest(group)}><Play className={cn("size-3.5", testing && "animate-pulse")} /></TableActionButton></TooltipTrigger><TooltipContent side="top">快速测试</TooltipContent></Tooltip><Tooltip delayDuration={250}><TooltipTrigger asChild><TableActionButton type="button" className="shrink-0" aria-label={`保存${group.name}`} disabled={busy || testing || deleting || !changed} onClick={() => void save()}><Save className={cn("size-3.5", busy && "animate-pulse")} /></TableActionButton></TooltipTrigger><TooltipContent side="top">保存修改</TooltipContent></Tooltip><Tooltip delayDuration={250}><TooltipTrigger asChild><TableActionButton destructive type="button" className="shrink-0" aria-label={`删除分组${group.name}`} disabled={busy || testing || deleting} onClick={() => onDelete(group)}><Trash2 className={cn("size-3.5", deleting && "animate-pulse")} /></TableActionButton></TooltipTrigger><TooltipContent side="top">删除分组</TooltipContent></Tooltip></div>
    </div>
    {expanded ? <div className="border-t border-border bg-muted/20 px-4 py-3"><div className="mb-2 flex items-center justify-between gap-3"><p className="text-xs font-medium text-foreground">分组账号</p><span className="text-[11px] text-muted-foreground">{accounts.length} 个</span></div>{accounts.length === 0 ? <p className="rounded-md border border-dashed border-border px-4 py-5 text-center text-xs text-muted-foreground">该分组暂未关联账号</p> : <div className="grid grid-cols-1 gap-3 sm:grid-cols-[repeat(auto-fill,minmax(260px,320px))] sm:justify-start">{accounts.map((account) => { const state = stateMeta[account.risk_state]; return <article key={account.external_id} className="rounded-md border border-border bg-card px-3 py-2.5 shadow-xs"><div className="flex items-start justify-between gap-2"><div className="min-w-0"><AccountName account={account} /><div className="mt-1 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-muted-foreground"><span>{account.platform || "-"} · {account.type || "-"}</span><span>#{account.external_id}</span>{account.model_type ? <span className="rounded bg-brand/10 px-1.5 py-0.5 font-medium text-brand">{account.model_type}</span> : null}</div></div><span className={cn("shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium", state.tone)}>{state.label}</span></div><div className="mt-2 flex items-center justify-between gap-3 border-t border-border pt-2 text-[11px]"><span className="text-muted-foreground">成本倍率 <strong className="ml-1 font-mono text-sm font-semibold tabular-nums text-foreground">{multiplier(account.cost_multiplier)}</strong></span><span className={cn("shrink-0 font-medium", account.schedulable ? "text-success" : "text-muted-foreground")}>{account.schedulable ? "调度已开启" : "调度已关闭"}</span></div></article> })}</div>}</div> : null}
  </div>
}

function GroupManagement({ overview, busy, refreshing, testingGroupID, deletingGroupID, onUpdate, onOrderChange, onTest, onDelete, onRefresh }: { overview: RelayOverview; busy: boolean; refreshing: boolean; testingGroupID: number | null; deletingGroupID: number | null; onUpdate: (groupID: number, patch: RelayGroupPatch) => Promise<void>; onOrderChange: (ids: number[]) => Promise<void>; onTest: (group: RelayGroupView) => void; onDelete: (group: RelayGroupView) => void; onRefresh: () => void }) {
  const [open, setOpen] = usePersistedOpen("uh_relay_group_management_open")
  const [typeFilter, setTypeFilter] = useState<"public" | "exclusive" | "all">("all")
  const [statusFilter, setStatusFilter] = useState<"active" | "inactive" | "all">("all")
  const [sort, setSort] = useState<SortState<GroupSortKey>>(null)
  const [localOrder, setLocalOrder] = useState<number[]>([])
  const [draggingID, setDraggingID] = useState<number | null>(null)
  const [savingOrder, setSavingOrder] = useState(false)
  const [inputFocused, setInputFocused] = useState(false)
  const [tableOverview, setTableOverview] = useState(overview)
  useEffect(() => { if (!inputFocused) setTableOverview(overview) }, [inputFocused, overview])
  useEffect(() => { setLocalOrder(tableOverview.groups.map((group) => group.external_id)) }, [tableOverview.groups])
  const accountsByGroup = useMemo(() => {
    const result = new Map<number, RelayAccountView[]>()
    for (const account of tableOverview.accounts) {
      for (const group of account.current_groups) {
        const items = result.get(group.external_id) ?? []
        items.push(account)
        result.set(group.external_id, items)
      }
    }
    return result
  }, [tableOverview.accounts])
  const orderedGroups = useMemo(() => {
    const byID = new Map(tableOverview.groups.map((group) => [group.external_id, group]))
    const ids = localOrder.length ? localOrder : tableOverview.groups.map((group) => group.external_id)
    return ids.map((id) => byID.get(id)).filter((group): group is RelayGroupView => Boolean(group))
  }, [localOrder, tableOverview.groups])
  const groups = useMemo(() => {
    const filtered = orderedGroups.filter((group) => {
      if (typeFilter === "public" && group.is_exclusive) return false
      if (typeFilter === "exclusive" && !group.is_exclusive) return false
      const active = group.status?.toLowerCase() === "active"
      if (statusFilter === "active" && !active) return false
      if (statusFilter === "inactive" && active) return false
      return true
    })
    if (!sort) return filtered
    const direction = sort.direction === "asc" ? 1 : -1
    return [...filtered].sort((a, b) => {
      const compared = sort.key === "name" ? a.name.localeCompare(b.name, "zh-CN") : a.rate_multiplier - b.rate_multiplier
      return compared * direction || a.name.localeCompare(b.name, "zh-CN")
    })
  }, [orderedGroups, sort, statusFilter, typeFilter])
  function moveGroup(targetID: number) {
    if (draggingID == null || draggingID === targetID || sort) return
    const visibleIDs = groups.map((group) => group.external_id)
    const from = visibleIDs.indexOf(draggingID)
    const to = visibleIDs.indexOf(targetID)
    if (from < 0 || to < 0) return
    const reorderedVisible = [...visibleIDs]
    reorderedVisible.splice(from, 1)
    reorderedVisible.splice(to, 0, draggingID)
    let cursor = 0
    const next = (localOrder.length ? localOrder : tableOverview.groups.map((group) => group.external_id)).map((id) => visibleIDs.includes(id) ? reorderedVisible[cursor++] : id)
    setLocalOrder(next)
    setDraggingID(null)
    setSavingOrder(true)
    void onOrderChange(next).catch((error) => toast.error(error instanceof Error ? error.message : "保存分组排序失败")).finally(() => setSavingOrder(false))
  }
  function toggleSort(key: GroupSortKey, initialDirection: SortDirection) {
    setSort((current) => current?.key === key ? { key, direction: current.direction === "asc" ? "desc" : "asc" } : { key, direction: initialDirection })
  }
  function resetFilters() { setTypeFilter("all"); setStatusFilter("all"); setSort(null) }
  return <Card className="gap-0 overflow-hidden border border-border py-0 shadow-none">
    <ListPanelHeader
          secondaryPlacement="inline"
          actionsPlacement="secondary"
          title={<CardTitle className="flex items-center gap-2 text-sm font-semibold"><Layers3 className="size-4 text-brand" />分组管理</CardTitle>}
          description={open ? <p className="mt-1 truncate text-xs text-muted-foreground">{overview.station.type === "newapi" ? "NewAPI 名称与状态由上游管理且没有独立变更接口；描述、模型类型、分组类型、监控和排序保存在本地，倍率修改需要 root 权限。" : "编辑名称、描述、倍率和模型类型；分组类型、监控与状态可直接切换。"}</p> : null}
actions={open ? <>
<Button type="button" variant="outline" size="sm" className="h-11 sm:h-9 gap-1.5" disabled={busy || refreshing || inputFocused} title={inputFocused ? "编辑中，刷新已暂停" : undefined} onClick={onRefresh}><RefreshCw className={cn("size-3.5", refreshing && !inputFocused && "animate-spin")} />刷新</Button>
</> : null}
          secondary={open ? <>
<ListFilterMenu
  activeCount={Number(typeFilter !== "all") + Number(statusFilter !== "all")}
  onReset={resetFilters}
  groups={[
    { label: "分组类型", value: typeFilter, onValueChange: (value) => setTypeFilter(value as typeof typeFilter), options: [{ value: "all", label: "All" }, { value: "public", label: "公开" }, { value: "exclusive", label: "专属" }] },
    { label: "分组状态", value: statusFilter, onValueChange: (value) => setStatusFilter(value as typeof statusFilter), options: [{ value: "all", label: "All" }, { value: "active", label: "已启用" }, { value: "inactive", label: "已停用" }] },
  ]}
/>
<Button type="button" variant="outline" size="sm" className="h-11 sm:h-9 gap-1.5" onClick={resetFilters}><RotateCcw className="size-3.5" />重置</Button>
{inputFocused ? <span className="self-center text-[11px] font-medium text-warning" aria-live="polite">编辑中，刷新已暂停</span> : null}
{sort ? <span className="self-center text-[11px] text-muted-foreground">清除名称/倍率排序后可拖拽</span> : null}
{savingOrder ? <span className="self-center text-[11px] text-muted-foreground">正在保存排序…</span> : null}
</> : null}
          meta={<><div className="flex shrink-0 items-center gap-2">{open ? <span className="text-xs text-muted-foreground">{groups.length} / {tableOverview.groups.length} 个</span> : null}<Button type="button" variant="ghost" size="icon" className="size-11 sm:size-9" aria-label={open ? "收起分组管理" : "展开分组管理"} aria-expanded={open} onClick={() => setOpen((value) => !value)}><ChevronDown className={cn("size-4 transition-transform duration-200", open && "rotate-180")} /></Button></div></>}
        ></ListPanelHeader>
    {open ? <CardContent className="border-t border-border p-0"><div className="relative h-[min(56vh,560px)]"><div className="h-full overflow-auto" onFocusCapture={(event) => { if (event.target instanceof HTMLInputElement) setInputFocused(true) }} onBlurCapture={(event) => { if (!(event.target instanceof HTMLInputElement)) return; if (event.relatedTarget instanceof HTMLInputElement && event.currentTarget.contains(event.relatedTarget)) return; setInputFocused(false) }}><div className="isolate"><div className="sticky top-0 z-30 hidden min-w-[1422px] grid-cols-[88px_minmax(152px,.8fr)_132px_minmax(172px,1fr)_114px_minmax(152px,.9fr)_128px_120px_144px_96px_144px] border-b border-border bg-muted text-[11px] text-foreground/90 font-medium lg:grid lg:[&>*]:px-4 lg:[&>*]:py-2"><span /><SortButton label="名称" active={sort?.key === "name"} direction={sort?.key === "name" ? sort.direction : undefined} onClick={() => toggleSort("name", "asc")} /><span>平台</span><span>描述</span><SortButton label="倍率" active={sort?.key === "rate"} direction={sort?.key === "rate" ? sort.direction : undefined} onClick={() => toggleSort("rate", "desc")} /><span className="inline-flex items-center gap-1">模型类型<ModelTypeHelp /></span><span>类型</span><span>监控</span><span>状态</span><span>账号</span><span className="fixed-column-shadow-desktop fixed-column-shadow-right sticky right-0 z-40 flex self-stretch items-center justify-center bg-muted">操作</span></div>{groups.length === 0 ? <p className="px-4 py-10 text-center text-sm text-muted-foreground">当前筛选没有分组</p> : groups.map((group) => <GroupManagementRow stationType={overview.station.type} key={group.external_id} group={group} accounts={accountsByGroup.get(group.external_id) ?? []} onUpdate={onUpdate} onTest={onTest} onDelete={onDelete} testing={testingGroupID === group.external_id} deleting={deletingGroupID === group.external_id} dragDisabled={Boolean(sort) || savingOrder} dragging={draggingID === group.external_id} onDragStart={setDraggingID} onDragOver={() => undefined} onDrop={moveGroup} />)}</div></div>{busy || (refreshing && !inputFocused) ? <div className="pointer-events-none absolute inset-x-0 top-10 z-40 flex justify-center"><span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground"><RefreshCw className="size-3.5 animate-spin text-brand" />正在刷新分组列表</span></div> : null}</div></CardContent> : null}
  </Card>
}

function isProfitableGroupCandidate(account: RelayAccountView, group: RelayGroupOption) {
  if (account.cost_multiplier == null || group.status?.toLowerCase() !== "active") return false
  if ((account.platform || "").trim().toLowerCase() !== (group.platform || "").trim().toLowerCase()) return false
  const modelType = (account.model_type || "").trim().toLowerCase()
  if (!modelType || !group.model_types?.map((value) => value.trim().toLowerCase()).includes(modelType)) return false
  const accountType = (account.type || "unknown").trim().toLowerCase()
  if (group.require_oauth_only && accountType === "apikey") return false
  if (group.account_types?.length && !group.account_types.map((value) => value.trim().toLowerCase()).includes(accountType)) return false
  return group.rate_multiplier > account.cost_multiplier
}

function GroupEditor({ account, groups, open, busy, onOpenChange, onSave }: { account: RelayAccountView | null; groups: RelayGroupOption[]; open: boolean; busy: boolean; onOpenChange: (open: boolean) => void; onSave: (ids: number[]) => void }) {
  const [ids, setIDs] = useState<number[]>([])
  useEffect(() => setIDs(account?.current_groups.map((group) => group.external_id) ?? []), [account])
  const orderedGroups = useMemo(() => publicFirst(groups), [groups])
  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent className="max-w-xl"><DialogHeader><DialogTitle>手动调整账号分组</DialogTitle><DialogDescription>{account ? <>{account.name} · {account.platform || "-"} · {account.type || "-"}{account.cost_multiplier == null ? " · 当前成本未知" : <> · 当前成本 <strong className="font-mono font-semibold text-foreground">{multiplier(account.cost_multiplier)}</strong></>}</> : "选择销售分组"}</DialogDescription></DialogHeader><div className="flex flex-wrap gap-2 text-[11px] text-muted-foreground"><span className="rounded bg-success/10 px-2 py-1 font-medium text-success">有利润候选</span><span>启用、同平台、同账号类型、同模型类型且倍率高于当前账号成本</span></div><div className="max-h-[min(52vh,460px)] space-y-2 overflow-y-auto pr-1">{orderedGroups.map((group) => { const checked = ids.includes(group.external_id); const profitable = account ? isProfitableGroupCandidate(account, group) : false; return <label key={group.external_id} className={cn("flex cursor-pointer items-center justify-between gap-3 border px-3 py-3 transition-colors", checked ? "border-brand/50 bg-brand/5" : profitable ? "border-success/30 bg-success/5 hover:bg-success/10" : "border-border hover:bg-muted/40")}><span className="flex min-w-0 items-center gap-3"><Checkbox checked={checked} onCheckedChange={(value) => setIDs((current) => value === true ? [...new Set([...current, group.external_id])] : current.filter((id) => id !== group.external_id))} /><span className="min-w-0"><span className="flex flex-wrap items-center gap-1.5"><span className="truncate text-sm font-medium">{group.name}</span>{profitable ? <span className="rounded bg-success/10 px-1.5 py-0.5 text-[10px] font-medium text-success">有利润候选</span> : null}<span className={cn("rounded px-1.5 py-0.5 text-[10px] font-medium", group.is_exclusive ? "bg-warning/10 text-warning" : "bg-blue-500/10 text-blue-700 dark:text-blue-400")}>{group.is_exclusive ? "专属" : "公开"}</span></span><span className="mt-1 block text-[11px] text-muted-foreground">{group.platform || "-"} · {group.model_types?.length ? group.model_types.join("/") : "未配置模型类型"} · {group.require_oauth_only ? "OAuth 类型" : group.account_types?.length ? group.account_types.join("/") : "通用类型"} · 倍率 <strong className="font-mono text-sm font-semibold tabular-nums text-foreground">{multiplier(group.rate_multiplier)}</strong></span></span></span><span className={cn("size-2 rounded-full", group.status === "active" ? "bg-success" : "bg-muted-foreground")} /></label> })}</div><DialogFooter><Button type="button" variant="outline" onClick={() => onOpenChange(false)}>取消</Button><Button type="button" disabled={busy} onClick={() => onSave(ids)}>保存分组</Button></DialogFooter></DialogContent></Dialog>
}

function BatchGroupEditor({ groups, count, open, busy, onOpenChange, onSave }: { groups: RelayGroupOption[]; count: number; open: boolean; busy: boolean; onOpenChange: (open: boolean) => void; onSave: (ids: number[]) => void }) {
  const [ids, setIDs] = useState<number[]>([])
  useEffect(() => { if (open) setIDs([]) }, [open])
  const orderedGroups = useMemo(() => publicFirst(groups), [groups])
  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent className="max-w-xl"><DialogHeader><DialogTitle>批量调整销售分组</DialogTitle><DialogDescription>将为已选的 {count} 个账号设置相同的销售分组，提交后逐个同步到 Sub2API。</DialogDescription></DialogHeader><div className="max-h-[min(52vh,460px)] space-y-2 overflow-y-auto pr-1">{orderedGroups.map((group) => { const checked = ids.includes(group.external_id); return <label key={group.external_id} className={cn("flex cursor-pointer items-center justify-between gap-3 border px-3 py-2.5 transition-colors", checked ? "border-brand/50 bg-brand/5" : "border-border hover:bg-muted/40")}><span className="flex min-w-0 items-center gap-3"><Checkbox checked={checked} onCheckedChange={(value) => setIDs((current) => value === true ? [...new Set([...current, group.external_id])] : current.filter((id) => id !== group.external_id))} /><span className="min-w-0"><span className="flex flex-wrap items-center gap-1.5"><span className="block truncate text-sm font-medium">{group.name}</span><span className={cn("rounded px-1.5 py-0.5 text-[10px] font-medium", group.is_exclusive ? "bg-warning/10 text-warning" : "bg-blue-500/10 text-blue-700 dark:text-blue-400")}>{group.is_exclusive ? "专属" : "公开"}</span></span><span className="mt-1 block text-[11px] text-muted-foreground">{group.platform || "-"} · {group.require_oauth_only ? "OAuth 类型" : "通用类型"} · 倍率 {multiplier(group.rate_multiplier)}</span></span></span><span className={cn("size-2 rounded-full", group.status === "active" ? "bg-success" : "bg-muted-foreground")} /></label> })}</div><DialogFooter><Button type="button" variant="outline" onClick={() => onOpenChange(false)}>取消</Button><Button type="button" disabled={busy} onClick={() => onSave(ids)}>保存分组</Button></DialogFooter></DialogContent></Dialog>
}

function newBatchCloneRow(source: RelayAccountView): BatchCloneFormRow {
  return { id: `${Date.now()}-${Math.random()}`, name: source.name, api_key: "", base_url: source.base_url?.trim() || "", model_type: source.model_type?.trim() || "" }
}

function cloneBatchCloneRow(row: BatchCloneFormRow): BatchCloneFormRow {
  return { ...row, id: `${Date.now()}-${Math.random()}` }
}

const compactSourceTagClass = "inline-flex max-w-full shrink-0 items-center gap-1 rounded bg-brand/10 px-1.5 py-0.5 text-[10px] font-medium text-brand"

function AccountTypeBadge({ type }: { type?: string }) {
  const value = type?.trim().toLowerCase().replace(/[-\s]/g, "_") || ""
  const meta = value === "apikey" || value === "api_key"
    ? { label: "API Key", Icon: KeyRound }
    : value === "oauth"
      ? { label: "OAuth", Icon: ShieldCheck }
      : value === "bedrock"
        ? { label: "Bedrock", Icon: Box }
        : value === "service_account"
          ? { label: "服务账号", Icon: Users }
          : value === "upstream"
            ? { label: "上游", Icon: Server }
            : { label: type?.trim() || "未知类型", Icon: CircleHelp }
  const Icon = meta.Icon
  return <span className={cn(compactSourceTagClass, "bg-muted text-muted-foreground")} title={type || "未知账号类型"}><Icon className="size-3" />{meta.label}</span>
}

function SourceAccountMeta({ account }: { account: RelayAccountView }) {
  return <div className="mt-1 flex min-w-0 items-center gap-1.5 overflow-x-auto whitespace-nowrap pb-0.5">
    {(account.current_groups ?? []).length > 0 ? (account.current_groups ?? []).map((group) => <span key={group.external_id} className={cn(compactSourceTagClass, "max-w-40 truncate")} title={`${group.name} · 倍率 ${multiplier(group.rate_multiplier)}`}>{group.name}</span>) : <span className={cn(compactSourceTagClass, "bg-muted text-muted-foreground")}>未分配分组</span>}
    <PlatformBadge platform={account.platform} compact />
    <AccountTypeBadge type={account.type} />
  </div>
}

function AccountRateBadge({ account }: { account: RelayAccountView }) {
  return <span className="inline-flex shrink-0 items-center rounded bg-brand/10 px-1.5 py-0.5 font-mono text-[10px] font-semibold tabular-nums text-brand" title="当前账号成本倍率">账号倍率 {multiplier(account.cost_multiplier)}</span>
}

function BatchCloneDialog({ accounts, availableModelTypes, open, busy, result, onOpenChange, onSubmit }: { accounts: RelayAccountView[]; availableModelTypes: string[]; open: boolean; busy: boolean; result: RelayAccountBatchCloneResult | null; onOpenChange: (open: boolean) => void; onSubmit: (groups: RelayAccountBatchCloneGroup[]) => void }) {
  const [formGroups, setFormGroups] = useState<BatchCloneFormGroup[]>([])
  const [error, setError] = useState("")
  const [sourceAccountQuery, setSourceAccountQuery] = useState("")
  const [sourceGroupFilter, setSourceGroupFilter] = useState("all")
  const accountByID = useMemo(() => new Map(accounts.map((account) => [account.external_id, account])), [accounts])
  const sourceGroupOptions = useMemo(() => {
    const groups = new Map<number, RelayGroupOption>()
    accounts.forEach((account) => (account.current_groups ?? []).forEach((group) => groups.set(group.external_id, group)))
    return publicFirst(Array.from(groups.values()))
  }, [accounts])
  const modelTypeOptions = useMemo(() => {
    const values = [...availableModelTypes, ...accounts.flatMap((account) => [account.model_type, ...(account.current_groups ?? []).flatMap((group) => group.model_types ?? [])])]
    return Array.from(new Set(values.map((value) => value?.trim().toLowerCase()).filter(Boolean) as string[])).sort((a, b) => a.localeCompare(b, "zh-CN"))
  }, [accounts, availableModelTypes])
  const filteredSourceAccounts = useMemo(() => {
    const query = sourceAccountQuery.trim().toLowerCase()
    return accounts.filter((account) => {
      if (query && !account.name.toLowerCase().includes(query)) return false
      if (sourceGroupFilter === "__unassigned") return (account.current_groups ?? []).length === 0
      if (sourceGroupFilter !== "all" && !(account.current_groups ?? []).some((group) => String(group.external_id) === sourceGroupFilter)) return false
      return true
    })
  }, [accounts, sourceAccountQuery, sourceGroupFilter])
  const total = formGroups.reduce((sum, group) => sum + group.rows.length, 0)

  function close() {
    if (busy) return
    setFormGroups([])
    setError("")
    setSourceAccountQuery("")
    setSourceGroupFilter("all")
    onOpenChange(false)
  }

  function toggleSource(account: RelayAccountView, checked: boolean) {
    setError("")
    setFormGroups((current) => {
      if (!checked) return current.filter((group) => group.source_account_external_id !== account.external_id)
      if (current.some((group) => group.source_account_external_id === account.external_id)) return current
      return [...current, { source_account_external_id: account.external_id, rows: [newBatchCloneRow(account)] }]
    })
  }

  function updateRow(sourceID: number, rowID: string, patch: Partial<BatchCloneFormRow>) {
    setError("")
    setFormGroups((current) => current.map((group) => group.source_account_external_id === sourceID ? { ...group, rows: group.rows.map((row) => row.id === rowID ? { ...row, ...patch } : row) } : group))
  }

  function addRow(sourceID: number) {
    const source = accountByID.get(sourceID)
    if (!source) return
    const group = formGroups.find((item) => item.source_account_external_id === sourceID)
    if (group && group.rows.length >= 100) {
      setError("每个源账号最多新增 100 个账号")
      return
    }
    setError("")
    setFormGroups((current) => current.map((group) => group.source_account_external_id === sourceID ? { ...group, rows: [...group.rows, newBatchCloneRow(source)] } : group))
  }

  function copyRow(sourceID: number, rowID: string) {
    const group = formGroups.find((item) => item.source_account_external_id === sourceID)
    if (!group) return
    if (group.rows.length >= 100) {
      setError("每个源账号最多新增 100 个账号")
      return
    }
    if (total >= 300) {
      setError("一次最多新增 300 个账号")
      return
    }
    setError("")
    setFormGroups((current) => current.map((item) => {
      if (item.source_account_external_id !== sourceID) return item
      const index = item.rows.findIndex((row) => row.id === rowID)
      if (index < 0) return item
      const rows = [...item.rows]
      rows.splice(index + 1, 0, cloneBatchCloneRow(item.rows[index]))
      return { ...item, rows }
    }))
  }

  function copyGroup(sourceID: number) {
    const group = formGroups.find((item) => item.source_account_external_id === sourceID)
    if (!group) return
    if (group.rows.length * 2 > 100) {
      setError("复制本组后会超过该源账号最多新增 100 个新增账号的限制")
      return
    }
    if (total + group.rows.length > 300) {
      setError("复制本组后会超过一次最多新增 300 个账号的限制")
      return
    }
    setError("")
    setFormGroups((current) => current.map((item) => item.source_account_external_id === sourceID ? { ...item, rows: [...item.rows, ...item.rows.map(cloneBatchCloneRow)] } : item))
  }

  function copyAll() {
    if (formGroups.some((group) => group.rows.length * 2 > 100)) {
      setError("复制全部后会超过某个源账号最多新增 100 个新增账号的限制")
      return
    }
    if (total * 2 > 300) {
      setError("复制全部后会超过一次最多新增 300 个账号的限制")
      return
    }
    setError("")
    setFormGroups((current) => current.map((group) => ({ ...group, rows: [...group.rows, ...group.rows.map(cloneBatchCloneRow)] })))
  }

  function removeRow(sourceID: number, rowID: string) {
    setError("")
    setFormGroups((current) => current.map((group) => group.source_account_external_id === sourceID && group.rows.length > 1 ? { ...group, rows: group.rows.filter((row) => row.id !== rowID) } : group))
  }

  function submit() {
    if (formGroups.length === 0) {
      setError("至少选择一个源账号")
      return
    }
    if (total > 300) {
      setError("一次最多新增 300 个账号")
      return
    }
    for (const group of formGroups) {
      const source = accountByID.get(group.source_account_external_id)
      if (!source) {
        setError("源账号已不在当前快照中，请关闭后刷新重试")
        return
      }
      for (const row of group.rows) {
        if (!row.api_key.trim()) {
          setError(`请填写“${source.name}”分组中每个新账号的 API Key`)
          return
        }
      }
    }
    onSubmit(formGroups.map((group) => ({ source_account_external_id: group.source_account_external_id, accounts: group.rows.map((row) => ({ name: row.name.trim(), api_key: row.api_key.trim(), base_url: row.base_url.trim(), model_type: row.model_type.trim() })) })))
  }

  return <Dialog open={open} onOpenChange={(next) => { if (next) onOpenChange(true); else close() }}>
    <DialogContent className="flex w-[calc(100vw-2rem)] max-h-[calc(100dvh-2rem)] sm:max-w-[900px] flex-col overflow-hidden p-0">
      {result ? <>
        <DialogHeader className="shrink-0 border-b border-border px-4 py-5 sm:px-6"><DialogTitle>批量新增结果</DialogTitle><DialogDescription>已完成逐个克隆和更新，API Key 不会显示在结果中。</DialogDescription></DialogHeader>
        <div className="min-h-0 flex-1 space-y-4 overflow-y-auto px-4 py-4 sm:px-6">
          <div className="grid grid-cols-3 gap-2 text-center"><div className="rounded-md border border-border bg-muted/40 px-2 py-2"><p className="text-[11px] text-muted-foreground">请求</p><p className="font-mono text-base font-semibold tabular-nums">{result.requested}</p></div><div className="rounded-md border border-success/25 bg-success/10 px-2 py-2"><p className="text-[11px] text-success">成功</p><p className="font-mono text-base font-semibold tabular-nums text-success">{result.succeeded}</p></div><div className="rounded-md border border-danger/25 bg-danger/10 px-2 py-2"><p className="text-[11px] text-danger">失败</p><p className="font-mono text-base font-semibold tabular-nums text-danger">{result.failed}</p></div></div>
          {result.sync_error ? <Alert variant="destructive"><CircleAlert className="size-4" /><AlertDescription>账号已在上游处理，但刷新本地快照失败：{result.sync_error}</AlertDescription></Alert> : null}
          <div className="max-h-[min(52vh,480px)] space-y-2 overflow-y-auto pr-1">{result.results.map((item, index) => <div key={`${item.source_account_external_id}-${item.external_id}-${index}`} className="flex min-w-0 items-start gap-2 rounded-md border border-border bg-card px-3 py-2.5 text-xs"><span className={cn("mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full", item.success ? "bg-success/10 text-success" : "bg-danger/10 text-danger")}>{item.success ? <Check className="size-3" /> : <CircleAlert className="size-3" />}</span><div className="min-w-0 flex-1"><div className="flex flex-wrap items-center justify-between gap-2"><span className="min-w-0 break-words font-medium">{item.name}</span><span className={cn("shrink-0 font-mono tabular-nums", item.success ? "text-muted-foreground" : "text-danger")}>{item.success ? "已创建" : "创建失败"}</span></div><p className="mt-1 text-[11px] text-muted-foreground">源账号：{item.source_account_name}</p>{item.error ? <p className="mt-1 break-words whitespace-pre-wrap text-[11px] text-danger">{item.error}</p> : null}</div></div>)}</div>
        </div>
        <DialogFooter className="shrink-0 border-t border-border px-4 py-4 sm:px-6"><Button type="button" onClick={close}>关闭</Button></DialogFooter>
      </> : <>
        <DialogHeader className="shrink-0 border-b border-border px-4 py-5 sm:px-6"><DialogTitle>批量新增账号</DialogTitle><DialogDescription>选择多个已有账号作为模板，每个模板下可新增多行。新账号会完整克隆模板，可分别修改名称、API Key、Base URL 和模型类型，并默认关闭调度、状态正常。</DialogDescription></DialogHeader>
        <div className="min-h-0 flex-1 space-y-4 overflow-y-auto px-4 py-4 sm:px-6">
          <div className="flex flex-wrap gap-2">
            <div className="relative min-w-48 flex-1">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
            <Input value={sourceAccountQuery} onChange={(event) => setSourceAccountQuery(event.target.value)} placeholder="按源账号名称筛选" aria-label="按源账号名称筛选" className="h-10 pl-8 text-sm" disabled={busy} />
            </div>
            <Select value={sourceGroupFilter} onValueChange={setSourceGroupFilter} disabled={busy}>
              <SelectTrigger className="h-10 w-full text-sm sm:w-52" aria-label="按源账号分组筛选"><SelectValue placeholder="按所在分组筛选" /></SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All</SelectItem>
                <SelectItem value="__unassigned">未分配分组</SelectItem>
                {sourceGroupOptions.map((group) => <SelectItem key={group.external_id} value={String(group.external_id)}>{group.name} · 倍率 {multiplier(group.rate_multiplier)}</SelectItem>)}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2"><div className="flex items-center justify-between gap-3"><Label>选择源账号</Label><span className="text-[11px] text-muted-foreground">已选 {formGroups.length} 个 · 匹配 {filteredSourceAccounts.length} 个</span></div><div className="grid max-h-48 gap-2 overflow-y-auto pr-1 sm:grid-cols-2">{accounts.length === 0 ? <p className="col-span-full rounded-md border border-dashed border-border px-3 py-5 text-center text-xs text-muted-foreground">当前没有可用账号</p> : filteredSourceAccounts.length === 0 ? <p className="col-span-full rounded-md border border-dashed border-border px-3 py-5 text-center text-xs text-muted-foreground">没有匹配的源账号</p> : filteredSourceAccounts.map((account) => { const checked = formGroups.some((group) => group.source_account_external_id === account.external_id); const id = `batch-clone-source-${account.external_id}`; return <label key={account.external_id} htmlFor={id} className={cn("flex min-w-0 cursor-pointer items-start gap-2 rounded-md border px-3 py-2.5 transition-colors", checked ? "border-brand/50 bg-brand/5" : "border-border hover:bg-muted/40")}><Checkbox id={id} checked={checked} disabled={busy} onCheckedChange={(value) => toggleSource(account, value === true)} /><span className="min-w-0 flex-1"><span className="flex min-w-0 items-center gap-1.5"><span className="min-w-0 truncate text-xs font-medium" title={account.name}>{account.name}</span><AccountRateBadge account={account} /></span><SourceAccountMeta account={account} /></span></label> })}</div></div>
          {formGroups.length === 0 ? <div className="rounded-md border border-dashed border-border bg-muted/20 px-4 py-8 text-center text-xs text-muted-foreground">选择源账号后，在这里为每个源账号填写要新增的账号。</div> : <div className="space-y-3">
            <div className="flex items-center justify-between gap-2"><Label>新增账号</Label><Button type="button" variant="outline" size="sm" className="h-9 gap-1.5" aria-label="整体复制新增账号" disabled={busy || total * 2 > 300 || formGroups.some((group) => group.rows.length * 2 > 100)} onClick={copyAll}><Copy className="size-3.5" />整体复制</Button></div>
            {formGroups.map((group) => {
              const source = accountByID.get(group.source_account_external_id)
              if (!source) return null
              const copyGroupDisabled = busy || group.rows.length * 2 > 100 || total + group.rows.length > 300
              return <div key={group.source_account_external_id} className="space-y-3 rounded-md border border-border p-3">
                <div className="flex min-w-0 flex-wrap items-center justify-between gap-2">
                  <div className="min-w-0">
                    <div className="flex min-w-0 items-center gap-1.5"><p className="min-w-0 truncate text-sm font-semibold" title={source.name}>{source.name}</p><AccountRateBadge account={source} /></div>
                    <SourceAccountMeta account={source} />
                    <p className="mt-1 text-[11px] text-muted-foreground">新账号默认关闭调度，状态正常</p>
                  </div>
                  <div className="flex shrink-0 items-center gap-1.5"><Button type="button" variant="outline" size="sm" className="h-8 gap-1.5 px-2.5 text-xs" disabled={copyGroupDisabled} onClick={() => copyGroup(group.source_account_external_id)}><Copy className="size-3.5" />复制本组</Button><span className="rounded bg-brand/10 px-2 py-1 text-[11px] font-medium text-brand">{group.rows.length} 个新账号</span></div>
                </div>
                <div className="space-y-2">
                  {group.rows.map((row, index) => <div key={row.id} className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)_84px]">
                    <div className="space-y-1"><Label htmlFor={`${row.id}-name`} className="text-[11px] text-muted-foreground">账号名称 {index + 1}</Label><Input id={`${row.id}-name`} value={row.name} disabled={busy} onChange={(event) => updateRow(group.source_account_external_id, row.id, { name: event.target.value })} placeholder={source.name} maxLength={255} /></div>
                    <div className="space-y-1"><Label htmlFor={`${row.id}-key`} className="text-[11px] text-muted-foreground">API Key</Label><Input id={`${row.id}-key`} type="password" autoComplete="new-password" value={row.api_key} disabled={busy} onChange={(event) => updateRow(group.source_account_external_id, row.id, { api_key: event.target.value })} placeholder="粘贴新账号 API Key" /></div>
                    <div className="space-y-1"><Label htmlFor={`${row.id}-base-url`} className="text-[11px] text-muted-foreground">Base URL</Label><Input id={`${row.id}-base-url`} type="url" value={row.base_url} disabled={busy} onChange={(event) => updateRow(group.source_account_external_id, row.id, { base_url: event.target.value })} placeholder="沿用源账号地址" maxLength={1024} /></div>
                    <div className="space-y-1"><Label htmlFor={`${row.id}-model-type`} className="text-[11px] text-muted-foreground">模型类型</Label><Select value={row.model_type || "__empty"} onValueChange={(value) => updateRow(group.source_account_external_id, row.id, { model_type: value === "__empty" ? "" : value })} disabled={busy}><SelectTrigger id={`${row.id}-model-type`} className="h-10 w-full text-sm" aria-label={`设置第 ${index + 1} 个新账号模型类型`}><SelectValue placeholder="未绑定模型类型" /></SelectTrigger><SelectContent><SelectItem value="__empty">未绑定模型类型</SelectItem>{modelTypeOptions.map((value) => <SelectItem key={value} value={value}>{value}</SelectItem>)}</SelectContent></Select></div>
                    <div className="flex items-end justify-end gap-1"><Tooltip delayDuration={250}><TooltipTrigger asChild><Button type="button" variant="ghost" size="icon" className="size-10" aria-label={`复制第 ${index + 1} 个新账号到本组`} title="复制到本组" disabled={busy || group.rows.length >= 100 || total >= 300} onClick={() => copyRow(group.source_account_external_id, row.id)}><Copy className="size-4 text-brand" /></Button></TooltipTrigger><TooltipContent side="top">复制到本组</TooltipContent></Tooltip><Tooltip delayDuration={250}><TooltipTrigger asChild><Button type="button" variant="ghost" size="icon" className="size-10" aria-label={`删除第 ${index + 1} 个新账号`} title="删除新账号" disabled={busy || group.rows.length <= 1} onClick={() => removeRow(group.source_account_external_id, row.id)}><Trash2 className="size-4 text-danger" /></Button></TooltipTrigger><TooltipContent side="top">删除新账号</TooltipContent></Tooltip></div>
                  </div>)}
                </div>
                <Button type="button" variant="outline" size="sm" className="h-9 gap-1.5" disabled={busy || total >= 300 || group.rows.length >= 100} onClick={() => addRow(group.source_account_external_id)}><Plus className="size-3.5" />继续添加账号</Button>
              </div>
            })}
          </div>}
          <div className="flex flex-wrap items-center justify-between gap-2 border-t border-border pt-3 text-xs"><span className="text-muted-foreground">预计新增 <strong className="font-mono text-foreground">{total}</strong> 个账号</span><span className="text-muted-foreground">名称留空使用源账号名称 · Base URL 默认沿用源账号地址 · 模型类型默认沿用源账号 · 最多 300 个</span></div>
          {error ? <Alert variant="destructive" className="py-3"><CircleAlert className="size-4" /><AlertDescription>{error}</AlertDescription></Alert> : null}
        </div>
        <DialogFooter className="shrink-0 border-t border-border px-4 py-4 sm:px-6"><Button type="button" variant="outline" disabled={busy} onClick={close}>取消</Button><Button type="button" className="gap-1.5" disabled={busy || formGroups.length === 0 || total === 0} onClick={submit}>{busy ? <RefreshCw className="size-3.5 animate-spin" /> : <Plus className="size-3.5" />}开始批量新增</Button></DialogFooter>
      </>}
    </DialogContent>
  </Dialog>
}

export default function RelayStationsPage() {
  const stations = useRelayStations()
  usePrefetchRelayOverviews(stations.data?.map((station) => station.id) ?? [])
  const { confirm, dialog: confirmDialog } = useConfirm()
  const [selectedID, setSelectedID] = useState<number | null>(null)
  const [detailModule, setDetailModule] = usePersistedDetailModule("uh_relay_detail_module")
  const [usageRange, setUsageRange] = useState<RelayUsageRange>("today")
  const [latencyGranularity, setLatencyGranularity] = useState<LatencyBucketGranularity>("auto")
  const [accountViewMode, setAccountViewMode] = usePersistedRelayAccountViewMode()
  const [storedAccountColumns, setVisibleAccountColumns] = usePersistedAccountColumns("gatewayops_relay_account_columns")
  const accountDisplayBucketCount = relayAccountDisplayBucketCount(accountViewMode)
  const overview = useRelayOverview(selectedID)
  const overviewReady = overview.data !== null
  const channels = useChannels()
  const channelAssociations = useChannelRelayAssociations()
  const channelLatencyTrends = useChannelLatencyTrends(60, detailModule === "accounts" && overviewReady)
  const channelLatencyBuckets = useChannelLatencyBuckets(usageRange, latencyGranularity, accountDisplayBucketCount, detailModule === "accounts" && overviewReady)
  const channelConsumptionBuckets = useChannelConsumptionBuckets(usageRange, latencyGranularity, accountDisplayBucketCount, detailModule === "accounts" && overviewReady)
  const accountModelCatalog = useRelayAccountModelCatalog(selectedID, detailModule === "accounts")
  const dailyConsumption = useRelayDailyConsumption(overviewReady ? selectedID : null, "30d")
  const usage = useRelayUsage(detailModule === "accounts" && overviewReady ? selectedID : null, usageRange)
  const scoreLatencyBuckets = useRelayLatencyBuckets(detailModule === "accounts" && overviewReady ? selectedID : null, usageRange, "auto", 60)
  const latencyBuckets = useRelayLatencyBuckets(detailModule === "accounts" && overviewReady ? selectedID : null, usageRange, latencyGranularity, accountDisplayBucketCount)
  const accountConsumptionBuckets = useRelayAccountConsumptionBuckets(detailModule === "accounts" && overviewReady ? selectedID : null, usageRange, latencyGranularity, accountDisplayBucketCount)
  const recentUsage = useRelayRecentUsage(detailModule === "usage" && overviewReady ? selectedID : null, usageRange)
  const [historyUser, setHistoryUser] = useState<{ id: number; email: string; name: string } | null>(null)
  const userHistory = useRelayUserBalanceHistory(selectedID, historyUser?.id ?? null, 1)
  const [editingID, setEditingID] = useState<number | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [batchCloneOpen, setBatchCloneOpen] = useState(false)
  const [batchCloneResult, setBatchCloneResult] = useState<RelayAccountBatchCloneResult | null>(null)
  const [busy, setBusy] = useState(false)
  const [autoAdjustEnabled, setAutoAdjustEnabled] = useState(false)
  const [autoAdjustNoProfitEnabled, setAutoAdjustNoProfitEnabled] = useState(false)
  const [autoPriorityEnabled, setAutoPriorityEnabled] = useState(false)
  const [autoPriorityRecallEnabled, setAutoPriorityRecallEnabled] = useState(false)
  const [autoPriorityRecallMinutes, setAutoPriorityRecallMinutes] = useState(180)
  const [accountNameFilter, setAccountNameFilter] = useState("")
  const [accountSearchResetVersion, setAccountSearchResetVersion] = useState(0)
  const [riskFilter, setRiskFilter] = useState<RiskFilter>("all")
  const [modelTypeFilter, setModelTypeFilter] = useState("all")
  const [schedulableFilter, setSchedulableFilter] = useState<"enabled" | "disabled" | "all">("enabled")
  const [groupFilter, setGroupFilter] = useState("all")
  const [selected, setSelected] = useState<number[]>([])
  const [requestedBatchMode, setRequestedBatchMode] = useState<BatchMode>("channel_group")
  const setBatchMode = setRequestedBatchMode
  const [batchChannelID, setBatchChannelID] = useState("")
  const [batchGroup, setBatchGroup] = useState("")
  const [batchMultiplier, setBatchMultiplier] = useState("")
  const [batchRuntimeValue, setBatchRuntimeValue] = useState("")
  const [batchModelType, setBatchModelType] = useState("")
  const [batchGroupDialogOpen, setBatchGroupDialogOpen] = useState(false)
  const [groupEditor, setGroupEditor] = useState<RelayAccountView | null>(null)
  const [schedulingAccountID, setSchedulingAccountID] = useState<number | null>(null)
  const [probingAccountID, setProbingAccountID] = useState<number | null>(null)
  const [testingAccountID, setTestingAccountID] = useState<number | null>(null)
  const [deletingAccountID, setDeletingAccountID] = useState<number | null>(null)
  const [testAccount, setTestAccount] = useState<RelayAccountView | null>(null)
  const [testModels, setTestModels] = useState<string[]>([])
  const [testModel, setTestModel] = useState("")
  const [testMode, setTestMode] = useState("regular")
  const [testModelsLoading, setTestModelsLoading] = useState(false)
  const [testResult, setTestResult] = useState<RelayAccountTestResult | null>(null)
  const [testError, setTestError] = useState("")
  const testOutput = useMemo(() => parseAccountTestOutput(testResult?.output ?? ""), [testResult?.output])
  const [testingGroupID, setTestingGroupID] = useState<number | null>(null)
  const [deletingGroupID, setDeletingGroupID] = useState<number | null>(null)
  const [testGroup, setTestGroup] = useState<RelayGroupView | null>(null)
  const [groupTestModels, setGroupTestModels] = useState<string[]>([])
  const [groupTestModel, setGroupTestModel] = useState("")
  const [groupTestCount, setGroupTestCount] = useState("1")
  const [groupTestModelsLoading, setGroupTestModelsLoading] = useState(false)
  const [groupTestResult, setGroupTestResult] = useState<RelayGroupTestResult | null>(null)
  const [groupTestError, setGroupTestError] = useState("")
  const [groupTestNeedsKey, setGroupTestNeedsKey] = useState(false)
  const [accountSort, setAccountSort] = useState<SortState<AccountSortKey>>({ key: "score", direction: "desc" })
  const [accountListOpen, setAccountListOpen] = usePersistedOpen("uh_relay_account_risk_open")
  const [accountAdjustmentDialog, setAccountAdjustmentDialog] = useState<AccountAdjustmentDialogState>(null)
  const [accountAdjustmentSelections, setAccountAdjustmentSelections] = useState<AccountAdjustmentSelection>({})
  const lastOverviewByStation = useRef(new Map<number, RelayOverview>())
  const accountAdjustmentResolver = useRef<((selection: AccountAdjustmentSelection | null) => void) | null>(null)

  useEffect(() => { if (selectedID == null && stations.data?.[0]) setSelectedID(stations.data[0].id); if (selectedID != null && stations.data && !stations.data.some((station) => station.id === selectedID)) setSelectedID(stations.data[0]?.id ?? null) }, [stations.data, selectedID])
  useEffect(() => { if (!overview.data) return; setAutoAdjustEnabled(overview.data.station.auto_adjust_enabled); setAutoAdjustNoProfitEnabled(overview.data.station.auto_adjust_no_profit_enabled); setAutoPriorityEnabled(overview.data.station.auto_priority_enabled); setAutoPriorityRecallEnabled(overview.data.station.auto_priority_recall_enabled); setAutoPriorityRecallMinutes(overview.data.station.auto_priority_recall_minutes || 180) }, [overview.data])
  useEffect(() => { setSelected([]) }, [selectedID])
  useEffect(() => { setBatchCloneOpen(false); setBatchCloneResult(null) }, [selectedID])
  useEffect(() => { if (selectedID != null && overview.data) lastOverviewByStation.current.set(selectedID, overview.data) }, [overview.data, selectedID])

  const baseOverview = overview.data ?? (selectedID == null ? null : lastOverviewByStation.current.get(selectedID) ?? null)
  const currentOverview = useMemo(() => {
    if (!baseOverview) return null
    const usageByAccount = new Map((usage.data?.accounts ?? []).map((account) => [account.external_id, account]))
    return {
      ...baseOverview,
      accounts: baseOverview.accounts.map((account) => {
        const stats = usageByAccount.get(account.external_id)
        return { ...account, usage_total_tokens: stats?.usage_total_tokens ?? null, user_charge_amount: stats?.user_charge_amount ?? null, account_cost_amount: stats?.account_cost_amount ?? null }
      }),
      range: usage.data?.range ?? usageRange,
      usage_complete: usage.data?.complete ?? false,
      usage_failed_accounts: usage.data?.failed_accounts ?? 0,
    }
  }, [baseOverview, usage.data, usageRange])
  const latencyBucketsByAccount = useMemo(() => new Map((latencyBuckets.data ?? []).map((item) => [item.account_id, item.buckets])), [latencyBuckets.data])
  const scoreLatencyBucketsByAccount = useMemo(() => new Map((scoreLatencyBuckets.data ?? []).map((item) => [item.account_id, item.buckets])), [scoreLatencyBuckets.data])
  const consumptionBucketsByAccount = useMemo(() => new Map((accountConsumptionBuckets.data ?? []).map((item) => [item.account_id, item.buckets])), [accountConsumptionBuckets.data])
  const channelRecentSamplesByChannel = useMemo(() => new Map((channelLatencyTrends.data ?? []).map((item) => [item.channel_id, item.samples])), [channelLatencyTrends.data])
  const channelLatencyBucketsByChannel = useMemo(() => new Map((channelLatencyBuckets.data ?? []).map((item) => [item.channel_id, item.buckets])), [channelLatencyBuckets.data])
  const channelConsumptionBucketsByChannel = useMemo(() => new Map((channelConsumptionBuckets.data ?? []).map((item) => [item.channel_id, item.buckets])), [channelConsumptionBuckets.data])
  const channelLinksByRelayAccount = useMemo(() => channelComparisonsByRelayAccount(channelAssociations.data, channels.data ?? []), [channelAssociations.data, channels.data])
  const stationType = currentOverview?.station.type ?? stations.data?.find((station) => station.id === selectedID)?.type ?? "sub2api"
  const batchMode = relayBatchModeForStation(stationType, requestedBatchMode)
  useEffect(() => {
    setRequestedBatchMode((current) => relayBatchModeForStation(stationType, current))
  }, [stationType])
  const capabilities = relayStationCapabilities(stationType)
  const visibleAccountColumns = useMemo(() => new Set(stationAccountColumns(stationType).map((column) => column.key).filter((key) => storedAccountColumns.has(key))), [stationType, storedAccountColumns])
  const accountTableGridStyle = useMemo(() => accountGridStyle(visibleAccountColumns), [visibleAccountColumns])
  const managedAccounts = useMemo(() => (currentOverview?.accounts ?? []).filter((account) => account.type?.trim().toLowerCase() !== "oauth"), [currentOverview])
  const healthScoreByAccount = useMemo(() => {
    const scores = new Map<number, LatencyHealthScore | null>()
    for (const account of managedAccounts) {
      scores.set(account.external_id, latencyHealthScore(account.latency_samples ?? [], scoreLatencyBucketsByAccount.get(account.external_id) ?? []))
    }
    return scores
  }, [managedAccounts, scoreLatencyBucketsByAccount])
  const latestCallByAccount = useMemo(() => {
    const latest = new Map<number, number>()
    for (const account of managedAccounts) {
      for (const sample of account.latency_samples ?? []) {
        const at = Date.parse(sample.created_at)
        if (!Number.isFinite(at)) continue
        if (at > (latest.get(account.external_id) ?? 0)) latest.set(account.external_id, at)
      }
    }
    return latest
  }, [managedAccounts])
  const cloneableAccounts = useMemo(() => managedAccounts.filter((account) => batchCloneableAccountTypes.has(account.type?.trim().toLowerCase() ?? "")), [managedAccounts])
  const managedAccountIDs = useMemo(() => new Set(managedAccounts.map((account) => account.external_id)), [managedAccounts])
  const usageTotals = useMemo(() => (usage.data?.accounts ?? []).filter((account) => managedAccountIDs.has(account.external_id)).reduce((totals, account) => ({
    tokens: totals.tokens + account.usage_total_tokens,
    charge: totals.charge + account.user_charge_amount,
  }), { tokens: 0, charge: 0 }), [managedAccountIDs, usage.data?.accounts])
  const selectedStation = useMemo(() => stations.data?.find((station) => station.id === selectedID) ?? null, [stations.data, selectedID])
  const groupAPIKeysURL = useMemo(() => {
    const baseURL = selectedStation?.base_url?.trim()
    if (!baseURL) return null
    try {
      return new URL(stationType === "newapi" ? "/console/token" : "/keys", baseURL).toString()
    } catch {
      return null
    }
  }, [selectedStation?.base_url, stationType])
  const accountModelTypeOptions = useMemo(() => Array.from(new Set(managedAccounts.map((account) => account.model_type?.trim()).filter(Boolean) as string[])).sort((a, b) => a.localeCompare(b, "zh-CN")), [managedAccounts])
  const supportedModelsByAccount = useMemo(() => new Map((accountModelCatalog.data ?? []).map((item) => [item.account_id, item.models])), [accountModelCatalog.data])
  const accountSearchIndex = useMemo(() => new Map(managedAccounts.map((account) => [
    account.external_id,
    relayAccountSearchText(account, supportedModelsByAccount.get(account.external_id), accountURLHost(account.base_url)),
  ])), [managedAccounts, supportedModelsByAccount])
  const activeAccountFilterCount = Number(modelTypeFilter !== "all") + Number(schedulableFilter !== "all") + Number(riskFilter !== "all") + Number(groupFilter !== "all")
  const hasActiveAccountFilters = activeAccountFilterCount > 0 || accountNameFilter.trim() !== ""
  const filteredAccounts = useMemo(() => {
    const query = accountNameFilter.trim().toLocaleLowerCase()
    const accounts = managedAccounts.filter((account) => {
      const searchMatches = query === "" || (accountSearchIndex.get(account.external_id) ?? "").includes(query)
      if (!searchMatches) return false
      if (modelTypeFilter === "__unassigned" && account.model_type?.trim()) return false
      if (modelTypeFilter !== "all" && modelTypeFilter !== "__unassigned" && account.model_type?.trim() !== modelTypeFilter) return false
      if (schedulableFilter === "enabled" && !account.schedulable) return false
      if (schedulableFilter === "disabled" && account.schedulable) return false
      if (riskFilter === "adjustable" && !account.can_apply) return false
      if (riskFilter === "downgradable" && accountDowngradeGroups(account).length === 0) return false
      if (riskFilter !== "all" && riskFilter !== "adjustable" && riskFilter !== "downgradable" && account.risk_state !== riskFilter) return false
      if (groupFilter !== "all" && !account.current_groups.some((group) => String(group.external_id) === groupFilter)) return false
      return true
    })
    if (!accountSort) return accounts
    const compareNumber = (left: number | null, right: number | null) => {
      if (left == null && right == null) return 0
      if (left == null) return 1
      if (right == null) return -1
      const result = left - right
      return accountSort.direction === "asc" ? result : -result
    }
    return [...accounts].sort((a, b) => {
      let result = 0
      switch (accountSort.key) {
        case "score":
          result = compareNumber(healthScoreByAccount.get(a.external_id)?.score ?? null, healthScoreByAccount.get(b.external_id)?.score ?? null)
          break
        case "recent_call":
          result = compareNumber(latestCallByAccount.get(a.external_id) ?? null, latestCallByAccount.get(b.external_id) ?? null)
          break
        case "last_used": {
          const left = a.last_used_at ? Date.parse(a.last_used_at) : Number.NaN
          const right = b.last_used_at ? Date.parse(b.last_used_at) : Number.NaN
          result = compareNumber(Number.isFinite(left) ? left : null, Number.isFinite(right) ? right : null)
          break
        }
        case "priority":
          result = compareNumber(a.priority, b.priority)
          break
        case "cost":
          result = compareNumber(a.cost_multiplier ?? null, b.cost_multiplier ?? null)
          break
        case "usage":
          result = compareNumber(a.user_charge_amount ?? null, b.user_charge_amount ?? null)
          if (result === 0) result = compareNumber(a.usage_total_tokens ?? null, b.usage_total_tokens ?? null)
          break
        case "name":
          result = a.name.localeCompare(b.name, "zh-CN", { numeric: true, sensitivity: "base" }) * (accountSort.direction === "asc" ? 1 : -1)
          break
      }
      return result || a.external_id - b.external_id
    })
  }, [managedAccounts, accountNameFilter, modelTypeFilter, schedulableFilter, riskFilter, groupFilter, accountSort, healthScoreByAccount, latestCallByAccount, accountSearchIndex])
  const allFilteredSelected = filteredAccounts.length > 0 && filteredAccounts.every((account) => selected.includes(account.external_id))
  const filteredEnableCount = filteredAccounts.filter((account) => !account.schedulable).length
  const filteredDisableCount = filteredAccounts.filter((account) => account.schedulable).length
  const filteredSuggestionAccounts = useMemo(() => filteredAccounts.filter((account) => account.can_apply), [filteredAccounts])
  const filteredDowngradeAccounts = useMemo(() => filteredAccounts.filter((account) => accountDowngradeGroups(account).length > 0), [filteredAccounts])
  const filteredSuggestionCount = filteredSuggestionAccounts.length
  const filteredDowngradeCount = filteredDowngradeAccounts.length
  const groupsByID = useMemo(() => new Map((currentOverview?.groups ?? []).map((group) => [group.external_id, group])), [currentOverview?.groups])
  const batchGroups = currentOverview?.monitor_channels.find((channel) => String(channel.id) === batchChannelID)?.groups ?? []
  const modelTypeOptions = useMemo(() => Array.from(new Set((currentOverview?.groups ?? []).flatMap((group) => group.model_types ?? []))).sort(), [currentOverview?.groups])

  function openAccountAdjustmentDialog(options: NonNullable<AccountAdjustmentDialogState>) {
    return new Promise<AccountAdjustmentSelection | null>((resolve) => {
      accountAdjustmentResolver.current?.(null)
      accountAdjustmentResolver.current = resolve
      setAccountAdjustmentSelections(defaultAccountAdjustmentSelections(options.mode, options.accounts, groupsByID))
      setAccountAdjustmentDialog(options)
    })
  }

  function finishAccountAdjustmentDialog(selection: AccountAdjustmentSelection | null) {
    const resolve = accountAdjustmentResolver.current
    accountAdjustmentResolver.current = null
    setAccountAdjustmentDialog(null)
    resolve?.(selection)
  }

  function updateAccountAdjustmentSelection(accountID: number, groupIDs: number[]) {
    setAccountAdjustmentSelections((current) => ({ ...current, [accountID]: [...new Set(groupIDs)] }))
  }

  async function reload() { await Promise.all([stations.refetch(), overview.refetch(), usage.refetch(), recentUsage.refetch()]) }
  function openCreate() { setEditingID(null); setShowForm(true) }
  function openBatchClone() { if (cloneableAccounts.length === 0) { toast.info("当前没有支持克隆的账号"); return }; setBatchCloneResult(null); setBatchCloneOpen(true) }
  function closeBatchClone() { if (busy) return; setBatchCloneOpen(false); setBatchCloneResult(null) }
  function openEdit() { if (selectedStation) { setEditingID(selectedStation.id); setShowForm(true) } }
  async function saveStation(form: StationForm) { setBusy(true); try { const payload = buildRelayStationPayload(form, stations.data?.find((station) => station.id === editingID)); const station = await apiFetch<RelayStation>(editingID == null ? "/relay-stations" : `/relay-stations/${editingID}`, { method: editingID == null ? "POST" : "PUT", body: JSON.stringify(payload) }); setShowForm(false); setSelectedID(station.id); await reload(); toast.success(editingID == null ? "中转站已添加" : "中转站配置已更新") } catch (error) { toast.error(relayStationErrorMessage(form.type, "save", error instanceof Error ? error.message : "保存失败")) } finally { setBusy(false) } }
  async function sync() { if (!selectedID) return; setBusy(true); try { await apiFetch(`/relay-stations/${selectedID}/sync`, { method: "POST" }); await reload(); toast.success(stationType === "newapi" ? "NewAPI 账号、分组与成本数据已同步" : "已实时探测 API Key 成本并刷新账号快照") } catch (error) { toast.error(relayStationErrorMessage(stationType, "sync", error instanceof Error ? error.message : "同步失败")); await reload() } finally { setBusy(false) } }
  async function syncAll() { setBusy(true); try { const result = await apiFetch<{ synced: number; failed: number }>("/relay-stations/sync-all", { method: "POST" }); await reload(); toast.success(`已同步 ${result.synced} 个中转站${result.failed ? `，${result.failed} 个失败` : ""}`) } catch (error) { toast.error(error instanceof Error ? error.message : "同步失败") } finally { setBusy(false) } }
  function changeUsageRange(nextRange: RelayUsageRange) {
    setUsageRange(nextRange)
    setLatencyGranularity((current) => normalizeLatencyGranularity(nextRange, current))
  }
  function resetFilters() { setAccountNameFilter(""); setAccountSearchResetVersion((value) => value + 1); setModelTypeFilter("all"); setSchedulableFilter("all"); setRiskFilter("all"); setGroupFilter("all") }
  async function savePolicy() { if (!selectedID) return; setBusy(true); try { await apiFetch(`/relay-stations/${selectedID}`, { method: "PUT", body: JSON.stringify({ auto_adjust_enabled: autoAdjustEnabled, auto_adjust_no_profit_enabled: autoAdjustNoProfitEnabled }) }); await reload(); toast.success("自动调组策略已保存") } catch (error) { toast.error(error instanceof Error ? error.message : "保存失败") } finally { setBusy(false) } }
  async function savePriorityPolicy() { if (!selectedID) return; setBusy(true); try { await apiFetch(`/relay-stations/${selectedID}`, { method: "PUT", body: JSON.stringify({ auto_priority_enabled: autoPriorityEnabled, auto_priority_recall_enabled: autoPriorityRecallEnabled, auto_priority_recall_minutes: autoPriorityRecallMinutes }) }); await reload(); toast.success("自动优先级策略已保存") } catch (error) { toast.error(error instanceof Error ? error.message : "保存失败") } finally { setBusy(false) } }
  async function updateGroup(groupID: number, patch: RelayGroupPatch) { if (!selectedID) return; await apiFetch(`/relay-stations/${selectedID}/groups/${groupID}`, { method: "PUT", body: JSON.stringify(patch) }); await reload(); toast.success("分组已更新") }
  async function updateGroupOrder(ids: number[]) { if (!selectedID) return; await apiFetch(`/relay-stations/${selectedID}/groups/sort-order`, { method: "PUT", body: JSON.stringify({ updates: ids.map((id, index) => ({ id, sort_order: index * 10 })) }) }); await reload(); toast.success("分组排序已保存") }
  async function applySuggestion(account: RelayAccountView) { if (!selectedID || !account.can_apply) return; const selection = await openAccountAdjustmentDialog({ mode: "suggestion", title: `应用“${account.name}”的调组建议？`, confirmLabel: "应用建议", accounts: [account], totalCount: 1 }); if (!selection) return; setBusy(true); try { const result = await apiFetch<RelayAccountBatchActionResult>(`/relay-stations/${selectedID}/accounts/apply-suggestions`, { method: "POST", body: JSON.stringify({ account_external_ids: [account.external_id], account_group_external_ids: selection }) }); if (result.failed) throw new Error(result.errors?.[0] || "应用建议失败"); await reload(); toast.success(result.applied > 0 ? "账号分组已更新并写入审计记录" : "未选择可应用分组，已跳过") } catch (error) { toast.error(error instanceof Error ? error.message : "应用建议失败") } finally { setBusy(false) } }
  async function addDowngradeGroups(account: RelayAccountView) { const groups = accountDowngradeGroups(account); if (!selectedID || groups.length === 0) return; const selection = await openAccountAdjustmentDialog({ mode: "downgrade", title: `为“${account.name}”追加降价分组？`, confirmLabel: "追加降价分组", accounts: [account], totalCount: 1 }); if (!selection) return; setBusy(true); try { const result = await apiFetch<RelayAccountBatchActionResult>(`/relay-stations/${selectedID}/accounts/add-downgrades`, { method: "POST", body: JSON.stringify({ account_external_ids: [account.external_id], account_group_external_ids: selection }) }); if (result.failed) throw new Error(result.errors?.[0] || "加入可降价分组失败"); const selectedGroups = selectedCandidateGroups(groups, selection[account.external_id] ?? []); await reload(); toast.success(result.applied > 0 ? `已加入 ${selectedGroups.length} 个可降价分组：${selectedGroups.map((group) => group.name).join("、")}；原分组保持不变` : "未选择新增降价分组，已跳过") } catch (error) { toast.error(error instanceof Error ? error.message : "加入可降价分组失败") } finally { setBusy(false) } }
  async function saveGroups(ids: number[]) { if (!selectedID || !groupEditor) return; setBusy(true); try { await apiFetch(`/relay-stations/${selectedID}/accounts/${groupEditor.external_id}/groups`, { method: "PUT", body: JSON.stringify({ group_external_ids: ids }) }); setGroupEditor(null); await reload(); toast.success("账号分组已手动更新") } catch (error) { toast.error(error instanceof Error ? error.message : "更新分组失败") } finally { setBusy(false) } }
  async function setSchedulable(account: RelayAccountView, schedulable: boolean) { if (!selectedID) return; setSchedulingAccountID(account.external_id); try { await apiFetch(`/relay-stations/${selectedID}/accounts/${account.external_id}/schedulable`, { method: "PUT", body: JSON.stringify({ schedulable }) }); await reload(); toast.success(schedulable ? "账号调度已开启" : "账号调度已关闭") } catch (error) { toast.error(error instanceof Error ? error.message : "更新调度状态失败") } finally { setSchedulingAccountID(null) } }
  async function probeAccount(account: RelayAccountView) { if (!selectedID) return; setProbingAccountID(account.external_id); try { await apiFetch(`/relay-stations/${selectedID}/accounts/${account.external_id}/probe`, { method: "POST" }); await reload(); toast.success(account.cost_override_mode ? "上游倍率已探测，当前手工成本覆盖继续优先" : "上游倍率已立即探测并刷新") } catch (error) { toast.error(error instanceof Error ? error.message : "上游倍率探测失败") } finally { setProbingAccountID(null) } }
  async function openAccountTest(account: RelayAccountView) { if (!selectedID) return; setTestAccount(account); setTestModels([]); setTestModel(""); setTestMode("regular"); setTestResult(null); setTestError(""); setTestModelsLoading(true); try { const models = await apiFetch<string[]>(`/relay-stations/${selectedID}/accounts/${account.external_id}/models`); setTestModels(models); setTestModel(models[0] ?? "") } catch (error) { setTestError(error instanceof Error ? error.message : "读取测试模型失败") } finally { setTestModelsLoading(false) } }
  async function runAccountTest() { if (!selectedID || !testAccount || !testModel) return; setTestResult(null); setTestError(""); setTestingAccountID(testAccount.external_id); try { const result = await apiFetch<RelayAccountTestResult>(`/relay-stations/${selectedID}/accounts/${testAccount.external_id}/test`, { method: "POST", body: JSON.stringify({ model_id: testModel, mode: testMode }) }); setTestResult(result); toast.success("账号连接测试成功") } catch (error) { setTestError(error instanceof Error ? error.message : "账号连接测试失败") } finally { setTestingAccountID(null) } }
  async function openGroupTest(group: RelayGroupView) { if (!selectedID) return; setTestGroup(group); setGroupTestModels([]); setGroupTestModel(""); setGroupTestCount("1"); setGroupTestResult(null); setGroupTestError(""); setGroupTestNeedsKey(false); setGroupTestModelsLoading(true); try { const models = await apiFetch<string[]>(`/relay-stations/${selectedID}/groups/${group.external_id}/models`); setGroupTestModels(models); setGroupTestModel(models[0] ?? "") } catch (error) { setGroupTestNeedsKey(isMissingGroupAPIKeyError(error)); setGroupTestError(relayStationErrorMessage(stationType, "group-models", error instanceof Error ? error.message : "读取分组模型失败")) } finally { setGroupTestModelsLoading(false) } }
  async function runGroupTest() { if (!selectedID || !testGroup || !groupTestModel) return; const count = Number(groupTestCount); if (!Number.isInteger(count) || count < 1 || count > 10) { setGroupTestError("调用次数必须是 1 到 10 次"); return } setGroupTestResult(null); setGroupTestError(""); setGroupTestNeedsKey(false); setTestingGroupID(testGroup.external_id); try { const result = await apiFetch<RelayGroupTestResult>(`/relay-stations/${selectedID}/groups/${testGroup.external_id}/test`, { method: "POST", body: JSON.stringify({ model: groupTestModel, count }) }); setGroupTestResult(result); await recentUsage.refetch(); if (result.failed === 0) toast.success(`${result.succeeded} 次真实调用均成功`); else toast.error(`测试完成：成功 ${result.succeeded} 次，失败 ${result.failed} 次`) } catch (error) { setGroupTestNeedsKey(isMissingGroupAPIKeyError(error)); setGroupTestError(relayStationErrorMessage(stationType, "group-test", error instanceof Error ? error.message : "分组快速测试失败")) } finally { setTestingGroupID(null) } }
  async function deleteGroup(group: RelayGroupView) { if (!selectedID) return; const accepted = await confirm({ title: `删除分组“${group.name}”？`, description: `将从当前中转站真实删除该分组，操作不可恢复。${group.account_count > 0 ? `当前关联 ${group.account_count} 个账号，删除后这些账号的分组配置可能受到影响。` : ""}`, confirmLabel: "确认删除", destructive: true }); if (!accepted) return; setDeletingGroupID(group.external_id); try { await apiFetch(`/relay-stations/${selectedID}/groups/${group.external_id}`, { method: "DELETE" }); if (testGroup?.external_id === group.external_id) setTestGroup(null); await reload(); toast.success(`分组“${group.name}”已删除`) } catch (error) { toast.error(error instanceof Error ? error.message : "删除分组失败") } finally { setDeletingGroupID(null) } }
  async function deleteAccount(account: RelayAccountView) { if (!selectedID) return; const accepted = await confirm({ title: `删除账号“${account.name}”？`, description: "将从当前中转站真实删除该账号及其调度配置，操作不可恢复；历史使用记录不会被删除。", confirmLabel: "确认删除", destructive: true }); if (!accepted) return; setDeletingAccountID(account.external_id); try { await apiFetch(`/relay-stations/${selectedID}/accounts/${account.external_id}`, { method: "DELETE" }); setSelected((current) => current.filter((id) => id !== account.external_id)); if (groupEditor?.external_id === account.external_id) setGroupEditor(null); if (testAccount?.external_id === account.external_id) setTestAccount(null); await reload(); toast.success(`账号“${account.name}”已删除`) } catch (error) { toast.error(error instanceof Error ? error.message : "删除账号失败") } finally { setDeletingAccountID(null) } }
  async function refreshAccounts() { await Promise.all([overview.refetch(), usage.refetch(), scoreLatencyBuckets.refetch(), latencyBuckets.refetch(), accountConsumptionBuckets.refetch()]); toast.success("账号列表已刷新") }
  async function refreshGroups() { await overview.refetch(); toast.success("分组列表已刷新") }
  async function saveBatchOverride(clear = false) { if (!selectedID || selected.length === 0) return; if (batchMode === "groups") { setBatchGroupDialogOpen(true); return } if (!clear && batchMode === "channel_group" && (!batchChannelID || !batchGroup)) { toast.error("请选择渠道和渠道分组"); return } if (!clear && batchMode === "manual" && (!batchMultiplier || !Number.isFinite(Number(batchMultiplier)) || Number(batchMultiplier) < 0)) { toast.error("手工倍率必须是非负数"); return } setBusy(true); try { await apiFetch(`/relay-stations/${selectedID}/accounts/cost-overrides`, { method: "PUT", body: JSON.stringify({ account_external_ids: selected, clear, mode: batchMode, monitor_channel_id: batchChannelID ? Number(batchChannelID) : undefined, upstream_group: batchGroup, manual_multiplier: batchMode === "manual" ? Number(batchMultiplier) : undefined }) }); setSelected([]); await reload(); toast.success(clear ? "已清除账号成本覆盖" : "账号成本覆盖已保存") } catch (error) { toast.error(error instanceof Error ? error.message : "保存成本覆盖失败") } finally { setBusy(false) } }
  async function submitBatchClone(groups: RelayAccountBatchCloneGroup[]) { if (!selectedID) return; setBusy(true); try { const result = await apiFetch<RelayAccountBatchCloneResult>(`/relay-stations/${selectedID}/accounts/batch-clone`, { method: "POST", body: JSON.stringify({ groups }) }); setBatchCloneResult(result); await reload(); if (result.sync_error) toast.warning(`已新增 ${result.succeeded} 个账号，但本地快照刷新失败`); else if (result.failed) toast.warning(`批量新增完成：成功 ${result.succeeded} 个，失败 ${result.failed} 个`); else toast.success(`已批量新增 ${result.succeeded} 个账号`) } catch (error) { toast.error(error instanceof Error ? error.message : "批量新增账号失败") } finally { setBusy(false) } }
  async function saveBatchRuntimeSettings() { if (!selectedID || selected.length === 0 || (batchMode !== "concurrency" && batchMode !== "priority" && batchMode !== "retry_count")) return; const value = Number(batchRuntimeValue); if (batchMode === "retry_count") { if (!Number.isInteger(value) || value < 0 || value > 10) { toast.error("同账号重试次数必须是 0 到 10 的整数"); return } const selectedAccounts = currentOverview?.accounts.filter((account) => selected.includes(account.external_id)) ?? []; const unsupported = selectedAccounts.find((account) => !["apikey", "bedrock"].includes((account.type || "").toLowerCase())); if (unsupported) { toast.error(`账号“${unsupported.name}”不支持同账号重试次数，仅支持 API Key 或 Bedrock 账号`); return } const poolModeOff = selectedAccounts.find((account) => account.pool_mode !== true); if (poolModeOff) { toast.error(`账号“${poolModeOff.name}”未开启池模式，请先在 Sub2API 账号编辑中开启池模式`); return } } else { if (!Number.isInteger(value) || value < 1 || value > 1000) { toast.error(`${batchMode === "concurrency" ? "并发数" : "优先级"}必须是 1 到 1000 的整数`); return } if (batchMode === "priority" && currentOverview?.accounts.some((account) => selected.includes(account.external_id) && relayPriorityRequiresOAuth(stationType, value, account.type))) { toast.error("优先级 1 仅保留给 OAuth 账号"); return } } setBusy(true); try { const field = batchMode === "retry_count" ? "pool_mode_retry_count" : batchMode; await apiFetch(`/relay-stations/${selectedID}/accounts/runtime-settings`, { method: "PUT", body: JSON.stringify({ account_external_ids: selected, [field]: value }) }); setSelected([]); setBatchRuntimeValue(""); await reload(); toast.success(`已批量设置账号${batchMode === "concurrency" ? "并发数" : batchMode === "priority" ? "优先级" : "同账号重试次数"}`) } catch (error) { toast.error(error instanceof Error ? error.message : "批量设置失败") } finally { setBusy(false) } }
  async function saveBatchModelType() { if (!selectedID || selected.length === 0) return; setBusy(true); try { await apiFetch(`/relay-stations/${selectedID}/accounts/model-types`, { method: "PUT", body: JSON.stringify({ account_external_ids: selected, model_type: batchModelType }) }); setSelected([]); setBatchModelType(""); await reload(); toast.success("已批量设置账号模型类型") } catch (error) { toast.error(error instanceof Error ? error.message : "批量设置模型类型失败") } finally { setBusy(false) } }
  async function saveBatchGroups(ids: number[]) { if (!selectedID || selected.length === 0) return; if (ids.length === 0) { toast.error("至少选择一个销售分组"); return } setBusy(true); try { await apiFetch(`/relay-stations/${selectedID}/accounts/groups`, { method: "PUT", body: JSON.stringify({ account_external_ids: selected, group_external_ids: ids }) }); setBatchGroupDialogOpen(false); setSelected([]); await reload(); toast.success("已批量调整账号销售分组") } catch (error) { toast.error(error instanceof Error ? error.message : "批量调整分组失败") } finally { setBusy(false) } }
  async function runFilteredAccountAction(path: string, schedulable: boolean | undefined, successLabel: string) { if (!selectedID || filteredAccounts.length === 0) { toast.info("当前筛选没有账号"); return } let accountGroupExternalIDs: AccountAdjustmentSelection | undefined; if (path === "apply-suggestions" || path === "add-downgrades") { const previewAccounts = path === "apply-suggestions" ? filteredSuggestionAccounts : filteredDowngradeAccounts; if (previewAccounts.length === 0) { toast.info(path === "apply-suggestions" ? "当前筛选没有可应用建议的账号" : "当前筛选没有可降价账号"); return } const selection = await openAccountAdjustmentDialog({ mode: path === "apply-suggestions" ? "suggestion" : "downgrade", title: path === "apply-suggestions" ? `接受 ${previewAccounts.length} 个账号的推荐调组？` : `接受 ${previewAccounts.length} 个账号的推荐降价？`, confirmLabel: path === "apply-suggestions" ? "接受推荐调组" : "接受推荐降价", accounts: previewAccounts, totalCount: filteredAccounts.length }); if (!selection) return; accountGroupExternalIDs = selection } setBusy(true); try { const result = await apiFetch<RelayAccountBatchActionResult>(`/relay-stations/${selectedID}/accounts/${path}`, { method: path === "schedulable" ? "PUT" : "POST", body: JSON.stringify({ account_external_ids: filteredAccounts.map((account) => account.external_id), ...(path === "schedulable" ? { schedulable } : {}), ...(accountGroupExternalIDs ? { account_group_external_ids: accountGroupExternalIDs } : {}) }) }); setSelected([]); await reload(); const detail = result.failed ? `，失败 ${result.failed} 个${result.errors?.[0] ? `：${result.errors[0]}` : ""}` : ""; const message = `${successLabel}：已处理 ${result.applied} 个，跳过 ${result.skipped} 个${detail}`; if (result.failed) toast.warning(message); else toast.success(message) } catch (error) { toast.error(error instanceof Error ? error.message : `${successLabel}失败`) } finally { setBusy(false) } }
  async function remove() { if (!selectedID || !selectedStation) return; const accepted = await confirm({ title: `删除中转站“${selectedStation.name}”？`, description: "将从系统真实删除该中转站及其账号快照、调整记录，操作不可恢复；历史使用记录不会被删除。", confirmLabel: "确认删除", destructive: true }); if (!accepted) return; setBusy(true); try { await apiFetch(`/relay-stations/${selectedID}`, { method: "DELETE" }); setSelectedID(null); await reload(); toast.success("中转站已删除") } catch (error) { toast.error(error instanceof Error ? error.message : "删除失败") } finally { setBusy(false) } }

  return <section className="space-y-4"><header className="list-panel-controls flex flex-wrap items-end justify-between gap-3 border-l-2 border-brand pl-3"><div><h1 className="flex items-center gap-2 text-xl font-bold text-foreground"><Server className="size-5 text-brand" />中转站管理</h1><p className="mt-1 text-xs text-muted-foreground">Sub2API 同步时自动探测上游倍率；New API 成本通过渠道关联或手工倍率配置。</p></div><div className="flex flex-wrap items-center gap-2"><Button asChild variant="outline" size="sm" className="h-11 gap-1.5 sm:h-9"><a href="/settings#background-tasks" title="前往系统设置修改全部中转站的同步任务"><Settings2 className="size-3.5" />全局同步设置</a></Button><Button variant="outline" size="sm" className="h-11 gap-1.5 sm:h-9" onClick={openCreate}><Plus className="size-3.5" />添加中转站</Button>{stations.data?.length ? <Button variant="outline" size="sm" className="h-11 gap-1.5 sm:h-9" disabled={busy} onClick={() => void syncAll()}><RefreshCw className={cn("size-3.5", busy && "animate-spin")} />同步全部</Button> : null}{selectedStation ? <><Button asChild variant="outline" size="sm" className="h-11 gap-1.5 sm:h-9"><a href={`/public/relay-monitor/${selectedStation.id}`} target="_blank" rel="noopener noreferrer"><Activity className="size-3.5" />分组监控</a></Button><Button asChild variant="outline" size="sm" className="h-11 gap-1.5 sm:h-9"><a href={`/public/model-pricing/${selectedStation.id}`} target="_blank" rel="noopener noreferrer"><CircleDollarSign className="size-3.5" />模型价格</a></Button><Button variant="outline" size="sm" className="h-11 sm:h-9" onClick={openEdit}>编辑</Button><Button size="sm" className="h-11 gap-1.5 sm:h-9" disabled={busy || !selectedStation.api_key_configured} onClick={() => void sync()}><RefreshCw className={cn("size-3.5", busy && "animate-spin")} />实时同步</Button><Button variant="ghost" size="icon" className="size-11 sm:size-9" aria-label="删除中转站" disabled={busy} onClick={() => void remove()}><Trash2 className="size-4 text-danger" /></Button></> : null}</div></header>

    {showForm ? <RelayStationEditor key={editingID ?? "new"} station={stations.data?.find((station) => station.id === editingID)} busy={busy} onSave={saveStation} onCancel={() => setShowForm(false)} /> : null}

    {stations.loading && !stations.data ? <div className="py-10 text-center text-sm text-muted-foreground">正在读取中转站配置...</div> : stations.data?.length ? <div><div className="mb-3 flex max-w-full items-center gap-2 overflow-x-auto border-b border-border pb-1 [scrollbar-width:thin]">{stations.data.map((station) => <button key={station.id} type="button" onClick={() => setSelectedID(station.id)} title={`${station.name} · ${station.base_url}`} className={cn("inline-flex shrink-0 cursor-pointer items-center gap-2 border-b-2 px-3 py-2 text-left text-sm transition-colors duration-200", selectedID === station.id ? "border-foreground font-medium text-foreground" : "border-transparent text-muted-foreground hover:border-border hover:text-foreground")}><span className="max-w-48 truncate">{station.name}</span><span data-station-type={station.type ?? "sub2api"} className="rounded border border-border px-1.5 py-0.5 text-[10px]">{station.type === "newapi" ? "NewAPI" : "Sub2API"}</span><span className={cn("size-1.5 rounded-full", station.last_error ? "bg-danger" : station.api_key_configured ? "bg-success" : "bg-warning")} /></button>)}</div><div className="min-w-0">{overview.loading && !currentOverview ? <div className="py-10 text-center text-sm text-muted-foreground">正在读取账号快照...</div> : currentOverview ? <div className="space-y-3">{overview.error ? <Alert variant="destructive"><CircleAlert /><AlertTitle>刷新失败，正在显示上次数据</AlertTitle><AlertDescription>{overview.error}</AlertDescription></Alert> : null}{currentOverview.station.last_error ? <Alert variant="destructive"><CircleAlert /><AlertTitle>最近同步失败</AlertTitle><AlertDescription>{currentOverview.station.last_error}</AlertDescription></Alert> : null}{!currentOverview.station.last_synced_at ? <Alert><CircleAlert /><AlertTitle>还没有账号快照</AlertTitle><AlertDescription>实时同步后将读取远端账号、成本倍率与分组关联。</AlertDescription></Alert> : null}<RiskSummary overview={currentOverview} /><RelayDailyConsumptionTrend data={dailyConsumption.data ?? []} loading={dailyConsumption.loading} />

      <div className="grid gap-3 xl:grid-cols-2">
        <Card className="gap-0 border border-border py-0 shadow-none">
          <CardHeader className="flex flex-row items-center justify-between gap-3 px-4 py-3">
            <div className="min-w-0"><CardTitle className="flex items-center gap-2 text-sm font-semibold"><Cog className="size-4 text-brand" />自动调组策略</CardTitle><p className="mt-1 truncate text-xs text-muted-foreground">分别控制亏损风险和无利润账号的自动处理。</p></div>
            <Button size="sm" className="h-9 shrink-0 gap-1.5" disabled={busy} onClick={() => void savePolicy()}><Save className="size-3.5" />保存</Button>
          </CardHeader>
          <CardContent className="grid divide-y divide-border border-t border-border px-4 md:grid-cols-2 md:divide-x md:divide-y-0">
            <div className="flex min-h-11 items-center justify-between gap-6 py-4 md:pr-6"><div className="min-w-0"><p className="truncate text-xs font-medium">亏损风险</p><p className="mt-1 flex min-w-0 items-center gap-1 text-[11px] text-muted-foreground"><span className="min-w-0 flex-1 truncate">成本高于销售倍率时调整</span><SyncPlanHelp label="亏损风险" help="当账号成本倍率高于当前销售倍率时，自动处理该账号，避免继续承担亏损。" /></p></div><Switch className="relative shrink-0 cursor-pointer before:absolute before:-inset-3 before:content-['']" checked={autoAdjustEnabled} onCheckedChange={setAutoAdjustEnabled} aria-label="自动处理亏损风险" /></div>
            <div className="flex min-h-11 items-center justify-between gap-6 py-4 md:pl-6"><div className="min-w-0"><p className="truncate text-xs font-medium">无利润分组</p><p className="mt-1 flex min-w-0 items-center gap-1 text-[11px] text-muted-foreground"><span className="min-w-0 flex-1 truncate">销售倍率与成本完全相同时调整</span><SyncPlanHelp label="无利润分组" help="当账号销售倍率与成本倍率完全相同时，自动处理无利润账号。" /></p></div><Switch className="relative shrink-0 cursor-pointer before:absolute before:-inset-3 before:content-['']" checked={autoAdjustNoProfitEnabled} onCheckedChange={setAutoAdjustNoProfitEnabled} aria-label="自动处理无利润分组" /></div>
          </CardContent>
        </Card>
        <Card className="gap-0 border border-border py-0 shadow-none">
          <CardHeader className="flex flex-row items-center justify-between gap-3 px-4 py-3">
            <div className="min-w-0"><CardTitle className="flex items-center gap-2 text-sm font-semibold"><SlidersHorizontal className="size-4 text-brand" />自动优先级</CardTitle><p className="mt-1 truncate text-xs text-muted-foreground">按最近调用流畅度自动分配调度优先级。</p></div>
            <Button size="sm" className="h-9 shrink-0 gap-1.5" disabled={busy} onClick={() => void savePriorityPolicy()}><Save className="size-3.5" />保存</Button>
          </CardHeader>
          <CardContent className="grid divide-y divide-border border-t border-border px-4 md:grid-cols-2 md:divide-x md:divide-y-0">
            <div className="flex min-h-11 items-center justify-between gap-4 py-4 md:pr-6"><div className="min-w-0"><p className="whitespace-nowrap text-xs font-medium">流畅度排序</p><p className="mt-1 flex items-center gap-1 truncate text-[11px] text-muted-foreground">按近期响应排序<Tooltip delayDuration={200}><TooltipTrigger asChild><button type="button" className="shrink-0 rounded text-muted-foreground hover:text-foreground" aria-label="查看流畅度排序说明"><CircleHelp className="size-3.5" /></button></TooltipTrigger><TooltipContent side="top" className="max-w-72 text-xs">OAuth 账号固定为优先级 1；未调用及响应最流畅的 API Key 从优先级 2 开始，按最近调用的首字与总耗时综合排序，最慢逐级排到 10。</TooltipContent></Tooltip></p></div><Switch className="relative shrink-0 cursor-pointer before:absolute before:-inset-3 before:content-['']" checked={autoPriorityEnabled} onCheckedChange={setAutoPriorityEnabled} aria-label="按最近调用流畅度自动调整优先级" /></div>
            <div className="flex min-w-0 min-h-11 items-center gap-2 py-4 md:pl-6"><Switch className="shrink-0" checked={autoPriorityRecallEnabled} onCheckedChange={setAutoPriorityRecallEnabled} disabled={!autoPriorityEnabled} aria-label="启用自动回调优先级" /><CompactPolicySelect label="自动回调优先级" help="已启用调度的 API Key 账号若在设定时间内未被调用，每次快照同步会将当前优先级提高一级，最低提升到按流畅度计算出的基础优先级 2；账号重新产生调用后继续按近期表现排序。" value={autoPriorityRecallMinutes} options={[30, 60, 180, 360, 720, 1440]} formatOption={(value) => ({ 30: "30 分", 60: "1 小时", 180: "3 小时", 360: "6 小时", 720: "12 小时", 1440: "1 天" })[value] ?? "未设置"} onValueChange={setAutoPriorityRecallMinutes} disabled={!autoPriorityEnabled || !autoPriorityRecallEnabled} /></div>
          </CardContent>
        </Card>
      </div>

      <div className="flex max-w-full flex-wrap items-center justify-between gap-2 border-b border-border pb-1">
        <div className="flex min-w-0 items-center gap-1 overflow-x-auto" role="tablist" aria-label="中转站详情模块">
          {([
            ["accounts", ShieldAlert, "账号列表"],
            ["usage", History, "最近使用记录"],
            ["users", Users, "用户管理"],
            ["groups", Layers3, "分组管理"],
          ] as const).map(([value, Icon, label]) => (
            <button
              key={value}
              type="button"
              role="tab"
              aria-selected={detailModule === value}
              onClick={() => setDetailModule(value)}
              className={cn(
                "inline-flex h-10 shrink-0 items-center gap-1.5 border-b-2 px-3 text-xs transition-colors sm:h-9",
                detailModule === value ? "border-foreground font-medium text-foreground" : "border-transparent text-muted-foreground hover:border-border hover:text-foreground",
              )}
            >
              <Icon className="size-3.5" />
              {label}
            </button>
          ))}
        </div>
        <div className="list-panel-controls flex min-w-0 max-w-full flex-wrap items-center justify-end gap-2" aria-label="中转站统一时间条件">
          <div className="inline-flex max-w-full flex-wrap rounded-md border border-border bg-muted/30 p-0.5" role="group" aria-label="中转站统计时间范围">
            {usageRanges.map((item) => (
              <button key={item.value} type="button" onClick={() => changeUsageRange(item.value)} className={cn("h-11 rounded px-3 text-xs transition-colors sm:h-8", usageRange === item.value ? "bg-background font-medium text-foreground shadow-xs" : "text-muted-foreground hover:text-foreground")}>
                {item.label}
              </button>
            ))}
          </div>
          <Select value={latencyGranularity} onValueChange={(value) => setLatencyGranularity(value as LatencyBucketGranularity)}>
            <SelectTrigger className="h-11 w-36 text-xs sm:h-9" aria-label="时间稳定性与调用消耗趋势聚合粒度" title="作用于时间稳定性与消费趋势"><SelectValue /></SelectTrigger>
            <SelectContent>{latencyGranularityOptionsForRange(usageRange).map((item) => <SelectItem key={item.value} value={item.value}>{`粒度：${item.label}`}</SelectItem>)}</SelectContent>
          </Select>
        </div>
      </div>

      {detailModule === "usage" ? <RecentUsageTable rows={recentUsage.data ?? []} loading={recentUsage.loading} refreshing={recentUsage.refreshing} error={recentUsage.error} onRefresh={recentUsage.refetch} range={usageRange} groupNames={new Map(currentOverview.groups.map((group) => [group.external_id, group.name]))} accountNames={new Map(currentOverview.accounts.map((account) => [account.external_id, account.name]))} accountURLs={new Map(currentOverview.accounts.filter((account) => account.base_url).map((account) => [account.external_id, account.base_url!] as const))} systemCapacity={capabilities.accountConcurrency ? { current: currentOverview.accounts.reduce((sum, account) => sum + account.current_concurrency, 0), total: currentOverview.accounts.reduce((sum, account) => sum + account.concurrency, 0) } : undefined} onUserClick={(userID, userEmail, userName) => setHistoryUser({ id: userID, email: userEmail, name: userName })} /> : null}
      <UserBalanceHistoryDialog open={historyUser != null} userEmail={historyUser?.email ?? ""} userName={historyUser?.name ?? ""} data={userHistory.data} loading={userHistory.loading} error={userHistory.error} onClose={() => setHistoryUser(null)} />

      {detailModule === "users" ? capabilities.userLimits ? <UserManagement stationID={selectedID!} range={usageRange} /> : <NewAPIUserManagement key={selectedID} stationID={selectedID!} range={usageRange} /> : null}

      {detailModule === "groups" ? <GroupManagement overview={currentOverview} busy={busy} refreshing={overview.refreshing} testingGroupID={testingGroupID} deletingGroupID={deletingGroupID} onUpdate={updateGroup} onOrderChange={updateGroupOrder} onTest={(group) => void openGroupTest(group)} onDelete={(group) => void deleteGroup(group)} onRefresh={() => void refreshGroups()} /> : null}

      {detailModule === "accounts" ? <Card className="gap-0 overflow-hidden border border-border py-0 shadow-none">
        <ListPanelHeader
          secondaryPlacement="inline"
          actionsPlacement="secondary"
          title={<CardTitle className="flex items-center gap-2 text-sm font-semibold"><ShieldAlert className="size-4 text-brand" />账号列表</CardTitle>}
          description={accountListOpen ? <p className="mt-1 truncate text-xs text-muted-foreground">展示非 OAuth 账号的调度、成本、销售分组与风险；成本覆盖优先于实时探测。</p> : null}
          summary={accountListOpen ? <div className="flex flex-wrap items-center gap-3" aria-live="polite"><div className="flex min-w-0 items-center gap-2"><span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-amber-400/15 text-amber-600 dark:text-amber-400"><Box className="size-4" /></span><div><p className="text-[10px] text-muted-foreground">总 Token</p><p className="font-mono text-sm font-semibold tabular-nums text-foreground">{usage.loading && !usage.data ? "读取中…" : compactNumber.format(usageTotals.tokens)}</p></div></div><div className="flex min-w-0 items-center gap-2"><span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-success/10 text-success"><CircleDollarSign className="size-4" /></span><div><p className="text-[10px] text-muted-foreground">总消费</p><p className="font-mono text-sm font-semibold tabular-nums text-success">{usage.loading && !usage.data ? "读取中…" : chargeAmount(usageTotals.charge)}</p></div></div><div className="flex min-w-0 items-center gap-2"><span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground"><Users className="size-4" /></span><div><p className="text-[10px] text-muted-foreground">账号数量</p><p className="font-mono text-sm font-semibold tabular-nums text-foreground">{filteredAccounts.length} / {managedAccounts.length}</p></div></div><div className="flex min-w-0 items-center gap-2"><span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-brand/10 text-brand"><ListChecks className="size-4" /></span><div><p className="text-[10px] text-muted-foreground">可调组</p><p className="font-mono text-sm font-semibold tabular-nums text-foreground">{filteredSuggestionCount}</p></div></div><div className="flex min-w-0 items-center gap-2"><span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-warning/10 text-warning"><TrendingDown className="size-4" /></span><div><p className="text-[10px] text-muted-foreground">可降级</p><p className="font-mono text-sm font-semibold tabular-nums text-foreground">{filteredDowngradeCount}</p></div></div></div> : null}
          actions={accountListOpen ? <>{accountViewMode === "gallery" ? <div className="inline-flex h-11 shrink-0 items-center gap-1.5 rounded-md border border-border px-2.5 text-xs text-muted-foreground sm:h-9" title="全选当前筛选账号；图册展示每个账号最近 60 次调用和 60 段趋势"><Checkbox checked={allFilteredSelected} onCheckedChange={(value) => { const ids = filteredAccounts.map((account) => account.external_id); setSelected((current) => value === true ? Array.from(new Set([...current, ...ids])) : current.filter((id) => !ids.includes(id))) }} aria-label="全选当前筛选账号" /><span>全选当前筛选</span><span className="font-mono text-[10px] tabular-nums text-foreground">{filteredAccounts.length}</span><span className="hidden font-mono text-[10px] text-muted-foreground min-[1920px]:inline">60 次 / 60 段</span></div> : null}<DropdownMenu><DropdownMenuTrigger asChild><Button type="button" size="sm" variant="outline" className="h-11 gap-1.5 text-xs sm:h-9"><ListChecks className="size-3.5" />批量操作</Button></DropdownMenuTrigger><DropdownMenuContent align="end" className="w-64"><DropdownMenuLabel className="text-xs text-muted-foreground">账号批量操作</DropdownMenuLabel><DropdownMenuItem disabled={busy || !cloneableAccounts.length} onSelect={openBatchClone}><Plus className="size-3.5" />批量新增账号</DropdownMenuItem><DropdownMenuSeparator /><DropdownMenuItem disabled={busy || filteredEnableCount === 0} onSelect={() => void runFilteredAccountAction("schedulable", true, "一键启用调度")}><Power className="size-3.5" />批量启用调度 <span className="ml-auto text-xs text-muted-foreground">{filteredEnableCount}</span></DropdownMenuItem><DropdownMenuItem disabled={busy || filteredDisableCount === 0} onSelect={() => void runFilteredAccountAction("schedulable", false, "一键禁用调度")}><PowerOff className="size-3.5" />批量禁用调度 <span className="ml-auto text-xs text-muted-foreground">{filteredDisableCount}</span></DropdownMenuItem><DropdownMenuSeparator /><DropdownMenuItem disabled={busy || filteredSuggestionCount === 0} onSelect={() => void runFilteredAccountAction("apply-suggestions", undefined, "接受推荐调组")}><ListChecks className="size-3.5" />接受推荐调组 <span className="ml-auto text-xs text-muted-foreground">{filteredSuggestionCount}</span></DropdownMenuItem><DropdownMenuItem disabled={busy || filteredDowngradeCount === 0} onSelect={() => void runFilteredAccountAction("add-downgrades", undefined, "接受推荐降级")}><TrendingDown className="size-3.5" />接受推荐降级 <span className="ml-auto text-xs text-muted-foreground">{filteredDowngradeCount}</span></DropdownMenuItem></DropdownMenuContent></DropdownMenu></> : null}
          secondary={accountListOpen ? <><DebouncedAccountSearchInput initialValue={accountNameFilter} resetVersion={accountSearchResetVersion} onValueChange={setAccountNameFilter} /><DropdownMenu><DropdownMenuTrigger asChild><Button type="button" size="sm" variant="outline" className="h-11 gap-1.5 text-xs sm:h-9"><SlidersHorizontal className="size-3.5" />筛选{activeAccountFilterCount > 0 ? <span className="rounded-full bg-brand/10 px-1.5 py-0.5 text-[10px] font-semibold text-brand">{activeAccountFilterCount}</span> : null}</Button></DropdownMenuTrigger><DropdownMenuContent align="end" className="w-56"><DropdownMenuLabel className="text-xs text-muted-foreground">账号筛选</DropdownMenuLabel><DropdownMenuSeparator /><DropdownMenuSub><DropdownMenuSubTrigger>模型类型</DropdownMenuSubTrigger><DropdownMenuSubContent className="w-52"><DropdownMenuRadioGroup value={modelTypeFilter} onValueChange={setModelTypeFilter}><DropdownMenuRadioItem value="all">All</DropdownMenuRadioItem><DropdownMenuRadioItem value="__unassigned">未绑定模型类型</DropdownMenuRadioItem>{accountModelTypeOptions.map((value) => <DropdownMenuRadioItem key={value} value={value}>{value}</DropdownMenuRadioItem>)}</DropdownMenuRadioGroup></DropdownMenuSubContent></DropdownMenuSub><DropdownMenuSub><DropdownMenuSubTrigger>调度状态</DropdownMenuSubTrigger><DropdownMenuSubContent className="w-44"><DropdownMenuRadioGroup value={schedulableFilter} onValueChange={(value) => setSchedulableFilter(value as "enabled" | "disabled" | "all")}><DropdownMenuRadioItem value="all">All</DropdownMenuRadioItem><DropdownMenuRadioItem value="enabled">调度开启</DropdownMenuRadioItem><DropdownMenuRadioItem value="disabled">调度关闭</DropdownMenuRadioItem></DropdownMenuRadioGroup></DropdownMenuSubContent></DropdownMenuSub><DropdownMenuSub><DropdownMenuSubTrigger>账号状态</DropdownMenuSubTrigger><DropdownMenuSubContent className="w-48"><DropdownMenuRadioGroup value={riskFilter} onValueChange={(value) => setRiskFilter(value as RiskFilter)}><DropdownMenuRadioItem value="all">All</DropdownMenuRadioItem><DropdownMenuRadioItem value="adjustable">可调组</DropdownMenuRadioItem><DropdownMenuRadioItem value="downgradable">可降级</DropdownMenuRadioItem>{Object.entries(stateMeta).map(([value, meta]) => <DropdownMenuRadioItem key={value} value={value}>{meta.label}</DropdownMenuRadioItem>)}</DropdownMenuRadioGroup></DropdownMenuSubContent></DropdownMenuSub><DropdownMenuSub><DropdownMenuSubTrigger>销售分组</DropdownMenuSubTrigger><DropdownMenuSubContent className="max-h-80 w-64 overflow-y-auto"><DropdownMenuRadioGroup value={groupFilter} onValueChange={setGroupFilter}><DropdownMenuRadioItem value="all">All</DropdownMenuRadioItem>{publicFirst(currentOverview.groups).map((group) => <DropdownMenuRadioItem key={group.external_id} value={String(group.external_id)}>{group.name} · {group.platform || "-"}</DropdownMenuRadioItem>)}</DropdownMenuRadioGroup></DropdownMenuSubContent></DropdownMenuSub><DropdownMenuSeparator /><DropdownMenuItem disabled={!hasActiveAccountFilters} onSelect={resetFilters}><RotateCcw className="size-3.5" />清除筛选与搜索</DropdownMenuItem></DropdownMenuContent></DropdownMenu><DropdownMenu><DropdownMenuTrigger asChild><Button type="button" size="sm" variant="outline" className="h-11 gap-1.5 text-xs sm:h-9">排序：{accountSortOptions.find((item) => item.value === accountSort?.key)?.label ?? "综合评分"}{accountSort?.direction === "asc" ? <ArrowUp className="size-3.5" /> : <ArrowDown className="size-3.5" />}</Button></DropdownMenuTrigger><DropdownMenuContent align="end" className="w-56"><DropdownMenuLabel className="text-xs text-muted-foreground">排序维度</DropdownMenuLabel><DropdownMenuRadioGroup value={accountSort?.key ?? "score"} onValueChange={(value) => { const key = value as AccountSortKey; setAccountSort({ key, direction: defaultAccountSortDirection(key) }) }}>{accountSortOptions.map((item) => <DropdownMenuRadioItem key={item.value} value={item.value}>{item.label}</DropdownMenuRadioItem>)}</DropdownMenuRadioGroup><DropdownMenuSeparator /><DropdownMenuLabel className="text-xs text-muted-foreground">排序方向</DropdownMenuLabel><DropdownMenuRadioGroup value={accountSort?.direction ?? "desc"} onValueChange={(value) => setAccountSort((current) => ({ key: current?.key ?? "score", direction: value as SortDirection }))}><DropdownMenuRadioItem value="desc"><ArrowDown className="size-3.5" />降序（高 → 低 / 新 → 旧）</DropdownMenuRadioItem><DropdownMenuRadioItem value="asc"><ArrowUp className="size-3.5" />升序（低 → 高 / 旧 → 新）</DropdownMenuRadioItem></DropdownMenuRadioGroup></DropdownMenuContent></DropdownMenu><div className="inline-flex items-center rounded-md border border-border bg-muted/20 p-0.5" aria-label="账号列表展示模式"><Button type="button" size="sm" variant={accountViewMode === "gallery" ? "secondary" : "ghost"} className="h-8 gap-1.5 px-2.5 text-xs shadow-none" aria-pressed={accountViewMode === "gallery"} onClick={() => setAccountViewMode("gallery")}><LayoutGrid className="size-3.5" />图册</Button><Button type="button" size="sm" variant={accountViewMode === "list" ? "secondary" : "ghost"} className="h-8 gap-1.5 px-2.5 text-xs shadow-none" aria-pressed={accountViewMode === "list"} onClick={() => setAccountViewMode("list")}><List className="size-3.5" />列表</Button></div>{accountViewMode === "list" ? <DropdownMenu><DropdownMenuTrigger asChild><Button type="button" variant="outline" size="sm" className="h-11 gap-1.5 sm:h-9"><Columns3 className="size-3.5" />列设置</Button></DropdownMenuTrigger><DropdownMenuContent align="end" className="w-48"><DropdownMenuLabel>账号列表列</DropdownMenuLabel><DropdownMenuSeparator />{stationAccountColumns(stationType).map((column) => <DropdownMenuCheckboxItem key={column.key} checked={visibleAccountColumns.has(column.key)} onSelect={(event) => event.preventDefault()} onCheckedChange={(checked) => setVisibleAccountColumns((current) => { const next = new Set(current); if (checked === true) next.add(column.key); else if (next.size > 1) next.delete(column.key); return next })}>{column.label}</DropdownMenuCheckboxItem>)}<DropdownMenuSeparator /><DropdownMenuItem onSelect={() => setVisibleAccountColumns(new Set(defaultAccountColumns))}>恢复默认列</DropdownMenuItem></DropdownMenuContent></DropdownMenu> : null}<Button type="button" variant="outline" size="sm" className="h-11 gap-1.5 sm:h-9" disabled={busy || overview.refreshing || usage.refreshing || scoreLatencyBuckets.refreshing || latencyBuckets.refreshing || accountConsumptionBuckets.refreshing} onClick={() => void refreshAccounts()}><RefreshCw className={cn("size-3.5", (overview.refreshing || usage.refreshing || scoreLatencyBuckets.refreshing || latencyBuckets.refreshing || accountConsumptionBuckets.refreshing) && "animate-spin")} />刷新</Button></> : null}
          meta={<><div className="flex min-w-0 flex-wrap items-center justify-end gap-2">{accountListOpen ? <>{usage.loading ? <span className="flex items-center gap-1 text-xs text-muted-foreground" aria-live="polite"><RefreshCw className="size-3 animate-spin" />消费统计读取中</span> : usage.error ? <span className="max-w-[min(34rem,50vw)] text-right text-xs leading-5 text-danger" title={usage.error}>消费统计读取失败</span> : usage.data && !usage.data.complete ? <span className="max-w-[min(38rem,50vw)] text-right text-xs leading-5 text-warning">有 {usage.data.failed_accounts} 个账号的区间消费读取失败，账号卡片中的消费数据可能不完整。</span> : null}</> : null}<Button type="button" variant="ghost" size="icon" className="size-11 sm:size-9 shrink-0" aria-label={accountListOpen ? "收起账号列表" : "展开账号列表"} aria-expanded={accountListOpen} onClick={() => setAccountListOpen((value) => !value)}><ChevronDown className={cn("size-4 transition-transform duration-200", accountListOpen && "rotate-180")} /></Button></div></>}
        ></ListPanelHeader>
        {accountListOpen ? <CardContent className="border-t border-border p-0">
          {selected.length ? <div className="border-b border-brand/20 bg-brand/5 px-4 py-2.5"><div className="flex flex-wrap items-center gap-2"><div className="mr-2 flex items-center gap-2 text-xs font-medium text-brand"><Users className="size-4" />已选 {selected.length} 个账号</div><Select value={batchMode} onValueChange={(value) => { setBatchMode(value as BatchMode); setBatchRuntimeValue("") }}><SelectTrigger className="h-9 w-40 text-xs"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="groups">批量调整分组</SelectItem><SelectItem value="model_type">设置模型类型</SelectItem>{capabilities.accountConcurrency ? <SelectItem value="concurrency">设置并发数</SelectItem> : null}<SelectItem value="priority">设置优先级</SelectItem>{capabilities.poolRetry ? <SelectItem value="retry_count">设置同账号重试次数</SelectItem> : null}<SelectItem value="channel_group">渠道分组绑定</SelectItem><SelectItem value="manual">手工设置倍率</SelectItem></SelectContent></Select>{batchMode === "model_type" ? <><Select value={batchModelType || "__empty"} onValueChange={(value) => setBatchModelType(value === "__empty" ? "" : value)}><SelectTrigger className="h-9 w-44 text-xs"><SelectValue placeholder="选择模型类型" /></SelectTrigger><SelectContent><SelectItem value="__empty">清除类型</SelectItem>{modelTypeOptions.map((value) => <SelectItem key={value} value={value}>{value}</SelectItem>)}</SelectContent></Select><Button size="sm" className="h-9 gap-1.5" disabled={busy} onClick={() => void saveBatchModelType()}><Save className="size-3.5" />保存</Button></> : batchMode === "channel_group" ? <><Select value={batchChannelID} onValueChange={(value) => { setBatchChannelID(value); setBatchGroup("") }}><SelectTrigger className="h-9 w-48 text-xs"><SelectValue placeholder="选择当前渠道" /></SelectTrigger><SelectContent>{currentOverview.monitor_channels.map((channel) => <SelectItem key={channel.id} value={String(channel.id)}>{channel.name}</SelectItem>)}</SelectContent></Select><Select value={batchGroup} onValueChange={(value) => setBatchGroup(value)} disabled={!batchChannelID}><SelectTrigger className="h-9 w-52 text-xs"><SelectValue placeholder="选择渠道分组" /></SelectTrigger><SelectContent>{batchGroups.map((group) => <SelectItem key={group} value={group}>{group}</SelectItem>)}</SelectContent></Select></> : batchMode === "manual" ? <Input className="h-9 w-44 font-mono text-xs" inputMode="decimal" value={batchMultiplier} onChange={(event) => setBatchMultiplier(event.target.value)} placeholder="成本倍率，如 0.8" /> : batchMode === "concurrency" || batchMode === "priority" || batchMode === "retry_count" ? <Input className="h-9 w-36 font-mono text-xs" inputMode="numeric" value={batchRuntimeValue} onChange={(event) => setBatchRuntimeValue(event.target.value)} placeholder={batchMode === "retry_count" ? "同账号重试次数 0-10" : batchMode === "concurrency" ? "并发数 1-1000" : "优先级 1-1000"} aria-label={batchMode === "retry_count" ? "批量设置同账号重试次数" : batchMode === "concurrency" ? "批量设置并发数" : "批量设置优先级"} /> : <Button size="sm" variant="outline" className="h-9 gap-1.5" disabled={busy} onClick={() => setBatchGroupDialogOpen(true)}><SlidersHorizontal className="size-3.5" />选择销售分组</Button>}{batchMode === "groups" || batchMode === "model_type" ? null : batchMode === "concurrency" || batchMode === "priority" || batchMode === "retry_count" ? <Button size="sm" className="h-9 gap-1.5" disabled={busy} onClick={() => void saveBatchRuntimeSettings()}><Save className="size-3.5" />保存</Button> : <><Button size="sm" className="h-9 gap-1.5" disabled={busy} onClick={() => void saveBatchOverride()}><Save className="size-3.5" />保存覆盖</Button><Button size="sm" variant="outline" className="h-9 gap-1.5" disabled={busy} onClick={() => void saveBatchOverride(true)}><X className="size-3.5" />清除覆盖</Button></>}</div></div> : null}
          {accountViewMode === "gallery" ? <div className="relative p-3">{overview.refreshing || usage.refreshing || scoreLatencyBuckets.refreshing || latencyBuckets.refreshing || accountConsumptionBuckets.refreshing ? <div className="pointer-events-none absolute inset-x-0 top-4 z-50 flex justify-center"><span className="inline-flex items-center gap-1.5 rounded-full border border-border bg-background/90 px-3 py-1 text-xs text-muted-foreground shadow-sm"><RefreshCw className="size-3.5 animate-spin text-brand" />正在刷新账号图册</span></div> : null}{filteredAccounts.length === 0 ? <div className="px-4 py-10 text-center text-sm text-muted-foreground">当前筛选没有账号</div> : <div className="grid grid-cols-1 items-stretch gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-3">{filteredAccounts.map((account) => <RelayAccountGalleryCard key={account.external_id} account={account} overview={currentOverview} selected={selected.includes(account.external_id)} busy={busy} scheduling={schedulingAccountID === account.external_id} probing={probingAccountID === account.external_id} testing={testingAccountID === account.external_id} deleting={deletingAccountID === account.external_id} stationID={selectedID!} latencyGranularity={latencyGranularity} stabilityBuckets={latencyBucketsByAccount.get(account.external_id) ?? []} stabilityLoading={latencyBuckets.loading} consumptionBuckets={consumptionBucketsByAccount.get(account.external_id) ?? []} consumptionLoading={accountConsumptionBuckets.loading} channelLinks={channelLinksByRelayAccount.get(relayAccountComparisonKey(selectedID!, account.external_id)) ?? []} channelLatencyBucketsByChannel={channelLatencyBucketsByChannel} channelRecentSamplesByChannel={channelRecentSamplesByChannel} channelConsumptionBucketsByChannel={channelConsumptionBucketsByChannel} healthScore={healthScoreByAccount.get(account.external_id) ?? null} onToggle={(checked) => setSelected((current) => checked ? [...new Set([...current, account.external_id])] : current.filter((id) => id !== account.external_id))} onApply={() => void applySuggestion(account)} onAddDowngrade={() => void addDowngradeGroups(account)} onEditGroups={() => setGroupEditor(account)} onSchedulableChange={(checked) => void setSchedulable(account, checked)} onProbe={() => void probeAccount(account)} onTest={() => void openAccountTest(account)} onDelete={() => void deleteAccount(account)} />)}</div>}</div> : null}
          {accountViewMode === "list" ? <div className="relative isolate h-[min(72vh,760px)]">{overview.refreshing || usage.refreshing || scoreLatencyBuckets.refreshing || latencyBuckets.refreshing || accountConsumptionBuckets.refreshing ? <div className="pointer-events-none absolute inset-x-0 top-10 z-50 flex justify-center"><span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground"><RefreshCw className="size-3.5 animate-spin text-brand" />正在刷新账号列表</span></div> : null}<div data-relay-account-scroll className="h-full overflow-auto"><div style={accountTableGridStyle} className="relay-account-grid sticky top-0 z-40 flex items-center gap-3 border-b border-border bg-muted px-4 py-2 text-[11px] text-foreground/90 font-medium lg:grid lg:gap-0 lg:p-0 lg:[&>*]:px-4 lg:[&>*]:py-2"><span className="fixed-column-shadow-desktop fixed-column-shadow-left lg:sticky lg:left-0 lg:z-40 lg:flex lg:self-stretch lg:items-center lg:bg-muted"><Checkbox checked={allFilteredSelected} onCheckedChange={(value) => { const ids = filteredAccounts.map((account) => account.external_id); setSelected((current) => value === true ? Array.from(new Set([...current, ...ids])) : current.filter((id) => !ids.includes(id))) }} aria-label="选择筛选结果" /></span><span className="fixed-column-shadow-desktop fixed-column-shadow-left hidden lg:sticky lg:left-[64px] lg:z-40 lg:flex lg:self-stretch lg:items-center lg:bg-muted">账号</span>{visibleAccountColumns.has("score") ? <span className="hidden lg:inline">综合评分</span> : null}{visibleAccountColumns.has("platform") ? <span className="hidden lg:inline">平台</span> : null}{visibleAccountColumns.has("concurrency") ? <span className="hidden lg:inline">并发数</span> : null}{visibleAccountColumns.has("capacity") ? <span className="hidden lg:inline" title="Sub2API 实时并发占用 / 并发上限">容量</span> : null}{visibleAccountColumns.has("priority") ? <span className="hidden lg:inline" title="数值越小调度优先级越高">优先级</span> : null}{visibleAccountColumns.has("retry") ? <span className="hidden lg:inline" title="池模式下同一账号的失败重试次数">重试次数</span> : null}{visibleAccountColumns.has("schedulable") ? <span className="hidden lg:inline">调度</span> : null}{visibleAccountColumns.has("model") ? <span className="hidden lg:inline-flex items-center gap-1">模型类型<ModelTypeHelp /></span> : null}{visibleAccountColumns.has("recent") ? <span className="hidden lg:inline">最近调用</span> : null}{visibleAccountColumns.has("stability") ? <span className="hidden lg:inline">时间稳定性</span> : null}{visibleAccountColumns.has("usage") ? <span className="hidden lg:inline">区间消费</span> : null}{visibleAccountColumns.has("consumption_trend") ? <span className="hidden lg:inline">消费趋势</span> : null}{visibleAccountColumns.has("cost") ? <span className="hidden lg:inline">成本</span> : null}{visibleAccountColumns.has("groups") ? <span className="hidden lg:inline">当前销售分组</span> : null}{visibleAccountColumns.has("risk") ? <span className="hidden lg:inline">风险</span> : null}{visibleAccountColumns.has("profit") ? <span className="hidden lg:inline" title="利润率 = (销售倍率 - 成本倍率) / 成本倍率">利润</span> : null}{visibleAccountColumns.has("downgrade") ? <span className="hidden lg:inline" title="存在倍率低于当前销售分组且仍高于账号成本的可用分组">可降级</span> : null}<span className="fixed-column-shadow-desktop fixed-column-shadow-right hidden lg:sticky lg:right-0 lg:z-50 lg:flex lg:self-stretch lg:items-center lg:justify-end lg:bg-muted">操作</span><span className="ml-auto lg:hidden">当前筛选 {filteredAccounts.length} 个</span></div>
          {filteredAccounts.length === 0 ? <div className="px-4 py-10 text-center text-sm text-muted-foreground">当前筛选没有账号</div> : filteredAccounts.map((account) => <RiskRow key={account.external_id} account={account} overview={currentOverview} selected={selected.includes(account.external_id)} busy={busy} scheduling={schedulingAccountID === account.external_id} probing={probingAccountID === account.external_id} testing={testingAccountID === account.external_id} deleting={deletingAccountID === account.external_id} stationID={selectedID!} latencyGranularity={latencyGranularity} stabilityBuckets={latencyBucketsByAccount.get(account.external_id) ?? []} stabilityLoading={latencyBuckets.loading} consumptionBuckets={consumptionBucketsByAccount.get(account.external_id) ?? []} consumptionLoading={accountConsumptionBuckets.loading} channelLinks={channelLinksByRelayAccount.get(relayAccountComparisonKey(selectedID!, account.external_id)) ?? []} channelLatencyBucketsByChannel={channelLatencyBucketsByChannel} channelRecentSamplesByChannel={channelRecentSamplesByChannel} channelConsumptionBucketsByChannel={channelConsumptionBucketsByChannel} healthScore={healthScoreByAccount.get(account.external_id) ?? null} visibleColumns={visibleAccountColumns} gridStyle={accountTableGridStyle} onToggle={(checked) => setSelected((current) => checked ? [...new Set([...current, account.external_id])] : current.filter((id) => id !== account.external_id))} onApply={() => void applySuggestion(account)} onAddDowngrade={() => void addDowngradeGroups(account)} onEditGroups={() => setGroupEditor(account)} onSchedulableChange={(checked) => void setSchedulable(account, checked)} onProbe={() => void probeAccount(account)} onTest={() => void openAccountTest(account)} onDelete={() => void deleteAccount(account)} />)}</div></div> : null}
        </CardContent> : null}
      </Card> : null}

      <RelayAdjustmentLog rows={currentOverview.adjustments} refreshing={overview.refreshing} />
    </div> : overview.error ? <Alert variant="destructive"><CircleAlert /><AlertTitle>读取失败</AlertTitle><AlertDescription>{overview.error}</AlertDescription></Alert> : null}</div></div> : <div className="border border-dashed border-border px-4 py-12 text-center"><Server className="mx-auto size-8 text-muted-foreground" /><p className="mt-3 text-sm font-medium">还没有配置中转站</p><Button className="mt-4 gap-1.5" size="sm" onClick={openCreate}><Plus className="size-3.5" />添加中转站</Button></div>}
    <BatchCloneDialog accounts={cloneableAccounts} availableModelTypes={Array.from(new Set([...modelTypeOptions, ...accountModelTypeOptions]))} open={batchCloneOpen} busy={busy} result={batchCloneResult} onOpenChange={(open) => { if (!open) closeBatchClone() }} onSubmit={(groups) => void submitBatchClone(groups)} />
    <GroupEditor account={groupEditor} groups={currentOverview?.groups.map((group) => ({ external_id: group.external_id, name: group.name, platform: group.platform, status: group.status, is_exclusive: group.is_exclusive, require_oauth_only: group.require_oauth_only, account_types: group.account_types, model_types: group.model_types, rate_multiplier: group.rate_multiplier })) ?? []} open={groupEditor != null} busy={busy} onOpenChange={(open) => { if (!open) setGroupEditor(null) }} onSave={(ids) => void saveGroups(ids)} />
    <BatchGroupEditor groups={currentOverview?.groups.map((group) => ({ external_id: group.external_id, name: group.name, platform: group.platform, status: group.status, is_exclusive: group.is_exclusive, require_oauth_only: group.require_oauth_only, account_types: group.account_types, model_types: group.model_types, rate_multiplier: group.rate_multiplier })) ?? []} count={selected.length} open={batchGroupDialogOpen} busy={busy} onOpenChange={setBatchGroupDialogOpen} onSave={(ids) => void saveBatchGroups(ids)} />
    <Dialog open={testGroup != null} onOpenChange={(open) => { if (!open && testingGroupID == null) setTestGroup(null) }}>
      <DialogContent className="w-[calc(100vw-2rem)] max-h-[calc(100dvh-2rem)] max-w-[772px] overflow-x-hidden overflow-y-auto p-0">
        <DialogHeader className="border-b border-border px-4 py-5 sm:px-6"><DialogTitle>快速测试分组</DialogTitle><DialogDescription className="sr-only">选择模型和调用次数后，使用管理员额度进行真实分组调用测试</DialogDescription></DialogHeader>
        <div className="space-y-4 px-4 py-4 sm:px-6">
          <div className="flex items-center justify-between gap-3 rounded-md border border-border bg-muted/30 px-3 py-2.5"><div className="min-w-0"><p className="truncate text-sm font-semibold text-foreground">{testGroup?.name || "-"}</p><p className="mt-0.5 text-[11px] text-muted-foreground">{testGroup?.platform || "未知平台"} · 分组 #{testGroup?.external_id || "-"}</p></div><span className={cn("shrink-0 rounded px-2 py-1 text-[11px] font-medium", testGroup?.status?.toLowerCase() === "active" ? "bg-success/10 text-success" : "bg-muted text-muted-foreground")}>{testGroup?.status?.toLowerCase() === "active" ? "已启用" : "未启用"}</span></div>
          <Alert className="border-warning/30 bg-warning/5 py-3 text-warning"><CircleDollarSign className="size-4" /><AlertDescription className="text-xs leading-5 text-warning"><strong>真实调用测试：</strong>会消耗该分组绑定的管理员额度，并写入使用记录、错误记录和分组监控。请按需设置次数。</AlertDescription></Alert>
          <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_132px]">
            <div className="space-y-1.5"><Label htmlFor="group-test-model">测试模型</Label><Select value={groupTestModel} onValueChange={setGroupTestModel} disabled={groupTestModelsLoading || groupTestModels.length === 0}><SelectTrigger id="group-test-model" className="w-full"><SelectValue placeholder={groupTestModelsLoading ? "正在读取模型..." : "没有可用模型"} /></SelectTrigger><SelectContent>{groupTestModels.map((model) => <SelectItem key={model} value={model}>{model}</SelectItem>)}</SelectContent></Select></div>
            <div className="space-y-1.5"><Label htmlFor="group-test-count">调用次数</Label><Input id="group-test-count" type="number" min={1} max={10} step={1} inputMode="numeric" value={groupTestCount} onChange={(event) => setGroupTestCount(event.target.value)} disabled={testingGroupID != null} className="font-mono tabular-nums" aria-describedby="group-test-count-help" /><p id="group-test-count-help" className="text-[10px] text-muted-foreground">1 - 10 次，串行执行</p></div>
          </div>
          {groupTestError ? <Alert variant="destructive" className="min-w-0 overflow-hidden py-3"><CircleAlert className="size-4 shrink-0" /><AlertDescription className="min-w-0 break-words text-xs leading-5"><div className="font-medium">{groupTestNeedsKey ? "未找到当前管理员名下匹配该分组的令牌" : "分组快速测试失败"}</div><p className="mt-1 whitespace-pre-wrap">{groupTestError}</p>{groupTestNeedsKey && groupAPIKeysURL ? <a href={groupAPIKeysURL} target="_blank" rel="noopener noreferrer" className="mt-2 inline-flex cursor-pointer items-center gap-1.5 rounded border border-warning/40 bg-warning/10 px-2 py-1.5 font-medium text-warning underline underline-offset-2 transition-colors hover:bg-warning/20 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"><ExternalLink className="size-3.5" />前往创建 API 密钥</a> : null}</AlertDescription></Alert> : null}
          <div className="min-h-32 rounded-md border border-border bg-muted/20 p-3">
            {testingGroupID != null ? <p className="flex min-h-24 items-center justify-center gap-2 text-sm text-muted-foreground" aria-live="polite"><RefreshCw className="size-4 animate-spin text-brand" />正在执行 {groupTestCount} 次真实调用，请稍候...</p> : groupTestResult ? <div className="min-w-0 max-w-full space-y-3 overflow-hidden"><div className="grid grid-cols-3 divide-x rounded-md border border-border bg-card text-center"><div className="px-2 py-2"><p className="text-[10px] text-muted-foreground">请求</p><p className="font-mono text-sm font-semibold tabular-nums">{groupTestResult.requested}</p></div><div className="px-2 py-2"><p className="text-[10px] text-muted-foreground">成功</p><p className="font-mono text-sm font-semibold tabular-nums text-success">{groupTestResult.succeeded}</p></div><div className="px-2 py-2"><p className="text-[10px] text-muted-foreground">失败</p><p className="font-mono text-sm font-semibold tabular-nums text-danger">{groupTestResult.failed}</p></div></div><div className="max-h-56 min-w-0 max-w-full space-y-1.5 overflow-x-hidden overflow-y-auto">{groupTestResult.results.map((item) => <div key={item.index} className="flex min-w-0 max-w-full items-start gap-2 overflow-hidden rounded-md border border-border bg-card px-2.5 py-2 text-xs"><span className={cn("mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full", item.success ? "bg-success/10 text-success" : "bg-danger/10 text-danger")}>{item.success ? <Check className="size-3" /> : <CircleAlert className="size-3" />}</span><div className="min-w-0 max-w-full flex-1"><div className="flex min-w-0 flex-wrap items-center justify-between gap-2"><span className="font-medium">第 {item.index} 次调用</span><span className={cn("font-mono tabular-nums", item.success ? "text-muted-foreground" : "text-danger")}>{item.status_code ? `HTTP ${item.status_code}` : "请求失败"} · {formatLatency(item.duration_ms)}</span></div><p className={cn("mt-1 max-h-40 overflow-x-hidden overflow-y-auto break-all whitespace-pre-wrap pr-1 text-[11px]", item.success ? "text-muted-foreground" : "text-danger")}>{item.output || (item.success ? "调用成功" : "无错误详情")}</p></div></div>)}</div></div> : <p className="flex min-h-24 items-center justify-center text-sm text-muted-foreground">选择模型和调用次数后开始真实测试。</p>}
          </div>
        </div>
        <DialogFooter className="border-t border-border px-4 py-4 sm:px-6"><Button type="button" variant="outline" disabled={testingGroupID != null} onClick={() => setTestGroup(null)}>关闭</Button><Button type="button" className="gap-1.5" disabled={testingGroupID != null || groupTestModelsLoading || !groupTestModel} onClick={() => void runGroupTest()}><Play className="size-3.5" />开始测试</Button></DialogFooter>
      </DialogContent>
    </Dialog>
    <Dialog open={testAccount != null} onOpenChange={(open) => { if (!open && testingAccountID == null) setTestAccount(null) }}>
      <DialogContent className="max-h-[calc(100dvh-2rem)] max-w-xl overflow-y-auto p-0">
        <DialogHeader className="border-b border-border px-4 py-5 sm:px-6"><DialogTitle>测试账号连接</DialogTitle><DialogDescription className="sr-only">选择模型和测试模式后测试账号连接</DialogDescription></DialogHeader>
        <div className="space-y-4 px-4 sm:px-6">
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1.5"><Label>选择测试模型</Label><Select value={testModel} onValueChange={setTestModel} disabled={testModelsLoading || testModels.length === 0}><SelectTrigger className="w-full"><SelectValue placeholder={testModelsLoading ? "正在读取模型..." : "没有可用模型"} /></SelectTrigger><SelectContent>{testModels.map((model) => <SelectItem key={model} value={model}>{model}</SelectItem>)}</SelectContent></Select></div>
            <div className="space-y-1.5"><Label>测试模式</Label><Select value={testMode} onValueChange={setTestMode}><SelectTrigger className="w-full"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="regular">常规请求</SelectItem><SelectItem value="stream">流式请求</SelectItem></SelectContent></Select></div>
          </div>
          <div className="min-h-56 rounded-md border border-slate-800 bg-slate-950 p-4 font-mono text-xs leading-5 text-slate-300">
            <p>开始测试账号：<span className="text-sky-400">{testAccount?.name || "-"}</span></p>
            <p>账号类型：<span className="text-slate-100">{testAccount?.type || "-"}</span></p>
            <p>测试模式：<span className="text-slate-100">{testMode === "stream" ? "流式请求" : "常规请求"}</span></p>
            {testingAccountID != null ? <p className="mt-2 flex items-center gap-2 text-teal-300"><RefreshCw className="size-3.5 animate-spin" />正在连接 API...</p> : null}
            {testError ? <><p className="mt-2 text-red-300">连接测试失败</p><p className="max-h-36 overflow-auto whitespace-pre-wrap break-words text-red-200">{testError}</p></> : null}
            {testResult ? <><p className="mt-2 text-emerald-300">已连接到 API</p><p>使用模型：<span className="text-cyan-300">{testOutput.model || testModel}</span></p><p>发送测试消息：<span className="text-slate-100">&quot;hi&quot;</span></p><p className="mt-2 text-amber-300">响应：</p><div className="max-h-44 overflow-auto whitespace-pre-wrap break-words text-emerald-200">{testOutput.content || testOutput.fallback || "上游未返回文本内容"}</div><p className="mt-3 flex items-center gap-2 text-emerald-300"><Check className="size-3.5" />测试完成 <span className="text-slate-500">HTTP {testResult.status_code} · {formatLatency(testResult.duration_ms)}</span></p></> : null}
            {!testingAccountID && !testError && !testResult ? <p className="mt-2 text-slate-500">准备测试，点击开始测试。</p> : null}
          </div>
        </div>
        <DialogFooter className="border-t border-border px-4 py-4 sm:px-6"><Button type="button" variant="outline" disabled={testingAccountID != null} onClick={() => setTestAccount(null)}>关闭</Button><Button type="button" className="gap-1.5 bg-teal-600 hover:bg-teal-700" disabled={testingAccountID != null || testModelsLoading || !testModel} onClick={() => void runAccountTest()}><Play className="size-3.5" />开始测试</Button></DialogFooter>
      </DialogContent>
    </Dialog>
    <AccountAdjustmentSelectionDialog state={accountAdjustmentDialog} selections={accountAdjustmentSelections} busy={busy} groupsByID={groupsByID} onSelectionChange={updateAccountAdjustmentSelection} onCancel={() => finishAccountAdjustmentDialog(null)} onConfirm={() => finishAccountAdjustmentDialog(accountAdjustmentSelections)} />
    {confirmDialog}
  </section>
}
