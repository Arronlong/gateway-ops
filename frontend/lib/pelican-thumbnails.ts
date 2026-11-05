import { createPelicanDocument, pelicanViewport } from "./pelican"
import { PelicanResourcePool } from "./pelican-resources"

export const pelicanThumbnailPool = new PelicanResourcePool<Blob>({
  maxBytes: 8 * 1024 * 1024, maxEntries: 96, maxConcurrent: 4, timeoutMs: 20000, size: blob => blob.size,
})

export const pelicanRenderPool = new PelicanResourcePool<Blob>({maxBytes: 0, maxEntries: 0, size: blob => blob.size})

let owners = 0, previews = 0
const activityListeners = new Set<() => void>()
const notifyActivity = () => {for (const listener of activityListeners) listener()}
export const pelicanThumbnailActive = () => !document.hidden && previews === 0
export function subscribePelicanActivity(listener: () => void) {
  if (!activityListeners.size) document.addEventListener("visibilitychange", notifyActivity)
  activityListeners.add(listener)
  return () => {
    activityListeners.delete(listener)
    if (!activityListeners.size) document.removeEventListener("visibilitychange", notifyActivity)
  }
}
export function retainPelicanThumbnails() {
  owners++
  return () => {if (--owners === 0) pelicanThumbnailPool.clear()}
}
export function suspendPelicanThumbnails() {
  previews++; notifyActivity()
  return () => {previews--; notifyActivity()}
}

// 共用观察器，同时考虑页面纵向滚动和作品行的横向裁切。
const visibleTargets = new Map<Element, (visible: boolean) => void>()
let visibilityObserver: IntersectionObserver | null = null
export function observePelicanThumbnail(target: Element, listener: (visible: boolean) => void) {
  if (!visibilityObserver) visibilityObserver = new IntersectionObserver(entries => {
    for (const entry of entries) visibleTargets.get(entry.target)?.(entry.isIntersecting)
  })
  visibleTargets.set(target, listener); visibilityObserver.observe(target)
  return () => {
    visibilityObserver?.unobserve(target); visibleTargets.delete(target)
    if (!visibleTargets.size) {visibilityObserver?.disconnect(); visibilityObserver = null}
  }
}

// 只在生成快照时短暂运行一个沙箱；完成、取消、超时都销毁文档。
export function renderPelicanThumbnail(html: string, signal: AbortSignal): Promise<Blob> {
  return new Promise((resolve, reject) => {
    signal.throwIfAborted()
    const frame = document.createElement("iframe"), channel = crypto.randomUUID()
    let snapshotReceived = false
    const viewport = pelicanViewport(html)
    frame.title = "生成作品静态预览"
    frame.dataset.pelicanRenderer = ""
    frame.setAttribute("sandbox", "allow-scripts"); frame.setAttribute("aria-hidden", "true")
    frame.referrerPolicy = "no-referrer"; frame.tabIndex = -1
    frame.style.cssText = `position:fixed;left:-10000px;top:0;width:${viewport.width}px;height:${viewport.height}px;border:0;pointer-events:none;contain:strict`
    const cleanup = () => {
      window.clearTimeout(timeout); window.removeEventListener("message", receive)
      signal.removeEventListener("abort", aborted); frame.remove(); frame.srcdoc = ""
    }
    const aborted = () => {cleanup(); reject(signal.reason)}
    const receive = (event: MessageEvent) => {
      if (event.source !== frame.contentWindow || event.data?.channel !== channel || event.data.type !== "pelican-thumbnail" || snapshotReceived) return
      snapshotReceived = true
      cleanup()
      if (typeof event.data.svg === "string" && event.data.svg.length <= 4 * 1024 * 1024) rasterizePelicanThumbnail(event.data.svg, signal).then(resolve,reject)
      else reject(new Error("预览生成失败，点击作品查看原图"))
    }
    const timeout = window.setTimeout(() => {cleanup(); reject(new Error("预览生成超时，点击作品查看原图"))}, 5000)
    signal.addEventListener("abort", aborted, {once: true}); window.addEventListener("message", receive)
    frame.srcdoc = createPelicanDocument(html, channel, {thumbnail: true})
    document.body.append(frame)
  })
}

export async function rasterizePelicanThumbnail(svg: string, signal: AbortSignal): Promise<Blob> {
  signal.throwIfAborted()
  const picture = new Image()
  await new Promise<void>((resolve,reject) => {
    const cleanup = () => {window.clearTimeout(timeout);signal.removeEventListener("abort",aborted);picture.onload=null;picture.onerror=null}
    const aborted = () => {cleanup();picture.src="";reject(signal.reason)}
    const timeout = window.setTimeout(() => {cleanup();picture.src="";reject(new Error("\u7f29\u7565\u56fe\u538b\u7f29\u8d85\u65f6"))},5000)
    picture.onload=()=>{cleanup();resolve()}
    picture.onerror=()=>{cleanup();reject(new Error("\u7f29\u7565\u56fe\u89e3\u7801\u5931\u8d25"))}
    signal.addEventListener("abort",aborted,{once:true})
    picture.src=`data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`
  })
  signal.throwIfAborted()
  const canvas=document.createElement("canvas")
  canvas.width=448;canvas.height=336
  const context=canvas.getContext("2d")
  if (!context) throw new Error("\u6d4f\u89c8\u5668\u65e0\u6cd5\u751f\u6210\u7f29\u7565\u56fe")
  context.fillStyle="#ffffff";context.fillRect(0,0,448,336)
  const scale=Math.min(448/picture.naturalWidth,336/picture.naturalHeight)
  const width=picture.naturalWidth*scale,height=picture.naturalHeight*scale
  context.drawImage(picture,(448-width)/2,(336-height)/2,width,height)
  for (const quality of [.82,.72,.6,.45]) {
    signal.throwIfAborted()
    const image=await new Promise<Blob|null>(resolve=>canvas.toBlob(resolve,"image/jpeg",quality))
    if (image && image.size<=96*1024) return image
  }
  throw new Error("\u7f29\u7565\u56fe\u65e0\u6cd5\u538b\u7f29\u5230 96 KB \u4ee5\u5185")
}
