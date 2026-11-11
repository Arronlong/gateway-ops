const assert = require("node:assert/strict")
const fs = require("node:fs")
const path = require("node:path")
const vm = require("node:vm")
const {chromium} = require(process.env.PLAYWRIGHT_MODULE || "playwright")

const labels = {
  menu: "\u6253\u5f00\u83dc\u5355", navigation: "\u4e3b\u83dc\u5355", refresh: "\u5237\u65b0\u89c6\u56fe",
  collected: "\u4e0a\u6b21\u91c7\u96c6", logout: "\u9000\u51fa\u767b\u5f55", docs: "\u67e5\u770b\u6587\u6863",
}
const navigationLabels = ["\u9996\u9875", "\u6e20\u9053\u7ba1\u7406", "\u4e2d\u8f6c\u7ad9\u7ba1\u7406", "\u9e48\u9e55\u6d4b\u667a", "\u6210\u672c\u7ba1\u7406", "\u672c\u5730\u53f7\u6c60", "\u5206\u7ec4\u76d1\u63a7", "\u670d\u52a1\u72b6\u6001", "\u7cfb\u7edf\u8bbe\u7f6e", "\u9a8c\u8bc1\u7801\u670d\u52a1", "\u9884\u8b66\u4e2d\u5fc3"]

async function layout(page) {
  await page.evaluate(async () => {
    await document.fonts.ready
    await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))
  })
  return page.getByRole("banner").evaluate(header => {
    const rect = element => {
      const bounds = element.getBoundingClientRect()
      return {x: bounds.x, y: bounds.y, width: bounds.width, height: bounds.height, right: bounds.right, bottom: bounds.bottom}
    }
    const nav = header.querySelector("nav")
    const actions = [...header.querySelectorAll("a,button")].filter(element => element.getBoundingClientRect().width > 0 && getComputedStyle(element).visibility !== "hidden")
    const time = [...header.querySelectorAll("span")].find(element => element.textContent.startsWith("\u4e0a\u6b21\u91c7\u96c6"))
    return {
      height: rect(header).height, nav: rect(nav), menuRows: [...new Set([...nav.children].filter(element => element.getBoundingClientRect().width > 0 && getComputedStyle(element).visibility !== "hidden").map(element => Math.round(rect(element).y)))],
      time: rect(time), timeText: time.textContent, actions: actions.map(element => ({label: element.getAttribute("aria-label") || element.textContent, ...rect(element)})),
      viewport: window.innerWidth, pageWidth: document.documentElement.scrollWidth,
    }
  })
}

async function main() {
  const browser = await chromium.launch({executablePath: "C:/Program Files/Google/Chrome/Application/chrome.exe", headless: true})
  const output = path.resolve("outputs/monitor-header-layout")
  fs.mkdirSync(output, {recursive: true})
  const defaults = vm.runInNewContext("(" + fs.readFileSync("frontend/lib/system-settings.ts", "utf8").match(/export const defaultSystemSettings: SystemSettings = (\{[\s\S]*?\n\})/)[1] + ")", {defaultPelicanPrompt: "test"})
  Object.assign(defaults, {channels_page_enabled: true, operations_management_enabled: true, pelican_page_enabled: true})
  const reports = [], failures = [], errors = []
  let age = 30
  try {
    const page = await browser.newPage({viewport: {width: 1366, height: 900}, locale: "zh-CN"})
    page.on("pageerror", error => errors.push(error.message))
    await page.addInitScript(() => localStorage.setItem("uh_token", "local-header-layout-test"))
    await page.route("http://localhost:3010/pelican", route => route.fulfill({path: path.resolve("frontend/dist/index.html"), contentType: "text/html"}))
    await page.route("http://localhost:3010/assets/**", route => route.fulfill({path: path.resolve("frontend/dist", new URL(route.request().url()).pathname.slice(1))}))
    await page.route("**/api/**", route => {
      assert.equal(route.request().method(), "GET", "layout verification must not mutate real data")
      const pathname = new URL(route.request().url()).pathname
      let data = []
      if (pathname === "/api/auth/me") data = {username: "fixture", auth_disabled: false}
      else if (pathname === "/api/public/branding") data = {product_name: "GatewayOps", product_icon: ""}
      else if (pathname === "/api/system-settings") data = defaults
      else if (pathname === "/api/sync-settings") data = {channel_enabled: false}
      else if (pathname === "/api/channels") data = [{id: 1, last_balance_at: new Date(Date.now() - age * 1000).toISOString()}]
      else if (pathname === "/api/channels/pelican" || pathname === "/api/relay-stations/pelican-sources") data = {groups: []}
      else if (pathname === "/api/version") data = {version: "fixture", revision: "unknown", channel: "development"}
      return route.fulfill({contentType: "application/json", body: JSON.stringify({data})})
    })
    for (const [width, fontSize] of [[1872, 24], [1366, 20], [1280, 16], [1366, 16], [1440, 16], [1536, 16], [1920, 16], [1024, 16], [768, 16], [640, 16], [390, 16]]) {
      await page.setViewportSize({width, height: 900})
      const samples = []
      for (const [name, seconds, expected] of [["seconds", 30, "30 \u79d2\u524d"], ["minutes", 360, "6 \u5206\u949f\u524d"], ["long-minutes", 3540, "59 \u5206\u949f\u524d"], ["hours", 82800, "23 \u5c0f\u65f6\u524d"]]) {
        age = seconds
        if (samples.length === 0) {
          await page.goto("http://localhost:3010/pelican")
          await page.addStyleTag({content: `html { font-size: ${fontSize}px; }`})
        }
        else await page.getByRole("banner").getByRole("button", {name: labels.refresh, exact: true}).filter({visible: true}).click()
        await page.waitForFunction(expected => [...document.querySelectorAll("header span")].some(element => element.textContent === expected), expected)
        const state = await layout(page)
        samples.push({name, ...state})
        if (state.height > fontSize * 3.5 + 1) failures.push(`${width}/${fontSize}/${name}: header grew to ${state.height}px`)
        if (state.menuRows.length > 1) failures.push(`${width}/${name}: navigation wrapped onto ${state.menuRows.length} rows`)
        if (state.pageWidth > width) failures.push(`${width}/${name}: horizontal page overflow`)
        for (const action of state.actions) if (action.x < 0 || action.right > width + 1 || action.bottom > state.height) failures.push(`${width}/${name}: clipped action ${action.label}`)
        for (let index = 1; index < state.actions.length; index++) if (state.actions[index-1].right > state.actions[index].x + 1) failures.push(`${width}/${name}: overlapping actions`)
        if (name === "minutes") await page.getByRole("banner").screenshot({path: path.join(output, `${width}-${fontSize}-${process.env.HEADER_TEST_REPORT || "current"}.png`)})
      }
      if (width >= 640 && samples.some(sample => Math.abs(sample.time.width - samples[0].time.width) > 1)) failures.push(`${width}: timestamp width changes between seconds and minutes`)
      const compactMenu = page.getByRole("banner").getByRole("button", {name: labels.menu, exact: true})
      if (await compactMenu.isVisible()) {
        await compactMenu.click()
        for (const label of navigationLabels) await page.getByRole("menuitem", {name: label}).waitFor({state: "visible", timeout: 5000})
        await page.keyboard.press("Escape")
      } else if (!(await page.getByRole("navigation", {name: labels.navigation}).isVisible())) failures.push(`${width}: no accessible navigation`)
      if (width >= 640) for (const name of [labels.logout, labels.docs, "GitHub \u4ed3\u5e93", "\u5207\u6362\u4e3b\u9898"]) await page.getByRole("banner").getByLabel(name, {exact: true}).waitFor({state: "visible"})
      reports.push({width, fontSize, samples})
    }
    fs.writeFileSync(path.join(output, `${process.env.HEADER_TEST_REPORT || "current"}.json`), JSON.stringify({reports, failures, errors}, null, 2), "utf8")
    console.log(JSON.stringify({widths: reports.map(report => report.width), failures, errors}))
    assert.deepEqual(errors, [])
    assert.deepEqual(failures, [], "header must stay on one row when collection age changes")
  } finally {await browser.close()}
}

main().catch(error => {console.error(error); process.exitCode = 1})
