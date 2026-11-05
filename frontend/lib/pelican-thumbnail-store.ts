// @ts-expect-error Node 22 原生 TypeScript 测试需要显式扩展名。
import { getToken, notifyUnauthorized } from "./api.ts"

export const pelicanThumbnailMaxBytes = 96 * 1024

function thumbnailHeaders() {
  const headers = new Headers()
  const token = getToken()
  if (token) headers.set("Authorization", `Bearer ${token}`)
  return headers
}

function validateThumbnail(image: Blob) {
  if (image.type !== "image/jpeg" || image.size < 1 || image.size > pelicanThumbnailMaxBytes) throw new Error("\u7f29\u7565\u56fe\u5fc5\u987b\u662f\u4e0d\u8d85\u8fc7 96 KB \u7684 JPEG \u56fe\u7247")
}

export async function loadPelicanThumbnail(path: string, revision: string, signal: AbortSignal, create: (signal: AbortSignal) => Promise<{image: Blob; html: string}>, writable: boolean): Promise<Blob> {
  const url = `/api${path}/thumbnail`
  const response = await fetch(`${url}?v=${encodeURIComponent(revision)}`, {signal, headers: thumbnailHeaders()})
  if (response.status === 401) notifyUnauthorized()
  if (response.ok) {
    const image = await response.blob()
    validateThumbnail(image)
    return image
  }
  if (response.status !== 404) throw new Error(`\u7f29\u7565\u56fe\u8bfb\u53d6\u5931\u8d25 (${response.status})`)
  if (!writable) throw new Error("\u9884\u89c8\u5c1a\u672a\u751f\u6210\uff0c\u70b9\u51fb\u67e5\u770b\u4f5c\u54c1")
  const generated = await create(signal)
  signal.throwIfAborted()
  validateThumbnail(generated.image)
  if (writable) {
    const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(generated.html))
    const headers = thumbnailHeaders()
    headers.set("Content-Type", "image/jpeg")
    headers.set("X-Pelican-Source-SHA256", Array.from(new Uint8Array(digest), value => value.toString(16).padStart(2, "0")).join(""))
    try {
      const saved = await fetch(url, {method:"PUT", signal, headers, body:generated.image})
      if (saved.status === 401) notifyUnauthorized()
      if (!saved.ok) console.warn("\u7f29\u7565\u56fe\u672a\u80fd\u4fdd\u5b58\uff0c\u4e0b\u6b21\u8bbf\u95ee\u65f6\u5c06\u91cd\u8bd5", saved.status)
    } catch (error) { if (signal.aborted) throw error }
  }
  return generated.image
}
