"use client"

import { CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip as RechartsTooltip, XAxis, YAxis } from "recharts"
import { Activity, CircleDollarSign, Coins } from "lucide-react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import type { RelayDailyConsumptionPoint } from "@/lib/api-types"
import { money, normalizeTokenCount, tokensInYi } from "@/lib/format"

function formatDay(value: string) {
  const date = new Date(value + "T00:00:00")
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleDateString("zh-CN", { month: "2-digit", day: "2-digit" })
}

type TooltipPayload = {
  payload?: RelayDailyConsumptionPoint
}

function DailyTooltip({ active, payload }: { active?: boolean; payload?: TooltipPayload[] }) {
  const point = payload?.[0]?.payload
  if (!active || !point) return null
  const totalTokens = normalizeTokenCount(point.total_tokens)
  return (
    <div className="min-w-52 rounded-lg border border-border bg-popover px-3 py-2 text-xs shadow-lg">
      <p className="font-medium text-foreground">{point.date}</p>
      <div className="mt-2 grid grid-cols-[1fr_auto] gap-x-5 gap-y-1.5">
        <span className="text-muted-foreground">用户消费</span>
        <span className="font-mono font-semibold tabular-nums text-success">{money(point.user_charge, { precise: point.user_charge < 1 })}</span>
        <span className="text-muted-foreground">账号成本</span>
        <span className="font-mono font-semibold tabular-nums text-warning">{money(point.account_cost, { precise: point.account_cost < 1 })}</span>
        <span className="text-muted-foreground">请求数</span>
        <span className="font-mono font-semibold tabular-nums text-foreground">{point.request_count.toLocaleString("zh-CN")}</span>
        <span className="text-muted-foreground">总 Token</span>
        <span className="font-mono font-semibold tabular-nums text-brand" title={`${totalTokens.toLocaleString("zh-CN")} Token`}>{tokensInYi(totalTokens)}</span>
      </div>
    </div>
  )
}

export function RelayDailyConsumptionTrend({
  data,
  loading,
}: {
  data: RelayDailyConsumptionPoint[]
  loading?: boolean
}) {
  const totalCharge = data.reduce((sum, point) => sum + point.user_charge, 0)
  const totalCost = data.reduce((sum, point) => sum + point.account_cost, 0)
  const totalRequests = data.reduce((sum, point) => sum + point.request_count, 0)
  const totalTokens = data.reduce((sum, point) => sum + normalizeTokenCount(point.total_tokens), 0)

  return (
    <Card className="gap-0 border border-border py-0 shadow-none">
      <CardHeader className="flex flex-row items-center justify-between gap-3 border-b border-border px-4 py-3">
        <div className="min-w-0">
          <CardTitle className="flex items-center gap-2 text-sm font-semibold">
            <CircleDollarSign className="size-4 text-brand" />
            每日消费趋势
          </CardTitle>
          <p className="mt-1 text-xs text-muted-foreground">按天汇总当前中转站的用户消费；悬停查看账号成本、请求数与总 Token。</p>
        </div>
      </CardHeader>
      <CardContent className="p-4">
        <div className="mb-3 flex flex-wrap gap-x-6 gap-y-2 text-xs">
          <span className="inline-flex items-center gap-1.5 text-muted-foreground"><CircleDollarSign className="size-3.5 text-success" />消费 <strong className="font-mono font-semibold tabular-nums text-success">{money(totalCharge, { precise: totalCharge < 1 })}</strong></span>
          <span className="inline-flex items-center gap-1.5 text-muted-foreground">成本 <strong className="font-mono font-semibold tabular-nums text-warning">{money(totalCost, { precise: totalCost < 1 })}</strong></span>
          <span className="inline-flex items-center gap-1.5 text-muted-foreground"><Activity className="size-3.5 text-brand" />请求 <strong className="font-mono font-semibold tabular-nums text-foreground">{totalRequests.toLocaleString("zh-CN")}</strong></span>
          <span className="inline-flex items-center gap-1.5 text-muted-foreground" title={`${totalTokens.toLocaleString("zh-CN")} Token`}><Coins className="size-3.5 text-brand" />总 Token <strong className="font-mono font-semibold tabular-nums text-brand">{tokensInYi(totalTokens)}</strong></span>
        </div>
        {loading && data.length === 0 ? (
          <div className="flex h-44 items-center justify-center text-xs text-muted-foreground">每日消费趋势读取中…</div>
        ) : data.length === 0 ? (
          <div className="flex h-36 items-center justify-center rounded-md border border-dashed border-border text-xs text-muted-foreground">暂无消费数据</div>
        ) : (
          <div className="h-52 w-full">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={data} margin={{ top: 6, right: 8, bottom: 0, left: 0 }}>
                <CartesianGrid vertical={false} stroke="var(--border)" strokeDasharray="3 3" />
                <XAxis
                  dataKey="date"
                  axisLine={false}
                  tickLine={false}
                  minTickGap={36}
                  tick={{ fill: "var(--muted-foreground)", fontSize: 10 }}
                  tickFormatter={formatDay}
                />
                <YAxis
                  axisLine={false}
                  tickLine={false}
                  width={46}
                  tick={{ fill: "var(--muted-foreground)", fontSize: 10 }}
                  tickFormatter={(value: number) => money(value)}
                />
                <RechartsTooltip content={<DailyTooltip />} cursor={{ stroke: "var(--border)", strokeDasharray: "4 4" }} />
                <Line
                  type="monotone"
                  dataKey="user_charge"
                  stroke="var(--success)"
                  strokeWidth={2}
                  dot={false}
                  activeDot={{ r: 3, fill: "var(--success)", strokeWidth: 0 }}
                  isAnimationActive={false}
                />
              </LineChart>
            </ResponsiveContainer>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
