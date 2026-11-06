const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')

async function main() {
  const browser = await chromium.launch({ executablePath: 'C:/Program Files/Google/Chrome/Application/chrome.exe', headless: true })
  const output = path.resolve('outputs/channel-score')
  fs.mkdirSync(output, { recursive: true })
  const defaults = vm.runInNewContext('(' + fs.readFileSync('frontend/lib/system-settings.ts', 'utf8').match(/export const defaultSystemSettings: SystemSettings = (\{[\s\S]*?\n\})/)[1] + ')', { defaultPelicanPrompt: 'test' })
  Object.assign(defaults, { pelican_page_enabled: false, recent_metrics_window_hours: 24, recent_metrics_limit: 60 })
  const stamp = new Date().toISOString()
  const channels = [1, 2, 3].map(id => ({ id, name: 'Score fixture ' + id, site_url: 'https://example.invalid/' + id, type: id === 1 ? 'sub2api' : 'newapi', credential_mode: 'token', username: 'fixture', login_headers: [], login_params: [], balance_mode: 'auto', monitor_enabled: true, balance_threshold: 5, last_balance: 100, accounts: [], last_balance_at: stamp, created_at: stamp, updated_at: stamp }))
  const samples = id => id === 3 ? [] : Array.from({ length: 12 }, (_, i) => ({ channel_account_id: id, created_at: new Date(Date.now() - (i + 2) * 60000).toISOString(), success_known: id === 2, success: id === 2, first_token_ms: id === 1 ? 1000 : 30000, duration_ms: id === 1 ? 2000 : 180000, input_tokens: id === 1 ? 10 : 100, cache_read_tokens: id === 1 ? 90 : 100 }))
  const reports = []
  try {
    for (const [width, mode] of [[1440, 'gallery'], [1440, 'list'], [390, 'gallery']]) {
      const context = await browser.newContext({ viewport: { width, height: 1000 }, locale: 'zh-CN' })
      const page = await context.newPage(), errors = []
      page.on('pageerror', error => errors.push(error.message))
      await page.route('**/api/**', async route => {
        const p = new URL(route.request().url()).pathname
        assert.equal(route.request().method(), 'GET', 'score verification must not trigger collection')
        let data = []
        if (p === '/api/auth/me') data = { auth_disabled: true, username: 'fixture' }
        else if (p === '/api/system-settings') data = defaults
        else if (p.endsWith('/branding')) data = { product_name: 'GatewayOps', product_icon: '' }
        else if (p === '/api/version') data = { version: 'fixture', revision: 'unknown', channel: 'development' }
        else if (p === '/api/channels') data = channels
        else if (p === '/api/channels/latency-trends') data = channels.map(channel => ({ channel_id: channel.id, samples: samples(channel.id) }))
        else if (p === '/api/channels/associations') data = { links: [], accounts: [], stations: [] }
        else if (p.endsWith('/pelican')) data = { groups: [], configs: [], results: [] }
        else if (p.endsWith('/summary')) data = { channels: [], relay: [], recent_rate_changes: [], recent_notification_logs: [], recent_relay_adjustments: [] }
        return route.fulfill({ contentType: 'application/json', body: JSON.stringify({ data }) })
      })
      await page.goto('http://localhost:3010/channels')
      await page.getByRole('button', { name: mode === 'gallery' ? '\u753b\u518c' : '\u5217\u8868', exact: true }).click()
      const first = page.getByRole('link', { name: 'Score fixture 1', exact: true })
      await first.waitFor()
      await page.waitForFunction(() => document.body.textContent.includes('Score fixture 1') && document.body.textContent.includes('97'))
      const names = await page.getByRole('link', { name: /^Score fixture / }).allTextContents()
      assert.deepEqual(names, ['Score fixture 1', 'Score fixture 2', 'Score fixture 3'])
      const pageText = await page.locator('body').innerText()
      assert.match(pageText, /Score fixture 1[\s\S]*?97/)
      const titles = await page.locator('[title]').evaluateAll(elements => elements.map(element => element.getAttribute('title') || ''))
      const title = titles.find(value => value.includes('1/3')) || ''
      assert.match(title, /1\/3/)
      assert.doesNotMatch(title, /\u6210\u529f\u7387|\u8fde\u8d25/)
      assert.match(pageText, mode === 'gallery' ? /Score fixture 2[\s\S]*?\u8bc4\u5206 50/ : /Score fixture 2[\s\S]*?\n50\n/)
      assert.deepEqual(errors, [])
      await page.screenshot({ path: path.join(output, `${mode}-${width}.png`), fullPage: true })
      reports.push({ width, mode, order: names, scores: [97, 50, null], errors })
      await context.close()
    }
    fs.writeFileSync(path.join(output, 'ui-results.json'), JSON.stringify(reports, null, 2), 'utf8')
    console.log('PASS: ' + reports.length + ' score display and sorting cases')
  } finally { await browser.close() }
}
main().catch(error => { console.error(error); process.exit(1) })
