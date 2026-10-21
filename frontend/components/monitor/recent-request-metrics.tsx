import { useEffect, useState } from "react"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { recentRequestMetrics, type RecentRequestSample } from "@/lib/recent-request-metrics"

function Ring({rate, label, color, detail}: {rate: number | null; label: string; color: string; detail: string}) {
  const value = rate == null ? "—" : `${(rate * 100).toFixed(rate === 1 || rate === 0 ? 0 : 1)}%`
  return <Tooltip><TooltipTrigger asChild><button type="button" className="flex shrink-0 flex-col items-center gap-1 rounded-md p-1 outline-none focus-visible:ring-2 focus-visible:ring-ring" aria-label={`${label}：${rate == null ? "暂无数据" : value}`}>
    <svg viewBox="0 0 64 64" className="size-14" role="img" aria-label={`${label} ${value}`}><circle cx="32" cy="32" r="26" fill="none" stroke="currentColor" className="text-muted" strokeWidth="6"/><circle cx="32" cy="32" r="26" fill="none" stroke={color} strokeWidth="6" strokeLinecap="round" pathLength="100" strokeDasharray={`${(rate ?? 0) * 100} 100`} transform="rotate(-90 32 32)"/><text x="32" y="36" textAnchor="middle" fill="currentColor" className="text-foreground" fontSize="12" fontWeight="600">{value}</text></svg>
    <span className="text-[10px] text-muted-foreground">{label}</span>
  </button></TooltipTrigger><TooltipContent className="max-w-72 whitespace-pre-line text-xs leading-5">{detail}</TooltipContent></Tooltip>
}
export function RecentRequestMetrics({samples, loading = false}: {samples: readonly RecentRequestSample[]; loading?: boolean}) {
  const [now, setNow] = useState(Date.now)
  useEffect(() => { const timer = window.setInterval(() => setNow(Date.now()), 30000); return () => window.clearInterval(timer) }, [])
  const metrics = recentRequestMetrics(samples, now)
  const latestText = metrics.latestAt ? `\n最新调用：${new Date(metrics.latestAt).toLocaleString("zh-CN", {hour12: false})}` : ""
  const windowText = `最近 1 小时，最多 60 次调用。当前 ${metrics.sampleCount} 次。${latestText}`
  return <div className="flex flex-wrap items-center gap-2" aria-busy={loading}>
    <Ring rate={metrics.cacheHitRate} label="1h 缓存命中" color="#38bdf8" detail={`${windowText}\n计算：有缓存读取的请求次数 / 缓存状态已知次数。\n命中 ${metrics.cacheHitCount} / ${metrics.cacheKnownCount} 次；缓存读取 ${metrics.cacheReadTokens.toLocaleString("zh-CN")} tokens。\n未提供缓存字段的调用不进入分母。`}/>
    <Ring rate={metrics.successRate} label="1h 成功率" color="#34d399" detail={`${windowText}\n计算：成功次数 / 结果已知次数。\n成功 ${metrics.successCount} / ${metrics.successKnownCount} 次。未知状态不进入分母。\n只统计已采集的调用日志，不代表上游未暴露的内部重试。`}/>
    <span className="text-[10px] text-muted-foreground">{loading && !samples.length ? "读取中…" : `1h · ${metrics.sampleCount}/60 次`}</span>
  </div>
}
