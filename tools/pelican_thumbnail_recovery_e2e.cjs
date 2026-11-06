const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')

async function main() {
  const browser = await chromium.launch({ executablePath: 'C:/Program Files/Google/Chrome/Application/chrome.exe', headless: true })
  try {
    const page = await browser.newPage({ viewport: { width: 1280, height: 900 } })
    await page.route('**/__thumbnail-recovery', route => route.fulfill({ contentType: 'text/html', body: '<!doctype html><html><body></body></html>' }))
    await page.route('**/api/**', route => route.abort())
    await page.goto('http://localhost:3010/__thumbnail-recovery')
    const reports = await page.evaluate(async () => {
      const renderer = await import('/lib/pelican-thumbnails.ts')
      const { loadPelicanThumbnail } = await import('/lib/pelican-thumbnail-store.ts')
      const reports = []
      async function inspect(image) {
        const bitmap = await createImageBitmap(image)
        const canvas = document.createElement('canvas')
        canvas.width = 448; canvas.height = 336
        const context = canvas.getContext('2d')
        context.fillStyle = 'white'; context.fillRect(0, 0, 448, 336)
        context.drawImage(bitmap, 0, 0, 448, 336); bitmap.close()
        const pixels = context.getImageData(0, 0, 448, 336).data
        let colored = 0
        for (let index = 0; index < pixels.length; index += 4) if (Math.min(pixels[index], pixels[index + 1], pixels[index + 2]) < 240) colored++
        return colored
      }
      const fixtures = {
        'delayed-canvas': '<html><body><canvas width="1024" height="768"></canvas><script>setTimeout(()=>{const c=document.querySelector("canvas").getContext("2d");c.fillStyle="#23794f";c.fillRect(100,100,800,500)},1800)</script></body></html>',
        'delayed-dom': '<html><body><script>setTimeout(()=>{const art=document.createElement("div");art.style.cssText="width:800px;height:500px;background:#e77439";document.body.append(art)},1800)</script></body></html>',
        'delayed-raf': '<html><body><canvas width="1024" height="768"></canvas><script>function draw(time){if(time<1800)return requestAnimationFrame(draw);const c=document.querySelector("canvas").getContext("2d");c.fillStyle="#9a4477";c.fillRect(100,100,800,500)}requestAnimationFrame(draw)</script></body></html>',
        'delayed-css': '<html><style>.art{width:800px;height:500px;background:#3467ac;opacity:0;animation:show .2s 1.8s forwards}@keyframes show{to{opacity:1}}</style><body><div class="art"></div></body></html>',
      }
      for (const [name, html] of Object.entries(fixtures)) {
        let image
        try { image = await renderer.renderPelicanThumbnail(html, new AbortController().signal) }
        catch (error) { throw new Error(name + ': ' + error.message) }
        const colored = await inspect(image)
        if (colored < 1000) throw new Error(name + ' captured a blank image')
        reports.push({ name, colored, bytes: image.size })
      }
      const html = '<svg viewBox="0 0 640 480"><rect width="640" height="480" fill="#428379"/></svg>'
      const canvas = document.createElement('canvas'); canvas.width = 448; canvas.height = 336
      const context = canvas.getContext('2d'); context.fillStyle = 'white'; context.fillRect(0, 0, 448, 336)
      const white = await new Promise(resolve => canvas.toBlob(resolve, 'image/jpeg'))
      const originalFetch = window.fetch
      for (const writable of [false, true]) {
        const methods = []
        let saved = null
        window.fetch = async (_url, options) => {
          methods.push(options?.method || 'GET')
          if (options?.method === 'PUT') { saved = options.body; return new Response(null, { status: 204 }) }
          return writable ? new Response(saved || white) : new Response(null, { status: 404 })
        }
        const image = await loadPelicanThumbnail(writable ? '/channels/1/pelican/results/1' : '/public/relay-stations/1/groups/2/pelican/results/1', 'test', new AbortController().signal,
          async signal => ({ image: await renderer.renderPelicanThumbnail(html, signal), html }), writable, renderer.verifyPelicanThumbnail)
        reports.push({ name: writable ? 'saved-white-recovery' : 'public-missing-recovery', colored: await inspect(image), bytes: image.size, methods })
      }
      window.fetch = originalFetch
      let rejected = false
      try { await renderer.renderPelicanThumbnail('<html><body></body></html>', new AbortController().signal) } catch { rejected = true }
      reports.push({ name: 'permanent-blank', rejected, renderers: document.querySelectorAll('iframe[data-pelican-renderer]').length })
      const controller = new AbortController()
      const pending = renderer.renderPelicanThumbnail(fixtures['delayed-canvas'], controller.signal)
      controller.abort(new DOMException('Aborted', 'AbortError'))
      try { await pending } catch {}
      reports.push({ name: 'cancelled', renderers: document.querySelectorAll('iframe[data-pelican-renderer]').length })
      return reports
    })
    console.log(JSON.stringify(reports))
    for (const report of reports.filter(item => 'colored' in item)) {
      assert.ok(report.colored > 1000, report.name + ' captured a blank image')
      assert.ok(report.bytes <= 96 * 1024, report.name + ' exceeds byte limit')
    }
    assert.deepEqual(reports.find(item => item.name === 'public-missing-recovery').methods, ['GET'])
    assert.deepEqual(reports.find(item => item.name === 'saved-white-recovery').methods, ['GET', 'PUT', 'GET'])
    assert.equal(reports.find(item => item.name === 'permanent-blank').rejected, true)
    assert.ok(reports.filter(item => 'renderers' in item).every(item => item.renderers === 0))
    const publicContext = await browser.newContext({ viewport: { width: 390, height: 1000 }, locale: 'zh-CN' })
    const publicPage = await publicContext.newPage(), errors = [], methods = []
    let bodies = 0
    publicPage.on('pageerror', error => errors.push(error.message))
    const defaults = vm.runInNewContext('(' + fs.readFileSync('frontend/lib/system-settings.ts', 'utf8').match(/export const defaultSystemSettings: SystemSettings = (\{[\s\S]*?\n\})/)[1] + ')', { defaultPelicanPrompt: 'test' })
    Object.assign(defaults, { pelican_page_enabled: true, pelican_tasks_enabled: false })
    const result = { id: 501, relay_station_id: 7, group_external_id: 2, model: 'recovery-test', reasoning_effort: 'medium', status: 'success', started_at: '2026-11-06T00:00:00Z', finished_at: '2026-11-06T00:01:00Z', duration_ms: 1000 }
    await publicPage.route('**/api/**', async route => {
      const url = new URL(route.request().url()), p = url.pathname
      assert.equal(route.request().method(), 'GET', 'public monitor must not upload thumbnails')
      let data = []
      if (p.endsWith('/pelican/results/501/thumbnail')) { methods.push('GET thumbnail'); return route.fulfill({ status: 404, body: '{}' }) }
      if (p.endsWith('/pelican/results/501')) { bodies++; data = { ...result, html: '<svg viewBox="0 0 640 480"><rect width="640" height="480" fill="#428379"/><circle cx="320" cy="240" r="110" fill="#f8cd56"/></svg>' } }
      else if (p === '/api/public/relay-stations') data = [{ id: 7, name: 'Recovery monitor' }]
      else if (p.endsWith('/group-pelican')) data = { page_enabled: true, configs: [], results: [result] }
      else if (p.endsWith('/group-monitor')) data = { station_id: 7, station_name: 'Recovery monitor', range: url.searchParams.get('range') || '24h', updated_at: result.finished_at, summary: { total: 1, available: 1, degraded: 0, unavailable: 0, idle: 0, disabled: 0, unknown: 0 }, groups: [{ external_id: 2, name: 'Recovery group', platform: 'newapi', rate_multiplier: 1, enabled: true, status: 'available', data_complete: true, updated_at: result.finished_at, success_count: 1, failure_count: 0, consecutive_failures: 0, calls: [], latency_buckets: [] }] }
      else if (p === '/api/auth/me') data = { auth_disabled: true, username: 'test' }
      else if (p === '/api/system-settings') data = defaults
      else if (p.endsWith('/branding')) data = { name: 'GatewayOps' }
      else if (p === '/api/version') data = { version: 'test', revision: 'unknown', channel: 'development' }
      await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ data }) })
    })
    await publicPage.goto('http://localhost:3010/public/relay-monitor/7')
    const card = publicPage.getByRole('button', { name: '\u67e5\u770b\u6d4b\u667a\u4f5c\u54c1 501', exact: true })
    await card.scrollIntoViewIfNeeded()
    await publicPage.locator('img[alt*="501"]').waitFor({ timeout: 15000 })
    await publicPage.waitForFunction(() => [...document.querySelectorAll('img[alt*="501"]')].every(image => image.complete && image.naturalWidth === 448))
    assert.equal(await publicPage.getByRole('dialog').count(), 0, 'thumbnail recovery must not open the HTML dialog')
    assert.equal(bodies, 1)
    assert.deepEqual(methods, ['GET thumbnail'])
    assert.deepEqual(errors, [])
    const screenshot = path.resolve('outputs/pelican-thumbnails/public-recovered-390.png')
    fs.mkdirSync(path.dirname(screenshot), { recursive: true })
    await publicPage.screenshot({ path: screenshot, fullPage: true })
    reports.push({ name: 'public-monitor-ui', bodies, methods, errors })
    await publicContext.close()
    const output = path.resolve('outputs/pelican-thumbnails/recovery.json')
    fs.mkdirSync(path.dirname(output), { recursive: true })
    fs.writeFileSync(output, JSON.stringify(reports, null, 2), 'utf8')
    console.log('PASS: thumbnail recovery and public monitor UI, ' + reports.length + ' cases')
  } finally { await browser.close() }
}
main().catch(error => { console.error(error); process.exit(1) })
