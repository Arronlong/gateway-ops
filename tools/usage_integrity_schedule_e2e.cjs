const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')

async function main() {
  const browser = await chromium.launch({ executablePath: 'C:/Program Files/Google/Chrome/Application/chrome.exe', headless: true })
  const output = path.resolve('outputs/usage-integrity-schedule')
  fs.mkdirSync(output, { recursive: true })
  const results = []
  try {
    for (const width of [1440, 390]) {
      const context = await browser.newContext({ viewport: { width, height: 1000 }, locale: 'zh-CN', timezoneId: 'America/Los_Angeles' })
      const page = await context.newPage()
      const errors = []
      page.on('pageerror', error => errors.push(error.message))
      const defaults = vm.runInNewContext('(' + fs.readFileSync('frontend/lib/system-settings.ts', 'utf8').match(/export const defaultSystemSettings: SystemSettings = (\{[\s\S]*?\n\})/)[1] + ')', { defaultPelicanPrompt: 'test' })
      Object.assign(defaults, { pelican_page_enabled: false })
      let settings = { group_monitor_interval_seconds: 30, channel_configured: true, channel_enabled: true, channel_interval_minutes: 30, channel_token_refresh_enabled: false, channel_token_refresh_interval_minutes: 30, channel_usage_integrity_enabled: true, channel_usage_integrity_range: '24h', channel_usage_integrity_time: '07:00', relay_rate_configured: true, relay_rate_enabled: false, relay_rate_interval_minutes: 1, relay_rate_interval_seconds: 10, relay_snapshot_configured: true, relay_snapshot_enabled: false, relay_snapshot_interval_minutes: 60, relay_snapshot_interval_seconds: 3600 }
      let automaticCountRequests = 0, manualRuns = 0, lastSaved = null, manualHistory = null
      const account = { id: 2, username: 'fixture-account', is_primary: true, credential_mode: 'token', created_at: '2026-10-01T00:00:00Z', updated_at: '2026-10-01T00:00:00Z' }
      const channel = { id: 1, name: 'Fixture Channel', site_url: 'https://example.invalid', type: 'newapi', username: 'fixture-account', credential_mode: 'token', login_headers: [], login_params: [], balance_mode: 'auto', manual_balance: 0, turnstile_enabled: false, monitor_enabled: true, balance_threshold: 5, last_balance: 90, accounts: [account], created_at: account.created_at, updated_at: account.updated_at }
      await page.route('**/api/**', async route => {
        const request = route.request(), url = new URL(request.url()), p = url.pathname
        let data = []
        if (p.endsWith('/usage-history')) {
          manualHistory = { method: request.method(), query: url.searchParams.toString() }
          return route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: ' + JSON.stringify({ stage: 'done', ok: true, message: 'fixture repair complete', time: new Date().toISOString() }) + '\n\n' })
        }
        if (p.endsWith('/usage-integrity')) automaticCountRequests++
        if (p === '/api/auth/me') data = { auth_disabled: true, username: 'fixture' }
        else if (p === '/api/system-settings') data = defaults
        else if (p === '/api/sync-settings') {
          if (request.method() === 'PUT') { lastSaved = request.postDataJSON(); settings = { ...settings, ...lastSaved } }
          data = settings
        }
        else if (p === '/api/tasks/channel_usage_integrity/run') { manualRuns++; return route.fulfill({ status: 202, contentType: 'application/json', body: JSON.stringify({ data: { accepted: true } }) }) }
        else if (p === '/api/tasks/channel_usage_integrity/runs') data = { runs: [], total: 0, succeeded: 0, failed: 0, running: 0, page: 1, page_size: 20 }
        else if (p === '/api/version') data = { name: 'gateway-ops', version: 'fixture', revision: 'unknown', build_time: 'unknown', channel: 'development' }
        else if (p.endsWith('/branding')) data = { product_name: 'GatewayOps', product_icon: '' }
        else if (p === '/api/channels') data = [channel]
        else if (p === '/api/channels/associations') data = { links: [], accounts: [], stations: [] }
        else if (p === '/api/channels/metrics') data = [{ channel_id: 1, current_balance: 90, usage_cost_amount: 40, usage_request_count: 12, cumulative_recharge_amount: 0, consumption_amount: 40 }]
        else if (p.endsWith('/summary')) data = { range: 'today', total_channels: 1, active_channels: 1, failed_channels: 0, total_balance: 90, consumption_amount: 40, usage_cost_amount: 40, usage_request_count: 12, cumulative_recharge_amount: 0, user_charge_amount: 0, matched_account_count: 0, user_charge_complete: true, lowest_balance: null, channels: [], rate_change_count: 0, recent_rate_changes: [], recent_notification_logs: [], relay: [], recent_relay_adjustments: [] }
        else if (p.endsWith('/pelican')) data = { page_enabled: false, configs: [], results: [] }
        return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data }) })
      })
      await page.goto('http://localhost:3010/settings#background-tasks')
      const range = page.getByRole('combobox', { name: '\u5168\u6e20\u9053\u7528\u91cf\u6838\u5bf9\u8303\u56f4' })
      const clock = page.getByLabel('\u5168\u6e20\u9053\u7528\u91cf\u6838\u5bf9\u5317\u4eac\u65f6\u95f4')
      await range.waitFor()
      const monitorFrequency = page.getByRole('combobox', { name: '\u5206\u7ec4\u76d1\u63a7\u8ba1\u7b97\u9891\u7387', exact: true })
      assert.match(await monitorFrequency.innerText(), /30/)
      await monitorFrequency.click()
      await page.getByRole('option', { name: '\u6bcf 2 \u5206\u949f', exact: true }).click()
      assert.match(await range.innerText(), /24/)
      assert.equal(await clock.inputValue(), '07:00')
      const toggle = page.getByRole('switch', { name: '\u542f\u7528\u5168\u6e20\u9053\u7528\u91cf\u6838\u5bf9\u4e0e\u4fee\u590d' })
      assert.equal(await toggle.getAttribute('aria-checked'), 'true')
      await range.click()
      for (const label of ['\u4eca\u5929', '24 \u5c0f\u65f6', '7 \u5929', 'All']) await page.getByRole('option', { name: label, exact: true }).waitFor()
      await page.getByRole('option', { name: '7 \u5929', exact: true }).click()
      await clock.fill('08:45')
      await page.getByRole('button', { name: '\u4fdd\u5b58\u4efb\u52a1\u8bbe\u7f6e', exact: true }).click()
      await page.waitForFunction(() => [...document.querySelectorAll('[data-sonner-toast]')].some(item => item.textContent.includes('\u540e\u53f0\u4efb\u52a1\u4e0e\u540c\u6b65\u8bbe\u7f6e\u5df2\u4fdd\u5b58')))
      assert.equal(lastSaved.group_monitor_interval_seconds, 120)
      assert.equal(lastSaved.channel_usage_integrity_range, '7d')
      assert.equal(lastSaved.channel_usage_integrity_time, '08:45')
      await toggle.click()
      assert.equal(await range.isDisabled(), false)
      await range.click()
      await page.getByRole('option', { name: 'All', exact: true }).click()
      await page.getByRole('button', { name: '\u4fdd\u5b58\u4efb\u52a1\u8bbe\u7f6e', exact: true }).click()
      await page.waitForResponse(response => response.url().includes('/api/sync-settings') && response.request().method() === 'GET')
      assert.equal(lastSaved.channel_usage_integrity_enabled, false)
      assert.equal(lastSaved.group_monitor_interval_seconds, 120)
      assert.equal(lastSaved.channel_usage_integrity_range, 'all')
      await page.getByRole('button', { name: '\u7acb\u5373\u6267\u884c', exact: true }).click()
      await page.waitForFunction(() => [...document.querySelectorAll('[data-sonner-toast]')].some(item => item.textContent.includes('\u5df2\u5f00\u59cb\u5168\u6e20\u9053\u7528\u91cf\u6838\u5bf9\u4e0e\u4fee\u590d')))
      assert.equal(manualRuns, 1)
      await range.scrollIntoViewIfNeeded()
      assert.ok(await range.evaluate(element => { const box = element.getBoundingClientRect(); return box.left >= 0 && box.right <= innerWidth }))
      await page.screenshot({ path: path.join(output, 'settings-' + width + '.png') })
      await page.getByRole('button', { name: '\u5168\u6e20\u9053\u7528\u91cf\u6838\u5bf9\u4e0e\u4fee\u590d\u67e5\u770b\u6267\u884c\u8bb0\u5f55' }).click()
      await page.getByRole('dialog').waitFor()
      await page.keyboard.press('Escape')
      for (const mode of ['\u753b\u518c', '\u5217\u8868']) {
        await page.goto('http://localhost:3010/channels')
        await page.getByRole('button', { name: mode, exact: true }).click()
        const entry = page.getByRole('button', { name: '\u91cd\u65b0\u62c9\u53d6\u5386\u53f2', exact: true })
        await entry.waitFor()
        assert.equal(automaticCountRequests, 0)
        await entry.click()
        const dialog = page.getByRole('dialog')
        await dialog.getByRole('button', { name: '\u5f00\u59cb\u6838\u5bf9\u5e76\u8865\u9f50', exact: true }).click()
        await page.waitForFunction(() => document.body.textContent.includes('fixture repair complete'))
        assert.equal(manualHistory.method, 'POST')
        assert.equal(automaticCountRequests, 0)
        await dialog.getByRole('button', { name: '\u5173\u95ed', exact: true }).click()
      }
      assert.deepEqual(errors, [])
      results.push({ width, defaultTime: '07:00', timezone: 'Asia/Shanghai', rangeSaved: 'all', manualWhileDisabled: true, galleryAndListManualRepair: true, automaticCountRequests, pageErrors: errors.length })
      await context.close()
    }
  } finally {
    await browser.close()
  }
  fs.writeFileSync(path.join(output, 'results.json'), JSON.stringify(results, null, 2), 'utf8')
  console.log(JSON.stringify({ passed: true, cases: results }))
}

main().catch(error => { console.error(error); process.exitCode = 1 })
