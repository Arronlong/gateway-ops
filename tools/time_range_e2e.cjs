const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')

const text = {
  custom: '\u81ea\u5b9a\u4e49',
  start: '\u5f00\u59cb\u65f6\u95f4',
  end: '\u7ed3\u675f\u65f6\u95f4',
  dashboard: '\u7edf\u8ba1\u65f6\u95f4\u8303\u56f4',
  channels: '\u6e20\u9053\u7edf\u8ba1\u65f6\u95f4\u8303\u56f4',
  costs: '\u6210\u672c\u7edf\u8ba1\u65f6\u95f4\u8303\u56f4',
  ledger: '\u8d26\u672c\u65f6\u95f4\u8303\u56f4',
  pool: '\u672c\u5730\u53f7\u6c60\u7edf\u8ba1\u65f6\u95f4\u8303\u56f4',
  relay: '\u4e2d\u8f6c\u7ad9\u7edf\u8ba1\u65f6\u95f4\u8303\u56f4',
  monitor: '\u5206\u7ec4\u76d1\u63a7\u65f6\u95f4\u8303\u56f4',
  alerts: '\u4e8b\u4ef6\u65f6\u95f4\u8303\u56f4',
}

async function main() {
  const browser = await chromium.launch({ executablePath: 'C:/Program Files/Google/Chrome/Application/chrome.exe', headless: true })
  const context = await browser.newContext({ viewport: { width: 1440, height: 1100 }, locale: 'zh-CN', timezoneId: 'Asia/Shanghai' })
  const page = await context.newPage()
  const errors = []
  const requests = []
  const results = []
  const output = path.resolve('outputs/time-range-ui')
  fs.mkdirSync(output, { recursive: true })
  page.on('pageerror', error => errors.push(error.message))
  const source = fs.readFileSync('frontend/lib/system-settings.ts', 'utf8')
  const defaults = vm.runInNewContext('(' + source.match(/export const defaultSystemSettings: SystemSettings = (\{[\s\S]*?\n\})/)[1] + ')', { defaultPelicanPrompt: 'test' })
  Object.assign(defaults, { channels_page_enabled: true, pelican_page_enabled: false })
  const station = { id: 1, name: 'Test Station', type: 'sub2api', base_url: 'https://example.invalid', api_key_configured: true, auto_adjust_enabled: false, auto_adjust_no_profit_enabled: false, auto_priority_enabled: false, created_at: new Date().toISOString() }
  const summary = range => ({ range, total_channels: 0, active_channels: 0, failed_channels: 0, total_balance: 0, consumption_amount: 0, usage_cost_amount: 0, usage_request_count: 0, cumulative_recharge_amount: 0, user_charge_amount: 0, matched_account_count: 0, user_charge_complete: true, lowest_balance: null, channels: [], rate_change_count: 0, recent_rate_changes: [], recent_notification_logs: [], relay: [], recent_relay_adjustments: [] })
  const operation = range => ({ range, ledger: { income_amount: 0, expense_amount: 0, net_amount: 0, account_purchase_amount: 0, upstream_recharge_amount: 0, entry_count: 0, relay_revenue_amount: 0 }, breakdown: [], local_pool: { total_count: 0 }, cost: { relay_revenue_amount: 0, relay_call_cost_amount: 0, relay_account_purchase_amount: 0, relay_upstream_recharge_amount: 0, relay_actual_cost_amount: 0, relay_cost_complete: true, relay_call_cost_complete: true, relay_failed_accounts: 0, relay_unknown_cost_accounts: 0, channel_balance_amount: 0, channel_recharge_amount: 0, channel_balance_decrease_amount: 0, channel_cost_variance: 0, relay_stations: [] } })
  await page.route('**/api/**', async route => {
    const url = new URL(route.request().url())
    requests.push(url)
    const p = url.pathname
    const range = url.searchParams.get('range') || 'today'
    let data = []
    if (p === '/api/auth/me') data = { auth_disabled: true, username: 'test' }
    else if (p === '/api/system-settings') data = defaults
    else if (p === '/api/sync-settings') data = { channel_configured: true, channel_enabled: true, channel_interval_minutes: 30, relay_rate_configured: true, relay_rate_enabled: false, relay_rate_interval_minutes: 1, relay_rate_interval_seconds: 10, relay_snapshot_configured: true, relay_snapshot_enabled: true, relay_snapshot_interval_minutes: 60, relay_snapshot_interval_seconds: 3600 }
    else if (p === '/api/version') data = { name: 'gateway-ops', version: 'test', revision: 'unknown', build_time: 'unknown', channel: 'development' }
    else if (p === '/api/branding' || p === '/api/public/branding') data = { name: 'GatewayOps' }
    else if (p === '/api/dashboard/summary' || p === '/api/channels/summary') data = summary(range)
    else if (p === '/api/channels/associations') data = { links: [], accounts: [], stations: [] }
    else if (p === '/api/operations/summary') data = operation(range)
    else if (p === '/api/relay-stations' || p === '/api/public/relay-stations') data = [station]
    else if (p === '/api/relay-stations/1') data = { station, groups: [], accounts: [], summary: { total_accounts: 0 }, adjustments: [], monitor_channels: [], sync_available: true, range, usage_complete: true, usage_failed_accounts: 0 }
    else if (p === '/api/relay-stations/1/usage') data = { range, accounts: [], complete: true, failed_accounts: 0 }
    else if (p === '/api/relay-stations/1/users') data = { range, users: [], items: [], total: 0, page: 1, page_size: 50, pages: 0, complete: true }
    else if (p === '/api/public/relay-stations/1/group-monitor') data = { station_id: 1, station_name: station.name, range, updated_at: new Date().toISOString(), groups: [], summary: { total: 0, available: 0, degraded: 0, unavailable: 0, idle: 0, disabled: 0, unknown: 0 } }
    else if (p.endsWith('/group-pelican') || p.endsWith('/pelican')) data = { page_enabled: false, configs: [], results: [] }
    else if (p === '/api/alerts/summary') data = { open_events: 0, enabled_rules: 0, enabled_channels: 0, actions_today: 0 }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data }) })
  })
  const cases = [
    { slug: 'dashboard', path: '/', label: text.dashboard, endpoint: '/api/dashboard/summary' },
    { slug: 'channels', path: '/channels', label: text.channels, endpoint: '/api/channels/summary' },
    { slug: 'costs', path: '/operations/costs', label: text.costs, endpoint: '/api/operations/summary' },
    { slug: 'ledger', path: '/operations/costs', label: text.ledger, endpoint: '/api/operations/ledger' },
    { slug: 'pool', path: '/operations/local-pool', label: text.pool, endpoint: '/api/relay-stations/1/usage' },
    { slug: 'relay', path: '/relay-stations', label: text.relay, endpoint: '/api/relay-stations/1/usage' },
    { slug: 'monitor', path: '/public/relay-monitor/1', label: text.monitor, endpoint: '/api/public/relay-stations/1/group-monitor' },
    { slug: 'alerts', path: '/alerts', label: text.alerts, endpoint: '/api/alerts/events', alert: true },
  ]
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 1100 })
    for (const item of cases) {
      await page.goto('http://localhost:3010' + item.path)
      const group = page.getByRole('group', { name: item.label, exact: true })
      await group.getByRole('button', { name: text.custom, exact: true }).click()
      const start = page.locator('input[id="' + item.label + '-start"]')
      const end = page.locator('input[id="' + item.label + '-end"]')
      assert.match(await end.inputValue(), /T23:59$/)
      await start.fill('2026-10-22T00:00')
      const requestCount = requests.length
      await end.fill('2026-10-24T23:59')
      await page.waitForFunction(() => document.querySelectorAll('input[type="datetime-local"]').length >= 2)
      await page.waitForTimeout(300)
      const requested = requests.slice(requestCount).find(url => url.pathname === item.endpoint && (item.alert ? url.searchParams.get('to') === '2026-10-24T15:59:59.999Z' : url.searchParams.get('end') === '2026-10-24T16:00:00.000Z'))
      assert.ok(requested, item.slug + ' did not request custom bounds: ' + requests.slice(requestCount).map(url => url.pathname + url.search).join(', '))
      assert.equal(requested.searchParams.get(item.alert ? 'from' : 'start'), '2026-10-21T16:00:00.000Z')
      const bounds = await group.boundingBox()
      assert.ok(bounds.x >= 0 && bounds.x + bounds.width <= width + 1, item.slug + ' buttons overflow at ' + width)
      for (const input of [start, end]) {
        const box = await input.boundingBox()
        assert.ok(box.x >= 0 && box.x + box.width <= width + 1, item.slug + ' date inputs overflow at ' + width)
      }
      await group.scrollIntoViewIfNeeded()
      await page.screenshot({ path: path.join(output, item.slug + '-' + width + '.png') })
      results.push({ page: item.slug, width, endpoint: requested.pathname, bounds: requested.search })
    }
  }
  assert.deepEqual(errors, [])
  fs.writeFileSync(path.join(output, 'results.json'), JSON.stringify({ passed: true, cases: results, errors }, null, 2), 'utf8')
  console.log('PASS: ' + results.length + ' page/viewport cases; screenshots=' + output)
  await browser.close()
}
main().catch(error => { console.error(error); process.exit(1) })
