const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')

async function main() {
 const browser = await chromium.launch({ executablePath: 'C:/Program Files/Google/Chrome/Application/chrome.exe', headless: true })
 try {
  const page = await browser.newPage({ viewport: { width: 1440, height: 1100 }, locale: 'zh-CN', timezoneId: 'Asia/Shanghai' })
  const errors=[]
  page.on('pageerror',error=>errors.push(error.message))
  const bucket={start:'2026-11-06T03:48:00Z',end:'2026-11-06T06:34:00Z',request_count:1033,success_rate:0.997,first_token_p50_ms:4600,first_token_p95_ms:25000,duration_p50_ms:13000,duration_p95_ms:58000,output_token_speed_p50:20,output_token_speed_p95:29}
  const sample={success:true,created_at:'2026-11-06T05:00:00Z',first_token_ms:4600,duration_ms:10000,output_tokens:100,model:'text'}
  const group={external_id:2,name:'dev',rate_multiplier:1,enabled:true,status:'available',data_complete:true,updated_at:new Date().toISOString(),latest_call_at:sample.created_at,success_count:1030,failure_count:3,consecutive_failures:0,calls:[sample],latency_buckets:[bucket]}
  await page.route('**/api/**',route=>{
   const url=new URL(route.request().url())
   let data=[]
   if(url.pathname.endsWith('/branding'))data={name:'GatewayOps'}
   else if(url.pathname.endsWith('/relay-stations'))data=[{id:1,name:'Test Station'}]
   else if(url.pathname.endsWith('/group-monitor'))data={station_id:1,station_name:'Test Station',range:url.searchParams.get('range'),sync_status:'ready',updated_at:new Date().toISOString(),groups:[group],summary:{total:1,available:1,degraded:0,unavailable:0,idle:0,disabled:0,unknown:0}}
   else if(url.pathname.endsWith('/group-pelican'))data={page_enabled:false,configs:[],results:[]}
   return route.fulfill({status:200,contentType:'application/json',body:JSON.stringify({data})})
  })
  const green='rgb(16, 185, 129)',amber='rgb(245, 158, 11)'
  for(const width of [1440,390]){
   await page.setViewportSize({width,height:1100})
   await page.goto('http://localhost:3010/public/relay-monitor/1')
   const lane=page.getByLabel('\u65f6\u95f4\u7a33\u5b9a\u6027\u6cf3\u9053',{exact:true})
   const cell=lane.getByRole('button')
   await cell.waitFor()
   const tones=await cell.locator('span').evaluateAll(spans=>spans.map(span=>getComputedStyle(span).backgroundColor))
   assert.deepEqual(tones,[green,amber,green,green])
   await cell.hover()
   const tooltip=page.getByRole('tooltip')
   await tooltip.waitFor()
   assert.ok(await tooltip.getByText('20 tok/s',{exact:true}).isVisible())
   assert.ok(await tooltip.getByText('29 tok/s',{exact:true}).isVisible())
   for(const [value,tone] of [['4.6\u79d2','rgb(4, 120, 87)'],['25\u79d2','rgb(180, 83, 9)'],['13\u79d2','rgb(4, 120, 87)'],['58\u79d2','rgb(4, 120, 87)']]){
    const classes=await tooltip.getByText(value,{exact:true}).getAttribute('class')
    assert.ok(classes.includes(tone==='rgb(180, 83, 9)'?'text-amber-700':'text-emerald-700'))
   }
   await page.keyboard.press("Escape")
   await page.mouse.move(0,0)
   await tooltip.waitFor({state:'hidden'})
   const recent=page.getByLabel('\u6700\u8fd1\u8c03\u7528\u5ef6\u65f6\u6cf3\u9053',{exact:true}).getByRole('button')
   await recent.hover()
   await page.getByRole('tooltip').getByText('10 tok/s',{exact:true}).waitFor()
   const output=path.resolve('outputs/group-monitor-metrics');fs.mkdirSync(output,{recursive:true})
   await page.screenshot({path:path.join(output,'monitor-'+width+'.png')})
  }
  assert.deepEqual(errors,[])
  console.log('PASS: quadrant and tooltip colors match; bucket and recent-call token speeds render; 2 viewports')
 } finally { await browser.close() }
}
main().catch(error=>{console.error(error);process.exitCode=1})
