interface StatisticsIdentity {
  credential_mode?: string
  username: string
  newapi_user_id?: string
}

interface StatisticsChannel extends StatisticsIdentity {
  id?: number
  name?: string
  type: string
  site_url: string
  balance_mode?: string
  exclude_from_statistics?: boolean
  accounts?: readonly StatisticsIdentity[]
}

function statisticsSite(value: string) {
  try {
    const url = new URL(value.trim())
    if (!["http:", "https:"].includes(url.protocol)) return null
    return `${url.host.toLowerCase()}${url.pathname.replace(/\/+$/, "")}`
  } catch {
    return null
  }
}

function statisticsIdentities(channel: StatisticsChannel) {
  const identities = new Set<string>()
  for (const account of [channel, ...(channel.accounts ?? [])]) {
    if (account.credential_mode === "password") {
      const username = account.username.trim().toLowerCase()
      if (username) identities.add(`login:${username}`)
    } else if (channel.type === "newapi" && account.newapi_user_id?.trim()) {
      identities.add(`newapi-user:${account.newapi_user_id.trim()}`)
    }
  }
  return identities
}

// 仅用可核对的登录身份提醒；Token 展示名称和相同域名都不足以证明数据重复。
export function duplicateStatisticsChannels<T extends StatisticsChannel>(candidate: StatisticsChannel, channels: readonly T[]): T[] {
  if (candidate.exclude_from_statistics || candidate.balance_mode === "manual") return []
  const site = statisticsSite(candidate.site_url)
  if (!site) return []
  const identities = statisticsIdentities(candidate)
  if (!identities.size) return []
  return channels.filter(channel => channel.id !== candidate.id && !channel.exclude_from_statistics
    && channel.balance_mode !== "manual" && channel.type === candidate.type && statisticsSite(channel.site_url) === site
    && [...statisticsIdentities(channel)].some(identity => identities.has(identity)))
}
