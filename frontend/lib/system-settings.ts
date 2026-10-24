import type { SystemSettings } from "@/lib/api-types"

export const defaultPelicanPrompt = "\u521b\u5efa\u4e00\u4e2a HTML\uff0c\u5185\u5bb9\u662f SVG \u7ed8\u5236\u4e00\u4e2a\u9e48\u9e55\u9a91\u81ea\u884c\u8f66\u7684 2D \u52a8\u753b\uff0c\u4f60\u4e0d\u9700\u8981\u4efb\u4f55\u6d4b\u8bd5\u3002"

const supportedPelicanTimezones = typeof Intl.supportedValuesOf === "function" ? Intl.supportedValuesOf("timeZone") : ["America/Los_Angeles", "America/New_York", "Asia/Shanghai", "Asia/Tokyo", "Europe/London"]
const currentPelicanTimezone = Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC"
export const pelicanTimezoneOptions = [...new Set(["UTC", ...supportedPelicanTimezones, currentPelicanTimezone])].map(value => ({
  value,
  label: value === currentPelicanTimezone ? `${value}（当前时区）` : value,
}))

export const pelicanWeekdayOptions = [
  { value: 1, label: "周一" },
  { value: 2, label: "周二" },
  { value: 3, label: "周三" },
  { value: 4, label: "周四" },
  { value: 5, label: "周五" },
  { value: 6, label: "周六" },
  { value: 0, label: "周日" },
] as const

export type ChannelSection = "recent" | "stability" | "consumption_trend" | "groups" | "rate_changes"

export function channelSectionVisible(settings: SystemSettings | null, section: ChannelSection) {
  if (!settings?.channels_page_enabled) return false
  if (section === "rate_changes") return settings.channels_show_rate_changes
  if (!settings.channels_show_channel_info) return false
  switch (section) {
    case "recent": return settings.channels_show_recent_latency
    case "stability": return settings.channels_show_stability
    case "consumption_trend": return settings.channels_show_consumption
    case "groups": return settings.channels_show_groups
  }
}

export const defaultSystemSettings: SystemSettings = {
  operations_management_enabled: true,
  channels_page_enabled: true,
  channels_show_rate_changes: true,
  channels_show_channel_info: true,
  channels_show_recent_latency: true,
  channels_show_groups: true,
  channels_groups_expanded: false,
  channels_show_stability: true,
  channels_show_consumption: true,
  recent_metrics_window_hours: 1,
  pelican_page_enabled: true,
  pelican_tasks_enabled: true,
  pelican_history_count: 30,
  pelican_plans: [{ model: "", prompt: defaultPelicanPrompt, reasoning_effort: "medium" }],
  pelican_model: "",
  pelican_prompt: defaultPelicanPrompt,
  pelican_reasoning_effort: "medium",
  pelican_parallel_count: 1,
  pelican_cron_expression: "*/30 * * * *",
  pelican_active_timezone: "Asia/Shanghai",
  pelican_active_start: "08:00",
  pelican_active_end: "22:00",
  pelican_active_weekdays: [0, 1, 2, 3, 4, 5, 6],
  iframe_embedding_enabled: false,
  iframe_embedding_mode: "allowlist",
  iframe_embedding_allowlist: [],
}
