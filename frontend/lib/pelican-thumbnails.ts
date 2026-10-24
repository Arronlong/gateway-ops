import { createPelicanDocument, pelicanViewport } from "./pelican"
import { PelicanResourcePool } from "./pelican-resources"

export const pelicanThumbnailPool = new PelicanResourcePool<Blob>({
  maxBytes: 8 * 1024 * 1024, maxEntries: 48, size: blob => blob.size,
})

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
      if (event.source !== frame.contentWindow || event.data?.channel !== channel || event.data.type !== "pelican-thumbnail") return
      cleanup()
      if (typeof event.data.svg === "string" && event.data.svg.length <= 4 * 1024 * 1024) resolve(new Blob([event.data.svg], {type: "image/svg+xml"}))
      else reject(new Error("预览生成失败，点击作品查看原图"))
    }
    const timeout = window.setTimeout(() => {cleanup(); reject(new Error("预览生成超时，点击作品查看原图"))}, 5000)
    signal.addEventListener("abort", aborted, {once: true}); window.addEventListener("message", receive)
    frame.srcdoc = createPelicanDocument(html, channel, {thumbnail: true})
    document.body.append(frame)
  })
}
