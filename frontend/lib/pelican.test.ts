import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22
import { defaultPelicanConfig, pelicanViewport, fitPelicanArtwork, createPelicanDocument } from "./pelican.ts"
test("账号测智默认关闭，并行为一份",()=>{
 const config=defaultPelicanConfig()
 assert.equal(config.enabled,false);assert.equal(config.parallel_count,1)
 assert.equal(config.reasoning_effort,"medium");assert.equal(config.cron_expression,"*/30 * * * *")
 assert.match(config.prompt,/SVG/)
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
