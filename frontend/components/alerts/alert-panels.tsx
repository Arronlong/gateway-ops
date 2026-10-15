"use client"

import { useEffect, useState } from "react"
import { useSearchParams } from "react-router-dom"
import { RefreshCw, Plus, Settings2, AlertTriangle } from "lucide-react"
import { toast } from "sonner"
import { apiFetch } from "@/lib/api"
import { useAlertEvents, useAlertRules, useRelayStations } from "@/lib/queries"
import type { AlertActionLog, AlertEvent, AlertRule, NotificationLog } from "@/lib/api-types"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardTitle } from "@/components/ui/card"
import { ListPanelHeader } from "@/components/ui/list-panel"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { actionLabels, initialRuleForm, metricLabels, stateLabels } from "./rule-form"

const date = (value?: string | null) => value ? new Date(value).toLocaleString("zh-CN", { hour12: false }) : "—"
const errorText = (error: unknown) => error instanceof Error ? error.message : "操作失败"
const selectClass = "h-8 rounded-md border border-input bg-background px-2 text-xs"
const severityLabels: Record<string, string> = { info: "提示", warning: "警告", critical: "严重" }

export function AlertEventPanel({ onRefresh }: { onRefresh: () => void }) {
  const [state, setState] = useState("open"), [target, setTarget] = useState(""), [severity, setSeverity] = useState("")
  const [station, setStation] = useState(""), [offset, setOffset] = useState(0)
  const [period, setPeriod] = useState(""), [from, setFrom] = useState("")
  const [selected, setSelected] = useState<AlertEvent | null>(null), [busy, setBusy] = useState(false)
  const [searchParams, setSearchParams] = useSearchParams(), eventID = searchParams.get("event")
  function closeDetail() {
    setSelected(null)
    if (eventID) { const next = new URLSearchParams(searchParams); next.delete("event"); setSearchParams(next, { replace: true }) }
  }
  useEffect(() => {
    if (!eventID || !/^[1-9]\d*$/.test(eventID)) return
    const controller = new AbortController()
    void apiFetch<{ event: AlertEvent }>(`/alerts/events/${eventID}`, { signal: controller.signal }).then((detail) => setSelected(detail.event)).catch((error) => { if (!controller.signal.aborted) toast.error(errorText(error)) })
    return () => controller.abort()
  }, [eventID])
  const events = useAlertEvents(state, { target, severity, station, offset, from }), stations = useRelayStations()
  useEffect(() => setOffset(0), [state, target, severity, station, from])
  async function update(event: AlertEvent, action: "ack" | "recover") {
    if (action === "recover" && !window.confirm("关闭此事件？此操作不会恢复远端调度或启用用户；后续异常仍会重新触发。")) return
    setBusy(true)
    try {
      await apiFetch(`/alerts/events/${event.id}/${action}`, { method: "POST" })
      toast.success(action === "ack" ? "事件已确认" : "事件已关闭，远端状态保持不变")
      await events.refetch(); onRefresh(); closeDetail()
    } catch (error) { toast.error(errorText(error)) } finally { setBusy(false) }
  }
  return <Card className="gap-0 overflow-hidden border border-border py-0 shadow-none">
    <ListPanelHeader title={<CardTitle className="flex items-center gap-2 text-sm font-semibold"><AlertTriangle className="size-4 text-brand" />预警事件</CardTitle>} description={<p className="mt-1 text-xs text-muted-foreground">异常、恢复与自动化结果集中追踪。确认事件不会改变远端状态。</p>} actions={<Button variant="outline" size="sm" onClick={() => void events.refetch()} disabled={events.refreshing}><RefreshCw className="size-3.5" />刷新</Button>} />
    <div className="flex flex-wrap gap-2 border-t border-border px-4 py-3">
      <select className={selectClass} aria-label="事件状态" value={state} onChange={(event) => setState(event.target.value)}><option value="open">未恢复</option><option value="">全部状态</option><option value="firing">触发中</option><option value="recovering">恢复中</option><option value="recovered">已恢复 / 已关闭</option></select>
      <select className={selectClass} aria-label="事件对象类型" value={target} onChange={(event) => setTarget(event.target.value)}><option value="">全部对象</option><option value="relay_account">中转站账号</option><option value="relay_user">中转站用户</option></select>
      <select className={selectClass} aria-label="事件级别" value={severity} onChange={(event) => setSeverity(event.target.value)}><option value="">全部级别</option>{Object.entries(severityLabels).map(([key, label]) => <option key={key} value={key}>{label}</option>)}</select>
      <select className={selectClass} aria-label="事件中转站" value={station} onChange={(event) => setStation(event.target.value)}><option value="">全部中转站</option>{stations.data?.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select>
      <select className={selectClass} aria-label="事件时间范围" value={period} onChange={(event) => { setPeriod(event.target.value); setFrom(event.target.value ? new Date(Date.now() - Number(event.target.value)*3600000).toISOString() : "") }}><option value="">全部时间</option><option value="24">最近 24 小时</option><option value="168">最近 7 天</option><option value="720">最近 30 天</option></select>
    </div>
    <CardContent className="border-t border-border p-0">
      {events.error && <p role="alert" className="px-4 py-3 text-sm text-danger">加载事件失败：{events.error}</p>}
      <div className="overflow-x-auto"><table className="w-full min-w-[850px] text-left text-xs"><thead className="border-b border-border bg-muted text-[11px] text-muted-foreground"><tr>{["事件 / 对象", "判定摘要", "状态", "开始时间", "操作"].map((label) => <th className="px-4 py-2 font-medium" key={label}>{label}</th>)}</tr></thead><tbody className="divide-y divide-border">{events.data?.map((event) => <tr key={event.id}><td className="px-4 py-3"><button className="text-left font-medium text-brand hover:underline" onClick={() => setSelected(event)}>#{event.id} {metricLabels[event.metric] ?? "预警事件"}</button><p className="mt-1 text-muted-foreground">{event.target_type === "relay_account" ? "账号" : "用户"} #{event.target_external_id} · {stations.data?.find((item) => item.id === event.relay_station_id)?.name ?? `中转站 #${event.relay_station_id}`}</p></td><td className="max-w-80 px-4 py-3"><p>{event.summary}</p><p className="mt-1 text-muted-foreground">实际值 {event.actual_value ?? "—"} / 阈值 {event.threshold_value ?? "—"}</p></td><td className="px-4 py-3"><span className={event.recovered_at ? "text-success" : "text-danger"}>{stateLabels[event.state] ?? event.state}</span><p className="mt-1 text-muted-foreground">{severityLabels[event.severity]}{event.acknowledged_at ? " · 已确认" : ""}</p></td><td className="whitespace-nowrap px-4 py-3 text-muted-foreground">{date(event.started_at)}</td><td className="px-4 py-3"><div className="flex gap-1"><Button variant="ghost" size="sm" onClick={() => setSelected(event)}>详情</Button>{!event.recovered_at && <><Button variant="ghost" size="sm" disabled={busy || !!event.acknowledged_at} onClick={() => void update(event, "ack")}>确认</Button><Button variant="ghost" size="sm" disabled={busy} onClick={() => void update(event, "recover")}>关闭</Button></>}</div></td></tr>)}</tbody></table></div>
      {!events.data?.length && !events.error && <p className="py-12 text-center text-sm text-muted-foreground">{events.loading ? "正在读取预警事件…" : "当前筛选下暂无预警事件"}</p>}
      <div className="flex items-center justify-between border-t border-border px-4 py-3 text-xs text-muted-foreground"><span>第 {offset / 50 + 1} 页 · 每页最多 50 条</span><div className="flex gap-2"><Button variant="outline" size="sm" disabled={offset === 0 || events.loading} onClick={() => setOffset(Math.max(0, offset - 50))}>上一页</Button><Button variant="outline" size="sm" disabled={(events.data?.length ?? 0) < 50 || events.loading} onClick={() => setOffset(offset + 50)}>下一页</Button></div></div>
    </CardContent>
    <EventDetail event={selected} onClose={closeDetail} />
  </Card>
}

interface EventDetailData { event: AlertEvent; actions: AlertActionLog[]; notifications: NotificationLog[]; rule?: AlertRule | null }

function EventDetail({ event, onClose }: { event: AlertEvent | null; onClose: () => void }) {
  const [detail, setDetail] = useState<EventDetailData | null>(null)
  const [error, setError] = useState("")
  useEffect(() => {
    setDetail(null); setError("")
    if (!event) return
    const controller = new AbortController()
    void apiFetch<EventDetailData>(`/alerts/events/${event.id}`, { signal: controller.signal }).then(setDetail).catch((reason) => { if (!controller.signal.aborted) setError(errorText(reason)) })
    return () => controller.abort()
  }, [event])
  const current = detail?.event ?? event
  const statuses: Record<string, string> = { prepared: "准备执行", succeeded: "执行成功", success: "执行成功", skipped: "已跳过", failed: "执行失败", uncertain: "结果待确认" }
  const recoveryForm = detail?.rule ? initialRuleForm(detail.rule) : null
  const actionSummary = detail?.actions.map((log) => `${actionLabels[log.action] ?? log.action}：${statuses[log.status] ?? log.status}`).join("；")
  return <Dialog open={!!event} onOpenChange={(open) => { if (!open) onClose() }}><DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-3xl"><DialogHeader><DialogTitle>事件 #{event?.id}</DialogTitle><DialogDescription>{current?.summary}</DialogDescription></DialogHeader>
    {error && <p role="alert" className="text-sm text-danger">详情加载失败：{error}</p>}
    <dl className="grid grid-cols-[90px_1fr] gap-2 text-sm"><dt className="text-muted-foreground">对象</dt><dd>中转站 #{current?.relay_station_id} · {current?.target_type === "relay_account" ? "账号" : "用户"} #{current?.target_external_id}</dd><dt className="text-muted-foreground">状态</dt><dd>{current ? stateLabels[current.state] : ""}</dd><dt className="text-muted-foreground">实际 / 阈值</dt><dd>{current?.actual_value ?? "—"} / {current?.threshold_value ?? "—"}</dd><dt className="text-muted-foreground">开始</dt><dd>{date(current?.started_at)}</dd><dt className="text-muted-foreground">确认</dt><dd>{date(current?.acknowledged_at)}</dd><dt className="text-muted-foreground">恢复 / 关闭</dt><dd>{date(current?.recovered_at)}</dd><dt className="text-muted-foreground">处置摘要</dt><dd>{actionSummary || current?.action_summary || "—"}</dd></dl>
    <h3 className="border-t border-border pt-4 text-sm font-semibold">自动化执行记录</h3>
    {!detail && !error && <p className="text-sm text-muted-foreground">正在读取执行记录…</p>}{detail && !detail.actions.length && <p className="text-sm text-muted-foreground">此事件没有自动化执行记录。</p>}
    {detail?.actions.map((log) => <div key={log.id} className="space-y-2 rounded-md border border-border p-3 text-xs"><div className="flex flex-wrap justify-between gap-2"><strong>{log.phase === "recovery" ? "恢复处理" : "触发处理"} · {actionLabels[log.action] ?? log.action}</strong><span className={log.success ? "text-success" : "text-warning"}>{statuses[log.status] ?? log.status}</span></div><p className="text-muted-foreground">{date(log.created_at)}</p>{log.error_message && <p>{log.error_message}</p>}<div className="grid gap-2 sm:grid-cols-2"><pre className="overflow-auto rounded bg-muted p-2">执行前：{log.before || "—"}</pre><pre className="overflow-auto rounded bg-muted p-2">执行后：{log.after || "—"}</pre></div></div>)}
    {recoveryForm && <p className="rounded-md bg-muted p-3 text-xs text-muted-foreground">恢复配置：连续正常 {recoveryForm.recoveryCount} 次{recoveryForm.recoveryThreshold ? `，恢复阈值 ${recoveryForm.recoveryThreshold}` : ""}；{recoveryForm.autoRecovery ? "满足条件后尝试安全恢复远端状态" : "远端状态由人工处理"}。</p>}
    <h3 className="border-t border-border pt-4 text-sm font-semibold">消息发送记录</h3>
    {detail && !detail.notifications?.length && <p className="text-xs text-muted-foreground">此事件暂无消息发送记录。</p>}
    {detail?.notifications?.map((log) => <div key={log.id} className="rounded-md border border-border p-3 text-xs"><div className="flex justify-between gap-2"><strong>{log.event === "alert_recovered" ? "恢复通知" : "触发通知"} · 通知渠道 #{log.channel_id}</strong><span className={log.success ? "text-success" : "text-danger"}>{log.success ? "发送成功" : "发送失败"}</span></div><p className="mt-1 text-muted-foreground">{date(log.sent_at)} · {log.subject}</p>{log.error_message && <p className="mt-1 text-danger">{log.error_message}</p>}</div>)}
    <details className="text-xs"><summary className="cursor-pointer text-muted-foreground">查看指标快照</summary><pre className="mt-2 max-h-48 overflow-auto whitespace-pre-wrap rounded bg-muted p-3">{current?.sample || "—"}</pre></details>
  </DialogContent></Dialog>
}

interface Preview { station_id: number; target_external_id: string; target_name: string; actual: number; violated: boolean; unknown: boolean; summary: string }

export function AlertRulePanel({ onNew, onEdit, onRefresh }: { onNew: () => void; onEdit: (rule: AlertRule) => void; onRefresh: () => void }) {
  const rules = useAlertRules(), [busy, setBusy] = useState<number | null>(null)
  const [preview, setPreview] = useState<{ name: string; targets: Preview[] } | null>(null)
  async function operate(rule: AlertRule, action: "toggle" | "delete" | "test") {
    if (action === "delete" && !window.confirm(`删除规则「${rule.name}」？已有执行记录会保留；删除不会恢复任何远端状态。`)) return
    if (action === "toggle" && rule.enabled && !window.confirm("暂停此规则？暂停后不再评估，也不会执行自动恢复。已有停用状态需人工处理。")) return
    setBusy(rule.id)
    try {
      if (action === "test") {
        const targets = await apiFetch<Preview[]>(`/alerts/rules/${rule.id}/test`, { method: "POST" })
        setPreview({ name: rule.name, targets })
      } else {
        await apiFetch(`/alerts/rules/${rule.id}${action === "toggle" ? "/enabled" : ""}`, { method: action === "toggle" ? "PATCH" : "DELETE", ...(action === "toggle" ? { body: JSON.stringify({ enabled: !rule.enabled }) } : {}) })
        toast.success(action === "delete" ? "规则已删除" : rule.enabled ? "规则已暂停" : "规则已启用")
        await rules.refetch(); onRefresh()
      }
    } catch (error) { toast.error(errorText(error)) } finally { setBusy(null) }
  }
  return <Card className="gap-0 overflow-hidden border border-border py-0 shadow-none"><ListPanelHeader title={<CardTitle className="flex items-center gap-2 text-sm font-semibold"><Settings2 className="size-4 text-brand" />订阅规则</CardTitle>} description={<p className="mt-1 text-xs text-muted-foreground">配置监控与触发；试运行只读取现有采样，不发送通知或执行动作。</p>} actions={<div className="flex gap-2"><Button variant="outline" size="sm" onClick={() => void rules.refetch()} disabled={rules.refreshing}><RefreshCw className="size-3.5" />刷新</Button><Button size="sm" onClick={onNew}><Plus className="size-3.5" />新建</Button></div>} />
    <CardContent className="border-t border-border p-0">{rules.error && <p role="alert" className="px-4 py-3 text-sm text-danger">加载规则失败：{rules.error}</p>}<div className="divide-y divide-border">{rules.data?.map((rule) => {
      const form = initialRuleForm(rule)
      return <div key={rule.id} className="flex flex-wrap items-center justify-between gap-4 px-4 py-4 text-xs"><div className="min-w-64 flex-1"><div className="flex items-center gap-2"><strong className="text-sm">{rule.name}</strong><span className={rule.enabled ? "text-success" : "text-muted-foreground"}>{rule.enabled ? "运行中" : "已暂停"}</span></div><p className="mt-1 text-muted-foreground">{metricLabels[rule.metric]} · {form.stationID === "all" ? "全部中转站" : `中转站 #${form.stationID}`} · {form.targetIDs || "全部对象"}</p><p className="mt-1 text-muted-foreground">{form.window} 秒窗口 · 阈值 {form.threshold} · {form.hitWindow ? `${form.hitWindow} 次中命中 ${form.trigger} 次` : `连续 ${form.trigger} 次`} · {actionLabels[form.action]} · {form.channels.length} 个通知渠道</p></div><div className="flex flex-wrap gap-1"><Button size="sm" variant="outline" disabled={busy === rule.id} onClick={() => void operate(rule, "test")}>{busy === rule.id ? "处理中…" : "试运行"}</Button><Button size="sm" variant="ghost" onClick={() => onEdit(rule)} disabled={busy === rule.id}>编辑</Button><Button size="sm" variant="ghost" onClick={() => void operate(rule, "toggle")} disabled={busy === rule.id}>{rule.enabled ? "暂停" : "启用"}</Button><Button size="sm" variant="ghost" className="text-danger" onClick={() => void operate(rule, "delete")} disabled={busy === rule.id}>删除</Button></div></div>
    })}</div>{!rules.data?.length && !rules.error && <p className="py-12 text-center text-sm text-muted-foreground">{rules.loading ? "正在读取规则…" : "暂无订阅规则，创建规则后开始监控。"}</p>}</CardContent>
    <Dialog open={!!preview} onOpenChange={(open) => { if (!open) setPreview(null) }}><DialogContent className="max-h-[80vh] overflow-y-auto sm:max-w-2xl"><DialogHeader><DialogTitle>试运行 · {preview?.name}</DialogTitle><DialogDescription>本次只验证当前条件；连续次数及实际自动化结果以正式运行记录为准。</DialogDescription></DialogHeader>{preview?.targets.length ? preview.targets.map((item) => <div className="rounded-md border border-border p-3 text-sm" key={`${item.station_id}:${item.target_external_id}`}><p className="font-medium">中转站 #{item.station_id} · {item.target_name || item.target_external_id}<span className={`ml-3 ${item.unknown ? "text-muted-foreground" : item.violated ? "text-danger" : "text-success"}`}>{item.unknown ? "数据不足" : item.violated ? "条件命中" : "正常"}</span></p><p className="mt-1 text-xs text-muted-foreground">{item.summary} · 实际值 {item.unknown ? "—" : item.actual}</p></div>) : <p className="text-sm text-muted-foreground">未找到可评估对象。请检查中转站、目标 ID 与采集状态。</p>}</DialogContent></Dialog>
  </Card>
}
