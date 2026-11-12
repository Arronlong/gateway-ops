import type { PelicanResult } from "./api-types"
// @ts-expect-error Node 22 原生 TypeScript 测试需要显式扩展名。
import { pelicanResultPath } from "./pelican-sources.ts"

export type PelicanSelectionPoint = {x: number; y: number}
export type PelicanSelectionRect = {left: number; top: number; right: number; bottom: number}

export function pelicanSelectionRect(start: PelicanSelectionPoint, end: PelicanSelectionPoint): PelicanSelectionRect {
  return {left: Math.min(start.x, end.x), top: Math.min(start.y, end.y), right: Math.max(start.x, end.x), bottom: Math.max(start.y, end.y)}
}

export function getPelicanSelection(entries: readonly {key: string; rect: PelicanSelectionRect}[], rect: PelicanSelectionRect, initial: Iterable<string> = []): Set<string> {
  const selected = new Set(initial)
  if (rect.right <= rect.left || rect.bottom <= rect.top) return selected
  for (const entry of entries) {
    const bounds = entry.rect
    if (bounds.right > bounds.left && bounds.bottom > bounds.top && bounds.left < rect.right && bounds.right > rect.left && bounds.top < rect.bottom && bounds.bottom > rect.top) selected.add(entry.key)
  }
  return selected
}

type DeletablePelicanResult = Pick<PelicanResult, "id" | "channel_id" | "relay_station_id" | "account_external_id" | "group_external_id" | "status">

export async function deletePelicanResults<T extends DeletablePelicanResult>(results: readonly T[], remove: (path: string, result: T) => Promise<unknown>) {
  const unique = [...new Map(results.map(result => [pelicanResultPath(result), result])).values()]
  const outcomes: ({result: T; error: unknown} | null)[] = []
  let next = 0
  await Promise.all(Array.from({length: Math.min(4, unique.length)}, async () => {
    while (next < unique.length) {
      const index = next++, result = unique[index]
      try {
        await remove(pelicanResultPath(result), result)
        outcomes[index] = null
      } catch (error) {
        outcomes[index] = {result, error}
      }
    }
  }))
  return {
    deleted: unique.filter((_, index) => outcomes[index] === null),
    failed: outcomes.filter((outcome): outcome is {result: T; error: unknown} => outcome !== null),
  }
}
