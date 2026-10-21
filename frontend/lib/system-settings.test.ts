import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22 native TypeScript tests require the explicit .ts extension.
import { defaultSystemSettings } from "./system-settings.ts"

test("operations management visibility defaults to enabled", () => {
  assert.equal(defaultSystemSettings.operations_management_enabled, true)
})

test("recent metrics window defaults to one hour", () => {
  assert.equal(defaultSystemSettings.recent_metrics_window_hours, 1)
})
