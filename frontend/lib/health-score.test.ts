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

test("\u56db\u9879\u6307\u6807\u5404\u5360\u56fa\u5b9a\u7684\u767e\u5206\u4e4b\u4e8c\u5341\u4e94", () => {
  const result = calculate([row(0), row(1)])
  assert.equal(result?.recentSuccess, 100)
  assert.equal(result?.recentLatency, 100)
  assert.equal(result?.stability, 100)
  assert.equal(result?.cacheHit, 50)
  assert.equal(result?.score, 88)
})

test("\u7f13\u5b58\u4ece\u96f6\u5230\u767e\u5206\u4e4b\u767e\u53ea\u5f71\u54cd\u4e8c\u5341\u4e94\u5206", () => {
  const miss = calculate([row(0, { cache_read_tokens: 0 }), row(1, { cache_read_tokens: 0 })])
  const hit = calculate([row(0, { input_tokens: 0 }), row(1, { input_tokens: 0 })])
  assert.equal(miss?.score, 75)
  assert.equal(hit?.score, 100)
})

test("\u7a33\u5b9a\u6027\u4e0d\u91cd\u590d\u5956\u52b1\u5feb\u901f\u54cd\u5e94", () => {
  const fast = calculate([row(0), row(1)])
  const slow = calculate([row(0, { first_token_ms: 30000, duration_ms: 180000 }), row(1, { first_token_ms: 30000, duration_ms: 180000 })])
  assert.equal(fast?.stability, 100)
  assert.equal(slow?.stability, 100)
  assert.equal(slow?.recentLatency, 0)
  assert.equal(fast!.score! - slow!.score!, 25)
})

test("\u5ef6\u8fdf\u6ce2\u52a8\u4f1a\u964d\u4f4e\u7a33\u5b9a\u6027", () => {
  const steady = calculate([row(0, { first_token_ms: 5000 }), row(1, { first_token_ms: 5000 }), row(2, { first_token_ms: 5000 })])
  const jitter = calculate([row(0, { first_token_ms: 1000 }), row(1, { first_token_ms: 9000 }), row(2, { first_token_ms: 1000 })])
  assert.equal(steady?.recentLatency, jitter?.recentLatency)
  assert.ok(jitter!.stability! < steady!.stability!)
})

test("\u540c\u6210\u529f\u7387\u4e0b\u8fde\u7eed\u5931\u8d25\u6bd4\u5206\u6563\u5931\u8d25\u66f4\u4e0d\u7a33\u5b9a", () => {
  const clustered = calculate(Array.from({ length: 8 }, (_, i) => row(i, { success: i > 1 })))
  const isolated = calculate(Array.from({ length: 8 }, (_, i) => row(i, { success: i !== 1 && i !== 5 })))
  assert.equal(clustered?.recentSuccess, isolated?.recentSuccess)
  assert.ok(clustered!.stability! < isolated!.stability!)
})

test("\u8fdb\u884c\u4e2d\u8bf7\u6c42\u4fdd\u7559\u5728\u6210\u529f\u7387\u5206\u6bcd\u4f46\u4e0d\u6784\u6210\u8fde\u7eed\u5931\u8d25", () => {
  const result = calculate([
    row(0, { success: false }),
    row(1, { in_progress: true, attempt_status: "in_progress" }),
    row(2, { success: false }),
    row(3),
    row(4),
  ])
  assert.equal(result?.attemptCount, 5)
  assert.equal(result?.recentSuccess, 40)
  assert.equal(result?.stability, 100)
})

test("\u7edf\u4e00\u4f7f\u7528\u914d\u7f6e\u65f6\u95f4\u7a97\u548c\u6700\u8fd1\u516d\u5341\u6b21\u9650\u5236", () => {
  const old = row(0, { created_at: new Date(now - 2 * 3600000).toISOString() })
  assert.equal(calculate([old, row(0)], 1)?.sampleCount, 1)
  assert.equal(calculate([old, row(0)], 4)?.sampleCount, 2)
  const many = Array.from({ length: 80 }, (_, i) => row(i))
  assert.equal(calculate(many)?.sampleCount, 60)
  assert.equal(calculate([row(0, { created_at: new Date(now - 1000).toISOString() })]), null)
})

test("\u5931\u8d25\u7684\u5feb\u901f\u54cd\u5e94\u4e0d\u80fd\u63d0\u9ad8\u5ef6\u8fdf\u5206", () => {
  const result = calculate([row(0, { first_token_ms: 30000, duration_ms: 180000 }), row(1, { success: false, first_token_ms: 1, duration_ms: 1 })])
  assert.equal(result?.recentLatency, 0)
})

test("\u7f3a\u9879\u6216\u5355\u6b21\u6837\u672c\u4e0d\u653e\u5927\u5269\u4f59\u6743\u91cd", () => {
  assert.equal(calculate([row(0)])?.score, null)
  assert.equal(calculate([row(0, { input_tokens: 0, cache_read_tokens: 0 }), row(1, { input_tokens: 0, cache_read_tokens: 0 })])?.score, null)
  assert.equal(calculate([row(0, { success_known: false }), row(1, { success_known: false })])?.score, null)
})

test("\u5c11\u4e8e\u4e24\u6b21\u6210\u529f\u5ef6\u8fdf\u4e0d\u4f2a\u9020\u6ce2\u52a8\u5206", () => {
  const result = calculate([row(0), row(1, { success: false })])
  assert.equal(result?.stability, null)
  assert.equal(result?.score, null)
})

test("\u5168\u90e8\u5931\u8d25\u4e14\u8fde\u7eed\u53d1\u751f\u65f6\u7a33\u5b9a\u6027\u4e3a\u96f6", () => {
  const result = calculate([row(0, { success: false }), row(1, { success: false })])
  assert.equal(result?.recentSuccess, 0)
  assert.equal(result?.stability, 0)
  assert.equal(result?.recentLatency, null)
  assert.equal(result?.score, null)
})

test("\u6210\u529f\u7387\u6309\u771f\u5b9e\u5c1d\u8bd5\u8ba1\u6570\u4e14\u5ffd\u7565\u63a2\u6d4b\u8bf7\u6c42", () => {
  const result = calculate([
    row(0), row(1), row(2, { success: false }),
    row(3, { success: false, request_type: "probe" }),
    row(4, { success: false, request_type: "cyber" }),
    row(5, { success: false, request_type: "auxiliary" }),
  ])
  assert.equal(result?.attemptCount, 3)
  assert.ok(Math.abs(result!.recentSuccess! - 200 / 3) < 1e-10)
})

test("\u8bc4\u5206\u8bf4\u660e\u5c55\u793a\u56db\u9879\u7b49\u6743\u548c\u7f6e\u4fe1\u5ea6", () => {
  const title = healthScoreDescription(calculate([row(0), row(1)]))
  assert.match(title, /4h/)
  assert.match(title, /25%/)
  assert.match(title, /\u7f13\u5b58\u547d\u4e2d 50\.0/)
  assert.match(title, /\u4f4e\u7f6e\u4fe1\u5ea6/)
  assert.match(healthScoreDescription(calculate([row(0)])), /\u6570\u636e\u4e0d\u8db3/)
})

test("\u7f13\u5b58\u5206\u4f7f\u7528 Token \u52a0\u6743\u800c\u4e0d\u662f\u547d\u4e2d\u6b21\u6570", () => {
  const result = calculate([row(0, { input_tokens: 900, cache_read_tokens: 0 }), row(1, { input_tokens: 0, cache_read_tokens: 100 })])
  assert.equal(result?.cacheHit, 10)
  assert.equal(result?.score, 78)
})

test("\u76f8\u540c\u7684\u76f8\u5bf9\u5ef6\u8fdf\u6ce2\u52a8\u4e0d\u53d7\u7edd\u5bf9\u901f\u5ea6\u5f71\u54cd", () => {
  const fast = calculate([row(0, { first_token_ms: 1000 }), row(1, { first_token_ms: 3000 })])
  const slow = calculate([row(0, { first_token_ms: 10000 }), row(1, { first_token_ms: 30000 })])
  assert.equal(fast?.stability, slow?.stability)
  assert.notEqual(fast?.recentLatency, slow?.recentLatency)
})

test("\u7a7a\u6837\u672c\u4e0d\u751f\u6210\u8bc4\u5206", () => {
  assert.equal(calculate([]), null)
})
