import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22
import { healthScoreDescription, latencyHealthScore } from "./latency.ts"

const now = Date.parse("2026-10-22T08:00:00Z")
const row = (index: number, extra = {}) => ({
  channel_account_id: 1, created_at: new Date(now - (index + 2) * 60000).toISOString(),
  success_known: true, success: true, first_token_ms: 1000, duration_ms: 2000,
  input_tokens: 100, cache_read_tokens: 100, ...extra,
})
const calculate = (rows: ReturnType<typeof row>[], windowHours = 4) => latencyHealthScore(rows, { now, windowHours })

test("\u5065\u5eb7\u8bc4\u5206\u548c\u6392\u5e8f\u8ddf\u968f\u914d\u7f6e\u7684\u6700\u8fd1\u6570\u91cf", () => {
  const rows = Array.from({length: 100}, (_, i) => row(i, {cache_read_tokens: i < 20 ? 100 : 0}))
  const limited = latencyHealthScore(rows, {now, windowHours: 4, sampleLimit: 20})
  const full = latencyHealthScore(rows, {now, windowHours: 4, sampleLimit: 100})
  assert.equal(limited?.sampleCount, 20)
  assert.equal(limited?.cacheHit, 50)
  assert.equal(full?.sampleCount, 100)
  assert.ok(limited!.score! > full!.score!)
})

test("\u4e09\u9879\u6307\u6807\u7b49\u6743\u8ba1\u7b97\u4e0d\u5305\u542b\u6210\u529f\u7387", () => {
  const result = calculate([row(0), row(1)])
  assert.equal(result?.recentLatency, 100)
  assert.equal(result?.stability, 100)
  assert.equal(result?.cacheHit, 50)
  assert.equal(result?.score, 83)
})

test("Sub2API \u6ca1\u6709\u6210\u529f\u72b6\u6001\u4f46\u6709\u5b9e\u6d4b\u6570\u503c\u65f6\u53ef\u4ee5\u8bc4\u5206", () => {
  const result = calculate([row(0, {success_known:false}), row(1, {success_known:false})])
  assert.equal(result?.score, 83)
  assert.deepEqual(result?.missingMetrics, [])
  assert.equal(result?.recentLatency, 100)
  assert.equal(result?.stability, 100)
})

test("\u6210\u529f\u6807\u5fd7\u3001HTTP \u72b6\u6001\u548c\u8fde\u7eed\u5931\u8d25\u4e0d\u6539\u53d8\u76f8\u540c\u5b9e\u6d4b\u6570\u503c\u7684\u8bc4\u5206", () => {
  const baseline=calculate(Array.from({length:8},(_,i)=>row(i)))
  for(const status of [
    {success_known:false,success:false},
    {success_known:true,success:false},
    {success_known:false,status_code:500},
    {success_known:true,success:false,attempt_status:"failed"},
  ]) {
    const result=calculate(Array.from({length:8},(_,i)=>row(i,status)))
    assert.equal(result?.score,baseline?.score)
    assert.equal(result?.stability,baseline?.stability)
    assert.equal(result?.recentLatency,baseline?.recentLatency)
  }
  const clustered=calculate(Array.from({length:8},(_,i)=>row(i,{success:i>1})))
  const isolated=calculate(Array.from({length:8},(_,i)=>row(i,{success:i!==1&&i!==5})))
  assert.equal(clustered?.score,isolated?.score)
})

test("\u7f13\u5b58\u4ece\u96f6\u5230\u767e\u5206\u4e4b\u767e\u5360\u4e09\u5206\u4e4b\u4e00\u6743\u91cd", () => {
  const miss=calculate([row(0,{cache_read_tokens:0}),row(1,{cache_read_tokens:0})])
  const hit=calculate([row(0,{input_tokens:0}),row(1,{input_tokens:0})])
  assert.equal(miss?.score,67)
  assert.equal(hit?.score,100)
})

test("\u7a33\u5b9a\u6027\u4e0d\u91cd\u590d\u5956\u52b1\u5feb\u901f\u54cd\u5e94", () => {
  const fast=calculate([row(0),row(1)])
  const slow=calculate([row(0,{first_token_ms:30000,duration_ms:180000}),row(1,{first_token_ms:30000,duration_ms:180000})])
  assert.equal(fast?.stability,100)
  assert.equal(slow?.stability,100)
  assert.equal(slow?.recentLatency,0)
  assert.equal(slow?.score,50)
})

test("\u5ef6\u8fdf\u6ce2\u52a8\u4f1a\u964d\u4f4e\u7a33\u5b9a\u6027", () => {
  const steady=calculate([row(0,{first_token_ms:5000}),row(1,{first_token_ms:5000}),row(2,{first_token_ms:5000})])
  const jitter=calculate([row(0,{first_token_ms:1000}),row(1,{first_token_ms:9000}),row(2,{first_token_ms:1000})])
  assert.equal(steady?.recentLatency,jitter?.recentLatency)
  assert.ok(jitter!.stability!<steady!.stability!)
})

test("\u7edf\u4e00\u4f7f\u7528\u914d\u7f6e\u65f6\u95f4\u7a97\u548c\u6700\u8fd1\u6570\u91cf\u9650\u5236", () => {
  const old=row(0,{created_at:new Date(now-2*3600000).toISOString()})
  assert.equal(calculate([old,row(0)],1)?.sampleCount,1)
  assert.equal(calculate([old,row(0)],4)?.sampleCount,2)
  assert.equal(calculate(Array.from({length:80},(_,i)=>row(i)))?.sampleCount,60)
  assert.equal(calculate([row(0,{created_at:new Date(now-1000).toISOString()})]),null)
})

test("\u7f3a\u5c11\u771f\u5b9e\u6307\u6807\u6216\u5355\u6b21\u6837\u672c\u4e0d\u653e\u5927\u5269\u4f59\u6743\u91cd", () => {
  assert.equal(calculate([row(0)])?.score,null)
  const noCache=calculate([row(0,{input_tokens:0,cache_read_tokens:0}),row(1,{input_tokens:0,cache_read_tokens:0})])
  assert.equal(noCache?.score,null)
  assert.deepEqual(noCache?.missingMetrics,["\u7f13\u5b58\u547d\u4e2d"])
  const noLatency=calculate([row(0,{first_token_ms:null,duration_ms:null}),row(1,{first_token_ms:null,duration_ms:null})])
  assert.equal(noLatency?.score,null)
  assert.deepEqual(noLatency?.missingMetrics,["\u7a33\u5b9a\u6027","\u5ef6\u8fdf"])
})

test("\u8fdb\u884c\u4e2d\u548c\u8f85\u52a9\u63a2\u6d4b\u4e0d\u6df7\u5165\u5b9e\u6d4b\u8bc4\u5206", () => {
  const baseline=calculate([row(0),row(1)])
  const result=calculate([
    row(0),row(1),
    row(2,{in_progress:true,attempt_status:"in_progress",first_token_ms:30000,duration_ms:180000}),
    row(3,{request_type:"probe",first_token_ms:30000,duration_ms:180000,input_tokens:10000,cache_read_tokens:0}),
    row(4,{is_auxiliary:true,first_token_ms:30000,duration_ms:180000}),
    row(5,{request_type:"cyber",first_token_ms:30000,duration_ms:180000}),
  ])
  assert.equal(result?.score,baseline?.score)
  assert.equal(result?.measuredCount,2)
})

test("\u8bc4\u5206\u8bf4\u660e\u5c55\u793a\u4e09\u9879\u7b49\u6743\u548c\u7f6e\u4fe1\u5ea6", () => {
  const title=healthScoreDescription(calculate([row(0),row(1)]))
  assert.match(title,/4h/)
  assert.match(title,/1\/3/)
  assert.match(title,/\u7f13\u5b58\u547d\u4e2d 50\.0/)
  assert.match(title,/\u4f4e\u7f6e\u4fe1\u5ea6/)
  assert.doesNotMatch(title,/\u6210\u529f\u7387|\u8fde\u8d25|25%/)
  assert.match(healthScoreDescription(calculate([row(0)])),/\u6570\u636e\u4e0d\u8db3/)
})

test("\u7f13\u5b58\u5206\u4f7f\u7528 Token \u52a0\u6743\u800c\u4e0d\u662f\u547d\u4e2d\u6b21\u6570", () => {
  const result=calculate([row(0,{input_tokens:900,cache_read_tokens:0}),row(1,{input_tokens:0,cache_read_tokens:100})])
  assert.equal(result?.cacheHit,10)
  assert.equal(result?.score,70)
})

test("\u76f8\u540c\u7684\u76f8\u5bf9\u5ef6\u8fdf\u6ce2\u52a8\u4e0d\u53d7\u7edd\u5bf9\u901f\u5ea6\u5f71\u54cd", () => {
  const fast=calculate([row(0,{first_token_ms:1000}),row(1,{first_token_ms:3000})])
  const slow=calculate([row(0,{first_token_ms:10000}),row(1,{first_token_ms:30000})])
  assert.equal(fast?.stability,slow?.stability)
  assert.notEqual(fast?.recentLatency,slow?.recentLatency)
})

test("\u7a7a\u6837\u672c\u4e0d\u751f\u6210\u8bc4\u5206", () => {
  assert.equal(calculate([]),null)
})
