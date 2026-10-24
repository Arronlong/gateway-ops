import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22 原生 TypeScript 测试需要显式扩展名。
import { getPelicanFilterOptions, matchesPelicanFilters } from "./pelican-library.ts"

const groups = [
  {results: [
    {model: "gpt-6.1-sol", reasoning_effort: "high", status: "success", relay_station_id: 1},
    {model: "gpt-6.1-sol", reasoning_effort: "medium", status: "failed", relay_station_id: 2},
    {model: "deepseek-v4-flash", reasoning_effort: "xhigh", status: "success", relay_station_id: 1},
  ]},
]

test("筛选选项去重并按模型名称和思考强度顺序排列", () => {
  assert.deepEqual(getPelicanFilterOptions(groups), {
    models: ["deepseek-v4-flash", "gpt-6.1-sol"],
    efforts: ["medium", "high", "xhigh"],
  })
})

test("模型和思考强度可组合筛选且全部选项取消限制", () => {
  const result = groups[0].results[0]
  assert.equal(matchesPelicanFilters(result, "gpt-6.1-sol", "high"), true)
  assert.equal(matchesPelicanFilters(result, "gpt-6.1-sol", "medium"), false)
  assert.equal(matchesPelicanFilters(result, "deepseek-v4-flash", "high"), false)
  assert.equal(matchesPelicanFilters(result, "all", "all"), true)
})
