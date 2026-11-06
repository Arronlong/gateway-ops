import assert from "node:assert/strict"
import { createRequire } from "node:module"
const require = createRequire(import.meta.url)
const { chromium } = require(process.env.PLAYWRIGHT_MODULE_PATH || "playwright")
const baseURL = process.env.GATEWAY_OPS_URL || "http://localhost:3010"

const fixture = `import React from "/node_modules/.vite/deps/react.js";
import ReactDOM from "/node_modules/.vite/deps/react-dom_client.js";
import "/app/globals.css";
import { ChannelFormDialog } from "/components/monitor/channel-form-dialog.tsx";
function Fixture(){
 const [open,setOpen]=React.useState(true);
 const [channel,setChannel]=React.useState({id:1,name:"first",type:"newapi",site_url:"http://upstream.invalid",balance_mode:"manual",manual_balance:12,accounts:[]});
 window.openChannel=async(id)=>{const response=await fetch("/api/channels/"+id);const body=await response.json();setChannel(body.data);setOpen(true)};
 return React.createElement(ChannelFormDialog,{open,onOpenChange:setOpen,channel});
}
ReactDOM.createRoot(document.getElementById("root")).render(React.createElement(Fixture));`

const browser = await chromium.launch({ headless: true, ...(process.env.CHROME_PATH ? { executablePath: process.env.CHROME_PATH } : {}) })
const page = await browser.newPage({ viewport: { width: 1280, height: 1000 } })
const errors = []
page.on("pageerror", (error) => errors.push(error.message))
const proxies = []
const channels = Object.fromEntries([1, 2].map((id) => [id, { id, name: `channel-${id}`, type: "newapi", site_url: "http://upstream.invalid", username: "monitor", credential_mode: "password", balance_mode: id === 1 ? "manual" : "auto", manual_balance: 12, accounts: [], proxy_id: null }]))
let createCount = 0
let saveCount = 0
const proxyURL = "http://user:p%40ss@127.0.0.1:7890"

try {
  await page.route("**/__channel_proxy_fixture.js*", (route) => route.fulfill({ contentType: "application/javascript", body: fixture }))
  await page.route("**/__channel_proxy_fixture", async (route) => {
    const response = await page.request.get(baseURL + "/")
    await route.fulfill({ contentType: "text/html", body: (await response.text()).replace("/src/main.tsx", "/__channel_proxy_fixture.js") })
  })
  await page.route("**/api/captcha-configs", (route) => route.fulfill({ json: { data: [] } }))
  await page.route("**/api/proxies", async (route) => {
    if (route.request().method() === "POST") {
      if (route.request().postDataJSON().url.startsWith("socks5:")) {
        await route.fulfill({ status: 400, json: { error: "代理地址必须是有效的 HTTP/HTTPS URL" } })
        return
      }
      createCount++
      assert.equal(route.request().postDataJSON().url, proxyURL)
      const item = { id: 7, display_url: "http://user:xxxxx@127.0.0.1:7890", created_at: "2026-11-06T00:00:00Z" }
      proxies.splice(0, proxies.length, item)
      await route.fulfill({ json: { data: item } })
    } else {
      await route.fulfill({ json: { data: [...proxies] } })
    }
  })
  await page.route("**/api/channels/*", async (route) => {
    const id = Number(new URL(route.request().url()).pathname.split("/").at(-1))
    if (route.request().method() === "PUT") {
      saveCount++
      Object.assign(channels[id], route.request().postDataJSON())
    }
    await route.fulfill({ json: { data: channels[id] } })
  })
  await page.goto(baseURL + "/__channel_proxy_fixture", { waitUntil: "networkidle" })
  const dialog = page.getByRole("dialog")
  const selector = page.getByRole("combobox", { name: "代理", exact: true })
  const save = dialog.getByRole("button", { name: "保存", exact: true })
  assert.match(await selector.innerText(), /不使用代理/u)
  await dialog.getByRole("button", { name: "新增代理", exact: true }).click()
  assert.equal(await save.isDisabled(), true)
  assert.equal(await dialog.getByRole("textbox", { name: "代理地址", exact: true }).count(), 1)
  await dialog.getByRole("textbox", { name: "代理地址", exact: true }).fill(proxyURL)
  await dialog.getByRole("textbox", { name: "代理地址", exact: true }).press("Enter")
  await page.waitForFunction(() => !document.querySelector("input[aria-label=代理地址]"))
  assert.equal(createCount, 1)
  assert.equal(saveCount, 0, "Enter in the proxy input must not submit the channel")
  await page.waitForFunction(() => document.getElementById("channel-proxy")?.textContent.includes("xxxxx"), undefined, { timeout: 5000 })
  assert.match(await selector.innerText(), /xxxxx@127\.0\.0\.1:7890/u)
  assert.equal((await dialog.innerText()).includes("p%40ss"), false)
  await save.click()
  await dialog.waitFor({ state: "hidden" })
  assert.equal(channels[1].proxy_id, 7)

  await page.evaluate(() => window.openChannel(2))
  await selector.click()
  await page.getByRole("option", { name: "http://user:xxxxx@127.0.0.1:7890", exact: true }).click()
  await save.click()
  await dialog.waitFor({ state: "hidden" })
  assert.equal(channels[2].proxy_id, 7)
  assert.equal(createCount, 1, "Second channel must reuse the existing global proxy")

  await page.evaluate(() => window.openChannel(1))
  await page.waitForFunction(() => document.getElementById("channel-proxy")?.textContent.includes("xxxxx"))
  assert.match(await selector.innerText(), /xxxxx/u)
  if (process.env.PROXY_SCREENSHOT_PATH) await dialog.screenshot({ path: process.env.PROXY_SCREENSHOT_PATH })
  await page.setViewportSize({ width: 390, height: 844 })
  assert.equal(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth + 1), true)
  if (process.env.PROXY_MOBILE_SCREENSHOT_PATH) await dialog.screenshot({ path: process.env.PROXY_MOBILE_SCREENSHOT_PATH })
  await selector.click()
  await page.getByRole("option", { name: "不使用代理", exact: true }).click()
  await save.click()
  await dialog.waitFor({ state: "hidden" })
  assert.equal(channels[1].proxy_id, null)
  assert.equal(channels[2].proxy_id, 7)
  await page.evaluate(() => window.openChannel(1))
  assert.match(await selector.innerText(), /不使用代理/u)
  await dialog.getByRole("button", { name: "新增代理", exact: true }).click()
  await dialog.getByRole("textbox", { name: "代理地址", exact: true }).fill("socks5://127.0.0.1:1080")
  await dialog.getByRole("button", { name: "添加并选用", exact: true }).click()
  await dialog.getByRole("alert").waitFor()
  assert.match(await dialog.getByRole("alert").innerText(), /HTTP\/HTTPS/u)
  assert.equal(await save.isDisabled(), true)
  await dialog.getByRole("button", { name: "取消", exact: true }).first().click()
  assert.equal(await save.isEnabled(), true)
  assert.equal(createCount, 1)
  assert.deepEqual(errors, [])
  console.log(JSON.stringify({ createCount, saveCount, crossChannelReuse: true, persistedSelection: true, cleared: true, mobileWidth: 390, errors }))
} finally {
  await browser.close()
}
