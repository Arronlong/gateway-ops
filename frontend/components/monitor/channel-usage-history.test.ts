import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"
import ts from "typescript"

const source = readFileSync(new URL("./channel-usage-history.tsx", import.meta.url), "utf8")

test("channel history does not automatically query integrity totals", () => {
  assert.doesNotMatch(source, /useChannelUsageIntegrity/)
})

test("channel history has valid TSX syntax", () => {
  const result = ts.transpileModule(source, { fileName: "channel-usage-history.tsx", reportDiagnostics: true, compilerOptions: { jsx: ts.JsxEmit.ReactJSX, target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } })
  assert.deepEqual((result.diagnostics ?? []).filter(item => item.category === ts.DiagnosticCategory.Error).map(item => ts.flattenDiagnosticMessageText(item.messageText, "\n")), [])
})

test("channel history retains the manual compare and repair flow", () => {
  assert.match(source, /refetchUsageHistoryStream\(channel\.id/)
  assert.match(source, /setOpen\(true\)/)
  assert.match(source, /historyRanges/)
  assert.match(source, /refetchHistory\(\)/)
})
