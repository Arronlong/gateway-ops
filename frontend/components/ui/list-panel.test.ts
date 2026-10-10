import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"
import ts from "typescript"

function source(relativePath: string) {
  return ts.createSourceFile(relativePath, readFileSync(new URL(relativePath, import.meta.url), "utf8"), ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX)
}

function elements(file: ts.SourceFile, name: string) {
  const result: ts.JsxElement[] = []
  function visit(node: ts.Node) {
    if (ts.isJsxElement(node) && node.openingElement.tagName.getText() === name) result.push(node)
    ts.forEachChild(node, visit)
  }
  visit(file)
  return result
}

test("公共列表头使用统一的左右布局和间距", () => {
  const file = source("./list-panel.tsx")
  const header = elements(file, "CardHeader")[0]
  assert.ok(header)
  assert.match(header.openingElement.getText(), /flex flex-col items-stretch px-4 py-3/)
  assert.match(header.openingElement.getText(), /secondaryPlacement === "row" && "gap-2"/)
  assert.match(file.text, /data-slot="list-panel-header-row"/)
  assert.match(file.text, /data-slot="list-panel-toolbar"/)
  assert.match(file.text, /flex-wrap items-center justify-end gap-2/)
  assert.match(file.text, /max-w-full/)
})


test("\u6b21\u7ea7\u5de5\u5177\u680f\u6e32\u67d3\u5728\u72ec\u7acb\u5206\u9694\u884c", () => {
  const file = source("./list-panel.tsx")
  assert.match(file.text, /secondary\?: ReactNode/)
  assert.match(file.text, /data-slot="list-panel-secondary-toolbar"/)
  assert.match(file.text, /border-t border-border pt-2/)
})

test("\u6709\u7edf\u8ba1\u7684\u5217\u8868\u5934\u5c06\u64cd\u4f5c\u533a\u4e0e\u7edf\u8ba1\u884c\u5bf9\u9f50", () => {
  const file = source("./list-panel.tsx")
  const row = elements(file, "div").find((element) => element.openingElement.getText().includes('data-slot="list-panel-header-row"'))
  assert.ok(row)
  assert.match(row.openingElement.getText(), /summary\s*\?\s*"items-end"\s*:\s*"items-center"/)
})

test("\u8d26\u53f7\u5217\u8868\u7b5b\u9009\u5de5\u5177\u4e0e\u6279\u91cf\u64cd\u4f5c\u5171\u7528\u4e00\u884c", () => {
  const component = source("./list-panel.tsx")
  assert.match(component.text, /secondaryPlacement\?: "row" \| "inline"/)
  assert.match(component.text, /actionsPlacement\?: "header" \| "secondary"/)
  assert.match(component.text, /secondaryPlacement === "inline"/)
  assert.match(component.text, /actionsPlacement === "secondary"/)

  const page = source("../../app/relay-stations-page.tsx")
  const header = elements(page, "ListPanelHeader").find((element) => element.getText().includes("账号列表"))
  assert.ok(header)
  assert.match(header.openingElement.getText(), /secondaryPlacement="inline"/)
  assert.match(header.openingElement.getText(), /actionsPlacement="secondary"/)
  assert.ok(header.getText().includes("全选当前筛选"))
  const actions = header.openingElement.attributes.properties.find((attribute) => ts.isJsxAttribute(attribute) && attribute.name.getText() === "actions")
  const secondary = header.openingElement.attributes.properties.find((attribute) => ts.isJsxAttribute(attribute) && attribute.name.getText() === "secondary")
  assert.ok(actions && actions.getText().includes("批量操作"))
  assert.ok(secondary && secondary.getText().includes("DebouncedAccountSearchInput"))
  assert.match(page.text, /placeholder="搜索账号 \/ 地址 \/ 平台 \/ 模型"/)
  assert.match(component.text, /2xl:flex-nowrap/)
})

test("\u8d26\u53f7\u641c\u7d22\u7ed3\u679c\u66f4\u65b0\u4f7f\u7528\u53ef\u4e2d\u65ad\u7684 transition", () => {
  const page = source("../../app/relay-stations-page.tsx")
  assert.match(page.text, /startTransition\(\(\) => onValueChange\(draft\)\)/)
})

test("\u8d26\u53f7\u641c\u7d22\u7684\u5ef6\u8fdf\u7ed3\u679c\u4e0d\u80fd\u8986\u76d6\u6b63\u5728\u8f93\u5165\u7684\u8349\u7a3f", () => {
  const page = source("../../app/relay-stations-page.tsx")
  assert.doesNotMatch(page.text, /setDraft\(value\)/)
  assert.match(page.text, /accountSearchResetVersion/)
  assert.match(page.text, /if \(lastResetVersion\.current === resetVersion\) return/)
})

const panels: [string, string[]][] = [
  ["../../app/relay-stations-page.tsx", ["账号列表", "最近使用记录", "分组管理"]],
  ["../relay/user-management.tsx", ["用户管理"]],
  ["../../app/local-pool-page.tsx", ["OAuth 号池统计"]],
  ["../monitor/bottom-panels.tsx", ["验证码服务配置", "通知渠道管理", "通知发送记录"]],
  ["../monitor/channel-cards.tsx", ["渠道列表"]],
  ["../../app/operations-costs-page.tsx", ["收支账本"]],
]

for (const [path, titles] of panels) {
  for (const title of titles) {
    test(`管理表格复用公共列表头: ${title}`, () => {
      const file = source(path)
      const headers = elements(file, "ListPanelHeader")
      const header = headers.find((element) => element.getText().includes(title))
      assert.ok(header, `Missing shared header for ${title}`)
      assert.match(header.openingElement.getText(), /title=\{/)
      assert.match(header.openingElement.getText(), /actions=\{/)
    })
  }
}

test("调整记录筛选标签与标题共用列表头", () => {
  const file = source("../monitor/relay-adjustment-log.tsx")
  const header = elements(file, "ListPanelHeader")[0]
  assert.ok(header)
  assert.ok(header.getText().includes("Tabs"))
  assert.ok(header.getText().includes("aria-expanded={open}"))
})

test("行内操作使用统一边框与危险状态", () => {
  const file = source("./table-action-button.tsx")
  assert.match(file.text, /variant="outline"/)
  assert.match(file.text, /size="icon"/)
  assert.match(file.text, /size-8 shrink-0/)
  assert.match(file.text, /border-danger\/30 text-danger hover:bg-danger\/10 hover:text-danger/)
  for (const path of ["../../app/relay-stations-page.tsx", "../monitor/channel-cards.tsx", "../monitor/bottom-panels.tsx", "../../app/operations-costs-page.tsx"]) {
    assert.ok(elements(source(path), "TableActionButton").length > 0, path)
  }
})
