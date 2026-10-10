import { useState } from "react"
import { LayoutList, Settings2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { useSyncSettings, useSystemSettings } from "@/lib/queries"
import { ChannelCards } from "@/components/monitor/channel-cards"
import { KpiRow } from "@/components/monitor/kpi-row"
import { MultiplierChanges } from "@/components/monitor/multiplier-changes"
import type { LatencyBucketGranularity, RelayUsageRange } from "@/lib/api-types"
import { cn } from "@/lib/utils"
import { latencyGranularityOptionsForRange, normalizeLatencyGranularity } from "@/lib/latency"
import { ConfigurationLoading } from "@/components/ui/configuration-loading"
import { channelSectionVisible } from "@/lib/system-settings"

const ranges: { value: RelayUsageRange; label: string }[] = [
  { value: "today", label: "今天" },
  { value: "24h", label: "24 小时" },
  { value: "7d", label: "7 天" },
  { value: "30d", label: "30 天" },
  { value: "all", label: "All" },
]

function channelSyncStatus(enabled: boolean, interval: number) {
  if (!enabled) return "自动同步：已关闭"
  if (interval === 1440) return "自动同步：每天"
  if (interval >= 60 && interval % 60 === 0) return "自动同步：" + interval / 60 + " 小时"
  return "自动同步：" + interval + " 分钟"
}

export default function ChannelsPage() {
  const settings = useSyncSettings()
  const systemSettings = useSystemSettings()
  const [range, setRange] = useState<RelayUsageRange>("today")
  const [latencyGranularity, setLatencyGranularity] = useState<LatencyBucketGranularity>("auto")

  function changeRange(nextRange: RelayUsageRange) {
    setRange(nextRange)
    setLatencyGranularity((current) => normalizeLatencyGranularity(nextRange, current))
  }

  const uiSettings = systemSettings.data

  if (!uiSettings) {
    return (
      <ConfigurationLoading query={systemSettings} title={<span className="flex items-center gap-2"><LayoutList className="size-4 text-brand" />{"\u6e20\u9053\u7ba1\u7406"}</span>} />
    )
  }

  if (!uiSettings.channels_page_enabled) {
    return (
      <section className="rounded-lg border border-border bg-card p-6">
        <h1 className="text-lg font-semibold text-foreground">渠道管理已隐藏</h1>
        <p className="mt-2 text-sm text-muted-foreground">可在“系统设置 → 渠道管理”中重新开启渠道管理页面。</p>
        <a href="/settings" className="mt-4 inline-flex h-9 items-center rounded-md bg-primary px-3 text-sm font-medium text-primary-foreground">前往系统设置</a>
      </section>
    )
  }

  return (
    <section className="space-y-4">
      <header className="list-panel-controls flex flex-wrap items-end justify-between gap-3 border-l-2 border-brand pl-3">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-bold text-foreground"><LayoutList className="size-5 text-brand" />{"渠道管理"}</h1>
          <p className="mt-1 text-xs text-muted-foreground">{"管理渠道可用性、账号、余额、消耗与同步任务。"}</p>
        </div>
        <div className="flex flex-wrap items-center justify-end gap-2">
          <div className="inline-flex rounded-md border border-border bg-muted/30 p-0.5" role="group" aria-label="渠道统计时间范围">
            {ranges.map((item) => (
            <button key={item.value} type="button" onClick={() => changeRange(item.value)} className={cn("h-11 rounded px-3 text-xs transition-colors sm:h-8", range === item.value ? "bg-background font-medium text-foreground shadow-xs" : "text-muted-foreground hover:text-foreground")}>
                {item.label}
              </button>
            ))}
          </div>
          <Select value={latencyGranularity} onValueChange={(value) => setLatencyGranularity(value as LatencyBucketGranularity)}>
            <SelectTrigger className="h-11 w-36 text-xs sm:h-9" aria-label="时间稳定性与调用消耗趋势聚合粒度" title="作用于时间稳定性与调用消耗趋势"><SelectValue /></SelectTrigger>
            <SelectContent>{latencyGranularityOptionsForRange(range).map((item) => <SelectItem key={item.value} value={item.value}>{`粒度：${item.label}`}</SelectItem>)}</SelectContent>
          </Select>
          <Button asChild variant="outline" size="sm" className="h-11 gap-1.5 sm:h-9" disabled={!settings.data}>
            <a href={settings.data ? "/settings#background-tasks" : undefined} title="前往系统设置修改全局渠道自动同步">
              <Settings2 className="size-3.5" />
              {settings.data ? channelSyncStatus(settings.data.channel_enabled, settings.data.channel_interval_minutes || 30) : "读取自动同步设置…"}
            </a>
          </Button>
        </div>
      </header>

      <KpiRow range={range} source="channels" />

      {channelSectionVisible(uiSettings, "rate_changes") ? <MultiplierChanges range={range} source="channels" /> : null}

      {uiSettings.channels_show_channel_info ? <ChannelCards usageRange={range} latencyGranularity={latencyGranularity} uiSettings={uiSettings} /> : null}
    </section>
  )
}
