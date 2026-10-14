export interface RecentRequestSample {
  created_at: string
  cache_known?: boolean
  cache_read_tokens?: number
  success_known?: boolean
  success?: boolean
  status_code?: number
}
export function recentRequestMetrics(samples: readonly RecentRequestSample[], now = Date.now()) {
  const windowStart = now - 60 * 60 * 1000
  const selected = samples.map((sample, index) => ({sample, index, time: Date.parse(sample.created_at)}))
    .filter(({time}) => Number.isFinite(time) && time >= windowStart && time <= now)
    .sort((a, b) => b.time - a.time || a.index - b.index).slice(0, 60)
  let successCount = 0, successKnownCount = 0, cacheHitCount = 0, cacheKnownCount = 0, cacheReadTokens = 0
  for (const {sample} of selected) {
    const readTokens = Math.max(0, sample.cache_read_tokens ?? 0)
    if (sample.cache_known === true || (sample.cache_known == null && readTokens > 0)) {
      cacheKnownCount++
      if (readTokens > 0) cacheHitCount++
      cacheReadTokens += readTokens
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
  return {
    sampleCount: selected.length, successCount, successKnownCount, cacheHitCount, cacheKnownCount, cacheReadTokens,
    successRate: successKnownCount ? successCount / successKnownCount : null,
    cacheHitRate: cacheKnownCount ? cacheHitCount / cacheKnownCount : null,
    latestAt: selected[0]?.sample.created_at ?? null,
  }
}
