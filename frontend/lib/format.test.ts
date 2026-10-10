import assert from "node:assert/strict"
import test from "node:test"

// @ts-expect-error Node 22 原生 TypeScript 测试需要显式 .ts 扩展名。
import { normalizeTokenCount, tokensInYi } from "./format.ts"

test("formats token totals in hundred-millions", () => {
  assert.equal(tokensInYi(123_456_789), "1.23 亿")
  assert.equal(tokensInYi(12_000_000), "0.12 亿")
  assert.equal(tokensInYi(0), "0.00 亿")
})

test("returns a placeholder for missing token totals", () => {
  assert.equal(tokensInYi(null), "—")
  assert.equal(tokensInYi(Number.NaN), "—")
})

test("normalizes missing token totals from older API responses", () => {
  assert.equal(normalizeTokenCount(undefined), 0)
  assert.equal(normalizeTokenCount(null), 0)
  assert.equal(normalizeTokenCount(Number.NaN), 0)
  assert.equal(normalizeTokenCount(123_456_789), 123_456_789)
})