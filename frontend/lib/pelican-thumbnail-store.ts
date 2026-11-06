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

async function thumbnailResponseError(response: Response, operation: string) {
  let detail=""
  try {
    const body=await response.json()
    if(typeof body?.error==="string")detail=body.error.slice(0,200)
  } catch {}
  return Object.assign(new Error(`${operation} (${response.status}${detail?`: ${detail}`:""})`),{status:response.status})
}

function waitForThumbnailRetry(signal: AbortSignal) {
  signal.throwIfAborted()
  return new Promise<void>((resolve,reject)=>{
    const aborted=()=>{clearTimeout(timer);reject(signal.reason)}
    const timer=setTimeout(()=>{signal.removeEventListener("abort",aborted);resolve()},450)
    signal.addEventListener("abort",aborted,{once:true})
  })
}

async function persistPelicanThumbnail(url: string, revision: string, generated: {image: Blob; html: string}, signal: AbortSignal, verify?: (image: Blob, signal: AbortSignal) => Promise<void>) {
  const digest=await crypto.subtle.digest("SHA-256",new TextEncoder().encode(generated.html))
  for(let attempt=0;attempt<2;attempt++) {
    try {
      signal.throwIfAborted()
      const headers=thumbnailHeaders()
      headers.set("Content-Type","image/jpeg")
      headers.set("X-Pelican-Source-SHA256",Array.from(new Uint8Array(digest),value=>value.toString(16).padStart(2,"0")).join(""))
      const saved=await fetch(url,{method:"PUT",signal,headers,body:generated.image})
      if(saved.status===401)notifyUnauthorized()
      if(!saved.ok)throw await thumbnailResponseError(saved,"\u7f29\u7565\u56fe\u4e0a\u4f20\u5931\u8d25")
      const confirmed=await fetch(`${url}?v=${encodeURIComponent(revision)}`,{signal,cache:"no-store",headers:thumbnailHeaders()})
      if(confirmed.status===401)notifyUnauthorized()
      if(!confirmed.ok) {
        const error=await thumbnailResponseError(confirmed,"\u7f29\u7565\u56fe\u4e0a\u4f20\u540e\u56de\u8bfb\u5931\u8d25")
        throw Object.assign(error,{retryable:confirmed.status===404})
      }
      const image=await confirmed.blob()
      validateThumbnail(image)
      await verify?.(image,signal)
      signal.throwIfAborted()
      return
    } catch(error) {
      signal.throwIfAborted()
      const failure=error as {status?:number;retryable?:boolean}
      if(attempt===1 || (failure.status!==undefined && failure.status<500 && failure.status!==408 && failure.status!==429 && !failure.retryable))throw error
      await waitForThumbnailRetry(signal)
    }
  }
}

export async function loadPelicanThumbnail(path: string, revision: string, signal: AbortSignal, create: (signal: AbortSignal) => Promise<{image: Blob; html: string}>, writable: boolean, verify?: (image: Blob, signal: AbortSignal) => Promise<void>, onSaveError?: (error: Error) => void): Promise<Blob> {
  const url = `/api${path}/thumbnail`
  const response = await fetch(`${url}?v=${encodeURIComponent(revision)}`, {signal, cache:"no-cache", headers: thumbnailHeaders()})
  if (response.status === 401) notifyUnauthorized()
  if (response.ok) {
    const image = await response.blob()
    try {
      validateThumbnail(image)
      await verify?.(image, signal)
      signal.throwIfAborted()
      return image
    } catch { signal.throwIfAborted() }
  }
  if (!response.ok && response.status !== 404) throw await thumbnailResponseError(response,"\u7f29\u7565\u56fe\u8bfb\u53d6\u5931\u8d25")
  const generated = await create(signal)
  signal.throwIfAborted()
  validateThumbnail(generated.image)
  await verify?.(generated.image, signal)
  signal.throwIfAborted()
  if (writable) {
    try {
      await persistPelicanThumbnail(url,revision,generated,signal,verify)
    } catch (error) {
      signal.throwIfAborted()
      const failure=error instanceof Error?error:new Error(String(error))
      onSaveError?.(failure)
      console.warn("\u7f29\u7565\u56fe\u672a\u4fdd\u5b58",failure.message)
    }
  }
  return generated.image
}
