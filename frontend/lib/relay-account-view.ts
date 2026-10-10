export type RelayAccountViewMode = "gallery" | "list"

interface RelayAccountModelSearchSource {
  model_type?: string
  current_groups?: Array<{ model_types?: string[] }>
  latency_samples?: Array<{ model?: string }>
}

interface RelayAccountSearchTextSource extends RelayAccountModelSearchSource {
  external_id: number
  name: string
  base_url?: string
  platform?: string
  type?: string
}

export function relayAccountDisplayBucketCount(mode: RelayAccountViewMode) {
  return mode === "gallery" ? 60 : 30
}

export function latestRelayLatencySamples<T extends { created_at: string }>(samples: T[], limit: number) {
  return [...samples]
    .sort((left, right) => new Date(left.created_at).getTime() - new Date(right.created_at).getTime())
    .slice(-limit)
}

export function relayAccountSearchModelTypes(account: RelayAccountModelSearchSource, supportedModels: string[] = []) {
  return Array.from(new Set([
    account.model_type,
    ...(account.latency_samples ?? []).map((sample) => sample.model),
    ...supportedModels,
  ].map((value) => value?.trim()).filter((value): value is string => Boolean(value))))
}

export function relayAccountSearchText(account: RelayAccountSearchTextSource, supportedModels: string[] = [], baseURLHost = "") {
  return [
    account.name,
    account.base_url ?? "",
    baseURLHost,
    account.platform ?? "",
    account.type ?? "",
    ...relayAccountSearchModelTypes(account, supportedModels),
    String(account.external_id),
  ].join("\u0000").toLocaleLowerCase()
}
