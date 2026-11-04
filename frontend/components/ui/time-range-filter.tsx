import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { cn } from "@/lib/utils"
import { defaultCustomTimeRange, serializeTimeRange, type CustomTimeRangeSelection, type TimeRangeSelection } from "@/lib/time-range"

export interface TimeRangePresetOption {
  value: string
  label: string
}

export function TimeRangeFilter<T extends string>({ value, options, onChange, label, className, buttonClassName }: {
  value: TimeRangeSelection<T>
  options: readonly TimeRangePresetOption[]
  onChange: (value: TimeRangeSelection<T>) => void
  label: string
  className?: string
  buttonClassName?: string
}) {
  const custom = typeof value !== "string"
  const selected = typeof value === "string" ? null : value
  const invalid = selected !== null && serializeTimeRange(selected) === null
  const updateCustom = (patch: Partial<CustomTimeRangeSelection>) => {
    const current = selected ?? defaultCustomTimeRange()
    onChange({ ...current, ...patch })
  }

  return <div className={cn("flex min-w-0 flex-wrap items-end gap-2", className)}>
    <div className="inline-flex max-w-full flex-wrap rounded-md border border-border bg-muted/30 p-0.5" role="group" aria-label={label}>
      {options.map((option) => <Button key={option.value} type="button" variant="ghost" aria-pressed={!custom && value === option.value} className={cn("h-8 rounded px-3 text-xs", !custom && value === option.value ? "bg-background font-medium text-foreground shadow-xs" : "text-muted-foreground hover:text-foreground", buttonClassName)} onClick={() => onChange(option.value as T)}>{option.label}</Button>)}
      <Button type="button" variant="ghost" aria-pressed={custom} className={cn("h-8 rounded px-3 text-xs", custom ? "bg-background font-medium text-foreground shadow-xs" : "text-muted-foreground hover:text-foreground", buttonClassName)} onClick={() => { if (!custom) onChange(defaultCustomTimeRange()) }}>自定义</Button>
    </div>
    {selected ? <div className="flex min-w-0 flex-wrap items-end gap-2" aria-label={`${label}自定义区间`}>
      <div className="grid min-w-40 gap-1"><Label htmlFor={`${label}-start`} className="text-[11px] text-muted-foreground">开始时间</Label><Input id={`${label}-start`} type="datetime-local" step={60} value={selected.start} aria-invalid={invalid} className="h-9 min-w-0 text-xs" onChange={(event) => updateCustom({ start: event.target.value })}/></div>
      <div className="grid min-w-40 gap-1"><Label htmlFor={`${label}-end`} className="text-[11px] text-muted-foreground">结束时间</Label><Input id={`${label}-end`} type="datetime-local" step={60} min={selected.start || undefined} value={selected.end} aria-invalid={invalid} className="h-9 min-w-0 text-xs" onChange={(event) => updateCustom({ end: event.target.value })}/></div>
      {invalid ? <p role="alert" className="basis-full text-xs text-danger">请填写有效的开始、结束时间，结束时间不能早于开始时间。</p> : null}
    </div> : null}
  </div>
}
