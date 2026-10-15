import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from "react"
import { Bird, Download, Pause, Play, RotateCcw, Settings2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { apiFetch } from "@/lib/api"
import type { PelicanConfig, PelicanGallery, PelicanResult, RelayAccountView } from "@/lib/api-types"
import { defaultPelicanConfig, defaultPelicanPrompt, pelicanEfforts, pelicanSchedules } from "@/lib/pelican"
import { usePelicanGallery } from "@/lib/queries"
import { PelicanArtwork } from "./pelican-artwork"

const timeLabel=(value:string)=>new Date(value).toLocaleString("zh-CN",{month:"2-digit",day:"2-digit",hour:"2-digit",minute:"2-digit",hour12:false,timeZone:"Asia/Shanghai"})
const running=(config?:PelicanConfig)=>Boolean(config?.lease_until&&Date.parse(config.lease_until)>Date.now())
type PelicanContextValue={
 stationID:number|null; data:PelicanGallery|null; error:string|null
 edit:(account:RelayAccountView)=>void; open:(result:PelicanResult)=>void
 toggle:(account:RelayAccountView,enabled:boolean)=>void; run:(accountID:number)=>Promise<void>
 body:(result:PelicanResult)=>Promise<PelicanResult>
}
const PelicanContext=createContext<PelicanContextValue|null>(null)
function usePelican(){const context=useContext(PelicanContext);if(!context)throw new Error("PelicanProvider required");return context}

export function PelicanProvider({stationID,children}:{stationID:number|null;children:ReactNode}) {
 const query=usePelicanGallery(stationID)
 const [editing,setEditing]=useState<RelayAccountView|null>(null),[selected,setSelected]=useState<PelicanResult|null>(null)
 const [actionError,setActionError]=useState<string|null>(null)
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
  const config=query.data?.configs.find(item=>item.account_external_id===account.external_id)
  if(!config?.model){setEditing(account);return}
  setActionError(null)
  try{await apiFetch(`/relay-stations/${stationID}/accounts/${account.external_id}/pelican`,{method:"PUT",body:JSON.stringify({...config,enabled})});await query.refetch()}
  catch(error){setActionError(error instanceof Error?error.message:"保存失败")}
 }
 const body=(result:PelicanResult)=>{
  const key=`${result.relay_station_id}/${result.account_external_id}/${result.id}`
  let promise=requests.current.get(key)
  if(!promise){promise=apiFetch<PelicanResult>(`/relay-stations/${result.relay_station_id}/accounts/${result.account_external_id}/pelican/results/${result.id}`).catch(error=>{requests.current.delete(key);throw error});requests.current.set(key,promise)}
  return promise
 }
 return <PelicanContext.Provider value={{stationID,data:query.data,error:actionError??query.error,edit:setEditing,open:setSelected,toggle:(account,enabled)=>void toggle(account,enabled),run,body}}>
  {children}
  {editing&&stationID!=null?<PelicanEditor key={`${stationID}-${editing.external_id}`} stationID={stationID} account={editing} config={query.data?.configs.find(item=>item.account_external_id===editing.external_id)} onClose={()=>setEditing(null)} onSaved={async()=>{await query.refetch();setEditing(null)}}/>:null}
  {selected?<PelicanPreview key={selected.id} result={selected} onClose={()=>setSelected(null)}/>:null}
 </PelicanContext.Provider>
}

export function PelicanDashboard({accounts}:{accounts:RelayAccountView[]}) {
 return <div className="min-w-0 space-y-4" aria-label="账号鹈鹕测智总览">
  <p className="text-xs leading-5 text-muted-foreground">每个账号独立配置，默认关闭。每行展示该账号最近 30 份结果，向右滚动查看历史，点击作品可交互播放。</p>
  {accounts.length?accounts.map(account=><section key={account.external_id} className="min-w-0 space-y-3 rounded-lg border border-border bg-card p-4">
   <div className="flex flex-wrap items-center gap-2"><h3 className="text-sm font-semibold">{account.name}</h3><span className="text-xs text-muted-foreground">{account.platform} · #{account.external_id}</span></div>
   <PelicanAccountPanel account={account}/>
  </section>):<p className="rounded-lg border border-dashed p-8 text-center text-sm text-muted-foreground">当前站点还没有账号，请先同步账号。</p>}
 </div>
}

export function PelicanAccountPanel({account}:{account:RelayAccountView}) {
 const context=usePelican()
 const config=context.data?.configs.find(item=>item.account_external_id===account.external_id)
 const results=context.data?.results.filter(item=>item.account_external_id===account.external_id)??[]
 const active=running(config)
 return <div className="col-span-full min-w-0 border-t border-border pt-3" data-pelican-account={account.external_id}>
  <div className="flex flex-wrap items-center justify-between gap-2 text-xs">
   <span className="inline-flex items-center gap-1.5 font-medium"><Bird className="size-3.5 text-teal-600"/>鹈鹕测智 <span className="font-normal text-muted-foreground">{active?"正在生成…":config?.enabled?"已启用":"已关闭"}</span></span>
   <div className="flex items-center gap-2">
    <Switch checked={config?.enabled??false} aria-label={`${account.name}启用鹈鹕测智`} onCheckedChange={enabled=>context.toggle(account,enabled)}/>
    <Button type="button" size="sm" variant="ghost" className="h-7 gap-1 px-1.5 text-xs" onClick={()=>context.edit(account)} aria-label={`${account.name}鹈鹕测智配置`}><Settings2 className="size-3"/>配置</Button>
    {config?.model?<Button type="button" size="sm" variant="outline" className="h-7 gap-1 px-2 text-xs" disabled={active} onClick={()=>void context.run(account.external_id)} aria-label={`${account.name}立即测智`}><Play className="size-3"/>测一次</Button>:null}
   </div>
  </div>
  {context.error?<p role="alert" className="mt-2 text-xs text-danger">{context.error}</p>:null}
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
 </div>
}

function PelicanEditor({stationID,account,config,onClose,onSaved}:{stationID:number;account:RelayAccountView;config?:PelicanConfig;onClose:()=>void;onSaved:()=>Promise<void>}){
 const [draft,setDraft]=useState<PelicanConfig>(()=>({...defaultPelicanConfig(),...config}))
 const [custom,setCustom]=useState(Boolean(config&&!pelicanSchedules.some(item=>item.value===config.cron_expression)))
 const [models,setModels]=useState<string[]>([]),[busy,setBusy]=useState(false),[error,setError]=useState("")
 useEffect(()=>{let current=true;apiFetch<string[]>(`/relay-stations/${stationID}/accounts/${account.external_id}/models`).then(models=>{if(current)setModels(models)}).catch(()=>{});return()=>{current=false}},[stationID,account.external_id])
 const patch=(value:Partial<PelicanConfig>)=>setDraft(current=>({...current,...value}))
 async function save(){setBusy(true);setError("");try{await apiFetch(`/relay-stations/${stationID}/accounts/${account.external_id}/pelican`,{method:"PUT",body:JSON.stringify(draft)});await onSaved()}catch(error){setError(error instanceof Error?error.message:"保存失败")}finally{setBusy(false)}}
 return <Dialog open onOpenChange={open=>{if(!open&&!busy)onClose()}}><DialogContent className="max-h-[90dvh] max-w-lg overflow-y-auto">
  <DialogHeader><DialogTitle>鹈鹕测智配置</DialogTitle><DialogDescription>{account.name} · #{account.external_id}，测试直接发给该账号。</DialogDescription></DialogHeader>
  <div className="space-y-4">
   <div className="space-y-2"><Label htmlFor="pelican-model">模型</Label><Input id="pelican-model" list="pelican-models" value={draft.model} onChange={event=>patch({model:event.target.value})} placeholder="输入或选择模型"/><datalist id="pelican-models">{models.map(model=><option key={model} value={model}/>)}</datalist><p className="text-xs text-muted-foreground">使用此账号支持的模型名称。</p></div>
   <div className="space-y-2"><div className="flex items-center justify-between"><Label htmlFor="pelican-prompt">题目</Label><button type="button" className="text-xs text-teal-600" onClick={()=>patch({prompt:defaultPelicanPrompt})}>恢复默认题目</button></div><textarea id="pelican-prompt" value={draft.prompt} maxLength={32000} rows={4} onChange={event=>patch({prompt:event.target.value})} className="w-full resize-y rounded-md border border-input bg-transparent px-3 py-2 text-sm"/><p className="text-xs leading-5 text-muted-foreground">发送时会加上“只输出独立 HTML”的要求。模型返回的原始作品会保留动画。</p></div>
   <div className="grid grid-cols-2 gap-4"><div className="space-y-2"><Label>思考强度</Label><Select value={draft.reasoning_effort} onValueChange={reasoning_effort=>patch({reasoning_effort})}><SelectTrigger className="w-full" aria-label="思考强度"><SelectValue/></SelectTrigger><SelectContent>{Object.entries(pelicanEfforts).map(([value,label])=><SelectItem key={value} value={value}>{label}</SelectItem>)}</SelectContent></Select></div><div className="space-y-2"><Label htmlFor="pelican-parallel">每次并行份数</Label><Input id="pelican-parallel" type="number" min={1} max={8} value={draft.parallel_count} onChange={event=>patch({parallel_count:Number(event.target.value)})}/><p className="text-xs text-muted-foreground">1–8 份，默认 1 份。</p></div></div>
   <div className="space-y-2"><Label>频率</Label><Select value={custom?"custom":draft.cron_expression} onValueChange={value=>{setCustom(value==="custom");if(value!=="custom")patch({cron_expression:value})}}><SelectTrigger className="w-full" aria-label="测智频率"><SelectValue/></SelectTrigger><SelectContent>{pelicanSchedules.map(item=><SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>)}<SelectItem value="custom">自定义</SelectItem></SelectContent></Select>{custom?<><Input aria-label="自定义 Cron" value={draft.cron_expression} onChange={event=>patch({cron_expression:event.target.value})}/><p className="text-xs text-muted-foreground">五段 Cron：分 时 日 月 周。例如 */20 * * * * 表示每 20 分钟。</p></>:null}<p className="text-xs text-muted-foreground">按北京时间执行（Asia/Shanghai）。</p></div>
   <div className="flex items-center justify-between"><Label htmlFor="pelican-enabled">启用定时测智</Label><Switch id="pelican-enabled" checked={draft.enabled} onCheckedChange={enabled=>patch({enabled})}/></div>
   <p className="text-xs leading-5 text-muted-foreground">默认关闭。生成作品会产生该账号的模型调用费用。每个账号保留最近 30 份结果；关闭后已开始的一轮仍会完成。</p>
   {error?<p role="alert" className="text-sm text-danger">{error}</p>:null}
  </div>
  <div className="flex justify-end gap-2"><Button variant="outline" disabled={busy} onClick={onClose}>取消</Button><Button disabled={busy||!draft.model.trim()||!draft.prompt.trim()||!Number.isInteger(draft.parallel_count)||draft.parallel_count<1||draft.parallel_count>8} onClick={()=>void save()}>{busy?"保存中…":"保存"}</Button></div>
 </DialogContent></Dialog>
}

function PelicanPreview({result,onClose}:{result:PelicanResult;onClose:()=>void}){
 const {value,error}=usePelicanBody(result,true)
 const [playing,setPlaying]=useState(true),[speed,setSpeed]=useState(1),[zoom,setZoom]=useState(0),[revision,setRevision]=useState(0)
 function download(){if(!value?.html)return;const url=URL.createObjectURL(new Blob([value.html],{type:"text/html;charset=utf-8"}));const link=document.createElement("a");link.href=url;link.download=`pelican-${result.relay_station_id}-${result.account_external_id}-${result.id}.html`;link.click();window.setTimeout(()=>URL.revokeObjectURL(url),1000)}
 return <Dialog open onOpenChange={open=>{if(!open)onClose()}}><DialogContent className="flex max-h-[92dvh] w-[calc(100vw-1rem)] max-w-6xl flex-col overflow-hidden p-4 sm:p-6">
  <DialogHeader><DialogTitle className="pr-6">{result.account_name} · {result.model}</DialogTitle><DialogDescription>{timeLabel(result.finished_at)} · 思考强度 {pelicanEfforts[result.reasoning_effort]??result.reasoning_effort} · 耗时 {(result.duration_ms/1000).toFixed(1)} 秒</DialogDescription></DialogHeader>
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
