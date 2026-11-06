const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')

const text = { entry: '\u91cd\u65b0\u62c9\u53d6\u5386\u53f2', pull: '\u91cd\u65b0\u62c9\u53d6', custom: '\u81ea\u5b9a\u4e49', list: '\u5217\u8868', gallery: '\u753b\u518c', close: '\u5173\u95ed', unknown: '\u6682\u65e0\u6cd5\u6838\u5bf9' }

async function main() {
  const browser = await chromium.launch({ executablePath: 'C:/Program Files/Google/Chrome/Application/chrome.exe', headless: true })
  const results = []
  const output = path.resolve('outputs/channel-usage-history')
  fs.mkdirSync(output, { recursive: true })
  for (const width of [1440, 390]) for (const mode of ['gallery', 'list']) {
    const context = await browser.newContext({ viewport: { width, height: 1000 }, locale: 'zh-CN', timezoneId: 'Asia/Shanghai' })
    const page = await context.newPage()
    const errors = []
    page.on('pageerror', error => errors.push(error.message))
    const defaults = vm.runInNewContext('(' + fs.readFileSync('frontend/lib/system-settings.ts', 'utf8').match(/export const defaultSystemSettings: SystemSettings = (\{[\s\S]*?\n\})/)[1] + ')', { defaultPelicanPrompt: 'test' })
    Object.assign(defaults, { channels_page_enabled: true, pelican_page_enabled: false })
    const account = { id: 2, username: 'account-a', is_primary: true, credential_mode: 'token', last_balance: 90, created_at: '2026-10-01T00:00:00Z', updated_at: '2026-10-01T00:00:00Z' }
    const channel = { id: 1, name: 'Test Channel', site_url: 'https://example.invalid', type: 'newapi', username: 'account-a', credential_mode: 'token', login_headers: [], login_params: [], balance_mode: 'auto', manual_balance: 0, turnstile_enabled: false, monitor_enabled: true, balance_threshold: 5, last_balance: 90, accounts: [account], created_at: account.created_at, updated_at: account.updated_at }
    let repaired = false, unknown = false, failNext = false, lastPull = null
    await page.route('**/api/**', async route => {
      const url = new URL(route.request().url()), p = url.pathname
      let data = []
      if (p.endsWith('/usage-history')) {
        lastPull = { method: route.request().method(), query: url.searchParams }
        const failed = failNext
        if (!failed) repaired = true
        const event = { stage: failed ? 'error' : 'done', ok: !failed, message: failed ? 'test upstream failed' : '\u5386\u53f2\u6570\u636e\u91cd\u65b0\u62c9\u53d6\u5b8c\u6210', time: new Date().toISOString() }
        return route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: ' + JSON.stringify(event) + '\n\n' })
      }
      if (p === '/api/auth/me') data = { auth_disabled: true, username: 'test' }
      else if (p === '/api/system-settings') data = defaults
      else if (p === '/api/sync-settings') data = { channel_configured: true, channel_enabled: true, channel_interval_minutes: 30 }
      else if (p === '/api/version') data = { name: 'gateway-ops', version: 'test', revision: 'unknown', build_time: 'unknown', channel: 'development' }
      else if (p.endsWith('/branding')) data = { name: 'GatewayOps' }
      else if (p === '/api/channels') data = [channel]
      else if (p === '/api/channels/associations') data = { links: [], accounts: [], stations: [] }
      else if (p === '/api/channels/metrics') data = [{ channel_id: 1, current_balance: 90, usage_cost_amount: 40, usage_request_count: repaired ? 15 : 12, cumulative_recharge_amount: 0, consumption_amount: 40 }]
      else if (p.endsWith('/usage-integrity')) data = { status: unknown ? 'unavailable' : repaired ? 'matched' : 'mismatch', local_count: repaired ? 15 : 12, upstream_count: unknown ? null : 15, since: url.searchParams.get('start') || '2026-10-22T00:00:00Z', until: url.searchParams.get('end') || '2026-10-24T16:00:00Z', checked_at: '2026-11-04T00:00:00Z', accounts: [{ account_id: 2, account_name: 'account-a', local_count: repaired ? 15 : 12, upstream_count: unknown ? null : 15, error: unknown ? 'total unavailable' : '' }] }
      else if (p.endsWith('/summary')) data = { range: 'today', total_channels: 1, active_channels: 1, failed_channels: 0, total_balance: 90, consumption_amount: 40, usage_cost_amount: 40, usage_request_count: 12, cumulative_recharge_amount: 0, user_charge_amount: 0, matched_account_count: 0, user_charge_complete: true, lowest_balance: null, channels: [], rate_change_count: 0, recent_rate_changes: [], recent_notification_logs: [], relay: [], recent_relay_adjustments: [] }
      else if (p.endsWith('/pelican')) data = { page_enabled: false, configs: [], results: [] }
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data }) })
    })
    await page.goto('http://localhost:3010/channels')
    await page.getByRole('button', { name: text[mode], exact: true }).click()
    if (mode === 'list') await page.getByRole('button', { name: '24 \u5c0f\u65f6', exact: true }).click()
    const entry = page.getByRole('button', { name: text.entry, exact: true })
    await entry.waitFor()
    await page.getByText(/12 \/ .*15/).first().waitFor()
    await entry.scrollIntoViewIfNeeded()
    const entryBounds = await entry.boundingBox()
    assert.ok(entryBounds.x >= 0 && entryBounds.x + entryBounds.width <= width + 1, 'history entry overflows')
    await entry.click()
    const dialog = page.getByRole('dialog')
    if (mode === 'list') assert.equal(await dialog.getByRole('button', { name: '24 \u5c0f\u65f6', exact: true }).getAttribute('aria-pressed'), 'true')
    await dialog.getByRole('button', { name: text.custom, exact: true }).click()
    const inputs = dialog.locator('input[type="datetime-local"]')
    assert.match(await inputs.nth(1).inputValue(), /T23:59$/)
    await inputs.nth(0).fill('2026-10-22T00:00')
    await inputs.nth(1).fill('2026-10-24T23:59')
    const box = await dialog.boundingBox()
    assert.ok(box.x >= 0 && box.x + box.width <= width + 1, 'history dialog overflows')
    for (const input of [inputs.nth(0), inputs.nth(1)]) { const b = await input.boundingBox(); assert.ok(b.x >= box.x && b.x + b.width <= box.x + box.width, 'history input overflows') }
    await dialog.getByRole('button', { name: text.pull, exact: true }).click()
    await dialog.getByText('\u6761\u6570\u4e00\u81f4\u00b715', { exact: true }).waitFor()
    assert.equal(lastPull.method, 'POST')
    assert.equal(lastPull.query.get('start'), '2026-10-21T16:00:00.000Z')
    assert.equal(lastPull.query.get('end'), '2026-10-24T16:00:00.000Z')
    await page.screenshot({ path: path.join(output, mode + '-' + width + '.png') })
    failNext = true
    await dialog.getByRole('button', { name: text.pull, exact: true }).click()
    await dialog.getByRole('alert').filter({ hasText: 'test upstream failed' }).waitFor()
    unknown = true
    await dialog.getByRole('button', { name: text.close, exact: true }).click()
    await entry.click()
    await dialog.getByText(text.unknown, { exact: true }).waitFor()
    assert.deepEqual(errors, [])
    results.push({ width, mode, repair: true, failure: true, unknown: true, pageErrors: errors.length })
    await context.close()
  }
  await browser.close()
  fs.writeFileSync(path.join(output, 'results.json'), JSON.stringify(results, null, 2), 'utf8')
  console.log('PASS: ' + results.length + ' viewport/layout cases with success, failure and unknown-count flows')
}
main().catch(error => { console.error(error); process.exit(1) })
