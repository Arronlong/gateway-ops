"use client"

import { useRef, useState, type ComponentProps } from "react"
import { Line, LineChart, ResponsiveContainer, Tooltip as RechartsTooltip } from "recharts"
import type { RelayAccountConsumptionBucket } from "@/lib/api-types"
import type { ChannelCompareLink } from "@/lib/channel-relay-association"
import { ChannelConsumptionComparisonTooltip } from "@/components/monitor/channel-relay-compare"
import { money } from "@/lib/format"
import { ViewportChart } from "@/components/monitor/viewport-chart"

const TOOLTIP_HEIGHT = 96
const TOOLTIP_GAP = 8
const CHART_HEIGHT = 56
const COMPACT_CHART_HEIGHT = 48

type TooltipPlacement = "top" | "bottom"

function formatBucketTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  })
}

type TooltipPayload = { payload?: RelayAccountConsumptionBucket }

function ConsumptionTooltip({ active, payload, channelLinks, channelConsumptionBucketsByChannel, relayAccountName }: {
  active?: boolean
  payload?: TooltipPayload[]
  channelLinks: ChannelCompareLink[]
  channelConsumptionBucketsByChannel: Map<number, { start: string; end: string; amount: number }[]>
  relayAccountName: string
}) {
  const point = payload?.[0]?.payload
  if (!active || !point) return null
  if (channelLinks.length > 0) {
    return (
      <ChannelConsumptionComparisonTooltip
        bucket={point}
        relayAccountName={relayAccountName}
        links={channelLinks}
        channelBucketsByChannel={channelConsumptionBucketsByChannel}
      />
    )
  }
  return (
    <div className="min-w-52 rounded-lg border border-border bg-popover px-3 py-2 text-xs shadow-lg">
      <p className="whitespace-nowrap font-medium text-foreground">{formatBucketTime(point.start)} - {formatBucketTime(point.end)}</p>
      <div className="mt-2 grid grid-cols-[1fr_auto] gap-x-4 gap-y-1.5">
        <span className="text-muted-foreground">用户消费</span>
        <span className="font-mono font-semibold tabular-nums text-success">{money(point.user_charge, { precise: point.user_charge < 1 })}</span>
        <span className="text-muted-foreground">账号成本</span>
        <span className="font-mono font-semibold tabular-nums text-warning">{money(point.account_cost, { precise: point.account_cost < 1 })}</span>
        <span className="text-muted-foreground">请求数</span>
        <span className="font-mono font-semibold tabular-nums text-foreground">{point.request_count.toLocaleString("zh-CN")}</span>
      </div>
    </div>
  )
}

export function RelayAccountConsumptionTrend(props: ComponentProps<typeof RelayAccountConsumptionTrendContent>) {
  return <ViewportChart placeholder={<div className={props.compact ? "h-12" : "h-14"} aria-label="滚动到此处加载消费图表" />}>
    <RelayAccountConsumptionTrendContent {...props} />
  </ViewportChart>
}

function RelayAccountConsumptionTrendContent({ buckets, loading, compact = false, channelLinks = [], channelConsumptionBucketsByChannel = new Map(), relayAccountName = "当前中转站账号" }: {
  buckets: RelayAccountConsumptionBucket[]
  loading?: boolean
  compact?: boolean
  channelLinks?: ChannelCompareLink[]
  channelConsumptionBucketsByChannel?: Map<number, { start: string; end: string; amount: number }[]>
  relayAccountName?: string
}) {
  const chartRef = useRef<HTMLDivElement>(null)
  const [tooltipPlacement, setTooltipPlacement] = useState<TooltipPlacement>("top")
  const chartHeight = compact ? COMPACT_CHART_HEIGHT : CHART_HEIGHT

  function updateTooltipPlacement() {
    const chart = chartRef.current
    if (!chart) return

    const chartRect = chart.getBoundingClientRect()
    const scrollViewport = chart.closest("[data-relay-account-scroll]") as HTMLElement | null
    const viewportRect = scrollViewport?.getBoundingClientRect()
    const viewportTop = viewportRect?.top ?? 0
    const viewportBottom = viewportRect?.bottom ?? window.innerHeight
    const requiredSpace = TOOLTIP_HEIGHT + TOOLTIP_GAP
    const spaceAbove = chartRect.top - viewportTop
    const spaceBelow = viewportBottom - chartRect.bottom

    setTooltipPlacement(spaceAbove >= requiredSpace || spaceAbove >= spaceBelow ? "top" : "bottom")
  }

  if (loading && buckets.length === 0) {
    return <div className="flex h-12 items-center text-[10px] text-muted-foreground">消费趋势读取中…</div>
  }
  if (buckets.length === 0) {
    return <div className="flex h-12 items-center text-[10px] text-muted-foreground">暂无消费数据</div>
  }

  return (
    <div ref={chartRef} className={`${compact ? "h-12" : "h-14"} w-full min-w-0`} onPointerEnter={updateTooltipPlacement} onFocusCapture={updateTooltipPlacement}>
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={buckets} margin={{ top: 5, right: 2, bottom: 5, left: 2 }}>
          <RechartsTooltip
            content={<ConsumptionTooltip channelLinks={channelLinks} channelConsumptionBucketsByChannel={channelConsumptionBucketsByChannel} relayAccountName={relayAccountName} />}
            cursor={{ stroke: "var(--border)", strokeDasharray: "3 3" }}
            position={{ y: tooltipPlacement === "top" ? -(TOOLTIP_HEIGHT + TOOLTIP_GAP) : chartHeight + TOOLTIP_GAP }}
            isAnimationActive={false}
            wrapperStyle={{ zIndex: 60 }}
          />
          <Line
            type="monotone"
            dataKey="user_charge"
            stroke="var(--success)"
            strokeWidth={1.8}
            dot={false}
            activeDot={{ r: 2.5, fill: "var(--success)", strokeWidth: 0 }}
            isAnimationActive={false}
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  )
}
