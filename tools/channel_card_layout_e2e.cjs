const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')

const labels = ['\u7ad9\u70b9', '\u8d26\u53f7', '\u4f59\u989d', '\u533a\u95f4\u4f59\u989d\u6d88\u8017', '\u7d2f\u8ba1\u5145\u503c', '\u533a\u95f4\u8c03\u7528\u6d88\u8017', '\u72b6\u6001', '\u4e0a\u6b21\u66f4\u65b0']
const historyLabel = '\u91cd\u65b0\u62c9\u53d6\u5386\u53f2'

async function main() {
  const browser = await chromium.launch({ executablePath: 'C:/Program Files/Google/Chrome/Application/chrome.exe', headless: true })
  const output = path.resolve('outputs/channel-card-layout')
  fs.mkdirSync(output, { recursive: true })
  const results = []
  try {
    for (const width of [1920, 1440, 768, 390]) {
      const context = await browser.newContext({ viewport: { width, height: 1100 }, locale: 'zh-CN', hasTouch: width <= 768 })
      const page = await context.newPage()
      const errors = []
      page.on('pageerror', error => errors.push(error.message))
      const defaults = vm.runInNewContext('(' + fs.readFileSync('frontend/lib/system-settings.ts', 'utf8').match(/export const defaultSystemSettings: SystemSettings = (\{[\s\S]*?\n\})/)[1] + ')', { defaultPelicanPrompt: 'test' })
      defaults.pelican_page_enabled = false
      let automaticCountRequests = 0, manualHistoryRuns = 0
      const account = { id: 2, username: 'kouq1@qq', is_primary: true, credential_mode: 'token', last_balance: 50.28, created_at: '2026-10-01T00:00:00Z', updated_at: new Date().toISOString() }
      let channel = { id: 1, name: 'Fixture Channel', site_url: 'https://cdn.coderelay.cn', type: 'newapi', username: account.username, credential_mode: 'token', login_headers: [], login_params: [], balance_mode: 'auto', manual_balance: 50.28, monitor_enabled: true, balance_threshold: 5, last_balance: 50.28, remark: 'fixture hidden remark', accounts: [account], created_at: account.created_at, updated_at: account.updated_at }
      await page.route('**/api/**', async route => {
        const request = route.request(), url = new URL(request.url()), p = url.pathname
        let data = []
        if (p.endsWith('/usage-history')) {
          assert.equal(request.method(), 'POST')
          manualHistoryRuns++
          return route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: ' + JSON.stringify({ stage: 'done', ok: true, message: 'fixture repair complete', time: new Date().toISOString() }) + '\n\n' })
        }
        if (p.endsWith('/usage-integrity')) automaticCountRequests++
        if (p === '/api/auth/me') data = { auth_disabled: true, username: 'fixture' }
        else if (p === '/api/system-settings') data = defaults
        else if (p === '/api/version') data = { name: 'gateway-ops', version: 'fixture', revision: 'unknown', build_time: 'unknown', channel: 'development' }
        else if (p.endsWith('/branding')) data = { product_name: 'GatewayOps', product_icon: '' }
        else if (p === '/api/channels') data = [channel]
        else if (p === '/api/channels/associations') data = { links: [], accounts: [], stations: [] }
        else if (p === '/api/channels/metrics') data = [{ channel_id: 1, current_balance: 50.28, usage_cost_amount: 3.364906, usage_request_count: 60, cumulative_recharge_amount: 40, consumption_amount: 3.42 }]
        else if (p.endsWith('/summary')) data = { range: 'today', total_channels: 1, active_channels: 1, failed_channels: 0, total_balance: 50.28, consumption_amount: 3.42, usage_cost_amount: 3.364906, usage_request_count: 60, cumulative_recharge_amount: 40, user_charge_amount: 0, matched_account_count: 0, user_charge_complete: true, lowest_balance: null, channels: [], rate_change_count: 0, recent_rate_changes: [], recent_notification_logs: [], relay: [], recent_relay_adjustments: [] }
        else if (p.endsWith('/pelican')) data = { page_enabled: false, configs: [], results: [] }
        return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data }) })
      })
      for (const variant of ['auto', 'manual', 'multi', 'error']) {
        channel = { ...channel, balance_mode: variant === 'manual' ? 'manual' : 'auto', last_error: variant === 'error' ? 'fixture collection error' : '', accounts: variant === 'multi' ? [account, { ...account, id: 3, username: 'secondary@example.invalid', is_primary: false }] : [account] }
        await page.goto('http://localhost:3010/channels')
        await page.getByRole('button', { name: '\u753b\u518c', exact: true }).click()
        const card = page.getByRole('link', { name: channel.name, exact: true }).locator('xpath=ancestor::*[@data-slot="card"][1]')
        const properties = card.getByText(labels[0], { exact: true }).locator('..').locator('..')
        await properties.waitFor()
        assert.deepEqual(await properties.locator(':scope > div > span:first-child').allTextContents(), labels)
        const boxes = await properties.locator(':scope > div').evaluateAll(rows => rows.map(row => { const r = row.getBoundingClientRect(); return { x: r.x, y: r.y, right: r.right, bottom: r.bottom } }))
        for (let i = 0; i < 8; i += 2) {
          assert.ok(Math.abs(boxes[i].y - boxes[i + 1].y) < 1, 'property pairs must share a row')
          assert.ok(boxes[i].right <= boxes[i + 1].x + 1, 'property pairs must have two columns')
          if (i > 0) assert.ok(boxes[i].y >= boxes[i - 2].bottom, 'property rows must follow the requested order')
        }
        if (variant === 'auto') {
          await page.waitForFunction(() => document.body.textContent.includes('$3.364906'))
          for (const value of ['$50.28', '$3.42', '\u00a540.00', '$3.364906']) assert.ok((await properties.innerText()).includes(value), 'missing property value: ' + value)
        }
        const entry = card.getByRole('button', { name: historyLabel, exact: true })
        if (variant === 'manual') {
          assert.equal(await entry.count(), 0)
        } else {
          await entry.waitFor()
          const buttonBox = await entry.boundingBox()
          const tpsBox = await card.getByRole('button', { name: /h TPS/ }).boundingBox()
          assert.ok(buttonBox.x >= 0 && buttonBox.x + buttonBox.width <= width, 'history button must fit the viewport')
          const metricsBox = await card.getByRole('button', { name: /h TPS/ }).locator('..').boundingBox()
          const rowBox = await entry.locator('..').locator('..').boundingBox()
          if (metricsBox.width + buttonBox.width + 8 <= rowBox.width + 1) {
            assert.ok(buttonBox.x > tpsBox.x + tpsBox.width, 'history button must sit right of the metrics')
            assert.ok(Math.abs(buttonBox.y + buttonBox.height / 2 - metricsBox.y - metricsBox.height / 2) < 1, 'history button must be vertically centered with the metrics')
          } else {
            assert.ok(buttonBox.y >= metricsBox.y + metricsBox.height, 'history button must wrap below the metrics when space is insufficient')
          }
          assert.ok(Math.abs(buttonBox.x + buttonBox.width - rowBox.x - rowBox.width) < 1, 'history button must stay aligned to the right')
          if (variant === 'error') {
            const status = properties.getByRole('button', { name: '\u67e5\u770b\u7ba1\u7406\u91c7\u96c6\u5f02\u5e38', exact: true })
            if (width <= 768) await status.tap()
            else { await status.focus(); await status.press('Enter') }
            const detail = page.getByRole('dialog', { name: '\u7ba1\u7406\u91c7\u96c6\u5f02\u5e38', exact: true })
            await detail.getByText(channel.last_error, { exact: true }).waitFor()
            await page.keyboard.press('Escape')
          }
        }
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false)
        if (variant === 'auto' || variant === 'manual') await card.screenshot({ path: path.join(output, variant + '-' + width + '.png') })
        if (variant === 'auto') {
          for (const mode of ['\u753b\u518c', '\u5217\u8868']) {
            await page.getByRole('button', { name: mode, exact: true }).click()
            await page.getByRole('button', { name: historyLabel, exact: true }).click()
            const dialog = page.getByRole('dialog')
            await dialog.getByRole('button', { name: '\u5f00\u59cb\u6838\u5bf9\u5e76\u8865\u9f50', exact: true }).click()
            await dialog.getByText('fixture repair complete', { exact: true }).waitFor()
            await dialog.getByRole('button', { name: '\u5173\u95ed', exact: true }).click()
          }
        }
        results.push({ width, variant, propertyCount: boxes.length })
      }
      assert.equal(manualHistoryRuns, 2)
      assert.equal(automaticCountRequests, 0)
      assert.deepEqual(errors, [])
      await context.close()
    }
  } finally {
    await browser.close()
  }
  fs.writeFileSync(path.join(output, 'results.json'), JSON.stringify({ passed: true, cases: results }, null, 2), 'utf8')
  console.log(JSON.stringify({ passed: true, cases: results }))
}

main().catch(error => { console.error(error); process.exitCode = 1 })
