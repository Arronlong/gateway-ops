import { TaskHistoryButton } from "@/components/system/task-history"
import { useEffect, useMemo, useState } from "react"
import { Bird, Plus, Save, Trash2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { Switch } from "@/components/ui/switch"
import { Checkbox } from "@/components/ui/checkbox"
import type { SystemSettings } from "@/lib/api-types"
import { addPelicanPlan, defaultPelicanPrompt, normalizePelicanPlans, pelicanEffortOptions, pelicanPlanValidationIssues, pelicanSchedules, removePelicanPlan, updatePelicanPlan } from "@/lib/pelican"
import { defaultSystemSettings, pelicanTimezoneOptions, pelicanWeekdayOptions } from "@/lib/system-settings"

export function PelicanSettingsCard({ settings, saving, onChange, onSave }: {
  settings: SystemSettings
  saving: boolean
  onChange: (update: (current: SystemSettings) => SystemSettings) => void
  onSave: () => void
}) {
  const [custom, setCustom] = useState(false)
  const customSchedule = custom || !pelicanSchedules.some(item => item.value === settings.pelican_cron_expression)
  const patch = (value: Partial<SystemSettings>) => onChange(current => ({ ...current, ...value }))
  const plans = useMemo(() => normalizePelicanPlans(settings), [settings])
  const issues = useMemo(() => pelicanPlanValidationIssues(plans), [plans])
  const issueFor = (index: number, field: "model" | "prompt" | "reasoning_effort") => issues.find(issue => issue.index === index && issue.field === field)
  const countIssue = issues.find(issue => issue.index === null)
  const patchPlans = (next: typeof plans) => patch({ pelican_plans: next })
  useEffect(() => {
    if (!Array.isArray(settings.pelican_plans) || JSON.stringify(settings.pelican_plans) !== JSON.stringify(plans)) {
      onChange(current => ({ ...current, pelican_plans: plans }))
    }
  }, [onChange, plans, settings])
  const validTime = /^([01]\d|2[0-3]):[0-5]\d$/
  const invalidWindow = !validTime.test(settings.pelican_active_start) || !validTime.test(settings.pelican_active_end)
  const activeWeekdays = Array.isArray(settings.pelican_active_weekdays) ? settings.pelican_active_weekdays : defaultSystemSettings.pelican_active_weekdays
  const invalidWeekdays = activeWeekdays.length === 0 || new Set(activeWeekdays).size !== activeWeekdays.length || activeWeekdays.some(day => !Number.isInteger(day) || day < 0 || day > 6)
  const invalidTimezone = !pelicanTimezoneOptions.some(option => option.value === settings.pelican_active_timezone)
  const invalidHistoryCount = !Number.isInteger(settings.pelican_history_count) || settings.pelican_history_count < 1 || settings.pelican_history_count > 100
  const invalid = invalidWindow || invalidWeekdays || invalidTimezone || invalidHistoryCount || issues.length > 0

  return <Card id="pelican-settings" className="scroll-mt-4 gap-0 border border-border py-0 shadow-none">
    <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-3 border-b border-border px-4 py-3">
      <div className="min-w-0">
        <CardTitle className="flex items-center gap-2 text-sm font-semibold"><Bird className="size-4 text-brand" />{"\u9e48\u9e55\u6d4b\u667a\u65b9\u6848"}</CardTitle>
        <p className="mt-1 text-xs leading-5 text-muted-foreground">{"\u6bcf\u8f6e\u6240\u6709\u65b9\u6848\u5e76\u884c\u8fd0\u884c\u4e00\u6b21\uff1b\u5b9a\u65f6\u65f6\u6bb5\u4e0e\u9891\u7387\u7edf\u4e00\u5e94\u7528\u4e8e\u6240\u6709\u5173\u8054\u8d26\u53f7\u3002"}</p>
      </div>
      <div className="ml-auto flex shrink-0 flex-wrap items-center justify-end gap-2">
        <TaskHistoryButton task="pelican" title={"\u9e48\u9e55\u5b9a\u65f6\u8c03\u5ea6\u68c0\u67e5"} />
        <Button size="sm" className="h-9 shrink-0 gap-1.5" disabled={saving || invalid} onClick={onSave}><Save className="size-3.5" />{"\u4fdd\u5b58\u6d4b\u667a\u8bbe\u7f6e"}</Button>
      </div>
    </CardHeader>
    <CardContent className="grid gap-4 px-4 py-4 sm:grid-cols-2">
      <div className="flex items-center justify-between gap-3 rounded-lg border border-border p-3">
        <div className="space-y-1"><Label htmlFor="pelican-page-enabled">页面展示</Label><p className="text-xs leading-5 text-muted-foreground">显示测智页面、导航入口和渠道作品区。</p></div>
        <Switch id="pelican-page-enabled" checked={settings.pelican_page_enabled} onCheckedChange={value => patch({ pelican_page_enabled: value })} />
      </div>
      <div className="flex items-center justify-between gap-3 rounded-lg border border-border p-3">
        <div className="space-y-1"><Label htmlFor="pelican-tasks-enabled">任务执行</Label><p className="text-xs leading-5 text-muted-foreground">控制定时和手动测试，已开始的测试会继续完成。</p></div>
        <Switch id="pelican-tasks-enabled" checked={settings.pelican_tasks_enabled} onCheckedChange={value => patch({ pelican_tasks_enabled: value })} />
      </div>
      <div className="min-w-0 space-y-3 sm:col-span-2">
        {plans.map((plan, index) => {
          const modelIssue = issueFor(index, "model")
          const promptIssue = issueFor(index, "prompt")
          const effortIssue = issueFor(index, "reasoning_effort")
          const modelID = `pelican-plan-${index}-model`
          const promptID = `pelican-plan-${index}-prompt`
          const effortID = `pelican-plan-${index}-effort`
          return <section key={index} className="min-w-0 space-y-3 rounded-md border border-border p-3 sm:p-4">
            <div className="flex min-w-0 flex-wrap items-center justify-between gap-2">
              <h3 className="text-sm font-medium">{"\u65b9\u6848"} {index + 1}</h3>
              <Button type="button" size="sm" variant="ghost" className="h-8 gap-1.5 text-xs" disabled={plans.length <= 1} onClick={() => patchPlans(removePelicanPlan(plans, index))}><Trash2 className="size-3.5" />{"\u5220\u9664\u65b9\u6848"}</Button>
            </div>
            <div className="grid min-w-0 gap-3 sm:grid-cols-2">
              <div className="min-w-0 space-y-2 sm:col-span-2">
                <Label htmlFor={modelID}>{"\u6a21\u578b\u4f18\u5148\u5217\u8868"}</Label>
                <Textarea id={modelID} className="w-full max-w-full" maxLength={256} rows={3} value={plan.model} aria-invalid={Boolean(modelIssue)} aria-describedby={modelIssue ? `${modelID}-error` : undefined} onChange={event => patchPlans(updatePelicanPlan(plans, index, { model: event.target.value }))} placeholder={"\u6bcf\u884c\u6216\u7528\u9017\u53f7\u5206\u9694\uff0c\u6309\u4f18\u5148\u987a\u5e8f\u586b\u5199"} />
                <p className="text-xs leading-5 text-muted-foreground">{"\u652f\u6301\u6362\u884c\u3001\u9017\u53f7\u6216\u5206\u53f7\u5206\u9694\uff0c\u6309\u987a\u5e8f\u4f7f\u7528\u9996\u4e2a\u53ef\u7528\u6a21\u578b\u3002"}</p>
                {modelIssue ? <p id={`${modelID}-error`} role="alert" className="text-xs text-danger">{modelIssue.message}</p> : null}
              </div>
              <div className="min-w-0 space-y-2 sm:col-span-2">
                <div className="flex min-w-0 flex-wrap items-center justify-between gap-2"><Label htmlFor={promptID}>{"\u9898\u76ee"}</Label><Button type="button" variant="ghost" size="sm" className="h-7 text-xs" onClick={() => patchPlans(updatePelicanPlan(plans, index, { prompt: defaultPelicanPrompt }))}>{"\u6062\u590d\u9ed8\u8ba4\u9898\u76ee"}</Button></div>
                <Textarea id={promptID} className="w-full max-w-full" value={plan.prompt} rows={4} maxLength={32000} aria-invalid={Boolean(promptIssue)} aria-describedby={promptIssue ? `${promptID}-error` : undefined} onChange={event => patchPlans(updatePelicanPlan(plans, index, { prompt: event.target.value }))} />
                {promptIssue ? <p id={`${promptID}-error`} role="alert" className="text-xs text-danger">{promptIssue.message}</p> : null}
              </div>
              <div className="min-w-0 space-y-2 sm:col-span-2">
                <Label htmlFor={effortID}>{"\u601d\u8003\u5f3a\u5ea6"}</Label>
                <Select value={plan.reasoning_effort} onValueChange={value => patchPlans(updatePelicanPlan(plans, index, { reasoning_effort: value }))}><SelectTrigger id={effortID} className="w-full" aria-invalid={Boolean(effortIssue)}><SelectValue /></SelectTrigger><SelectContent>{Object.entries(pelicanEffortOptions).map(([value, label]) => <SelectItem key={value} value={value}>{label}</SelectItem>)}</SelectContent></Select>
                {effortIssue ? <p role="alert" className="text-xs text-danger">{effortIssue.message}</p> : null}
              </div>
            </div>
          </section>
        })}
        {countIssue ? <p role="alert" className="text-xs text-danger">{countIssue.message}</p> : null}
        <div className="flex flex-wrap items-center gap-2">
          <Button type="button" size="sm" variant="outline" className="gap-1.5" disabled={plans.length >= 8} onClick={() => patchPlans(addPelicanPlan(plans))}><Plus className="size-3.5" />{"\u65b0\u589e\u65b9\u6848"}</Button>
          <span className="text-xs text-muted-foreground">{"\u6700\u591a 8 \u9879\uff0c\u6bcf\u9879\u6bcf\u8f6e\u6267\u884c\u4e00\u6b21"}</span>
        </div>
      </div>
      <div className="space-y-2 sm:col-span-2">
        <fieldset className="space-y-2">
          <legend className="mb-2 text-sm font-medium">自动测试允许时段（{settings.pelican_active_timezone}）</legend>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <div className="space-y-2"><Label htmlFor="pelican-active-start">{"\u5f00\u59cb\u65f6\u95f4"}</Label><Input id="pelican-active-start" type="time" step={60} required aria-describedby="pelican-window-help" value={settings.pelican_active_start} onChange={event => patch({ pelican_active_start: event.target.value })} /></div>
            <div className="space-y-2"><Label htmlFor="pelican-active-end">{"\u7ed3\u675f\u65f6\u95f4"}</Label><Input id="pelican-active-end" type="time" step={60} required aria-describedby="pelican-window-help" value={settings.pelican_active_end} onChange={event => patch({ pelican_active_end: event.target.value })} /></div>
            <div className="space-y-2"><Label htmlFor="pelican-active-timezone">时区</Label><Select value={settings.pelican_active_timezone} onValueChange={value => patch({ pelican_active_timezone: value })}><SelectTrigger id="pelican-active-timezone" className="w-full"><SelectValue /></SelectTrigger><SelectContent>{pelicanTimezoneOptions.map(option => <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>)}</SelectContent></Select></div>
          </div>
          <div className="space-y-2">
            <Label>运行星期</Label>
            <div role="group" aria-label="运行星期" className="grid grid-cols-4 gap-2 sm:grid-cols-7">
              {pelicanWeekdayOptions.map(day => <label key={day.value} className="flex cursor-pointer items-center gap-2 rounded-md border border-border px-2 py-2 text-sm"><Checkbox checked={activeWeekdays.includes(day.value)} onCheckedChange={checked => {
                const weekdays = checked === true ? [...new Set([...activeWeekdays, day.value])] : activeWeekdays.filter(value => value !== day.value)
                patch({ pelican_active_weekdays: weekdays.sort((left, right) => left - right) })
              }} /><span>{day.label}</span></label>)}
            </div>
            {invalidWeekdays ? <p role="alert" className="text-xs text-danger">每周至少选择一天，且星期不能重复。</p> : null}
          </div>
          <p id="pelican-window-help" className="text-xs leading-5 text-muted-foreground">仅在已选星期和此时间段内按设定频率自动测试；包含开始时间、不包含结束时间。跨午夜的时段按开始日归属，次日凌晨仍沿用开始日设置。起止相同表示全天。手动测试不受限制，已开始的测试会继续完成。</p>
          {invalidWindow ? <p role="alert" className="text-xs text-danger">{"\u8bf7\u586b\u5199\u5b8c\u6574\u7684\u5f00\u59cb\u548c\u7ed3\u675f\u65f6\u95f4\u3002"}</p> : null}
          {invalidTimezone ? <p role="alert" className="text-xs text-danger">请选择有效时区。</p> : null}
        </fieldset>
      </div>
      <div className="space-y-2 sm:col-span-2">
        <Label htmlFor="pelican-history-count">每个中转站账号保留作品数</Label>
        <Input id="pelican-history-count" type="number" min={1} max={100} step={1} value={settings.pelican_history_count} aria-describedby="pelican-history-help" onChange={event => patch({ pelican_history_count: Number(event.target.value) })} />
        <p id="pelican-history-help" className="text-xs leading-5 text-muted-foreground">设置范围 1–100，默认 30。超过上限会自动删除较早作品；调低上限会立即清理超出部分，已删除作品无法恢复。</p>
        {invalidHistoryCount ? <p role="alert" className="text-xs text-danger">保留数量必须是 1 到 100 之间的整数。</p> : null}
      </div>
      <div className="space-y-2 sm:col-span-2">
        <Label htmlFor="pelican-schedule">{"\u5b9a\u65f6\u6d4b\u667a\u9891\u7387"}</Label>
        <Select value={customSchedule ? "custom" : settings.pelican_cron_expression} onValueChange={value => { setCustom(value === "custom"); if (value !== "custom") patch({ pelican_cron_expression: value }) }}><SelectTrigger id="pelican-schedule" className="w-full"><SelectValue /></SelectTrigger><SelectContent>{pelicanSchedules.map(item => <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>)}<SelectItem value="custom">{"\u81ea\u5b9a\u4e49"}</SelectItem></SelectContent></Select>
        {customSchedule ? <Input aria-label={"\u81ea\u5b9a\u4e49 Cron"} value={settings.pelican_cron_expression} onChange={event => patch({ pelican_cron_expression: event.target.value })} /> : null}
        <p className="text-xs leading-5 text-muted-foreground">按上方所选时区执行；自定义使用五段 Cron（分 时 日 月 周）。修改时段、星期、时区或频率会重算各账号下次执行时间，时段外的任务不会补跑。</p>
      </div>
      <p className="text-xs leading-5 text-muted-foreground sm:col-span-2">{"\u4fdd\u5b58\u5373\u751f\u6548\uff0c\u4e0d\u4f1a\u81ea\u52a8\u5f00\u542f\u4efb\u4f55\u8d26\u53f7\u3002\u6a21\u578b\u8c03\u7528\u4f1a\u4ea7\u751f\u8d39\u7528\uff1b\u5df2\u5f00\u59cb\u7684\u4e00\u8f6e\u4f7f\u7528\u542f\u52a8\u65f6\u7684\u53c2\u6570\u3002"}</p>
    </CardContent>
  </Card>
}
