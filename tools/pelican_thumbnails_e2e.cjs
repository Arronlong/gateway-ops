const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')

async function main() {
  const browser = await chromium.launch({ executablePath: 'C:/Program Files/Google/Chrome/Application/chrome.exe', headless: true })
  const defaults = vm.runInNewContext('(' + fs.readFileSync('frontend/lib/system-settings.ts', 'utf8').match(/export const defaultSystemSettings: SystemSettings = (\{[\s\S]*?\n\})/)[1] + ')', { defaultPelicanPrompt: 'test' })
  Object.assign(defaults, { pelican_page_enabled: true, pelican_tasks_enabled: false, pelican_history_count: 4 })
  const artwork = [
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 480"><rect width="640" height="480" fill="#7ad7ef"/><circle cx="320" cy="240" r="100" fill="#ffe789"/><text x="170" y="420" font-size="40">SVG preview</text></svg>',
    '<!doctype html><html><style>body{margin:0;background:#20304a;display:grid;place-items:center;height:768px}.shape{width:240px;height:240px;background:#fc8843;border-radius:50%;animation:spin 3s infinite}@keyframes spin{to{transform:translateX(100px)}}</style><body><div class="shape"></div></body></html>',
    '<!doctype html><html><body style="margin:0;background:#e8f3db"><canvas width="1024" height="768"></canvas><script>const c=document.querySelector("canvas").getContext("2d");c.fillStyle="#e8f3db";c.fillRect(0,0,1024,768);c.fillStyle="#28794e";c.fillRect(250,180,500,400);c.fillStyle="white";c.font="48px sans-serif";c.fillText("Canvas preview",320,380);</script></body></html>',
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 480"><rect width="640" height="480" fill="#f8e8d8"/><circle cx="320" cy="240" r="120" fill="#a883d6"><animate attributeName="r" values="80;120;80" dur="2s" repeatCount="indefinite"/></circle><text x="110" y="430" font-size="38">\u6d4b\u667a\u4f5c\u54c1\u9884\u89c8</text></svg>',
  ]
  const results = artwork.map((html, index) => ({ id:index+1, channel_id:1, relay_station_id:0, account_external_id:0, account_name:'test', model:'model-'+index, reasoning_effort:'medium', trigger:'manual', status:'success', error:'', duration_ms:1000, started_at:'2026-11-04T00:00:00Z', finished_at:'2026-11-04T00:01:00Z' }))
  const saved = new Map(), reports = []
  const built = process.env.PELICAN_TEST_BUILT === '1'
  const output = path.resolve(built ? 'outputs/pelican-thumbnails-built' : 'outputs/pelican-thumbnails')
  fs.mkdirSync(output, { recursive:true })
  for (const run of ['first', 'new-browser', 'mobile', 'save-failure']) {
    if (run === 'save-failure') saved.delete(1)
    const context = await browser.newContext({ viewport:{width:run==='mobile'?390:1440,height:1100}, locale:'zh-CN' })
    const page = await context.newPage(), errors = []
    const scripts = []
    page.on('request', request => {if (request.resourceType()==='script') scripts.push(new URL(request.url()).pathname)})
    if (built) {
      await page.route('http://localhost:3010/pelican', route => route.fulfill({path:path.resolve('frontend/dist/index.html'),contentType:'text/html'}))
      await page.route('http://localhost:3010/assets/**', route => route.fulfill({path:path.resolve('frontend/dist',new URL(route.request().url()).pathname.slice(1))}))
    }
    let bodyRequests = 0, uploads = 0, thumbnailRequests = 0
    page.on('pageerror', error => errors.push(error.message))
    await page.addInitScript(() => {
      if (window.top === window) localStorage.setItem('uh_token', 'local-thumbnail-test')
      window.__thumbnailStats = {images:[], peakRenderers:0}
      document.addEventListener('load', event => {if (event.target instanceof HTMLImageElement && event.target.src.startsWith('blob:')) window.__thumbnailStats.images.push({at:performance.now(),width:event.target.naturalWidth,height:event.target.naturalHeight})}, true)
      new MutationObserver(() => {window.__thumbnailStats.peakRenderers = Math.max(window.__thumbnailStats.peakRenderers,document.querySelectorAll('iframe[data-pelican-renderer]').length)}).observe(document,{subtree:true,childList:true})
    })
    await page.route('**/api/**', async route => {
      const url = new URL(route.request().url()), p = url.pathname
      if (p.startsWith('/api/channels/') || p.startsWith('/api/relay-stations/') || p === '/api/auth/me') assert.equal(route.request().headers().authorization, 'Bearer local-thumbnail-test', 'private thumbnail requests must include the login token')
      const match = p.match(/\/pelican\/results\/(\d+)(\/thumbnail)?$/)
      if (match) {
        const id = Number(match[1])
        if (match[2]) {
          if (route.request().method() === 'PUT') {
            uploads++
            if (run === 'save-failure') return route.fulfill({ status:503, contentType:'application/json', body:JSON.stringify({error:'temporary storage failure'}) })
            const data = route.request().postDataBuffer()
            assert.ok(data.length>0 && data.length<=96*1024)
            assert.equal(data.readUInt16BE(0),0xffd8)
            assert.match(route.request().headers()['x-pelican-source-sha256'],/^[a-f0-9]{64}$/)
            saved.set(id,data)
            return route.fulfill({status:204})
          }
          thumbnailRequests++
          return saved.has(id) ? route.fulfill({status:200,contentType:'image/jpeg',body:saved.get(id)}) : route.fulfill({status:404,body:'{}'})
        }
        bodyRequests++
        return route.fulfill({status:200,contentType:'application/json',body:JSON.stringify({data:{...results[id-1],html:artwork[id-1]}})})
      }
      if (route.request().method()!=='GET') throw new Error('Unexpected mutation '+p)
      let data = []
      if (p==='/api/auth/me') data={username:'test'}
      else if(p==='/api/system-settings') data=defaults
      else if(p==='/api/sync-settings') data={channel_enabled:false}
      else if(p.endsWith('/branding')) data={name:'GatewayOps'}
      else if(p==='/api/version') data={version:'test',revision:'unknown',channel:'development'}
      else if(p==='/api/channels/pelican') data={groups:[{channel:{id:1,name:'Thumbnail performance',type:'newapi',site_url:'https://example.invalid',pelican_key_configured:false},targets:[],results}]}
      else if(p==='/api/relay-stations/pelican-sources') data={groups:[]}
      await route.fulfill({status:200,contentType:'application/json',body:JSON.stringify({data})})
    })
    await page.goto('http://localhost:3010/pelican')
    const count = run==='mobile'?1:4
    await page.waitForFunction(count => document.querySelectorAll('img[src^="blob:"]').length>=count,count,{timeout:18000})
    const stats = await page.evaluate(() => window.__thumbnailStats)
    assert.ok(stats.images.every(image=>image.width===448 && image.height===336))
    assert.ok(stats.peakRenderers<=1)
    if (run==='first') {assert.equal(bodyRequests,4);assert.equal(uploads,4);assert.equal(saved.size,4)}
    else if (run === 'save-failure') {
      assert.equal(bodyRequests,1,'retry must reuse the generated JPEG without loading artwork HTML again')
      assert.equal(uploads,2,'transient save failures must retry once')
      await page.getByText('\u9884\u89c8\u5df2\u751f\u6210\uff0c\u672a\u4fdd\u5b58\uff1a', {exact:false}).waitFor()
    }
    else {assert.equal(bodyRequests,0);assert.equal(uploads,0);assert.equal(stats.peakRenderers,0)}
    if (run === 'new-browser') {
      await page.reload()
      await page.waitForFunction(() => document.querySelectorAll('img[src^="blob:"]').length >= 4, null, {timeout:15000})
      assert.equal(bodyRequests,0,'refresh must read saved thumbnails without loading artwork HTML')
      assert.equal(uploads,0,'refresh must not upload thumbnails again')
    }
    await page.screenshot({path:path.join(output,run+'.png')})
    const beforePreview=bodyRequests
    await page.getByRole('button',{name:'\u67e5\u770b\u6d4b\u667a\u4f5c\u54c1 1',exact:true}).click()
    await page.getByRole('dialog').waitFor()
    assert.ok(bodyRequests>beforePreview && bodyRequests<=beforePreview+2)
    assert.deepEqual(errors,[])
    if (built) {
      assert.ok(!scripts.some(script=>script.includes('relay-stations-page-')||script.includes('system-settings-page-')||script.includes('LineChart-')),'unrelated large page was eagerly loaded')
      assert.ok(scripts.some(script=>script.includes('pelican-library-page-')))
    }
    reports.push({run,bodyRequestsBeforePreview:beforePreview,uploads,thumbnailRequests,sizes:Array.from(saved.values(),value=>value.length),scripts,...stats})
    await context.close()
  }
  fs.writeFileSync(path.join(output,'results.json'),JSON.stringify(reports,null,2),'utf8')
  console.log(JSON.stringify(reports))
  await browser.close()
}
main().catch(error=>{console.error(error);process.exit(1)})
