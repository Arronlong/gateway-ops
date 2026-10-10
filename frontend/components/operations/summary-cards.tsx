import { BookOpenCheck, CircleDollarSign, Gauge, ReceiptText, WalletCards } from "lucide-react"
import { Card } from "@/components/ui/card"
import type { OperationSummary } from "@/lib/api-types"
import { cn } from "@/lib/utils"

export function cny(value: number | null | undefined) {
  if (value == null || !Number.isFinite(value)) return "—"
  return `¥${value.toLocaleString("zh-CN", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
}

export function usd(value: number | null | undefined) {
  if (value == null || !Number.isFinite(value)) return "—"
  return `$${value.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
}

export function multiplier(value: number | null | undefined) {
  if (value == null || !Number.isFinite(value)) return "—"
  return `${value.toLocaleString("zh-CN", { minimumFractionDigits: 2, maximumFractionDigits: 2 })} 倍`
}

export function OperationSummaryCards({ data, loading }: { data: OperationSummary | null; loading: boolean }) {
  const multiplierFooter = "实时倍率 = 实际消耗成本 ÷ 中转站用户消耗"

  const items = [
    { label: "总收入", value: cny(data?.ledger.income_amount), footer: "所选范围内经营收入", tone: "text-success", background: "bg-success/10", icon: ReceiptText },
    { label: "支出", value: cny(data?.ledger.expense_amount), footer: "支出 = 账号采购 + 上游充值", tone: "text-danger", background: "bg-danger/10", icon: CircleDollarSign },
    { label: "现金净额", value: cny(data?.ledger.net_amount), footer: "总收入减支出", tone: "text-success", background: "bg-success/10", icon: WalletCards },
    { label: "账号采购成本", value: cny(data?.cost.relay_account_purchase_amount), footer: "采购用于中转站服务的账号所支付的金额", tone: "text-warning", background: "bg-warning/10", icon: BookOpenCheck },
    { label: "上游充值支出", value: cny(data?.cost.relay_upstream_recharge_amount), footer: "向上游渠道充值、购买调用额度所支付的金额", tone: "text-warning", background: "bg-warning/10", icon: ReceiptText },
    { label: "当前渠道余额", value: cny(data?.cost.channel_balance_amount), footer: "尚未被消耗的渠道预付资产", tone: "text-brand", background: "bg-brand/10", icon: WalletCards },
    { label: "实际消耗成本", value: cny(data?.cost.relay_actual_cost_amount), footer: "实际消耗成本 = 支出 - 当前渠道余额", tone: "text-danger", background: "bg-danger/10", icon: CircleDollarSign },
    { label: "中转站用户消耗", value: usd(data?.cost.relay_revenue_amount), footer: "用户通过中转站调用模型服务产生的累计计费金额", tone: "text-success", background: "bg-success/10", icon: ReceiptText },
    { label: "实时倍率", value: multiplier(data?.cost.realtime_multiplier), footer: multiplierFooter, tone: "text-brand", background: "bg-brand/10", icon: Gauge },
  ]

  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
      {items.map(({ label, value, footer, tone, background, icon: Icon }) => (
        <Card key={label} className="flex flex-row items-start justify-between gap-0 border border-border p-4 shadow-none">
          <div className="flex min-w-0 flex-col">
            <p className="text-xs text-muted-foreground">{label}</p>
            <p className={cn("mt-1 text-2xl font-bold tracking-tight tabular-nums", tone)}>{loading && !data ? "加载中…" : value}</p>
            <p className="mt-1 text-xs text-muted-foreground">{footer}</p>
          </div>
          <span className={cn("flex size-10 shrink-0 items-center justify-center rounded-xl", background)}>
            <Icon className={cn("size-5", tone)} />
          </span>
        </Card>
      ))}
    </div>
  )
}
