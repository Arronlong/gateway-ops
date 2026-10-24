import { useEffect, useMemo, useState } from "react"
import { Link } from "react-router-dom"
import { ExternalLink } from "lucide-react"
import { Button } from "@/components/ui/button"
import { PelicanAccountPanel, PelicanProvider, PelicanResultsStrip, PelicanTargetRunButton } from "./pelican"
import type { PelicanGallery } from "@/lib/api-types"
import { useGroupPelicanGallery, useSystemSettings, type QueryState } from "@/lib/queries"
import { getPelicanFilterOptions, matchesPelicanFilters } from "@/lib/pelican-library"
import { pelicanEffortOptions } from "@/lib/pelican"

export function PelicanTargetPanel({stationID,externalID,name,scope,query,compact=false,siteURL,status="all"}:{
 stationID:number;externalID:number;name:string;scope:"account"|"group";query:QueryState<PelicanGallery>;compact?:boolean;siteURL?:string;status?:string
}) {
 const settings=useSystemSettings()
 const [model,setModel]=useState("all"),[effort,setEffort]=useState("all")
 const sourceResults=useMemo(()=>(query.data?.results??[]).filter(result=>scope==="group"?result.group_external_id===externalID:result.account_external_id===externalID),[query.data,scope,externalID])
 const filterOptions=useMemo(()=>getPelicanFilterOptions([{results:sourceResults}]),[sourceResults])
 useEffect(()=>{if(model!=="all"&&!filterOptions.models.includes(model))setModel("all");if(effort!=="all"&&!filterOptions.efforts.includes(effort))setEffort("all")},[filterOptions,model,effort])
 const limit=compact?(settings.data?.pelican_history_count??30):3
 const results=sourceResults.filter(result=>(status==="all"||result.status===status)&&matchesPelicanFilters(result,model,effort)).slice(0,limit)
 const filtered={...query,data:query.data?{configs:query.data.configs,results}:null}
 if(!settings.data?.pelican_page_enabled)return null
 return <div className="min-w-0 space-y-2" data-pelican-target={`${scope}:${stationID}:${externalID}`}>
  <PelicanProvider stationID={stationID} galleryQuery={filtered} scope={scope}>
  {compact?<header className="flex min-w-0 flex-wrap items-center gap-2" data-pelican-source-header>
   <h2 className="min-w-0 max-w-full truncate text-sm font-semibold" title={name}>{siteURL?<a href={siteURL} target="_blank" rel="noopener noreferrer" className="inline-flex max-w-full items-center gap-1 text-foreground hover:text-brand hover:underline"><span className="truncate">{name}</span><ExternalLink className="size-3 shrink-0"/></a>:name}</h2>
   <span className="text-[10px] text-muted-foreground">{results.length}/{limit}</span>
   <span className="rounded border px-1.5 py-0.5 text-[10px] text-muted-foreground">{scope==="group"?"中转站分组":"本地号池"}</span>
   {filterOptions.models.length?<div className="flex max-w-full flex-wrap items-center gap-1 text-[10px]" role="group" aria-label={`${name}按模型筛选`}><span>模型</span>{["all",...filterOptions.models].map(value=><Button key={value} size="sm" variant={model===value?"default":"outline"} className="h-6 max-w-full px-1.5 text-[10px]" aria-pressed={model===value} onClick={()=>setModel(value)} title={value}><span className="max-w-64 truncate">{value==="all"?"全部":value}</span></Button>)}</div>:null}
   {filterOptions.efforts.length?<div className="flex flex-wrap items-center gap-1 text-[10px]" role="group" aria-label={`${name}按思考强度筛选`}><span>思考强度</span>{["all",...filterOptions.efforts].map(value=><Button key={value} size="sm" variant={effort===value?"default":"outline"} className="h-6 px-1.5 text-[10px]" aria-pressed={effort===value} onClick={()=>setEffort(value)}>{value==="all"?"全部":pelicanEffortOptions[value]}</Button>)}</div>:null}
   <div className="ml-auto">{compact?<PelicanAccountPanel account={{external_id:externalID,name,relay_station_id:stationID}} controlsOnly compactControls/>:<PelicanTargetRunButton externalID={externalID} name={name}/>}</div>
  </header>:null}
  {query.error?<p role="alert" className="text-xs text-danger">读取测智结果失败：{query.error}</p>:null}
   {compact?null:<PelicanAccountPanel account={{external_id:externalID,name,relay_station_id:stationID}} controlsOnly/>}
   <PelicanResultsStrip limit={limit} grid={!compact} allowDelete={compact}/>
  </PelicanProvider>
  {!compact?<div className="flex items-center justify-between text-[10px] text-muted-foreground"><span>{sourceResults.length}/{settings.data.pelican_history_count} 份结果</span><Link className="hover:text-brand hover:underline" to={`/pelican?source_id=${encodeURIComponent(`${scope==="group"?"group":"pool"}:${stationID}:${externalID}`)}`}>查看更多</Link></div>:null}
 </div>
}

export function GroupPelicanPanel({stationID,group,query}:{stationID:number;group:{external_id:number;name:string};query?:QueryState<PelicanGallery>}) {
 const settings=useSystemSettings()
 const fallback=useGroupPelicanGallery(query||!settings.data?.pelican_page_enabled?null:stationID)
 if(!settings.data?.pelican_page_enabled)return null
 return <div className="max-w-2xl px-4 py-3"><PelicanTargetPanel stationID={stationID} externalID={group.external_id} name={group.name} scope="group" query={query??fallback}/></div>
}

export function PublicGroupPelicanResults({stationID,groupID,query}:{stationID:number;groupID:number;query:QueryState<PelicanGallery>}) {
 const data=useMemo(()=>query.data?{configs:[],results:query.data.results.filter(result=>result.group_external_id===groupID)}:null,[query.data,groupID])
 if(query.data?.page_enabled===false)return null
 if(query.loading&&!data)return <p className="text-xs text-muted-foreground">正在读取测智作品…</p>
 if(query.error)return <p className="text-xs text-muted-foreground">测智作品暂时不可用</p>
 return <section className="mt-3 min-w-0 border-t border-border pt-3" aria-label="分组鹈鹕测智结果">
  <p className="mb-2 text-xs font-medium">鹈鹕测智结果</p>
  <PelicanProvider stationID={stationID} galleryQuery={{...query,data}} scope="group" readOnly><PelicanResultsStrip limit={3} grid allowDelete={false}/></PelicanProvider>
 </section>
}
