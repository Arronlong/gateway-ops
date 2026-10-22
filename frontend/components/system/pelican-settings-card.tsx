import { useState } from "react"
import { Bird, Save } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import type { SystemSettings } from "@/lib/api-types"
import { defaultPelicanPrompt, pelicanEfforts, pelicanSchedules } from "@/lib/pelican"

export function PelicanSettingsCard({ settings, saving, onChange, onSave }: {
  settings: SystemSettings
  saving: boolean
  onChange: (update: (current: SystemSettings) => SystemSettings) => void
  onSave: () => void
}) {
  const [custom, setCustom] = useState(false)
  const customSchedule = custom || !pelicanSchedules.some(item => item.value === settings.pelican_cron_expression)
  const patch = (value: Partial<SystemSettings>) => onChange(current => ({ ...current, ...value }))
  const invalid = !Number.isInteger(settings.pelican_parallel_count) || settings.pelican_parallel_count < 1 || settings.pelican_parallel_count > 8 || !settings.pelican_prompt.trim()

  return <Card id="pelican-settings" className="scroll-mt-4 gap-0 border border-border py-0 shadow-none">
    <CardHeader className="flex flex-row items-center justify-between gap-3 border-b border-border px-4 py-3">
      <div className="min-w-0">
        <CardTitle className="flex items-center gap-2 text-sm font-semibold"><Bird className="size-4 text-brand" />{"\u9e48\u9e55\u6d4b\u667a\u5168\u5c40\u914d\u7f6e"}</CardTitle>
        <p className="mt-1 text-xs leading-5 text-muted-foreground">{"\u6240\u6709\u4e2d\u8f6c\u7ad9\u8d26\u53f7\u5171\u7528\u4ee5\u4e0b\u53c2\u6570\u3002\u8d26\u53f7\u9875\u9762\u53ea\u7ba1\u7406\u5b9a\u65f6\u5f00\u5173\u3001\u624b\u52a8\u6d4b\u8bd5\u548c\u7ed3\u679c\u67e5\u770b\u3002"}</p>
      </div>
      <Button size="sm" className="h-9 shrink-0 gap-1.5" disabled={saving || invalid} onClick={onSave}><Save className="size-3.5" />{"\u4fdd\u5b58\u6d4b\u667a\u8bbe\u7f6e"}</Button>
    </CardHeader>
    <CardContent className="grid gap-4 px-4 py-4 sm:grid-cols-2">
      <div className="space-y-2 sm:col-span-2">
        <Label htmlFor="pelican-model">{"\u6a21\u578b"}</Label>
        <Input id="pelican-model" maxLength={256} value={settings.pelican_model} onChange={event => patch({ pelican_model: event.target.value })} placeholder={"\u8f93\u5165\u5f85\u6d4b\u6a21\u578b\u540d\u79f0"} />
        <p className="text-xs text-muted-foreground">{"\u4f7f\u7528\u5f85\u6d4b\u8d26\u53f7\u652f\u6301\u7684\u6a21\u578b\u540d\u79f0\uff1b\u7559\u7a7a\u65f6\u4e0d\u4f1a\u6267\u884c\u6d4b\u667a\u3002"}</p>
      </div>
      <div className="space-y-2 sm:col-span-2">
        <div className="flex items-center justify-between gap-2"><Label htmlFor="pelican-prompt">{"\u9898\u76ee"}</Label><Button type="button" variant="ghost" size="sm" className="h-7 text-xs" onClick={() => patch({ pelican_prompt: defaultPelicanPrompt })}>{"\u6062\u590d\u9ed8\u8ba4\u9898\u76ee"}</Button></div>
        <Textarea id="pelican-prompt" value={settings.pelican_prompt} rows={4} maxLength={32000} onChange={event => patch({ pelican_prompt: event.target.value })} />
        <p className="text-xs leading-5 text-muted-foreground">{"\u53d1\u9001\u65f6\u81ea\u52a8\u8ffd\u52a0\u201c\u53ea\u8f93\u51fa\u72ec\u7acb HTML\u201d\u8981\u6c42\uff0c\u4fdd\u7559\u6a21\u578b\u8fd4\u56de\u7684 HTML/SVG \u52a8\u753b\u3002"}</p>
      </div>
      <div className="space-y-2"><Label htmlFor="pelican-effort">{"\u601d\u8003\u5f3a\u5ea6"}</Label><Select value={settings.pelican_reasoning_effort} onValueChange={value => patch({ pelican_reasoning_effort: value })}><SelectTrigger id="pelican-effort" className="w-full"><SelectValue /></SelectTrigger><SelectContent>{Object.entries(pelicanEfforts).map(([value, label]) => <SelectItem key={value} value={value}>{label}</SelectItem>)}</SelectContent></Select></div>
      <div className="space-y-2"><Label htmlFor="pelican-parallel">{"\u6bcf\u6b21\u5e76\u884c\u4efd\u6570"}</Label><Input id="pelican-parallel" type="number" min={1} max={8} step={1} value={settings.pelican_parallel_count} onChange={event => patch({ pelican_parallel_count: Number(event.target.value) })} /><p className="text-xs text-muted-foreground">{"\u6bcf\u4e2a\u8d26\u53f7\u6bcf\u8f6e 1\u20138 \u4efd\uff0c\u9ed8\u8ba4 1 \u4efd\u3002"}</p></div>
      <div className="space-y-2 sm:col-span-2">
        <Label htmlFor="pelican-schedule">{"\u5b9a\u65f6\u6d4b\u667a\u9891\u7387"}</Label>
        <Select value={customSchedule ? "custom" : settings.pelican_cron_expression} onValueChange={value => { setCustom(value === "custom"); if (value !== "custom") patch({ pelican_cron_expression: value }) }}><SelectTrigger id="pelican-schedule" className="w-full"><SelectValue /></SelectTrigger><SelectContent>{pelicanSchedules.map(item => <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>)}<SelectItem value="custom">{"\u81ea\u5b9a\u4e49"}</SelectItem></SelectContent></Select>
        {customSchedule ? <Input aria-label={"\u81ea\u5b9a\u4e49 Cron"} value={settings.pelican_cron_expression} onChange={event => patch({ pelican_cron_expression: event.target.value })} /> : null}
        <p className="text-xs leading-5 text-muted-foreground">{"\u6309\u5317\u4eac\u65f6\u95f4\uff08Asia/Shanghai\uff09\u6267\u884c\uff1b\u81ea\u5b9a\u4e49\u4f7f\u7528\u4e94\u6bb5 Cron\uff08\u5206 \u65f6 \u65e5 \u6708 \u5468\uff09\u3002\u4fee\u6539\u9891\u7387\u4f1a\u91cd\u7b97\u5404\u8d26\u53f7\u4e0b\u6b21\u6267\u884c\u65f6\u95f4\u3002"}</p>
      </div>
      <p className="text-xs leading-5 text-muted-foreground sm:col-span-2">{"\u4fdd\u5b58\u5373\u751f\u6548\uff0c\u4e0d\u4f1a\u81ea\u52a8\u5f00\u542f\u4efb\u4f55\u8d26\u53f7\u3002\u6a21\u578b\u8c03\u7528\u4f1a\u4ea7\u751f\u8d39\u7528\uff1b\u6bcf\u4e2a\u8d26\u53f7\u4fdd\u7559\u6700\u8fd1 30 \u4efd\u7ed3\u679c\uff0c\u5df2\u5f00\u59cb\u7684\u4e00\u8f6e\u4f7f\u7528\u542f\u52a8\u65f6\u7684\u53c2\u6570\u3002"}</p>
    </CardContent>
  </Card>
}
