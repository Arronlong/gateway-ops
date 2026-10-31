import { useEffect, useMemo, useState } from "react"
import { Link } from "react-router-dom"
import { Bird, ChevronRight, ExternalLink, RefreshCw } from "lucide-react"
import { Button } from "@/components/ui/button"
import { PelicanAccountPanel, PelicanChannelRunButton, PelicanProvider, PelicanResultsStrip, PelicanRunButton } from "@/components/relay/pelican"
import type { Channel, ChannelPelicanGallery, PelicanGallery } from "@/lib/api-types"
import { useSystemSettings, type QueryState } from "@/lib/queries"
import { getPelicanFilterOptions, matchesPelicanFilters } from "@/lib/pelican-library"
import { pelicanEffortOptions } from "@/lib/pelican"

export function ChannelPelicanPanel({channel,query,limit=3,showMore=true,status="all",stationID="all",compact=false}:{
  channel: Pick<Channel,"id"|"name"|"site_url">
  query: QueryState<ChannelPelicanGallery>
  limit?:number
  showMore?:boolean
  status?:string
  stationID?:string
  compact?:boolean
}) {
  const settings=useSystemSettings()
  const [modelFilter,setModelFilter]=useState("all")
  const [effortFilter,setEffortFilter]=useState("all")
  const tasksEnabled=settings.data?.pelican_tasks_enabled===true
  const displayLimit=compact?(settings.data?.pelican_history_count??30):limit
  const group=query.data?.groups.find(item=>item.channel.id===channel.id)
  const filterOptions=useMemo(()=>getPelicanFilterOptions(group?[group]:[]),[group])
  useEffect(()=>{
    if(group&&modelFilter!=="all"&&!filterOptions.models.includes(modelFilter))setModelFilter("all")
    if(group&&effortFilter!=="all"&&!filterOptions.efforts.includes(effortFilter))setEffortFilter("all")
  },[effortFilter,filterOptions,group,modelFilter])
  const retainedResults=(group?.results??[]).filter(result=>(status==="all"||result.status===status)&&(stationID==="all"||String(result.relay_station_id)===stationID)&&matchesPelicanFilters(result,modelFilter,effortFilter))
  const results=retainedResults.slice(0,displayLimit)
  const galleryQuery:QueryState<PelicanGallery>={...query,data:group?{configs:group.targets.map(target=>target.config),results}:null}
  const targets=group?.targets.filter(target=>stationID==="all"||String(target.relay_station_id)===stationID)??[]
  const directTarget=Boolean(group?.channel.pelican_key_configured&&targets.length===0)
  const nextRun=targets.filter(target=>target.config.enabled&&target.config.next_run_at).map(target=>target.config.next_run_at!).sort((a,b)=>Date.parse(a)-Date.parse(b))[0]
  const scheduleLabel=!tasksEnabled?"任务已暂停":nextRun?`下次 ${new Date(nextRun).toLocaleString("zh-CN",{month:"2-digit",day:"2-digit",hour:"2-digit",minute:"2-digit",hour12:false,timeZone:"Asia/Shanghai"})}`:"定时已关闭"
  return <div className={`${compact?"":"mt-3 "}min-w-0 space-y-2`} data-channel-pelican={channel.id}>
    {query.loading&&!query.data?<p role="status" className="border-t border-border py-3 text-xs text-muted-foreground">正在读取测智记录…</p>:null}
    {query.error?<div role="alert" className="space-y-2 text-xs text-danger"><p>读取测智记录失败：{query.error}</p><Button variant="outline" size="sm" onClick={()=>void query.refetch().catch(()=>{})}><RefreshCw className="mr-1 size-3"/>重试</Button></div>:null}
    {group?<PelicanProvider stationID={null} galleryQuery={galleryQuery} showKeyGroup>
      {compact?<header className="flex flex-wrap items-center gap-x-4 gap-y-2" data-pelican-group-header>
        <h2 className="min-w-0 max-w-full truncate text-sm font-semibold" title={channel.name}>{channel.site_url?<a href={channel.site_url} target="_blank" rel="noopener noreferrer" aria-label={`打开上游网站：${channel.name}`} title={channel.site_url} className="inline-flex max-w-full items-center gap-1.5 text-foreground hover:text-brand hover:underline"><span className="truncate">{channel.name}</span><ExternalLink className="size-3.5 shrink-0" aria-hidden="true"/></a>:channel.name}<span className="ml-2 text-[10px] font-normal text-muted-foreground">{retainedResults.length}/{displayLimit}</span></h2>
        {filterOptions.models.length?<div role="group" aria-label={`${channel.name}按模型筛选`} className="flex min-w-0 max-w-full flex-wrap items-center gap-1.5 text-xs">
          <span className="text-muted-foreground">模型</span>
          <Button type="button" size="sm" variant={modelFilter==="all"?"default":"outline"} aria-pressed={modelFilter==="all"} className="h-7 px-2 text-xs" onClick={()=>setModelFilter("all")}>全部</Button>
          {filterOptions.models.map(model=><Button key={model} type="button" size="sm" variant={modelFilter===model?"default":"outline"} aria-pressed={modelFilter===model} title={model} className="h-7 max-w-full min-w-0 px-2 text-xs" onClick={()=>setModelFilter(model)}><span className="max-w-[18rem] truncate">{model}</span></Button>)}
        </div>:null}
        {filterOptions.efforts.length?<div role="group" aria-label={`${channel.name}按思考强度筛选`} className="flex min-w-0 max-w-full flex-wrap items-center gap-1.5 text-xs">
          <span className="text-muted-foreground">思考强度</span>
          <Button type="button" size="sm" variant={effortFilter==="all"?"default":"outline"} aria-pressed={effortFilter==="all"} className="h-7 px-2 text-xs" onClick={()=>setEffortFilter("all")}>全部</Button>
          {filterOptions.efforts.map(effort=><Button key={effort} type="button" size="sm" variant={effortFilter===effort?"default":"outline"} aria-pressed={effortFilter===effort} className="h-7 px-2 text-xs" onClick={()=>setEffortFilter(effort)}>{pelicanEffortOptions[effort]}</Button>)}
        </div>:null}
        {compact&&targets.length?<div className="ml-auto flex flex-wrap items-center gap-2">
          {targets.map(target=><div key={`${target.relay_station_id}/${target.account_external_id}`} className="inline-flex items-center gap-2" data-pelican-channel-account={target.account_external_id}>
            {targets.length>1?<span className="max-w-40 truncate text-[10px] text-muted-foreground" title={`${target.station_name} · ${target.account_name}`}>{target.account_name||target.station_name}</span>:null}
            <PelicanAccountPanel account={{external_id:target.account_external_id,name:target.account_name||channel.name,relay_station_id:target.relay_station_id,channel_id:channel.id}} controlsOnly compactControls/>
          </div>)}
        </div>:null}
        {compact?directTarget?<div className="ml-auto"><PelicanChannelRunButton channelID={channel.id} channelName={channel.name} onRefresh={query.refetch}/></div>:null:<div className="ml-auto">{directTarget?<PelicanChannelRunButton channelID={channel.id} channelName={channel.name} onRefresh={query.refetch}/>:<PelicanRunButton targets={targets} channelName={channel.name} channelID={channel.id}/>}</div>}
      </header>:targets.length?targets.map(target=><div key={`${target.relay_station_id}/${target.account_external_id}`} className="min-w-0">
        {targets.length>1?<p className="truncate pt-2 text-[10px] text-muted-foreground" title={`${target.station_name} · ${target.account_name}`}>{target.station_name} · {target.account_name}</p>:null}
        <PelicanAccountPanel account={{external_id:target.account_external_id,name:target.account_name||channel.name,relay_station_id:target.relay_station_id,channel_id:channel.id}} controlsOnly/>
      </div>):directTarget?<div className="flex items-center gap-2 border-t border-border pt-3 text-xs text-muted-foreground"><Bird className="size-3.5"/>鹈鹕测智 · 使用渠道中配置的 API Key 直连测试。</div>:<p className="flex items-center gap-1.5 border-t border-border pt-3 text-xs text-muted-foreground"><Bird className="size-3.5"/>鹈鹕测智 · 请先在“渠道 / 账号”中关联中转站账号。</p>}
      {!compact?<div className="flex min-w-0 items-center gap-2 whitespace-nowrap text-[10px] text-muted-foreground" data-pelican-summary>
        <span className="min-w-0 truncate" title={!tasksEnabled?"请在系统设置中开启测智任务执行":nextRun?`${scheduleLabel}（北京时间）`:"定时测智已关闭，仍可手动测试"}>{scheduleLabel}</span>
        <span className="shrink-0" title={`最近 ${displayLimit} 份 · 最新在前`}>{retainedResults.length}/{settings.data?.pelican_history_count??30} 份</span>
        {showMore?<Button asChild variant="link" size="sm" className="ml-auto h-5 shrink-0 gap-0.5 px-0 text-[10px] has-[>svg]:px-0"><Link to={`/pelican?channel_id=${channel.id}`} aria-label={`${channel.name}查看更多测智结果`}>查看更多<ChevronRight className="size-3"/></Link></Button>:null}
      </div>:null}
      {targets.length||results.length?<>
        <PelicanResultsStrip limit={displayLimit} allowDelete={compact} grid={!compact}/>
      </>:null}
    </PelicanProvider>:null}
  </div>
}
