"use client"

import { useEffect, useId, useState, type ReactNode } from "react"
import { toast } from "sonner"
import { apiFetch } from "@/lib/api"
import { useNotificationChannels, useRelayStations } from "@/lib/queries"
import type { AlertMetric, AlertRule, AlertScopeFilter, AlertTargetType } from "@/lib/api-types"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { actionLabels, initialRuleForm, metricLabels, rulePayload, targetChanged, type RuleForm } from "./rule-form"

const selectClass = "h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
type ScopeField = { field: string; label: string; type: "text" | "number" | "boolean" | "time" }

const scopeFields: Record<AlertTargetType, ScopeField[]> = {
  channel: [
    { field: "name", label: "渠道名称", type: "text" }, { field: "type", label: "渠道类型", type: "text" },
    { field: "monitor_enabled", label: "监控状态", type: "boolean" }, { field: "balance_mode", label: "余额模式", type: "text" },
    { field: "balance", label: "当前余额", type: "number" }, { field: "score", label: "运行评分", type: "number" },
    { field: "cache_hit_rate", label: "缓存命中率（%）", type: "number" }, { field: "model", label: "模型", type: "text" },
  ],
  relay_account: [
    { field: "station_type", label: "站点类型", type: "text" }, { field: "station_name", label: "站点名称", type: "text" },
    { field: "name", label: "账号名称", type: "text" }, { field: "identity", label: "账号标识", type: "text" },
    { field: "platform", label: "平台", type: "text" }, { field: "type", label: "账号类型", type: "text" },
    { field: "status", label: "账号状态", type: "text" }, { field: "schedulable", label: "调度状态", type: "boolean" },
    { field: "account_plan", label: "套餐", type: "text" }, { field: "model_type", label: "模型类型", type: "text" },
    { field: "expires_at", label: "过期时间", type: "time" }, { field: "last_used_at", label: "最近使用时间", type: "time" },
  ],
  relay_user: [
    { field: "station_type", label: "站点类型", type: "text" }, { field: "station_name", label: "站点名称", type: "text" },
    { field: "username", label: "用户名", type: "text" }, { field: "email", label: "邮箱", type: "text" },
    { field: "role", label: "角色", type: "text" }, { field: "status", label: "用户状态", type: "text" },
    { field: "balance", label: "用户余额", type: "number" }, { field: "concurrency", label: "并发上限", type: "number" },
    { field: "rpm_limit", label: "每分钟请求上限", type: "number" }, { field: "last_used_at", label: "最近使用时间", type: "time" },
    { field: "created_at", label: "创建时间", type: "time" },
  ],
}

const operatorOptions: Record<ScopeField["type"], { value: string; label: string }[]> = {
  text: [{ value: "eq", label: "等于" }, { value: "ne", label: "不等于" }, { value: "contains", label: "包含" }, { value: "not_contains", label: "不包含" }],
  number: [{ value: "gt", label: "大于" }, { value: "gte", label: "大于等于" }, { value: "lt", label: "小于" }, { value: "lte", label: "小于等于" }, { value: "eq", label: "等于" }, { value: "ne", label: "不等于" }],
  boolean: [{ value: "eq", label: "等于" }, { value: "ne", label: "不等于" }],
  time: [{ value: "gt", label: "晚于" }, { value: "gte", label: "不早于" }, { value: "lt", label: "早于" }, { value: "lte", label: "不晚于" }],
}

type PreviewTarget = { station_id: number; target_external_id: string; target_name: string; actual: number; violated: boolean; unknown: boolean; summary: string }
type ScopePreview = { matched: number; targets: PreviewTarget[] }

function Field({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  const id = useId()
  return <div className="space-y-1.5"><Label htmlFor={id}>{label}</Label><div id={id}>{children}</div>{hint && <p className="text-xs leading-relaxed text-muted-foreground">{hint}</p>}</div>
}

export function AlertRuleDialog({ open, onOpenChange, rule, onSaved }: { open: boolean; onOpenChange: (value: boolean) => void; rule?: AlertRule | null; onSaved: () => void }) {
  const channels = useNotificationChannels(), stations = useRelayStations()
  const [form, setForm] = useState<RuleForm>(() => initialRuleForm())
  const [step, setStep] = useState(0), [saving, setSaving] = useState(false), [previewing, setPreviewing] = useState(false)
  const [scopePreview, setScopePreview] = useState<ScopePreview | null>(null)
  useEffect(() => { if (open) { setForm(initialRuleForm(rule)); setStep(0); setScopePreview(null) } }, [open, rule])
  const set = <K extends keyof RuleForm>(key: K, value: RuleForm[K]) => { setForm((current) => ({ ...current, [key]: value })); setScopePreview(null) }
  const number = (key: keyof RuleForm, label: string, hint?: string) => <Field label={label} hint={hint}><Input aria-label={label} type="number" step="any" value={String(form[key])} onChange={(event) => set(key, event.target.value)} /></Field>
  const spend = form.metric === "account_spend" || form.metric === "user_spend"
  const balance = form.metric === "balance_low" || form.metric === "user_balance_low"
  const rateChange = form.metric === "channel_rate_change" || form.metric === "rate_change"
  const disabling = form.action === "disable_account_scheduling" || form.action === "disable_user"
  const stationIDs = form.stationID === "all" ? [] : form.stationID.split(",")
  const allowedMetrics: Record<AlertTargetType, AlertMetric[]> = {
    channel: ["balance_low", "channel_rate_change"],
    relay_account: ["account_spend", "account_stability"],
    relay_user: ["user_spend", "user_multi_ip", "user_balance_low"],
  }
  const metrics = [...allowedMetrics[form.targetType]]
  if (rule?.target_type === form.targetType && !metrics.includes(form.metric)) metrics.push(form.metric)

  async function save() {
    try {
      const payload = rulePayload(form)
      if (rule) {
        const original = rulePayload(initialRuleForm(rule))
        for (const key of ["scope", "condition", "action", "recovery"] as const) {
          if (JSON.stringify(payload[key]) === JSON.stringify(original[key])) Object.assign(payload, { [key]: JSON.parse(rule[key]) })
        }
      }
      setSaving(true)
      await apiFetch(rule ? `/alerts/rules/${rule.id}` : "/alerts/rules", { method: rule ? "PUT" : "POST", body: JSON.stringify(payload) })
      toast.success(rule ? "规则已保存" : "规则已创建")
      onSaved(); onOpenChange(false)
    } catch (error) { toast.error(error instanceof Error ? error.message : "保存失败") } finally { setSaving(false) }
  }

  async function previewScope() {
    try {
      const payload = rulePayload(form)
      setPreviewing(true)
      setScopePreview(await apiFetch<ScopePreview>("/alerts/rules/preview", { method: "POST", body: JSON.stringify(payload) }))
    } catch (error) { toast.error(error instanceof Error ? error.message : "试算失败") } finally { setPreviewing(false) }
  }

  function changeMetric(metric: AlertMetric) {
    const balanceMetric = metric === "balance_low" || metric === "user_balance_low"
    setForm((current) => ({ ...current, metric, threshold: metric === "user_multi_ip" ? "3" : balanceMetric ? "10" : "20", baseline: "", recoveryThreshold: "", trustedIPs: "", minRequests: metric === "channel_rate_change" || metric === "rate_change" || balanceMetric ? "0" : current.minRequests }))
    setScopePreview(null)
  }

  function addFilter() {
    if (form.filters.length >= 20) return
    const available = scopeFields[form.targetType].find((field) => !form.filters.some((filter) => filter.field === field.field)) ?? scopeFields[form.targetType][0]
    set("filters", [...form.filters, { field: available.field, operator: operatorOptions[available.type][0].value, value: "" }])
  }

  function updateFilter(index: number, patch: Partial<AlertScopeFilter>) {
    const next = form.filters.map((filter, itemIndex) => itemIndex === index ? { ...filter, ...patch } : filter)
    set("filters", next)
  }

  const scopeSummary = form.filters.length ? form.filters.map((filter) => `${scopeFields[form.targetType].find((field) => field.field === filter.field)?.label ?? filter.field} ${operatorOptions[scopeFields[form.targetType].find((field) => field.field === filter.field)?.type ?? "text"].find((operator) => operator.value === filter.operator)?.label ?? filter.operator} ${filter.value}`).join(form.filterLogic === "all" ? " 且 " : " 或 ") : "未添加属性筛选"

  return <Dialog open={open} onOpenChange={(value) => { if (!saving && !previewing) onOpenChange(value) }}><DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
    <DialogHeader><DialogTitle>{rule ? "编辑订阅规则" : "新建订阅规则"}</DialogTitle><DialogDescription>按对象属性圈定范围，再配置触发条件、处置与告警级别。通知消息使用系统内置模板。</DialogDescription></DialogHeader>
    <nav aria-label="规则配置步骤" className="grid grid-cols-4 gap-2 border-b border-border pb-3">{["监控对象", "判定条件", "触发处理", "告警策略"].map((label, index) => <Button key={label} type="button" size="sm" variant={index === step ? "default" : "ghost"} onClick={() => setStep(index)} aria-current={index === step ? "step" : undefined}>{index + 1}. {label}</Button>)}</nav>
    {step === 0 && <div className="space-y-4">
      <Field label="规则名称"><Input aria-label="规则名称" maxLength={128} value={form.name} onChange={(event) => set("name", event.target.value)} placeholder="例如：渠道余额偏低" /></Field>
      <div className="grid gap-4 sm:grid-cols-2"><Field label="对象类型"><select aria-label="对象类型" className={selectClass} value={form.targetType} onChange={(event) => { setForm(targetChanged(form, event.target.value as AlertTargetType)); setScopePreview(null) }}><option value="channel">渠道</option><option value="relay_account">中转站账号</option><option value="relay_user">中转站用户</option></select></Field><Field label="预警指标"><select aria-label="预警指标" className={selectClass} value={form.metric} onChange={(event) => changeMetric(event.target.value as AlertMetric)}>{metrics.map((metric) => <option key={metric} value={metric}>{metric === "balance_low" && form.targetType === "relay_account" ? "旧规则：关联渠道账号余额" : metricLabels[metric]}</option>)}</select></Field></div>
      {form.targetType !== "channel" && <Field label="中转站范围" hint="留空勾选项表示全部中转站；按规则属性筛选出的对象会在每轮评估时重新匹配。"><div className="rounded-md border border-border p-3 space-y-2"><label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={form.stationID === "all"} onChange={(event) => set("stationID", event.target.checked ? "all" : String(stations.data?.[0]?.id ?? "all"))} />全部中转站</label>{stations.error && <p role="alert" className="text-xs text-danger">中转站加载失败：{stations.error}</p>}{stations.loading && <p className="text-xs text-muted-foreground">正在加载中转站…</p>}{stations.data?.map((station) => <label key={station.id} className="flex items-center gap-2 text-sm"><input type="checkbox" checked={stationIDs.includes(String(station.id))} onChange={(event) => { const next = event.target.checked ? [...stationIDs, String(station.id)] : stationIDs.filter((id) => id !== String(station.id)); set("stationID", next.join(",") || "all") }} />{station.name}<span className="text-muted-foreground">#{station.id} · {station.type}</span></label>)}</div></Field>}
      <Field label={form.targetType === "channel" ? "指定渠道 ID（可选）" : form.targetType === "relay_account" ? "指定账号 ID（可选）" : "指定用户 ID（可选）"} hint={form.targetType === "channel" ? "留空时由属性条件动态圈定渠道。" : "留空时由属性条件动态圈定对象；多个远端数字 ID 用逗号分隔。"}><Input aria-label="目标 ID" value={form.targetIDs} onChange={(event) => set("targetIDs", event.target.value)} placeholder="例如：12, 35, 68" /></Field>
      <section className="space-y-3 rounded-md border border-border p-3">
        <div className="flex flex-wrap items-center justify-between gap-2"><div><p className="text-sm font-medium">对象属性筛选</p><p className="mt-1 text-xs text-muted-foreground">对象离开匹配范围后不再评估；范围变化不会触发远端自动恢复。</p></div><div className="flex items-center gap-1" role="group" aria-label="筛选组合方式"><Button type="button" size="sm" variant={form.filterLogic === "all" ? "secondary" : "ghost"} onClick={() => set("filterLogic", "all")}>满足全部</Button><Button type="button" size="sm" variant={form.filterLogic === "any" ? "secondary" : "ghost"} onClick={() => set("filterLogic", "any")}>满足任一</Button></div></div>
        {form.filters.map((filter, index) => {
          const field = scopeFields[form.targetType].find((item) => item.field === filter.field) ?? scopeFields[form.targetType][0]
          const operators = operatorOptions[field.type]
          return <div className="grid items-end gap-2 sm:grid-cols-[1fr_1fr_1fr_auto]" key={`${index}-${filter.field}`}>
            <Field label={index === 0 ? "属性" : ""}><select aria-label={`筛选属性 ${index + 1}`} className={selectClass} value={filter.field} onChange={(event) => { const nextField = scopeFields[form.targetType].find((item) => item.field === event.target.value)!; updateFilter(index, { field: nextField.field, operator: operatorOptions[nextField.type][0].value, value: "" }) }}>{scopeFields[form.targetType].map((item) => <option key={item.field} value={item.field}>{item.label}</option>)}</select></Field>
            <Field label={index === 0 ? "比较方式" : ""}><select aria-label={`筛选比较方式 ${index + 1}`} className={selectClass} value={filter.operator} onChange={(event) => updateFilter(index, { operator: event.target.value })}>{operators.map((operator) => <option key={operator.value} value={operator.value}>{operator.label}</option>)}</select></Field>
            <Field label={index === 0 ? "匹配值" : ""}>{field.type === "boolean" ? <select aria-label={`筛选值 ${index + 1}`} className={selectClass} value={filter.value} onChange={(event) => updateFilter(index, { value: event.target.value })}><option value="">选择状态</option><option value="true">开启</option><option value="false">未开启</option></select> : <Input aria-label={`筛选值 ${index + 1}`} type={field.type === "number" ? "number" : "text"} value={filter.value} onChange={(event) => updateFilter(index, { value: event.target.value })} placeholder={field.type === "time" ? "RFC3339 含时区" : "输入值"} />}</Field>
            <Button type="button" variant="ghost" size="icon" aria-label={`删除筛选条件 ${index + 1}`} title="删除筛选条件" onClick={() => set("filters", form.filters.filter((_, itemIndex) => itemIndex !== index))}>×</Button>
          </div>
        })}
        <div className="flex flex-wrap items-center justify-between gap-2"><Button type="button" variant="outline" size="sm" onClick={addFilter} disabled={form.filters.length >= 20}>＋ 添加筛选条件</Button><Button type="button" variant="outline" size="sm" onClick={() => void previewScope()} disabled={previewing || saving}>{previewing ? "试算中…" : "试算匹配对象"}</Button></div>
        {scopePreview && <div className="border-t border-border pt-3" role="status"><p className="text-sm font-medium">匹配到 {scopePreview.matched} 个对象</p>{scopePreview.targets.length > 0 && <ul className="mt-2 space-y-1 text-xs text-muted-foreground">{scopePreview.targets.slice(0, 5).map((target) => <li key={`${target.station_id}:${target.target_external_id}`}>{form.targetType === "channel" ? "渠道" : `中转站 #${target.station_id}`} · {target.target_name || `#${target.target_external_id}`} · {target.unknown ? "指标数据不足" : target.violated ? "当前命中" : "当前正常"}</li>)}</ul>}{scopePreview.matched === 0 && <p className="mt-1 text-xs text-muted-foreground">没有对象符合当前条件。</p>}</div>}
      </section>
    </div>}
    {step === 1 && <div className="space-y-4">
      {rateChange && <div className="grid gap-4 sm:grid-cols-2"><Field label="变化计算"><select aria-label="变化计算" className={selectClass} value={form.rateMode} onChange={(event) => set("rateMode", event.target.value)}><option value="percent">百分比变化（%）</option><option value="absolute">倍率绝对差值</option></select></Field><Field label="变化方向"><select aria-label="变化方向" className={selectClass} value={form.direction} onChange={(event) => set("direction", event.target.value)}><option value="any">上涨或下跌</option><option value="increase">仅上涨</option><option value="decrease">仅下跌</option></select></Field></div>}
      {form.metric === "account_stability" && <Field label="稳定性指标"><select aria-label="稳定性指标" className={selectClass} value={form.stabilityMetric} onChange={(event) => set("stabilityMetric", event.target.value)}><option value="error_rate">错误率（%）</option><option value="first_token_p95">首字耗时 P95（毫秒）</option><option value="duration_p95">请求耗时 P95（毫秒）</option><option value="consecutive_failures">连续请求失败数</option></select></Field>}
      <div className="grid gap-4 sm:grid-cols-2">{number("threshold", balance ? "余额低于或等于" : form.metric === "user_multi_ip" ? "不同 IP 数达到" : spend ? "窗口累计消耗达到（美元）" : "指标达到阈值", balance ? (form.metric === "balance_low" ? "按渠道当前聚合余额判定；停止监控或余额快照过期时不参与判定。" : "按中转站用户最近一次有效余额快照判定。") : rateChange ? "在统计窗口内按渠道与模型计算倍率变化。" : undefined)}{form.metric !== "balance_low" && form.metric !== "user_balance_low" && number("window", form.metric === "channel_rate_change" ? "倍率变化窗口（秒）" : "统计窗口（秒）", "60–86400 秒。用量预警按滑动窗口聚合。")}</div>
      <div className="grid gap-4 sm:grid-cols-2">{number("trigger", "触发命中次数", "默认连续 3 个有效评估周期命中。重复采样不会增加次数。")}{number("hitWindow", "最近评估次数 M（可选）", "留空表示连续命中；填写后按最近 M 次中命中 N 次触发。")}</div>
      {(spend || form.metric === "account_stability" || form.metric === "user_multi_ip") && number("minRequests", "窗口最小请求数", "请求数不足或采集数据不完整时标记数据不足，不触发自动化。")}
      {spend && number("baseline", "历史消耗倍数（可选）", "需同时超过金额阈值及最近 7 天同窗口平均消耗的指定倍数；至少 3 个完整历史窗口。")}
      {form.metric === "user_multi_ip" && <Field label="可信 IP / CIDR（可选）" hint="每行一个；计数时排除这些地址。"><textarea aria-label="可信 IP" className={`${selectClass} h-24 py-2`} value={form.trustedIPs} onChange={(event) => set("trustedIPs", event.target.value)} /></Field>}
      <div className="grid gap-4 sm:grid-cols-2">{number("recoveryThreshold", "恢复阈值（可选）", balance ? "余额需高于触发阈值，以避免事件反复切换。" : "其他指标需低于触发阈值，以避免事件反复切换。")}{number("recoveryCount", "连续恢复次数", "连续达到正常或恢复阈值后关闭事件；自动化是否恢复远端状态由下一步决定。")}</div>
    </div>}
    {step === 2 && <div className="space-y-4">
      {form.targetType === "channel" ? <p className="rounded-md bg-muted p-3 text-sm text-muted-foreground">渠道规则仅记录事件并发送通知，不会修改渠道状态。</p> : <><Field label="自动化处理"><select aria-label="自动化处理" className={selectClass} value={form.action} onChange={(event) => { set("action", event.target.value); set("autoRecovery", false) }}>{Object.entries(actionLabels).filter(([kind]) => kind === "none" || (form.targetType === "relay_account" ? kind.includes("account") : kind.includes("user"))).map(([kind, label]) => <option key={kind} value={kind}>{label}</option>)}</select></Field>
      {disabling && <div className="rounded-md border border-border p-3"><label className="flex items-center justify-between gap-3 text-sm">条件恢复后自动恢复远端状态<Switch checked={form.autoRecovery} onCheckedChange={(value) => set("autoRecovery", value)} /></label><p className="mt-2 text-xs leading-relaxed text-muted-foreground">仅恢复由本事件停用、状态未被人工修改且没有其他停用规则约束的对象；无法确认状态时保留执行记录，等待人工处理。</p></div>}
      {form.action.startsWith("enable_") && <p className="rounded-md bg-warning/10 p-3 text-xs text-warning">条件命中时直接启用对象；存在其他规则的停用约束时跳过。启用动作不自动反向停用。</p>}</>}
      <Field label="通知渠道" hint="仅向勾选且已启用的渠道发送。留空时只记录事件与执行结果。"><div className="rounded-md border border-border p-3 space-y-2">{channels.error && <p role="alert" className="text-xs text-danger">通知渠道加载失败：{channels.error}</p>}{channels.loading && <p className="text-xs text-muted-foreground">正在加载通知渠道…</p>}{!channels.loading && !channels.error && !channels.data?.length && <p className="text-xs text-muted-foreground">尚无通知渠道，可保存规则后在「通知渠道」中添加。</p>}{channels.data?.map((channel) => <label className="flex items-center gap-2 text-sm" key={channel.id}><input type="checkbox" checked={form.channels.includes(channel.id)} onChange={(event) => set("channels", event.target.checked ? [...form.channels, channel.id] : form.channels.filter((id) => id !== channel.id))} />{channel.name}<span className="text-xs text-muted-foreground">{channel.type}{channel.enabled ? "" : " · 已停用"}</span></label>)}</div></Field>
      {number("cooldown", "异常通知冷却（分钟）", "按对象与事件分别冷却；恢复通知独立发送一次。失败会在后续评估重试。")}
      <div className="rounded-md bg-muted p-3 text-xs leading-6"><p className="font-medium">内置消息内容</p><p>规则名称、预警级别、站点与对象、指标、实际值、阈值、统计窗口、触发时间、自动化结果及事件编号。</p></div>
    </div>}
    {step === 3 && <div className="space-y-4">
      <Field label="预警级别"><select aria-label="预警级别" className={selectClass} value={form.severity} onChange={(event) => set("severity", event.target.value)}><option value="info">提示</option><option value="warning">警告</option><option value="critical">严重</option></select></Field>
      <dl className="grid grid-cols-[100px_1fr] gap-3 rounded-md bg-muted p-4 text-sm"><dt className="text-muted-foreground">规则</dt><dd>{form.name || "尚未填写"}</dd><dt className="text-muted-foreground">对象</dt><dd>{form.targetType === "channel" ? "渠道" : form.targetType === "relay_account" ? "中转站账号" : "中转站用户"} · {form.targetType !== "channel" && (form.stationID === "all" ? "全部中转站" : `中转站 #${form.stationID}`)} · {form.targetIDs || scopeSummary}</dd><dt className="text-muted-foreground">指标</dt><dd>{form.metric === "balance_low" && form.targetType === "relay_account" ? "旧规则：关联渠道账号余额" : metricLabels[form.metric]}</dd><dt className="text-muted-foreground">判定</dt><dd>{form.window} 秒窗口，阈值 {form.threshold}；{form.hitWindow ? `最近 ${form.hitWindow} 次中命中 ${form.trigger} 次` : `连续命中 ${form.trigger} 次`}</dd><dt className="text-muted-foreground">告警级别</dt><dd>{form.severity === "critical" ? "严重" : form.severity === "info" ? "提示" : "警告"}</dd><dt className="text-muted-foreground">处理</dt><dd>{form.targetType === "channel" ? "仅记录与通知" : actionLabels[form.action]}{disabling ? form.autoRecovery ? "，满足条件后自动恢复" : "，无自动恢复" : ""}</dd><dt className="text-muted-foreground">通知</dt><dd>{form.channels.length} 个渠道，冷却 {form.cooldown} 分钟</dd></dl>
      <Field label="备注"><Input aria-label="备注" maxLength={1000} value={form.remark} onChange={(event) => set("remark", event.target.value)} /></Field>
      <label className="flex items-center justify-between text-sm">保存后启用规则<Switch checked={form.enabled} onCheckedChange={(value) => set("enabled", value)} /></label>
      {rule && <p className="text-xs text-muted-foreground">存在未恢复事件时，仅允许修改名称、通知渠道、冷却、备注与启用状态。修改条件或动作前需先处理活动事件。</p>}
    </div>}
    <DialogFooter><Button type="button" variant="outline" disabled={saving || previewing} onClick={() => step ? setStep(step - 1) : onOpenChange(false)}>{step ? "上一步" : "取消"}</Button>{step < 3 ? <Button type="button" onClick={() => setStep(step + 1)}>下一步</Button> : <Button type="button" onClick={() => void save()} disabled={saving}>{saving ? "保存中…" : "保存规则"}</Button>}</DialogFooter>
  </DialogContent></Dialog>
}
