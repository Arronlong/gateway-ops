import type { PelicanConfig, PelicanTestPlan, PelicanResult } from "./api-types"
// @ts-expect-error Node 22 原生 TypeScript 运行时要求显式扩展名。
import { defaultPelicanPrompt } from "./system-settings.ts"

export { defaultPelicanPrompt }

export function pelicanGroupLabels(result: Pick<PelicanResult,"upstream_group_name"|"upstream_group_multiplier">) {
 const multiplier=result.upstream_group_multiplier
 return {name:result.upstream_group_name?.trim()||"分组未知",rate:typeof multiplier==="number"&&Number.isFinite(multiplier)&&multiplier>=0?`${multiplier}×`:"倍率未知"}
}

export const pelicanSchedules = [
 {value:"*/15 * * * *",label:"每 15 分钟"}, {value:"*/30 * * * *",label:"每 30 分钟"},
 {value:"0 * * * *",label:"每 1 小时"}, {value:"0 */2 * * *",label:"每 2 小时"},
 {value:"0 */6 * * *",label:"每 6 小时"}, {value:"0 9 * * *",label:"每天 9:00"},
]
export const pelicanEfforts: Record<string,string> = {none:"\u65e0",minimal:"\u6700\u4f4e",low:"\u4f4e",medium:"\u4e2d",high:"\u9ad8",xhigh:"\u6781\u9ad8",max:"\u6700\u9ad8",ultra:"\u8d85\u9ad8"}
export const pelicanEffortOptions: Record<string,string> = {none:"\u65e0 - None",minimal:"\u6700\u4f4e - Minimal",low:"\u4f4e - Low",medium:"\u4e2d - Medium",high:"\u9ad8 - High",xhigh:"\u6781\u9ad8 - XHigh",max:"\u6700\u9ad8 - Max",ultra:"\u8d85\u9ad8 - Ultra"}
export function defaultPelicanPlan(): PelicanTestPlan {
 return {enabled:true,model:"",prompt:defaultPelicanPrompt,reasoning_effort:"medium"}
}
export type PelicanSettingsSource = {
 pelican_plans?: unknown
 pelican_model?: unknown
 pelican_prompt?: unknown
 pelican_reasoning_effort?: unknown
 pelican_parallel_count?: unknown
}
function normalizePelicanPlan(value: unknown): PelicanTestPlan {
 const plan=value&&typeof value==="object"?value as Record<string,unknown>:{}
 return {
  ...(typeof plan.enabled==="boolean"?{enabled:plan.enabled}:{}),
  model:typeof plan.model==="string"?plan.model:"",
  prompt:typeof plan.prompt==="string"?plan.prompt:defaultPelicanPrompt,
  reasoning_effort:typeof plan.reasoning_effort==="string"?plan.reasoning_effort:"medium",
 }
}
export function normalizePelicanPlans(settings: PelicanSettingsSource = {}): PelicanTestPlan[] {
 if(Array.isArray(settings.pelican_plans)){
  const plans=settings.pelican_plans.slice(0,8).map(normalizePelicanPlan)
  return plans.length?plans:[defaultPelicanPlan()]
 }
 return [normalizePelicanPlan({model:settings.pelican_model,prompt:settings.pelican_prompt,reasoning_effort:settings.pelican_reasoning_effort})]
}
function pelicanModelEntries(model:string) { return model.split(/[\n\r,，;；]/).map(value=>value.trim()).filter(Boolean) }
export function pelicanRunOptions(settings: PelicanSettingsSource | null | undefined): PelicanTestPlan[] {
 const seen=new Set<string>()
 return normalizePelicanPlans(settings??{}).filter(plan=>plan.enabled!==false).flatMap(plan=>pelicanModelEntries(plan.model).flatMap(model=>{
  if(seen.has(model))return []
  seen.add(model)
  return [{...plan,model}]
 }))
}
function pelicanTextBytes(value:string) { return new TextEncoder().encode(value).length }
export function hasConfiguredPelicanPlan(settings?: PelicanSettingsSource | null): boolean {
 return normalizePelicanPlans(settings??{}).some(plan=>plan.enabled!==false&&pelicanModelEntries(plan.model).length>0)
}
export type PelicanPlanValidationField = "model" | "prompt" | "reasoning_effort" | "plans"
export type PelicanPlanValidationIssue = {index:number|null;field:PelicanPlanValidationField;message:string}
export function pelicanPlanValidationIssues(plans: PelicanTestPlan[]): PelicanPlanValidationIssue[] {
 const issues:PelicanPlanValidationIssue[]=[]
 if(plans.length<1||plans.length>8)issues.push({index:null,field:"plans",message:"\u65b9\u6848\u6570\u91cf\u9700\u4e3a 1\u20138 \u9879"})
 plans.forEach((plan,index)=>{
  if(pelicanTextBytes(plan.model)>256)issues.push({index,field:"model",message:"\u6a21\u578b\u4f18\u5148\u5217\u8868\u4e0d\u80fd\u8d85\u8fc7 256 \u4e2a\u5b57\u8282"})
  else if(plans.length>1&&pelicanModelEntries(plan.model).length===0)issues.push({index,field:"model",message:"\u591a\u65b9\u6848\u65f6\u8bf7\u4e3a\u6bcf\u9879\u586b\u5199\u6a21\u578b"})
  if(!plan.prompt.trim())issues.push({index,field:"prompt",message:"\u9898\u76ee\u4e0d\u80fd\u4e3a\u7a7a"})
  if(pelicanTextBytes(plan.prompt)>32000)issues.push({index,field:"prompt",message:"\u9898\u76ee\u4e0d\u80fd\u8d85\u8fc7 32000 \u4e2a\u5b57\u8282"})
  if(!Object.prototype.hasOwnProperty.call(pelicanEffortOptions,plan.reasoning_effort))issues.push({index,field:"reasoning_effort",message:"\u8bf7\u9009\u62e9\u6709\u6548\u7684\u601d\u8003\u5f3a\u5ea6"})
 })
 return issues
}
export function updatePelicanPlan(plans: PelicanTestPlan[],index: number,update: Partial<PelicanTestPlan>): PelicanTestPlan[] {
 if(!Number.isInteger(index)||index<0||index>=plans.length)return plans
 return plans.map((plan,position)=>position===index?{...plan,...update}:plan)
}
export function addPelicanPlan(plans: PelicanTestPlan[]): PelicanTestPlan[] {
 return plans.length>=8?plans:[...plans,defaultPelicanPlan()]
}
export function removePelicanPlan(plans: PelicanTestPlan[],index: number): PelicanTestPlan[] {
 if(plans.length<=1||!Number.isInteger(index)||index<0||index>=plans.length)return plans
 return plans.filter((_,position)=>position!==index)
}
export function defaultPelicanConfig(): PelicanConfig {
 return {enabled:false}
}
export function pelicanViewport(html: string) {
 if (/<html[\s>]/i.test(html)) return {width:1024,height:768}
 const match=html.match(/<svg[^>]*viewBox\s*=\s*["']\s*[-\d.]+[ ,]+[-\d.]+[ ,]+([\d.]+)[ ,]+([\d.]+)\s*["']/i)
 const width=Number(match?.[1]),height=Number(match?.[2])
 return width>0&&width<=4096&&height>0&&height<=4096 ? {width,height} : {width:1024,height:768}
}
export function fitPelicanArtwork(artwork: {width:number;height:number},container: {width:number;height:number}) {
 const scale=Math.max(0,Math.min(container.width/artwork.width,container.height/artwork.height))
 return {width:artwork.width*scale,height:artwork.height*scale,scale}
}

// 控制脚本先于模型脚本安装，共享 CSS、SMIL、rAF 和定时器的播放时钟。
const playbackBridge = String.raw`(() => {
 const channel=__CHANNEL__;
 const nativeRAF=window.requestAnimationFrame.bind(window), nativeCancel=window.cancelAnimationFrame.bind(window), nativeNow=performance.now.bind(performance), nativeTimeout=window.setTimeout.bind(window);
 const epoch=Date.now(), frames=new Map(), timers=new Map();
 let id=1, virtual=0, previous=nativeNow(), playing=true, speed=1, scheduled=0;
 function schedule(){if(playing&&!scheduled){previous=nativeNow();scheduled=nativeRAF(tick)}}
 window.requestAnimationFrame=callback=>{const key=id++;frames.set(key,callback);return key};
 window.cancelAnimationFrame=key=>frames.delete(key);
 window.setTimeout=(callback,delay=0,...args)=>{const key=id++;timers.set(key,{callback,args,due:virtual+Math.max(0,Number(delay)||0),repeat:0});return key};
 window.setInterval=(callback,delay=0,...args)=>{const key=id++;const repeat=Math.max(1,Number(delay)||1);timers.set(key,{callback,args,due:virtual+repeat,repeat});return key};
 window.clearTimeout=window.clearInterval=key=>timers.delete(key);
 try{Object.defineProperty(performance,'now',{value:()=>virtual});Date.now=()=>epoch+virtual}catch{}
 const smil=new Map();
 function synchronize(){
  for(const animation of document.getAnimations()){
   if(animation.playbackRate!==speed)animation.updatePlaybackRate(speed);
   if(playing&&animation.playState==='paused')animation.play();
   if(!playing&&animation.playState==='running')animation.pause();
  }
  for(const svg of document.querySelectorAll('svg')){
   if(!smil.has(svg)){try{smil.set(svg,{start:virtual,time:svg.getCurrentTime()});svg.pauseAnimations()}catch{}}
  }
 }
 window.addEventListener('animationstart',synchronize,true);
 new MutationObserver(synchronize).observe(document,{childList:true,subtree:true});
 window.addEventListener('message',event=>{
  if(event.source!==parent||event.data?.type!=='pelican-control'||event.data.channel!==channel)return;
  if(typeof event.data.playing==='boolean')playing=event.data.playing;
  if(Number.isFinite(event.data.speed))speed=Math.max(.25,Math.min(4,event.data.speed));
  if(!playing&&scheduled){nativeCancel(scheduled);scheduled=0}
  synchronize();schedule();
 });
 function tick(real){
  scheduled=0;
  const delta=Math.min(100,Math.max(0,real-previous));previous=real;
  if(playing){
   virtual+=delta*speed;
   const batch=Array.from(frames);frames.clear();for(const [key,callback]of batch){try{callback(virtual)}catch(error){console.error(error)}}
   for(const [key,timer]of timers){if(timer.due>virtual)continue;if(timer.repeat)timer.due=virtual+timer.repeat;else timers.delete(key);try{if(typeof timer.callback==='function')timer.callback(...timer.args)}catch(error){console.error(error)}}
   for(const [svg,clock]of smil){if(!svg.isConnected){smil.delete(svg);continue}try{svg.setCurrentTime(clock.time+(virtual-clock.start)/1000)}catch{}}
  }
  if(playing)scheduled=nativeRAF(tick);
 }
 schedule();
 function snapshot(){
  try{
   playing=false;if(scheduled){nativeCancel(scheduled);scheduled=0}synchronize();
   const clone=document.documentElement.cloneNode(true);
   const originals=[document.documentElement,...document.documentElement.querySelectorAll("*")];
   const copies=[clone,...clone.querySelectorAll("*")];
   const indexes=new Map(originals.map((node,index)=>[node,index]));
   for(const animation of document.getAnimations()){
    const target=animation.effect?.target,copy=copies[indexes.get(target)];
    if(!target||!copy?.style)continue;
    const computed=getComputedStyle(target);
    for(const frame of animation.effect.getKeyframes())for(const property of Object.keys(frame)){
     if(["offset","computedOffset","easing","composite"].includes(property))continue;
     const css=property.replace(/[A-Z]/g,letter=>"-"+letter.toLowerCase());
     copy.style.setProperty(css,computed.getPropertyValue(css),"important");
    }
    copy.style.setProperty("animation","none","important");
   }
   for(let index=0;index<originals.length;index++){
    const source=originals[index],copy=copies[index];
    if(copy.style){
     const computed=getComputedStyle(source);
     for(const property of ["width","height","min-width","min-height","max-width","max-height","font-size","font-family","line-height","letter-spacing","padding-top","padding-right","padding-bottom","padding-left","margin-top","margin-right","margin-bottom","margin-left","top","right","bottom","left","gap"]){
      const value=computed.getPropertyValue(property);if(value)copy.style.setProperty(property,value,"important");
     }
     if(index===0)for(const property of computed)if(property.startsWith("--"))copy.style.setProperty(property,computed.getPropertyValue(property));
    }
    if(source instanceof HTMLCanvasElement){const image=document.createElement("img");image.src=source.toDataURL();image.style.cssText=copy.style.cssText;image.width=source.width;image.height=source.height;copy.replaceWith(image)}
    for(const attr of [...copy.attributes])if(/^on/i.test(attr.name))copy.removeAttribute(attr.name);
   }
   for(const node of clone.querySelectorAll("script,meta,link,iframe,object,embed,base,audio,video,animate,animateTransform,animateMotion,set"))node.remove();
   const frozen=document.createElement("style");frozen.textContent="*,*::before,*::after{animation:none!important;transition:none!important;caret-color:transparent!important}";clone.append(frozen);
   const namespace="http://www.w3.org/2000/svg",svg=document.createElementNS(namespace,"svg"),foreign=document.createElementNS(namespace,"foreignObject");
   const viewport=__VIEWPORT__;
   svg.setAttribute("width","448");svg.setAttribute("height",String(Math.round(448*viewport.height/viewport.width)));
   svg.setAttribute("viewBox","0 0 "+viewport.width+" "+viewport.height);
   foreign.setAttribute("width",String(viewport.width));foreign.setAttribute("height",String(viewport.height));
   foreign.append(clone);svg.append(foreign);
   parent.postMessage({type:"pelican-thumbnail",channel,svg:new XMLSerializer().serializeToString(svg)},"*");
  }catch(error){parent.postMessage({type:"pelican-thumbnail",channel,error:String(error)},"*")}
 }
 if(__THUMBNAIL__)window.addEventListener("DOMContentLoaded",()=>nativeTimeout(snapshot,180),{once:true});
 window.addEventListener('DOMContentLoaded',()=>{synchronize();parent.postMessage({type:'pelican-ready',channel},'*')});
})();`

export function createPelicanDocument(html: string, channel: string, options: {thumbnail?:boolean} = {}) {
 const bridge=playbackBridge.replace("__CHANNEL__",JSON.stringify(channel).replace(/</g,()=>String.fromCharCode(92)+"u003c")).replace("__THUMBNAIL__",String(Boolean(options.thumbnail))).replace("__VIEWPORT__",JSON.stringify(pelicanViewport(html)))
 const csp="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob:; font-src data:; media-src data: blob:; connect-src 'none'; frame-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'"
 const previewStyle=options.thumbnail?"<style>html,body{overflow:hidden!important;scrollbar-width:none!important}*{scrollbar-width:none!important}*::-webkit-scrollbar{display:none!important}</style>":""
 return `<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="${csp}"><style>html,body{margin:0;min-height:100%}svg{max-width:100%}</style><script>${bridge}</script></head><body>${html.replace(/<!doctype[^>]*>/ig,"")}${previewStyle}</body></html>`
}
