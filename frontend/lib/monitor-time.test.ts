import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22 原生 TypeScript 测试需要显式扩展名。
import { formatMonitorTime } from "./monitor-time.ts"

test("监控时间保持本地时区及分钟、秒格式", () => {
  const value = "2026-10-14T10:20:30Z"
  const date = new Date(value)
  const options = { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false } as const
  assert.equal(formatMonitorTime(value), date.toLocaleString("zh-CN", options))
  assert.equal(formatMonitorTime(value, true), date.toLocaleString("zh-CN", { ...options, second: "2-digit" }))
  assert.equal(formatMonitorTime("invalid"), "-")
})

test("批量格式化复用格式化器，不再逐格调用 toLocaleString", () => {
  const original = Date.prototype.toLocaleString
  const originalFormatter = Intl.DateTimeFormat
  Date.prototype.toLocaleString = () => { throw new Error("Repeated locale formatter construction") }
  Intl.DateTimeFormat = new Proxy(originalFormatter, { construct() { throw new Error("Repeated locale formatter construction") } })
  try {
    for (let index = 0; index < 2000; index++) {
      assert.notEqual(formatMonitorTime("2026-10-14T10:20:30Z", index % 2 === 0), "-")
    }
  } finally {
    Date.prototype.toLocaleString = original
    Intl.DateTimeFormat = originalFormatter
  }
})
