const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')

async function main() {
  const defaults = vm.runInNewContext('(' + fs.readFileSync('frontend/lib/system-settings.ts', 'utf8').match(/export const defaultSystemSettings: SystemSettings = (\{[\s\S]*?\n\})/)[1] + ')', { defaultPelicanPrompt: 'test' })
  defaults.pelican_page_enabled = false
  const browser = await chromium.launch({ executablePath: 'C:/Program Files/Google/Chrome/Application/chrome.exe', headless: true })
  const cases = []
  const output = path.resolve('outputs/channel-statistics')
  fs.mkdirSync(output, { recursive: true })
  try {
    for (const width of [1440, 768, 390]) {
      const stamp = new Date().toISOString()
      const channels = [1, 2].map(id => ({ id, name: `Channel ${id}`, type: 'newapi', site_url: 'https://upstream.invalid', username: 'shared@example.com', credential_mode: 'password', balance_mode: 'auto', manual_balance: 0, monitor_enabled: true, exclude_from_statistics: false, balance_threshold: 0, last_balance: 100, accounts: [], login_headers: [], login_params: [], created_at: stamp, updated_at: stamp }))
      const context = await browser.newContext({ viewport: { width, height: 1000 }, locale: 'zh-CN' })
      const page = await context.newPage()
      const errors = [], writes = []
      page.on('pageerror', error => errors.push(error.message))
      await page.route('http://localhost:3010/**', route => {
        const p = new URL(route.request().url()).pathname
        if (p.startsWith('/assets/')) return route.fulfill({ path: path.resolve('frontend/dist', p.slice(1)) })
        if (route.request().resourceType() === 'document') return route.fulfill({ path: path.resolve('frontend/dist/index.html'), contentType: 'text/html' })
        return route.fallback()
      })
      await page.route('**/api/**', async route => {
        const request = route.request(), p = new URL(request.url()).pathname
        let data = []
        if (request.method() !== 'GET') {
          assert.ok((request.method() === 'POST' && p === '/api/channels') || (request.method() === 'PUT' && p === '/api/channels/1'), `unexpected write: ${p}`)
          const body = request.postDataJSON()
          writes.push({ method: request.method(), body })
          if (request.method() === 'PUT') Object.assign(channels[0], body)
          data = { ...body, id: request.method() === 'PUT' ? 1 : 3 }
        } else if (p === '/api/auth/me') data = { auth_disabled: true, username: 'fixture' }
        else if (p === '/api/system-settings') data = defaults
        else if (p.endsWith('/branding')) data = { product_name: 'GatewayOps', product_icon: '' }
        else if (p === '/api/version') data = { version: 'fixture', channel: 'development', revision: 'test' }
        else if (p === '/api/channels') data = channels
        else if (p === '/api/channels/associations') data = { links: [], stations: [] }
        else if (p === '/api/channels/metrics') data = channels.map(ch => ({ channel_id: ch.id, usage_cost_amount: 40, usage_request_count: 100, current_balance: 100, consumption_amount: 40 }))
        else if (p.endsWith('/summary')) data = { total_channels: 2, active_channels: 2, total_balance: 100, channels: [], relay: [], recent_rate_changes: [], recent_notification_logs: [], recent_relay_adjustments: [] }
        return route.fulfill({ contentType: 'application/json', body: JSON.stringify({ data }) })
      })
      await page.goto('http://localhost:3010/channels')
      await page.getByText('可能重复统计', { exact: true }).first().waitFor()
      await page.getByRole('button', { name: '新增', exact: true }).click()
      let dialog = page.getByRole('dialog')
      let toggle = dialog.getByRole('switch', { name: '计入汇总统计', exact: true })
      assert.equal(await toggle.getAttribute('aria-checked'), 'true', 'new channels participate by default')
      await dialog.getByRole('button', { name: '取消', exact: true }).click()
      await page.getByRole('button', { name: '渠道 / 账号', exact: true }).first().click()
      dialog = page.getByRole('dialog')
      toggle = dialog.getByRole('switch', { name: '计入汇总统计', exact: true })
      assert.equal(await toggle.getAttribute('aria-checked'), 'true')
      await dialog.getByRole('status').filter({ hasText: '汇总可能重复' }).waitFor()
      await toggle.scrollIntoViewIfNeeded()
      const box = await toggle.boundingBox()
      assert.ok(box && box.x >= 0 && box.x + box.width <= width, 'switch must fit on small screens')
      await toggle.click()
      assert.equal(await toggle.getAttribute('aria-checked'), 'false')
      await dialog.getByRole('button', { name: '保存', exact: true }).click()
      await dialog.waitFor({ state: 'hidden' })
      assert.equal(writes[0].body.exclude_from_statistics, true)
      assert.equal(writes[0].body.monitor_enabled, true, 'statistics exclusion must not stop monitoring')
      await page.getByText('不计入汇总', { exact: true }).waitFor()
      assert.equal(await page.getByText('可能重复统计', { exact: true }).count(), 0)
      await page.getByRole('button', { name: '列表', exact: true }).click()
      await page.getByText('不计入汇总', { exact: true }).waitFor()
      await page.getByRole('button', { name: '编辑渠道', exact: true }).first().click()
      dialog = page.getByRole('dialog')
      toggle = dialog.getByRole('switch', { name: '计入汇总统计', exact: true })
      assert.equal(await toggle.getAttribute('aria-checked'), 'false', 'reopening must preserve opt-out')
      await toggle.scrollIntoViewIfNeeded()
      await page.screenshot({ path: path.join(output, `statistics-${width}.png`) })
      await toggle.click()
      await dialog.getByRole('button', { name: '保存', exact: true }).click()
      await dialog.waitFor({ state: 'hidden' })
      assert.equal(writes[1].body.exclude_from_statistics, false)
      assert.equal(writes[1].body.monitor_enabled, true)
      assert.deepEqual(errors, [])
      cases.push({ width, defaultIncluded: true, editPersists: true, independentMonitoring: true, galleryAndList: true, duplicateReminder: true })
      await context.close()
    }
    console.log(JSON.stringify({ passed: true, cases }))
  } finally {
    await browser.close()
  }
}

main().catch(error => { console.error(error); process.exitCode = 1 })
