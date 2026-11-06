const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')

async function main() {
  const browser = await chromium.launch({ executablePath: 'C:/Program Files/Google/Chrome/Application/chrome.exe', headless: true })
  try {
    const defaults = vm.runInNewContext('(' + fs.readFileSync('frontend/lib/system-settings.ts', 'utf8').match(/export const defaultSystemSettings: SystemSettings = (\{[\s\S]*?\n\})/)[1] + ')', { defaultPelicanPrompt: 'test' })
    Object.assign(defaults, {channels_page_enabled:true,pelican_page_enabled:false})
    const channel={id:1,name:'History Audit',type:'newapi',site_url:'https://example.invalid',username:'test',balance_mode:'auto',monitor_enabled:true,balance:10,balance_threshold:0,created_at:'2026-10-01T00:00:00Z',updated_at:'2026-11-05T00:00:00Z',accounts:[]}
    const output=path.resolve('outputs/usage-history/ui')
    fs.mkdirSync(output,{recursive:true})
    const reports=[]
    for (const [width,mode,type] of [[1440,'list','newapi'],[390,'gallery','sub2api']]) {
      channel.type=type
      const context=await browser.newContext({viewport:{width,height:1100},locale:'zh-CN',timezoneId:'Asia/Shanghai'})
      const page=await context.newPage(),errors=[]
      const requestedRanges=[]
      let posts=0,checks=0,repaired=false,failNext=false,pauseNext=false,pausePosts=0,retentionNext=false
      page.setDefaultTimeout(3000)
      page.on('pageerror',error=>errors.push(error.message))
      page.on('console',message=>{if(message.type()==='error')console.error('BROWSER: '+message.text())})
      await page.addInitScript(mode=>localStorage.setItem('gatewayops_channel_view_mode',mode),mode)
      await page.addInitScript(()=>{
        const original=window.fetch.bind(window)
        window.fetch=async(input,init)=>{
          const url=String(input)
          const check=url.includes('/usage-integrity'),repair=url.includes('/usage-history')
          const response=await original(input,init)
          if(!check&&!repair)return response
          let result=check?(await response.json()).data:null
          const originalBody=repair?await response.text():''
          if(repair) {
            const frames=originalBody.split('\n\n').filter(block=>block.startsWith('data: ')).map(block=>JSON.parse(block.slice(6)))
            result=frames.find(frame=>frame.stage==='done')?.data ?? frames.find(frame=>frame.data?.kind==='usage_integrity')?.data.result
          }
          const encoder=new TextEncoder(),timers=[]
          return new Response(new ReadableStream({start(controller){
            const emit=event=>controller.enqueue(encoder.encode('data: '+JSON.stringify(event)+'\n\n'))
            const snapshot={...result,status:'mismatch',local_count:result.local_count===7852?7852:12,error:undefined,phase:'total',accounts:result.accounts.map(account=>({...account,local_count:result.local_count===7852?7852:12,error:undefined})),audit:{checked_windows:1,skipped_windows:0,anomalous_days:0,repaired_days:0,windows:[]}}
            emit({stage:'usage',message:repair?'\u6b63\u5728\u6838\u5bf9\u603b\u6570\uff08\u624b\u52a8\u91cd\u62c9\uff09':'\u6b63\u5728\u6838\u5bf9\u603b\u6570\uff08\u81ea\u52a8\u6838\u5bf9\uff09',data:{kind:'usage_integrity',phase:'total',result:snapshot}})
            timers.push(setTimeout(()=>emit({stage:'usage',message:'\u6309\u6708\u6838\u5bf9 2026-10 \u00b7 \u5df2\u68c0\u67e5 1 \u4e2a\u533a\u95f4',data:{kind:'usage_integrity',phase:'locating',result:{...snapshot,phase:'locating',audit:{...result.audit,repaired_days:0,windows:result.audit.windows?.map(window=>({...window,status:'mismatch'}))}}}}),1500))
            timers.push(setTimeout(()=>{
              if(repair)controller.enqueue(encoder.encode(originalBody))
              else emit({stage:'done',ok:true,message:'\u6838\u5bf9\u5b8c\u6210',data:result})
              controller.close()
            },1700))
            init?.signal?.addEventListener('abort',()=>{timers.forEach(clearTimeout);controller.error(new DOMException('Aborted','AbortError'))},{once:true})
          },cancel(){timers.forEach(clearTimeout)}}),{headers:{'Content-Type':'text/event-stream'}})
        }
      })
      await page.route('**/api/**',async route=>{
        const url=new URL(route.request().url()),p=url.pathname
        if(p.endsWith('/usage-integrity'))checks++
        let data=[]
        if(p==='/api/auth/me') data={auth_disabled:true,username:'test'}
        else if(p==='/api/system-settings') data=defaults
        else if(p==='/api/sync-settings') data={channel_configured:true,channel_enabled:true,channel_interval_minutes:30,relay_rate_configured:true,relay_rate_enabled:false,relay_snapshot_configured:true,relay_snapshot_enabled:false}
        else if(p==='/api/version') data={name:'gateway-ops',version:'test',revision:'unknown',build_time:'unknown',channel:'development'}
        else if(p.endsWith('/branding')) data={name:'GatewayOps'}
        else if(p==='/api/channels') data=[channel]
        else if(p==='/api/channels/associations') data={links:[],accounts:[],stations:[]}
        else if(p==='/api/channels/summary') data={range:'today',total_channels:1,active_channels:1,failed_channels:0,total_balance:10,consumption_amount:0,usage_cost_amount:0,usage_request_count:0,cumulative_recharge_amount:0,user_charge_amount:0,matched_account_count:0,user_charge_complete:true,lowest_balance:channel,channels:[],rate_change_count:0,recent_rate_changes:[],recent_notification_logs:[],relay:[],recent_relay_adjustments:[]}
        else if(p.endsWith('/usage-integrity')) data=retentionNext?retentionResult():failNext?conflictResult():{phase:'complete',status:repaired?'matched':'mismatch',local_count:repaired?15:12,upstream_count:15,since:'2026-10-01T00:00:00Z',until:'2026-11-01T00:00:00Z',checked_at:'2026-11-05T00:00:00Z',accounts:[{account_id:1,account_name:'test',local_count:repaired?15:12,upstream_count:15}],audit:{checked_windows:20,skipped_windows:14,anomalous_days:repaired?0:1,repaired_days:0,windows:repaired?[]:[{account_id:1,account_name:'test',level:'day',status:'mismatch',since:'2026-10-22T00:00:00Z',until:'2026-10-23T00:00:00Z',local_count:2,upstream_count:5}]}}
        else if(p.endsWith('/usage-history')) {
          posts++
          requestedRanges.push(url.searchParams.get('range'))
          assert.equal(route.request().method(),'POST')
          if(!failNext&&!pauseNext&&url.searchParams.get('range')==='custom'){
            assert.equal(url.searchParams.get('range'),'custom')
            assert.equal(url.searchParams.get('start'),'2026-09-30T16:00:00.000Z')
            assert.equal(url.searchParams.get('end'),'2026-11-01T16:00:00.000Z')
          }
          repaired=true
          const fixed=retentionNext?retentionResult():{phase:'complete',status:'matched',local_count:15,upstream_count:15,since:'2026-10-01T00:00:00Z',until:'2026-11-01T00:00:00Z',checked_at:'2026-11-05T00:00:00Z',accounts:[{account_id:1,account_name:'test',local_count:15,upstream_count:15}],audit:{checked_windows:20,skipped_windows:14,anomalous_days:1,repaired_days:1,windows:[{account_id:1,account_name:'test',level:'day',status:'repaired',since:'2026-10-22T00:00:00Z',until:'2026-10-23T00:00:00Z',local_count:2,upstream_count:5}]}}
          if(type==='sub2api'&&!retentionNext)fixed.audit.windows=[{account_id:1,account_name:'test',level:'history',status:'repaired',since:'0001-01-01T00:00:00Z',until:fixed.until,local_count:15,upstream_count:15,page:2,saved_pages:3,saved_rows:15}]
          if(pauseNext && pausePosts++===0){
            const paused={...fixed,phase:'paused',status:'mismatch',local_count:14,retry_status:429,retry_at:new Date(Date.now()+3500).toISOString(),error:'upstream HTTP 429',accounts:[{account_id:1,account_name:'test',local_count:14,upstream_count:15,history_verified_until:'2026-10-20T00:00:00+08:00'}],audit:{...fixed.audit,saved_pages:3,saved_rows:14}}
            const events=[{stage:'usage',message:'paused',data:{kind:'usage_integrity',phase:'paused',result:paused}},{stage:'error',ok:false,message:'upstream HTTP 429'}]
            return route.fulfill({status:200,contentType:'text/event-stream',body:events.map(event=>'data: '+JSON.stringify(event)+'\n\n').join('')})
          }
          const events=failNext?[{stage:'usage',message:'interrupted',data:{kind:'usage_integrity',phase:'interrupted',result:conflictResult()}},{stage:'error',ok:false,message:conflictResult().error}]:[{stage:'done',message:'\u5386\u53f2\u6821\u9a8c\u5b8c\u6210\uff1a\u68c0\u67e5 20 \u4e2a\u533a\u95f4\uff0c\u8df3\u8fc7 14 \u4e2a\u4e00\u81f4\u533a\u95f4\uff0c\u4fee\u590d 1 \u5929',ok:true,data:fixed,time:new Date().toISOString()}]
          return route.fulfill({status:200,contentType:'text/event-stream',body:events.map(event=>'data: '+JSON.stringify(event)+'\n\n').join('')})
        }
        await route.fulfill({status:200,contentType:'application/json',body:JSON.stringify({data})})
      })
      await page.goto('http://localhost:3010/channels',{waitUntil:'domcontentloaded'})
      await page.getByText(/\u7f3a\u5c11 3 \u6761/).first().waitFor()
      const initialChecks=checks
      await page.getByRole('button',{name:'\u91cd\u65b0\u62c9\u53d6\u5386\u53f2',exact:true}).click()
      const dialog=page.getByRole('dialog')
      await dialog.waitFor()
      await page.waitForTimeout(100)
      assert.equal(posts,0,'opening must wait for a manual start')
      await dialog.getByRole('button',{name:'\u5168\u90e8',exact:true}).click()
      assert.equal(posts,0,'selecting all history must not start a repair')
      assert.equal(checks,initialChecks,'dialog must not launch a separate read-only precheck')
      assert.equal(await dialog.getByRole('button',{name:'\u91cd\u65b0\u62c9\u53d6',exact:true}).count(),0)
      await dialog.getByRole('button',{name:'\u5f00\u59cb\u6838\u5bf9\u5e76\u8865\u9f50',exact:true}).click()
      await dialog.getByText(/\u5386\u53f2\u6821\u9a8c\u5b8c\u6210/).waitFor()
      await dialogSections(page)
      assert.equal(posts,1,'one explicit click must start exactly one repair')
      assert.equal(requestedRanges[0],'all','first request must use the range explicitly selected by the user')
      if(type==='sub2api') {
        const details=dialog.getByRole('region',{name:'\u5f02\u5e38\u533a\u95f4'})
        await details.getByText(/\u5386\u53f2\u8303\u56f4\u6838\u5bf9/).waitFor()
        await details.getByText(/\u7d2f\u8ba1\u4fdd\u5b58 3 \u9875\u300115 \u6761\u8c03\u7528/).waitFor()
        await details.getByText('\u5df2\u4fee\u590d',{exact:true}).waitFor()
        assert.equal((await details.innerText()).includes('0001'),false)
      }
      await dialog.getByRole('button',{name:'\u81ea\u5b9a\u4e49',exact:true}).click()
      await dialog.locator('input[type="datetime-local"]').nth(0).fill('2026-10-01T00:00')
      await dialog.locator('input[type="datetime-local"]').nth(1).fill('2026-11-01T23:59')
      assert.equal(posts,1,'custom range must not run while dates are being edited')
      await dialog.getByRole('heading').first().click()
      assert.equal(posts,1,'leaving custom date inputs must not start a repair')
      await dialog.getByRole('button',{name:'\u5f00\u59cb\u6838\u5bf9\u5e76\u8865\u9f50',exact:true}).click()
      await dialog.getByText(/\u5386\u53f2\u6821\u9a8c\u5b8c\u6210/).waitFor()
      assert.equal(posts,2)
      const box=await dialog.boundingBox()
      assert.ok(box.x>=0 && box.x+box.width<=width+1)
      await page.screenshot({path:path.join(output,mode+'-'+width+'.png')})
      await dialog.getByText(/\u6761\u6570\u4e00\u81f4/).first().waitFor()
      failNext=true
      await dialog.getByRole('button',{name:'7 \u5929',exact:true}).click()
      assert.equal(posts,2,'changing presets must wait for a manual start')
      await dialog.getByRole('button',{name:'\u5f00\u59cb\u6838\u5bf9\u5e76\u8865\u9f50',exact:true}).click()
      await dialog.getByRole('alert').filter({hasText:'\u4e0a\u6e38\u7edf\u8ba1\u4e0d\u4e00\u81f4'}).waitFor()
      await dialog.getByText('7852',{exact:true}).waitFor()
      await dialog.getByText('7800',{exact:true}).waitFor()
      await dialog.getByText(/\u591a 52 \u6761/).first().waitFor()
      await dialog.getByText(/\u5f85\u7ec6\u5206/).waitFor()
      await page.screenshot({path:path.join(output,mode+'-'+width+'-interrupted.png')})
      assert.equal(posts,3,'failure must not cause an automatic retry loop')
      failNext=false
      await dialog.getByRole('button',{name:'\u7ee7\u7eed\u5904\u7406',exact:true}).click()
      await dialog.getByText(/\u5386\u53f2\u6821\u9a8c\u5b8c\u6210/).waitFor()
      assert.equal(posts,4)
      pauseNext=true
      await dialog.getByRole('button',{name:'\u5168\u90e8',exact:true}).click()
      await dialog.getByRole('button',{name:'\u5f00\u59cb\u6838\u5bf9\u5e76\u8865\u9f50',exact:true}).click()
      await dialog.getByText(/HTTP 429.*\u81ea\u52a8\u7ee7\u7eed/).waitFor()
      await dialog.getByText(/\u5df2\u9010\u9875\u6838\u5bf9\u5e76\u4fdd\u5b58 3 \u9875.*14 \u6761\u8c03\u7528/).waitFor()
      await dialog.getByText(/\u5386\u53f2\u8fde\u7eed\u6838\u5bf9\u5b8c\u6210\u81f3.*2026\/10\/20/).waitFor()
      assert.equal(await dialog.getByRole('button',{name:'\u7b49\u5f85\u4e0a\u6e38\u6062\u590d',exact:true}).isDisabled(),true)
      await page.screenshot({path:path.join(output,mode+'-'+width+'-paused.png')})
      await dialog.getByText(/\u5386\u53f2\u6821\u9a8c\u5b8c\u6210/).waitFor({timeout:20000})
      assert.equal(posts,6)
      retentionNext=true
      await dialog.getByRole('button',{name:'30 \u5929',exact:true}).click()
      await dialog.getByRole('button',{name:'\u5f00\u59cb\u6838\u5bf9\u5e76\u8865\u9f50',exact:true}).click()
      await dialog.getByText('\u672c\u5730\u53ef\u6838\u5bf9',{exact:true}).waitFor()
      await dialog.getByText(/\u53e6\u4fdd\u7559 5831 \u6761\u66f4\u65e9\u5386\u53f2/).first().waitFor()
      assert.equal(await dialog.getByText('2021',{exact:true}).count(),2)
      await dialog.getByText(/\u5386\u53f2\u6821\u9a8c\u5b8c\u6210/).waitFor()
      assert.equal(await dialog.getByRole('alert').count(),0)
      assert.equal(posts,7)
      assert.equal(checks,initialChecks,'range changes and retries must not start read-only prechecks')
      await page.screenshot({path:path.join(output,mode+'-'+width+'-retained.png')})
      await dialog.getByRole('button',{name:'\u5173\u95ed',exact:true}).click()
      await page.getByRole('button',{name:'\u91cd\u65b0\u62c9\u53d6\u5386\u53f2',exact:true}).click()
      await dialog.getByRole('button',{name:'\u5f00\u59cb\u6838\u5bf9\u5e76\u8865\u9f50',exact:true}).waitFor()
      await page.waitForTimeout(100)
      assert.equal(posts,7,'reopening a completed dialog must wait for another manual start')
      assert.deepEqual(errors,[])
      reports.push({mode,width,type,posts,errors,conflictPreserved:true,rateLimitResumed:true})
      await context.close()
    }
    fs.writeFileSync(path.join(output,'results.json'),JSON.stringify(reports,null,2),'utf8')
    console.log('PASS: '+reports.length+' history dialog cases')
  } finally {await browser.close()}
}
function conflictResult(){return {phase:'interrupted',status:'mismatch',local_count:7852,upstream_count:7800,since:'0001-01-01T00:00:00Z',until:'2026-11-01T00:00:00Z',checked_at:'2026-11-05T00:00:00Z',error:'\u4e0a\u6e38\u7edf\u8ba1\u4e0d\u4e00\u81f4\uff1a\u603b\u6570 7800 \u6761\uff0c\u6309\u6708\u5408\u8ba1 7810 \u6761\uff1b\u5df2\u505c\u6b62\u4fee\u590d',accounts:[{account_id:1,account_name:'test',local_count:7852,upstream_count:7800}],audit:{checked_windows:22,skipped_windows:18,anomalous_days:0,repaired_days:0,windows:[{account_id:1,account_name:'test',level:'month',status:'mismatch',since:'2026-10-01T00:00:00Z',until:'2026-11-01T00:00:00Z',local_count:100,upstream_count:110}]}}}
function retentionResult(){return {phase:'complete',status:'matched',local_count:7852,retained_count:5831,upstream_count:2021,since:'0001-01-01T00:00:00Z',until:'2026-11-06T01:30:00Z',checked_at:'2026-11-06T01:36:00Z',accounts:[{account_id:1,account_name:'test',local_count:7852,retained_count:5831,upstream_count:2021,history_available_since:'2026-10-22T04:00:00Z',history_verified_until:'2026-11-06T01:30:00Z'}],audit:{checked_windows:1,skipped_windows:0,anomalous_days:0,repaired_days:0}}}
async function dialogSections(page){
  await page.getByText(/1\. \u603b\u4f53\u6838\u5bf9/).first().waitFor()
  await page.getByText(/2\. \u5f02\u5e38\u533a\u95f4/).first().waitFor()
  await page.getByText(/3\. \u5904\u7406\u8fdb\u5ea6/).first().waitFor()
}
main().catch(error=>{console.error(error);process.exit(1)})
