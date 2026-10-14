import assert from 'node:assert/strict'
import { createRequire } from 'node:module'

// 启动 Vite 后运行此脚本；可通过环境变量指定 Playwright 模块、Chrome 路径和站点地址。
const require = createRequire(import.meta.url)
const { chromium } = require(process.env.PLAYWRIGHT_MODULE_PATH || 'playwright')
const baseURL = process.env.GATEWAY_OPS_URL || 'http://127.0.0.1:3010'

async function main() {
  const browser = await chromium.launch({ headless: true, ...(process.env.CHROME_PATH ? { executablePath: process.env.CHROME_PATH } : {}) })
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } })
  const errors = []
  page.on('pageerror', error => errors.push(error.message))
  const code = `import React from '/node_modules/.vite/deps/react.js';
import ReactDOM from '/node_modules/.vite/deps/react-dom_client.js';
const { createRoot } = ReactDOM;
import { LatencyStabilityLane } from '/components/monitor/latency-stability-lane.tsx';
import '/app/globals.css';
const buckets = Array.from({length:60}, (_,i) => ({start:new Date(1735689600000+i*60000).toISOString(), end:new Date(1735689660000+i*60000).toISOString(), request_count:1, first_token_p50_ms:100, first_token_p95_ms:200, duration_p50_ms:1000, duration_p95_ms:2000, success_rate:1}));
function Fixture() {
 const [version, setVersion] = React.useState(0);
 window.updateCharts = () => setVersion(v => v+1);
 return React.createElement('div', null,
  React.createElement('form', {style:{position:'fixed',top:0,left:0,zIndex:100,background:'white',padding:12}},
   React.createElement('input',{id:'probe-first',defaultValue:''}),React.createElement('input',{id:'probe-second',defaultValue:''})),
  React.createElement('div',{style:{height:2000}}),
  ...Array.from({length:24},(_,i) => React.createElement('section',{key:i,id:'lane-'+i,style:{height:180}},
    React.createElement(LatencyStabilityLane,{buckets:buckets.map(b => ({...b,request_count:version+1})),recentSamples:i%2 ? buckets.map((b,id)=>({id,created_at:b.start,first_token_ms:100,duration_ms:1000,status_code:200,model:'probe',output_tokens:100})) : undefined,loadSamples:async()=>[],layout:'fill'}))));
}
createRoot(document.getElementById('root')).render(React.createElement(Fixture));`
  await page.route('**/__viewport_fixture.js*', route => route.fulfill({ contentType: 'application/javascript', body: code }))
  await page.route('**/__viewport_fixture', async route => {
    const response = await page.request.get(baseURL + '/')
    await route.fulfill({ contentType: 'text/html', body: (await response.text()).replace('/src/main.tsx', '/__viewport_fixture.js') })
  })
  try {
    await page.goto(baseURL + '/__viewport_fixture', { waitUntil: 'networkidle' })
    await page.locator('#probe-first').waitFor()
    const initialTriggers = await page.locator('[data-slot="tooltip-trigger"]').count()
    assert.equal(initialTriggers, 0, 'Offscreen chart cells must not mount tooltips')
    await page.locator('#lane-0').scrollIntoViewIfNeeded()
    await page.locator('#lane-0 [data-slot="tooltip-trigger"]').first().waitFor()
    const mountedTriggers = await page.locator('[data-slot="tooltip-trigger"]').count()
    assert.ok(mountedTriggers >= 60 && mountedTriggers < 300, `Visible charts must mount, bounded count: ${mountedTriggers}`)
    const firstCell = page.locator('#lane-0 [data-slot="tooltip-trigger"]').first()
    await firstCell.hover()
    await page.locator('[data-slot="tooltip-content"]').first().waitFor()
    await page.mouse.move(1, 1)
    await firstCell.click()
    await page.getByRole('dialog').waitFor()
    await page.keyboard.press('Escape')
    await page.getByRole('dialog').waitFor({ state: 'hidden' })
    await page.locator('#lane-1 [data-slot="tooltip-trigger"]').first().hover()
    await page.locator('[data-slot="tooltip-content"]').first().waitFor()
    await page.mouse.move(1, 1)
    await page.evaluate(() => window.scrollTo(0, 0))
    await page.locator('#probe-first').click()
    await page.evaluate(() => { window.updateCharts(); window.__tasks = []; new PerformanceObserver(list => window.__tasks.push(...list.getEntries().map(e => e.duration))).observe({type:'longtask'}) })
    await page.locator('#probe-first').pressSequentially('test-relay-input')
    const start = Date.now()
    await page.locator('#probe-second').click()
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))))
    const focusElapsed = Date.now() - start
    assert.ok(focusElapsed < 300, `Focus switch took ${focusElapsed} ms`)
    await page.evaluate(() => window.updateCharts())
    await page.locator('#lane-0').scrollIntoViewIfNeeded()
    await page.waitForFunction(() => document.querySelector('#lane-0 [data-slot="tooltip-trigger"]')?.getAttribute('aria-label')?.includes('3'))
    assert.deepEqual(errors, [])
    const result = { initialTriggers, mountedTriggers, focusElapsed, tasks: await page.evaluate(() => window.__tasks), errors }
    await page.addInitScript(() => { delete window.IntersectionObserver })
    await page.reload({ waitUntil: 'networkidle' })
    assert.equal(await page.locator('[data-slot="tooltip-trigger"]').count(), 1440, 'Charts must render when IntersectionObserver is unavailable')
    assert.deepEqual(errors, [])
    console.log(JSON.stringify(result))
  } catch (error) { console.log(JSON.stringify({ errors, text: (await page.locator('body').innerText()).slice(0, 400) })); throw error }
  finally { await browser.close() }
}
main().catch(error => { console.error(error); process.exitCode = 1 })
