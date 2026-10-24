const assert = require('node:assert/strict')
const fs = require('node:fs')
const vm = require('node:vm')
const path = require('node:path')
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')

async function main() {
 const browser = await chromium.launch({executablePath:'C:/Program Files/Google/Chrome/Application/chrome.exe',headless:true})
 const context = await browser.newContext({viewport:{width:1440,height:1100},locale:'zh-CN',timezoneId:'Asia/Shanghai'})
 const page = await context.newPage()
 const errors = []
 page.on('pageerror', error => errors.push(error.message))
 const source = fs.readFileSync('frontend/lib/system-settings.ts','utf8')
 const defaults = vm.runInNewContext('('+source.match(/export const defaultSystemSettings: SystemSettings = (\{[\s\S]*?\n\})/)[1]+')')
 let requests=0, detailRequests=0
 const root=path.resolve('outputs/task-history-ui');fs.mkdirSync(root,{recursive:true})
 const run = (id) => ({id,task:'channel_sync',status:id===25&&detailRequests<2?'running':id===24?'failed':'success',started_at:new Date(Date.now()-id*60000).toISOString(),finished_at:id===25&&detailRequests<2?null:new Date(Date.now()-id*60000+2200).toISOString(),duration_ms:2200,summary:id===24?'\u4e0a\u6e38\u8bf7\u6c42\u5931\u8d25':'\u6267\u884c\u5b8c\u6210'})
 await page.route('**/api/**', async route => {
  const url=new URL(route.request().url());let data={};let status=200
  if(url.pathname==='/api/auth/me')data={auth_disabled:true,username:'test'}
  else if(url.pathname==='/api/system-settings')data=defaults
  else if(url.pathname==='/api/sync-settings')data={channel_configured:true,channel_enabled:true,channel_interval_minutes:30,relay_rate_configured:true,relay_rate_enabled:false,relay_rate_interval_minutes:1,relay_rate_interval_seconds:10,relay_snapshot_configured:true,relay_snapshot_enabled:true,relay_snapshot_interval_minutes:60,relay_snapshot_interval_seconds:3600}
  else if(url.pathname==='/api/version')data={name:'gateway-ops',version:'test',revision:'unknown',build_time:'unknown',channel:'development'}
  else if(url.pathname.startsWith('/api/tasks/')){
   requests++
   if(url.pathname==='/api/tasks/channel_sync/runs'){
    const n=Number(url.searchParams.get('page'));data={runs:Array.from({length:25},(_,i)=>run(25-i)).slice((n-1)*20,n*20),total:25,succeeded:detailRequests>=2?24:23,failed:1,running:detailRequests>=2?0:1,page:n,page_size:20}
   }else if(url.pathname==='/api/tasks/channel_sync/runs/25'){
    detailRequests++;data={...run(25),logs:detailRequests<2?'':'[INFO] \u6e20\u9053\u540c\u6b65\u5b8c\u6210\n[INFO] \u6210\u529f 3\uff0c\u5931\u8d25 0'}
   }else if(url.pathname==='/api/tasks/retention/runs'){
    status=400;data={error:'\u6267\u884c\u8bb0\u5f55\u8bfb\u53d6\u5931\u8d25'}
   }else data={runs:[],total:0,succeeded:0,failed:0,running:0,page:1,page_size:20}
  }else data=[]
  await route.fulfill({status,contentType:'application/json',body:JSON.stringify(status===200?{data}:data)})
 })
 await page.goto((process.env.TASK_HISTORY_PREVIEW_URL || 'http://127.0.0.1:18310')+'/settings#background-tasks')
 const channelButton=page.getByRole('button',{name:'\u6e20\u9053\u4f59\u989d\u4e0e\u500d\u7387\u540c\u6b65\u67e5\u770b\u6267\u884c\u8bb0\u5f55',exact:true})
 await channelButton.click()
 await page.getByRole('cell',{name:'#25',exact:true}).waitFor()
 assert.equal(await page.locator('tbody tr').count(),20)
 assert.ok((await page.getByRole('dialog').innerText()).includes('\u7d2f\u8ba1\u6267\u884c'))
 await page.waitForTimeout(600)
 const desktopBounds=await page.getByRole('dialog').boundingBox();assert.ok(desktopBounds.x+desktopBounds.width<=1441)
 await page.screenshot({path:path.join(root,'desktop-list.png')})
 await page.getByRole('button',{name:'\u4e0b\u4e00\u9875',exact:true}).click()
 await page.getByRole('cell',{name:'#5',exact:true}).waitFor()
 assert.equal(await page.locator('tbody tr').count(),5)
 assert.ok(await page.getByRole('button',{name:'\u4e0b\u4e00\u9875',exact:true}).isDisabled())
 await page.getByRole('button',{name:'\u4e0a\u4e00\u9875',exact:true}).click()
 await page.getByRole('button',{name:'\u67e5\u770b\u6267\u884c 25 \u7684\u65e5\u5fd7',exact:true}).click()
 await page.locator('pre').filter({hasText:'\u4efb\u52a1\u6267\u884c\u4e2d'}).waitFor()
 await page.locator('pre').filter({hasText:'\u6e20\u9053\u540c\u6b65\u5b8c\u6210'}).waitFor({timeout:9000})
 await page.screenshot({path:path.join(root,'desktop-log.png')})
 await page.getByRole('button',{name:'\u8fd4\u56de\u6267\u884c\u5217\u8868',exact:true}).click()
 await page.keyboard.press('Escape')
 await page.getByRole('dialog').waitFor({state:'hidden'})
 const beforeClose=requests;await page.waitForTimeout(5300);assert.equal(requests,beforeClose)
 await page.getByRole('button',{name:'API Key \u500d\u7387\u63a2\u6d4b\u67e5\u770b\u6267\u884c\u8bb0\u5f55',exact:true}).click()
 await page.getByText('\u6682\u65e0\u6267\u884c\u8bb0\u5f55\uff0c\u4efb\u52a1\u4e0b\u6b21\u81ea\u52a8\u6267\u884c\u540e\u4f1a\u663e\u793a\u5728\u8fd9\u91cc\u3002',{exact:true}).waitFor()
 await page.keyboard.press('Escape')
 await page.getByRole('button',{name:'\u5386\u53f2\u6570\u636e\u6e05\u7406\u67e5\u770b\u6267\u884c\u8bb0\u5f55',exact:true}).click()
 await page.getByRole('alert').filter({hasText:'\u6267\u884c\u8bb0\u5f55\u8bfb\u53d6\u5931\u8d25'}).waitFor()
 await page.keyboard.press('Escape')
 await page.setViewportSize({width:390,height:844})
 await channelButton.click()
 await page.getByRole('button',{name:'\u67e5\u770b\u6267\u884c 25 \u7684\u65e5\u5fd7',exact:true}).waitFor()
 await page.waitForTimeout(600)
 assert.equal(await page.locator('thead th:visible').count(),4)
 const bounds=await page.getByRole('dialog').boundingBox();assert.ok(bounds.width<=390&&bounds.x>=0&&bounds.x+bounds.width<=391)
 await page.screenshot({path:path.join(root,'mobile-list.png')})
 assert.deepEqual(errors,[])
 fs.writeFileSync(path.join(root,'results.json'),JSON.stringify({passed:true,cases:['desktop list','pagination','live completion log','close stops polling','disabled task history','empty state','error state','mobile drawer'],requests,detailRequests,errors},null,2),'utf8')
 console.log('PASS: desktop, pagination, logs, polling cleanup, empty/error states, mobile; screenshots='+root)
 await browser.close()
}
main().catch(error=>{console.error(error);process.exit(1)})
