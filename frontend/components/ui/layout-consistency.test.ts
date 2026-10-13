import assert from "node:assert/strict"
import { existsSync, readFileSync } from "node:fs"
import test from "node:test"

const read = (file: string) => readFileSync(new URL(file, import.meta.url), "utf8")

test("\u516c\u5171\u5f39\u7a97\u4fdd\u7559\u89c6\u53e3\u8fb9\u8ddd\u548c\u5185\u90e8\u6eda\u52a8", () => {
  const source = read("dialog.tsx")
  assert.match(source, /w-\[calc\(100vw-2rem\)\]/)
  assert.match(source, /max-h-\[calc\(100dvh-2rem\)\]/)
  assert.match(source, /overflow-y-auto/)
  assert.doesNotMatch(source, /sm:max-w-lg/)
})

test("\u5361\u7247\u5934\u4e0d\u518d\u9690\u5f0f\u8986\u76d6\u5e95\u90e8\u7559\u767d", () => {
  const source = read("card.tsx")
  assert.doesNotMatch(source, /\[\.border-b\]:pb-6/)
  assert.match(source, /px-4 py-3/)
  const balance = read("../monitor/balance-overview.tsx")
  assert.match(balance, /<ListPanelHeader/)
  assert.doesNotMatch(balance, /CardTitle className="text-base/)
})

test("\u5de5\u5177\u680f\u6309\u89c6\u53e3\u7edf\u4e00\u63a7\u4ef6\u9ad8\u5ea6", () => {
  assert.match(read("list-panel.tsx"), /list-panel-controls/)
  const css = read("../../app/globals.css")
  assert.match(css, /\.list-panel-controls/)
  assert.match(css, /height: 2\.75rem/)
  assert.match(css, /height: 2\.25rem/)
  assert.match(css, /role='checkbox'/)
  assert.match(css, /role='switch'/)
})

test("\u4e2d\u8f6c\u7ad9\u65f6\u95f4\u6761\u4ef6\u5141\u8bb8\u6536\u7f29\u6362\u884c", () => {
  const source = read("../../app/relay-stations-page.tsx")
  const line = source.split("\n").find((line) => line.includes('aria-label="\u4e2d\u8f6c\u7ad9\u7edf\u4e00\u65f6\u95f4\u6761\u4ef6"'))
  assert.ok(line)
  assert.match(line, /min-w-0 max-w-full/)
  assert.doesNotMatch(line, /shrink-0/)
})

test("\u901a\u77e5\u5206\u7ec4\u7528\u6237\u4f7f\u7528\u7edf\u4e00\u7b5b\u9009\u83dc\u5355", () => {
  for (const file of ["../monitor/bottom-panels.tsx", "../../app/relay-stations-page.tsx", "../relay/user-management.tsx"]) {
    assert.match(read(file), /<ListFilterMenu/)
  }
  assert.ok(existsSync(new URL("list-filter-menu.tsx", import.meta.url)))
  const menu = read("list-filter-menu.tsx")
  assert.match(menu, /DropdownMenuRadioGroup/)
  assert.match(menu, /onValueChange=\{group.onValueChange\}/)
  assert.match(menu, /onSelect=\{onReset\}/)
  assert.match(menu, /activeCount > 0/)
})

test("\u7528\u6237\u6ce8\u518c IP \u7b5b\u9009\u548c\u5206\u7ec4\u91cd\u7f6e\u884c\u4e3a\u4fdd\u7559", () => {
  const users = read("../relay/user-management.tsx")
  assert.match(users, /setRegistrationIPDraft\(event.target.value\)/)
  assert.match(users, /value=\{registrationIPDraft\}/)
  const groups = read("../../app/relay-stations-page.tsx")
  assert.match(groups, /function resetFilters\(\).*setTypeFilter\("all"\).*setStatusFilter\("all"\).*setSort\(null\)/)
})

test("\u5efa\u8bae\u7c7b\u64cd\u4f5c\u5165\u53e3\u4fdd\u6301\u4e0d\u53d8", () => {
  const costs = read("../../app/operations-costs-page.tsx")
  const pageHeader = costs.slice(costs.indexOf("<header"), costs.indexOf("</header>"))
  assert.match(pageHeader, /\u8bb0\u4e00\u7b14/)
  const ledgerHeader = costs.slice(costs.indexOf("<ListPanelHeader", costs.indexOf("<ListPanelHeader") + 1), costs.indexOf("</ListPanelHeader>"))
  assert.doesNotMatch(ledgerHeader, /\u8bb0\u4e00\u7b14/)
})

test("\u9996\u9875\u500d\u7387\u53d8\u52a8\u9690\u85cf\u65f6\u6e20\u9053\u4f59\u989d\u8d8b\u52bf\u94fa\u6ee1\u6574\u884c", () => {
  const page = read("../../app/page.tsx")
  const multiplier = read("../monitor/multiplier-changes.tsx")
  assert.match(page, /lg:only:col-span-5/)
  assert.match(page, /<MultiplierChanges[^>]*className="lg:col-span-2"/)
  assert.doesNotMatch(page, /<div className="lg:col-span-2"><MultiplierChanges/)
  assert.match(multiplier, /className\?: string/)
  assert.match(multiplier, /cn\([^\n]*className/)
})
