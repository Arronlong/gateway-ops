export interface RelayCompareBucket {
  start: string
  end: string
  request_count: number
}

/**
 * 在渠道时间桶与中转站账号时间桶之间做对齐。
 *
 * 两侧后端的窗口切分规则一致（从 since 起按桶数等分），但两次请求的 until 会
 * 截断到各自的“分钟”，因此同一个业务时段在两边的 start 可能相差几分钟，
 * 无法只靠字符串相等匹配。优先级：同 start -> 时间区间覆盖 -> 最近的 start。
 *
 * 未提供目标时间桶（例如“最近调用”这类没有单一时间点的视图）时，
 * 回退到最近一个有请求的桶，便于展示最近一次中转站数据。
 */
export function pickRelayCompareBucket<T extends RelayCompareBucket>(buckets: T[] | undefined, start?: string | null): T | null {
  if (!buckets || buckets.length === 0) return null
  if (start) {
    const exact = buckets.find((bucket) => bucket.start === start)
    if (exact) return exact
    const target = Date.parse(start)
    if (Number.isFinite(target)) {
      const covering = buckets.find((bucket) => {
        const from = Date.parse(bucket.start)
        const to = Date.parse(bucket.end)
        return Number.isFinite(from) && Number.isFinite(to) && from <= target && target < to
      })
      if (covering) return covering
      let nearest: T | null = null
      let nearestGap = Number.POSITIVE_INFINITY
      for (const bucket of buckets) {
        const gap = Math.abs(Date.parse(bucket.start) - target)
        if (Number.isFinite(gap) && gap < nearestGap) {
          nearestGap = gap
          nearest = bucket
        }
      }
      if (nearest) return nearest
    }
  }
  for (const bucket of [...buckets].reverse()) {
    if (bucket.request_count > 0) return bucket
  }
  return buckets[buckets.length - 1] ?? null
}
