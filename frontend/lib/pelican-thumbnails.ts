import { createPelicanDocument, pelicanViewport } from "./pelican"
import { PelicanResourcePool } from "./pelican-resources"

export const pelicanThumbnailPool = new PelicanResourcePool<{image:Blob;saveError:string}>({
  maxBytes: 8 * 1024 * 1024, maxEntries: 96, maxConcurrent: 4, timeoutMs: 20000, size: value => value.image.size,
})

export const pelicanRenderPool = new PelicanResourcePool<Blob>({maxBytes: 0, maxEntries: 0, timeoutMs:15000, size: blob => blob.size})

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
    let snapshotReceived = false, settled = false, attempts = 0
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
    const fail = (error: unknown) => {if (!settled) {settled=true;cleanup();reject(error)}}
    const aborted = () => fail(signal.reason)
    const receive = (event: MessageEvent) => {
      if (event.source !== frame.contentWindow || event.data?.channel !== channel || event.data.type !== "pelican-thumbnail" || snapshotReceived) return
      snapshotReceived = true;attempts++
      if (typeof event.data.svg === "string" && event.data.svg.length <= 4 * 1024 * 1024) rasterizePelicanThumbnail(event.data.svg, signal).then(image=>{
        if(!settled){settled=true;cleanup();resolve(image)}
      },error=>{
        if(settled)return
        if(error instanceof BlankPelicanThumbnailError && attempts<4 && !signal.aborted){
          snapshotReceived=false;frame.contentWindow?.postMessage({type:"pelican-thumbnail-retry",channel},"*")
        }else fail(error)
      })
      else fail(new Error("\u9884\u89c8\u751f\u6210\u5931\u8d25\uff0c\u70b9\u51fb\u4f5c\u54c1\u67e5\u770b\u539f\u56fe"))
    }
    const timeout = window.setTimeout(() => fail(new Error("\u9884\u89c8\u751f\u6210\u8d85\u65f6\uff0c\u70b9\u51fb\u4f5c\u54c1\u67e5\u770b\u539f\u56fe")), 10000)
    signal.addEventListener("abort", aborted, {once: true}); window.addEventListener("message", receive)
    frame.srcdoc = createPelicanDocument(html, channel, {thumbnail: true})
    document.body.append(frame)
  })
}

async function decodePelicanImage(source: string, signal: AbortSignal): Promise<HTMLImageElement> {
  signal.throwIfAborted()
  const picture = new Image()
  await new Promise<void>((resolve,reject) => {
    const cleanup = () => {window.clearTimeout(timeout);signal.removeEventListener("abort",aborted);picture.onload=null;picture.onerror=null}
    const aborted = () => {cleanup();picture.src="";reject(signal.reason)}
    const timeout = window.setTimeout(() => {cleanup();picture.src="";reject(new Error("\u7f29\u7565\u56fe\u538b\u7f29\u8d85\u65f6"))},5000)
    picture.onload=()=>{cleanup();resolve()}
    picture.onerror=()=>{cleanup();reject(new Error("\u7f29\u7565\u56fe\u89e3\u7801\u5931\u8d25"))}
    signal.addEventListener("abort",aborted,{once:true})
    picture.src=source
  })
  signal.throwIfAborted()
  return picture
}

class BlankPelicanThumbnailError extends Error {
  constructor(){super("\u4f5c\u54c1\u6682\u672a\u7ed8\u5236\u51fa\u753b\u9762\uff0c\u8bf7\u7a0d\u540e\u91cd\u8bd5")}
}

function assertPelicanImageContent(context: CanvasRenderingContext2D, width: number, height: number) {
  const pixels=context.getImageData(0,0,width,height).data
  const minimum=Math.max(4,Math.floor(width*height*.0001))
  let colored=0
  for(let index=0;index<pixels.length;index+=4){
    if(pixels[index+3]>0 && Math.min(pixels[index],pixels[index+1],pixels[index+2])<248 && ++colored>=minimum)return
  }
  throw new BlankPelicanThumbnailError()
}

export async function verifyPelicanThumbnail(image: Blob, signal: AbortSignal): Promise<void> {
  const source=URL.createObjectURL(image)
  try {
    const picture=await decodePelicanImage(source,signal)
    const canvas=document.createElement("canvas");canvas.width=64;canvas.height=48
    const context=canvas.getContext("2d",{willReadFrequently:true})
    if(!context)throw new Error("\u6d4f\u89c8\u5668\u65e0\u6cd5\u68c0\u67e5\u7f29\u7565\u56fe")
    context.fillStyle="#ffffff";context.fillRect(0,0,64,48);context.drawImage(picture,0,0,64,48)
    assertPelicanImageContent(context,64,48)
  } finally {URL.revokeObjectURL(source)}
}

export async function rasterizePelicanThumbnail(svg: string, signal: AbortSignal): Promise<Blob> {
  const picture=await decodePelicanImage(`data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`,signal)
  const canvas=document.createElement("canvas")
  canvas.width=448;canvas.height=336
  const context=canvas.getContext("2d")
  if (!context) throw new Error("\u6d4f\u89c8\u5668\u65e0\u6cd5\u751f\u6210\u7f29\u7565\u56fe")
  context.fillStyle="#ffffff";context.fillRect(0,0,448,336)
  const scale=Math.min(448/picture.naturalWidth,336/picture.naturalHeight)
  const width=picture.naturalWidth*scale,height=picture.naturalHeight*scale
  context.drawImage(picture,(448-width)/2,(336-height)/2,width,height)
  assertPelicanImageContent(context,448,336)
  for (const quality of [.82,.72,.6,.45]) {
    signal.throwIfAborted()
    const image=await new Promise<Blob|null>(resolve=>canvas.toBlob(resolve,"image/jpeg",quality))
    if (image && image.size<=96*1024) return image
  }
  throw new Error("\u7f29\u7565\u56fe\u65e0\u6cd5\u538b\u7f29\u5230 96 KB \u4ee5\u5185")
}
