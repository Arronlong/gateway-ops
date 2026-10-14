import { useEffect, useMemo, useState } from "react"
import { Download, History, Play } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { useRelayTestRuns } from "@/lib/queries"
import { relayTestReportSVG } from "@/lib/relay-test-report"
import type { RelayGroupView, RelayTestRun } from "@/lib/api-types"
import { cn } from "@/lib/utils"
const timeLabel = (value: string) => new Date(value).toLocaleString("zh-CN", {month:"2-digit",day:"2-digit",hour:"2-digit",minute:"2-digit",second:"2-digit",hour12:false})
function downloadReport(run: RelayTestRun) {
  const url=URL.createObjectURL(new Blob([relayTestReportSVG(run)],{type:"image/svg+xml;charset=utf-8"}))
  const link=document.createElement("a"); link.href=url;link.download=`relay-${run.relay_station_id}-test-${run.id}.svg`;link.click()
  window.setTimeout(()=>URL.revokeObjectURL(url),1000)
}
export function RelayLatestTest({run,onOpen,compact=false}: {run?:RelayTestRun;onOpen:()=>void;compact?:boolean}) {
  return <button type="button" onClick={onOpen} className={cn("flex min-w-0 items-center gap-2 rounded-md border border-border bg-muted/20 px-3 py-2 text-left transition-colors hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring",compact && "border-0 bg-transparent px-1 py-1 text-[10px]")} aria-label="查看链路测试历史">
    <History className="size-3.5 shrink-0 text-muted-foreground"/>
    {run ? <><svg viewBox="0 0 80 20" className="h-5 w-16 shrink-0" role="img" aria-label={`最新测试：成功 ${run.succeeded}，失败 ${run.failed}`}><path d="M8 10 H72" stroke={(run.failed || run.error) ? "#ef4444" : "#34d399"} strokeWidth="2"/>{[8,40,72].map((x)=><circle key={x} cx={x} cy="10" r="4" fill={(run.failed || run.error) ? "#ef4444" : "#34d399"}/>)}</svg><span className="min-w-0"><span className={cn("block truncate font-medium",(run.failed || run.error) ? "text-danger" : "text-success")}>{run.setup_error ? "测试未执行" : compact ? `${run.succeeded}/${run.results.length} 成功` : `最新链路测试：成功 ${run.succeeded}，失败 ${run.failed} · ${run.target_name}`}</span><span className="block text-[10px] text-muted-foreground">{timeLabel(run.finished_at)}</span></span></> : <span className="text-xs text-muted-foreground">暂无链路测试记录</span>}
  </button>
}
export function RelayTestHistory({stationID,stationName,open,onOpenChange}: {stationID:number|null;stationName:string;open:boolean;onOpenChange:(open:boolean)=>void}) {
  const history=useRelayTestRuns(open ? stationID : null)
  const [selectedID,setSelectedID]=useState<number|null>(null)
  useEffect(()=>setSelectedID(null),[stationID,open])
  const run=history.data?.find((item)=>item.id===selectedID) ?? history.data?.[0]
  const svg=useMemo(()=>run ? relayTestReportSVG(run) : "",[run])
  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent className="flex max-h-[90dvh] w-[calc(100vw-2rem)] max-w-6xl flex-col overflow-hidden">
    <DialogHeader><DialogTitle>{stationName} · 链路测试历史</DialogTitle><DialogDescription>每个中转站保留最近 30 次测试，新记录在前。每次测试的图和调用结果都可重新查看。</DialogDescription></DialogHeader>
    {history.error ? <div role="alert" className="text-sm text-danger">{history.error}<Button variant="outline" className="ml-3" onClick={()=>void history.refetch()}>重试</Button></div> : null}
    {history.loading && !history.data ? <p className="py-8 text-center text-sm text-muted-foreground">正在读取历史…</p> : !run ? <p className="py-8 text-center text-sm text-muted-foreground">还没有测试记录</p> : <div className="grid min-h-0 flex-1 gap-4 overflow-y-auto md:grid-cols-[220px_minmax(0,1fr)]">
      <nav aria-label="测试时间轴" className="max-h-48 space-y-2 overflow-y-auto md:max-h-none">{history.data?.map((item)=><button key={item.id} type="button" onClick={()=>setSelectedID(item.id)} aria-current={item.id===run.id ? "true" : undefined} className={cn("block w-full rounded-md border-l-2 p-3 text-left text-xs",item.id===run.id ? "border-brand bg-brand/10" : "border-border hover:bg-muted")}><span className="block font-medium">{timeLabel(item.finished_at)}</span><span className="mt-1 block truncate text-muted-foreground">{item.target_name} · {item.model}</span><span className={cn("mt-1 block",(item.failed || item.error) ? "text-danger" : "text-success")}>{item.setup_error ? "测试未执行" : `成功 ${item.succeeded} / 失败 ${item.failed}`}</span></button>)}</nav>
      <div className="min-w-0 space-y-3 md:overflow-y-auto"><div className="flex items-center justify-between gap-2"><span className="text-xs text-muted-foreground">{run.target_type==="group" ? "真实分组调用" : "账号连接测试"} · {run.mode==="stream" ? "流式" : "普通"}</span><Button size="sm" variant="outline" onClick={()=>downloadReport(run)} className="gap-1"><Download className="size-3.5"/>下载 SVG</Button></div><div className="overflow-x-auto rounded-md border border-border"><div className="min-w-[640px]" dangerouslySetInnerHTML={{__html:svg}}/></div>
      {run.error ? <p role="alert" className="break-words rounded-md bg-danger/10 p-3 text-xs text-danger">{run.error}</p> : null}
      {run.results.map((call)=><details key={call.index} className="rounded-md border border-border p-3"><summary className="cursor-pointer text-xs">#{call.index} · {call.success ? "成功" : "失败"} · {call.duration_ms} ms · {call.status_code ? `HTTP ${call.status_code}` : "无 HTTP 响应"}</summary><pre className="mt-2 max-h-48 overflow-auto whitespace-pre-wrap break-all text-xs">{call.output || "无输出"}</pre></details>)}</div>
    </div>}
  </DialogContent></Dialog>
}
export function RelayLinkTestPicker({open,onOpenChange,groups,onTest}: {open:boolean;onOpenChange:(open:boolean)=>void;groups:RelayGroupView[];onTest:(group:RelayGroupView)=>void}) {
  const [groupID,setGroupID]=useState("")
  const selected=groups.find((group)=>String(group.external_id)===groupID) ?? groups[0]
  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent><DialogHeader><DialogTitle>中转站链路测试</DialogTitle><DialogDescription>选择销售分组后设置模型和次数，执行真实调用。测试会消耗管理员额度，结果自动保存。</DialogDescription></DialogHeader>
    {groups.length ? <><Select value={selected ? String(selected.external_id) : ""} onValueChange={setGroupID}><SelectTrigger aria-label="测试分组"><SelectValue/></SelectTrigger><SelectContent>{groups.map((group)=><SelectItem key={group.external_id} value={String(group.external_id)}>{group.name}</SelectItem>)}</SelectContent></Select><Button className="gap-1.5" onClick={()=>{if(selected){onOpenChange(false);onTest(selected)}}}><Play className="size-4"/>选择模型并测试</Button></> : <p className="text-sm text-muted-foreground">当前没有可测试的分组，请先同步中转站。</p>}
  </DialogContent></Dialog>
}
