export type ProductBranding = { product_name: string; product_icon: string }
export const defaultProductName = "GatewayOps"
export const defaultProductIcon = `data:image/svg+xml,${encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="8" fill="#171717"/><svg x="7" y="7" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M22 12h-4l-3 9L9 3l-3 9H2"/></svg></svg>')}`

export function normalizeBranding(value: Partial<ProductBranding> = {}): ProductBranding {
  return {product_name:value.product_name?.trim() || defaultProductName, product_icon:value.product_icon || ""}
}

export function brandingPageTitle(page: string, product: string) {
  const name = normalizeBranding({product_name:product}).product_name
  return page ? `${page} · ${name}` : name
}

export function validateBrandingUpload(file: Pick<File,"type"|"size">): string | null {
  if(!["image/png","image/jpeg","image/webp"].includes(file.type)) return "请选择 PNG、JPEG 或 WebP 图片"
  if(file.size===0 || file.size>1024*1024) return "图片大小需为 1 字节到 1 MiB"
  return null
}

export async function prepareProductIcon(file: File): Promise<string> {
  const error = validateBrandingUpload(file)
  if(error) throw new Error(error)
  let image: ImageBitmap
  try { image=await createImageBitmap(file) } catch { throw new Error("图片无法读取，请选择有效图片") }
  try {
    const canvas=document.createElement("canvas")
    canvas.width=128;canvas.height=128
    const context=canvas.getContext("2d")
    if(!context) throw new Error("当前浏览器无法处理图片")
    const scale=Math.min(128/image.width,128/image.height)
    const width=image.width*scale,height=image.height*scale
    context.drawImage(image,(128-width)/2,(128-height)/2,width,height)
    return canvas.toDataURL("image/png")
  } finally { image.close() }
}
