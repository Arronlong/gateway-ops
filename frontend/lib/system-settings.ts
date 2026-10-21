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
  iframe_embedding_enabled: false,
  iframe_embedding_mode: "allowlist",
  iframe_embedding_allowlist: [],
}
