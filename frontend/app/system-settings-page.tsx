import { useEffect, useState } from "react"
import { Clock3, Eye, LayoutList, Save, Settings2, ShieldCheck, WalletCards } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Switch } from "@/components/ui/switch"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { Input } from "@/components/ui/input"
import { apiFetch } from "@/lib/api"
import type { SyncSettings, SystemSettings } from "@/lib/api-types"
import { useSyncSettings, useSystemSettings } from "@/lib/queries"
import { ConfigurationLoading } from "@/components/ui/configuration-loading"
import { VersionCard } from "@/components/system/version-card"
import { useTriggerRefresh } from "@/lib/refresh-context"
import { cn } from "@/lib/utils"

const channelSyncIntervals = [5, 10, 15, 30, 60, 180, 360, 720, 1440]
const relayRateSyncIntervals = [5, 10, 20, 30, 60, 300, 600, 1800, 3600, 18000, 43200, 86400]
const relaySnapshotSyncIntervals = [5, 10, 30, 60, 120, 180, 300, 600, 900, 1800, 3600, 10800]

function formatMinutes(value: number) {
  if (value < 60) return `每 ${value} 分钟`
  if (value === 1440) return "每天"
  if (value % 60 === 0) return `每 ${value / 60} 小时`
  return `每 ${value} 分钟`
}

function formatSeconds(value: number) {
  if (value < 60) return `每 ${value} 秒`
  if (value === 86400) return "每天"
  if (value % 3600 === 0) return `每 ${value / 3600} 小时`
  return `每 ${value / 60} 分钟`
}

function SyncTaskRow({
  title,
  description,
  checked,
  value,
  options,
  formatOption,
  disabled = false,
  onCheckedChange,
  onValueChange,
}: {
  title: string
  description: string
  checked: boolean
  value: number
  options: readonly number[]
  formatOption: (value: number) => string
  disabled?: boolean
  onCheckedChange: (checked: boolean) => void
  onValueChange: (value: number) => void
}) {
  return (
    <div className={cn("grid min-h-16 gap-3 py-3 md:grid-cols-[minmax(0,1fr)_auto_auto] md:items-center", disabled && "opacity-50")}>
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <p className="text-sm font-medium text-foreground">{title}</p>
          <span className={cn("rounded px-1.5 py-0.5 text-[10px] font-medium", checked ? "bg-success/10 text-success" : "bg-muted text-muted-foreground")}>{checked ? "运行中" : "已关闭"}</span>
        </div>
        <p className="mt-1 text-xs leading-5 text-muted-foreground">{description}</p>
      </div>
      <Select value={String(value)} disabled={disabled || !checked} onValueChange={(nextValue) => onValueChange(Number(nextValue))}>
        <SelectTrigger className="h-9 w-32 text-xs" aria-label={`${title}执行频率`}><SelectValue /></SelectTrigger>
        <SelectContent>{options.map((option) => <SelectItem key={option} value={String(option)}>{formatOption(option)}</SelectItem>)}</SelectContent>
      </Select>
      <Switch checked={checked} disabled={disabled} onCheckedChange={onCheckedChange} className="shrink-0 justify-self-end" aria-label={`启用${title}`} />
    </div>
  )
}

function LevelBadge({ children }: { children: string }) {
  return <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground">{children}</span>
}

function OptionRow({
  title,
  description,
  value,
  disabled = false,
  onValueChange,
}: {
  title: string
  description: string
  value: string
  disabled?: boolean
  onValueChange: (value: string) => void
}) {
  return (
    <div className={cn("flex min-h-16 items-center justify-between gap-5 py-3", disabled && "opacity-50")}>
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <p className="text-sm font-medium text-foreground">{title}</p>
          <span className="rounded bg-brand/10 px-1.5 py-0.5 text-[10px] font-medium text-brand">选项控制</span>
        </div>
        <p className="mt-1 text-xs leading-5 text-muted-foreground">{description}</p>
      </div>
      <Select value={value} disabled={disabled} onValueChange={onValueChange}>
        <SelectTrigger className="h-9 w-28 shrink-0 text-xs"><SelectValue /></SelectTrigger>
        <SelectContent>
          <SelectItem value="collapsed">默认折叠</SelectItem>
          <SelectItem value="expanded">默认展开</SelectItem>
        </SelectContent>
      </Select>
    </div>
  )
}

function SettingRow({
  level,
  title,
  description,
  checked,
  disabled = false,
  onCheckedChange,
}: {
  level: "系统" | "目录" | "页面" | "页面内容" | "内容信息"
  title: string
  description: string
  checked: boolean
  disabled?: boolean
  onCheckedChange: (checked: boolean) => void
}) {
  return (
    <div className={cn("flex min-h-16 items-center justify-between gap-5 py-3", disabled && "opacity-50")}>
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <p className="text-sm font-medium text-foreground">{title}</p>
          <LevelBadge>{level}</LevelBadge>
        </div>
        <p className="mt-1 text-xs leading-5 text-muted-foreground">{description}</p>
      </div>
      <Switch checked={checked} disabled={disabled} onCheckedChange={onCheckedChange} className="shrink-0" />
    </div>
  )
}

export default function SystemSettingsPage() {
  const query = useSystemSettings()
  const syncQuery = useSyncSettings()
  const refreshAll = useTriggerRefresh()
  const [settingsDraft, setSettingsDraft] = useState<SystemSettings | null>(null)
  const [syncSettingsDraft, setSyncSettingsDraft] = useState<SyncSettings | null>(null)
  const [iframeAllowlistDraft, setIframeAllowlistDraft] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [savingSync, setSavingSync] = useState(false)

  useEffect(() => {
    const data = query.data
    if (!data) return
    setSettingsDraft((current) => current ?? data)
    setIframeAllowlistDraft((current) => current ?? data.iframe_embedding_allowlist.join("\n"))
  }, [query.data])

  useEffect(() => {
    if (!syncQuery.data) return
    setSyncSettingsDraft((current) => current ?? syncQuery.data)
  }, [syncQuery.data])

  const settings = settingsDraft ?? query.data
  const syncSettings = syncSettingsDraft ?? syncQuery.data
  const iframeAllowlistText = iframeAllowlistDraft ?? query.data?.iframe_embedding_allowlist.join("\n") ?? ""

  function updateSettings(update: (current: SystemSettings) => SystemSettings) {
    setSettingsDraft((current) => {
      const source = current ?? query.data
      return source ? update(source) : null
    })
  }

  function updateSyncSettings(update: (current: SyncSettings) => SyncSettings) {
    setSyncSettingsDraft((current) => {
      const source = current ?? syncQuery.data
      return source ? update(source) : null
    })
  }

  async function save() {
    if (!settings) return
    setSaving(true)
    try {
      const payload: SystemSettings = {
        ...settings,
        iframe_embedding_allowlist: iframeAllowlistText
          .split(/\r?\n/)
          .map((item) => item.trim())
          .filter(Boolean),
      }
      const saved = await apiFetch<SystemSettings>("/system-settings", {
        method: "PUT",
        body: JSON.stringify(payload),
      })
      setSettingsDraft(saved)
      setIframeAllowlistDraft(saved.iframe_embedding_allowlist.join("\n"))
      await query.refetch()
      refreshAll()
      toast.success("系统配置已保存")
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "保存系统配置失败")
    } finally {
      setSaving(false)
    }
  }

  async function saveSyncSettings() {
    if (!syncSettings) return
    setSavingSync(true)
    try {
      const saved = await apiFetch<SyncSettings>("/sync-settings", {
        method: "PUT",
        body: JSON.stringify({
          channel_enabled: syncSettings.channel_enabled,
          channel_interval_minutes: syncSettings.channel_interval_minutes,
          relay_rate_enabled: syncSettings.relay_rate_enabled,
          relay_rate_interval_minutes: Math.ceil(syncSettings.relay_rate_interval_seconds / 60),
          relay_rate_interval_seconds: syncSettings.relay_rate_interval_seconds,
          relay_snapshot_enabled: syncSettings.relay_snapshot_enabled,
          relay_snapshot_interval_minutes: Math.ceil(syncSettings.relay_snapshot_interval_seconds / 60),
          relay_snapshot_interval_seconds: syncSettings.relay_snapshot_interval_seconds,
        }),
      })
      setSyncSettingsDraft(saved)
      await syncQuery.refetch()
      refreshAll()
      toast.success("后台任务与同步设置已保存")
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "保存同步设置失败")
    } finally {
      setSavingSync(false)
    }
  }

  if (!query.data || !settings) {
    return <ConfigurationLoading query={query} title={"系统设置"} />
  }

  const channelPageDisabled = !settings.channels_page_enabled
  const channelInfoDisabled = channelPageDisabled || !settings.channels_show_channel_info
  const iframeEmbeddingDisabled = !settings.iframe_embedding_enabled

  return (
    <section className="space-y-4">
      <header className="list-panel-controls flex flex-wrap items-end justify-between gap-3 border-l-2 border-brand pl-3">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-bold text-foreground">
            <Settings2 className="size-5 text-brand" />
            系统设置
          </h1>
          <p className="mt-1 text-xs text-muted-foreground">
            统一管理全局后台任务、界面显示和安全策略；站点或账号自身的策略仍在对应业务页面配置。
          </p>
        </div>
      </header>

      <VersionCard />

      <Card id="background-tasks" className="scroll-mt-4 gap-0 border border-border py-0 shadow-none">
        <CardHeader className="flex flex-row items-center justify-between gap-3 border-b border-border px-4 py-3">
          <div className="min-w-0">
            <CardTitle className="flex items-center gap-2 text-sm font-semibold">
              <Clock3 className="size-4 text-brand" />
              后台任务与同步
            </CardTitle>
            <p className="mt-1 text-xs text-muted-foreground">统一配置作用于全部渠道或全部中转站的周期任务；手动同步入口仍保留在业务页面。</p>
          </div>
          <Button size="sm" className="h-9 shrink-0 gap-1.5" disabled={savingSync || syncQuery.loading || !syncQuery.data || !syncSettings} onClick={() => void saveSyncSettings()}>
            <Save className="size-3.5" />
            保存同步设置
          </Button>
        </CardHeader>
        {!syncQuery.data || !syncSettings ? <CardContent className="px-4 pb-4"><ConfigurationLoading query={syncQuery} title={"后台任务与同步设置"} className="py-5" /></CardContent> : <CardContent className="px-4 pb-4">
          <div className="py-3">
            <div className="flex items-center gap-2">
              <p className="text-xs font-semibold text-foreground">渠道任务</p>
              <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground">全局</span>
            </div>
            {
              <SyncTaskRow
                title="渠道余额与倍率同步"
                description="按设定间隔同步全部启用监控渠道的余额与分组倍率；暂停监控的渠道不会自动执行。"
                checked={syncSettings.channel_enabled}
                value={syncSettings.channel_interval_minutes || 30}
                options={channelSyncIntervals}
                formatOption={formatMinutes}
                  onCheckedChange={(value) => updateSyncSettings((current) => ({ ...current, channel_enabled: value }))}
                  onValueChange={(value) => updateSyncSettings((current) => ({ ...current, channel_interval_minutes: value }))}
              />
            }
          </div>

          <div className="border-t border-border py-3">
            <div className="flex items-center gap-2">
              <p className="text-xs font-semibold text-foreground">中转站任务</p>
              <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground">全部中转站</span>
            </div>
            {
              <div className="divide-y divide-border">
                <SyncTaskRow
                  title="API Key 倍率探测"
                  description="定期读取账号上游成本倍率，更新成本统计，并作为利润判断和自动调组的依据。"
                  checked={syncSettings.relay_rate_enabled}
                  value={syncSettings.relay_rate_interval_seconds || 10}
                  options={relayRateSyncIntervals}
                  formatOption={formatSeconds}
                   onCheckedChange={(value) => updateSyncSettings((current) => ({ ...current, relay_rate_enabled: value }))}
                   onValueChange={(value) => updateSyncSettings((current) => ({ ...current, relay_rate_interval_seconds: value }))}
                />
                <SyncTaskRow
                  title="中转站快照同步"
                  description="定期刷新全部中转站的账号、分组和关联关系快照，仅同步配置与状态，不执行倍率探测。"
                  checked={syncSettings.relay_snapshot_enabled}
                  value={syncSettings.relay_snapshot_interval_seconds || 3600}
                  options={relaySnapshotSyncIntervals}
                  formatOption={formatSeconds}
                   onCheckedChange={(value) => updateSyncSettings((current) => ({ ...current, relay_snapshot_enabled: value }))}
                   onValueChange={(value) => updateSyncSettings((current) => ({ ...current, relay_snapshot_interval_seconds: value }))}
                />
              </div>
            }
          </div>
        </CardContent>}
      </Card>

      <Card id="monitor-metrics" className="scroll-mt-4 gap-0 border border-border py-0 shadow-none">
        <CardHeader className="border-b border-border px-4 py-3">
          <CardTitle className="flex items-center gap-2 text-sm font-semibold"><Clock3 className="size-4 text-brand" />监控指标</CardTitle>
          <p className="mt-1 text-xs text-muted-foreground">配置缓存命中率和重试感知成功率的统计时间范围，保存后立即对渠道和中转站账号生效。</p>
        </CardHeader>
        <CardContent className="px-4 pb-4">
          <div className="flex min-h-16 items-center justify-between gap-5 py-3">
            <div className="min-w-0"><p className="text-sm font-medium text-foreground">缓存与成功率统计时长</p><p className="mt-1 text-xs leading-5 text-muted-foreground">默认 1 小时，可填写 1～24 的整数，单位为小时。</p></div>
            <div className="flex shrink-0 items-center gap-2"><Input className="h-9 w-20 text-center font-mono text-xs" type="number" min={1} max={24} step={1} inputMode="numeric" aria-label="缓存与成功率统计时长" value={settings.recent_metrics_window_hours} onChange={(event) => updateSettings((current) => ({ ...current, recent_metrics_window_hours: Number(event.target.value) }))} /><span className="text-xs text-muted-foreground">小时</span></div>
          </div>
        </CardContent>
      </Card>

      <div className="pt-2">
        <div className="flex items-center justify-between gap-3">
          <h2 className="text-sm font-semibold text-foreground">界面与页面</h2>
          <Button size="sm" className="h-11 shrink-0 gap-1.5 sm:h-9" disabled={saving || query.loading} onClick={() => void save()}>
            <Save className="size-3.5" />
            保存界面与监控设置
          </Button>
        </div>
        <p className="mt-1 text-xs text-muted-foreground">控制导航入口和页面内容的显示方式；关闭父级只会暂时隐藏下级内容，不会清空原有选择。</p>
      </div>

      <Card className="gap-0 border border-border py-0 shadow-none">
        <CardHeader className="border-b border-border px-4 py-3">
          <CardTitle className="flex items-center gap-2 text-sm font-semibold">
            <WalletCards className="size-4 text-brand" />
            运营
          </CardTitle>
          <p className="mt-1 text-xs text-muted-foreground">控制运营相关目录在主导航中的显示。</p>
        </CardHeader>
        <CardContent className="px-4 pb-4">
          <SettingRow
            level="目录"
            title="运营管理目录"
            description="关闭后在桌面和移动端主导航中隐藏“运营管理”及其子菜单；重新开启后恢复显示。"
            checked={settings.operations_management_enabled}
            onCheckedChange={(value) => updateSettings((current) => ({ ...current, operations_management_enabled: value }))}
          />
        </CardContent>
      </Card>

      <Card className="gap-0 border border-border py-0 shadow-none">
        <CardHeader className="border-b border-border px-4 py-3">
          <CardTitle className="flex items-center gap-2 text-sm font-semibold">
            <LayoutList className="size-4 text-brand" />
            渠道管理
          </CardTitle>
          <p className="mt-1 text-xs text-muted-foreground">配置渠道管理页面以及页面内部各内容块的显示层级。</p>
        </CardHeader>
        <CardContent className="px-4 pb-4">
          <SettingRow
            level="页面"
            title="渠道管理页面"
            description="关闭后从主导航隐藏“渠道管理”，直接访问页面时也不会展示渠道管理内容。"
            checked={settings.channels_page_enabled}
            onCheckedChange={(value) => updateSettings((current) => ({ ...current, channels_page_enabled: value }))}
          />

          <div className={cn("ml-2 border-l border-border pl-4", channelPageDisabled && "opacity-70")}>
            <SettingRow
              level="页面内容"
              title="分组倍率变动"
              description="显示顶部倍率变动面板，以及渠道分组上的最近倍率变化标记和提示。"
              checked={settings.channels_show_rate_changes}
              disabled={channelPageDisabled}
              onCheckedChange={(value) => updateSettings((current) => ({ ...current, channels_show_rate_changes: value }))}
            />

            <div className="border-t border-border">
              <SettingRow
                level="页面内容"
                title="渠道信息"
                description="控制每个渠道卡片的详细信息区域。关闭后，下级信息设置会保留但暂时不显示。"
                checked={settings.channels_show_channel_info}
                disabled={channelPageDisabled}
                onCheckedChange={(value) => updateSettings((current) => ({ ...current, channels_show_channel_info: value }))}
              />

              <div className={cn("ml-3 border-l border-border pl-4", channelInfoDisabled && "opacity-70")}>
                <SettingRow
                  level="内容信息"
                  title="最近调用延时"
                  description="显示每个渠道最近 60 次调用的首字响应时间与总耗时。"
                  checked={settings.channels_show_recent_latency}
                  disabled={channelInfoDisabled}
                  onCheckedChange={(value) => updateSettings((current) => ({ ...current, channels_show_recent_latency: value }))}
                />
                <div className="border-t border-border">
                  <SettingRow
                    level="内容信息"
                    title="渠道分组"
                    description="显示当前分组倍率列表和手动分组编辑区。关闭后不改变已有分组配置。"
                    checked={settings.channels_show_groups}
                    disabled={channelInfoDisabled}
                    onCheckedChange={(value) => updateSettings((current) => ({ ...current, channels_show_groups: value }))}
                  />
                  <div className="ml-3 border-l border-border pl-4">
                    <OptionRow
                      title="分组默认状态"
                      description="渠道分组显示时，控制首次进入页面默认折叠还是默认展开；不影响用户当前手动展开/收起。"
                      value={settings.channels_groups_expanded ? "expanded" : "collapsed"}
                      disabled={channelInfoDisabled || !settings.channels_show_groups}
                      onValueChange={(value) => updateSettings((current) => ({ ...current, channels_groups_expanded: value === "expanded" }))}
                    />
                  </div>
                </div>
                <div className="border-t border-border">
                  <SettingRow
                    level="内容信息"
                    title="时间稳定性"
                    description="显示 TTFT / 总耗时 P50、P95 的时间桶泳道。"
                    checked={settings.channels_show_stability}
                    disabled={channelInfoDisabled}
                    onCheckedChange={(value) => updateSettings((current) => ({ ...current, channels_show_stability: value }))}
                  />
                </div>
                <div className="border-t border-border">
                  <SettingRow
                    level="内容信息"
                    title="消耗趋势"
                    description="显示与稳定性时间桶对齐的调用消耗折线。"
                    checked={settings.channels_show_consumption}
                    disabled={channelInfoDisabled}
                    onCheckedChange={(value) => updateSettings((current) => ({ ...current, channels_show_consumption: value }))}
                  />
                </div>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card className="gap-0 border border-border py-0 shadow-none">
        <CardHeader className="border-b border-border px-4 py-3">
          <CardTitle className="flex items-center gap-2 text-sm font-semibold">
            <ShieldCheck className="size-4 text-brand" />
            安全与嵌入
          </CardTitle>
          <p className="mt-1 text-xs text-muted-foreground">
            控制 GatewayOps 是否允许被第三方页面通过 iframe 嵌入。默认关闭；白名单规则按父页面域名匹配。
          </p>
        </CardHeader>
        <CardContent className="px-4 pb-4">
          <SettingRow
            level="系统"
            title="允许 iframe 嵌入"
            description="关闭时所有站点都不能嵌入 GatewayOps；开启后可选择允许所有来源或仅允许指定白名单。"
            checked={settings.iframe_embedding_enabled}
            onCheckedChange={(value) => updateSettings((current) => ({ ...current, iframe_embedding_enabled: value }))}
          />

          <div className={cn("ml-2 border-l border-border pl-4", iframeEmbeddingDisabled && "opacity-60")}>
            <div className="flex min-h-16 items-center justify-between gap-5 py-3">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <p className="text-sm font-medium text-foreground">允许来源</p>
                  <span className="rounded bg-brand/10 px-1.5 py-0.5 text-[10px] font-medium text-brand">选项控制</span>
                </div>
                <p className="mt-1 text-xs leading-5 text-muted-foreground">
                  “允许所有”不限制父页面来源；“指定白名单”按下方域名 Glob 逐条匹配。
                </p>
              </div>
              <Select
                value={settings.iframe_embedding_mode}
                disabled={iframeEmbeddingDisabled}
                onValueChange={(value) => updateSettings((current) => ({
                  ...current,
                  iframe_embedding_mode: value as SystemSettings["iframe_embedding_mode"],
                }))}
              >
                <SelectTrigger className="h-9 w-32 shrink-0 text-xs"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="allowlist">指定白名单</SelectItem>
                  <SelectItem value="all">允许所有</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className={cn(
              "border-t border-border py-4",
              (iframeEmbeddingDisabled || settings.iframe_embedding_mode !== "allowlist") && "opacity-50",
            )}>
              <div className="flex flex-wrap items-center gap-2">
                <p className="text-sm font-medium text-foreground">iframe 域名白名单</p>
                <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground">每行一条</span>
              </div>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                只填写域名模式，不要填写 http(s)://、端口或路径。保存后无需重启服务。
              </p>
              <Textarea
                className="mt-3 min-h-36 font-mono text-xs"
                disabled={iframeEmbeddingDisabled || settings.iframe_embedding_mode !== "allowlist"}
                spellCheck={false}
                placeholder={"example.com\n*.example.com\n**.example.com\na*abc.com\n**.devai.**"}
                value={iframeAllowlistText}
                onChange={(event) => setIframeAllowlistDraft(event.target.value)}
              />
              <div className="mt-3 grid gap-1 text-[11px] leading-5 text-muted-foreground sm:grid-cols-2">
                <span><code>*</code>：所有域名</span>
                <span><code>*.example.com</code>：恰好一层子域名</span>
                <span><code>**.example.com</code>：任意多层子域名，不含根域名</span>
                <span><code>a*abc.com</code>：单个域名段内字符串通配</span>
                <span><code>*.devai.*</code>：devai 前后各一层</span>
                <span><code>**.devai.**</code>：devai 前后任意多层</span>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card className="gap-0 border border-border py-0 shadow-none">
        <CardHeader className="border-b border-border px-4 py-3">
          <CardTitle className="flex items-center gap-2 text-sm font-semibold">
            <Eye className="size-4 text-brand" />
            层级说明
          </CardTitle>
        </CardHeader>
        <CardContent className="px-4 py-4 text-xs leading-6 text-muted-foreground">
          页面控制入口是否展示；页面内容控制一个完整功能块；内容信息只控制功能块内部的具体信息。父级关闭时不会修改下级开关值。
        </CardContent>
      </Card>
    </section>
  )
}

