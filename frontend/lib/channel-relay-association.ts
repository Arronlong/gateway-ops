import type { Channel, ChannelRelayAssociation, ChannelRelayAssociationSnapshot } from "./api-types"

export type AssociationLinkValue = Pick<ChannelRelayAssociation, "mode" | "linked" | "relay_account_external_id">

export function associationValue(link?: AssociationLinkValue): string {
  if (link?.mode === "manual") {
    return link.linked && link.relay_account_external_id > 0
      ? `manual:${link.relay_account_external_id}`
      : "manual:none"
  }
  return "auto"
}

export function linkedStationsByChannel(snapshot?: Pick<ChannelRelayAssociationSnapshot, "links"> | null) {
  const result = new Map<number, Set<number>>()
  for (const link of snapshot?.links ?? []) {
    if (!link.linked) continue
    const stations = result.get(link.channel_id) ?? new Set<number>()
    stations.add(link.relay_station_id)
    result.set(link.channel_id, stations)
  }
  return result
}


export interface RelayCompareAccount {
  externalID: number
  name: string
  channelAccountID: number
}

export interface RelayCompareLink {
  stationID: number
  stationName: string
  accounts: RelayCompareAccount[]
}

export interface ChannelCompareLink {
  channelID: number
  channelName: string
  accountIDs: number[]
  accountNames: string[]
}

export function relayAccountComparisonKey(stationID: number, accountExternalID: number) {
  return `${stationID}:${accountExternalID}`
}

/**
 * 按渠道聚合「已命中账号」的中转站关联，供渠道指标悬停时并排对照中转站数据。
 * 只收集 linked 且 external_id 大于 0 的记录，未关联的渠道不会出现在结果里。
 */
export function relayComparisonsByChannel(snapshot?: Pick<ChannelRelayAssociationSnapshot, "links"> | null) {
  const byChannel = new Map<number, RelayCompareLink[]>()
  const stationIndex = new Map<number, Map<number, RelayCompareLink>>()
  for (const link of snapshot?.links ?? []) {
    if (!link.linked || link.relay_account_external_id <= 0) continue
    let stations = stationIndex.get(link.channel_id)
    if (!stations) {
      stations = new Map<number, RelayCompareLink>()
      stationIndex.set(link.channel_id, stations)
    }
    let entry = stations.get(link.relay_station_id)
    if (!entry) {
      const list = byChannel.get(link.channel_id) ?? []
      entry = { stationID: link.relay_station_id, stationName: link.relay_station_name, accounts: [] }
      stations.set(link.relay_station_id, entry)
      list.push(entry)
      byChannel.set(link.channel_id, list)
    }
    if (entry.accounts.some((account) => account.externalID === link.relay_account_external_id)) continue
    entry.accounts.push({
      externalID: link.relay_account_external_id,
      name: link.relay_account_name?.trim() || "账号 " + link.relay_account_external_id,
      channelAccountID: link.channel_account_id,
    })
  }
  for (const list of byChannel.values()) {
    list.sort((left, right) => left.stationID - right.stationID)
    for (const entry of list) entry.accounts.sort((left, right) => left.externalID - right.externalID)
  }
  return byChannel
}

/**
 * 按中转站账号反向聚合「已命中渠道」的关联，供中转站账号指标悬停时并排对照渠道数据。
 * 同一渠道的多个渠道账号只展示一次，但会保留渠道账号用户名作为辅助说明。
 */
export function channelComparisonsByRelayAccount(
  snapshot?: Pick<ChannelRelayAssociationSnapshot, "links"> | null,
  channels: Pick<Channel, "id" | "name">[] = [],
) {
  const channelNames = new Map(channels.map((channel) => [channel.id, channel.name]))
  const byAccount = new Map<string, ChannelCompareLink[]>()
  const accountIndex = new Map<string, Map<number, ChannelCompareLink>>()

  for (const link of snapshot?.links ?? []) {
    if (!link.linked || link.relay_account_external_id <= 0) continue
    const key = relayAccountComparisonKey(link.relay_station_id, link.relay_account_external_id)
    let channelsForAccount = accountIndex.get(key)
    if (!channelsForAccount) {
      channelsForAccount = new Map<number, ChannelCompareLink>()
      accountIndex.set(key, channelsForAccount)
    }
    let entry = channelsForAccount.get(link.channel_id)
    if (!entry) {
      const list = byAccount.get(key) ?? []
      entry = {
        channelID: link.channel_id,
        channelName: channelNames.get(link.channel_id)?.trim() || `渠道 ${link.channel_id}`,
        accountIDs: [],
        accountNames: [],
      }
      channelsForAccount.set(link.channel_id, entry)
      list.push(entry)
      byAccount.set(key, list)
    }
    const accountName = link.channel_account_username.trim() || `账号 ${link.channel_account_id}`
    if (!entry.accountIDs.includes(link.channel_account_id)) entry.accountIDs.push(link.channel_account_id)
    if (!entry.accountNames.includes(accountName)) entry.accountNames.push(accountName)
  }

  for (const list of byAccount.values()) {
    list.sort((left, right) => left.channelID - right.channelID)
    for (const entry of list) {
      entry.accountIDs.sort((left, right) => left - right)
      entry.accountNames.sort((left, right) => left.localeCompare(right, "zh-CN", { numeric: true }))
    }
  }
  return byAccount
}
