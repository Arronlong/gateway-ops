export interface UsageAccountIntegrity {
	retained_count?: number
	history_available_since?: string
	history_verified_until?: string
  account_id: number
  account_name: string
  local_count: number
  upstream_count: number | null
  error?: string
}

export interface UsageIntegrity {
	retained_count?: number
	phase?: "total" | "locating" | "located" | "repairing" | "complete" | "interrupted" | "paused"
	error?: string
	retry_at?: string
	retry_status?: number
	audit?: {
		saved_pages?: number
		saved_rows?: number
		checked_windows: number
		skipped_windows: number
		anomalous_days: number
		repaired_days: number
		windows?: {since:string;until:string;level:string;status:string;local_count:number;upstream_count:number;total_capped?:boolean;counts_pending?:boolean;page?:number;saved_pages?:number;saved_rows?:number;account_id?:number;account_name?:string}[]
	}
  status: "matched" | "mismatch" | "unavailable" | "pending" | "not_applicable"
  local_count: number
  upstream_count: number | null
  since: string
  until: string
  checked_at: string
  accounts: UsageAccountIntegrity[]
}

export function usageIntegritySummary(result: UsageIntegrity | null) {
  const audit=result?.audit
	if (audit?.saved_pages) return `\u5df2\u9010\u9875\u6838\u5bf9\u5e76\u4fdd\u5b58 ${audit.saved_pages} \u9875\uff0c${audit.saved_rows ?? 0} \u6761\u8c03\u7528` + (result?.phase === "paused" || result?.phase === "interrupted" ? "\uff1b\u4e0b\u6b21\u4ece\u8fde\u7eed\u5b8c\u6210\u4f4d\u7f6e\u7ee7\u7eed" : "")
	if (audit && result?.phase === "paused") return `\u5df2\u4fee\u590d ${audit.repaired_days} \u5929 \u00b7 \u5df2\u5b8c\u6210\u7684\u6570\u636e\u5df2\u4fdd\u5b58\uff0c\u7b49\u5f85\u4e0a\u6e38\u6062\u590d\u540e\u7ee7\u7eed`
	if (result?.status === "matched" && result.retained_count) return `\u53ef\u6838\u5bf9\u8303\u56f4\u5185\u6761\u6570\u4e00\u81f4\uff0c\u53e6\u4fdd\u7559 ${result.retained_count} \u6761\u66f4\u65e9\u5386\u53f2`
	if (result?.status === "matched" && audit?.anomalous_days === 0) return "\u603b\u4f53\u6761\u6570\u4e00\u81f4\uff0c\u65e0\u9700\u5206\u6bb5\u6838\u5bf9"
	if (audit && result?.phase === "interrupted") return `\u5df2\u6821\u9a8c ${audit.checked_windows} \u4e2a\u533a\u95f4 \u00b7 \u8df3\u8fc7 ${audit.skipped_windows} \u4e2a\u4e00\u81f4\u533a\u95f4 \u00b7 \u5df2\u5b9a\u4f4d ${audit.anomalous_days} \u5929\uff0c\u5b9a\u4f4d\u672a\u5b8c\u6210` + (audit.repaired_days ? ` \u00b7 \u5df2\u4fee\u590d ${audit.repaired_days} \u5929` : "")
	if (audit && (result?.phase === "total" || result?.phase === "locating")) return `\u5df2\u6821\u9a8c ${audit.checked_windows} \u4e2a\u533a\u95f4 \u00b7 \u8df3\u8fc7 ${audit.skipped_windows} \u4e2a\u4e00\u81f4\u533a\u95f4 \u00b7 \u6b63\u5728\u5b9a\u4f4d\u5dee\u5f02`
	if (audit?.repaired_days) return `\u5df2\u6821\u9a8c ${audit.checked_windows} \u4e2a\u533a\u95f4 \u00b7 \u53d1\u73b0\u5f02\u5e38 ${audit.anomalous_days} \u5929 \u00b7 \u5df2\u4fee\u590d ${audit.repaired_days} \u5929`
	if (result?.status === "mismatch" && audit) return `\u5df2\u6838\u5bf9 ${audit.checked_windows} \u4e2a\u533a\u95f4 \u00b7 \u53d1\u73b0\u603b\u4f53\u5dee\u5f02${audit.anomalous_days ? ` \u00b7 \u5b9a\u4f4d\u5f02\u5e38 ${audit.anomalous_days} \u5929` : ""}`
  return audit ? `\u5df2\u6821\u9a8c ${audit.checked_windows} \u4e2a\u533a\u95f4 \u00b7 \u8df3\u8fc7 ${audit.skipped_windows} \u4e2a\u4e00\u81f4\u533a\u95f4 \u00b7 \u5f02\u5e38 ${audit.anomalous_days} \u5929` : ""
}

export function usageIntegrityLabel(result: UsageIntegrity | null, loading: boolean) {
  if (!result) return loading ? "\u6761\u6570\u6838\u5bf9\u4e2d\u2026" : "\u6682\u65e0\u6cd5\u6838\u5bf9"
	if (result.phase === "paused" && result.upstream_count === null) return result.retry_status === 429 ? "\u4e0a\u6e38\u9650\u6d41\uff0c\u6682\u7f13\u6838\u5bf9" : "\u4e0a\u6e38\u62d2\u7edd\u8bbf\u95ee\uff0c\u6682\u7f13\u6838\u5bf9"
	const local=usageIntegrityComparableCount(result)
	if (result.phase === "interrupted" && result.status === "matched") return `\u53ef\u6838\u5bf9\u6761\u6570\u4e00\u81f4\uff08${local}\uff09\uff0c\u5904\u7406\u672a\u5b8c\u6210`
  if (result.status === "matched") return `${result.retained_count ? "\u53ef\u6838\u5bf9\u6761\u6570\u4e00\u81f4" : "\u6761\u6570\u4e00\u81f4"}\u00b7${local}`
  if (result.status === "mismatch") return result.upstream_count === null ? "\u5df2\u53d1\u73b0\u6761\u6570\u5dee\u5f02\uff0c\u90e8\u5206\u8d26\u53f7\u603b\u6570\u5c1a\u672a\u53d6\u5f97" : `\u6761\u6570\u5dee\u5f02\uff1a\u672c\u5730 ${local} / \u6e20\u9053 ${result.upstream_count} \u00b7 ${usageIntegrityDifference(local,result.upstream_count)}` + (result.audit?.anomalous_days ? ` \u00b7 ${result.audit.anomalous_days} \u5929\u5f02\u5e38` : "")
  if (result.status === "pending") return "\u533a\u95f4\u5c1a\u672a\u7a33\u5b9a"
  return "\u6682\u65e0\u6cd5\u6838\u5bf9"
}

export function usageIntegrityPause(result: UsageIntegrity | null,now=Date.now()) {
	if (result?.phase !== "paused" || !result.retry_at) return ""
	const until=new Date(result.retry_at)
	const seconds=Math.max(0,Math.ceil((until.getTime()-now)/1000))
	const reason=result.retry_status===429 ? "\u4e0a\u6e38\u8bf7\u6c42\u9891\u7387\u53d7\u9650\uff08HTTP 429\uff09" : `\u4e0a\u6e38\u62d2\u7edd\u8bbf\u95ee\uff08HTTP ${result.retry_status ?? 403}\uff09\uff0c\u8bf7\u68c0\u67e5\u8d26\u53f7\u6743\u9650\u6216\u7ad9\u70b9\u8bbf\u95ee\u9650\u5236`
	return `${reason}\uff1b${until.toLocaleTimeString()} \u540e\u53ef\u7ee7\u7eed\uff0c\u5269\u4f59 ${seconds} \u79d2`
}

export function usageIntegrityProgress(event:{message:string;data?:unknown}) {
	if (event.data && typeof event.data === "object" && "kind" in event.data && event.data.kind === "usage_wait" && "retry_at" in event.data && typeof event.data.retry_at === "string") {
		return `\u6b63\u5728\u7b49\u5f85\u4e0a\u6e38\u8bf7\u6c42\u989d\u5ea6\uff0c${new Date(event.data.retry_at).toLocaleTimeString()} \u540e\u81ea\u52a8\u7ee7\u7eed`
	}
	return event.message
}

export function usageIntegrityDifference(local: number, remote: number | null) {
	if (remote === null) return "\u4e0a\u6e38\u6761\u6570\u672a\u53d6\u5f97"
	if (local < remote) return `\u7f3a\u5c11 ${remote-local} \u6761`
	if (local > remote) return `\u591a ${local-remote} \u6761`
	return "\u4e00\u81f4"
}

export function usageIntegrityComparableCount(result:{local_count:number;retained_count?:number}) {
	return result.local_count-(result.retained_count ?? 0)
}

export function usageIntegritySnapshot(data: unknown): UsageIntegrity | null {
	if (!data || typeof data !== "object") return null
	const candidate = "kind" in data && data.kind === "usage_integrity" && "result" in data ? data.result : data
	if (!candidate || typeof candidate !== "object" || !("status" in candidate) || !("accounts" in candidate) || !Array.isArray(candidate.accounts)) return null
	return candidate as UsageIntegrity
}

export function usageIntegrityWindows(result: UsageIntegrity | null) {
	const windows = result?.audit?.windows?.filter(window => window.status !== "matched" && window.level !== "total") ?? []
	const unique = new Map<string, typeof windows[number]>()
	for (const window of windows) {
		const key = `${window.account_id ?? ""}/${window.level}/${window.since}/${window.until}`
		const existing = unique.get(key)
		if (!existing || (window.status === "repaired" && existing.status !== "repaired")) unique.set(key, window)
	}
	const deduplicated = [...unique.values()]
	return deduplicated.filter(window => !deduplicated.some(child => child !== window && child.account_id === window.account_id && child.since >= window.since && child.until <= window.until && (child.since !== window.since || child.until !== window.until || child.level !== window.level)))
}
