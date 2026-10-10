import assert from "node:assert/strict"
import test from "node:test"

// @ts-expect-error Node 22 原生 TypeScript 测试需要显式 .ts 扩展名。
import { scheduleQueryUpdate } from "./query-transition.ts"

test("schedules query result updates through the provided transition scheduler", () => {
  const callbacks: Array<() => void> = []
  let updated = false

  scheduleQueryUpdate(() => {
    updated = true
  }, (callback) => callbacks.push(callback))

  assert.equal(updated, false)
  assert.equal(callbacks.length, 1)
  callbacks[0]()
  assert.equal(updated, true)
})
