import type { PelicanConfig } from "./api-types"

export const defaultPelicanPrompt = "创建一个 HTML，内容是 SVG 绘制一个鹈鹕骑自行车的 2D 动画，你不需要任何测试。"
export const pelicanSchedules = [
 {value:"*/15 * * * *",label:"每 15 分钟"}, {value:"*/30 * * * *",label:"每 30 分钟"},
 {value:"0 * * * *",label:"每小时"}, {value:"0 */2 * * *",label:"每 2 小时"},
 {value:"0 */6 * * *",label:"每 6 小时"}, {value:"0 9 * * *",label:"每天 9:00"},
]
export const pelicanEfforts: Record<string,string> = {none:"无",minimal:"最低",low:"低",medium:"中",high:"高",xhigh:"极高",max:"最高",ultra:"超高"}
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
 const nativeRAF=window.requestAnimationFrame.bind(window), nativeNow=performance.now.bind(performance);
 const epoch=Date.now(), frames=new Map(), timers=new Map();
 let id=1, virtual=0, previous=nativeNow(), playing=true, speed=1;
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
  synchronize();
 });
 function tick(real){
  const delta=Math.min(100,Math.max(0,real-previous));previous=real;
  if(playing){
   virtual+=delta*speed;
   const batch=Array.from(frames);frames.clear();for(const [key,callback]of batch){try{callback(virtual)}catch(error){console.error(error)}}
   for(const [key,timer]of timers){if(timer.due>virtual)continue;if(timer.repeat)timer.due=virtual+timer.repeat;else timers.delete(key);try{if(typeof timer.callback==='function')timer.callback(...timer.args)}catch(error){console.error(error)}}
   for(const [svg,clock]of smil){if(!svg.isConnected){smil.delete(svg);continue}try{svg.setCurrentTime(clock.time+(virtual-clock.start)/1000)}catch{}}
  }
  nativeRAF(tick);
 }
 nativeRAF(tick);
 window.addEventListener('DOMContentLoaded',()=>{synchronize();parent.postMessage({type:'pelican-ready',channel},'*')});
})();`

export function createPelicanDocument(html: string, channel: string) {
 const bridge=playbackBridge.replace("__CHANNEL__",JSON.stringify(channel).replace(/</g,()=>String.fromCharCode(92)+"u003c"))
 const csp="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob:; font-src data:; media-src data: blob:; connect-src 'none'; frame-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'"
 return `<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="${csp}"><style>html,body{margin:0;min-height:100%}svg{max-width:100%}</style><script>${bridge}</script></head><body>${html.replace(/<!doctype[^>]*>/ig,"")}</body></html>`
}
