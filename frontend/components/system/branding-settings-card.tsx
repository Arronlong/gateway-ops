import { useRef, useState } from "react"
import { ImagePlus, RotateCcw, Save } from "lucide-react"
import type { SystemSettings } from "@/lib/api-types"
import { normalizeBranding, prepareProductIcon } from "@/lib/branding"
import { ProductIcon } from "@/components/product-branding"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

export function BrandingSettingsCard({settings,saving,onChange,onSave}:{settings:SystemSettings;saving:boolean;onChange:(update:(current:SystemSettings)=>SystemSettings)=>void;onSave:()=>void}){
  const [error,setError]=useState("")
  const [uploading,setUploading]=useState(false)
  const input=useRef<HTMLInputElement>(null)
  const generation=useRef(0)
  const branding=normalizeBranding(settings)
  const invalid=Array.from((settings.product_name??"").trim()).length>40||/[\u0000-\u001f\u007f]/.test(settings.product_name??"")
  async function upload(file:File){
    const current=++generation.current
    setError("");setUploading(true)
    try{const icon=await prepareProductIcon(file);if(current===generation.current)onChange(value=>({...value,product_icon:icon}))}
    catch(error){if(current===generation.current)setError(error instanceof Error?error.message:"图片读取失败")}
    finally{if(current===generation.current)setUploading(false)}
  }
  return <Card id="branding-settings" className="scroll-mt-20 gap-0 border border-border py-0 shadow-none">
    <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-3 border-b border-border px-4 py-3">
      <div><CardTitle className="flex items-center gap-2 text-sm font-semibold"><ProductIcon icon={branding.product_icon} className="size-4"/>品牌与产品名称</CardTitle><p className="mt-1 text-xs text-muted-foreground">保存后应用于所有页面和浏览器标签。</p></div>
      <Button size="sm" className="h-9 gap-1.5" disabled={saving||uploading||invalid} onClick={onSave}><Save className="size-3.5"/>{saving?"保存中…":"保存品牌设置"}</Button>
    </CardHeader>
    <CardContent className="grid min-w-0 gap-5 p-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)]">
      <div className="min-w-0 space-y-4">
        <div className="space-y-2"><Label htmlFor="product-name">产品名称</Label><Input id="product-name" value={settings.product_name??"GatewayOps"} placeholder="GatewayOps" disabled={saving} aria-invalid={invalid} onChange={event=>onChange(current=>({...current,product_name:event.target.value}))}/>{invalid?<p role="alert" className="text-xs text-danger">产品名称最多 40 个字符，不能包含控制字符。</p>:null}</div>
        <div className="space-y-2"><Label>产品图标</Label><div className="flex items-center gap-3 rounded-md border border-border p-3"><ProductIcon icon={branding.product_icon} className="size-12 rounded-lg"/><div className="min-w-0 text-xs"><p className="font-medium">{branding.product_icon?"自定义图标":"首页默认图标"}</p><p className="mt-1 text-muted-foreground">PNG、JPEG、WebP，最大 1 MiB</p></div></div>
          <input ref={input} type="file" accept="image/png,image/jpeg,image/webp" aria-label="选择产品图标" className="sr-only" onChange={event=>{const file=event.target.files?.[0];event.target.value="";if(file)void upload(file)}}/>
          <div className="flex flex-wrap gap-2"><Button type="button" size="sm" variant="outline" disabled={saving||uploading} onClick={()=>input.current?.click()}><ImagePlus className="size-3.5"/>{uploading?"处理图片…":"上传图标"}</Button><Button type="button" size="sm" variant="outline" disabled={saving||!branding.product_icon} onClick={()=>{generation.current++;setUploading(false);setError("");onChange(current=>({...current,product_icon:""}))}}><RotateCcw className="size-3.5"/>恢复首页默认图标</Button></div>
          {error?<p role="alert" className="text-xs text-danger">{error}</p>:null}
        </div>
      </div>
      <div className="min-w-0 space-y-3"><p className="text-xs font-medium text-muted-foreground">效果预览</p><div className="overflow-hidden rounded-lg border border-border"><div className="flex min-w-0 items-center gap-2.5 border-b border-border p-3"><ProductIcon icon={branding.product_icon} className="size-8"/><span className="truncate text-sm font-semibold">{branding.product_name}</span></div><div className="space-y-3 p-4"><p className="flex items-center gap-2 text-base font-bold"><ProductIcon icon={branding.product_icon}/>系统设置</p><p className="text-xs text-muted-foreground">页面标题统一使用产品图标，功能区保留业务图标。</p></div></div><div className="rounded-lg border border-border bg-muted/30 p-3"><p className="mb-2 text-xs text-muted-foreground">浏览器标签</p><div className="inline-flex max-w-full items-center gap-2 rounded-md border border-border bg-background px-3 py-2 text-xs"><ProductIcon icon={branding.product_icon} className="size-4"/><span className="truncate">系统设置 · {branding.product_name}</span></div></div></div>
    </CardContent>
  </Card>
}
