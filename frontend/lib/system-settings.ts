import type { SystemSettings } from "@/lib/api-types"

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
  pelican_model: "",
  pelican_prompt: "\u521b\u5efa\u4e00\u4e2a HTML\uff0c\u5185\u5bb9\u662f SVG \u7ed8\u5236\u4e00\u4e2a\u9e48\u9e55\u9a91\u81ea\u884c\u8f66\u7684 2D \u52a8\u753b\uff0c\u4f60\u4e0d\u9700\u8981\u4efb\u4f55\u6d4b\u8bd5\u3002",
  pelican_reasoning_effort: "medium",
  pelican_parallel_count: 1,
  pelican_cron_expression: "*/30 * * * *",
  iframe_embedding_enabled: false,
  iframe_embedding_mode: "allowlist",
  iframe_embedding_allowlist: [],
}
