import { useEffect, useState } from "react"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { recentRequestMetrics, type RecentRequestSample, type RecentRequestMetricValues } from "@/lib/recent-request-metrics"

function Ring({rate, label, color, detail}: {rate: number | null; label: string; color: string; detail: string}) {
  const value = rate == null ? "—" : `${(rate * 100).toFixed(rate === 1 || rate === 0 ? 0 : 1)}%`
  return <Tooltip><TooltipTrigger asChild><button type="button" className="flex shrink-0 flex-col items-center gap-1 rounded-md p-1 outline-none focus-visible:ring-2 focus-visible:ring-ring" aria-label={`${label}：${rate == null ? "暂无数据" : value}`}>
    <svg viewBox="0 0 64 64" className="size-14" role="img" aria-label={`${label} ${value}`}><circle cx="32" cy="32" r="26" fill="none" stroke="currentColor" className="text-muted" strokeWidth="6"/><circle cx="32" cy="32" r="26" fill="none" stroke={color} strokeWidth="6" strokeLinecap="round" pathLength="100" strokeDasharray={`${(rate ?? 0) * 100} 100`} transform="rotate(-90 32 32)"/><text x="32" y="36" textAnchor="middle" fill="currentColor" className="text-foreground" fontSize="12" fontWeight="600">{value}</text></svg>
    <span className="text-[10px] text-muted-foreground">{label}</span>
  </button></TooltipTrigger><TooltipContent className="max-w-72 whitespace-pre-line text-xs leading-5">{detail}</TooltipContent></Tooltip>
}
function TpsRing({tps, score, label, detail}: {tps: number | null; score: number | null; label: string; detail: string}) {
  const value = tps == null ? "—" : Number(tps.toFixed(1)).toLocaleString("zh-CN")
  const progress = score ?? 0
  return <Tooltip><TooltipTrigger asChild><button type="button" className="flex shrink-0 flex-col items-center gap-1 rounded-md p-1 outline-none focus-visible:ring-2 focus-visible:ring-ring" aria-label={`${label}：${tps == null ? "不可用" : `${value} Token/s`}`}>
    <svg viewBox="0 0 64 64" className="size-14" role="img" aria-label={`${label} ${value}`}><circle cx="32" cy="32" r="26" fill="none" stroke="currentColor" className="text-muted" strokeWidth="6"/><circle cx="32" cy="32" r="26" fill="none" stroke="#38bdf8" strokeWidth="6" strokeLinecap="round" pathLength="100" strokeDasharray={`${progress} 100`} transform="rotate(-90 32 32)"/><text x="32" y="36" textAnchor="middle" fill="currentColor" className="text-foreground" fontSize="12" fontWeight="600">{value}</text></svg>
    <span className="text-[10px] text-muted-foreground">{label}</span>
  </button></TooltipTrigger><TooltipContent className="max-w-72 whitespace-pre-line text-xs leading-5">{detail}</TooltipContent></Tooltip>
}
export function RecentRequestMetrics({samples, loading = false, windowHours = 1, sampleLimit = 60}: {samples: readonly RecentRequestSample[]; loading?: boolean; windowHours?: number; sampleLimit?: number}) {
  const [now, setNow] = useState(Date.now)
  useEffect(() => { const timer = window.setInterval(() => setNow(Date.now()), 30000); return () => window.clearInterval(timer) }, [])
  const metrics = recentRequestMetrics(samples, now, windowHours, sampleLimit)
  return <RecentRequestMetricsDisplay metrics={metrics} loading={loading} />
}

export function RecentRequestMetricsDisplay({metrics, loading = false, showSampleCount = true}: {metrics: RecentRequestMetricValues; loading?: boolean; showSampleCount?: boolean}) {
  const latestText = metrics.latestAt ? `\n最新调用：${new Date(metrics.latestAt).toLocaleString("zh-CN", {hour12: false})}` : ""
  const windowText = `最近 ${metrics.windowHours} 小时，截止当前时间前 45 秒，最多 ${metrics.sampleLimit} 次调用。当前 ${metrics.sampleCount} 次。${latestText}`
  const tpsValue = metrics.tps == null ? "不可用" : `${Number(metrics.tps.toFixed(1)).toLocaleString("zh-CN")} Token/s`
  const tpsScore = metrics.tpsScore == null ? "不可用" : Number(metrics.tpsScore.toFixed(1)).toLocaleString("zh-CN")
  const tpsCalculation = metrics.tps == null ? "TPS：不可用" : `${metrics.tpsOutputTokens.toLocaleString("zh-CN")} ÷ ${metrics.tpsDurationSeconds.toLocaleString("zh-CN", {maximumFractionDigits: 1})} = ${tpsValue}`
  const tpsDetail = `${metrics.windowHours}h TPS\n模型：${metrics.tpsModels.length ? metrics.tpsModels.join("、") : "无可计量样本"}\n${windowText}\n使用所选时段内符合现有条件的请求模型输出 Token 与请求耗时。图片补计量不参与统计。\nTPS = 输出 Token 总数 ÷ 总耗时（秒）\n${tpsCalculation}\n评分与排序\n评分 = min(100, TPS × 2)，当前评分：${tpsScore}\n0 TPS 为 0 分，25 TPS 为 50 分，50 TPS 及以上为 100 分。仅独立排序，不改变综合健康权重。\n没有符合条件的样本时显示不可用。`
  return <div className="flex flex-wrap items-center gap-2" aria-busy={loading}>
    <Ring rate={metrics.cacheHitRate} label={`${metrics.windowHours}h 缓存命中`} color="#38bdf8" detail={`${windowText}\n缓存命中率 = 缓存读取 Token ÷ (输入 Token + 缓存读取 Token + 缓存创建 Token)。\n${metrics.cacheReadTokens.toLocaleString("zh-CN")} ÷ (${metrics.cacheInputTokens.toLocaleString("zh-CN")} + ${metrics.cacheReadTokens.toLocaleString("zh-CN")} + ${metrics.cacheCreationTokens.toLocaleString("zh-CN")}) × 100% = ${metrics.cacheHitRate == null ? "未知" : `${(metrics.cacheHitRate * 100).toFixed(1)}%`}。\n该指标按 Token 数量加权，不按请求次数加权。\n未报告缓存字段的普通输入会计入分母；没有有效 Token 时显示未知。`}/>
    <Ring rate={metrics.successRate} label={`${metrics.windowHours}h 重试感知成功率`} color="#34d399" detail={`${windowText}\n计算方法：成功次数 ÷ 实际尝试次数。\n成功 ${metrics.successCount} / ${metrics.successAttemptCount} 次。\n一次失败后重试成功，失败和成功分别计入。\n辅助调用、探测和已确认的 cyber 请求不计入成功或失败，也不计入分母。\n进行中的文字尝试保留在分母内；图片等待完成计入。无样本显示未知。`}/>
    <TpsRing tps={metrics.tps} score={metrics.tpsScore} label={`${metrics.windowHours}h TPS`} detail={tpsDetail}/>
    {showSampleCount ? <span className="text-[10px] text-muted-foreground">{loading && !metrics.sampleCount ? "读取中…" : `${metrics.windowHours}h · ${metrics.sampleCount}/${metrics.sampleLimit} 次`}</span> : null}
  </div>
}
