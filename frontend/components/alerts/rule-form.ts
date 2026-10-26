import type { AlertMetric, AlertRule, AlertScopeFilter, AlertTargetType } from "../../lib/api-types"

export interface RuleForm {
  name: string; targetType: AlertTargetType; metric: AlertMetric; stationID: string; targetIDs: string
  threshold: string; window: string; minRequests: string; trigger: string; recoveryCount: string
  hitWindow: string; recoveryThreshold: string; baseline: string; trustedIPs: string
  rateMode: string; direction: string; stabilityMetric: string; action: string; autoRecovery: boolean
  channels: number[]; enabled: boolean; severity: string; cooldown: string; remark: string
  filterLogic: "all" | "any"; filters: AlertScopeFilter[]
}

function parse(raw?: string): Record<string, unknown> {
  try { const value = JSON.parse(raw || "{}"); return value && typeof value === "object" ? value : {} } catch { return {} }
}

export function initialRuleForm(rule?: AlertRule | null): RuleForm {
  const condition = parse(rule?.condition), scope = parse(rule?.scope), recovery = parse(rule?.recovery)
  const actions = parse(rule?.action).actions as { kind: string; enabled: boolean }[] | undefined
  return {
    name: rule?.name ?? "", targetType: rule?.target_type ?? "relay_account", metric: rule?.metric ?? "account_spend",
    stationID: (scope.station_ids as number[])?.length ? (scope.station_ids as number[]).join(",") : "all", targetIDs: (scope.target_ids as string[])?.join(", ") ?? "",
    threshold: String(condition.threshold ?? 20), window: String(condition.window_seconds ?? 900), minRequests: String(condition.min_requests ?? 0),
    trigger: String(condition.trigger_consecutive ?? 3), recoveryCount: String(recovery.consecutive || condition.recovery_consecutive || 3), hitWindow: condition.hit_window ? String(condition.hit_window) : "",
    recoveryThreshold: recovery.threshold != null ? String(recovery.threshold) : condition.recovery_threshold != null ? String(condition.recovery_threshold) : "",
    baseline: condition.baseline_multiplier ? String(condition.baseline_multiplier) : "", trustedIPs: (condition.trusted_ips as string[])?.join("\n") ?? "",
    rateMode: String(condition.rate_mode || "percent"), direction: String(condition.direction || "any"), stabilityMetric: String(condition.stability_metric || "error_rate"),
    action: actions?.find((item) => item.enabled)?.kind ?? "none", autoRecovery: recovery.enabled === true, channels: [...(rule?.channel_ids ?? [])],
    enabled: rule?.enabled ?? true, severity: rule?.severity ?? "warning", cooldown: String((rule?.cooldown_seconds ?? 1800) / 60), remark: rule?.remark ?? "",
    filterLogic: scope.logic === "any" ? "any" : "all", filters: Array.isArray(scope.filters) ? scope.filters as AlertScopeFilter[] : [],
  }
}

export function targetChanged(form: RuleForm, targetType: AlertTargetType): RuleForm {
  return { ...form, targetType, metric: targetType === "channel" ? "balance_low" : targetType === "relay_user" ? "user_spend" : "account_spend", action: "none", autoRecovery: false, targetIDs: "", filters: [], filterLogic: "all", baseline: "", trustedIPs: "", minRequests: "0", recoveryThreshold: "" }
}

export function rulePayload(form: RuleForm) {
  const num = (raw: string, label: string, min: number, max: number, integer = false) => {
    const value = Number(raw)
    if (!raw.trim() || !Number.isFinite(value) || value < min || value > max || (integer && !Number.isInteger(value))) throw new Error(`${label}超出有效范围`)
    return value
  }
  if (!form.name.trim() || form.name.trim().length > 128) throw new Error("请填写 1–128 字的规则名称")
  if (form.filters.length > 20 || form.filters.some((filter) => !filter.field || !filter.operator || !filter.value.trim() || Array.from(filter.value).length > 256)) throw new Error("请完整填写所有对象筛选条件，每条不超过 256 字")
  const isBalance = form.metric === "balance_low" || form.metric === "user_balance_low"
  const threshold = num(form.threshold, "阈值", isBalance ? 0 : Number.MIN_VALUE, 1e12)
  if (form.metric === "user_multi_ip" && (!Number.isInteger(threshold) || threshold < 2)) throw new Error("IP 阈值必须是大于等于 2 的整数")
  const trigger = num(form.trigger, "触发次数", 1, 100, true)
  const hitWindow = form.hitWindow.trim() ? num(form.hitWindow, "评估次数窗口", trigger, 100, true) : 0
  const stationIDs = form.stationID === "all" ? [] : [...new Set(form.stationID.split(",").map((id) => num(id, "中转站 ID", 1, 2 ** 32 - 1, true)))]
  const targetIDs = [...new Set(form.targetIDs.split(/[,，\s]+/).filter(Boolean))]
  if (form.targetType === "channel" && targetIDs.some((id) => !/^[1-9]\d*$/.test(id))) throw new Error("渠道 ID 必须是正整数")
  if (targetIDs.length && form.targetType !== "channel" && stationIDs.length !== 1) throw new Error("指定账号或用户 ID 时必须选择一个中转站")
  if (form.targetType === "channel" && stationIDs.length) throw new Error("渠道规则不选择中转站")
  if (form.targetType === "channel" && form.action !== "none") throw new Error("渠道规则暂不支持自动化动作")
  if (form.action !== "none" && targetIDs.some((id) => !/^[1-9]\d*$/.test(id))) throw new Error("自动化目标必须使用远端数字 ID，不能使用邮箱")
  const recoveryThreshold = form.recoveryThreshold.trim() ? num(form.recoveryThreshold, "恢复阈值", 0, 1e12) : undefined
  if (recoveryThreshold != null && (isBalance ? recoveryThreshold <= threshold : recoveryThreshold >= threshold)) throw new Error("恢复阈值需留出滞回区间：余额高于触发值，其他指标低于触发值")
  const enableAction = form.action === "enable_user" || form.action === "enable_account_scheduling"
  return {
    name: form.name.trim(), target_type: form.targetType, metric: form.metric, severity: form.severity, enabled: form.enabled,
    scope: { station_ids: stationIDs, target_ids: targetIDs, ...(form.filters.length || form.filterLogic === "any" ? { logic: form.filterLogic, filters: form.filters } : {}) },
    condition: { threshold, window_seconds: num(form.window, "时间窗口", 60, 86400, true), min_requests: ["rate_change", "channel_rate_change", "balance_low", "user_balance_low"].includes(form.metric) ? 0 : num(form.minRequests, "最小请求数", 0, 1000000, true), trigger_consecutive: trigger, recovery_consecutive: num(form.recoveryCount, "恢复次数", 1, 100, true), hit_window: hitWindow, recovery_threshold: recoveryThreshold,
      rate_mode: form.rateMode, direction: form.direction, stability_metric: form.stabilityMetric,
      baseline_multiplier: form.baseline.trim() ? num(form.baseline, "历史基线倍数", 1.001, 1e6) : undefined,
      trusted_ips: form.trustedIPs.split(/[,，\s]+/).filter(Boolean) },
    action: { actions: form.action === "none" ? [] : [{ kind: form.action, enabled: true }] },
    recovery: { enabled: !enableAction && form.autoRecovery, threshold: recoveryThreshold, consecutive: num(form.recoveryCount, "恢复次数", 1, 100, true) },
    cooldown_seconds: num(form.cooldown, "通知冷却分钟数", 1, 10080) * 60, channel_ids: [...new Set(form.channels)], remark: form.remark,
  }
}

export const metricLabels: Record<AlertMetric, string> = { channel_rate_change: "倍率变化", rate_change: "倍率变化（旧规则）", balance_low: "渠道余额低", account_spend: "账号消耗异常", account_stability: "账号稳定性异常", user_spend: "用户消耗异常", user_multi_ip: "用户多 IP", user_balance_low: "用户余额低" }
export const actionLabels: Record<string, string> = { none: "仅记录与通知", disable_account_scheduling: "自动停止账号调度", enable_account_scheduling: "自动恢复账号调度", disable_user: "自动停用用户", enable_user: "自动启用用户" }
export const stateLabels: Record<string, string> = { normal: "正常", pending: "待触发", firing: "触发中", recovering: "恢复中", recovered: "已恢复", unknown: "数据不足" }
