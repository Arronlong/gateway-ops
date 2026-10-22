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

test("pelican generation parameters belong to global settings", () => {
  assert.equal(defaultSystemSettings.pelican_model, "")
  assert.match(defaultSystemSettings.pelican_prompt, /SVG/)
  assert.equal(defaultSystemSettings.pelican_reasoning_effort, "medium")
  assert.equal(defaultSystemSettings.pelican_parallel_count, 1)
  assert.equal(defaultSystemSettings.pelican_cron_expression, "*/30 * * * *")
})
