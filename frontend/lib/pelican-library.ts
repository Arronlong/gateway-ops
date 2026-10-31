import type { ChannelPelicanGallery, ChannelPelicanGroup, PelicanResult } from "./api-types"
// @ts-expect-error Node 22 原生 TypeScript 测试需要显式扩展名。
import { pelicanEffortOptions } from "./pelican.ts"

export function normalizeChannelPelicanGallery(gallery: {groups: readonly (Omit<ChannelPelicanGroup, "results"> & {results: ChannelPelicanGroup["results"] | null})[]}): ChannelPelicanGallery {
  return {...gallery, groups: gallery.groups.map(group => ({...group, results: group.results ?? []}))}
}

export function getPelicanFilterOptions(groups: readonly {results: readonly Pick<PelicanResult, "model" | "reasoning_effort">[]}[]) {
  const models = new Set<string>()
  const efforts = new Set<string>()
  for (const group of groups) {
    for (const result of group.results) {
      if (result.model.trim()) models.add(result.model)
      if (result.reasoning_effort in pelicanEffortOptions) efforts.add(result.reasoning_effort)
    }
  }
  return {
    models: [...models].sort((left, right) => left.localeCompare(right)),
    efforts: Object.keys(pelicanEffortOptions).filter(effort => efforts.has(effort)),
  }
}

export function matchesPelicanFilters(result: Pick<PelicanResult, "model" | "reasoning_effort">, model: string, effort: string) {
  return (model === "all" || result.model === model) && (effort === "all" || result.reasoning_effort === effort)
}
