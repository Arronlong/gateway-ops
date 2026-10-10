import type { Channel } from "@/lib/api-types"

function hostOf(value: string) {
  try {
    return new URL(value).host
  } catch {
    return value
  }
}

export function channelMatchesSearch(channel: Channel, rawQuery: string, associatedModelNames: readonly string[] = []) {
  const query = rawQuery.trim().toLocaleLowerCase()
  if (!query) return true

  return [
    channel.name,
    channel.remark ?? "",
    channel.site_url,
    hostOf(channel.site_url),
    ...(channel.model_names ?? []),
    ...associatedModelNames,
  ].some((value) => value.toLocaleLowerCase().includes(query))
}
