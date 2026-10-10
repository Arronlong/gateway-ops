"use client"

import { useCallback, useLayoutEffect, useState, useRef } from "react"
import { createPortal } from "react-dom"
import type { ReactNode } from "react"
import type { ConsumptionBucket, LatencyBucket, LatencyBucketGranularity, RelayAccountConsumptionBucket, RelayUsageRange } from "@/lib/api-types"
import type { ChannelCompareLink, RelayCompareAccount, RelayCompareLink } from "@/lib/channel-relay-association"
import { useRelayAccountConsumptionBuckets, useRelayLatencyBuckets } from "@/lib/queries"
import { formatLatency, latencyTooltipTextClass, latencyTone } from "@/lib/latency"
import { money } from "@/lib/format"
import { pickRelayCompareBucket, type RelayCompareBucket } from "@/lib/relay-compare-bucket"
import { cn } from "@/lib/utils"

export type RelayCompareMetric = "latency" | "consumption"
export type RelayCompareTone = "light" | "theme"

interface ToneStyle {
  wrap: string
  header: string
  station: string
  account: string
  label: string
  value: string
  divider: string
  hint: string
}

const toneStyles: Record<RelayCompareTone, ToneStyle> = {
  light: {
    wrap: "rounded-lg bg-white p-2.5 shadow-sm ring-1 ring-slate-200/80",
    header: "text-slate-500",
    station: "text-slate-900",
    account: "text-slate-700",
    label: "text-slate-500",
    value: "text-slate-900",
    divider: "border-slate-200",
    hint: "text-slate-500",
  },
  theme: {
    wrap: "rounded-lg border border-border bg-popover p-2.5 text-popover-foreground shadow-xl",
    header: "text-muted-foreground",
    station: "text-foreground",
    account: "text-foreground",
    label: "text-muted-foreground",
    value: "text-foreground",
    divider: "border-border",
    hint: "text-muted-foreground",
  },
}

interface CompareBaseProps {
  links: RelayCompareLink[]
  range: RelayUsageRange
  granularity: LatencyBucketGranularity
  bucketCount: number
  /** 渠道侧时间桶的起点；缺省时取中转站最近一个有请求的桶。 */
  bucketStart?: string | null
  tone?: RelayCompareTone
}

interface StationCompareProps {
  link: RelayCompareLink
  range: RelayUsageRange
  granularity: LatencyBucketGranularity
  bucketCount: number
  bucketStart?: string | null
  showBucketWindow: boolean
  tone: RelayCompareTone
}

function formatBucketTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return "-"
  return date.toLocaleString("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false })
}

type BucketLike = RelayCompareBucket

function toneTextClass(value: number | null, metric: "first" | "duration", fallback: string) {
  if (value == null) return fallback
  return latencyTooltipTextClass[latencyTone(value, metric)]
}

function Row({ label, children, styles }: { label: string; children: React.ReactNode; styles: ToneStyle }) {
  return (
    <>
      <span className={cn("font-sans", styles.label)}>{label}</span>
      <span className={cn("font-mono font-semibold", styles.value)}>{children}</span>
    </>
  )
}

function AccountHeader({ account, bucket, styles, showBucketWindow }: { account: RelayCompareAccount; bucket: BucketLike | null; styles: ToneStyle; showBucketWindow: boolean }) {
  return (
    <div className="flex items-center justify-between gap-2">
      <span className={cn("min-w-0 truncate text-[11px] font-medium", styles.account)} title={account.name}>{account.name}</span>
      <span className={cn("shrink-0 font-mono text-[10px] tabular-nums", styles.hint)}>
        {bucket ? (showBucketWindow ? formatBucketTime(bucket.start) + " 起 · " + bucket.request_count + " 次请求" : bucket.request_count + " 次请求") : "该时段无数据"}
      </span>
    </div>
  )
}

function LatencyAccount({ account, buckets, bucketStart, showBucketWindow, styles }: { account: RelayCompareAccount; buckets: LatencyBucket[] | undefined; bucketStart?: string | null; showBucketWindow: boolean; styles: ToneStyle }) {
  const bucket = pickRelayCompareBucket(buckets, bucketStart)
  const firstP50 = bucket?.first_token_p50_ms ?? null
  const firstP95 = bucket?.first_token_p95_ms ?? null
  const durationP50 = bucket?.duration_p50_ms ?? null
  const durationP95 = bucket?.duration_p95_ms ?? null
  const successRate = bucket?.success_rate ?? null
  return (
    <div className={cn("mt-1.5 border-t pt-1.5 first:mt-0 first:border-t-0 first:pt-0", styles.divider)}>
      <AccountHeader account={account} bucket={bucket} styles={styles} showBucketWindow={showBucketWindow} />
      {bucket && bucket.request_count > 0 ? (
        <div className="mt-1 grid grid-cols-[auto_1fr] gap-x-2 gap-y-0.5 text-[10px] tabular-nums">
          <Row label="首字 P50 / P95" styles={styles}>
            <span className={toneTextClass(firstP50, "first", styles.hint)}>{formatLatency(firstP50)}</span>
            <span className={styles.hint}> / </span>
            <span className={toneTextClass(firstP95, "first", styles.hint)}>{formatLatency(firstP95)}</span>
          </Row>
          <Row label="总耗时 P50 / P95" styles={styles}>
            <span className={toneTextClass(durationP50, "duration", styles.hint)}>{formatLatency(durationP50)}</span>
            <span className={styles.hint}> / </span>
            <span className={toneTextClass(durationP95, "duration", styles.hint)}>{formatLatency(durationP95)}</span>
          </Row>
          <Row label="成功率" styles={styles}>
            {successRate == null ? "未提供" : (successRate * 100).toFixed(successRate === 1 ? 0 : 1) + "%"}
          </Row>
        </div>
      ) : null}
    </div>
  )
}

function ConsumptionAccount({ account, buckets, bucketStart, showBucketWindow, styles }: { account: RelayCompareAccount; buckets: RelayAccountConsumptionBucket[] | undefined; bucketStart?: string | null; showBucketWindow: boolean; styles: ToneStyle }) {
  const bucket = pickRelayCompareBucket(buckets, bucketStart)
  return (
    <div className={cn("mt-1.5 border-t pt-1.5 first:mt-0 first:border-t-0 first:pt-0", styles.divider)}>
      <AccountHeader account={account} bucket={bucket} styles={styles} showBucketWindow={showBucketWindow} />
      {bucket && bucket.request_count > 0 ? (
        <div className="mt-1 grid grid-cols-[auto_1fr] gap-x-2 gap-y-0.5 text-[10px] tabular-nums">
          <Row label="用户消费" styles={styles}>{money(bucket.user_charge, { precise: bucket.user_charge < 1 })}</Row>
          <Row label="账号成本" styles={styles}>{money(bucket.account_cost, { precise: bucket.account_cost < 1 })}</Row>
        </div>
      ) : null}
    </div>
  )
}

function LatencyStation({ link, range, granularity, bucketCount, bucketStart, showBucketWindow, tone }: StationCompareProps) {
  const styles = toneStyles[tone]
  const { data, loading } = useRelayLatencyBuckets(link.stationID, range, granularity, bucketCount)
  const series = data ?? []
  return (
    <div>
      <p className={cn("truncate text-[10px] font-medium", styles.station)} title={link.stationName}>{link.stationName}</p>
      {loading && series.length === 0 ? <p className={cn("mt-1 text-[10px]", styles.hint)}>读取中转站数据…</p> : link.accounts.map((account) => (
        <LatencyAccount key={account.externalID} account={account} buckets={series.find((item) => item.account_id === account.externalID)?.buckets} bucketStart={bucketStart} showBucketWindow={showBucketWindow} styles={styles} />
      ))}
    </div>
  )
}

function ConsumptionStation({ link, range, granularity, bucketCount, bucketStart, showBucketWindow, tone }: StationCompareProps) {
  const styles = toneStyles[tone]
  const { data, loading } = useRelayAccountConsumptionBuckets(link.stationID, range, granularity, bucketCount)
  const series = data ?? []
  return (
    <div>
      <p className={cn("truncate text-[10px] font-medium", styles.station)} title={link.stationName}>{link.stationName}</p>
      {loading && series.length === 0 ? <p className={cn("mt-1 text-[10px]", styles.hint)}>读取中转站数据…</p> : link.accounts.map((account) => (
        <ConsumptionAccount key={account.externalID} account={account} buckets={series.find((item) => item.account_id === account.externalID)?.buckets} bucketStart={bucketStart} showBucketWindow={showBucketWindow} styles={styles} />
      ))}
    </div>
  )
}

function ConsumptionPairValue({ channelValue, relayValue }: { channelValue: number | null; relayValue: number | null }) {
  return (
    <span className="whitespace-nowrap font-mono font-semibold tabular-nums">
      <span className="text-success">{channelValue == null ? "-" : money(channelValue, { precise: channelValue < 1 })}</span>
      <span className="px-1 text-muted-foreground">/</span>
      <span className="text-blue-700 dark:text-blue-300">{relayValue == null ? "-" : money(relayValue, { precise: relayValue < 1 })}</span>
    </span>
  )
}

function RelayConsumptionComparisonAccount({ account, channelBucket, relayBucket }: {
  account: RelayCompareAccount
  channelBucket: ConsumptionBucket
  relayBucket: RelayAccountConsumptionBucket | null
}) {
  return (
    <div className="mt-2 border-t border-border pt-2 first:mt-0 first:border-t-0 first:pt-0">
      <span className="block min-w-0 truncate text-[11px] font-semibold text-foreground" title={account.name}>{account.name}</span>
      <div className="mt-2 grid grid-cols-[82px_1fr] items-center gap-x-3 gap-y-1.5 rounded-md border border-border bg-muted/45 p-2 text-xs">
        <span className="text-muted-foreground">调用消耗</span>
        <ConsumptionPairValue channelValue={channelBucket.amount} relayValue={relayBucket?.user_charge ?? null} />
        <span className="text-muted-foreground">账号成本</span>
        <ConsumptionPairValue channelValue={null} relayValue={relayBucket?.account_cost ?? null} />
        <span className="text-muted-foreground">请求数</span>
        <span className="whitespace-nowrap font-mono font-semibold tabular-nums text-foreground">
          <span className="text-muted-foreground">-</span><span className="px-1 text-muted-foreground">/</span>{relayBucket?.request_count ?? "-"}
        </span>
      </div>
    </div>
  )
}

function RelayConsumptionComparisonStation({ link, channelBucket, range, granularity, bucketCount }: {
  link: RelayCompareLink
  channelBucket: ConsumptionBucket
  range: RelayUsageRange
  granularity: LatencyBucketGranularity
  bucketCount: number
}) {
  const { data, loading } = useRelayAccountConsumptionBuckets(link.stationID, range, granularity, bucketCount)
  const series = data ?? []
  return (
    <section className="mt-2 border-t border-border pt-2 first:mt-0 first:border-t-0 first:pt-0">
      <div className="flex items-center justify-between gap-2">
        <span className="min-w-0 truncate text-[11px] font-medium text-muted-foreground" title={link.stationName}>{link.stationName}</span>
        <span className="shrink-0 text-[10px] text-muted-foreground">{link.accounts.length} 个关联账号</span>
      </div>
      {loading && series.length === 0 ? (
        <p className="mt-2 text-[10px] text-muted-foreground">正在读取中转站数据…</p>
      ) : link.accounts.map((account) => (
        <RelayConsumptionComparisonAccount
          key={account.externalID}
          account={account}
          channelBucket={channelBucket}
          relayBucket={pickRelayCompareBucket(series.find((item) => item.account_id === account.externalID)?.buckets, channelBucket.start)}
        />
      ))}
    </section>
  )
}

export function RelayConsumptionComparisonTooltip({ bucket, links, range, granularity, bucketCount }: {
  bucket: ConsumptionBucket
  links: RelayCompareLink[]
  range: RelayUsageRange
  granularity: LatencyBucketGranularity
  bucketCount: number
}) {
  return (
    <div className="w-[min(23rem,calc(100vw-32px))] rounded-lg border border-border bg-popover p-3 text-xs text-popover-foreground shadow-2xl ring-1 ring-black/5">
      <div className="flex items-start justify-between gap-4">
        <p className="font-semibold text-foreground">{formatBucketTime(bucket.start)} - {formatBucketTime(bucket.end)}</p>
        <span className="flex shrink-0 items-center gap-1 rounded-md border border-border bg-card px-2 py-1 text-[10px] font-medium shadow-sm">
          <span className="text-muted-foreground">数值顺序</span>
          <span className="rounded bg-slate-900 px-1.5 py-0.5 text-white dark:bg-slate-100 dark:text-slate-900">渠道</span>
          <span className="text-muted-foreground">/</span><span className="rounded bg-blue-500/15 px-1.5 py-0.5 text-blue-700 dark:text-blue-300">中转站</span>
        </span>
      </div>
      <div className="mt-2 rounded-lg border border-border bg-popover p-2.5 shadow-sm">
        {links.map((link) => (
          <RelayConsumptionComparisonStation
            key={link.stationID}
            link={link}
            channelBucket={bucket}
            range={range}
            granularity={granularity}
            bucketCount={bucketCount}
          />
        ))}
      </div>
    </div>
  )
}

function successRateText(value: number | null | undefined, missing = "未提供") {
  if (value == null) return missing
  return `${(value * 100).toFixed(value === 1 ? 0 : 1)}%`
}

function LatencyPairValue({ channelValue, relayValue, metric }: {
  channelValue: number | null
  relayValue: number | null
  metric: "first" | "duration"
}) {
  return (
    <span className="whitespace-nowrap font-mono font-semibold tabular-nums">
      <span className={toneTextClass(channelValue, metric, "text-muted-foreground")}>{formatLatency(channelValue)}</span>
      <span className="px-1 text-muted-foreground">/</span>
      <span className={toneTextClass(relayValue, metric, "text-muted-foreground")}>{formatLatency(relayValue)}</span>
    </span>
  )
}

function RelayLatencyComparisonAccount({ account, channelBucket, relayBucket }: {
  account: RelayCompareAccount
  channelBucket: LatencyBucket
  relayBucket: LatencyBucket | null
}) {
  return (
    <div className="mt-2 border-t border-border pt-2 first:mt-0 first:border-t-0 first:pt-0">
      <div className="flex min-w-0 items-center justify-between gap-3">
        <span className="min-w-0 truncate text-[11px] font-semibold text-foreground" title={account.name}>{account.name}</span>
      </div>
      <div className="mt-2 grid grid-cols-[68px_1fr_1fr] items-center gap-x-3 gap-y-1.5 rounded-md border border-border bg-muted/45 p-2 text-xs">
        <span />
        <span className="text-[10px] font-medium text-muted-foreground">P50</span>
        <span className="text-[10px] font-medium text-muted-foreground">P95</span>
        <span className="text-muted-foreground">首字</span>
        <LatencyPairValue channelValue={channelBucket.first_token_p50_ms} relayValue={relayBucket?.first_token_p50_ms ?? null} metric="first" />
        <LatencyPairValue channelValue={channelBucket.first_token_p95_ms} relayValue={relayBucket?.first_token_p95_ms ?? null} metric="first" />
        <span className="text-muted-foreground">总耗时</span>
        <LatencyPairValue channelValue={channelBucket.duration_p50_ms} relayValue={relayBucket?.duration_p50_ms ?? null} metric="duration" />
        <LatencyPairValue channelValue={channelBucket.duration_p95_ms} relayValue={relayBucket?.duration_p95_ms ?? null} metric="duration" />
      </div>
      <div className="mt-2 grid grid-cols-2 gap-2">
        <div className="rounded-md border border-border bg-card px-2.5 py-2">
          <p className="text-[10px] text-muted-foreground">成功率</p>
          <p className="mt-0.5 whitespace-nowrap font-mono text-xs font-semibold tabular-nums text-foreground">
            {successRateText(channelBucket.success_rate, "渠道未提供")} <span className="text-muted-foreground">/</span> {successRateText(relayBucket?.success_rate, "-")}
          </p>
        </div>
        <div className="rounded-md border border-border bg-card px-2.5 py-2">
          <p className="text-[10px] text-muted-foreground">请求数</p>
          <p className="mt-0.5 whitespace-nowrap font-mono text-xs font-semibold tabular-nums text-foreground">
            {channelBucket.request_count} <span className="text-muted-foreground">/</span> {relayBucket?.request_count ?? "-"}
          </p>
        </div>
      </div>
    </div>
  )
}

function RelayLatencyComparisonStation({ link, channelBucket, range, granularity, bucketCount }: {
  link: RelayCompareLink
  channelBucket: LatencyBucket
  range: RelayUsageRange
  granularity: LatencyBucketGranularity
  bucketCount: number
}) {
  const { data, loading } = useRelayLatencyBuckets(link.stationID, range, granularity, bucketCount)
  const series = data ?? []
  return (
    <section className="mt-2 border-t border-border pt-2 first:mt-0 first:border-t-0 first:pt-0">
      <div className="flex items-center justify-between gap-2">
        <span className="min-w-0 truncate text-[11px] font-medium text-muted-foreground" title={link.stationName}>{link.stationName}</span>
        <span className="shrink-0 text-[10px] text-muted-foreground">{link.accounts.length} 个关联账号</span>
      </div>
      {loading && series.length === 0 ? (
        <p className="mt-2 text-[10px] text-muted-foreground">正在读取中转站数据…</p>
      ) : link.accounts.map((account) => (
        <RelayLatencyComparisonAccount
          key={account.externalID}
          account={account}
          channelBucket={channelBucket}
          relayBucket={pickRelayCompareBucket(series.find((item) => item.account_id === account.externalID)?.buckets, channelBucket.start)}
        />
      ))}
    </section>
  )
}

export function RelayLatencyComparisonTooltip({ bucket, links, range, granularity, bucketCount }: {
  bucket: LatencyBucket
  links: RelayCompareLink[]
  range: RelayUsageRange
  granularity: LatencyBucketGranularity
  bucketCount: number
}) {
  return (
    <div className="w-[min(24rem,calc(100vw-32px))] p-1 text-xs">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="font-semibold text-foreground">{formatBucketTime(bucket.start)} - {formatBucketTime(bucket.end)}</p>
          <p className="mt-0.5 text-[10px] text-muted-foreground">点击查看该时间桶的调用明细</p>
        </div>
        <span className="flex shrink-0 items-center gap-1 rounded-md border border-border bg-card px-2 py-1 text-[10px] font-medium shadow-sm">
          <span className="text-muted-foreground">数值顺序</span>
          <span className="rounded bg-slate-900 px-1.5 py-0.5 text-white dark:bg-slate-100 dark:text-slate-900">渠道</span>
          <span className="text-muted-foreground">/</span><span className="rounded bg-blue-500/15 px-1.5 py-0.5 text-blue-700 dark:text-blue-300">中转站</span>
        </span>
      </div>
      <div className="mt-2 rounded-lg border border-border bg-popover p-2.5 shadow-sm">
        {links.map((link) => (
          <RelayLatencyComparisonStation
            key={link.stationID}
            link={link}
            channelBucket={bucket}
            range={range}
            granularity={granularity}
            bucketCount={bucketCount}
          />
        ))}
      </div>
    </div>
  )
}

function RelayFirstLatencyPairValue({ relayValue, channelValue, metric }: {
  relayValue: number | null
  channelValue: number | null
  metric: "first" | "duration"
}) {
  return (
    <span className="whitespace-nowrap font-mono font-semibold tabular-nums">
      <span className={toneTextClass(relayValue, metric, "text-muted-foreground")}>{formatLatency(relayValue)}</span>
      <span className="px-1 text-muted-foreground">/</span>
      <span className={toneTextClass(channelValue, metric, "text-muted-foreground")}>{formatLatency(channelValue)}</span>
    </span>
  )
}

function ChannelLatencyComparisonEntry({ relayAccountName, link, relayBucket, channelBucket }: {
  relayAccountName: string
  link: ChannelCompareLink
  relayBucket: LatencyBucket
  channelBucket: LatencyBucket | null
}) {
  return (
    <section className="mt-2 border-t border-border pt-2 first:mt-0 first:border-t-0 first:pt-0">
      <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          <span className="block truncate text-[11px] font-semibold text-foreground" title={link.channelName}>{link.channelName}</span>
          <span className="block truncate text-[10px] text-muted-foreground" title={link.accountNames.join("、")}>{link.accountNames.join("、") || "关联渠道账号"}</span>
        </div>
        <span className="shrink-0 text-[10px] text-muted-foreground">中转站 / 渠道</span>
      </div>
      <div className="mt-2 grid grid-cols-[68px_1fr_1fr] items-center gap-x-3 gap-y-1.5 rounded-md border border-border bg-muted/45 p-2 text-xs">
        <span />
        <span className="text-[10px] font-medium text-muted-foreground">P50</span>
        <span className="text-[10px] font-medium text-muted-foreground">P95</span>
        <span className="text-muted-foreground">首字</span>
        <RelayFirstLatencyPairValue relayValue={relayBucket.first_token_p50_ms} channelValue={channelBucket?.first_token_p50_ms ?? null} metric="first" />
        <RelayFirstLatencyPairValue relayValue={relayBucket.first_token_p95_ms} channelValue={channelBucket?.first_token_p95_ms ?? null} metric="first" />
        <span className="text-muted-foreground">总耗时</span>
        <RelayFirstLatencyPairValue relayValue={relayBucket.duration_p50_ms} channelValue={channelBucket?.duration_p50_ms ?? null} metric="duration" />
        <RelayFirstLatencyPairValue relayValue={relayBucket.duration_p95_ms} channelValue={channelBucket?.duration_p95_ms ?? null} metric="duration" />
      </div>
      <div className="mt-2 grid grid-cols-2 gap-2">
        <div className="rounded-md border border-border bg-card px-2.5 py-2">
          <p className="text-[10px] text-muted-foreground">成功率</p>
          <p className="mt-0.5 whitespace-nowrap font-mono text-xs font-semibold tabular-nums text-foreground">
            {successRateText(relayBucket.success_rate, "-")} <span className="text-muted-foreground">/</span> {successRateText(channelBucket?.success_rate, "-")}
          </p>
        </div>
        <div className="rounded-md border border-border bg-card px-2.5 py-2">
          <p className="text-[10px] text-muted-foreground">请求数</p>
          <p className="mt-0.5 whitespace-nowrap font-mono text-xs font-semibold tabular-nums text-foreground">
            {relayBucket.request_count} <span className="text-muted-foreground">/</span> {channelBucket?.request_count ?? "-"}
          </p>
        </div>
      </div>
      <p className="mt-1 text-[9px] text-muted-foreground">当前账号：{relayAccountName}</p>
    </section>
  )
}

export function ChannelLatencyComparisonTooltip({ bucket, relayAccountName, links, channelBucketsByChannel }: {
  bucket: LatencyBucket
  relayAccountName: string
  links: ChannelCompareLink[]
  channelBucketsByChannel: Map<number, LatencyBucket[]>
}) {
  return (
    <div className="w-[min(24rem,calc(100vw-32px))] p-1 text-xs">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="font-semibold text-foreground">{formatBucketTime(bucket.start)} - {formatBucketTime(bucket.end)}</p>
          <p className="mt-0.5 text-[10px] text-muted-foreground">点击查看该时间桶的调用明细</p>
        </div>
        <span className="flex shrink-0 items-center gap-1 rounded-md border border-border bg-card px-2 py-1 text-[10px] font-medium shadow-sm">
          <span className="text-muted-foreground">数值顺序</span>
          <span className="rounded bg-blue-500/15 px-1.5 py-0.5 text-blue-700 dark:text-blue-300">中转站</span>
          <span className="text-muted-foreground">/</span><span className="rounded bg-slate-900 px-1.5 py-0.5 text-white dark:bg-slate-100 dark:text-slate-900">渠道</span>
        </span>
      </div>
      <div className="mt-2 rounded-lg border border-border bg-popover p-2.5 shadow-sm">
        {links.map((link) => (
          <ChannelLatencyComparisonEntry
            key={link.channelID}
            relayAccountName={relayAccountName}
            link={link}
            relayBucket={bucket}
            channelBucket={pickRelayCompareBucket(channelBucketsByChannel.get(link.channelID), bucket.start)}
          />
        ))}
      </div>
    </div>
  )
}

function pickChannelConsumptionBucket(buckets: ConsumptionBucket[] | undefined, start: string) {
  if (!buckets || buckets.length === 0) return null
  const exact = buckets.find((bucket) => bucket.start === start)
  if (exact) return exact
  const target = Date.parse(start)
  if (!Number.isFinite(target)) return buckets[buckets.length - 1] ?? null
  const covering = buckets.find((bucket) => {
    const from = Date.parse(bucket.start)
    const to = Date.parse(bucket.end)
    return Number.isFinite(from) && Number.isFinite(to) && from <= target && target < to
  })
  if (covering) return covering
  return buckets.reduce<ConsumptionBucket | null>((nearest, bucket) => {
    if (!nearest) return bucket
    return Math.abs(Date.parse(bucket.start) - target) < Math.abs(Date.parse(nearest.start) - target) ? bucket : nearest
  }, null)
}

export function ChannelConsumptionComparisonTooltip({ bucket, relayAccountName, links, channelBucketsByChannel }: {
  bucket: RelayAccountConsumptionBucket
  relayAccountName: string
  links: ChannelCompareLink[]
  channelBucketsByChannel: Map<number, ConsumptionBucket[]>
}) {
  return (
    <div className="w-[min(23rem,calc(100vw-32px))] rounded-lg border border-border bg-popover p-3 text-xs text-popover-foreground shadow-2xl ring-1 ring-black/5">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="font-semibold text-foreground">{formatBucketTime(bucket.start)} - {formatBucketTime(bucket.end)}</p>
          <p className="mt-0.5 text-[10px] text-muted-foreground">中转站账号与关联渠道的同一时间段对照</p>
        </div>
        <span className="flex shrink-0 items-center gap-1 rounded-md border border-border bg-card px-2 py-1 text-[10px] font-medium shadow-sm">
          <span className="text-muted-foreground">数值顺序</span>
          <span className="rounded bg-blue-500/15 px-1.5 py-0.5 text-blue-700 dark:text-blue-300">中转站</span>
          <span className="text-muted-foreground">/</span><span className="rounded bg-slate-900 px-1.5 py-0.5 text-white dark:bg-slate-100 dark:text-slate-900">渠道</span>
        </span>
      </div>
      <div className="mt-2 rounded-lg border border-border bg-popover p-2.5 shadow-sm">
        {links.map((link) => {
          const channelBucket = pickChannelConsumptionBucket(channelBucketsByChannel.get(link.channelID), bucket.start)
          return (
            <section key={link.channelID} className="mt-2 border-t border-border pt-2 first:mt-0 first:border-t-0 first:pt-0">
              <div className="flex min-w-0 items-start justify-between gap-3">
                <div className="min-w-0">
                  <span className="block truncate text-[11px] font-semibold text-foreground" title={link.channelName}>{link.channelName}</span>
                  <span className="block truncate text-[10px] text-muted-foreground" title={link.accountNames.join("、")}>{link.accountNames.join("、") || "关联渠道账号"}</span>
                </div>
                <span className="shrink-0 text-[10px] text-muted-foreground">中转站 / 渠道</span>
              </div>
              <div className="mt-2 grid grid-cols-[92px_1fr] items-center gap-x-3 gap-y-1.5 rounded-md border border-border bg-muted/45 p-2 text-xs">
                <span className="text-muted-foreground">用户消费 / 调用消耗</span>
                <span className="whitespace-nowrap font-mono font-semibold tabular-nums"><span className="text-blue-700 dark:text-blue-300">{money(bucket.user_charge, { precise: bucket.user_charge < 1 })}</span><span className="px-1 text-muted-foreground">/</span><span className="text-slate-700 dark:text-slate-200">{channelBucket ? money(channelBucket.amount, { precise: channelBucket.amount < 1 }) : "-"}</span></span>
                <span className="text-muted-foreground">账号成本</span>
                <span className="font-mono font-semibold tabular-nums text-blue-700 dark:text-blue-300">{money(bucket.account_cost, { precise: bucket.account_cost < 1 })}</span>
                <span className="text-muted-foreground">请求数</span>
                <span className="whitespace-nowrap font-mono font-semibold tabular-nums text-foreground">{bucket.request_count}<span className="px-1 text-muted-foreground">/</span>-</span>
              </div>
              <p className="mt-1 text-[9px] text-muted-foreground">当前账号：{relayAccountName}</p>
            </section>
          )
        })}
      </div>
    </div>
  )
}

export type RelayCompareSide = "left" | "right"

const RELAY_COMPARE_PANEL_WIDTH = 360
const RELAY_COMPARE_PANEL_MIN_WIDTH = 220
const RELAY_COMPARE_GAP = 12
const RELAY_COMPARE_VIEWPORT_MARGIN = 12
const RELAY_COMPARE_ESTIMATED_HEIGHT = 360

interface RelayComparePlacement {
  top: number
  left: number
  width: number
  side: RelayCompareSide
}

function resolveRelayComparePlacement(
  anchorRect: Pick<DOMRect, "left" | "right" | "top">,
  preferredSide: RelayCompareSide,
  viewportWidth: number,
  viewportHeight: number,
): RelayComparePlacement {
  const maxPanelWidth = Math.max(1, Math.min(RELAY_COMPARE_PANEL_WIDTH, viewportWidth - RELAY_COMPARE_VIEWPORT_MARGIN * 2))
  const rightSpace = Math.max(0, viewportWidth - RELAY_COMPARE_VIEWPORT_MARGIN - anchorRect.right - RELAY_COMPARE_GAP)
  const leftSpace = Math.max(0, anchorRect.left - RELAY_COMPARE_VIEWPORT_MARGIN - RELAY_COMPARE_GAP)
  const preferredSpace = preferredSide === "right" ? rightSpace : leftSpace
  const oppositeSpace = preferredSide === "right" ? leftSpace : rightSpace
  const side = preferredSpace >= RELAY_COMPARE_PANEL_MIN_WIDTH
    ? preferredSide
    : oppositeSpace >= RELAY_COMPARE_PANEL_MIN_WIDTH
      ? preferredSide === "right" ? "left" : "right"
      : preferredSpace >= oppositeSpace
        ? preferredSide
        : preferredSide === "right" ? "left" : "right"
  const sideSpace = side === "right" ? rightSpace : leftSpace
  const width = Math.max(1, Math.min(maxPanelWidth, sideSpace))
  const left = side === "right"
    ? anchorRect.right + RELAY_COMPARE_GAP
    : anchorRect.left - RELAY_COMPARE_GAP - width
  const maxTop = Math.max(RELAY_COMPARE_VIEWPORT_MARGIN, viewportHeight - RELAY_COMPARE_VIEWPORT_MARGIN - RELAY_COMPARE_ESTIMATED_HEIGHT)
  const top = Math.max(RELAY_COMPARE_VIEWPORT_MARGIN, Math.min(anchorRect.top, maxTop))

  return { top, left, width, side }
}

/**
 * 根据触发元素在视口中的水平位置给出对照面板的初始方向。
 * 最终位置还会以当前渠道浮层的实际矩形为基准再次校正。
 */
export function useRelayCompareSide() {
  const [side, setSide] = useState<RelayCompareSide>("right")
  const update = useCallback((element: HTMLElement | null, pointerX?: number) => {
    if (!element || typeof window === "undefined") return
    const rect = element.getBoundingClientRect()
    const x = pointerX ?? (rect.left + rect.width / 2)
    const rightSpace = window.innerWidth - x
    const leftSpace = x
    setSide(rightSpace >= 360 ? "right" : leftSpace >= 360 ? "left" : rightSpace >= leftSpace ? "right" : "left")
  }, [])
  return { side, update }
}

type RelayCompareHoverProps = CompareBaseProps & { metric: RelayCompareMetric; side: RelayCompareSide; className?: string; children: ReactNode }

/**
 * 将当前渠道浮层与中转站对照面板分开定位：当前渠道信息保持在触发点附近，
 * 对照面板只放在当前渠道浮层的外侧，避免在视口边缘发生覆盖。
 * 未关联中转站账号时直接返回原内容，不增加任何包装。
 */
export function RelayCompareHover({ side, className, children, ...panel }: RelayCompareHoverProps) {
  const anchorRef = useRef<HTMLDivElement | null>(null)
  const [position, setPosition] = useState<RelayComparePlacement | null>(null)
  const frameRef = useRef<number | null>(null)

  const updatePosition = useCallback(() => {
    const anchor = anchorRef.current
    if (!anchor || typeof window === "undefined") return
    const tooltip = anchor.closest<HTMLElement>('[data-slot="tooltip-content"]')
    const rect = (tooltip ?? anchor).getBoundingClientRect()
    setPosition(resolveRelayComparePlacement(rect, side, window.innerWidth, window.innerHeight))
  }, [side])

  const schedulePositionUpdate = useCallback(() => {
    if (typeof window === "undefined") return
    if (frameRef.current != null) window.cancelAnimationFrame(frameRef.current)
    frameRef.current = window.requestAnimationFrame(() => {
      // Radix Tooltip 可能会在这一帧里完成视口碰撞修正，再延后一帧读取最终矩形。
      frameRef.current = window.requestAnimationFrame(() => {
        frameRef.current = null
        updatePosition()
      })
    })
  }, [updatePosition])

  useLayoutEffect(() => {
    if (panel.links.length === 0) return
    schedulePositionUpdate()
    const resizeObserver = typeof ResizeObserver === "undefined"
      ? null
      : new ResizeObserver(schedulePositionUpdate)
    const mutationObserver = typeof MutationObserver === "undefined"
      ? null
      : new MutationObserver(schedulePositionUpdate)
    if (anchorRef.current) {
      const tooltip = anchorRef.current.closest<HTMLElement>('[data-slot="tooltip-content"]')
      resizeObserver?.observe(tooltip ?? anchorRef.current)
      if (tooltip) {
        mutationObserver?.observe(tooltip, {
          attributes: true,
          attributeFilter: ["class", "style", "data-side", "data-align", "data-state"],
        })
      }
    }
    window.addEventListener("resize", schedulePositionUpdate)
    window.addEventListener("scroll", schedulePositionUpdate, true)
    return () => {
      if (frameRef.current != null) window.cancelAnimationFrame(frameRef.current)
      frameRef.current = null
      resizeObserver?.disconnect()
      mutationObserver?.disconnect()
      window.removeEventListener("resize", schedulePositionUpdate)
      window.removeEventListener("scroll", schedulePositionUpdate, true)
    }
  }, [panel.bucketStart, panel.links.length, schedulePositionUpdate])

  if (panel.links.length === 0) return <>{children}</>

  return (
    <>
      <div ref={anchorRef} className={cn("relative z-10 shrink-0", className)}>{children}</div>
      {position && typeof document !== "undefined" ? createPortal(
        <div
          className="pointer-events-auto fixed z-[100] max-w-[calc(100vw-24px)] overflow-y-auto"
          data-side={position.side}
          style={{
            top: position.top,
            left: position.left,
            width: position.width,
            maxHeight: "calc(100vh - 24px)",
          }}
        >
          <RelayComparePanel {...panel} />
        </div>,
        document.body,
      ) : null}
    </>
  )
}

/**
 * 渠道指标悬停时，并排展示该渠道已关联中转站账号的同类数据。
 * 未关联中转站账号的渠道不会渲染任何内容。
 */
export function RelayComparePanel({ links, metric, range, granularity, bucketCount, bucketStart, tone = "theme" }: CompareBaseProps & { metric: RelayCompareMetric }) {
  const styles = toneStyles[tone]
  if (links.length === 0) return null
  const showBucketWindow = !bucketStart
  const accountCount = links.reduce((sum, link) => sum + link.accounts.length, 0)
  const shared: StationCompareProps = { link: links[0], range, granularity, bucketCount, bucketStart, showBucketWindow, tone }
  return (
    <div className={styles.wrap}>
      <div className="flex items-center justify-between gap-2">
        <span className={cn("text-[10px] font-semibold", styles.header)}>中转站账号对照</span>
        <span className={cn("font-mono text-[10px] tabular-nums", styles.hint)}>{accountCount} 个账号</span>
      </div>
      <div className="mt-1.5">
        {links.map((link) => {
          const props: StationCompareProps = { ...shared, link }
          return metric === "latency"
            ? <LatencyStation key={link.stationID} {...props} />
            : <ConsumptionStation key={link.stationID} {...props} />
        })}
      </div>
    </div>
  )
}

