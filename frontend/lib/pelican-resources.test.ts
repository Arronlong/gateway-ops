import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22
import { PelicanResourcePool } from "./pelican-resources.ts"

const turn = () => new Promise(resolve => setTimeout(resolve, 0))

test("\u4e0b\u8f7d\u4e0e\u751f\u6210\u72ec\u7acb\u6392\u961f\uff0c\u751f\u6210\u5360\u6ee1\u65f6\u4e0b\u8f7d\u4ecd\u53ef\u7ee7\u7eed", async () => {
  const downloads = new PelicanResourcePool<string>({maxBytes:0,maxEntries:0,maxConcurrent:Infinity,size:value=>value.length})
  const renders = new PelicanResourcePool<string>({maxBytes:0,maxEntries:0,maxConcurrent:10,size:value=>value.length})
  const signal = new AbortController().signal
  const downloadFinishes: Array<() => void> = [], renderFinishes: Array<() => void> = []
  let activeDownloads=0, downloaded=0, rendered=0
  const tasks = Array.from({length:24}, async (_,index) => {
    const html = await downloads.load(String(index),signal,()=>new Promise<string>(resolve=>{
      activeDownloads++
      downloadFinishes.push(()=>{activeDownloads--;downloaded++;resolve(String(index))})
    }))
    return renders.load(String(index),signal,()=>new Promise<string>(resolve=>{
      renderFinishes.push(()=>{rendered++;resolve(html)})
    }))
  })
  await turn()
  assert.equal(activeDownloads,24)
  downloadFinishes.splice(0).forEach(finish=>finish())
  await turn()
  assert.equal(activeDownloads,0)
  assert.equal(renderFinishes.length,10)
  assert.equal(downloaded,24)
  assert.equal(rendered,0)
  assert.equal(renders.cachedEntries,0)
  for(const count of [10,10,4]) {
    assert.equal(renderFinishes.length,count)
    renderFinishes.splice(0).forEach(finish=>finish())
    await turn()
  }
  assert.deepEqual(await Promise.all(tasks),Array.from({length:24},(_,index)=>String(index)))
  assert.equal(downloads.cachedEntries,0)
})

test("\u53d6\u6d88\u4e00\u4e2a\u5171\u4eab\u4e0b\u8f7d\u65f6\u7acb\u5373\u7ed3\u675f\uff0c\u5176\u4ed6\u4f7f\u7528\u8005\u4ecd\u80fd\u5b8c\u6210", async () => {
  const pool = new PelicanResourcePool<string>({maxBytes:0,maxEntries:0,size:value=>value.length})
  const controller = new AbortController()
  let finish!: (value:string)=>void, calls=0, aborted=false
  const load = (signal:AbortSignal) => new Promise<string>(resolve=>{
    calls++;finish=resolve
    signal.addEventListener("abort",()=>{aborted=true},{once:true})
  })
  const first=pool.load("same",controller.signal,load)
  const second=pool.load("same",new AbortController().signal,load)
  await turn()
  const rejected=assert.rejects(first,{name:"AbortError"})
  controller.abort()
  await rejected
  assert.equal(aborted,false)
  finish("html")
  assert.equal(await second,"html")
  assert.equal(calls,1)
})

test("\u79bb\u5c4f\u53d6\u6d88\u5df2\u4e0b\u8f7d\u4f5c\u54c1\u540e\u4e0d\u518d\u751f\u6210", async () => {
  const pool = new PelicanResourcePool<string>({maxBytes:0,maxEntries:0,size:value=>value.length})
  let finish!: (value:string)=>void, calls=0
  const first=pool.load("first",new AbortController().signal,()=>new Promise<string>(resolve=>{finish=resolve}))
  const controller=new AbortController()
  const queued=pool.load("queued",controller.signal,async()=>{calls++;return "skip"})
  const rejected=assert.rejects(queued,{name:"AbortError"})
  await turn()
  controller.abort()
  await rejected
  finish("image")
  assert.equal(await first,"image")
  await turn()
  assert.equal(calls,0)
})

test("thumbnail downloads have bounded concurrency while generation defaults to one", async () => {
  const pool = new PelicanResourcePool<string>({maxBytes: 32, maxEntries: 8, size: value => value.length, maxConcurrent: 3})
  let active = 0, peak = 0
  const finishes: Array<() => void> = []
  const leases = [0, 1, 2, 3, 4].map(index => pool.acquire(String(index), () => new Promise<string>(resolve => {
    peak = Math.max(peak, ++active)
    finishes.push(() => { active--; resolve("ok") })
  })))
  await turn()
  assert.equal(active, 3)
  finishes.splice(0).forEach(finish => finish())
  await turn()
  assert.equal(active, 2)
  finishes.splice(0).forEach(finish => finish())
  await Promise.all(leases.map(lease => lease.promise))
  leases.forEach(lease => lease.release())
  assert.equal(peak, 3)
  assert.equal(active, 0)
})

test("缩略图串行生成，共享请求在最后一个使用者离开时取消", async () => {
  const pool = new PelicanResourcePool<string>({maxBytes: 8, maxEntries: 2, size: value => value.length})
  let active = 0, peak = 0, calls = 0
  const pending: Array<() => void> = []
  const load = (signal: AbortSignal) => new Promise<string>((resolve, reject) => {
    calls++; peak = Math.max(peak, ++active)
    const finish = () => { active--; resolve("data") }
    pending.push(finish)
    signal.addEventListener("abort", () => {active--; reject(signal.reason)}, {once: true})
  })
  const a = pool.acquire("a", load), shared = pool.acquire("a", load), b = pool.acquire("b", load)
  await turn()
  assert.equal(calls, 1)
  a.release()
  pending.shift()!()
  assert.equal(await shared.promise, "data")
  shared.release()
  await turn()
  const rejected = assert.rejects(b.promise, {name: "AbortError"})
  b.release()
  await rejected
  assert.equal(peak, 1)
  assert.equal(active, 0)
})

test("离屏排队的作品不再请求，取消后不占用缓存", async () => {
  const pool = new PelicanResourcePool<string>({maxBytes: 8, maxEntries: 2, size: value => value.length})
  let resolve!: (value: string) => void, calls = 0
  const first = pool.acquire("first", () => new Promise<string>(done => {resolve = done}))
  const queued = pool.acquire("queued", async () => {calls++; return "skip"})
  const rejected = assert.rejects(queued.promise, {name: "AbortError"})
  queued.release()
  await rejected
  await turn()
  resolve("one"); await first.promise; first.release()
  await turn()
  assert.equal(calls, 0)
  assert.equal(pool.cachedEntries, 1)
  pool.clear()
  assert.equal(pool.cachedBytes, 0)
})

test("缩略图缓存同时受总字节数和数量约束，失败可重新加载", async () => {
  const pool = new PelicanResourcePool<string>({maxBytes: 8, maxEntries: 2, size: value => value.length})
  let calls = 0
  async function read(key: string, value = "1234") {
    const lease = pool.acquire(key, async () => {calls++; return value})
    try {return await lease.promise} finally {lease.release()}
  }
  await read("a"); await read("b"); await read("a"); await read("c")
  assert.equal(calls, 3)
  assert.equal(pool.cachedBytes, 8)
  assert.equal(pool.cachedEntries, 2)
  await read("b")
  assert.equal(calls, 4)
  await read("large", "123456789")
  assert.ok(pool.cachedBytes <= 8)
  const failed = pool.acquire("fail", async () => {throw new Error("failed")})
  await assert.rejects(failed.promise, /failed/); failed.release()
  assert.equal(await read("fail"), "1234")
  pool.clear()
  assert.equal(pool.cachedEntries, 0)
})

test("超时的预览请求会中断，后面的作品可以继续生成", async () => {
  const pool = new PelicanResourcePool<string>({maxBytes: 8, maxEntries: 2, size: value => value.length, timeoutMs: 25})
  let aborted = false
  const slow = pool.acquire("slow", signal => new Promise<string>((_resolve, reject) => {
    signal.addEventListener("abort", () => {aborted = true; reject(signal.reason)}, {once: true})
  }))
  const next = pool.acquire("next", async () => "ok")
  await assert.rejects(slow.promise, {name: "TimeoutError"})
  assert.equal(await next.promise, "ok")
  assert.equal(aborted, true)
  slow.release(); next.release(); pool.clear()
})
