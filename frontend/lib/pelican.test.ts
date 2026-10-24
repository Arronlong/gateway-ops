import assert from "node:assert/strict"
import test from "node:test"
import vm from "node:vm"
// @ts-expect-error Node 22
import { defaultPelicanConfig, pelicanViewport, fitPelicanArtwork, createPelicanDocument, pelicanRunOptions } from "./pelican.ts"
test("账号测智只保留默认关闭的开关",()=>{
 const config=defaultPelicanConfig()
 assert.deepEqual(config,{enabled:false})
})
test("手动测智选项展开已配置模型并保留对应方案参数",()=>{
 assert.deepEqual(pelicanRunOptions({pelican_plans:[
  {model:"model-a, model-b",prompt:"first prompt",reasoning_effort:"high"},
  {model:"model-b\nmodel-c",prompt:"second prompt",reasoning_effort:"low"},
 ]}),[
  {model:"model-a",prompt:"first prompt",reasoning_effort:"high"},
  {model:"model-b",prompt:"first prompt",reasoning_effort:"high"},
  {model:"model-c",prompt:"second prompt",reasoning_effort:"low"},
 ])
})
test("缩略图和详情使用同一作品视口",()=>{
 assert.deepEqual(pelicanViewport('<svg viewBox="0 0 960 600"></svg>'),{width:960,height:600})
 assert.deepEqual(pelicanViewport('<html>unknown</html>'),{width:1024,height:768})
 assert.deepEqual(fitPelicanArtwork({width:960,height:600},{width:240,height:200}),{width:240,height:150,scale:.25})
})
test("作品文档先安装网络限制和动画控制",()=>{
 const document=createPelicanDocument('<html><body><svg></svg><script>window.modelAnimation=true</script></body></html>',"channel")
 assert.ok(document.indexOf('Content-Security-Policy')<document.indexOf('window.modelAnimation'))
 assert.match(document,/connect-src 'none'/);assert.match(document,/form-action 'none'/)
 assert.match(document,/getAnimations/);assert.match(document,/setCurrentTime/);assert.match(document,/requestAnimationFrame/)
 assert.match(document,/pelican-control/)
})
test("缩略图文档禁止内部滚动而详情保留原始样式",()=>{
 const html='<html><head><style>body{overflow:scroll}svg{width:1400px;height:1000px}</style></head><body><svg></svg></body></html>'
 const thumbnail=createPelicanDocument(html,"thumbnail",{thumbnail:true})
 const detail=createPelicanDocument(html,"detail")
 assert.match(thumbnail,/overflow:hidden!important/)
 assert.match(thumbnail,/scrollbar-width:none!important/)
 assert.ok(thumbnail.lastIndexOf('overflow:hidden!important')>thumbnail.indexOf('overflow:scroll'))
 assert.doesNotMatch(detail,/scrollbar-width:none!important/)
})

test("暂停取消浏览器逐帧回调，恢复后仅保留一个播放循环",()=>{
 const callbacks=new Map<number,(time:number)=>void>(),listeners=new Map<string,(event:unknown)=>void>()
 let id=0
 const parent={postMessage:()=>{}},document={getAnimations:()=>[],querySelectorAll:()=>[]}
 const window={
  requestAnimationFrame:(callback:(time:number)=>void)=>{callbacks.set(++id,callback);return id},
  cancelAnimationFrame:(key:number)=>{callbacks.delete(key)},setTimeout:()=>0,
  addEventListener:(name:string,listener:(event:unknown)=>void)=>listeners.set(name,listener),
 }
 const source=createPelicanDocument("<svg></svg>","paused").match(/<script>([\s\S]*?)<\/script>/)![1]
 vm.runInNewContext(source,{window,parent,document,performance:{now:()=>100},MutationObserver:class{observe(){}},console})
 assert.equal(callbacks.size,1)
 const control=(playing:boolean)=>listeners.get("message")!({source:parent,data:{type:"pelican-control",channel:"paused",playing,speed:1}})
 control(false);assert.equal(callbacks.size,0)
 control(true);control(true);assert.equal(callbacks.size,1)
 control(false);assert.equal(callbacks.size,0)
})
