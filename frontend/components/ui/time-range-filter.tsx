import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { cn } from "@/lib/utils"
import { defaultCustomTimeRange, serializeTimeRange, timeRangeLabel, type CustomTimeRangeSelection, type TimeRangeSelection } from "@/lib/time-range"

export interface TimeRangePresetOption {
  value: string
  label: string
}

export function TimeRangeFilter<T extends string>({ value, options, onChange, label, className, buttonClassName, allowCustom = true }: {
  value: TimeRangeSelection<T>
  options: readonly TimeRangePresetOption[]
  onChange: (value: TimeRangeSelection<T>) => void
  label: string
  className?: string
  buttonClassName?: string
  allowCustom?: boolean
}) {
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState<CustomTimeRangeSelection | null>(null)
  const custom = typeof value !== "string"
  const selected = typeof value === "string" ? null : value
  const current = draft ?? selected ?? defaultCustomTimeRange()
  const invalid = serializeTimeRange(current) === null
  const errorID = `${label}-error`
  const updateCustom = (patch: Partial<CustomTimeRangeSelection>) => {
    const next = { ...current, ...patch }
    setDraft(next)
    // 日期编辑中的空值或无效区间不替换已生效的查询。
    if (serializeTimeRange(next)) onChange(next)
  }

  return <Popover open={open} onOpenChange={(nextOpen) => {
    if (nextOpen) setDraft(selected ?? defaultCustomTimeRange())
    setOpen(nextOpen)
  }}>
    <div className={cn("min-w-0 max-w-full", className)}>
      <div className="flex w-max max-w-full flex-nowrap items-center overflow-x-auto rounded-md border border-border bg-muted/30 p-0.5" role="group" aria-label={label}>
        {options.map((option) => <Button key={option.value} type="button" variant="ghost" aria-pressed={!custom && value === option.value} className={cn("h-8 shrink-0 whitespace-nowrap rounded px-3 text-xs", !custom && value === option.value ? "bg-background font-medium text-foreground shadow-xs" : "text-muted-foreground hover:text-foreground", buttonClassName)} onClick={() => { setOpen(false); setDraft(null); onChange(option.value as T) }}>{option.label}</Button>)}
        {allowCustom ? <PopoverTrigger asChild>
          <Button type="button" variant="ghost" aria-pressed={custom} title={selected ? timeRangeLabel(selected, options) : undefined} className={cn("h-8 shrink-0 whitespace-nowrap rounded px-3 text-xs", custom ? "bg-background font-medium text-foreground shadow-xs" : "text-muted-foreground hover:text-foreground", buttonClassName)}>自定义</Button>
        </PopoverTrigger> : null}
      </div>
    </div>
    {allowCustom ? <PopoverContent align="end" collisionPadding={12} aria-label={`${label}自定义区间`} className="w-[min(30rem,calc(100vw-1.5rem))] p-3">
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div className="grid min-w-0 gap-1">
          <Label htmlFor={`${label}-start`} className="text-[11px] text-muted-foreground">开始时间</Label>
          <Input id={`${label}-start`} type="datetime-local" step={60} value={current.start} aria-invalid={invalid} aria-describedby={invalid ? errorID : undefined} className="h-9 w-full min-w-0 text-xs" onChange={(event) => updateCustom({ start: event.target.value })}/>
        </div>
        <div className="grid min-w-0 gap-1">
          <Label htmlFor={`${label}-end`} className="text-[11px] text-muted-foreground">结束时间</Label>
          <Input id={`${label}-end`} type="datetime-local" step={60} min={current.start || undefined} value={current.end} aria-invalid={invalid} aria-describedby={invalid ? errorID : undefined} className="h-9 w-full min-w-0 text-xs" onChange={(event) => updateCustom({ end: event.target.value })}/>
        </div>
      </div>
      {invalid ? <p id={errorID} role="alert" className="mt-2 text-xs text-danger">请填写有效的开始、结束时间，结束时间不能早于开始时间。</p> : null}
    </PopoverContent> : null}
  </Popover>
}
