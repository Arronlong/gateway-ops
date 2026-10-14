const options = { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false } as const
const minuteFormatter = new Intl.DateTimeFormat("zh-CN", options)
const secondFormatter = new Intl.DateTimeFormat("zh-CN", { ...options, second: "2-digit" })

/** 泳道会一次格式化大量时间，复用格式化器，保留浏览器本地时区。 */
export function formatMonitorTime(value: string, withSeconds = false): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return "-"
  return (withSeconds ? secondFormatter : minuteFormatter).format(date)
}
