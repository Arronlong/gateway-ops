import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22
import { relayTestReportSVG } from "./relay-test-report.ts"
const run = {id:1,relay_station_id:2,station_name:"station <script>alert(1)</script>",target_type:"group",target_id:3,target_name:"group & test",model:"gpt-5",mode:"regular",requested:2,succeeded:1,failed:1,duration_ms:1000,error:"",started_at:"2026-10-14T08:00:00Z",finished_at:"2026-10-14T08:00:01Z",results:[{index:1,success:true,status_code:200,duration_ms:400,output:"ok"},{index:2,success:false,status_code:503,duration_ms:600,output:"error"}]}
test("报告保持矢量形式，保留每次调用状态和动画", () => {
 const svg=relayTestReportSVG(run)
 assert.match(svg, /<svg/)
 assert.match(svg, /200/)
 assert.match(svg, /503/)
 assert.match(svg, /400 ms/)
 assert.match(svg, /600 ms/)
 assert.match(svg, /@keyframes/)
 assert.match(svg, /prefers-reduced-motion/)
 assert.doesNotMatch(svg, /<image/)
})
test("上游名称作为文本转义，避免 SVG 脚本注入", () => {
 const svg=relayTestReportSVG(run)
 assert.doesNotMatch(svg, /<script>/)
 assert.match(svg, /&lt;script&gt;/)
 assert.match(svg, /group &amp; test/)
})
