"use client"

import { useEffect, useId, useState, type ReactNode } from "react"
import { toast } from "sonner"
import { apiFetch } from "@/lib/api"
import { useNotificationChannels, useRelayStations } from "@/lib/queries"
import type { AlertMetric, AlertRule, AlertTargetType } from "@/lib/api-types"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { actionLabels, initialRuleForm, metricLabels, rulePayload, targetChanged, type RuleForm } from "./rule-form"

const selectClass = "h-9 w-full rounded-md border border-input bg-background px-3 text-sm"

function Field({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  const id = useId()
  return <div className="space-y-1.5"><Label htmlFor={id}>{label}</Label><div id={id}>{children}</div>{hint && <p className="text-xs leading-relaxed text-muted-foreground">{hint}</p>}</div>
}

export function AlertRuleDialog({ open, onOpenChange, rule, onSaved }: { open: boolean; onOpenChange: (value: boolean) => void; rule?: AlertRule | null; onSaved: () => void }) {
  const channels = useNotificationChannels(), stations = useRelayStations()
  const [form, setForm] = useState<RuleForm>(() => initialRuleForm())
  const [step, setStep] = useState(0), [saving, setSaving] = useState(false)
  useEffect(() => { if (open) { setForm(initialRuleForm(rule)); setStep(0) } }, [open, rule])
  const set = <K extends keyof RuleForm>(key: K, value: RuleForm[K]) => setForm((current) => ({ ...current, [key]: value }))
  const number = (key: keyof RuleForm, label: string, hint?: string) => <Field label={label} hint={hint}><Input aria-label={label} type="number" step="any" value={String(form[key])} onChange={(event) => set(key, event.target.value)} /></Field>
  const spend = form.metric === "account_spend" || form.metric === "user_spend"
  const disabling = form.action === "disable_account_scheduling" || form.action === "disable_user"
  const stationIDs = form.stationID === "all" ? [] : form.stationID.split(",")
  const metrics = (Object.keys(metricLabels) as AlertMetric[]).filter((metric) => form.targetType === "relay_user" ? metric.startsWith("user_") : !metric.startsWith("user_"))
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
  function changeMetric(metric: AlertMetric) {
    setForm((current) => ({ ...current, metric, threshold: metric === "user_multi_ip" ? "3" : metric === "balance_low" ? "10" : "20", baseline: "", recoveryThreshold: "", trustedIPs: "", minRequests: metric === "rate_change" || metric === "balance_low" ? "0" : current.minRequests }))
  }
  return <Dialog open={open} onOpenChange={(value) => { if (!saving) onOpenChange(value) }}><DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
    <DialogHeader><DialogTitle>{rule ? "编辑订阅规则" : "新建订阅规则"}</DialogTitle><DialogDescription>针对中转站账号和用户配置条件、自动化与通知。消息内容由系统统一生成。</DialogDescription></DialogHeader>
    <nav aria-label="规则配置步骤" className="grid grid-cols-4 gap-2 border-b border-border pb-3">{["监控对象", "判定条件", "触发处理", "确认配置"].map((label, index) => <Button key={label} type="button" size="sm" variant={index === step ? "default" : "ghost"} onClick={() => setStep(index)} aria-current={index === step ? "step" : undefined}>{index + 1}. {label}</Button>)}</nav>
    {step === 0 && <div className="space-y-4">
      <Field label="规则名称"><Input aria-label="规则名称" maxLength={128} value={form.name} onChange={(event) => set("name", event.target.value)} placeholder="例如：账号余额不足自动停止调度" /></Field>
      <div className="grid gap-4 sm:grid-cols-2"><Field label="对象类型"><select aria-label="对象类型" className={selectClass} value={form.targetType} onChange={(event) => setForm(targetChanged(form, event.target.value as AlertTargetType))}><option value="relay_account">中转站账号</option><option value="relay_user">中转站用户</option></select></Field><Field label="预警指标"><select aria-label="预警指标" className={selectClass} value={form.metric} onChange={(event) => changeMetric(event.target.value as AlertMetric)}>{metrics.map((metric) => <option key={metric} value={metric}>{metricLabels[metric]}</option>)}</select></Field></div>
      <Field label="中转站范围" hint="不选择具体中转站时覆盖全部中转站。指定账号或用户 ID 时，只能选择一个中转站。">
        <div className="rounded-md border border-border p-3 space-y-2"><label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={form.stationID === "all"} onChange={(event) => set("stationID", event.target.checked ? "all" : String(stations.data?.[0]?.id ?? "all"))} />全部中转站</label>
          {stations.error && <p role="alert" className="text-xs text-danger">中转站加载失败：{stations.error}</p>}{stations.loading && <p className="text-xs text-muted-foreground">正在加载中转站…</p>}
          {stations.data?.map((station) => <label key={station.id} className="flex items-center gap-2 text-sm"><input type="checkbox" checked={stationIDs.includes(String(station.id))} onChange={(event) => { const next = event.target.checked ? [...stationIDs, String(station.id)] : stationIDs.filter((id) => id !== String(station.id)); set("stationID", next.join(",") || "all") }} />{station.name}<span className="text-muted-foreground">#{station.id}</span></label>)}
        </div>
      </Field>
      <Field label={form.targetType === "relay_account" ? "账号 ID" : "用户 ID"} hint="留空表示所选中转站的全部对象；多个远端 ID 用逗号分隔。自动化使用远端数字 ID。"><Input aria-label="目标 ID" value={form.targetIDs} onChange={(event) => set("targetIDs", event.target.value)} placeholder="例如：12, 35, 68" /></Field>
      <Field label="预警级别"><select aria-label="预警级别" className={selectClass} value={form.severity} onChange={(event) => set("severity", event.target.value)}><option value="info">提示</option><option value="warning">警告</option><option value="critical">严重</option></select></Field>
    </div>}
    {step === 1 && <div className="space-y-4">
      {form.metric === "rate_change" && <div className="grid gap-4 sm:grid-cols-2"><Field label="变化计算"><select aria-label="变化计算" className={selectClass} value={form.rateMode} onChange={(event) => set("rateMode", event.target.value)}><option value="percent">百分比变化（%）</option><option value="absolute">倍率绝对差值</option></select></Field><Field label="变化方向"><select aria-label="变化方向" className={selectClass} value={form.direction} onChange={(event) => set("direction", event.target.value)}><option value="any">上涨或下跌</option><option value="increase">仅上涨</option><option value="decrease">仅下跌</option></select></Field></div>}
      {form.metric === "account_stability" && <Field label="稳定性指标"><select aria-label="稳定性指标" className={selectClass} value={form.stabilityMetric} onChange={(event) => set("stabilityMetric", event.target.value)}><option value="error_rate">错误率（%）</option><option value="first_token_p95">首字耗时 P95（毫秒）</option><option value="duration_p95">请求耗时 P95（毫秒）</option><option value="consecutive_failures">连续请求失败数</option></select></Field>}
      <div className="grid gap-4 sm:grid-cols-2">{number("threshold", form.metric === "balance_low" ? "余额低于或等于" : form.metric === "user_multi_ip" ? "不同 IP 数达到" : spend ? "窗口累计消耗达到（美元）" : "指标达到阈值", form.metric === "balance_low" ? "按账号关联渠道的最低有效余额判定。" : form.metric === "rate_change" ? "以最近正常倍率为基准，异常期间冻结基准。" : undefined)}{number("window", "统计窗口（秒）", "60–86400 秒。每 60 秒评估一次；余额每 300 秒采集一次。")}</div>
      <div className="grid gap-4 sm:grid-cols-2">{number("trigger", "触发命中次数", "默认连续 3 个有效采样命中。重复采样不会增加次数。")}{number("hitWindow", "最近评估次数 M（可选）", "留空：连续命中；填写：最近 M 次中命中上述 N 次。")}</div>
      {(spend || form.metric === "account_stability" || form.metric === "user_multi_ip") && number("minRequests", "窗口最小请求数", "请求数不足或采集数据不完整时标记数据不足，不触发自动化。")}
      {spend && number("baseline", "历史消耗倍数（可选）", "需同时超过金额阈值及最近 7 天同窗口平均消耗的指定倍数；至少 3 个完整历史窗口。")}
      {form.metric === "user_multi_ip" && <Field label="可信 IP / CIDR（可选）" hint="每行一个，例如 192.0.2.1 或 2001:db8::/32；计数时排除这些地址。"><textarea aria-label="可信 IP" className={`${selectClass} h-24 py-2`} value={form.trustedIPs} onChange={(event) => set("trustedIPs", event.target.value)} /></Field>}
      <div className="grid gap-4 sm:grid-cols-2">{number("recoveryThreshold", "恢复阈值（可选）", "余额需高于触发阈值，其他指标需低于触发阈值，以避免反复切换。")}{number("recoveryCount", "连续恢复次数", "连续达到正常或恢复阈值后关闭事件；是否恢复远端状态由下一步决定。")}</div>
    </div>}
    {step === 2 && <div className="space-y-4">
      <Field label="自动化处理"><select aria-label="自动化处理" className={selectClass} value={form.action} onChange={(event) => { set("action", event.target.value); set("autoRecovery", false) }}>{Object.entries(actionLabels).filter(([kind]) => kind === "none" || (form.targetType === "relay_account" ? kind.includes("account") : kind.includes("user"))).map(([kind, label]) => <option key={kind} value={kind}>{label}</option>)}</select></Field>
      {disabling && <div className="rounded-md border border-border p-3"><label className="flex items-center justify-between gap-3 text-sm">条件恢复后自动恢复远端状态<Switch checked={form.autoRecovery} onCheckedChange={(value) => set("autoRecovery", value)} /></label><p className="mt-2 text-xs leading-relaxed text-muted-foreground">{form.autoRecovery ? "仅恢复由本事件停用、状态未被人工修改且没有其他停用规则约束的对象；无法确认状态时保留执行记录，等待人工处理。" : "事件恢复后保持远端停用状态，由你手动恢复。"}</p></div>}
      {form.action.startsWith("enable_") && <p className="rounded-md bg-warning/10 p-3 text-xs text-warning">条件命中时直接启用对象；存在其他规则的停用约束时跳过。启用动作不自动反向停用。</p>}
      <Field label="通知渠道" hint="仅向勾选且已启用的渠道发送。留空时只记录事件与执行结果。"><div className="rounded-md border border-border p-3 space-y-2">{channels.error && <p role="alert" className="text-xs text-danger">通知渠道加载失败：{channels.error}</p>}{channels.loading && <p className="text-xs text-muted-foreground">正在加载通知渠道…</p>}{!channels.loading && !channels.error && !channels.data?.length && <p className="text-xs text-muted-foreground">尚无通知渠道，可保存规则后在「通知渠道」中添加。</p>}{channels.data?.map((channel) => <label className="flex items-center gap-2 text-sm" key={channel.id}><input type="checkbox" checked={form.channels.includes(channel.id)} onChange={(event) => set("channels", event.target.checked ? [...form.channels, channel.id] : form.channels.filter((id) => id !== channel.id))} />{channel.name}<span className="text-xs text-muted-foreground">{channel.type}{channel.enabled ? "" : " · 已停用"}</span></label>)}</div></Field>
      {number("cooldown", "异常通知冷却（分钟）", "按事件与渠道分别冷却；恢复通知独立发送一次。失败会在后续评估重试。")}
      <div className="rounded-md bg-muted p-3 text-xs leading-6"><p className="font-medium">内置消息内容</p><p>规则名称、预警级别、中转站与对象、指标、实际值、阈值、统计窗口、触发时间、自动化结果及事件编号。触发与恢复使用统一格式。</p></div>
    </div>}
    {step === 3 && <div className="space-y-4">
      <dl className="grid grid-cols-[100px_1fr] gap-3 rounded-md bg-muted p-4 text-sm"><dt className="text-muted-foreground">规则</dt><dd>{form.name || "尚未填写"}</dd><dt className="text-muted-foreground">监控</dt><dd>{metricLabels[form.metric]} · {form.stationID === "all" ? "全部中转站" : `中转站 #${form.stationID}`} · {form.targetIDs || "全部对象"}</dd><dt className="text-muted-foreground">判定</dt><dd>{form.window} 秒窗口，阈值 {form.threshold}；{form.hitWindow ? `最近 ${form.hitWindow} 次中命中 ${form.trigger} 次` : `连续命中 ${form.trigger} 次`}</dd><dt className="text-muted-foreground">恢复</dt><dd>连续正常 {form.recoveryCount} 次{form.recoveryThreshold ? `，恢复阈值 ${form.recoveryThreshold}` : ""}</dd><dt className="text-muted-foreground">处理</dt><dd>{actionLabels[form.action]}{disabling ? form.autoRecovery ? "，满足条件后自动恢复" : "，无自动恢复" : ""}</dd><dt className="text-muted-foreground">通知</dt><dd>{form.channels.length} 个渠道，冷却 {form.cooldown} 分钟</dd></dl>
      <Field label="备注"><Input aria-label="备注" maxLength={1000} value={form.remark} onChange={(event) => set("remark", event.target.value)} /></Field>
      <label className="flex items-center justify-between text-sm">保存后启用规则<Switch checked={form.enabled} onCheckedChange={(value) => set("enabled", value)} /></label>
      {rule && <p className="text-xs text-muted-foreground">存在未恢复事件时，仅允许修改名称、通知渠道、冷却、备注与启用状态。修改条件或动作前需先处理活动事件。</p>}
    </div>}
    <DialogFooter><Button type="button" variant="outline" disabled={saving} onClick={() => step ? setStep(step - 1) : onOpenChange(false)}>{step ? "上一步" : "取消"}</Button>{step < 3 ? <Button type="button" onClick={() => setStep(step + 1)}>下一步</Button> : <Button type="button" onClick={() => void save()} disabled={saving}>{saving ? "保存中…" : "保存规则"}</Button>}</DialogFooter>
  </DialogContent></Dialog>
}
