import { useEffect, useMemo, useRef, useState } from "react"
import { createPelicanDocument, fitPelicanArtwork, pelicanViewport } from "@/lib/pelican"

export function PelicanArtwork({html,title,playing=true,speed=1,zoom=0,interactive=true}: {html:string;title:string;playing?:boolean;speed?:number;zoom?:number;interactive?:boolean}) {
 const container=useRef<HTMLDivElement>(null),frame=useRef<HTMLIFrameElement>(null)
 const [available,setAvailable]=useState({width:0,height:0})
 const [pageVisible,setPageVisible]=useState(()=>!document.hidden)
 const viewport=useMemo(()=>pelicanViewport(html),[html])
 const channel=useMemo(()=>crypto.randomUUID(),[html])
 const srcdoc=useMemo(()=>createPelicanDocument(html,channel,{thumbnail:!interactive}),[html,channel,interactive])
 const fitted=fitPelicanArtwork(viewport,available)
 const scale=zoom||fitted.scale
 const control=useRef({playing,speed});control.current={playing:playing&&pageVisible,speed}
 const send=()=>frame.current?.contentWindow?.postMessage({type:"pelican-control",channel,...control.current},"*")
 useEffect(()=>{
  const target=container.current;if(!target)return
  const resize=(width:number,height:number)=>{
   const next={width:Math.floor(width),height:Math.floor(height)}
   setAvailable(current=>current.width===next.width&&current.height===next.height?current:next)
  }
  const bounds=target.getBoundingClientRect();resize(bounds.width,bounds.height)
  const observer=new ResizeObserver(entries=>{const entry=entries[0];if(entry)resize(entry.contentRect.width,entry.contentRect.height)});observer.observe(target)
  return()=>observer.disconnect()
 },[])
 useEffect(()=>{const update=()=>setPageVisible(!document.hidden);document.addEventListener("visibilitychange",update);return()=>document.removeEventListener("visibilitychange",update)},[])
 useEffect(()=>{send()},[playing,speed,channel,pageVisible])
 useEffect(()=>{
  const receive=(event:MessageEvent)=>{if(event.source===frame.current?.contentWindow&&event.data?.type==="pelican-ready"&&event.data.channel===channel)send()}
  window.addEventListener("message",receive);return()=>window.removeEventListener("message",receive)
 },[channel])
 return <div ref={container} className={`h-full w-full bg-slate-100 ${interactive&&zoom>0?"overflow-auto":"overflow-hidden"}`} data-pelican-artwork>
  <div className="flex min-h-full min-w-full items-center justify-center" style={{width:Math.max(available.width,viewport.width*scale),height:Math.max(available.height,viewport.height*scale)}}>
   <div className="relative shrink-0 overflow-hidden" style={{width:viewport.width*scale,height:viewport.height*scale}}>
    <iframe ref={frame} title={title} srcDoc={srcdoc} sandbox="allow-scripts" referrerPolicy="no-referrer" scrolling={interactive?"auto":"no"} tabIndex={interactive?0:-1} onLoad={send} className="absolute left-0 top-0 border-0" style={{width:viewport.width,height:viewport.height,transform:`scale(${scale})`,transformOrigin:"top left",pointerEvents:interactive?"auto":"none"}}/>
   </div>
  </div>
 </div>
}
