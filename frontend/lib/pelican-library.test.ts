import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22 原生 TypeScript 测试需要显式扩展名。
import { getPelicanFilterOptions, matchesPelicanFilters, normalizeChannelPelicanGallery } from "./pelican-library.ts"

const groups = [
  {results: [
    {model: "gpt-6.1-sol", reasoning_effort: "high", status: "success", relay_station_id: 1},
    {model: "gpt-6.1-sol", reasoning_effort: "medium", status: "failed", relay_station_id: 2},
    {model: "deepseek-v4-flash", reasoning_effort: "xhigh", status: "success", relay_station_id: 1},
  ]},
]

test("\u6e20\u9053\u6d4b\u667a\u7684\u7a7a\u7ed3\u679c\u54cd\u5e94\u5f52\u4e00\u5316\u540e\u53ef\u6b63\u5e38\u7b5b\u9009", () => {
  const response = JSON.parse(JSON.stringify({groups: [
    {channel: {id: 5}, targets: [], results: null},
    {channel: {id: 6}, targets: [], results: groups[0].results},
  ]}))
  const normalized = normalizeChannelPelicanGallery(response)
  assert.deepEqual(normalized.groups[0].results, [])
  assert.deepEqual(normalized.groups[1].results, groups[0].results)
  assert.deepEqual(normalized.groups[0].channel, response.groups[0].channel)
  assert.deepEqual(normalized.groups[0].targets, response.groups[0].targets)
  assert.equal(response.groups[0].results, null)
  assert.deepEqual(getPelicanFilterOptions(normalized.groups), getPelicanFilterOptions(groups))
})

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
