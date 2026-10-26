import { createContext, useContext, useEffect, useId, useMemo, useRef, useState, useSyncExternalStore, type ReactNode } from "react"
import { Bird, ChevronDown, Download, Pause, Play, RotateCcw, Settings2, Trash2 } from "lucide-react"
import { toast } from "sonner"
import { Link } from "react-router-dom"
import { Button } from "@/components/ui/button"
import { ButtonGroup } from "@/components/ui/button-group"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Label } from "@/components/ui/label"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Switch } from "@/components/ui/switch"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { useConfirm } from "@/components/ui/confirm-dialog"
import { apiFetch } from "@/lib/api"
import type { ChannelPelicanTarget, PelicanConfig, PelicanGallery, PelicanLibraryPage, PelicanResult, PelicanTestPlan, RelayAccountView } from "@/lib/api-types"
import { hasConfiguredPelicanPlan, pelicanEfforts, pelicanEffortOptions, pelicanGroupLabels, pelicanRunOptions } from "@/lib/pelican"
import { pelicanResultPath } from "@/lib/pelican-sources"
import { observePelicanThumbnail, pelicanThumbnailActive, pelicanThumbnailPool, renderPelicanThumbnail, retainPelicanThumbnails, subscribePelicanActivity, suspendPelicanThumbnails } from "@/lib/pelican-thumbnails"
import { usePelicanGallery, useSystemSettings, type QueryState } from "@/lib/queries"
import { PelicanArtwork } from "./pelican-artwork"

const timeLabel=(value:string)=>new Date(value).toLocaleString("zh-CN",{month:"2-digit",day:"2-digit",hour:"2-digit",minute:"2-digit",hour12:false,timeZone:"Asia/Shanghai"})
const running=(config?:PelicanConfig)=>Boolean(config?.lease_until&&Date.parse(config.lease_until)>Date.now())
type PelicanAccount = Pick<RelayAccountView,"external_id"|"name"> & {relay_station_id?:number}
type PelicanContextValue={
	showKeyGroup:boolean
	readOnly:boolean
 stationID:number|null; data:PelicanGallery|null; error:string|null
 open:(result:PelicanResult)=>void
	 toggle:(account:PelicanAccount,enabled:boolean)=>void; run:(accountID:number,stationID?:number,plan?:PelicanTestPlan)=>Promise<void>
 body:(result:PelicanResult,signal:AbortSignal)=>Promise<PelicanResult>
 remove:(result:PelicanResult)=>Promise<void>; deletingID:number|null
}
const PelicanContext=createContext<PelicanContextValue|null>(null)
function usePelican(){const context=useContext(PelicanContext);if(!context)throw new Error("PelicanProvider required");return context}

export function PelicanProvider({stationID,children,libraryQuery,galleryQuery,scope="account",readOnly=false,showKeyGroup=false}:{stationID:number|null;children:ReactNode;libraryQuery?:QueryState<PelicanLibraryPage>;galleryQuery?:QueryState<PelicanGallery>;scope?:"account"|"group";readOnly?:boolean;showKeyGroup?:boolean}) {
 const stationQuery=usePelicanGallery(galleryQuery||libraryQuery?null:stationID)
 const query=galleryQuery??libraryQuery??stationQuery
 const sourceData=useMemo(()=>galleryQuery?galleryQuery.data:libraryQuery?.data?{configs:[],results:libraryQuery.data.results}:stationQuery.data,[galleryQuery?.data,libraryQuery?.data,stationQuery.data])
 const [selected,setSelected]=useState<PelicanResult|null>(null)
 const [actionError,setActionError]=useState<string|null>(null)
 const {confirm,dialog:deleteDialog}=useConfirm()
 const [deletingID,setDeletingID]=useState<number|null>(null)
 const [removedIDs,setRemovedIDs]=useState<Set<number>>(()=>new Set())
 const deleting=useRef(false)
 useEffect(retainPelicanThumbnails,[])
 const refetch=useRef(query.refetch);refetch.current=query.refetch
 const anyRunning=sourceData?.results.some(result=>result.status==="running")||(!readOnly&&sourceData?.configs.some(running))
 useEffect(()=>{
  if(!anyRunning)return
  const timer=window.setInterval(()=>{if(!document.hidden)void refetch.current().catch(()=>{})},5000)
  return()=>window.clearInterval(timer)
 },[anyRunning,stationID])
	const run=async(accountID:number,targetStationID=stationID??undefined,plan?:PelicanTestPlan)=>{
  if(targetStationID==null||readOnly)return
  setActionError(null)
	try{await apiFetch(`/relay-stations/${targetStationID}/${scope==="group"?"groups":"accounts"}/${accountID}/pelican/run`,{method:"POST",...(plan?{body:JSON.stringify(plan)}:{})});await query.refetch()}
  catch(error){setActionError(error instanceof Error?error.message:"启动测智失败")}
 }
 const toggle=async(account:PelicanAccount,enabled:boolean)=>{
  const targetStationID=account.relay_station_id??stationID
  if(targetStationID==null||readOnly)return
  setActionError(null)
  try{await apiFetch(`/relay-stations/${targetStationID}/${scope==="group"?"groups":"accounts"}/${account.external_id}/pelican`,{method:"PUT",body:JSON.stringify({enabled})});await query.refetch()}
  catch(error){setActionError(error instanceof Error?error.message:"保存失败")}
 }
 const body=(result:PelicanResult,signal:AbortSignal)=>apiFetch<PelicanResult>(pelicanResultPath(result,readOnly),{signal})
 const remove=async(result:PelicanResult)=>{
  if(deleting.current||readOnly)return
  deleting.current=true
  try{
   const accepted=await confirm({title:"\u5220\u9664\u8fd9\u4efd\u6d4b\u667a\u4f5c\u54c1\uff1f",description:`${result.account_name} \u00b7 ${result.model} \u00b7 ${timeLabel(result.finished_at)}\u3002\u5c06\u6c38\u4e45\u5220\u9664\u672c\u6b21\u751f\u6210\u7684\u4f5c\u54c1\u53ca\u7ed3\u679c\uff0c\u4e0d\u5f71\u54cd\u8d26\u53f7\u914d\u7f6e\u548c\u5176\u4ed6\u5386\u53f2\u8bb0\u5f55\u3002\u64cd\u4f5c\u4e0d\u53ef\u6062\u590d\u3002`,confirmLabel:"\u786e\u8ba4\u5220\u9664",destructive:true})
   if(!accepted)return
   setDeletingID(result.id)
   await apiFetch(pelicanResultPath(result),{method:"DELETE"})
   pelicanThumbnailPool.delete(`${result.relay_station_id}/${result.account_external_id}/${result.id}`)
   setRemovedIDs(current=>new Set(current).add(result.id))
   setSelected(current=>current?.id===result.id?null:current)
   toast.success("\u6d4b\u667a\u4f5c\u54c1\u5df2\u5220\u9664")
   try{await query.refetch()}catch{toast.warning("\u4f5c\u54c1\u5df2\u5220\u9664\uff0c\u4f46\u5237\u65b0\u5217\u8868\u5931\u8d25\uff0c\u8bf7\u7a0d\u540e\u5237\u65b0\u3002")}
  }catch(error){toast.error(error instanceof Error?error.message:"\u5220\u9664\u4f5c\u54c1\u5931\u8d25")}
  finally{setDeletingID(null);deleting.current=false}
 }
 const data=sourceData?{...sourceData,results:sourceData.results.filter(result=>!removedIDs.has(result.id))}:null
 return <PelicanContext.Provider value={{stationID,readOnly,showKeyGroup,data,error:actionError??query.error,open:setSelected,toggle:(account,enabled)=>void toggle(account,enabled),run,body,remove,deletingID}}>
  {children}
  {selected?<PelicanPreview key={selected.id} result={selected} onClose={()=>setSelected(null)}/>:null}
  {deleteDialog}
 </PelicanContext.Provider>
}

export function PelicanLibraryResults({query}:{query:QueryState<PelicanLibraryPage>}) {
 const context=usePelican()
 if(query.loading&&!query.data)return <p className="border-t border-border py-8 text-center text-sm text-muted-foreground">正在加载测智记录…</p>
 if(query.error&&!query.data)return <p role="alert" className="border-t border-danger/30 py-8 text-center text-sm text-danger">读取测智作品失败：{query.error}</p>
 const results=context.data?.results??[]
 if(!results.length)return <p className="border-t border-border py-12 text-center text-sm text-muted-foreground">没有符合条件的测智记录。</p>
 return <div className="grid grid-cols-1 gap-3 border-t border-border pt-4 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4" aria-label="鹈鹕测智结果列表">{results.map(result=><div key={`${result.relay_station_id}/${result.account_external_id}/${result.id}`} className="min-w-0"><PelicanThumbnail result={result} grid/><p className="mt-1 truncate text-xs text-muted-foreground" title={`${result.station_name} · ${result.account_name}`}>{result.station_name} · {result.account_name}</p></div>)}</div>
}

export function PelicanDashboard({accounts}:{accounts:RelayAccountView[]}) {
 const settings=useSystemSettings()
 const historyCount=settings.data?.pelican_history_count??30
 return <div className="min-w-0 space-y-4" aria-label="账号鹈鹕测智总览">
  <div className="flex flex-wrap items-center justify-between gap-2"><p className="text-xs leading-5 text-muted-foreground">{`每个账号的定时开关默认关闭，测智参数在系统设置中统一配置。每行展示最近 ${historyCount} 份结果，点击作品可交互播放。`}</p><Button asChild variant="outline" size="sm" className="h-8 gap-1 text-xs"><Link to="/settings#pelican-settings"><Settings2 className="size-3" />{"\u5168\u5c40\u6d4b\u667a\u8bbe\u7f6e"}</Link></Button></div>
  {accounts.length?accounts.map(account=><section key={account.external_id} className="min-w-0 space-y-3 rounded-lg border border-border bg-card p-4">
   <div className="flex flex-wrap items-center gap-2"><h3 className="text-sm font-semibold">{account.name}</h3><span className="text-xs text-muted-foreground">{account.platform} · #{account.external_id}</span></div>
   <PelicanAccountPanel account={account}/>
  </section>):<p className="rounded-lg border border-dashed p-8 text-center text-sm text-muted-foreground">当前站点还没有账号，请先同步账号。</p>}
 </div>
}

export function PelicanTargetRunButton({externalID,name}:{externalID:number;name:string}) {
 const context=usePelican()
 const [starting,setStarting]=useState(false)
 const config=context.data?.configs.find(item=>item.account_external_id===externalID)
 const active=starting||running(config)
 const run=async(plan?:PelicanTestPlan)=>{setStarting(true);try{await context.run(externalID,undefined,plan)}finally{setStarting(false)}}
 return <PelicanOneShotButton name={name} active={active} disabled={!context.data||context.readOnly} onRun={run}/>
}

function PelicanOneShotButton({name,active,disabled,onRun}:{name:string;active:boolean;disabled?:boolean;onRun:(plan?:PelicanTestPlan)=>Promise<void>}) {
 const settings=useSystemSettings()
 const options=useMemo(()=>pelicanRunOptions(settings.data),[settings.data])
 const [open,setOpen]=useState(false)
 const [plan,setPlan]=useState<PelicanTestPlan|null>(null)
 const id=useId()
 const tasksEnabled=settings.data?.pelican_tasks_enabled===true
 const unavailable=Boolean(disabled||!tasksEnabled||options.length===0)
 useEffect(()=>{
  if(!options.length){setPlan(null);return}
  setPlan(current=>current&&options.some(option=>option.model===current.model)?current:options[0])
 },[options])
 const runDefault=()=>{if(!options[0])return;setOpen(false);void onRun(options[0])}
 const runSelected=()=>{if(!plan||!plan.prompt.trim())return;setOpen(false);void onRun(plan)}
 return <Popover open={open} onOpenChange={setOpen}>
  <ButtonGroup className="h-7 shrink-0" aria-label={`${name}鹈鹕测智操作`}>
   <Button type="button" size="sm" variant="outline" className="h-7 gap-1 px-2 text-xs" disabled={unavailable||active} title={!tasksEnabled?"请在系统设置中开启测智任务执行":undefined} aria-label={`${name}立即测智`} onClick={runDefault}><Play className="size-3"/>{active?"生成中…":"测一次"}</Button>
   <PopoverTrigger asChild><Button type="button" size="icon" variant="outline" className="size-7 px-1" disabled={unavailable||active} aria-label={`${name}测智参数`} title="选择本次测智参数"><ChevronDown className="size-3.5"/></Button></PopoverTrigger>
  </ButtonGroup>
  <PopoverContent align="end" className="w-[min(22rem,calc(100vw-2rem))] space-y-3 p-3">
   <div className="grid grid-cols-2 gap-3">
    <div className="min-w-0 space-y-1.5"><Label htmlFor={`${id}-model`}>模型</Label><Select value={plan?.model??""} onValueChange={value=>setPlan(options.find(option=>option.model===value)??null)} disabled={options.length===0||active}><SelectTrigger id={`${id}-model`} className="w-full"><SelectValue placeholder="没有已配置模型"/></SelectTrigger><SelectContent>{options.map(option=><SelectItem key={option.model} value={option.model}>{option.model}</SelectItem>)}</SelectContent></Select></div>
    <div className="min-w-0 space-y-1.5"><Label htmlFor={`${id}-effort`}>思考强度</Label><Select value={plan?.reasoning_effort??""} onValueChange={value=>setPlan(current=>current?{...current,reasoning_effort:value}:current)} disabled={!plan||active}><SelectTrigger id={`${id}-effort`} className="w-full"><SelectValue/></SelectTrigger><SelectContent>{Object.entries(pelicanEffortOptions).map(([value,label])=><SelectItem key={value} value={value}>{label}</SelectItem>)}</SelectContent></Select></div>
   </div>
   <div className="space-y-1.5"><Label htmlFor={`${id}-prompt`}>测智提示词</Label><Textarea id={`${id}-prompt`} value={plan?.prompt??""} rows={5} maxLength={32000} placeholder="输入本次测智提示词" disabled={!plan||active} onChange={event=>setPlan(current=>current?{...current,prompt:event.target.value}:current)}/></div>
   <div className="flex justify-end"><Button type="button" size="sm" className="h-8 gap-1.5" disabled={unavailable||active||!plan||!plan.prompt.trim()} onClick={runSelected}><Play className="size-3.5"/>手动测一次</Button></div>
  </PopoverContent>
 </Popover>
}

export function PelicanAccountPanel({account,controlsOnly=false,compactControls=false}:{account:PelicanAccount;controlsOnly?:boolean;compactControls?:boolean}) {
 const context=usePelican()
 const settings=useSystemSettings()
 const [starting,setStarting]=useState(false)
 const configured=hasConfiguredPelicanPlan(settings.data)
 const tasksEnabled=settings.data?.pelican_tasks_enabled===true
 const config=context.data?.configs.find(item=>item.account_external_id===account.external_id&&(!account.relay_station_id||item.relay_station_id===account.relay_station_id))
 const results=context.data?.results.filter(item=>item.account_external_id===account.external_id&&(!account.relay_station_id||item.relay_station_id===account.relay_station_id))??[]
 const active=running(config)||starting
 const run=async(plan?:PelicanTestPlan)=>{setStarting(true);try{await context.run(account.external_id,account.relay_station_id,plan)}finally{setStarting(false)}}
  const controls=<div className="flex items-center gap-2">
   <Switch checked={config?.enabled??false} disabled={!context.data||settings.loading||(!configured&&!config?.enabled)} aria-label={`${account.name}启用鹈鹕测智`} onCheckedChange={enabled=>context.toggle(account,enabled)}/>
   <PelicanOneShotButton name={account.name} active={active} disabled={!context.data||context.readOnly} onRun={run}/>
  </div>
  if(compactControls)return <div className="inline-flex items-center gap-2" data-pelican-account={account.external_id}>{controls}{context.error?<p role="alert" className="text-xs text-danger">{context.error}</p>:null}</div>
 return <div className="col-span-full min-w-0 border-t border-border pt-3" data-pelican-account={account.external_id}>
  <div className="flex flex-wrap items-center justify-between gap-2 text-xs">
   <span className="inline-flex items-center gap-1.5 font-medium"><Bird className="size-3.5 text-teal-600"/>鹈鹕测智 <span className="font-normal text-muted-foreground">{active?"正在生成…":!tasksEnabled?"任务已暂停":config?.enabled?"已启用":"已关闭"}</span></span>
   {controls}
  </div>
  {context.error?<p role="alert" className="mt-2 text-xs text-danger">{context.error}</p>:null}
  {!configured?<p className="mt-2 text-xs text-muted-foreground">{"\u8bf7\u5148\u5728"}<Link className="text-brand underline" to="/settings#pelican-settings">{"\u7cfb\u7edf\u8bbe\u7f6e"}</Link>{"\u4e2d\u914d\u7f6e\u6d4b\u667a\u6a21\u578b\u3002"}</p>:null}
  {!controlsOnly&&(config?.enabled||results.length||active)?<>
   <div className="my-2 flex flex-wrap justify-between gap-1 text-[10px] text-muted-foreground"><span>{results.length}/{settings.data?.pelican_history_count??30} 份作品·最新在前</span><span>{config?.enabled&&config.next_run_at?`下次 ${timeLabel(config.next_run_at)}（北京时间）`:"定时测智已关闭"}</span></div>
   {results.length?<div className="flex min-w-0 snap-x gap-3 overflow-x-auto pb-2" aria-label={`${account.name}最近${settings.data?.pelican_history_count??30}份测智结果`}>{results.map(result=><PelicanThumbnail key={result.id} result={result}/>)}</div>:<p className="rounded-md border border-dashed p-4 text-xs text-muted-foreground">{active?"正在等待模型生成作品，完成后自动显示。":"暂无作品，可点击“测一次”，或等待定时测智。"}</p>}
  </>:null}
 </div>
}
function usePelicanBody(result:PelicanResult,visible:boolean){
 const context=usePelican(),[value,setValue]=useState<PelicanResult|null>(null),[error,setError]=useState("")
 useEffect(()=>{setValue(null);setError("");if(!visible||result.status!=="success")return;const controller=new AbortController();context.body(result,controller.signal).then(value=>{if(!controller.signal.aborted)setValue(value)}).catch(error=>{if(!controller.signal.aborted)setError(error instanceof Error?error.message:"读取失败")});return()=>controller.abort()},[visible,result.id,result.relay_station_id,result.account_external_id])
 return {value,error}
}
export function PelicanRunButton({targets,channelName}:{targets:ChannelPelicanTarget[];channelName:string}) {
 const context=usePelican()
 const [selected,setSelected]=useState(""),[starting,setStarting]=useState(false)
 const available=targets.filter(target=>target.station_type!=="newapi")
 const key=(target:ChannelPelicanTarget)=>`${target.relay_station_id}/${target.account_external_id}`
 const target=available.find(item=>key(item)===selected)??available[0]
 const active=Boolean(target&&running(context.data?.configs.find(config=>config.relay_station_id===target.relay_station_id&&config.account_external_id===target.account_external_id)))
 const run=async(plan?:PelicanTestPlan)=>{if(!target)return;setStarting(true);try{await context.run(target.account_external_id,target.relay_station_id,plan)}finally{setStarting(false)}}
 return <div className="flex max-w-full flex-wrap items-center justify-end gap-2">
  {available.length>1?<Select value={target?key(target):""} onValueChange={setSelected}><SelectTrigger className="h-7 w-40 text-xs" aria-label={`${channelName}测智账号`}><SelectValue/></SelectTrigger><SelectContent>{available.map(item=><SelectItem key={key(item)} value={key(item)}>{item.account_name||item.station_name}</SelectItem>)}</SelectContent></Select>:null}
  <PelicanOneShotButton name={channelName} active={starting||active} disabled={!target||!context.data||context.readOnly} onRun={run}/>
  {context.error?<p role="alert" className="max-w-sm text-xs text-danger">{context.error}</p>:null}
 </div>
}
export function PelicanResultsStrip({limit,allowDelete=false,grid=limit===3}:{limit:number;allowDelete?:boolean;grid?:boolean}) {
 const context=usePelican()
 const results=context.data?.results.slice(0,limit)??[]
 if(!results.length)return <p className="rounded-md border border-dashed p-4 text-xs text-muted-foreground">{context.readOnly?"暂无测智作品":"暂无作品，可点击“测一次”，或等待定时测智。"}</p>
 return <div className={grid?"grid min-w-0 grid-cols-3 gap-2":"flex min-w-0 snap-x items-start gap-3 overflow-x-auto overflow-y-hidden pb-3"} aria-label={`最近${limit}份测智结果`} data-pelican-results={limit}>
  {results.map(result=><PelicanThumbnail key={`${result.relay_station_id}/${result.id}`} result={result} grid={grid} allowDelete={allowDelete}/>)}
 </div>
}
function PelicanThumbnail({result,grid=false,allowDelete=false}:{result:PelicanResult;grid?:boolean;allowDelete?:boolean}){
 const context=usePelican(),ref=useRef<HTMLDivElement>(null),[visible,setVisible]=useState(false)
 const active=useSyncExternalStore(subscribePelicanActivity,pelicanThumbnailActive)
 const [image,setImage]=useState(""),[error,setError]=useState("")
 const groupLabels=pelicanGroupLabels(result)
 useEffect(()=>{const target=ref.current;if(target)return observePelicanThumbnail(target,setVisible)},[])
 useEffect(()=>{
  setImage("");setError("")
  if(!visible||!active||result.status!=="success")return
  let disposed=false,url=""
  // 等滚动停稳后才入队，避免快速浏览时生成路过的作品。
  let release=()=>{}
  const timer=window.setTimeout(()=>{
   const lease=pelicanThumbnailPool.acquire(`${result.relay_station_id}/${result.account_external_id}/${result.id}`,async signal=>{
    const value=await context.body(result,signal)
    signal.throwIfAborted()
    if(!value.html)throw new Error("作品没有可预览的内容")
    return renderPelicanThumbnail(value.html,signal)
   })
   release=lease.release
   lease.promise.then(blob=>{if(!disposed){url=URL.createObjectURL(blob);setImage(url)}}).catch(error=>{if(!disposed)setError(error instanceof Error?error.message:"预览读取失败")})
  },120)
  return()=>{disposed=true;window.clearTimeout(timer);release();if(url)URL.revokeObjectURL(url)}
 },[visible,active,result.id,result.relay_station_id,result.account_external_id,result.status])
 return <div ref={ref} className={`relative ${grid?"w-full min-w-0":"w-56 shrink-0 snap-start"} overflow-hidden rounded-lg border border-border bg-card`}>
  {allowDelete&&result.status!=="running"?<PelicanDeleteButton result={result} compact/>:null}
  <div className="aspect-[4/3] overflow-hidden bg-muted/30">
   {image?<img src={image} alt={`测智作品 ${result.id} 静态预览`} width={448} height={336} decoding="async" className="h-full w-full object-contain" onError={()=>{setImage("");setError("预览不可用，点击查看原图")}}/>:null}
   {!image||error?<p className="flex h-full items-center justify-center p-3 text-center text-xs text-muted-foreground">{result.status==="running"?"生成中…":result.status==="failed"?"本次生成失败":error||"点击作品可交互播放"}</p>:null}
  </div>
  <div className="space-y-1 p-2 text-[10px]">
   <p className="truncate font-mono text-xs" title={result.model}>{result.model}</p>
   <p className="flex flex-wrap justify-between gap-x-1 text-muted-foreground"><span>{timeLabel(result.finished_at)}</span><span className="whitespace-nowrap">{result.status==="running"?"等待完成":`${(result.duration_ms/1000).toFixed(1)} 秒`}</span></p>
   <p className={result.status==="running"?"text-muted-foreground":result.status==="success"?"text-teal-600":"text-danger"}>{result.status==="running"?"生成中…":result.status==="success"?`思考强度 ${pelicanEfforts[result.reasoning_effort]??result.reasoning_effort}`:"生成失败·点击查看"}</p>
   {context.showKeyGroup?<p data-pelican-key-group className="relative z-[1] flex min-w-0 items-center justify-between gap-2 text-muted-foreground" onClick={()=>context.open(result)}>
    <span className="min-w-0 flex-1 truncate" title={groupLabels.name}>{groupLabels.name}</span>
    <span className="shrink-0 whitespace-nowrap tabular-nums" title="生成时 Key 对应的分组倍率">{groupLabels.rate}</span>
   </p>:null}
  </div>
  <button type="button" className="absolute inset-0 rounded-lg outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring" aria-label={`查看测智作品 ${result.id}`} disabled={result.status==="running"} onClick={()=>context.open(result)}/>
 </div>
}

function PelicanDeleteButton({result,compact=false}:{result:PelicanResult;compact?:boolean}){
 const context=usePelican()
 const busy=context.deletingID===result.id
 return <Button type="button" size={compact?"icon":"sm"} variant="outline" className={compact?"absolute right-2 top-2 z-10 size-8 bg-card/95 text-danger shadow-sm hover:text-danger":"gap-1 text-danger hover:text-danger"} aria-label={`\u5220\u9664\u6d4b\u667a\u4f5c\u54c1 ${result.id}`} disabled={context.deletingID!==null} onClick={event=>{event.stopPropagation();void context.remove(result)}}><Trash2 className="size-3.5"/>{compact?null:busy?"\u5220\u9664\u4e2d\u2026":"\u5220\u9664\u4f5c\u54c1"}</Button>
}


function PelicanPreview({result,onClose}:{result:PelicanResult;onClose:()=>void}){
 useEffect(suspendPelicanThumbnails,[])
 const context=usePelican()
 const {value,error}=usePelicanBody(result,true)
 const [playing,setPlaying]=useState(true),[speed,setSpeed]=useState(1),[zoom,setZoom]=useState(0),[revision,setRevision]=useState(0)
 function download(){if(!value?.html)return;const url=URL.createObjectURL(new Blob([value.html],{type:"text/html;charset=utf-8"}));const link=document.createElement("a");link.href=url;link.download=`pelican-${result.relay_station_id}-${result.account_external_id}-${result.id}.html`;link.click();window.setTimeout(()=>URL.revokeObjectURL(url),1000)}
 return <Dialog open onOpenChange={open=>{if(!open)onClose()}}><DialogContent className="flex max-h-[92dvh] w-[calc(100vw-1rem)] max-w-6xl flex-col overflow-hidden p-4 sm:p-6">
  <DialogHeader><DialogTitle className="pr-6">{result.station_name?`${result.station_name} · `:""}{result.account_name} · {result.model}</DialogTitle><DialogDescription>{timeLabel(result.finished_at)} · 思考强度 {pelicanEfforts[result.reasoning_effort]??result.reasoning_effort} · 耗时 {(result.duration_ms/1000).toFixed(1)} 秒</DialogDescription></DialogHeader>
  <div className="flex flex-wrap items-center gap-2" role="toolbar" aria-label="作品操作">
  {result.status==="success"?<>
   <Button size="sm" variant="outline" className="gap-1" onClick={()=>setPlaying(current=>!current)}>{playing?<Pause className="size-3.5"/>:<Play className="size-3.5"/>}{playing?"暂停":"恢复"}</Button>
   <Button size="sm" variant="outline" className="gap-1" onClick={()=>{setRevision(current=>current+1);setPlaying(true)}}><RotateCcw className="size-3.5"/>重播</Button>
   <Select value={String(speed)} onValueChange={value=>setSpeed(Number(value))}><SelectTrigger className="w-28" aria-label="播放速度"><SelectValue/></SelectTrigger><SelectContent>{[.25,.5,1,1.5,2,4].map(value=><SelectItem key={value} value={String(value)}>{value}× 速度</SelectItem>)}</SelectContent></Select>
   <Select value={String(zoom)} onValueChange={value=>setZoom(Number(value))}><SelectTrigger className="w-28" aria-label="作品缩放"><SelectValue/></SelectTrigger><SelectContent><SelectItem value="0">适应窗口</SelectItem><SelectItem value="1">100%</SelectItem></SelectContent></Select>
  </>:null}
   <div className="ml-auto flex max-w-full flex-wrap items-center justify-end gap-2">
    {result.status==="success"?<Button size="sm" variant="outline" className="gap-1" disabled={!value?.html} onClick={download}><Download className="size-3.5"/>下载 HTML</Button>:null}
    {!context.readOnly?<PelicanDeleteButton result={result}/>:null}
   </div>
  </div>
  {context.error?<p role="alert" className="text-xs text-danger">{context.error}</p>:null}
  <div className="h-[min(62dvh,680px)] min-h-48 overflow-hidden rounded-lg border border-border">
   {result.status==="failed"?<p role="alert" className="p-5 text-sm text-danger">{result.error||"本次生成失败，没有可展示的作品。"}</p>:value?.html?<PelicanArtwork key={revision} html={value.html} title={`鹈鹕作品 ${result.id}`} playing={playing} speed={speed} zoom={zoom}/>:<p className="p-5 text-sm text-muted-foreground">{error||"正在读取作品…"}</p>}
  </div>
  <p className="text-xs text-muted-foreground">作品在独立沙箱中运行，无法读取管理页面；已限制外部资源和网络接口请求。支持 CSS、SVG 和 JavaScript 动画播放控制。</p>
 </DialogContent></Dialog>
}
