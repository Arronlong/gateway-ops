"use client"

import { useState } from "react"
import { BellRing, CheckCircle2, Plus, Settings2, ShieldAlert } from "lucide-react"
import { useAlertSummary } from "@/lib/queries"
import { useTriggerRefresh } from "@/lib/refresh-context"
import type { AlertRule } from "@/lib/api-types"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { NotificationLogs, NotificationStatus } from "@/components/monitor/bottom-panels"
import { AlertRuleDialog } from "@/components/alerts/alert-rule-dialog"
import { AlertEventPanel, AlertRulePanel } from "@/components/alerts/alert-panels"

const tabs = ["预警事件", "订阅规则", "通知渠道", "发送记录"]

export default function NotificationsPage() {
  const [tab, setTab] = useState(0), [ruleOpen, setRuleOpen] = useState(false)
  const [editingRule, setEditingRule] = useState<AlertRule | null>(null)
  const refresh = useTriggerRefresh(), summary = useAlertSummary()
  const openNew = () => { setEditingRule(null); setRuleOpen(true) }
  return <section className="space-y-4">
    <header className="list-panel-controls flex flex-wrap items-end justify-between gap-3 border-l-2 border-brand pl-3"><div><h1 className="flex items-center gap-2 text-xl font-bold text-foreground"><BellRing className="size-5 text-brand" />预警中心</h1><p className="mt-1 text-xs text-muted-foreground">规则触发、自动处置、消息通知与执行记录。</p></div><Button size="sm" onClick={openNew}><Plus className="size-3.5" />新建规则</Button></header>
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <MetricCard label="待处理事件" value={summary.data?.open_events} detail="未恢复预警" icon={ShieldAlert} tone="text-danger" />
      <MetricCard label="运行中规则" value={summary.data?.enabled_rules} detail="启用中的订阅规则" icon={Settings2} tone="text-brand" />
      <MetricCard label="通知渠道" value={summary.data?.enabled_channels} detail="已启用渠道" icon={BellRing} tone="text-warning" />
      <MetricCard label="自动化处置" value={summary.data?.actions_today} detail="今日成功执行" icon={CheckCircle2} tone="text-success" />
    </div>
    {summary.error && <p role="alert" className="text-xs text-danger">指标加载失败：{summary.error}</p>}
    <div className="flex gap-5 overflow-x-auto border-b border-border" role="tablist" aria-label="预警中心分区">{tabs.map((label, index) => <button key={label} type="button" id={`alert-tab-${index}`} role="tab" aria-selected={tab === index} aria-controls={`alert-panel-${index}`} tabIndex={tab === index ? 0 : -1} onKeyDown={(event) => {
      if (!["ArrowRight", "ArrowLeft", "Home", "End"].includes(event.key)) return
      event.preventDefault()
      const next = event.key === "Home" ? 0 : event.key === "End" ? tabs.length - 1 : (index + (event.key === "ArrowRight" ? 1 : -1) + tabs.length) % tabs.length
      setTab(next); document.getElementById(`alert-tab-${next}`)?.focus()
    }} onClick={() => setTab(index)} className={`shrink-0 border-b-2 px-1 py-3 text-sm font-medium transition-colors ${tab === index ? "border-brand text-brand" : "border-transparent text-muted-foreground hover:text-foreground"}`}>{label}{index < 2 && summary.data && <span className="ml-1.5 rounded bg-muted px-1.5 py-0.5 text-[10px]">{index === 0 ? summary.data.open_events : summary.data.enabled_rules}</span>}</button>)}</div>
    <div role="tabpanel" id={`alert-panel-${tab}`} aria-labelledby={`alert-tab-${tab}`}>
      {tab === 0 && <AlertEventPanel onRefresh={refresh} />}
      {tab === 1 && <AlertRulePanel onNew={openNew} onEdit={(rule) => { setEditingRule(rule); setRuleOpen(true) }} onRefresh={refresh} />}
      {tab === 2 && <NotificationStatus fullPage />}
      {tab === 3 && <NotificationLogs />}
    </div>
    <AlertRuleDialog open={ruleOpen} onOpenChange={setRuleOpen} rule={editingRule} onSaved={refresh} />
  </section>
}

function MetricCard({ label, value, detail, icon: Icon, tone }: { label: string; value?: number; detail: string; icon: typeof BellRing; tone: string }) {
  return <Card className="gap-0 border border-border py-0 shadow-none"><CardContent className="px-4 py-4"><div className="flex items-center justify-between text-xs text-muted-foreground"><span>{label}</span><Icon className={`size-4 ${tone}`} /></div><p className="mt-2 font-mono text-2xl font-semibold">{value ?? "—"}</p><p className="mt-1 text-[11px] text-muted-foreground">{detail}</p></CardContent></Card>
}
