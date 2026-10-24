import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22
import { PelicanResourcePool } from "./pelican-resources.ts"

const turn = () => new Promise(resolve => setTimeout(resolve, 0))

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
