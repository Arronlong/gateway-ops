import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22 原生 TypeScript 测试需要显式扩展名。
import { usageIntegrityLabel, usageIntegritySummary, usageIntegrityPause, usageIntegrityProgress, usageIntegrityWindows, type UsageIntegrity } from "./usage-integrity.ts"

test("usage integrity distinguishes unknown totals, matches and mismatches", () => {
  const result: UsageIntegrity = { status: "matched", local_count: 12, upstream_count: 12, since: "", until: "", checked_at: "", accounts: [] }
  assert.equal(usageIntegrityLabel(result, false), "\u6761\u6570\u4e00\u81f4\u00b712")
  assert.match(usageIntegrityLabel({ ...result, status: "mismatch", upstream_count: 15 }, false), /12 \/ .*15/)
  assert.equal(usageIntegrityLabel({ ...result, status: "unavailable", upstream_count: null }, false), "\u6682\u65e0\u6cd5\u6838\u5bf9")
  assert.equal(usageIntegrityLabel(null, true), "\u6761\u6570\u6838\u5bf9\u4e2d\u2026")
})

test("\u4fdd\u7559\u66f4\u65e9\u5386\u53f2\u4e0d\u8ba1\u4e3a\u5f02\u5e38\u5dee\u5f02",()=>{
  const result:UsageIntegrity & {retained_count:number}={status:"matched",phase:"complete",local_count:7852,upstream_count:2021,retained_count:5831,since:"",until:"",checked_at:"",accounts:[],audit:{checked_windows:1,skipped_windows:0,anomalous_days:0,repaired_days:0}}
  assert.match(usageIntegrityLabel(result,false),/2021/)
  assert.match(usageIntegritySummary(result),/5831.*\u66f4\u65e9\u5386\u53f2/)
  assert.match(usageIntegrityLabel({...result,status:"mismatch",local_count:7850},false),/\u7f3a\u5c11 2 \u6761/)
})

test("\u9875\u7ea7\u4fdd\u5b58\u8fdb\u5ea6\u548c\u65ad\u70b9\u7eed\u67e5\u63d0\u793a",()=>{
  const result:UsageIntegrity={phase:"paused",status:"mismatch",local_count:14,upstream_count:15,since:"",until:"",checked_at:"",accounts:[{account_id:1,account_name:"test",local_count:14,upstream_count:15,history_verified_until:"2026-10-20T00:00:00Z"}],audit:{checked_windows:1,skipped_windows:0,anomalous_days:0,repaired_days:0,saved_pages:3,saved_rows:14}}
  assert.match(usageIntegritySummary(result),/3 \u9875.*14 \u6761\u8c03\u7528/)
  assert.match(usageIntegritySummary(result),/\u4e0b\u6b21\u4ece\u8fde\u7eed\u5b8c\u6210\u4f4d\u7f6e\u7ee7\u7eed/)
})

test("\u9650\u6d41\u663e\u793a\u6062\u590d\u65f6\u95f4\u5e76\u4fdd\u7559\u5df2\u4fee\u590d\u8fdb\u5ea6",()=>{
  const result:UsageIntegrity={phase:"paused",retry_at:"2026-11-05T14:32:49Z",retry_status:429,status:"mismatch",local_count:13,upstream_count:15,since:"",until:"",checked_at:"",accounts:[],audit:{checked_windows:8,skipped_windows:3,anomalous_days:2,repaired_days:1}}
  const text=usageIntegrityPause(result,new Date("2026-11-05T14:32:19Z").getTime())
  assert.match(text,/429/)
  assert.match(text,/30 \u79d2/)
  assert.match(usageIntegrityLabel(result,false),/\u7f3a\u5c11 2 \u6761/)
  assert.match(usageIntegritySummary(result),/\u5df2\u4fee\u590d 1 \u5929/)
  assert.match(usageIntegrityProgress({message:"waiting",data:{kind:"usage_wait",retry_at:result.retry_at}}),/\u7b49\u5f85.*\u8bf7\u6c42\u989d\u5ea6/)
})

test("\u603b\u4f53\u7ed3\u679c\u663e\u793a\u5dee\u503c\u5e76\u4fdd\u7559\u5df2\u786e\u8ba4\u7684\u5dee\u5f02", () => {
  const result: UsageIntegrity & {phase:string} = { status:"mismatch", phase:"interrupted", local_count:12, upstream_count:15, since:"", until:"", checked_at:"", accounts:[{account_id:1,account_name:"test",local_count:12,upstream_count:15,error:"count conflict"}] }
  assert.match(usageIntegrityLabel(result,false),/\u7f3a\u5c11 3 \u6761/)
  assert.match(usageIntegrityLabel({...result,local_count:18},false),/\u591a 3 \u6761/)
  assert.match(usageIntegritySummary({...result,audit:{checked_windows:22,skipped_windows:18,anomalous_days:0,repaired_days:0}}),/\u5b9a\u4f4d\u672a\u5b8c\u6210/)
})

test("\u5206\u5c42\u6821\u9a8c\u5c55\u793a\u8df3\u8fc7\u533a\u95f4\u548c\u5f02\u5e38\u65e5\u671f", () => {
  const result: UsageIntegrity={status:"mismatch",local_count:12,upstream_count:15,since:"",until:"",checked_at:"",accounts:[],audit:{checked_windows:20,skipped_windows:14,anomalous_days:3,repaired_days:0}}
  assert.match(usageIntegrityLabel(result,false),/3 \u5929\u5f02\u5e38/)
  assert.equal(usageIntegritySummary(result),"\u5df2\u6838\u5bf9 20 \u4e2a\u533a\u95f4 \u00b7 \u53d1\u73b0\u603b\u4f53\u5dee\u5f02 \u00b7 \u5b9a\u4f4d\u5f02\u5e38 3 \u5929")
  assert.equal(usageIntegritySummary(null),"")
  assert.equal(usageIntegritySummary({...result,audit:undefined}),"")
  assert.equal(usageIntegritySummary({...result,status:"matched",upstream_count:12,audit:{checked_windows:1,skipped_windows:1,anomalous_days:0,repaired_days:0}}),"\u603b\u4f53\u6761\u6570\u4e00\u81f4，\u65e0\u9700\u5206\u6bb5\u6838\u5bf9")
})

test("\u76f8\u540c\u5468\u533a\u95f4\u53ea\u5c55\u793a\u4e00\u6b21", () => {
  const result: UsageIntegrity = { status:"mismatch", local_count:1388, upstream_count:1388, since:"", until:"", checked_at:"", accounts:[], audit:{ checked_windows:4, skipped_windows:0, anomalous_days:0, repaired_days:0, windows:[
    { account_id:1, account_name:"longxuan", level:"week", status:"mismatch", since:"2026-10-05T00:00:00Z", until:"2026-10-12T00:00:00Z", local_count:1388, upstream_count:1388 },
    { account_id:1, account_name:"longxuan", level:"week", status:"mismatch", since:"2026-10-05T00:00:00Z", until:"2026-10-12T00:00:00Z", local_count:1388, upstream_count:1388 },
  ]}}
  assert.equal(usageIntegrityWindows(result).length, 1)
})
