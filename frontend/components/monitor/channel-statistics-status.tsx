import type { Channel } from "@/lib/api-types"
import { duplicateStatisticsChannels } from "@/lib/channel-statistics"

export function ChannelStatisticsStatus({ channel, channels }: { channel: Channel; channels: Channel[] }) {
  if (channel.exclude_from_statistics) return <span className="shrink-0 rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground" title="继续监控并保留明细，不计入渠道汇总">不计入汇总</span>
  const duplicates = duplicateStatisticsChannels(channel, channels)
  if (!duplicates.length) return null
  return <span className="shrink-0 rounded bg-amber-500/10 px-1.5 py-0.5 text-[10px] text-amber-700 dark:text-amber-400" title={`与 ${duplicates.map(item => item.name).join("、")} 使用相同上游账号，汇总可能重复；可在渠道设置中关闭重复渠道的「计入汇总统计」。`}>可能重复统计</span>
}
