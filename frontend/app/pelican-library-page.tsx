import { ProductIcon } from "@/components/product-branding"
import { useMemo, useState } from "react"
import { Link, useSearchParams } from "react-router-dom"
import { Bird, RefreshCw, Search, Settings2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { ChannelPelicanPanel } from "@/components/monitor/channel-pelican-panel"
import { PelicanTargetPanel } from "@/components/relay/pelican-target-panel"
import { useChannelPelicanGallery, usePelicanSources, useSystemSettings } from "@/lib/queries"
import { ConfigurationLoading } from "@/components/ui/configuration-loading"

export default function PelicanLibraryPage() {
  const settings = useSystemSettings()
  if (!settings.data) return <ConfigurationLoading query={settings} title="鹈鹕测智" />
  if (!settings.data.pelican_page_enabled) return <section className="rounded-lg border border-border bg-card p-6">
    <h1 className="text-lg font-semibold"><ProductIcon className="mr-2 size-5 align-middle"/>鹈鹕测智页面已隐藏</h1>
    <p className="mt-2 text-sm text-muted-foreground">可在系统设置中重新开启页面展示。</p>
    <Button asChild className="mt-4" size="sm"><Link to="/settings#pelican-settings">前往测智设置</Link></Button>
  </section>
  return <PelicanLibraryContent historyCount={settings.data.pelican_history_count} />
}

function PelicanLibraryContent({historyCount}:{historyCount:number}) {
  const query=useChannelPelicanGallery()
  const [params,setParams]=useSearchParams()
  const channelID=params.get("channel_id")??"all"
  const sourceID=params.get("source_id")??"all"
  const sourcesQuery=usePelicanSources(sourceID)
  const [search,setSearch]=useState("")
  const [stationID,setStationID]=useState("all")
  const [status,setStatus]=useState("all")
  const stations=useMemo(()=>{
    const entries=new Map<number,string>()
    for(const group of query.data?.groups??[])for(const target of group.targets)entries.set(target.relay_station_id,target.station_name)
    for(const group of sourcesQuery.data?.groups??[])entries.set(group.station_id,group.station_name)
    return [...entries]
  },[query.data,sourcesQuery.data])
  const groups=(query.data?.groups??[]).filter(group=>{
    if(sourceID!=="all")return false
    if(channelID!=="all"&&String(group.channel.id)!==channelID)return false
    if(channelID==="all"&&!group.targets.length&&!group.results.length)return false
    if(stationID!=="all"&&!group.targets.some(target=>String(target.relay_station_id)===stationID))return false
    if(status!=="all"&&!group.results.some(result=>result.status===status&&(stationID==="all"||String(result.relay_station_id)===stationID))&&group.targets.every(target=>target.config.enabled))return false
    const text=[group.channel.name,group.channel.site_url,...group.targets.flatMap(target=>[target.station_name,target.account_name]),...group.results.map(result=>result.model)].join(" ").toLowerCase()
    return text.includes(search.trim().toLowerCase())
  })
  const sourceGroups=(sourcesQuery.data?.groups??[]).filter(group=>{
    if(channelID!=="all")return false
    if(sourceID!=="all"&&group.id!==sourceID)return false
    if(stationID!=="all"&&String(group.station_id)!==stationID)return false
    if(status!=="all"&&!group.results.some(result=>result.status===status)&&group.config.enabled)return false
    return [group.name,group.station_name,group.site_url,...group.results.map(result=>result.model)].join(" ").toLowerCase().includes(search.trim().toLowerCase())
  })
  return <section className="min-w-0 space-y-4">
      <header className="flex flex-wrap items-center justify-between gap-3 border-l-2 border-brand pl-3">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-bold"><ProductIcon className="size-5 sm:size-6"/>{"\u9e48\u9e55\u6d4b\u667a\u4f5c\u54c1\u5e93"}</h1>
          <p className="mt-1 text-xs text-muted-foreground">渠道、号池账号和中转站分组各自一组，最新结果在前，每组最多 {historyCount} 份。点击作品可交互播放。</p>
        </div>
        <div className="flex flex-wrap gap-2"><Button asChild variant="outline" size="sm"><Link to="/settings#pelican-settings"><Settings2 className="mr-1 size-3.5"/>全局测智设置</Link></Button><Button type="button" variant="outline" size="sm" disabled={query.refreshing||sourcesQuery.refreshing} onClick={()=>void Promise.all([query.refetch(),sourcesQuery.refetch()]).catch(()=>{})} aria-label="刷新作品列表"><RefreshCw className={`mr-1 size-3.5 ${query.refreshing?"animate-spin":""}`}/>刷新</Button></div>
      </header>

      <section className="space-y-3" aria-label="测智作品筛选">
        <div className="grid grid-cols-1 gap-3 md:grid-cols-[minmax(16rem,1fr)_minmax(12rem,0.6fr)_minmax(10rem,0.45fr)]">
          <div className="space-y-1.5"><Label htmlFor="pelican-library-search">搜索渠道或模型</Label><div className="relative"><Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" /><Input id="pelican-library-search" className="pl-9" value={search} onChange={event => setSearch(event.target.value)} placeholder="渠道、中转站、账号或模型" /></div></div>
          <div className="space-y-1.5"><Label htmlFor="pelican-library-station">中转站</Label><Select value={stationID} onValueChange={setStationID}><SelectTrigger id="pelican-library-station" className="w-full"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">全部中转站</SelectItem>{stations.map(([id,name])=><SelectItem key={id} value={String(id)}>{name}</SelectItem>)}</SelectContent></Select></div>
          <div className="space-y-1.5"><Label htmlFor="pelican-library-status">测试状态</Label><Select value={status} onValueChange={setStatus}><SelectTrigger id="pelican-library-status" className="w-full"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">全部结果</SelectItem><SelectItem value="success">成功</SelectItem><SelectItem value="failed">失败</SelectItem></SelectContent></Select></div>
        </div>

        <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border pb-3 text-xs text-muted-foreground">
          <span>共 {groups.length+sourceGroups.length} 组 · 每组最多 {historyCount} 份结果</span>
          {channelID!=="all"||sourceID!=="all"?<Button variant="link" size="sm" className="h-7 px-0 text-xs" onClick={()=>setParams({})}>查看全部来源</Button>:null}
        </div>
      </section>

      {query.loading&&!query.data?<p role="status" className="py-8 text-center text-sm text-muted-foreground">正在读取测智记录…</p>:null}
      {query.error?<p role="alert" className="text-sm text-danger">读取测智作品失败：{query.error}</p>:null}
      {sourcesQuery.loading&&!sourcesQuery.data?<p role="status" className="text-xs text-muted-foreground">正在读取号池及分组作品…</p>:null}
      {sourcesQuery.error?<p role="alert" className="text-xs text-danger">读取号池及分组作品失败：{sourcesQuery.error}</p>:null}
      <div className="min-w-0 space-y-5" aria-label="按渠道分组的测智结果">
        {groups.map(group=><section key={group.channel.id} data-pelican-channel={group.channel.id} className="min-w-0 rounded-xl border border-border bg-card p-3 sm:p-4">
          <ChannelPelicanPanel channel={group.channel} query={query} limit={historyCount} showMore={false} status={status} stationID={stationID} compact/>
        </section>)}
        {sourceGroups.map(group=><section key={group.id} data-pelican-source={group.id} className="min-w-0 rounded-xl border border-border bg-card p-3 sm:p-4">
          <PelicanTargetPanel stationID={group.station_id} externalID={group.external_id} name={`${group.station_name} · ${group.name}`} siteURL={group.site_url} scope={group.kind==="relay_group"?"group":"account"} compact status={status} query={{...sourcesQuery,data:{configs:[{...group.config,account_external_id:group.external_id}],results:group.results}}}/>
        </section>)}
      </div>
      {!query.loading&&!sourcesQuery.loading&&!query.error&&!sourcesQuery.error&&!groups.length&&!sourceGroups.length?<p className="rounded-lg border border-dashed p-8 text-center text-sm text-muted-foreground">没有符合条件的测智记录。</p>:null}
    </section>
}
