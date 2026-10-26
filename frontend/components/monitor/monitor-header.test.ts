import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"
import ts from "typescript"

const source = ts.createSourceFile(
  "monitor-header.tsx",
  readFileSync(new URL("monitor-header.tsx", import.meta.url), "utf8"),
  ts.ScriptTarget.Latest,
  true,
  ts.ScriptKind.TSX,
)
const documentationUrl = "https://gateway-ops.netlify.app/"

test("刷新视图按钮仅显示图标并保留无障碍标签", () => {
  const buttons: ts.JsxElement[] = []
  function visit(node: ts.Node) {
    if (ts.isJsxElement(node) && node.openingElement.tagName.getText() === "Button" && attribute(node, "onClick") === "{handleRefresh}") buttons.push(node)
    ts.forEachChild(node, visit)
  }
  visit(source)
  assert.equal(buttons.length, 2)
  for (const button of buttons) {
    assert.equal(attribute(button, "size"), '"icon"')
    assert.equal(attribute(button, "disabled"), "{syncing}")
    assert.ok(attribute(button, "aria-label"))
    const children = button.children.filter((child) => !ts.isJsxText(child) || child.getText().trim())
    assert.equal(children.length, 1)
    assert.ok(ts.isJsxSelfClosingElement(children[0]) && children[0].tagName.getText() === "RefreshCw")
  }
})

function attribute(element: ts.JsxElement | ts.JsxSelfClosingElement, name: string) {
  const opening = ts.isJsxElement(element) ? element.openingElement : element
  const property = opening.attributes.properties.find((item) => ts.isJsxAttribute(item) && item.name.getText() === name)
  return property && ts.isJsxAttribute(property) ? property.initializer?.getText() : undefined
}

for (const href of ["/public/relay-monitor", "/public/service-status"]) {
  test(`\u684c\u9762\u4e0e\u79fb\u52a8\u7aef\u76d1\u63a7\u83dc\u5355\u663e\u793a\u65b0\u6807\u7b7e\u9875\u56fe\u6807: ${href}`, () => {
    const links: ts.JsxElement[] = []
    function visit(node: ts.Node) {
      if (ts.isJsxElement(node) && node.openingElement.tagName.getText() === "a" && attribute(node, "href") === JSON.stringify(href)) links.push(node)
      ts.forEachChild(node, visit)
    }
    visit(source)
    assert.equal(links.length, 2)
    for (const link of links) {
      assert.equal(attribute(link, "target"), '"_blank"')
      assert.equal(attribute(link, "rel"), '"noopener noreferrer"')
      const icon = link.children.find((child) => ts.isJsxSelfClosingElement(child) && child.tagName.getText() === "ExternalLink")
      assert.ok(icon && ts.isJsxSelfClosingElement(icon))
      assert.equal(attribute(icon, "aria-hidden"), '"true"')
      assert.match(attribute(icon, "className") ?? "", /ml-auto/)
      const label = link.children.findIndex((child) => ts.isJsxText(child) && child.getText().trim().length > 0)
      assert.ok(label >= 0 && link.children.indexOf(icon) > label)
      const hint = link.children.find((child) => ts.isJsxElement(child) && attribute(child, "className") === '"sr-only"')
      assert.ok(hint && hint.getText().includes("\u5728\u65b0\u6807\u7b7e\u9875\u6253\u5f00"))
    }
  })
}

test("desktop header exposes a compact documentation icon link", () => {
  const links: ts.JsxElement[] = []
  function visit(node: ts.Node) {
    if (ts.isJsxElement(node) && node.openingElement.tagName.getText() === "a" && attribute(node, "href") === JSON.stringify(documentationUrl)) links.push(node)
    ts.forEachChild(node, visit)
  }
  visit(source)
  assert.equal(links.length, 2)

  const desktopLink = links.find((link) => ts.isJsxElement(link.parent) && link.parent.openingElement.tagName.getText() === "Button")
  assert.ok(desktopLink)
  const button = desktopLink.parent
  assert.ok(ts.isJsxElement(button))
  assert.match(attribute(button, "className") ?? "", /hidden[^\n]*sm:inline-flex/)
  assert.ok(attribute(button, "aria-label"))
  assert.equal(attribute(desktopLink, "target"), '"_blank"')
  assert.equal(attribute(desktopLink, "rel"), '"noopener noreferrer"')
  assert.ok(desktopLink.children.some((child) => ts.isJsxSelfClosingElement(child) && child.tagName.getText() === "BookOpen"))
})

test("mobile menu keeps documentation reachable without adding a toolbar control", () => {
  const links: ts.JsxElement[] = []
  function visit(node: ts.Node) {
    if (ts.isJsxElement(node) && node.openingElement.tagName.getText() === "a" && attribute(node, "href") === JSON.stringify(documentationUrl)) links.push(node)
    ts.forEachChild(node, visit)
  }
  visit(source)

  const menuLink = links.find((link) => ts.isJsxElement(link.parent) && link.parent.openingElement.tagName.getText() === "DropdownMenuItem")
  assert.ok(menuLink)
  const menuItem = menuLink.parent
  assert.ok(ts.isJsxElement(menuItem))
  assert.match(attribute(menuItem, "className") ?? "", /sm:hidden/)
  assert.equal(attribute(menuLink, "target"), '"_blank"')
  assert.equal(attribute(menuLink, "rel"), '"noopener noreferrer"')
  assert.ok(menuLink.children.some((child) => ts.isJsxSelfClosingElement(child) && child.tagName.getText() === "BookOpen"))
})
