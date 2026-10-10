import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22 native TypeScript tests require the explicit .ts extension.
import { associationValue, channelComparisonsByRelayAccount, linkedStationsByChannel, relayAccountComparisonKey, relayComparisonsByChannel } from "./channel-relay-association.ts"

test("manual linked association uses the selected relay account", () => {
  assert.equal(
    associationValue({ mode: "manual", linked: true, relay_account_external_id: 42 }),
    "manual:42",
  )
})

test("missing or automatic association falls back to auto", () => {
  assert.equal(associationValue(undefined), "auto")
  assert.equal(
    associationValue({ mode: "auto", linked: false, relay_account_external_id: 0 }),
    "auto",
  )
})

test("manual unlinked association keeps the explicit unlinked choice", () => {
  assert.equal(
    associationValue({ mode: "manual", linked: false, relay_account_external_id: 0 }),
    "manual:none",
  )
})

test("channel station aggregation ignores unlinked associations", () => {
  const result = linkedStationsByChannel({
    links: [
      {
        channel_id: 10,
        channel_account_id: 100,
        channel_account_username: "channel@example.com",
        relay_station_id: 1,
        relay_station_name: "站点 A",
        relay_account_external_id: 42,
        mode: "manual",
        linked: true,
      },
      {
        channel_id: 10,
        channel_account_id: 100,
        channel_account_username: "channel@example.com",
        relay_station_id: 2,
        relay_station_name: "站点 B",
        relay_account_external_id: 0,
        mode: "manual",
        linked: false,
      },
    ],
  })

  assert.deepEqual([...result.get(10)!], [1])
})

test("relay comparison aggregation keeps only linked relay accounts", () => {
  const result = relayComparisonsByChannel({
    links: [
      {
        channel_id: 10,
        channel_account_id: 100,
        channel_account_username: "channel@example.com",
        relay_station_id: 2,
        relay_station_name: "站点 B",
        relay_account_external_id: 7,
        relay_account_name: "备用号",
        mode: "manual",
        linked: true,
      },
      {
        channel_id: 10,
        channel_account_id: 101,
        channel_account_username: "second@example.com",
        relay_station_id: 2,
        relay_station_name: "站点 B",
        relay_account_external_id: 7,
        relay_account_name: "备用号",
        mode: "auto",
        linked: true,
      },
      {
        channel_id: 10,
        channel_account_id: 100,
        channel_account_username: "channel@example.com",
        relay_station_id: 1,
        relay_station_name: "站点 A",
        relay_account_external_id: 0,
        mode: "manual",
        linked: false,
      },
    ],
  })

  assert.deepEqual(result.get(10), [
    { stationID: 2, stationName: "站点 B", accounts: [{ externalID: 7, name: "备用号" }] },
  ])
  assert.equal(result.get(11), undefined)
})

test("relay comparison aggregation falls back to external id when name missing", () => {
  const result = relayComparisonsByChannel({
    links: [
      {
        channel_id: 20,
        channel_account_id: 200,
        channel_account_username: "third@example.com",
        relay_station_id: 3,
        relay_station_name: "站点 C",
        relay_account_external_id: 91,
        mode: "manual",
        linked: true,
      },
    ],
  })

  assert.deepEqual(result.get(20), [
    { stationID: 3, stationName: "站点 C", accounts: [{ externalID: 91, name: "账号 91" }] },
  ])
})

test("channel comparison aggregation reverses linked associations by relay account", () => {
  const result = channelComparisonsByRelayAccount({
    links: [
      {
        channel_id: 10,
        channel_account_id: 100,
        channel_account_username: "primary@example.com",
        relay_station_id: 2,
        relay_station_name: "站点 B",
        relay_account_external_id: 7,
        relay_account_name: "备用号",
        mode: "manual",
        linked: true,
      },
      {
        channel_id: 10,
        channel_account_id: 101,
        channel_account_username: "secondary@example.com",
        relay_station_id: 2,
        relay_station_name: "站点 B",
        relay_account_external_id: 7,
        relay_account_name: "备用号",
        mode: "auto",
        linked: true,
      },
      {
        channel_id: 11,
        channel_account_id: 110,
        channel_account_username: "other@example.com",
        relay_station_id: 2,
        relay_station_name: "站点 B",
        relay_account_external_id: 7,
        relay_account_name: "备用号",
        mode: "auto",
        linked: true,
      },
      {
        channel_id: 12,
        channel_account_id: 120,
        channel_account_username: "ignored@example.com",
        relay_station_id: 2,
        relay_station_name: "站点 B",
        relay_account_external_id: 0,
        mode: "manual",
        linked: false,
      },
    ],
  }, [
    { id: 10, name: "主渠道" },
    { id: 11, name: "备用渠道" },
  ])

  assert.deepEqual(result.get(relayAccountComparisonKey(2, 7)), [
    { channelID: 10, channelName: "主渠道", accountNames: ["primary@example.com", "secondary@example.com"] },
    { channelID: 11, channelName: "备用渠道", accountNames: ["other@example.com"] },
  ])
})

test("channel comparison aggregation falls back to channel id and isolates stations", () => {
  const result = channelComparisonsByRelayAccount({
    links: [
      {
        channel_id: 20,
        channel_account_id: 200,
        channel_account_username: "channel@example.com",
        relay_station_id: 3,
        relay_station_name: "站点 C",
        relay_account_external_id: 91,
        mode: "manual",
        linked: true,
      },
      {
        channel_id: 20,
        channel_account_id: 200,
        channel_account_username: "channel@example.com",
        relay_station_id: 4,
        relay_station_name: "站点 D",
        relay_account_external_id: 91,
        mode: "manual",
        linked: true,
      },
    ],
  }, [])

  assert.deepEqual(result.get(relayAccountComparisonKey(3, 91)), [
    { channelID: 20, channelName: "渠道 20", accountNames: ["channel@example.com"] },
  ])
  assert.deepEqual(result.get(relayAccountComparisonKey(4, 91)), [
    { channelID: 20, channelName: "渠道 20", accountNames: ["channel@example.com"] },
  ])
})
