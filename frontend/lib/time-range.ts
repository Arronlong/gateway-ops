export type CustomTimeRangeSelection = {
  mode: "custom"
  start: string
  end: string
}

export type TimeRangeSelection<T extends string> = T | CustomTimeRangeSelection

export function localDateTimeInputValue(date: Date) {
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000)
  return local.toISOString().slice(0, 16)
}

export function defaultCustomTimeRange(now = new Date()): CustomTimeRangeSelection {
  const end = new Date(now)
  end.setHours(23, 59, 0, 0)
  const start = new Date(end)
  start.setHours(0, 0, 0, 0)
  return { mode: "custom", start: localDateTimeInputValue(start), end: localDateTimeInputValue(end) }
}

export function serializeTimeRange<T extends string>(selection: TimeRangeSelection<T>) {
  const query = new URLSearchParams()
  if (typeof selection === "string") {
    query.set("range", selection)
    return query
  }
  if (!selection.start || !selection.end) return null
  const start = new Date(selection.start)
  const end = new Date(selection.end)
  if (!Number.isFinite(start.getTime()) || !Number.isFinite(end.getTime()) || end < start) return null
  query.set("range", "custom")
  query.set("start", start.toISOString())
  query.set("end", new Date(end.getTime() + 60_000).toISOString())
  const timeZone = Intl.DateTimeFormat().resolvedOptions().timeZone
  if (timeZone) query.set("time_zone", timeZone)
  return query
}

export function timeRangeQuery<T extends string>(selection: TimeRangeSelection<T>) {
  const query = serializeTimeRange(selection)
  return query ? `?${query.toString()}` : null
}

export function timeRangeKey<T extends string>(selection: TimeRangeSelection<T>) {
  return serializeTimeRange(selection)?.toString() ?? "range=invalid"
}

export function timeRangeLabel<T extends string>(selection: TimeRangeSelection<T>, presets: readonly { value: string; label: string }[]) {
  if (typeof selection === "string") return presets.find((preset) => preset.value === selection)?.label ?? selection
  return `${selection.start.replace("T", " ")} - ${selection.end.replace("T", " ")}`
}
