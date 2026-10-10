import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22 native TypeScript tests require the explicit .ts extension.
import { channelMatchesSearch } from "./channel-search.ts"
import type { Channel } from "./api-types"

const channel = {
  name: "Primary Channel",
  remark: "production upstream",
  site_url: "https://api.example.com",
  model_names: ["claude-3-7-sonnet", "gpt-5.6-sol"],
} as Channel

test("channel search matches model names case-insensitively", () => {
  assert.equal(channelMatchesSearch(channel, "CLAUDE-3-7"), true)
  assert.equal(channelMatchesSearch(channel, "gpt-5.6-sol"), true)
})

test("channel search keeps existing channel fields", () => {
  assert.equal(channelMatchesSearch(channel, "primary"), true)
  assert.equal(channelMatchesSearch(channel, "production"), true)
  assert.equal(channelMatchesSearch(channel, "api.example.com"), true)
  assert.equal(channelMatchesSearch(channel, "gemini"), false)
})

test("channel search matches models supported by an associated relay account", () => {
  const channelWithoutUsage = { ...channel, model_names: [] }
  assert.equal(channelMatchesSearch(channelWithoutUsage, "gpt-5.6-sol", ["gpt-5.6-sol"]), true)
})
