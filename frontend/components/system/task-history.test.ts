import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"
import ts from "typescript"

test("task history provides pagination, scoped logs and request cancellation", () => {
 const text = readFileSync(new URL("./task-history.tsx", import.meta.url), "utf8")
 for (const pattern of [/SheetContent/, /page_size/, /AbortController/, /clearInterval/, /encodeURIComponent/, /aria-label/, /succeeded/, /duration_ms/]) assert.match(text, pattern)
})

test("background task rows have stable task identifiers", () => {
 const text = readFileSync(new URL("../../app/system-settings-page.tsx", import.meta.url), "utf8")
 for (const task of ["channel_sync", "relay_rate_sync", "relay_snapshot_sync", "daily_balances", "retention", "pelican"]) assert.ok(text.includes(task))
 assert.ok(!text.includes('checked ? "\u8fd0\u884c\u4e2d"'))
})


test("\u8bbe\u7f6e\u9875\u9876\u90e8\u4e0d\u663e\u793a\u591a\u4f59\u7684\u540e\u53f0\u4efb\u52a1\u8df3\u8f6c\u6309\u94ae", () => {
 const text = readFileSync(new URL("../../app/system-settings-page.tsx", import.meta.url), "utf8")
 const header = text.slice(text.indexOf('<header className='), text.indexOf('</header>'))
 assert.ok(!header.includes('href="#background-tasks"'))
 assert.ok(!header.includes("\u540e\u53f0\u4efb\u52a1\u4e0e\u6267\u884c\u8bb0\u5f55"))
 assert.ok(text.includes('<TaskHistoryButton task={task} title={title} />'))
})

test("\u540e\u53f0\u4efb\u52a1\u5b9a\u4f4d\u7b49\u5f85\u914d\u7f6e\u5f02\u6b65\u52a0\u8f7d\u5b8c\u6210", () => {
 const text = readFileSync(new URL("../../app/system-settings-page.tsx", import.meta.url), "utf8")
 assert.match(text, /useLocation/)
 assert.match(text, /scrollIntoView/)
 assert.match(text, /\[hash, settingsLoaded\]/)
})

test("\u6267\u884c\u8bb0\u5f55\u6309\u94ae\u4f7f\u7528\u660e\u663e\u7684\u63cf\u8fb9\u6837\u5f0f", () => {
 const text = readFileSync(new URL("./task-history.tsx", import.meta.url), "utf8")
 assert.match(text.slice(text.indexOf('export function TaskHistoryButton'), text.indexOf('<Sheet open=')), /variant="outline"/)
})


test("\u6d4b\u667a\u6267\u884c\u8bb0\u5f55\u4e0e\u4fdd\u5b58\u6309\u94ae\u5728\u53f3\u4fa7\u540c\u7ec4\u76f8\u90bb\u6392\u5217", () => {
 const text = readFileSync(new URL("./pelican-settings-card.tsx", import.meta.url), "utf8")
 const file = ts.createSourceFile("pelican-settings-card.tsx", text, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX)
 let group: ts.JsxElement | undefined
 function visit(node: ts.Node) {
  if (ts.isJsxElement(node) && node.openingElement.tagName.getText(file) === "div") {
   const elements = node.children.filter(child => ts.isJsxElement(child) || ts.isJsxSelfClosingElement(child))
   const names = elements.map(child => ts.isJsxElement(child) ? child.openingElement.tagName.getText(file) : child.tagName.getText(file))
   if (names.join(",") === "TaskHistoryButton,Button") group = node
  }
  ts.forEachChild(node, visit)
 }
 visit(file)
 assert.ok(group, "history and save actions must be adjacent in one group")
 const attribute = group.openingElement.attributes.properties.find(attr => ts.isJsxAttribute(attr) && attr.name.getText(file) === "className")
 assert.ok(attribute && ts.isJsxAttribute(attribute) && attribute.initializer && ts.isStringLiteral(attribute.initializer))
 const classes = attribute.initializer.text.split(/\s+/)
 for (const token of ["ml-auto", "flex", "items-center", "gap-2"]) assert.ok(classes.includes(token), `missing action group style: ${token}`)
})
