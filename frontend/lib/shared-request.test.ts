import assert from "node:assert/strict"
import test from "node:test"

// @ts-expect-error Node 22 原生 TypeScript 测试需要显式 .ts 扩展名。
import { createSharedRequestRegistry } from "./shared-request.ts"

test("aborts a shared request after its last subscriber releases", async () => {
  const registry = createSharedRequestRegistry<string>()
  let aborted = false
  const request = registry.acquire("relay-stations", (signal) => new Promise<string>((resolve, reject) => {
    signal.addEventListener("abort", () => {
      aborted = true
      reject(new Error("aborted"))
    }, { once: true })
  }))

  request.release()

  await assert.rejects(request.promise, /aborted/)
  assert.equal(aborted, true)
})

test("keeps a shared request alive until every subscriber releases", async () => {
  const registry = createSharedRequestRegistry<string>()
  let resolveRequest!: (value: string) => void
  let aborted = false
  const run = (signal: AbortSignal) => new Promise<string>((resolve, reject) => {
    resolveRequest = resolve
    signal.addEventListener("abort", () => {
      aborted = true
      reject(new Error("aborted"))
    }, { once: true })
  })

  const first = registry.acquire("same-request", run)
  const second = registry.acquire("same-request", run)
  first.release()
  assert.equal(aborted, false)

  resolveRequest("ok")
  assert.equal(await first.promise, "ok")
  assert.equal(await second.promise, "ok")

  second.release()
  assert.equal(aborted, false)
})

test("reuses the request when a subscriber returns before cancellation runs", async () => {
  const registry = createSharedRequestRegistry<string>()
  let starts = 0
  let resolveRequest!: (value: string) => void
  const run = () => {
    starts += 1
    return new Promise<string>((resolve) => { resolveRequest = resolve })
  }

  const first = registry.acquire("same-request", run)
  first.release()
  const second = registry.acquire("same-request", run)
  assert.equal(starts, 1)

  resolveRequest("ok")
  assert.equal(await second.promise, "ok")
  second.release()
})

test("cancels every active request when navigation starts", async () => {
  const registry = createSharedRequestRegistry<string>()
  let aborted = 0
  const run = (signal: AbortSignal) => new Promise<string>((_resolve, reject) => {
    signal.addEventListener("abort", () => {
      aborted += 1
      reject(new Error("aborted"))
    }, { once: true })
  })

  const first = registry.acquire("first", run)
  const second = registry.acquire("second", run)
  registry.cancelAll()

  await assert.rejects(first.promise, /aborted/)
  await assert.rejects(second.promise, /aborted/)
  assert.equal(aborted, 2)
})

test("navigation preserves a request needed by a persistent subscriber", async () => {
  const registry = createSharedRequestRegistry<string>()
  let resolveRequest!: (value: string) => void
  let aborted = false
  const run = (signal: AbortSignal) => new Promise<string>((resolve, reject) => {
    resolveRequest = resolve
    signal.addEventListener("abort", () => {
      aborted = true
      reject(new Error("aborted"))
    }, { once: true })
  })

  const header = registry.acquire("channels", run, { cancelOnNavigation: false })
  const page = registry.acquire("channels", run)
  registry.cancelAll()
  await Promise.resolve()
  assert.equal(aborted, false)

  resolveRequest("ok")
  assert.equal(await header.promise, "ok")
  assert.equal(await page.promise, "ok")
  page.release()
  header.release()
})
