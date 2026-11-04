import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22 原生 TypeScript 测试需要显式扩展名。
import { defaultCustomTimeRange, serializeTimeRange } from "./time-range.ts"

test("custom ranges default to the complete local day", () => {
  const range = defaultCustomTimeRange(new Date(2026, 10, 4, 9, 25, 36))
  assert.equal(range.start, "2026-11-04T00:00")
  assert.equal(range.end, "2026-11-04T23:59")
  const query = serializeTimeRange(range)
  assert.equal(query?.get("end"), new Date(2026, 10, 5, 0, 0).toISOString())
})

test("preset ranges serialize without changing their existing backend contract", () => {
  assert.equal(serializeTimeRange("7d")?.toString(), "range=7d")
  assert.equal(serializeTimeRange("all")?.toString(), "range=all")
})

test("custom local dates serialize as UTC and include the selected end minute", () => {
  const previousTimezone = process.env.TZ
  process.env.TZ = "Asia/Shanghai"
  try {
    const query = serializeTimeRange({
      mode: "custom",
      start: "2026-10-22T00:00",
      end: "2026-10-24T23:59",
    })
    assert.equal(query?.get("range"), "custom")
    assert.equal(query?.get("start"), "2026-10-21T16:00:00.000Z")
    assert.equal(query?.get("end"), "2026-10-24T16:00:00.000Z")
  } finally {
    if (previousTimezone === undefined) delete process.env.TZ
    else process.env.TZ = previousTimezone
  }
})

test("invalid or incomplete custom ranges do not produce a request", () => {
  assert.equal(serializeTimeRange({ mode: "custom", start: "", end: "2026-10-24T23:59" }), null)
  assert.equal(serializeTimeRange({ mode: "custom", start: "2026-10-24T23:59", end: "2026-10-22T00:00" }), null)
})
