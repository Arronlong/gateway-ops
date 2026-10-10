import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"
import ts from "typescript"

function parse(relativePath: string) {
  return ts.createSourceFile(relativePath, readFileSync(new URL(relativePath, import.meta.url), "utf8"), ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX)
}

function listHeader(file: ts.SourceFile, title: string) {
  let result: ts.JsxElement | undefined
  function visit(node: ts.Node) {
    if (ts.isJsxElement(node) && node.openingElement.tagName.getText() === "ListPanelHeader") {
      const heading = node.openingElement.attributes.properties.find((attribute) => ts.isJsxAttribute(attribute) && attribute.name.getText() === "title")
      if (heading?.getText().includes(title)) result = node
    }
    ts.forEachChild(node, visit)
  }
  visit(file)
  assert.ok(result, `Missing ListPanelHeader: ${title}`)
  return result
}

const panels: [string, string][] = [
  ["../../app/relay-stations-page.tsx", "\u6700\u8fd1\u4f7f\u7528\u8bb0\u5f55"],
  ["../../app/relay-stations-page.tsx", "\u5206\u7ec4\u7ba1\u7406"],
  ["../relay/user-management.tsx", "\u7528\u6237\u7ba1\u7406"],
  ["../monitor/channel-cards.tsx", "\u6e20\u9053\u5217\u8868"],
  ["../../app/local-pool-page.tsx", "OAuth \u53f7\u6c60\u7edf\u8ba1"],
  ["../monitor/bottom-panels.tsx", "\u901a\u77e5\u53d1\u9001\u8bb0\u5f55"],
]

for (const [path, title] of panels) {
  test(`\u7ba1\u7406\u5217\u8868\u91c7\u7528\u6807\u9898\u884c\u5de6\u53f3\u5e03\u5c40: ${title}`, () => {
    const header = listHeader(parse(path), title)
    const attributes = header.openingElement.attributes.properties
    const secondary = attributes.find((attribute) => ts.isJsxAttribute(attribute) && attribute.name.getText() === "secondary")
    const actions = attributes.find((attribute) => ts.isJsxAttribute(attribute) && attribute.name.getText() === "actions")
    const placement = attributes.find((attribute) => ts.isJsxAttribute(attribute) && attribute.name.getText() === "secondaryPlacement")
    assert.ok(secondary, `Missing secondary row: ${title}`)
    assert.doesNotMatch(actions?.getText() ?? "", /<Input\b|<Select\b|<Tabs\b/)
    assert.match(placement?.getText() ?? "", /"inline"/)
    assert.match(header.openingElement.getText(), /actionsPlacement="secondary"/)
  })
}

test("\u8d26\u53f7\u5217\u8868\u7684\u641c\u7d22\u548c\u6279\u91cf\u64cd\u4f5c\u4e0e\u6307\u6807\u884c\u5de6\u53f3\u5e03\u5c40", () => {
  const header = listHeader(parse("../../app/relay-stations-page.tsx"), "\u8d26\u53f7\u5217\u8868")
  const props = header.openingElement.getText()
  assert.match(props, /secondaryPlacement="inline"/)
  assert.match(props, /actionsPlacement="secondary"/)
  assert.match(props, /DebouncedAccountSearchInput/)
  assert.match(props, /summary=\{/)
})

test("\u53f7\u6c60\u53ea\u4fdd\u7559\u4e00\u5957\u641c\u7d22\u7b5b\u9009\u548c\u89c6\u56fe\u63a7\u4ef6", () => {
  const file = parse("../../app/local-pool-page.tsx")
  assert.equal(file.text.split('placeholder="\u8d26\u53f7\u540d\u79f0\u6216 ID"').length - 1, 1)
  const header = listHeader(file, "OAuth \u53f7\u6c60\u7edf\u8ba1")
  const props = header.openingElement.getText()
  assert.match(props, /setView\("aggregate"\)/)
  assert.match(props, /setRange\(item.value\)/)
  assert.match(props, /setStationID/)
})

test("\u6536\u652f\u8d26\u672c\u4fdd\u7559\u65f6\u95f4\u548c\u64cd\u4f5c\u4e0e\u6807\u9898\u540c\u884c", () => {
  const header = listHeader(parse("../../app/operations-costs-page.tsx"), "\u6536\u652f\u8d26\u672c")
  const props = header.openingElement.getText()
  assert.match(props, /actions=/)
  assert.match(props, /aria-label="\u8d26\u672c\u65f6\u95f4\u8303\u56f4"/)
  assert.doesNotMatch(props, /secondary=/)
})

test("\u53ea\u8bfb\u6c47\u603b\u548c\u8bb0\u5f55\u5217\u8868\u540c\u6837\u4f7f\u7528\u516c\u5171\u5217\u8868\u5934", () => {
  listHeader(parse("../../app/operations-costs-page.tsx"), "\u4e2d\u8f6c\u7ad9\u6536\u652f\u6c47\u603b\u8868")
  listHeader(parse("../monitor/multiplier-changes.tsx"), "\u6700\u8fd1\u500d\u7387\u53d8\u52a8")
})
