"use client"

import { CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip as RechartsTooltip, XAxis, YAxis } from "recharts"
import type { ConsumptionBucket, LatencyBucketGranularity, RelayUsageRange } from "@/lib/api-types"
import type { RelayCompareLink } from "@/lib/channel-relay-association"
import { RelayConsumptionComparisonTooltip } from "@/components/monitor/channel-relay-compare"
import { money } from "@/lib/format"
import { latencyBucketGranularityLabel } from "@/lib/latency"

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

function formatAxisTime(value: string) {
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

function niceCeil(value: number) {
  if (!Number.isFinite(value) || value <= 0) return 1
  const padded = value * 1.12
  const magnitude = 10 ** Math.floor(Math.log10(padded))
  const normalized = padded / magnitude
  const step = normalized <= 1 ? 1 : normalized <= 2 ? 2 : normalized <= 5 ? 5 : 10
  return step * magnitude
}

type TooltipPayload = {
  value?: number
  payload?: {
    start: string
    end: string
    amount: number
  }
}

interface ConsumptionTooltipProps {
  active?: boolean
  payload?: TooltipPayload[]
  relayLinks: RelayCompareLink[]
  range: RelayUsageRange
  granularity: LatencyBucketGranularity
  bucketCount: number
}

function ConsumptionTooltip({ active, payload, relayLinks, range, granularity, bucketCount }: ConsumptionTooltipProps) {
  const point = payload?.[0]?.payload
  if (!active || !point) return null
  if (relayLinks.length > 0) {
    return (
      <RelayConsumptionComparisonTooltip
        bucket={point}
        links={relayLinks}
        range={range}
        granularity={granularity}
        bucketCount={bucketCount}
      />
    )
  }
  return (
    <div className="w-52 rounded-lg border border-border bg-popover px-3 py-2 text-xs text-popover-foreground shadow-2xl ring-1 ring-black/5">
      <p className="font-medium text-foreground">{formatBucketTime(point.start)} - {formatBucketTime(point.end)}</p>
      <div className="mt-2 flex items-center justify-between gap-4">
        <span className="text-muted-foreground">调用消耗</span>
        <span className="font-mono font-semibold tabular-nums text-success">{money(point.amount, { precise: point.amount < 1 })}</span>
      </div>
    </div>
  )
}

export function ChannelConsumptionTrend({
  buckets,
  loading,
  granularity = "auto",
  compact = false,
  emptyLabel = "暂无消耗数据",
  relayLinks = [],
  range = "today",
  bucketCount = 60,
}: {
  buckets: ConsumptionBucket[]
  loading?: boolean
  granularity?: LatencyBucketGranularity
  compact?: boolean
  emptyLabel?: string
  relayLinks?: RelayCompareLink[]
  range?: RelayUsageRange
  bucketCount?: number
}) {
  const data = buckets.map((bucket) => ({
    start: bucket.start,
    end: bucket.end,
    amount: Math.max(0, bucket.amount),
  }))
  const total = data.reduce((sum, point) => sum + point.amount, 0)
  const hasConsumption = data.some((point) => point.amount > 0)
  const max = data.reduce((current, point) => Math.max(current, point.amount), 0)
  const yMax = niceCeil(max)

  return (
    <div className={compact ? "" : "mt-3 border-t border-border pt-3"}>
      {!compact ? <div className="mb-1.5 flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
        <span className="text-[11px] font-medium text-muted-foreground">调用消耗趋势</span>
        <span className="flex items-center gap-2 text-[10px] text-muted-foreground">
          <span>粒度 {latencyBucketGranularityLabel(granularity)}</span>
          <span className="font-mono tabular-nums">调用归属合计 <strong className="font-semibold text-success">{money(total, { precise: total < 1 })}</strong></span>
        </span>
      </div> : null}
      {loading && buckets.length === 0 ? (
        <div className={`flex items-center justify-center text-[10px] text-muted-foreground ${compact ? "h-12" : "h-28"}`}>消耗趋势读取中…</div>
      ) : buckets.length === 0 || !hasConsumption ? (
        <div className={`flex items-center justify-center rounded border border-dashed border-border text-[10px] text-muted-foreground ${compact ? "h-12" : "h-20"}`}>{emptyLabel}</div>
      ) : (
        <div className={compact ? "h-12 w-full" : "h-28 w-full"}>
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={data} margin={compact ? { top: 3, right: 3, bottom: 3, left: 3 } : { top: 6, right: 4, bottom: 0, left: 0 }}>
              {compact ? null : <CartesianGrid vertical={false} stroke="var(--border)" strokeDasharray="3 3" />}
              {compact ? null : <XAxis
                dataKey="start"
                axisLine={false}
                tickLine={false}
                minTickGap={42}
                tick={{ fill: "var(--muted-foreground)", fontSize: 9 }}
                tickFormatter={formatAxisTime}
              />}
              {compact ? null : <YAxis
                axisLine={false}
                tickLine={false}
                width={42}
                domain={[0, yMax]}
                tick={{ fill: "var(--muted-foreground)", fontSize: 9 }}
                tickFormatter={(value: number) => money(value)}
              />}
              <RechartsTooltip content={<ConsumptionTooltip relayLinks={relayLinks} range={range} granularity={granularity} bucketCount={bucketCount} />} cursor={{ stroke: "var(--border)", strokeDasharray: "4 4" }} offset={12} wrapperStyle={{ zIndex: 100 }} allowEscapeViewBox={{ x: true, y: true }} />
              <Line
                type="monotone"
                dataKey="amount"
                stroke="var(--success)"
                strokeWidth={1.8}
                dot={false}
                activeDot={{ r: 3, fill: "var(--success)", strokeWidth: 0 }}
                isAnimationActive={false}
              />
            </LineChart>
          </ResponsiveContainer>
        </div>
      )}
    </div>
  )
}
