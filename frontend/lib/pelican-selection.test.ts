import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22 原生 TypeScript 测试需要显式扩展名。
import { deletePelicanResults, getPelicanSelection, pelicanSelectionRect } from "./pelican-selection.ts"

test("反向框选与正向框选使用相同矩形，触碰边界不算选中", () => {
  const rect = pelicanSelectionRect({x: 200, y: 180}, {x: 20, y: 30})
  assert.deepEqual(rect, {left: 20, top: 30, right: 200, bottom: 180})
  assert.deepEqual(rect, pelicanSelectionRect({x: 20, y: 30}, {x: 200, y: 180}))
  const entries = [
    {key: "channel:1", rect: {left: 0, top: 0, right: 50, bottom: 50}},
    {key: "account:2", rect: {left: 190, top: 170, right: 250, bottom: 250}},
    {key: "group:3", rect: {left: 200, top: 50, right: 300, bottom: 100}},
    {key: "hidden", rect: {left: 0, top: 40, right: 0, bottom: 80}},
  ]
  assert.deepEqual([...getPelicanSelection(entries, rect)], ["channel:1", "account:2"])
  assert.deepEqual([...getPelicanSelection(entries, rect, ["group:3"])], ["group:3", "channel:1", "account:2"])
  assert.deepEqual([...getPelicanSelection(entries, {left: 25, top: 25, right: 25, bottom: 60})], [])
})

test("跨渠道、账号和分组删除使用各自路径，同一作品去重且跳过生成中作品", async () => {
  const channel = {id: 7, channel_id: 2, relay_station_id: 1, account_external_id: 3, group_external_id: 0, status: "success" as const}
  const account = {...channel, channel_id: 0, relay_station_id: 4}
  const group = {...account, group_external_id: 6, status: "failed" as const}
  const paths: string[] = []
  const result = await deletePelicanResults([channel, account, group, channel, {...channel, id: 8, status: "running"}], async (path: string) => {paths.push(path)})
  assert.deepEqual(paths.sort(), [
    "/channels/2/pelican/results/7",
    "/relay-stations/4/accounts/3/pelican/results/7",
    "/relay-stations/4/groups/6/pelican/results/7",
  ])
  assert.deepEqual(result.deleted, [channel, account, group])
  assert.deepEqual(result.failed, [])
})

test("删除限制并发数量，单项失败不阻断其余作品并保留错误", async () => {
  const results = Array.from({length: 9}, (_, id) => ({id, channel_id: 1, relay_station_id: 1, account_external_id: 2, group_external_id: 0, status: "success" as const}))
  let active = 0, peak = 0
  const error = new Error("delete failed")
  const deleted = await deletePelicanResults(results, async (path: string) => {
    active++; peak = Math.max(peak, active)
    await new Promise(resolve => setTimeout(resolve, 1))
    active--
    if (path.endsWith("/3")) throw error
  })
  assert.equal(peak, 4)
  assert.deepEqual(deleted.deleted.map((result: {id: number}) => result.id), [0, 1, 2, 4, 5, 6, 7, 8])
  assert.deepEqual(deleted.failed, [{result: results[3], error}])
})
