const assert = require("node:assert/strict")
const fs = require("node:fs")
const path = require("node:path")
const vm = require("node:vm")
const {chromium} = require(process.env.PLAYWRIGHT_MODULE || "playwright")

async function main() {
  const browser = await chromium.launch({executablePath: "C:/Program Files/Google/Chrome/Application/chrome.exe", headless: true})
  try {
    const page = await browser.newPage({viewport: {width: 1440, height: 1900}, locale: "zh-CN"})
    if (process.env.PELICAN_TEST_BUILT === "1") {
      await page.route("http://localhost:3010/pelican", route => route.fulfill({path: path.resolve("frontend/dist/index.html"), contentType: "text/html"}))
      await page.route("http://localhost:3010/assets/**", route => route.fulfill({path: path.resolve("frontend/dist", new URL(route.request().url()).pathname.slice(1))}))
    }
    const errors = [], calls = [], deleted = new Set(), failed = new Set()
    let refreshFails = false
    page.on("pageerror", error => errors.push(error.message))
    await page.addInitScript(() => {if (window.top === window) localStorage.setItem("uh_token", "local-batch-delete-test")})
    const defaults = vm.runInNewContext("(" + fs.readFileSync("frontend/lib/system-settings.ts", "utf8").match(/export const defaultSystemSettings: SystemSettings = (\{[\s\S]*?\n\})/)[1] + ")", {defaultPelicanPrompt: "test"})
    Object.assign(defaults, {pelican_page_enabled: true, pelican_tasks_enabled: false, pelican_history_count: 30})
    const result = (id, channelID, model, status = "success") => ({id, channel_id: channelID, relay_station_id: 4, account_external_id: 3, account_name: "测试账号", model, status, reasoning_effort: "medium", trigger: "manual", error: "", duration_ms: 1000, started_at: "2026-11-09T00:00:00Z", finished_at: "2026-11-09T00:01:00Z"})
    const channelResults = [result(7, 1, "alpha"), result(8, 1, "beta", "failed"), result(10, 1, "gamma"), result(9, 1, "alpha", "running")]
    channelResults[2].reasoning_effort = "low"
    const secondResult = result(7, 2, "alpha")
    const accountResult = result(7, 0, "alpha")
    const groupResult = {...result(7, 0, "alpha"), group_external_id: 6}
    const resultPath = r => r.channel_id ? `/channels/${r.channel_id}/pelican/results/${r.id}` : `/relay-stations/${r.relay_station_id}/${r.group_external_id ? "groups" : "accounts"}/${r.group_external_id || r.account_external_id}/pelican/results/${r.id}`
    const allResults = [...channelResults, secondResult, accountResult, groupResult]
    const retained = rows => rows.filter(r => !deleted.has(resultPath(r)))
    const thumbnail = Buffer.from(await page.evaluate(() => {
      const canvas = document.createElement("canvas")
      canvas.width = 448; canvas.height = 336
      const ctx = canvas.getContext("2d")
      ctx.fillStyle = "#a5d7db"; ctx.fillRect(0, 0, 448, 336)
      ctx.fillStyle = "#fbbf24"; ctx.beginPath(); ctx.arc(224, 160, 75, 0, Math.PI * 2); ctx.fill()
      return canvas.toDataURL("image/jpeg").split(",")[1]
    }), "base64")
    await page.route("**/api/**", async route => {
      const request = route.request(), p = new URL(request.url()).pathname
      let data = []
      if (p.endsWith("/thumbnail")) return route.fulfill({status: 200, contentType: "image/jpeg", body: thumbnail})
      if (/\/pelican\/results\/\d+$/.test(p)) {
        const target = p.slice(4)
        if (request.method() === "DELETE") {
          calls.push(target)
          await new Promise(resolve => setTimeout(resolve, 25))
          if (failed.has(target)) return route.fulfill({status: 500, contentType: "application/json", body: JSON.stringify({error: "fixture delete failure"})})
          deleted.add(target)
          return route.fulfill({status: 204})
        }
        data = {...allResults.find(r => resultPath(r) === target), html: "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 640 480\"><rect width=\"640\" height=\"480\" fill=\"skyblue\"/></svg>"}
      } else {
        assert.equal(request.method(), "GET", `Unexpected mutation: ${p}`)
        if (p === "/api/auth/me") data = {username: "test"}
        else if (p === "/api/system-settings") data = defaults
        else if (p === "/api/sync-settings") data = {sync_interval: 60}
        else if (p === "/api/version") data = {version: "test", channel: "development"}
        else if (p === "/api/channels/pelican") {
          if (refreshFails) return route.fulfill({status: 503, contentType: "application/json", body: JSON.stringify({error: "fixture refresh failure"})})
          data = {groups: [
            {channel: {id: 1, name: "渠道甲", type: "newapi", site_url: "https://example.invalid"}, targets: [], results: retained(channelResults)},
            {channel: {id: 2, name: "渠道乙", type: "newapi", site_url: "https://example.invalid"}, targets: [], results: retained([secondResult])},
          ]}
        } else if (p === "/api/relay-stations/pelican-sources") data = {groups: [
          {id: "pool:4:3", kind: "pool_account", name: "本地账号", station_id: 4, station_name: "测试站", site_url: "https://example.invalid", external_id: 3, config: {enabled: false}, results: retained([accountResult])},
          {id: "group:4:6", kind: "relay_group", name: "分组账号", station_id: 4, station_name: "测试站", site_url: "https://example.invalid", external_id: 6, config: {enabled: false}, results: retained([groupResult])},
        ]}
      }
      return route.fulfill({status: 200, contentType: "application/json", body: JSON.stringify({data})})
    })
    const toolbar = page.getByRole("toolbar", {name: "测智作品批量操作"})
    const artwork = r => page.locator(`[data-pelican-artwork="${resultPath(r)}"]`)
    const checkbox = r => artwork(r).getByRole("checkbox")
    const selectedCount = async count => {
      await page.waitForFunction(count => {
        const toolbar = document.querySelector("[role=toolbar]")
        return document.querySelectorAll("[data-pelican-artwork] [role=checkbox][data-state=checked]").length === count && toolbar?.getAttribute("aria-busy") === "false" && !toolbar.closest("[aria-hidden=true]") && !document.querySelector("[role=alertdialog]")
      }, count)
      assert.match(await toolbar.innerText(), new RegExp(`已选 ${count} 份`))
    }
    const clear = () => toolbar.getByRole("button", {name: "清空选择", exact: true}).click()
    const drag = async (from, to) => {
      await page.mouse.move(from.x, from.y)
      await page.mouse.down()
      await page.mouse.move(to.x, to.y, {steps: 12})
      await page.mouse.up()
    }
    await page.goto("http://localhost:3010/pelican")
    await checkbox(groupResult).waitFor()
    assert.equal(await page.getByRole("checkbox").count(), 7)
    assert.equal(await artwork(channelResults[3]).getByRole("checkbox").count(), 1)
    await artwork(channelResults[3]).getByRole("button", {name: "删除测智作品 9", exact:true}).click()
    await page.getByRole("alertdialog").getByRole("button", {name:"确认删除", exact:true}).click()
    await artwork(channelResults[3]).waitFor({state:"hidden"})
    assert.deepEqual(calls, [resultPath(channelResults[3])])
    deleted.clear(); calls.length=0
    await page.reload()
    await checkbox(channelResults[3]).waitFor()
    const card = await artwork(channelResults[0]).boundingBox(), check = await checkbox(channelResults[0]).boundingBox()
    assert.ok(check.x >= card.x && check.x < card.x + 40 && check.y >= card.y && check.y < card.y + 40)
    await checkbox(channelResults[0]).click()
    await checkbox(accountResult).click()
    await selectedCount(2)
    const refresh = page.waitForResponse(response => response.url().endsWith("/api/channels/pelican"))
    await page.getByRole("button", {name: "刷新作品列表"}).click()
    await refresh
    await selectedCount(2)
    await clear()

    const channelSection = page.locator('[data-pelican-channel="1"]')
    const accountSection = page.locator('[data-pelican-source="pool:4:3"]')
    const groupSection = page.locator('[data-pelican-source="group:4:6"]')
    const selectGroup = section => section.getByRole("button", {name:/全选当前筛选结果$/})
    const unselectGroup = section => section.getByRole("button", {name:/全不选当前筛选结果$/})
    await checkbox(secondResult).click()
    await channelSection.getByRole("group", {name:"渠道甲按模型筛选"}).getByRole("button", {name:"alpha", exact:true}).click()
    await selectGroup(channelSection).click()
    await selectedCount(3)
    assert.equal(await checkbox(channelResults[0]).getAttribute("data-state"), "checked")
    assert.equal(await checkbox(channelResults[3]).getAttribute("data-state"), "checked")
    assert.equal(await selectGroup(channelSection).isDisabled(), true)
    await unselectGroup(channelSection).click()
    await selectedCount(1)
    assert.equal(await checkbox(secondResult).getAttribute("data-state"), "checked")
    assert.equal(await unselectGroup(channelSection).isDisabled(), true)
    await selectGroup(accountSection).click()
    await selectGroup(groupSection).click()
    await selectedCount(3)
    await unselectGroup(accountSection).click()
    await selectedCount(2)
    assert.equal(await checkbox(groupResult).getAttribute("data-state"), "checked")
    await unselectGroup(groupSection).click()
    await selectedCount(1)
    await channelSection.getByRole("group", {name:"渠道甲按模型筛选"}).getByRole("button", {name:"全部", exact:true}).click()
    await selectGroup(channelSection).click()
    await selectedCount(5)
    await unselectGroup(channelSection).click()
    await selectedCount(1)
    await channelSection.getByRole("group", {name:"渠道甲按思考强度筛选"}).getByRole("button", {name:"低 - Low", exact:true}).click()
    await selectGroup(channelSection).click()
    await selectedCount(2)
    assert.equal(await checkbox(channelResults[2]).getAttribute("data-state"), "checked")
    await unselectGroup(channelSection).click()
    await selectedCount(1)
    await channelSection.getByRole("group", {name:"渠道甲按思考强度筛选"}).getByRole("button", {name:"全部", exact:true}).click()
    await clear()

    // 从图片区域起拖，框选后不能误触预览。
    const first = await artwork(channelResults[0]).boundingBox(), second = await artwork(channelResults[1]).boundingBox()
    // Escape 后保持按下并移动，松键不能打开预览；原选择应保留。
    await checkbox(channelResults[1]).click()
    await page.mouse.move(first.x + 60, first.y + 80)
    await page.mouse.down()
    await page.mouse.move(first.x + 100, first.y + 110, {steps: 4})
    await page.keyboard.press("Escape")
    await page.waitForTimeout(30)
    await page.mouse.move(first.x + 120, first.y + 120)
    await selectedCount(1)
    assert.equal(await checkbox(channelResults[1]).getAttribute("data-state"), "checked")
    assert.equal(await checkbox(channelResults[0]).getAttribute("data-state"), "unchecked")
    await page.mouse.up()
    assert.equal(await page.getByRole("dialog").count(), 0)
    await clear()

    // 合成失焦及外部松键后的移动，验证浏览器中的监听清理。
    const gestureTarget = artwork(channelResults[0]).locator("[data-pelican-preview]")
    const pointer = {pointerId: 61, pointerType: "mouse", button: 0, buttons: 1, clientX: first.x + 60, clientY: first.y + 80}
    await gestureTarget.dispatchEvent("pointerdown", pointer)
    await page.dispatchEvent("body", "pointermove", {...pointer, clientX: first.x + 120, clientY: first.y + 120})
    await selectedCount(1)
    await page.evaluate(() => window.dispatchEvent(new Event("blur")))
    await page.dispatchEvent("body", "pointermove", {...pointer, buttons: 0, clientX: second.x + 80, clientY: second.y + 100})
    await selectedCount(0)
    assert.equal(await page.locator('[aria-hidden="true"].border-brand').count(), 0)

    // 没有失焦事件时，左键状态也能兜底结束手势。
    await gestureTarget.dispatchEvent("pointerdown", {...pointer, pointerId: 62})
    await page.dispatchEvent("body", "pointermove", {...pointer, pointerId: 62, clientX: first.x + 120, clientY: first.y + 120})
    await selectedCount(1)
    await page.dispatchEvent("body", "pointermove", {...pointer, pointerId: 62, buttons: 0, clientX: second.x + 80})
    await selectedCount(0)
    assert.equal(await page.locator('[aria-hidden="true"].border-brand').count(), 0)

    await drag({x: first.x + 40, y: first.y + 60}, {x: second.x + second.width - 20, y: second.y + 150})
    await selectedCount(2)
    assert.equal(await page.getByRole("dialog").count(), 0)
    await clear()
    await drag({x: second.x + second.width - 20, y: second.y + 150}, {x: first.x + 40, y: first.y + 60})
    await selectedCount(2)
    await clear()
    await checkbox(channelResults[0]).click()
    const other = await artwork(secondResult).boundingBox()
    await page.keyboard.down("Control")
    await drag({x: other.x + 45, y: other.y + 70}, {x: other.x + 150, y: other.y + 140})
    await page.keyboard.up("Control")
    await selectedCount(2)
    await clear()

    await artwork(channelResults[0]).getByRole("button", {name: "查看测智作品 7", exact: true}).click()
    await page.getByRole("dialog").waitFor()
    await page.keyboard.press("Escape")
    await page.getByRole("dialog").waitFor({state: "hidden"})
    await checkbox(channelResults[1]).click()
    await page.locator("[data-pelican-channel=\"1\"]").getByRole("group", {name: "渠道甲按模型筛选"}).getByRole("button", {name: "alpha", exact: true}).click()
    await selectedCount(0)
    await page.locator("[data-pelican-channel=\"1\"]").getByRole("group", {name: "渠道甲按模型筛选"}).getByRole("button", {name: "全部", exact: true}).click()
    await checkbox(channelResults[0]).click()
    await checkbox(accountResult).click()
    await page.getByRole("textbox", {name: "搜索渠道或模型"}).fill("本地账号")
    await selectedCount(1)
    await page.getByRole("textbox", {name: "搜索渠道或模型"}).fill("")
    await clear()

    // 取消确认不发送删除请求；部分失败保留原来源的勾选，随后可以重试。
    await toolbar.getByRole("button", {name: "全选当前结果", exact: true}).click()
    await selectedCount(7)
    await toolbar.getByRole("button", {name: "批量删除 (7)", exact: true}).click()
    await page.getByRole("alertdialog").getByRole("button", {name: "取消", exact: true}).click()
    assert.deepEqual(calls, [])
    await selectedCount(7)
    failed.add(resultPath(groupResult))
    await toolbar.getByRole("button", {name: "批量删除 (7)", exact: true}).click()
    await page.getByRole("alertdialog").getByRole("button", {name: "确认批量删除", exact: true}).click()
    await selectedCount(1)
    assert.equal(calls.length, 7)
    assert.equal(new Set(calls).size, 7)
    assert.ok(calls.includes(resultPath(channelResults[3])))
    assert.equal(await page.getByRole("checkbox").count(), 1)
    assert.equal(await checkbox(groupResult).getAttribute("data-state"), "checked")
    failed.clear()
    await toolbar.getByRole("button", {name: "批量删除 (1)", exact: true}).click()
    await page.getByRole("alertdialog").getByRole("button", {name: "确认批量删除", exact: true}).click()
    await selectedCount(0)
    assert.equal(calls.filter(p => p === resultPath(groupResult)).length, 2)
    assert.equal(await page.getByRole("checkbox").count(), 0)

    // 刷新失败仍隐藏已删除作品；移动端横向滚动和触摸不触发框选。
    deleted.clear()
    await page.reload()
    await checkbox(groupResult).waitFor()
    await page.setViewportSize({width: 390, height: 844})
    await toolbar.scrollIntoViewIfNeeded()
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false)
    await selectGroup(channelSection).click()
    await selectedCount(4)
    await unselectGroup(channelSection).click()
    await selectedCount(0)
    await checkbox(channelResults[0]).focus()
    await page.keyboard.press("Space")
    await selectedCount(1)
    await clear()
    const mobile = await artwork(channelResults[0]).boundingBox()
    await artwork(channelResults[0]).dispatchEvent("pointerdown", {pointerId: 41, pointerType: "touch", button: 0, clientX: mobile.x + 50, clientY: mobile.y + 80})
    await page.dispatchEvent("body", "pointermove", {pointerId: 41, pointerType: "touch", clientX: 350, clientY: mobile.y + 150})
    await page.dispatchEvent("body", "pointerup", {pointerId: 41, pointerType: "touch", clientX: 350, clientY: mobile.y + 150})
    await selectedCount(0)
    const strip = artwork(channelResults[0]).locator("..")
    await strip.evaluate(element => {element.scrollLeft = element.scrollWidth})
    assert.ok(await strip.evaluate(element => element.scrollLeft > 0))
    await strip.evaluate(element => {element.scrollLeft = 0})
    await checkbox(channelResults[0]).click()
    fs.mkdirSync("outputs/pelican-batch-delete", {recursive: true})
    await page.screenshot({path: "outputs/pelican-batch-delete/mobile.png"})
    await page.setViewportSize({width: 1440, height: 1100})
    await toolbar.scrollIntoViewIfNeeded()
    await page.screenshot({path: "outputs/pelican-batch-delete/desktop.png"})
    refreshFails = true
    await toolbar.getByRole("button", {name: "批量删除 (1)", exact: true}).click()
    await page.getByRole("alertdialog").getByRole("button", {name: "确认批量删除", exact: true}).click()
    await selectedCount(0)
    assert.equal(await artwork(channelResults[0]).count(), 0)
    assert.deepEqual(errors, [])
    const report = {passed: true, groupSelection: true, filteredGroupSelection: true, groupSelectionIsolation: true, mobileGroupSelection: true, escapeDoesNotPreview: true, blurCancelsSelection: true, lostPointerUpCancelsSelection: true, checkboxTopLeft: true, crossSource: true, dragBothDirections: true, additiveDrag: true, preview: true, filterCleanup: true, refreshKeepsSelection: true, cancelDoesNotDelete: true, partialFailureRetry: true, runningSingleDelete: true, runningBatchDelete: true, keyboard: true, mobile: true, refreshFailureHidesDeleted: true, deleteRequests: calls.length, errors}
    fs.writeFileSync("outputs/pelican-batch-delete/report.json", JSON.stringify(report, null, 2), "utf8")
    console.log(JSON.stringify(report))
  } finally {await browser.close()}
}
main().catch(error => {console.error(error); process.exit(1)})
