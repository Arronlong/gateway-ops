import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from "react"
import { Bird, Download, Pause, Play, RotateCcw, Settings2, Trash2 } from "lucide-react"
import { toast } from "sonner"
import { Link } from "react-router-dom"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Switch } from "@/components/ui/switch"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { useConfirm } from "@/components/ui/confirm-dialog"
import { apiFetch } from "@/lib/api"
import type { PelicanConfig, PelicanGallery, PelicanResult, RelayAccountView } from "@/lib/api-types"
import { pelicanEfforts } from "@/lib/pelican"
import { usePelicanGallery, useSystemSettings } from "@/lib/queries"
import { PelicanArtwork } from "./pelican-artwork"

const timeLabel=(value:string)=>new Date(value).toLocaleString("zh-CN",{month:"2-digit",day:"2-digit",hour:"2-digit",minute:"2-digit",hour12:false,timeZone:"Asia/Shanghai"})
const running=(config?:PelicanConfig)=>Boolean(config?.lease_until&&Date.parse(config.lease_until)>Date.now())
type PelicanContextValue={
 stationID:number|null; data:PelicanGallery|null; error:string|null
 open:(result:PelicanResult)=>void
 toggle:(account:RelayAccountView,enabled:boolean)=>void; run:(accountID:number)=>Promise<void>
 body:(result:PelicanResult)=>Promise<PelicanResult>
 remove:(result:PelicanResult)=>Promise<void>; deletingID:number|null
}
const PelicanContext=createContext<PelicanContextValue|null>(null)
function usePelican(){const context=useContext(PelicanContext);if(!context)throw new Error("PelicanProvider required");return context}

export function PelicanProvider({stationID,children}:{stationID:number|null;children:ReactNode}) {
 const query=usePelicanGallery(stationID)
 const [selected,setSelected]=useState<PelicanResult|null>(null)
 const [actionError,setActionError]=useState<string|null>(null)
 const {confirm,dialog:deleteDialog}=useConfirm()
 const [deletingID,setDeletingID]=useState<number|null>(null)
 const [removedIDs,setRemovedIDs]=useState<Set<number>>(()=>new Set())
 const deleting=useRef(false)
 const requests=useRef(new Map<string,Promise<PelicanResult>>())
 useEffect(()=>{
  if(!query.data)return
  const kept=new Set(query.data.results.map(result=>`${result.relay_station_id}/${result.account_external_id}/${result.id}`))
  for(const key of requests.current.keys()){if(!kept.has(key))requests.current.delete(key)}
 },[query.data])
 const refetch=useRef(query.refetch);refetch.current=query.refetch
 const anyRunning=query.data?.configs.some(running)
 useEffect(()=>{
  if(!anyRunning)return
  const timer=window.setInterval(()=>{if(!document.hidden)void refetch.current().catch(()=>{})},5000)
  return()=>window.clearInterval(timer)
 },[anyRunning,stationID])
 const run=async(accountID:number)=>{
  if(stationID==null)return
  setActionError(null)
  try{await apiFetch(`/relay-stations/${stationID}/accounts/${accountID}/pelican/run`,{method:"POST"});await query.refetch()}
  catch(error){setActionError(error instanceof Error?error.message:"启动测智失败")}
 }
 const toggle=async(account:RelayAccountView,enabled:boolean)=>{
  if(stationID==null)return
  setActionError(null)
  try{await apiFetch(`/relay-stations/${stationID}/accounts/${account.external_id}/pelican`,{method:"PUT",body:JSON.stringify({enabled})});await query.refetch()}
  catch(error){setActionError(error instanceof Error?error.message:"保存失败")}
 }
 const body=(result:PelicanResult)=>{
  const key=`${result.relay_station_id}/${result.account_external_id}/${result.id}`
  let promise=requests.current.get(key)
  if(!promise){promise=apiFetch<PelicanResult>(`/relay-stations/${result.relay_station_id}/accounts/${result.account_external_id}/pelican/results/${result.id}`).catch(error=>{requests.current.delete(key);throw error});requests.current.set(key,promise)}
  return promise
 }
 const remove=async(result:PelicanResult)=>{
  if(deleting.current)return
  deleting.current=true
  try{
   const accepted=await confirm({title:"\u5220\u9664\u8fd9\u4efd\u6d4b\u667a\u4f5c\u54c1\uff1f",description:`${result.account_name} \u00b7 ${result.model} \u00b7 ${timeLabel(result.finished_at)}\u3002\u5c06\u6c38\u4e45\u5220\u9664\u672c\u6b21\u751f\u6210\u7684\u4f5c\u54c1\u53ca\u7ed3\u679c\uff0c\u4e0d\u5f71\u54cd\u8d26\u53f7\u914d\u7f6e\u548c\u5176\u4ed6\u5386\u53f2\u8bb0\u5f55\u3002\u64cd\u4f5c\u4e0d\u53ef\u6062\u590d\u3002`,confirmLabel:"\u786e\u8ba4\u5220\u9664",destructive:true})
   if(!accepted)return
   setDeletingID(result.id)
   await apiFetch(`/relay-stations/${result.relay_station_id}/accounts/${result.account_external_id}/pelican/results/${result.id}`,{method:"DELETE"})
   requests.current.delete(`${result.relay_station_id}/${result.account_external_id}/${result.id}`)
   setRemovedIDs(current=>new Set(current).add(result.id))
   setSelected(current=>current?.id===result.id?null:current)
   toast.success("\u6d4b\u667a\u4f5c\u54c1\u5df2\u5220\u9664")
   try{await query.refetch()}catch{toast.warning("\u4f5c\u54c1\u5df2\u5220\u9664\uff0c\u4f46\u5237\u65b0\u5217\u8868\u5931\u8d25\uff0c\u8bf7\u7a0d\u540e\u5237\u65b0\u3002")}
  }catch(error){toast.error(error instanceof Error?error.message:"\u5220\u9664\u4f5c\u54c1\u5931\u8d25")}
  finally{setDeletingID(null);deleting.current=false}
 }
 const data=query.data?{...query.data,results:query.data.results.filter(result=>!removedIDs.has(result.id))}:null
 return <PelicanContext.Provider value={{stationID,data,error:actionError??query.error,open:setSelected,toggle:(account,enabled)=>void toggle(account,enabled),run,body,remove,deletingID}}>
  {children}
  {selected?<PelicanPreview key={selected.id} result={selected} onClose={()=>setSelected(null)}/>:null}
  {deleteDialog}
 </PelicanContext.Provider>
}

export function PelicanDashboard({accounts}:{accounts:RelayAccountView[]}) {
 return <div className="min-w-0 space-y-4" aria-label="账号鹈鹕测智总览">
  <div className="flex flex-wrap items-center justify-between gap-2"><p className="text-xs leading-5 text-muted-foreground">{"\u6bcf\u4e2a\u8d26\u53f7\u7684\u5b9a\u65f6\u5f00\u5173\u9ed8\u8ba4\u5173\u95ed\uff0c\u6d4b\u667a\u53c2\u6570\u5728\u7cfb\u7edf\u8bbe\u7f6e\u4e2d\u7edf\u4e00\u914d\u7f6e\u3002\u6bcf\u884c\u5c55\u793a\u6700\u8fd1 30 \u4efd\u7ed3\u679c\uff0c\u70b9\u51fb\u4f5c\u54c1\u53ef\u4ea4\u4e92\u64ad\u653e\u3002"}</p><Button asChild variant="outline" size="sm" className="h-8 gap-1 text-xs"><Link to="/settings#pelican-settings"><Settings2 className="size-3" />{"\u5168\u5c40\u6d4b\u667a\u8bbe\u7f6e"}</Link></Button></div>
  {accounts.length?accounts.map(account=><section key={account.external_id} className="min-w-0 space-y-3 rounded-lg border border-border bg-card p-4">
   <div className="flex flex-wrap items-center gap-2"><h3 className="text-sm font-semibold">{account.name}</h3><span className="text-xs text-muted-foreground">{account.platform} · #{account.external_id}</span></div>
   <PelicanAccountPanel account={account}/>
  </section>):<p className="rounded-lg border border-dashed p-8 text-center text-sm text-muted-foreground">当前站点还没有账号，请先同步账号。</p>}
 </div>
}

export function PelicanAccountPanel({account}:{account:RelayAccountView}) {
 const context=usePelican()
 const settings=useSystemSettings()
 const configured=Boolean(settings.data?.pelican_model?.trim())
 const config=context.data?.configs.find(item=>item.account_external_id===account.external_id)
 const results=context.data?.results.filter(item=>item.account_external_id===account.external_id)??[]
 const active=running(config)
 return <div className="col-span-full min-w-0 border-t border-border pt-3" data-pelican-account={account.external_id}>
  <div className="flex flex-wrap items-center justify-between gap-2 text-xs">
   <span className="inline-flex items-center gap-1.5 font-medium"><Bird className="size-3.5 text-teal-600"/>鹈鹕测智 <span className="font-normal text-muted-foreground">{active?"正在生成…":config?.enabled?"已启用":"已关闭"}</span></span>
   <div className="flex items-center gap-2">
    <Switch checked={config?.enabled??false} disabled={!context.data||settings.loading||(!configured&&!config?.enabled)} aria-label={`${account.name}启用鹈鹕测智`} onCheckedChange={enabled=>context.toggle(account,enabled)}/>
    <Button type="button" size="sm" variant="outline" className="h-7 gap-1 px-2 text-xs" disabled={active||!configured||!context.data} onClick={()=>void context.run(account.external_id)} aria-label={`${account.name}\u7acb\u5373\u6d4b\u667a`}><Play className="size-3" />{"\u6d4b\u4e00\u6b21"}</Button>
   </div>
  </div>
  {context.error?<p role="alert" className="mt-2 text-xs text-danger">{context.error}</p>:null}
  {!configured?<p className="mt-2 text-xs text-muted-foreground">{"\u8bf7\u5148\u5728"}<Link className="text-brand underline" to="/settings#pelican-settings">{"\u7cfb\u7edf\u8bbe\u7f6e"}</Link>{"\u4e2d\u914d\u7f6e\u6d4b\u667a\u6a21\u578b\u3002"}</p>:null}
  {config?.enabled||results.length||active?<>
   <div className="my-2 flex flex-wrap justify-between gap-1 text-[10px] text-muted-foreground"><span>{results.length}/30 份作品·最新在前</span><span>{config?.enabled&&config.next_run_at?`下次 ${timeLabel(config.next_run_at)}（北京时间）`:"定时测智已关闭"}</span></div>
   {results.length?<div className="flex min-w-0 snap-x gap-3 overflow-x-auto pb-2" aria-label={`${account.name}最近30份测智结果`}>{results.map(result=><PelicanThumbnail key={result.id} result={result}/>)}</div>:<p className="rounded-md border border-dashed p-4 text-xs text-muted-foreground">{active?"正在等待模型生成作品，完成后自动显示。":"暂无作品，可点击“测一次”，或等待定时测智。"}</p>}
  </>:null}
 </div>
}
function usePelicanBody(result:PelicanResult,visible:boolean){
 const context=usePelican(),[value,setValue]=useState<PelicanResult|null>(null),[error,setError]=useState("")
 useEffect(()=>{if(!visible||result.status!=="success")return;let cancelled=false;context.body(result).then(value=>{if(!cancelled)setValue(value)}).catch(error=>{if(!cancelled)setError(error instanceof Error?error.message:"读取失败")});return()=>{cancelled=true}},[visible,result.id])
 return {value,error}
}
function PelicanThumbnail({result}:{result:PelicanResult}){
 const context=usePelican(),ref=useRef<HTMLDivElement>(null),[visible,setVisible]=useState(false)
 useEffect(()=>{const target=ref.current;if(!target)return;const observer=new IntersectionObserver(entries=>{setVisible(entries.some(entry=>entry.isIntersecting))},{rootMargin:"100px"});observer.observe(target);return()=>observer.disconnect()},[])
 const {value,error}=usePelicanBody(result,visible)
 return <div ref={ref} className="relative w-56 shrink-0 snap-start overflow-hidden rounded-lg border border-border bg-card">
  <div className="aspect-[4/3] overflow-hidden bg-muted/30">
   {value?.html&&visible?<PelicanArtwork html={value.html} title={`缩略图 ${result.id}`} playing interactive={false}/>:<p className="flex h-full items-center justify-center p-3 text-center text-xs text-muted-foreground">{result.status==="failed"?"本次生成失败":error||"读取作品…"}</p>}
  </div>
  <div className="space-y-1 p-2 text-[10px]"><p className="truncate font-mono text-xs" title={result.model}>{result.model}</p><p className="flex justify-between text-muted-foreground"><span>{timeLabel(result.finished_at)}</span><span>{(result.duration_ms/1000).toFixed(1)} 秒</span></p><p className={result.status==="success"?"text-teal-600":"text-danger"}>{result.status==="success"?`思考强度 ${pelicanEfforts[result.reasoning_effort]??result.reasoning_effort}`:"生成失败·点击查看"}</p></div>
  <button type="button" className="absolute inset-0 rounded-lg outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring" aria-label={`查看测智作品 ${result.id}`} onClick={()=>context.open(result)}/>
  <PelicanDeleteButton result={result} compact/>
 </div>
}

function PelicanDeleteButton({result,compact=false}:{result:PelicanResult;compact?:boolean}){
 const context=usePelican()
 const busy=context.deletingID===result.id
 return <Button type="button" size={compact?"icon":"sm"} variant="outline" className={compact?"absolute right-2 top-2 z-10 size-8 bg-card/95 text-danger shadow-sm hover:text-danger":"gap-1 text-danger hover:text-danger"} aria-label={`\u5220\u9664\u6d4b\u667a\u4f5c\u54c1 ${result.id}`} disabled={context.deletingID!==null} onClick={event=>{event.stopPropagation();void context.remove(result)}}><Trash2 className="size-3.5"/>{compact?null:busy?"\u5220\u9664\u4e2d\u2026":"\u5220\u9664\u4f5c\u54c1"}</Button>
}


function PelicanPreview({result,onClose}:{result:PelicanResult;onClose:()=>void}){
 const {value,error}=usePelicanBody(result,true)
 const [playing,setPlaying]=useState(true),[speed,setSpeed]=useState(1),[zoom,setZoom]=useState(0),[revision,setRevision]=useState(0)
 function download(){if(!value?.html)return;const url=URL.createObjectURL(new Blob([value.html],{type:"text/html;charset=utf-8"}));const link=document.createElement("a");link.href=url;link.download=`pelican-${result.relay_station_id}-${result.account_external_id}-${result.id}.html`;link.click();window.setTimeout(()=>URL.revokeObjectURL(url),1000)}
 return <Dialog open onOpenChange={open=>{if(!open)onClose()}}><DialogContent className="flex max-h-[92dvh] w-[calc(100vw-1rem)] max-w-6xl flex-col overflow-hidden p-4 sm:p-6">
  <DialogHeader><DialogTitle className="pr-6">{result.account_name} · {result.model}</DialogTitle><DialogDescription>{timeLabel(result.finished_at)} · 思考强度 {pelicanEfforts[result.reasoning_effort]??result.reasoning_effort} · 耗时 {(result.duration_ms/1000).toFixed(1)} 秒</DialogDescription></DialogHeader>
  <div className="flex justify-end"><PelicanDeleteButton result={result}/></div>
  {result.status==="success"?<div className="flex flex-wrap items-center gap-2">
   <Button size="sm" variant="outline" className="gap-1" onClick={()=>setPlaying(current=>!current)}>{playing?<Pause className="size-3.5"/>:<Play className="size-3.5"/>}{playing?"暂停":"恢复"}</Button>
   <Button size="sm" variant="outline" className="gap-1" onClick={()=>{setRevision(current=>current+1);setPlaying(true)}}><RotateCcw className="size-3.5"/>重播</Button>
   <Select value={String(speed)} onValueChange={value=>setSpeed(Number(value))}><SelectTrigger className="w-28" aria-label="播放速度"><SelectValue/></SelectTrigger><SelectContent>{[.25,.5,1,1.5,2,4].map(value=><SelectItem key={value} value={String(value)}>{value}× 速度</SelectItem>)}</SelectContent></Select>
   <Select value={String(zoom)} onValueChange={value=>setZoom(Number(value))}><SelectTrigger className="w-28" aria-label="作品缩放"><SelectValue/></SelectTrigger><SelectContent><SelectItem value="0">适应窗口</SelectItem><SelectItem value="1">100%</SelectItem></SelectContent></Select>
   <Button size="sm" variant="outline" className="ml-auto gap-1" disabled={!value?.html} onClick={download}><Download className="size-3.5"/>下载 HTML</Button>
  </div>:null}
  <div className="h-[min(62dvh,680px)] min-h-48 overflow-hidden rounded-lg border border-border">
   {result.status==="failed"?<p role="alert" className="p-5 text-sm text-danger">{result.error||"本次生成失败，没有可展示的作品。"}</p>:value?.html?<PelicanArtwork key={revision} html={value.html} title={`鹈鹕作品 ${result.id}`} playing={playing} speed={speed} zoom={zoom}/>:<p className="p-5 text-sm text-muted-foreground">{error||"正在读取作品…"}</p>}
  </div>
  <p className="text-xs text-muted-foreground">作品在独立沙箱中运行，无法读取管理页面；已限制外部资源和网络接口请求。支持 CSS、SVG 和 JavaScript 动画播放控制。</p>
 </DialogContent></Dialog>
}
