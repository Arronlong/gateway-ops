// @ts-expect-error Node 22 原生 TypeScript 测试需要显式扩展名。
import { getToken, notifyUnauthorized } from "./api.ts"
import type { UsageIntegrity } from "./usage-integrity"
export interface UsageIntegrityEvent {
  stage: string
  message: string
  ok?: boolean
  data?: unknown
}

export async function fetchUsageIntegrity(path: string, signal: AbortSignal, onEvent: (event: UsageIntegrityEvent) => void): Promise<UsageIntegrity> {
  signal.throwIfAborted()
  const controller = new AbortController()
  const abort = () => controller.abort(signal.reason)
  signal.addEventListener("abort", abort, { once: true })
  const cancelled = new Promise<never>((_, reject) => {
    controller.signal.addEventListener("abort", () => reject(controller.signal.reason), { once: true })
  })
  let timeout: ReturnType<typeof setTimeout> | undefined
  let reader: ReadableStreamDefaultReader<Uint8Array> | undefined
  const waitFor = <T,>(pending: Promise<T>): Promise<T> => {
    timeout = setTimeout(() => controller.abort(new Error("\u6838\u5bf9\u8fdb\u5ea6\u8d85\u65f6\uff0c\u8bf7\u91cd\u8bd5")), 60_000)
    return Promise.race([pending, cancelled]).finally(() => clearTimeout(timeout))
  }
  try {
    const token = getToken()
    const response = await waitFor(fetch(`/api${path}${path.includes("?") ? "&" : "?"}stream=1`, {
      method: "GET",
      signal: controller.signal,
      headers: { Accept: "text/event-stream", ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    }))
    if (response.status === 401) notifyUnauthorized()
    if (!response.ok) throw new Error(`\u6838\u5bf9\u8bf7\u6c42\u5931\u8d25 (${response.status})`)
    if (!response.headers.get("Content-Type")?.includes("text/event-stream")) return (await waitFor(response.json())).data
    if (!response.body) throw new Error("\u6d4f\u89c8\u5668\u4e0d\u652f\u6301\u6838\u5bf9\u8fdb\u5ea6\u6d41")
    reader = response.body.getReader()
    const decoder = new TextDecoder("utf-8")
    let buffer = ""
    let result: UsageIntegrity | null = null
    const consume = (block: string) => {
      const text = block.split("\n").filter(line => line.startsWith("data:")).map(line => line.slice(5).trimStart()).join("\n")
      if (!text) return
      const event = JSON.parse(text) as UsageIntegrityEvent
      onEvent(event)
      if (event.stage === "error" || event.ok === false) throw new Error(event.message)
      if (event.stage === "done" && event.data) result = event.data as UsageIntegrity
    }
    while (true) {
      const next = await waitFor(reader.read())
      if (next.done) break
      buffer += decoder.decode(next.value, { stream: true }).replace(/\r\n/g, "\n")
      let index: number
      while ((index = buffer.indexOf("\n\n")) >= 0) {
        consume(buffer.slice(0, index))
        buffer = buffer.slice(index + 2)
      }
    }
    if (buffer.trim()) consume(buffer)
    if (!result) throw new Error("\u6838\u5bf9\u8fde\u63a5\u5df2\u4e2d\u65ad\uff0c\u672a\u6536\u5230\u5b8c\u6210\u7ed3\u679c\uff0c\u8bf7\u91cd\u8bd5")
    return result
  } finally {
    clearTimeout(timeout)
    signal.removeEventListener("abort", abort)
    if (reader) {
      await reader.cancel().catch(() => {})
      reader.releaseLock()
    }
  }
}
