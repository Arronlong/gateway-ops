import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"
import ts from "typescript"

function parseFile(relativePath: string) {
  return ts.createSourceFile(relativePath, readFileSync(new URL(relativePath, import.meta.url), "utf8"), ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX)
}

function findElement(source: ts.SourceFile, predicate: (element: ts.JsxElement) => boolean) {
  let result: ts.JsxElement | undefined
  function visit(node: ts.Node) {
    if (result) return
    if (ts.isJsxElement(node) && predicate(node)) result = node
    else ts.forEachChild(node, visit)
  }
  visit(source)
  assert.ok(result, "Expected JSX element was not found")
  return result
}

function tag(element: ts.JsxElement) {
  return element.openingElement.tagName.getText()
}

function elementAncestors(node: ts.Node) {
  const elements: ts.JsxElement[] = []
  for (let parent = node.parent; parent; parent = parent.parent) {
    if (ts.isJsxElement(parent)) elements.push(parent)
  }
  return elements
}

function className(element: ts.JsxElement) {
  const attribute = element.openingElement.attributes.properties.find((property) => ts.isJsxAttribute(property) && property.name.getText() === "className")
  if (!attribute || !ts.isJsxAttribute(attribute) || !attribute.initializer || !ts.isStringLiteral(attribute.initializer)) return ""
  return attribute.initializer.text
}

test("\u8d26\u53f7\u641c\u7d22\u4e0e\u6279\u91cf\u64cd\u4f5c\u6309\u529f\u80fd\u5206\u5c42", () => {
  const source = parseFile("../../app/relay-stations-page.tsx")
  const batch = findElement(source, (element) => tag(element) === "DropdownMenuItem" && element.getText().includes("\u6279\u91cf\u65b0\u589e\u8d26\u53f7"))
  const search = findElement(source, (element) => tag(element) === "div" && element.openingElement.getText().includes('className="relative') && element.getText().includes('placeholder="搜索账号 / 地址 / 平台 / 模型"'))
  const header = findElement(source, (element) => tag(element) === "ListPanelHeader" && element.getText().includes("\u8d26\u53f7\u5217\u8868"))
  const actions = header.openingElement.attributes.properties.find((attribute) => ts.isJsxAttribute(attribute) && attribute.name.getText() === "actions")
  const secondary = header.openingElement.attributes.properties.find((attribute) => ts.isJsxAttribute(attribute) && attribute.name.getText() === "secondary")
  assert.match(header.openingElement.getText(), /secondaryPlacement="inline"/)
  assert.match(header.openingElement.getText(), /actionsPlacement="secondary"/)
  assert.ok(actions && actions.getText().includes("\u6279\u91cf\u64cd\u4f5c") && actions.getText().includes(batch.getText()))
  assert.doesNotMatch(actions?.getText() ?? "", /placeholder=/)
  const delegatedSearch = secondary?.getText().includes("<DebouncedAccountSearchInput") && source.text.includes("function DebouncedAccountSearchInput")
  assert.ok(secondary && (secondary.getText().includes(search.getText()) || delegatedSearch))
  assert.match(secondary?.getText() ?? "", /\u7b5b\u9009/)
})

test("\u4e2d\u8f6c\u7ad9\u8d26\u53f7\u5217\u8868\u9ed8\u8ba4\u53ea\u663e\u793a\u8c03\u5ea6\u5f00\u542f\u8d26\u53f7", () => {
  const source = parseFile("../../app/relay-stations-page.tsx")
  assert.match(source.text, /useState<"enabled" \| "disabled" \| "all">\("enabled"\)/)
})

test("\u4e2d\u8f6c\u7ad9\u5207\u6362\u590d\u7528\u5e76\u9884\u53d6\u5feb\u7167\u6570\u636e", () => {
  const page = parseFile("../../app/relay-stations-page.tsx")
  const queries = readFileSync(new URL("../../lib/queries.ts", import.meta.url), "utf8")
  assert.match(queries, /RELAY_OVERVIEW_CACHE_TTL_MS/)
  assert.match(queries, /export function usePrefetchRelayOverviews/)
  assert.match(page.text, /usePrefetchRelayOverviews\(/)
})

test("\u4e2d\u8f6c\u7ad9\u91cd\u578b\u7edf\u8ba1\u7b49\u8d26\u53f7\u5feb\u7167\u9996\u5c4f\u540e\u518d\u52a0\u8f7d", () => {
  const page = parseFile("../../app/relay-stations-page.tsx")
  assert.match(page.text, /const\s+overviewReady\s*=\s*overview\.data\s*!==\s*null/)
  assert.match(page.text, /useRelayLatencyBuckets\([^\n]*overviewReady/)
  assert.match(page.text, /useRelayAccountConsumptionBuckets\([^\n]*overviewReady/)
  assert.match(page.text, /useChannelLatencyBuckets\([^\n]*overviewReady/)
  assert.match(page.text, /useChannelConsumptionBuckets\([^\n]*overviewReady/)
})

test("渠道标题与列表共用圆角卡片", () => {
  const source = parseFile("channel-cards.tsx")
  const title = findElement(source, (element) => ["h2", "CardTitle"].includes(tag(element)) && element.getText().includes("渠道列表"))
  const ancestors = elementAncestors(title)
  const header = ancestors.find((element) => tag(element) === "ListPanelHeader")
  const card = ancestors.find((element) => tag(element) === "Card")
  assert.ok(header, "Channel title must be inside ListPanelHeader")
  assert.ok(card, "Channel list must have a shared Card container")
  assert.match(title.openingElement.getText(), /role="heading"/)
  assert.match(title.openingElement.getText(), /aria-level=\{2\}/)
  assert.match(className(card), /\bpy-0\b/)
  assert.match(className(card), /\boverflow-hidden\b/)
  const content = findElement(source, (element) => tag(element) === "CardContent" && element.getText().includes('viewMode === "gallery"'))
  assert.ok(elementAncestors(content).includes(card))
  assert.match(className(content), /\bborder-t\b/)
})

test("渠道表格不再重复添加外边框", () => {
  const source = parseFile("channel-cards.tsx")
  const scroll = findElement(source, (element) => tag(element) === "div" && className(element).includes("max-h-[min(72vh,760px)]"))
  assert.doesNotMatch(className(scroll), /\brounded-lg\b|\bborder\b/)
  assert.ok(elementAncestors(scroll).some((element) => tag(element) === "CardContent"))
})

test("账号列表折叠时隐藏消费统计", () => {
  const source = parseFile("../../app/relay-stations-page.tsx")
  const statistic = findElement(source, (element) => tag(element) === "div" && element.openingElement.getText().includes('aria-live="polite"') && element.getText().includes("usageTotals.tokens"))
  let conditional = false
  for (let node: ts.Node | undefined = statistic.parent; node; node = node.parent) {
    if (ts.isJsxElement(node) && tag(node) === "ListPanelHeader") break
    if (ts.isConditionalExpression(node) && node.condition.getText() === "accountListOpen") conditional = true
  }
  assert.ok(conditional, "Statistics must respect accountListOpen")
})

test("收支账本标题与时间操作区同行左右布局", () => {
  const source = parseFile("../../app/operations-costs-page.tsx")
  const title = findElement(source, (element) => tag(element) === "CardTitle" && element.getText().includes("收支账本"))
  const toolbar = findElement(source, (element) => tag(element) === "div" && element.openingElement.getText().includes('aria-label="收支账本筛选"'))
  const sharedRow = elementAncestors(title).find((element) => elementAncestors(toolbar).includes(element))
  assert.ok(sharedRow)
  assert.equal(tag(sharedRow), "ListPanelHeader")
  const actions = sharedRow.openingElement.attributes.properties.find((attribute) => ts.isJsxAttribute(attribute) && attribute.name.getText() === "actions")
  assert.ok(actions && actions.getText().includes(toolbar.getText()))
  const common = parseFile("../ui/list-panel.tsx")
  assert.match(common.text, /flex-wrap justify-between/)
  assert.match(common.text, /ml-auto.*justify-end/)
  assert.ok(toolbar.getText().includes('aria-label="账本时间范围"'))
  assert.ok(toolbar.getText().includes("setLedgerQuery"))
  assert.ok(toolbar.getText().includes("ledger.refetch()"))
})
