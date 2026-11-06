import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"
import ts from "typescript"

const source = readFileSync(new URL("./system-settings-page.tsx", import.meta.url), "utf8")

test("background tasks exposes all-channel usage repair settings and manual run", () => {
  assert.match(source, /channel_usage_integrity_enabled/)
  assert.match(source, /channel_usage_integrity_range/)
  assert.match(source, /channel_usage_integrity_time/)
  assert.match(source, /value: "today"/)
  assert.match(source, /value: "24h"/)
  assert.match(source, /value: "7d"/)
  assert.match(source, /value: "all"/)
  assert.match(source, /\/tasks\/channel_usage_integrity\/run/)
  assert.match(source, /TaskHistoryButton task="channel_usage_integrity"/)
})

test("settings page has valid TSX syntax", () => {
  const result = ts.transpileModule(source, { fileName: "system-settings-page.tsx", reportDiagnostics: true, compilerOptions: { jsx: ts.JsxEmit.ReactJSX, target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } })
  assert.deepEqual((result.diagnostics ?? []).filter(item => item.category === ts.DiagnosticCategory.Error).map(item => ts.flattenDiagnosticMessageText(item.messageText, "\n")), [])
})
