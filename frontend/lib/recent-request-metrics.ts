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
}

const RECENT_WINDOW_MS = 60 * 60 * 1000
const RECENT_CUTOFF_MS = 45 * 1000
const RECENT_LIMIT = 60

function nonNegative(value: number | undefined) {
  return Number.isFinite(value) ? Math.max(0, value ?? 0) : 0
}

function cacheCreationTokens(sample: RecentRequestSample) {
  if (sample.cache_creation_tokens != null) return nonNegative(sample.cache_creation_tokens)
  return nonNegative(sample.cache_creation_5m_tokens) + nonNegative(sample.cache_creation_1h_tokens)
}

export function recentRequestMetrics(samples: readonly RecentRequestSample[], now = Date.now()) {
  const cutoff = now - RECENT_CUTOFF_MS
  const windowStart = cutoff - RECENT_WINDOW_MS
  const selected = samples.map((sample, index) => ({sample, index, time: Date.parse(sample.created_at)}))
    .filter(({time}) => Number.isFinite(time) && time >= windowStart && time <= cutoff)
    .sort((a, b) => b.time - a.time || a.index - b.index).slice(0, RECENT_LIMIT)
  let successCount = 0, successKnownCount = 0, cacheHitCount = 0, cacheKnownCount = 0
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
    if (sample.success_known != null) {
      if (sample.success_known) {
        successKnownCount++
        if (sample.success) successCount++
      }
    } else if ((sample.status_code ?? 0) >= 100) {
      successKnownCount++
      if (sample.status_code! >= 200 && sample.status_code! < 300) successCount++
    }
  }
  const cacheTokenTotal = cacheInputTokens + cacheReadTokens + cacheCreationTotal
  return {
    sampleCount: selected.length, successCount, successKnownCount, cacheHitCount, cacheKnownCount,
    cacheInputTokens, cacheReadTokens, cacheCreationTokens: cacheCreationTotal, cacheTokenTotal,
    successRate: successKnownCount ? successCount / successKnownCount : null,
    cacheHitRate: cacheTokenTotal ? cacheReadTokens / cacheTokenTotal : null,
    latestAt: selected[0]?.sample.created_at ?? null,
  }
}
