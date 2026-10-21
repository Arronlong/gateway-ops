export interface RecentRequestSample {
  created_at: string
  cache_known?: boolean
  input_tokens?: number
  cache_read_tokens?: number
  cache_creation_tokens?: number
  cache_creation_5m_tokens?: number
  cache_creation_1h_tokens?: number
  success_known?: boolean
  success?: boolean
  status_code?: number
  request_type?: string
  is_auxiliary?: boolean
  is_probe?: boolean
  is_cyber?: boolean
  in_progress?: boolean
  attempt_status?: "in_progress" | "completed" | "failed"
}

const DEFAULT_WINDOW_HOURS = 1
const MIN_WINDOW_HOURS = 1
const MAX_WINDOW_HOURS = 24
const RECENT_CUTOFF_MS = 45 * 1000
const RECENT_LIMIT = 60

function nonNegative(value: number | undefined) {
  return Number.isFinite(value) ? Math.max(0, value ?? 0) : 0
}

function excludedFromSuccessRate(sample: RecentRequestSample) {
  const requestType = sample.request_type?.trim().toLowerCase() ?? ""
  return sample.is_auxiliary === true || sample.is_probe === true || sample.is_cyber === true || /(?:^|[_-])(auxiliary|probe|cyber)(?:$|[_-])/.test(requestType)
}

function cacheCreationTokens(sample: RecentRequestSample) {
  if (sample.cache_creation_tokens != null) return nonNegative(sample.cache_creation_tokens)
  return nonNegative(sample.cache_creation_5m_tokens) + nonNegative(sample.cache_creation_1h_tokens)
}

export function recentRequestMetrics(samples: readonly RecentRequestSample[], now = Date.now(), windowHours = DEFAULT_WINDOW_HOURS) {
  const normalizedWindowHours = Number.isInteger(windowHours) ? Math.min(MAX_WINDOW_HOURS, Math.max(MIN_WINDOW_HOURS, windowHours)) : DEFAULT_WINDOW_HOURS
  const cutoff = now - RECENT_CUTOFF_MS
  const windowStart = cutoff - normalizedWindowHours * 60 * 60 * 1000
  const selected = samples.map((sample, index) => ({sample, index, time: Date.parse(sample.created_at)}))
    .filter(({time}) => Number.isFinite(time) && time >= windowStart && time <= cutoff)
    .sort((a, b) => b.time - a.time || a.index - b.index).slice(0, RECENT_LIMIT)
  let successCount = 0, successAttemptCount = 0, cacheHitCount = 0, cacheKnownCount = 0
  let cacheInputTokens = 0, cacheReadTokens = 0, cacheCreationTotal = 0
  for (const {sample} of selected) {
    const inputTokens = nonNegative(sample.input_tokens)
    const readTokens = nonNegative(sample.cache_read_tokens)
    const creationTokens = cacheCreationTokens(sample)
    const tokenTotal = inputTokens + readTokens + creationTokens
    if (tokenTotal > 0) {
      cacheKnownCount++
      cacheInputTokens += inputTokens
      cacheReadTokens += readTokens
      cacheCreationTotal += creationTokens
      if (readTokens > 0) cacheHitCount++
    }
    if (!excludedFromSuccessRate(sample)) {
      const inProgress = sample.in_progress === true || sample.attempt_status === "in_progress"
      const hasStatus = (sample.status_code ?? 0) >= 100
      const isAttempt = inProgress || sample.success_known === true || hasStatus
      if (!isAttempt) continue
      successAttemptCount++
      const succeeded = !inProgress && (sample.success_known === true
        ? sample.success === true
        : (sample.status_code ?? 0) >= 200 && (sample.status_code ?? 0) < 300)
      if (succeeded) successCount++
    }
  }
  const cacheTokenTotal = cacheInputTokens + cacheReadTokens + cacheCreationTotal
  return {
    sampleCount: selected.length, windowHours: normalizedWindowHours, successCount, successAttemptCount, successKnownCount: successAttemptCount, cacheHitCount, cacheKnownCount,
    cacheInputTokens, cacheReadTokens, cacheCreationTokens: cacheCreationTotal, cacheTokenTotal,
    successRate: successAttemptCount ? successCount / successAttemptCount : null,
    cacheHitRate: cacheTokenTotal ? cacheReadTokens / cacheTokenTotal : null,
    latestAt: selected[0]?.sample.created_at ?? null,
  }
}
