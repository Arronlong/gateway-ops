import assert from "node:assert/strict"
import test from "node:test"

// @ts-expect-error Node 22 原生 TypeScript 测试需要显式 .ts 扩展名。
import { apiFetch } from "./api.ts"

test("does not retry an aborted GET request", async () => {
  const originalFetch = globalThis.fetch
  let calls = 0
  globalThis.fetch = (async () => {
    calls += 1
    throw new TypeError("fetch failed")
  }) as typeof fetch

  try {
    await assert.rejects(
      apiFetch("/slow-endpoint", { signal: AbortSignal.abort() }),
      /fetch failed/,
    )
    assert.equal(calls, 1)
  } finally {
    globalThis.fetch = originalFetch
  }
})

test("passes an AbortSignal through to an in-flight GET request", async () => {
  const originalFetch = globalThis.fetch
  let calls = 0
  let receivedSignal: AbortSignal | undefined
  globalThis.fetch = ((_, init) => {
    calls += 1
    receivedSignal = init?.signal ?? undefined
    return new Promise<Response>((_resolve, reject) => {
      init?.signal?.addEventListener("abort", () => {
        reject(new DOMException("The operation was aborted", "AbortError"))
      }, { once: true })
    })
  }) as typeof fetch

  const controller = new AbortController()
  try {
    const request = apiFetch("/slow-endpoint", { signal: controller.signal })
    controller.abort()
    await assert.rejects(request, (error: unknown) => error instanceof DOMException && error.name === "AbortError")
    assert.equal(calls, 1)
    assert.equal(receivedSignal, controller.signal)
  } finally {
    globalThis.fetch = originalFetch
  }
})

test("stops the retry delay when the request is aborted", async () => {
  const originalFetch = globalThis.fetch
  const controller = new AbortController()
  let calls = 0
  globalThis.fetch = (async () => {
    calls += 1
    if (calls === 1) {
      setTimeout(() => controller.abort(), 10)
      throw new TypeError("temporary network failure")
    }
    return new Response("{}", { status: 200, headers: { "Content-Type": "application/json" } })
  }) as typeof fetch

  try {
    await assert.rejects(apiFetch("/slow-endpoint", { signal: controller.signal }), (error: unknown) => error instanceof DOMException && error.name === "AbortError")
    assert.equal(calls, 1)
  } finally {
    globalThis.fetch = originalFetch
  }
})
