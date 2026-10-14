import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
const require = createRequire(import.meta.url)
const { chromium } = require(process.env.PLAYWRIGHT_MODULE_PATH || 'playwright')
const baseURL = process.env.GATEWAY_OPS_URL || 'http://127.0.0.1:3010'

async function main() {
 const browser=await chromium.launch({headless:true,...(process.env.CHROME_PATH?{executablePath:process.env.CHROME_PATH}:{})})
 const page=await browser.newPage()
 const errors=[]
 page.on('pageerror',error=>errors.push(error.message))
 const code=`import React from '/node_modules/.vite/deps/react.js';
import ReactDOM from '/node_modules/.vite/deps/react-dom_client.js';
import { RefreshProvider } from '/lib/refresh-context.tsx';
import { useRelayRecentUsage } from '/lib/queries.ts';
function Probe(){const [station,setStation]=React.useState(1);const query=useRelayRecentUsage(station,'today');window.switchStation=setStation;window.refreshQuery=query.refetch;return React.createElement('div',{id:'query-state'},station+':'+(query.data?'loaded':query.error||'loading'))}
ReactDOM.createRoot(document.getElementById('root')).render(React.createElement(RefreshProvider,null,React.createElement(Probe)));`
 await page.route('**/__query_fixture.js*',route=>route.fulfill({contentType:'application/javascript',body:code}))
 await page.route('**/__query_fixture',async route=>{
  const response=await page.request.get(baseURL+'/')
  await route.fulfill({contentType:'text/html',body:(await response.text()).replace('/src/main.tsx','/__query_fixture.js')})
 })
 let requests=0
 await page.route('**/api/relay-stations/*/usage/recent?*',async route=>{
  requests++
  await new Promise(resolve=>setTimeout(resolve,1100))
  await route.fulfill({contentType:'application/json',body:JSON.stringify({data:[]})}).catch(()=>{})
 })
 await page.addInitScript(()=>{const interval=window.setInterval;window.setInterval=(callback,delay,...args)=>interval(callback,delay===30000?100:delay,...args)})
 try {
  await page.goto(baseURL+'/__query_fixture',{waitUntil:'domcontentloaded'})
  await page.waitForTimeout(1800)
  assert.equal(await page.locator('#query-state').innerText(),'1:loaded','Background polling must allow a slow first response to finish')
  assert.ok(requests<=2,`Background polling started ${requests} overlapping requests`)
  await page.evaluate(()=>window.refreshQuery())
  assert.equal(await page.locator('#query-state').innerText(),'1:loaded')
  await page.evaluate(()=>window.switchStation(2))
  assert.equal(await page.locator('#query-state').innerText(),'2:loading','Station switch must not display previous station data')
  await page.waitForFunction(()=>document.querySelector('#query-state')?.textContent==='2:loaded')
  assert.deepEqual(errors,[])
  console.log(JSON.stringify({requests,loaded:true,errors}))
 }finally {await browser.close()}
}
main().catch(error=>{console.error(error);process.exitCode=1})
