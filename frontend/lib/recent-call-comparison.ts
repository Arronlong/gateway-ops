export interface RecentCallComparable {
  created_at: string
  model?: string
  input_tokens?: number
  output_tokens?: number
  cache_read_tokens?: number
  cache_creation_tokens?: number
  cache_creation_5m_tokens?: number
  cache_creation_1h_tokens?: number
}
function timestamp(sample: RecentCallComparable) {
  const value = Date.parse(sample.created_at)
  return Number.isFinite(value) ? value : 0
}
function totalTokens(sample: RecentCallComparable) {
  return (sample.input_tokens ?? 0)
    + (sample.output_tokens ?? 0)
    + (sample.cache_read_tokens ?? 0)
    + (sample.cache_creation_tokens ?? 0)
    + (sample.cache_creation_5m_tokens ?? 0)
    + (sample.cache_creation_1h_tokens ?? 0)
}

function ordered<T extends RecentCallComparable>(samples: readonly T[]) {
  return [...samples].sort((left, right) => timestamp(left) - timestamp(right))
}

function sameSample(left: RecentCallComparable, right: RecentCallComparable) {
  return left === right || (
    left.created_at === right.created_at
    && (left.model ?? "") === (right.model ?? "")
    && totalTokens(left) === totalTokens(right)
  )
}

export function matchRelatedRecentCall<T extends RecentCallComparable, U extends RecentCallComparable>(
  sample: T,
  primarySamples: readonly T[],
  counterpartSamples: readonly U[],
): U | null {
  if (counterpartSamples.length === 0) return null

  const sampleTime = timestamp(sample)
  const model = sample.model?.trim().toLocaleLowerCase() ?? ""
  const tokens = totalTokens(sample)
  const closeCandidates = counterpartSamples
    .map((candidate) => ({
      candidate,
      distance: Math.abs(timestamp(candidate) - sampleTime),
      sameModel: model !== "" && candidate.model?.trim().toLocaleLowerCase() === model,
      sameTokens: tokens > 0 && totalTokens(candidate) === tokens,
    }))
    .filter((item) => item.distance <= 10 * 60 * 1000 && (item.sameModel || item.sameTokens))
    .sort((left, right) => Number(right.sameModel) - Number(left.sameModel)
      || Number(right.sameTokens) - Number(left.sameTokens)
      || left.distance - right.distance)

  if (closeCandidates.length > 0) return closeCandidates[0].candidate

  const primary = ordered(primarySamples)
  const counterpart = ordered(counterpartSamples)
  const primaryIndex = primary.findIndex((candidate) => sameSample(candidate, sample))
  if (primaryIndex < 0) return counterpart[counterpart.length - 1] ?? null
  if (primary.length === 1 || counterpart.length === 1) return counterpart[counterpart.length - 1] ?? null
  const relativeIndex = primaryIndex / (primary.length - 1)
  return counterpart[Math.round(relativeIndex * (counterpart.length - 1))] ?? null
}
