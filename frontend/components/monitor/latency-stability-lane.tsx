"use client"

import { useEffect, useRef, useState } from "react"
import type { ReactNode } from "react"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import type { ChannelUsageSample, LatencyBucket, LatencyBucketGranularity, PublicGroupMonitorCall, RelayLatencySample, RelayUsageRange } from "@/lib/api-types"
import { relayAccountComparisonKey, type ChannelCompareLink, type RelayCompareLink } from "@/lib/channel-relay-association"
import { ChannelLatencyComparisonTooltip, RelayLatencyComparisonTooltip } from "@/components/monitor/channel-relay-compare"
import { formatLatency, formatOutputTokenSpeed, latencyBucketGranularityLabel, latencyColor, latencyTone, latencyTooltipTextClass, outputTokenSpeed } from "@/lib/latency"
import { matchRelatedRecentCall } from "@/lib/recent-call-comparison"
import { cn } from "@/lib/utils"

function formatBucketTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return "-"
  return date.toLocaleString("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false })
}

function formatSampleTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return "-"
  return date.toLocaleString("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false })
}

type LatencyDetailSample = RelayLatencySample | ChannelUsageSample | PublicGroupMonitorCall

function totalTokens(sample: LatencyDetailSample) {
  const inputTokens = "input_tokens" in sample ? (sample.input_tokens ?? 0) : 0
  const outputTokens = "output_tokens" in sample ? (sample.output_tokens ?? 0) : 0
  const cacheReadTokens = "cache_read_tokens" in sample ? (sample.cache_read_tokens ?? 0) : 0
  const cacheCreationTokens = "cache_creation_tokens" in sample ? (sample.cache_creation_tokens ?? 0) : 0
  const extraCreation = "cache_creation_5m_tokens" in sample
    ? (sample.cache_creation_5m_tokens ?? 0) + (sample.cache_creation_1h_tokens ?? 0)
    : 0
  return inputTokens + outputTokens + cacheReadTokens + cacheCreationTokens + extraCreation
}

function BucketTooltip({ bucket }: { bucket: LatencyBucket }) {
  const firstP50 = bucket.first_token_p50_ms == null ? null : latencyTone(bucket.first_token_p50_ms, "first")
  const firstP95 = bucket.first_token_p95_ms == null ? null : latencyTone(bucket.first_token_p95_ms, "first")
  const durationP50 = bucket.duration_p50_ms == null ? null : latencyTone(bucket.duration_p50_ms, "duration")
  const durationP95 = bucket.duration_p95_ms == null ? null : latencyTone(bucket.duration_p95_ms, "duration")
  return (
    <div className="w-64 space-y-2 p-1 text-xs">
      <div>
        <p className="font-semibold text-foreground">{formatBucketTime(bucket.start)} - {formatBucketTime(bucket.end)}</p>
        <p className="mt-0.5 text-[10px] text-muted-foreground">点击查看该时间桶的调用明细</p>
      </div>
      <div className="grid grid-cols-[72px_1fr_1fr] gap-x-2 gap-y-1 rounded-md bg-muted/60 p-2 font-mono tabular-nums">
        <span />
        <span className="font-sans text-[10px] text-muted-foreground">P50</span>
        <span className="font-sans text-[10px] text-muted-foreground">P95</span>
        <span className="font-sans text-muted-foreground">首字</span>
        <span className={firstP50 ? latencyTooltipTextClass[firstP50] : "text-muted-foreground"}>{bucket.first_token_p50_ms == null ? "未提供" : formatLatency(bucket.first_token_p50_ms)}</span>
        <span className={firstP95 ? latencyTooltipTextClass[firstP95] : "text-muted-foreground"}>{bucket.first_token_p95_ms == null ? "未提供" : formatLatency(bucket.first_token_p95_ms)}</span>
        <span className="font-sans text-muted-foreground">总耗时</span>
        <span className={durationP50 ? latencyTooltipTextClass[durationP50] : "text-muted-foreground"}>{bucket.duration_p50_ms == null ? "未提供" : formatLatency(bucket.duration_p50_ms)}</span>
        <span className={durationP95 ? latencyTooltipTextClass[durationP95] : "text-muted-foreground"}>{bucket.duration_p95_ms == null ? "未提供" : formatLatency(bucket.duration_p95_ms)}</span>
        <span className="font-sans text-muted-foreground">输出速度</span>
        <span>{formatOutputTokenSpeed(bucket.output_token_speed_p50)}</span>
        <span>{formatOutputTokenSpeed(bucket.output_token_speed_p95)}</span>
      </div>
      <div className="grid grid-cols-2 gap-2">
        <div className="rounded-md border border-border px-2 py-1.5">
          <p className="text-[10px] text-muted-foreground">成功率</p>
          <p className="mt-0.5 font-mono font-semibold tabular-nums">{bucket.success_rate == null ? "渠道未提供" : `${(bucket.success_rate * 100).toFixed(bucket.success_rate === 1 ? 0 : 1)}%`}</p>
        </div>
        <div className="rounded-md border border-border px-2 py-1.5">
          <p className="text-[10px] text-muted-foreground">请求数</p>
          <p className="mt-0.5 font-mono font-semibold tabular-nums">{bucket.request_count}</p>
        </div>
      </div>
    </div>
  )
}

function LatencyHoverContent({ bucket, relay, channel }: { bucket: LatencyBucket; relay: RelayCompareConfig; channel: ChannelCompareConfig }) {
  if (relay.links.length > 0) {
    return (
      <RelayLatencyComparisonTooltip
        bucket={bucket}
        links={relay.links}
        range={relay.range}
        granularity={relay.granularity}
        bucketCount={relay.bucketCount}
      />
    )
  }
  if (channel.links.length > 0) {
    return (
      <ChannelLatencyComparisonTooltip
        bucket={bucket}
        relayAccountName={channel.relayAccountName}
        links={channel.links}
        channelBucketsByChannel={channel.bucketsByChannel}
      />
    )
  }
  return <BucketTooltip bucket={bucket} />
}

function RecentCallValue({ sample, metric }: { sample: LatencyDetailSample | null; metric: "first" | "duration" | "speed" }) {
  if (!sample) return <span className="text-muted-foreground">-</span>
  if (metric === "speed") return <span>{formatOutputTokenSpeed(outputTokenSpeed("output_tokens" in sample ? sample.output_tokens : null, sample.duration_ms))}</span>
  const value = metric === "first" ? sample.first_token_ms : sample.duration_ms
  const tone = value == null ? null : latencyTone(value, metric)
  return <span className={tone ? latencyTooltipTextClass[tone] : "text-muted-foreground"}>{formatLatency(value)}</span>
}

function RecentValueOrder({ primaryLabel, counterpartLabel }: { primaryLabel: string; counterpartLabel: string }) {
  return (
    <span className="flex shrink-0 items-center gap-1 rounded-md border border-border bg-card px-2 py-1 text-[10px] font-medium shadow-sm">
      <span className="text-muted-foreground">{"\u6570\u503c\u987a\u5e8f"}</span>
      <span className="rounded bg-blue-500/15 px-1.5 py-0.5 text-blue-700 dark:text-blue-300">{primaryLabel}</span>
      <span className="text-muted-foreground">/</span>
      <span className="rounded bg-slate-900 px-1.5 py-0.5 text-white dark:bg-slate-100 dark:text-slate-900">{counterpartLabel}</span>
    </span>
  )
}

function RecentCallPair({ title, subtitle, primary, counterpart }: {
  title: string
  subtitle: string
  primary: LatencyDetailSample
  counterpart: LatencyDetailSample | null
}) {
  const rows: { label: string; metric: "first" | "duration" | "speed" }[] = [
    { label: "\u9996\u5b57", metric: "first" },
    { label: "\u603b\u8017\u65f6", metric: "duration" },
    { label: "Token \u901f\u5ea6", metric: "speed" },
  ]
  return (
    <section className="mt-2 border-t border-border pt-2 first:mt-0 first:border-t-0 first:pt-0">
      <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0"><p className="truncate text-[11px] font-semibold text-foreground" title={title}>{title}</p><p className="truncate text-[10px] text-muted-foreground" title={subtitle}>{subtitle}</p></div>
        {!counterpart ? <span className="shrink-0 text-[9px] text-muted-foreground">{"\u672a\u5339\u914d\u5230\u5173\u8054\u8c03\u7528"}</span> : null}
      </div>
      <div className="mt-2 grid grid-cols-3 gap-px overflow-hidden rounded-md border border-border bg-border font-mono text-xs tabular-nums">
        {rows.map((row) => <div key={row.metric} className="min-w-0 bg-muted/45 px-2 py-2"><span className="block font-sans text-[10px] text-muted-foreground">{row.label}</span><span className="mt-1 grid grid-rows-2 gap-0.5 font-semibold leading-4"><RecentCallValue sample={primary} metric={row.metric} /><RecentCallValue sample={counterpart} metric={row.metric} /></span></div>)}
      </div>
    </section>
  )
}

function RecentSampleTooltip({ sample, primarySamples, relay, channel }: { sample: LatencyDetailSample; primarySamples: LatencyDetailSample[]; relay: RelayCompareConfig; channel: ChannelCompareConfig }) {
  const channelAccountID = "channel_account_id" in sample ? sample.channel_account_id : null
  const relayPairs = relay.links.flatMap((link) => link.accounts
    .filter((account) => channelAccountID == null || account.channelAccountID === channelAccountID)
    .map((account) => {
      const counterpartSamples = relay.recentSamplesByAccount.get(relayAccountComparisonKey(link.stationID, account.externalID)) ?? []
      const scopedPrimary = channelAccountID == null ? primarySamples : primarySamples.filter((item) => "channel_account_id" in item && item.channel_account_id === channelAccountID)
      return { key: `${link.stationID}:${account.externalID}`, title: link.stationName, subtitle: account.name, counterpart: matchRelatedRecentCall(sample, scopedPrimary, counterpartSamples) }
    }))
  const channelPairs = channel.links.map((link) => {
    const counterpartSamples = (channel.recentSamplesByChannel.get(link.channelID) ?? []).filter((item) => link.accountIDs.includes(item.channel_account_id))
    return { key: String(link.channelID), title: link.channelName, subtitle: link.accountNames.join("\u3001"), counterpart: matchRelatedRecentCall(sample, primarySamples, counterpartSamples) }
  })
  const valueOrder = relayPairs.length > 0
    ? { primaryLabel: "\u6e20\u9053\u6570\u636e", counterpartLabel: "\u4e2d\u8f6c\u7ad9\u6570\u636e" }
    : channelPairs.length > 0
      ? { primaryLabel: "\u4e2d\u8f6c\u7ad9\u6570\u636e", counterpartLabel: "\u6e20\u9053\u6570\u636e" }
      : null
  const hasComparison = relayPairs.length > 0 || channelPairs.length > 0
  return (
    <div className={cn("space-y-2 p-1 text-xs", hasComparison ? "w-[min(20rem,calc(100vw-32px))]" : "w-fit max-w-[calc(100vw-32px)]")}>
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="font-semibold text-foreground">{formatSampleTime(sample.created_at)}</p>
          <p className="mt-0.5 truncate text-[10px] text-muted-foreground">{hasComparison ? "\u5f53\u524d\u8c03\u7528\u4e0e\u5173\u8054\u5bf9\u8c61\u7684\u540c\u6279\u6b21\u8c03\u7528\u5bf9\u6bd4" : "\u5f53\u524d\u8c03\u7528\u6838\u5fc3\u6307\u6807"}</p>
        </div>
        {valueOrder ? <RecentValueOrder primaryLabel={valueOrder.primaryLabel} counterpartLabel={valueOrder.counterpartLabel} /> : null}
      </div>
      {relayPairs.map((pair) => <RecentCallPair key={pair.key} title={pair.title} subtitle={pair.subtitle} primary={sample} counterpart={pair.counterpart} />)}
      {channelPairs.map((pair) => <RecentCallPair key={pair.key} title={pair.title} subtitle={pair.subtitle} primary={sample} counterpart={pair.counterpart} />)}
      {!hasComparison ? <div className="grid grid-cols-3 gap-px overflow-hidden rounded-md border border-border bg-border font-mono text-xs tabular-nums"><div className="min-w-0 bg-muted/45 px-2 py-2"><span className="block font-sans text-[10px] text-muted-foreground">{"\u9996\u5b57"}</span><span className="mt-1 block whitespace-nowrap font-semibold leading-4"><RecentCallValue sample={sample} metric="first" /></span></div><div className="min-w-0 bg-muted/45 px-2 py-2"><span className="block font-sans text-[10px] text-muted-foreground">{"\u603b\u8017\u65f6"}</span><span className="mt-1 block whitespace-nowrap font-semibold leading-4"><RecentCallValue sample={sample} metric="duration" /></span></div><div className="min-w-0 bg-muted/45 px-2 py-2"><span className="block font-sans text-[10px] text-muted-foreground">{"Token \u901f\u5ea6"}</span><span className="mt-1 block whitespace-nowrap font-semibold leading-4"><RecentCallValue sample={sample} metric="speed" /></span></div></div> : null}
    </div>
  )
}

function bucketForSample(buckets: LatencyBucket[], sample: LatencyDetailSample): LatencyBucket | null {
  const timestamp = Date.parse(sample.created_at)
  if (!Number.isFinite(timestamp)) return null
  return buckets.find((bucket) => {
    const start = Date.parse(bucket.start)
    const end = Date.parse(bucket.end)
    return Number.isFinite(start) && Number.isFinite(end) && start <= timestamp && timestamp < end
  }) ?? null
}

function recentSampleBucket(sample: LatencyDetailSample): LatencyBucket {
  const timestamp = Date.parse(sample.created_at)
  const start = Number.isFinite(timestamp) ? new Date(timestamp).toISOString() : sample.created_at
  const end = Number.isFinite(timestamp) ? new Date(timestamp + 1).toISOString() : sample.created_at
  const outputTokens = "output_tokens" in sample ? sample.output_tokens : null
  let successRate: number | null = null
  if ("success_known" in sample && sample.success_known) {
    successRate = sample.success ? 1 : 0
  } else if ("success" in sample) {
    successRate = sample.success ? 1 : 0
  } else if ("status_code" in sample && sample.status_code != null) {
    successRate = sample.status_code === 0 || (sample.status_code >= 200 && sample.status_code < 400) ? 1 : 0
  }
  const speed = outputTokenSpeed(outputTokens, sample.duration_ms)
  return {
    start,
    end,
    first_token_p50_ms: sample.first_token_ms ?? null,
    first_token_p95_ms: sample.first_token_ms ?? null,
    duration_p50_ms: sample.duration_ms ?? null,
    duration_p95_ms: sample.duration_ms ?? null,
    output_token_speed_p50: speed,
    output_token_speed_p95: speed,
    success_rate: successRate,
    request_count: 1,
  }
}

function RecentSampleCell({ sample, bucket, onOpen, compact, primarySamples, relay, channel }: {
  sample: LatencyDetailSample
  bucket: LatencyBucket | null
  onOpen: (sample: LatencyDetailSample, bucket: LatencyBucket | null) => void
  compact: boolean
  primarySamples: LatencyDetailSample[]
  relay: RelayCompareConfig
  channel: ChannelCompareConfig
}) {
  const firstTone = sample.first_token_ms == null ? null : latencyTone(sample.first_token_ms, "first")
  const durationTone = sample.duration_ms == null ? null : latencyTone(sample.duration_ms, "duration")
  const cells = [firstTone, firstTone, durationTone, durationTone]
  return (
    <Tooltip delayDuration={120}>
      <TooltipTrigger asChild>
        <button
          type="button"
          className={cn("block min-w-0 flex-1 cursor-pointer overflow-hidden rounded-[2px] p-0 opacity-90 transition-opacity hover:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring", compact ? "h-8" : "h-10")}
          aria-label={formatSampleTime(sample.created_at) + ", 首字 " + formatLatency(sample.first_token_ms) + ", 总耗时 " + formatLatency(sample.duration_ms)}
          onClick={() => onOpen(sample, bucket)}
        >
          <span className="grid h-full w-full grid-cols-2 grid-rows-2 overflow-hidden rounded-[2px] bg-border" aria-hidden="true">
            {cells.map((tone, index) => <span key={index} style={{ backgroundColor: tone ? latencyColor[tone] : "var(--muted)" }} />)}
          </span>
        </button>
      </TooltipTrigger>
      <TooltipContent side="top" sideOffset={8} className="border border-border bg-popover p-2 text-popover-foreground shadow-2xl ring-1 ring-black/5">
        <RecentSampleTooltip sample={sample} primarySamples={primarySamples} relay={relay} channel={channel} />
      </TooltipContent>
    </Tooltip>
  )
}

interface RelayCompareConfig {
  links: RelayCompareLink[]
  range: RelayUsageRange
  granularity: LatencyBucketGranularity
  bucketCount: number
  recentSamplesByAccount: Map<string, RelayLatencySample[]>
}

interface ChannelCompareConfig {
  links: ChannelCompareLink[]
  bucketsByChannel: Map<number, LatencyBucket[]>
  relayAccountName: string
  recentSamplesByChannel: Map<number, ChannelUsageSample[]>
}

function BucketCell({ bucket, onOpen, relay, channel }: { bucket: LatencyBucket; onOpen: (bucket: LatencyBucket) => void; relay: RelayCompareConfig; channel: ChannelCompareConfig }) {
  if (bucket.request_count === 0) {
    return <span className="block h-8 w-full rounded-sm bg-muted/55 ring-1 ring-inset ring-border" title={formatBucketTime(bucket.start) + " - " + formatBucketTime(bucket.end) + " · 无调用"} />
  }
  const cells = [
    bucket.first_token_p50_ms == null ? null : latencyTone(bucket.first_token_p50_ms, "first"),
    bucket.first_token_p95_ms == null ? null : latencyTone(bucket.first_token_p95_ms, "first"),
    bucket.duration_p50_ms == null ? null : latencyTone(bucket.duration_p50_ms, "duration"),
    bucket.duration_p95_ms == null ? null : latencyTone(bucket.duration_p95_ms, "duration"),
  ]
  return (
    <Tooltip delayDuration={120}>
      <TooltipTrigger asChild>
        <button
          type="button"
          className="grid h-8 w-full grid-cols-2 grid-rows-2 overflow-hidden rounded-sm bg-border transition-transform hover:brightness-110 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          aria-label={formatBucketTime(bucket.start) + " 至 " + formatBucketTime(bucket.end) + "，" + bucket.request_count + " 次请求"}
          onClick={() => onOpen(bucket)}
        >
          {cells.map((tone, index) => <span key={index} style={{ backgroundColor: tone ? latencyColor[tone] : "var(--muted)" }} />)}
        </button>
      </TooltipTrigger>
      <TooltipContent side="top" sideOffset={8} className="border border-border bg-popover p-2 text-popover-foreground shadow-2xl ring-1 ring-black/5">
        <LatencyHoverContent bucket={bucket} relay={relay} channel={channel} />
      </TooltipContent>
    </Tooltip>
  )
}

function DetailPercentileValue({ p50, p95, format }: {
  p50: number | null
  p95: number | null
  format: (value: number | null | undefined) => string
}) {
  return (
    <div className="mt-1 grid grid-rows-2 grid-cols-[auto_auto] items-baseline justify-start gap-x-2 gap-y-0.5 font-mono text-xs font-semibold tabular-nums">
      <span className="font-sans text-[9px] font-normal text-muted-foreground">P50</span>
      <span className="whitespace-nowrap">{format(p50)}</span>
      <span className="font-sans text-[9px] font-normal text-muted-foreground">P95</span>
      <span className="whitespace-nowrap">{format(p95)}</span>
    </div>
  )
}

function InlineLatencyDetails({ selected, samples, detailLoading, detailError, onClose }: {
  selected: LatencyBucket
  samples: LatencyDetailSample[]
  detailLoading: boolean
  detailError: string
  onClose: () => void
}) {
  return (
    <div className="mt-3 overflow-hidden rounded-lg border border-border bg-card">
      <div className="flex items-center justify-between gap-3 border-b border-border px-4 py-3">
        <div>
          <p className="text-sm font-semibold text-foreground">调用明细</p>
          <p className="mt-0.5 text-[11px] text-muted-foreground">{formatBucketTime(selected.start)} - {formatBucketTime(selected.end)} · 调用 {selected.request_count} 次请求</p>
        </div>
        <button type="button" className="text-xs font-medium text-muted-foreground hover:text-foreground" onClick={onClose}>收起</button>
      </div>
      <div className="grid grid-cols-2 gap-px border-b border-border bg-border sm:grid-cols-4">
        <div className="bg-card px-4 py-3"><p className="text-[10px] text-muted-foreground">成功率</p><p className="mt-1 font-mono text-sm font-semibold tabular-nums">{selected.success_rate == null ? "渠道未提供" : `${(selected.success_rate * 100).toFixed(selected.success_rate === 1 ? 0 : 1)}%`}</p></div>
        <div className="bg-card px-4 py-3"><p className="text-[10px] text-muted-foreground">首字 P50 / P95</p><DetailPercentileValue p50={selected.first_token_p50_ms} p95={selected.first_token_p95_ms} format={formatLatency} /></div>
        <div className="bg-card px-4 py-3"><p className="text-[10px] text-muted-foreground">总耗时 P50 / P95</p><DetailPercentileValue p50={selected.duration_p50_ms} p95={selected.duration_p95_ms} format={formatLatency} /></div>
        <div className="bg-card px-4 py-3"><p className="text-[10px] text-muted-foreground">输出速度 P50 / P95</p><DetailPercentileValue p50={selected.output_token_speed_p50} p95={selected.output_token_speed_p95} format={formatOutputTokenSpeed} /></div>
      </div>
      <div className="max-h-[58vh] overflow-y-auto bg-muted/30 p-3">
        {detailLoading ? <div className="py-10 text-center text-sm text-muted-foreground">正在读取调用明细…</div> : detailError ? <div className="py-10 text-center text-sm text-danger">{detailError}</div> : samples.length === 0 ? <div className="py-10 text-center text-sm text-muted-foreground">该时间桶暂无可展示的调用明细</div> : (
          <div className="space-y-2">
            {samples.map((sample, index) => {
              const first = sample.first_token_ms == null ? null : latencyTone(sample.first_token_ms, "first")
              const duration = sample.duration_ms == null ? null : latencyTone(sample.duration_ms, "duration")
              const statusCode = "status_code" in sample ? sample.status_code : undefined
              const successful = "success" in sample
                ? sample.success
                : statusCode == null || statusCode === 0 || (statusCode >= 200 && statusCode < 400)
              const successKnown = "success_known" in sample
                ? sample.success_known
                : "success" in sample || statusCode != null
              return (
                <div key={sample.created_at + "-" + index} className="rounded-lg border border-border bg-card p-3">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <time className="font-mono text-xs font-semibold tabular-nums">{formatSampleTime(sample.created_at)}</time>
                    <div className="flex items-center gap-2">
                      {statusCode ? <span className={cn("rounded px-1.5 py-0.5 font-mono text-[10px] font-semibold", successful ? "bg-emerald-500/10 text-emerald-700" : "bg-red-500/10 text-red-700")}>HTTP {statusCode}</span> : successKnown ? <span className={cn("rounded px-1.5 py-0.5 text-[10px] font-semibold", successful ? "bg-emerald-500/10 text-emerald-700" : "bg-red-500/10 text-red-700")}>{successful ? "成功" : "失败"}</span> : null}
                      <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">#{index + 1}</span>
                    </div>
                  </div>
                  <div className="mt-2 flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
                    <span className="min-w-0 max-w-[240px] truncate font-mono text-xs font-semibold" title={sample.model || "-"}>{sample.model || "-"}</span>
                  </div>
                  <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 rounded-md bg-muted/50 px-2.5 py-2 font-mono text-xs font-semibold tabular-nums">
                    <span className={first ? latencyTooltipTextClass[first] : "text-muted-foreground"}>首字 {sample.first_token_ms == null ? "渠道未提供" : formatLatency(sample.first_token_ms)}</span>
                    <span className={duration ? latencyTooltipTextClass[duration] : "text-muted-foreground"}>总耗时 {sample.duration_ms == null ? "渠道未提供" : formatLatency(sample.duration_ms)}</span>
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>
    </div>
  )
}

export function LatencyStabilityLane({
  buckets,
  loading,
  loadSamples,
  layout = "fixed",
  compact = false,
  granularity = "auto",
  emptyLabel = "暂无区间数据",
  relayLinks = [],
  channelLinks = [],
  channelLatencyBucketsByChannel = new Map(),
  relayRecentSamplesByAccount = new Map(),
  channelRecentSamplesByChannel = new Map(),
  relayAccountName = "当前中转站账号",
  recentSamples,
  range = "today",
  bucketCount = 60,
  title,
  headerRight,
  detailMode = "dialog",
}: {
  buckets: LatencyBucket[]
  loading?: boolean
  loadSamples: (bucket: LatencyBucket) => Promise<LatencyDetailSample[]>
  layout?: "fill" | "fixed"
  compact?: boolean
  granularity?: LatencyBucketGranularity
  emptyLabel?: string
  relayLinks?: RelayCompareLink[]
  channelLinks?: ChannelCompareLink[]
  channelLatencyBucketsByChannel?: Map<number, LatencyBucket[]>
  relayRecentSamplesByAccount?: Map<string, RelayLatencySample[]>
  channelRecentSamplesByChannel?: Map<number, ChannelUsageSample[]>
  relayAccountName?: string
  /** 传入时保留原有的最近调用样本条带，仅将交互统计指向所属时间桶。 */
  recentSamples?: LatencyDetailSample[]
  range?: RelayUsageRange
  bucketCount?: number
  /** 展示的标题，默认“时间稳定性”。 */
  title?: ReactNode
  headerRight?: ReactNode
  detailMode?: "dialog" | "inline"
}) {
  const [selected, setSelected] = useState<LatencyBucket | null>(null)
  const [samples, setSamples] = useState<LatencyDetailSample[]>([])
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailError, setDetailError] = useState("")
  const laneRef = useRef<HTMLDivElement | null>(null)
  const relayConfig: RelayCompareConfig = { links: relayLinks, range, granularity, bucketCount, recentSamplesByAccount: relayRecentSamplesByAccount }
  const channelConfig: ChannelCompareConfig = { links: channelLinks, bucketsByChannel: channelLatencyBucketsByChannel, relayAccountName, recentSamplesByChannel: channelRecentSamplesByChannel }
  const isRecentMode = recentSamples != null
  const orderedRecentSamples = isRecentMode ? [...recentSamples].reverse() : []

  useEffect(() => {
    const lane = laneRef.current
    if (!lane || buckets.length === 0 || layout !== "fixed") return
    lane.scrollLeft = lane.scrollWidth
  }, [buckets, layout])

  async function openBucket(bucket: LatencyBucket) {
    if (detailMode === "inline" && selected?.start === bucket.start) {
      setSelected(null)
      setSamples([])
      setDetailError("")
      return
    }
    setSelected(bucket)
    setSamples([])
    setDetailError("")
    setDetailLoading(true)
    try {
      setSamples(await loadSamples(bucket))
    } catch (error) {
      setDetailError(error instanceof Error ? error.message : "读取调用明细失败")
    } finally {
      setDetailLoading(false)
    }
  }

  function openRecentSample(sample: LatencyDetailSample, bucket: LatencyBucket | null) {
    if (bucket) {
      void openBucket(bucket)
      return
    }
    const detailBucket = recentSampleBucket(sample)
    if (detailMode === "inline" && selected?.start === detailBucket.start) {
      setSelected(null)
      setSamples([])
      setDetailError("")
      return
    }
    setSelected(recentSampleBucket(sample))
    setSamples([sample])
    setDetailError("")
    setDetailLoading(false)
  }

  if (loading && buckets.length === 0) {
    return <div className="flex h-7 items-center text-[10px] text-muted-foreground">{isRecentMode ? "最近调用延时读取中…" : "时间稳定性读取中…"}</div>
  }
  const hasRequests = isRecentMode ? orderedRecentSamples.length > 0 : buckets.some((bucket) => bucket.request_count > 0)

  return (
    <>
      <div className={compact ? "" : "mt-3 border-t border-border pt-3"}>
        {!compact ? <div className="mb-1.5 flex flex-wrap items-center justify-between gap-x-2 gap-y-1">
          <span className="text-[11px] font-medium text-muted-foreground">{title ?? "时间稳定性"}</span>
          {headerRight ?? <span className="flex items-center gap-2 text-[10px] text-muted-foreground">
            <span>粒度 {latencyBucketGranularityLabel(granularity)}</span>
            <span>左 P50 · 右 P95</span>
          </span>}
        </div> : null}
        {(isRecentMode ? orderedRecentSamples.length === 0 : buckets.length === 0) || !hasRequests ? (
          <div className="flex h-7 items-center rounded border border-dashed border-border px-2 text-[10px] text-muted-foreground">{emptyLabel}</div>
        ) : (
          <div ref={laneRef} className={cn("max-w-full px-1 pb-1", layout === "fill" ? "overflow-hidden" : "overflow-x-auto")} aria-label={isRecentMode ? "最近调用延时泳道" : "时间稳定性泳道"}>
            <div className={cn("flex items-stretch gap-px", isRecentMode ? (compact ? "h-8" : "h-10") : "h-8", layout === "fill" ? "w-full" : "w-max")}>
              {isRecentMode
                ? orderedRecentSamples.map((sample, index) => {
                    const bucket = bucketForSample(buckets, sample)
                    return <RecentSampleCell key={sample.created_at + "-" + index} sample={sample} bucket={bucket} onOpen={openRecentSample} compact={compact} primarySamples={recentSamples ?? []} relay={relayConfig} channel={channelConfig} />
                  })
                : buckets.map((bucket) => <div key={bucket.start} className={layout === "fill" ? "min-w-0 flex-1" : "w-2 shrink-0"}><BucketCell bucket={bucket} onOpen={(value) => void openBucket(value)} relay={relayConfig} channel={channelConfig} /></div>)}
            </div>
          </div>
        )}
        {!compact ? <div className="mt-1 flex items-center gap-2 text-[9px] text-muted-foreground">
          <span>上首字 · 下总耗时 · 左 P50 · 右 P95</span>
          <span className="ml-auto flex items-center gap-1.5">
            <i className="size-1.5 rounded-full bg-emerald-500" />正常
            <i className="size-1.5 rounded-full bg-amber-400" />关注
            <i className="size-1.5 rounded-full bg-red-500" />异常
          </span>
        </div> : null}
      </div>

      {detailMode === "inline" && selected ? <InlineLatencyDetails selected={selected} samples={samples} detailLoading={detailLoading} detailError={detailError} onClose={() => setSelected(null)} /> : null}

      <Dialog open={detailMode === "dialog" && selected != null} onOpenChange={(open) => { if (!open) setSelected(null) }}>
        <DialogContent className="max-h-[82vh] gap-0 overflow-hidden p-0 sm:max-w-2xl">
          <DialogHeader className="border-b border-border px-5 py-4">
            <DialogTitle className="text-base">时间桶调用明细</DialogTitle>
            <DialogDescription>
              {selected ? formatBucketTime(selected.start) + " - " + formatBucketTime(selected.end) + " · 共 " + selected.request_count + " 次请求，默认展示最近 " + Math.min(30, selected.request_count) + " 条" : ""}
            </DialogDescription>
          </DialogHeader>
          {selected ? (
            <div className="mx-3 mt-3 grid grid-cols-2 gap-px overflow-hidden rounded-lg border border-border bg-border sm:grid-cols-[104px_repeat(3,minmax(0,1fr))]">
              <div className="bg-card px-4 py-3"><p className="text-[10px] text-muted-foreground">成功率</p><p className="mt-1 font-mono text-sm font-semibold tabular-nums">{selected.success_rate == null ? "渠道未提供" : `${(selected.success_rate * 100).toFixed(selected.success_rate === 1 ? 0 : 1)}%`}</p></div>
              <div className="bg-card px-4 py-3"><p className="text-[10px] text-muted-foreground">首字 P50 / P95</p><DetailPercentileValue p50={selected.first_token_p50_ms} p95={selected.first_token_p95_ms} format={formatLatency} /></div>
              <div className="bg-card px-4 py-3"><p className="text-[10px] text-muted-foreground">总耗时 P50 / P95</p><DetailPercentileValue p50={selected.duration_p50_ms} p95={selected.duration_p95_ms} format={formatLatency} /></div>
              <div className="bg-card px-4 py-3"><p className="text-[10px] text-muted-foreground">输出速度 P50 / P95</p><DetailPercentileValue p50={selected.output_token_speed_p50} p95={selected.output_token_speed_p95} format={formatOutputTokenSpeed} /></div>
            </div>
          ) : null}
          <div className="max-h-[58vh] overflow-y-auto bg-muted/30 p-3">
            {detailLoading ? <div className="py-10 text-center text-sm text-muted-foreground">正在读取调用明细…</div> : detailError ? <div className="py-10 text-center text-sm text-danger">{detailError}</div> : samples.length === 0 ? <div className="py-10 text-center text-sm text-muted-foreground">该时间桶暂无可展示的调用明细</div> : (
              <div className="space-y-2">
                {samples.map((sample, index) => {
                  const first = sample.first_token_ms == null ? null : latencyTone(sample.first_token_ms, "first")
                  const duration = sample.duration_ms == null ? null : latencyTone(sample.duration_ms, "duration")
                  const outputTokens = "output_tokens" in sample ? sample.output_tokens : null
                  const outputSpeed = outputTokenSpeed(outputTokens, sample.duration_ms)
                  const statusCode = "status_code" in sample ? sample.status_code : undefined
                  const requestType = "request_type" in sample ? sample.request_type : undefined
                  const userEmail = "user_email" in sample ? sample.user_email : undefined
                  const successKnown = "success_known" in sample
                    ? sample.success_known
                    : "success" in sample || statusCode != null
                  const successful = "success" in sample
                    ? sample.success
                    : statusCode == null || statusCode === 0 || (statusCode >= 200 && statusCode < 400)
                  return (
                    <div key={sample.created_at + "-" + index} className="rounded-lg border border-border bg-card p-3">
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <time className="font-mono text-xs font-semibold tabular-nums">{formatSampleTime(sample.created_at)}</time>
                        <div className="flex items-center gap-2">
                          {statusCode ? <span className={cn("rounded px-1.5 py-0.5 font-mono text-[10px] font-semibold", successful ? "bg-emerald-500/10 text-emerald-700" : "bg-red-500/10 text-red-700")}>HTTP {statusCode}</span> : successKnown ? <span className={cn("rounded px-1.5 py-0.5 text-[10px] font-semibold", successful ? "bg-emerald-500/10 text-emerald-700" : "bg-red-500/10 text-red-700")}>{successful ? "成功" : "失败"}</span> : null}
                          <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">#{index + 1}</span>
                        </div>
                      </div>
                      <div className="mt-2 flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
                        <span className="min-w-0 max-w-[240px] truncate font-mono text-xs font-semibold" title={sample.model || "-"}>{sample.model || "-"}</span>
                        {requestType ? <span className="text-[10px] text-muted-foreground">{requestType}</span> : null}
                        {userEmail ? <span className="min-w-0 max-w-[220px] truncate text-[10px] text-muted-foreground" title={userEmail}>{userEmail}</span> : null}
                      </div>
                      <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 rounded-md bg-muted/50 px-2.5 py-2 font-mono text-xs font-semibold tabular-nums">
                        <span className={first ? latencyTooltipTextClass[first] : "text-muted-foreground"}>首字 {sample.first_token_ms == null ? "渠道未提供" : formatLatency(sample.first_token_ms)}</span>
                        <span className={duration ? latencyTooltipTextClass[duration] : "text-muted-foreground"}>总耗时 {sample.duration_ms == null ? "渠道未提供" : formatLatency(sample.duration_ms)}</span>
                        <span>输出速度 {formatOutputTokenSpeed(outputSpeed)}</span>
                        <span className="text-muted-foreground">Token {totalTokens(sample).toLocaleString("zh-CN")}</span>
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}

