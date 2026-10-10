import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22 native TypeScript tests require the explicit .ts extension.
import { latestRelayLatencySamples, relayAccountDisplayBucketCount, relayAccountSearchModelTypes, relayAccountSearchText } from "./relay-account-view.ts"

test("gallery uses 60 timeline buckets while list keeps 30", () => {
  assert.equal(relayAccountDisplayBucketCount("gallery"), 60)
  assert.equal(relayAccountDisplayBucketCount("list"), 30)
})

test("gallery recent latency keeps the latest 60 calls in chronological order", () => {
  const samples = Array.from({ length: 75 }, (_, index) => ({
    id: index + 1,
    created_at: new Date(Date.UTC(2026, 8, 22, 0, index)).toISOString(),
  })).reverse()

  const latest = latestRelayLatencySamples(samples, 60)

  assert.equal(latest.length, 60)
  assert.equal(latest[0]?.id, 16)
  assert.equal(latest.at(-1)?.id, 75)
})

test("account search includes concrete models from calls and the supported model catalog", () => {
  const models = relayAccountSearchModelTypes({
    model_type: "claude",
    latency_samples: [{ model: "gpt-6.1-sol" }],
    current_groups: [
      { model_types: ["deepseek", "gpt"] },
      { model_types: ["gpt", ""] },
    ],
  }, ["gpt-6-astra", "gpt-6.1-sol"])

  assert.deepEqual(models, ["claude", "gpt-6.1-sol", "gpt-6-astra"])
})

test("account search text flattens stable fields and concrete model names", () => {
  const text = relayAccountSearchText({
    external_id: 42355,
    name: "AIHub",
    base_url: "https://aihub.example/v1",
    platform: "openai",
    type: "apikey",
    model_type: "gpt",
    latency_samples: [{ model: "gpt-6.1-sol" }],
    current_groups: [{ model_types: ["deepseek"] }],
  }, ["gpt-6-astra"])

  assert.match(text, /aihub/)
  assert.match(text, /aihub\.example/)
  assert.match(text, /gpt-6\.1-sol/)
  assert.match(text, /gpt-6-astra/)
})

test("account search does not treat an assigned sales group as an actually supported model", () => {
  const text = relayAccountSearchText({
    external_id: 42355,
    name: "AIHub",
    base_url: "https://aihub.example/v1",
    platform: "openai",
    type: "apikey",
    latency_samples: [],
    current_groups: [{ model_types: ["deepseek"] }],
  }, ["gpt-6.1-sol"])

  assert.doesNotMatch(text, /deepseek/)
})
