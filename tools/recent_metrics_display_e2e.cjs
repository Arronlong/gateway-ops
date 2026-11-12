const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')

async function main() {
  const browser = await chromium.launch({ executablePath: 'C:/Program Files/Google/Chrome/Application/chrome.exe', headless: true })
  const output = path.resolve('outputs/recent-metrics-display')
  fs.mkdirSync(output, { recursive: true })
  const defaults = vm.runInNewContext('(' + fs.readFileSync('frontend/lib/system-settings.ts', 'utf8').match(/export const defaultSystemSettings: SystemSettings = (\{[\s\S]*?\n\})/)[1] + ')', { defaultPelicanPrompt: 'test' })
  const windowHours = Number(process.env.RECENT_METRICS_TEST_HOURS || 24)
  const sampleLimit = Number(process.env.RECENT_METRICS_TEST_LIMIT || 60)
  Object.assign(defaults, { pelican_page_enabled: false, recent_metrics_window_hours: windowHours, recent_metrics_limit: sampleLimit })
  const stamp = new Date().toISOString()
  const samples = Array.from({ length: sampleLimit === 1000 ? 1000 : 2 }, (_, index) => ({ created_at: new Date(Date.now() - (index * 6 + 2) * 60000).toISOString(), model: 'text-fixture', success_known: true, success: index === 0, first_token_ms: 1000, duration_ms: 10000, input_tokens: 100, cache_read_tokens: 100, output_tokens: 400 }))
  const channel = { id: 1, name: 'Metrics Channel', site_url: 'https://example.invalid', type: 'newapi', credential_mode: 'token', username: 'fixture', login_headers: [], login_params: [], balance_mode: 'auto', monitor_enabled: true, balance_threshold: 5, last_balance: 100, accounts: [], created_at: stamp, updated_at: stamp }
  const station = { id: 1, name: 'Metrics Station', type: 'newapi', base_url: 'https://example.invalid', api_key_configured: true, auto_adjust_enabled: false, auto_adjust_no_profit_enabled: false, auto_priority_enabled: false, created_at: stamp }
  const account = { external_id: 1, name: 'Metrics Account', platform: 'openai', type: 'apikey', status: 'active', schedulable: true, concurrency: 1, current_concurrency: 0, priority: 1, pool_mode: false, pool_mode_retry_count: 0, latency_samples: samples, current_groups: [], unsafe_groups: [], no_profit_groups: [], downgrade_groups: [], suggested_group_ids: [], risk_state: 'protected', can_apply: false, usage_total_tokens: 1200, user_charge_amount: 1, account_cost_amount: 0.5 }
  const recentMetrics = { window_hours: windowHours, sample_limit: sampleLimit, sample_count: samples.length, latest_at: samples[0].created_at, cache_hit_rate: 0.5, success_rate: 0.5, success_count: 1, success_attempt_count: samples.length, cache_input_tokens: samples.length * 100, cache_read_tokens: samples.length * 100, cache_creation_tokens: 0, tps: 40, tps_score: 80, tps_output_tokens: samples.length * 400, tps_duration_seconds: samples.length * 10, tps_models: ['text-fixture'] }
  const group = { external_id: 1, name: 'Metrics Group', rate_multiplier: 1, enabled: true, status: 'available', data_complete: true, updated_at: stamp, latest_call_at: samples[0].created_at, success_count: 1, failure_count: 1, consecutive_failures: 0, calls: samples, latency_buckets: [], recent_metrics: recentMetrics }
  const reports = []
  try {
    for (const width of [1440, 768, 390]) {
      const context = await browser.newContext({ viewport: { width, height: 1100 }, locale: 'zh-CN' })
      const page = await context.newPage(), errors = []
      if (process.env.PELICAN_TEST_BUILT === '1') {
        await page.route('http://localhost:3010/**', route => {
          const pathname = new URL(route.request().url()).pathname
          if (pathname.startsWith('/assets/')) return route.fulfill({path:path.resolve('frontend/dist', pathname.slice(1))})
          if (route.request().resourceType() === 'document') return route.fulfill({path:path.resolve('frontend/dist/index.html'), contentType:'text/html'})
          return route.fallback()
        })
      }
      page.on('pageerror', error => errors.push(error.message))
      await page.route('**/api/**', async route => {
        const request = route.request(), p = new URL(request.url()).pathname
        assert.equal(request.method(), 'GET', 'display verification must not trigger collection')
        let data = []
        if (p === '/api/auth/me') data = { auth_disabled: true, username: 'fixture' }
        else if (p === '/api/system-settings') data = defaults
        else if (p.endsWith('/branding')) data = { product_name: 'GatewayOps', product_icon: '' }
        else if (p === '/api/version') data = { version: 'fixture', revision: 'unknown', channel: 'development' }
        else if (p === '/api/channels') data = [channel]
        else if (p === '/api/channels/latency-trends') data = [{ channel_id: 1, samples }]
        else if (p === '/api/relay-stations/latency-trends') data = [{ station_id: 1, account_id: 1, samples }]
        else if (p === '/api/channels/associations') data = { links: [], accounts: [], stations: [] }
        else if (p.endsWith('/pelican') || p.endsWith('/group-pelican')) data = { page_enabled: false, configs: [], results: [], groups: [] }
        else if (p.endsWith('/summary')) data = { channels: [], relay: [], recent_rate_changes: [], recent_notification_logs: [], recent_relay_adjustments: [] }
        else if (p === '/api/relay-stations' || p === '/api/public/relay-stations') data = [station]
        else if (p === '/api/relay-stations/1') data = { station, groups: [], accounts: [account], summary: { account_count: 1, risk_account_count: 0, no_profit_account_count: 0, no_safe_candidate_count: 0, unknown_cost_count: 0, auto_adjusted_count: 0 }, adjustments: [], monitor_channels: [], sync_available: true, range: 'today', usage_complete: true, usage_failed_accounts: 0 }
        else if (p === '/api/relay-stations/1/usage') data = { range: 'today', accounts: [], complete: true, failed_accounts: 0 }
        else if (p.endsWith('/group-monitor')) data = { station_id: 1, station_name: station.name, range: '24h', sync_status: 'ready', updated_at: stamp, groups: [group], summary: { total: 1, available: 1, degraded: 0, unavailable: 0, idle: 0, disabled: 0, unknown: 0 } }
        return route.fulfill({ contentType: 'application/json', body: JSON.stringify({ data }) })
      })
      for (const target of [{ name: 'channels', url: '/channels', modes: ['gallery', 'list'] }, { name: 'accounts', url: '/relay-stations', modes: ['gallery', 'list'] }, { name: 'monitor', url: '/public/relay-monitor/1', modes: ['default'] }]) {
        await page.goto('http://localhost:3010' + target.url)
        for (const mode of target.modes) {
          if (mode !== 'default') {
            const modeGroup = target.name === 'accounts' ? page.getByLabel('\u8d26\u53f7\u5217\u8868\u5c55\u793a\u6a21\u5f0f', { exact: true }) : page
            const name = mode === 'list' ? '\u5217\u8868' : target.name === 'accounts' ? '\u56fe\u518c' : '\u753b\u518c'
            await modeGroup.getByRole('button', { name, exact: true }).click({ timeout: 5000 }).catch(async error => {
              console.error(JSON.stringify({ target: target.name, url: page.url(), errors, body: (await page.locator('body').textContent()).slice(-3500) }))
              throw error
            })
          }
          const cache = page.getByRole('button', { name: new RegExp(`${windowHours}h \u7f13\u5b58\u547d\u4e2d\uff1a50\\.0%`) }).first()
          const tps = page.getByRole('button', { name: new RegExp(`${windowHours}h TPS\uff1a40 Token/s`) }).first()
          await cache.waitFor({ timeout: 5000 }).catch(async error => {
            console.error(JSON.stringify({ target: target.name, mode, width, buttons: await page.locator('button[aria-label]').evaluateAll(elements => elements.map(element => element.getAttribute('aria-label'))), body: (await page.locator('body').textContent()).slice(-2500) }))
            throw error
          })
          await tps.waitFor()
          const metrics = cache.locator('..')
          if (sampleLimit === 1000 && target.name !== 'monitor') assert.ok((await page.locator('body').innerText()).includes(`${samples.length}/${sampleLimit} 次`), 'all configured samples must be counted')
          assert.equal(await metrics.getByRole('button').count(), 2, 'only cache and TPS rings should remain')
          assert.equal(await page.getByRole('button', { name: /h \u91cd\u8bd5\u611f\u77e5\u6210\u529f\u7387/ }).count(), 0)
          await tps.scrollIntoViewIfNeeded()
          const a = await cache.boundingBox(), b = await tps.boundingBox(), bounds = await metrics.boundingBox()
          assert.ok(Math.abs(a.y - b.y) < 2, 'both rings should share a row')
          assert.ok(b.x + b.width <= bounds.x + bounds.width + 1, 'TPS must fit inside the metrics column')
          await tps.hover()
          const tooltip = page.getByRole('tooltip')
          await tooltip.waitFor()
          assert.ok((await tooltip.innerText()).includes('40 Token/s'))
          await page.keyboard.press('Escape')
          await page.mouse.move(0, 0)
          if (target.name !== 'monitor') {
            await page.getByRole('button', { name: /^\u6392\u5e8f/ }).click()
            assert.equal(await page.getByRole('menuitemradio', { name: '\u91cd\u8bd5\u611f\u77e5\u6210\u529f\u7387', exact: true }).count(), 0)
            await page.keyboard.press('Escape')
            await page.getByRole('menu').waitFor({ state: 'hidden' })
          }
          await page.screenshot({ path: path.join(output, `${target.name}-${mode}-${width}.png`) })
          reports.push({ target: target.name, mode, width, cache: '50%', tps: 40, ringCount: 2 })
        }
      }
      assert.deepEqual(errors, [])
      await context.close()
    }
    fs.writeFileSync(path.join(output, 'results.json'), JSON.stringify(reports, null, 2), 'utf8')
    console.log(JSON.stringify({ passed: true, cases: reports }))
  } finally { await browser.close() }
}
main().catch(error => { console.error(error); process.exitCode = 1 })
