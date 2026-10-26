import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"
// @ts-expect-error Node 22 native TypeScript tests require the explicit .ts extension.
import { matchRelatedRecentCall } from "./recent-call-comparison.ts"

function sample(created_at: string, model: string, input_tokens: number, output_tokens: number) {
  return { created_at, model, input_tokens, output_tokens }
}

test("matches the closest call with the same model and token shape", () => {
  const primary = [
    sample("2026-10-12T10:00:00Z", "gpt-6.1-sol", 100, 20),
    sample("2026-10-12T10:01:00Z", "gpt-6.1-sol", 200, 30),
  ]
  const counterpart = [
    sample("2026-10-12T09:59:59Z", "gpt-6.1-sol", 100, 20),
    sample("2026-10-12T10:01:02Z", "gpt-6.1-sol", 200, 30),
  ]

  assert.equal(matchRelatedRecentCall(primary[1], primary, counterpart), counterpart[1])
})
test("falls back to the same relative position for unmatched calls", () => {
  const primary = [
    sample("2026-10-12T10:00:00Z", "a", 1, 1),
    sample("2026-10-12T10:01:00Z", "b", 2, 2),
    sample("2026-10-12T10:02:00Z", "c", 3, 3),
  ]
  const counterpart = [
    sample("2026-10-11T10:00:00Z", "x", 10, 10),
    sample("2026-10-11T10:01:00Z", "y", 20, 20),
    sample("2026-10-11T10:02:00Z", "z", 30, 30),
    sample("2026-10-11T10:03:00Z", "w", 40, 40),
  ]

  assert.equal(matchRelatedRecentCall(primary[0], primary, counterpart), counterpart[0])
  assert.equal(matchRelatedRecentCall(primary[2], primary, counterpart), counterpart[3])
})
test("channel and relay pages provide counterpart recent-call series", () => {
  const channels = readFileSync(new URL("../components/monitor/channel-cards.tsx", import.meta.url), "utf8")
  const relays = readFileSync(new URL("../app/relay-stations-page.tsx", import.meta.url), "utf8")

  assert.match(channels, /useRelayLatencyTrends\(recentLatencyLimit, recentVisible\)/)
  assert.match(channels, /relayRecentSamplesByAccount=\{relayRecentSamplesByAccount\}/)
  assert.match(relays, /useChannelLatencyTrends\(metricsLimit, detailModule === "accounts" && overviewReady\)/)
  assert.match(relays, /channelRecentSamplesByChannel=\{channelRecentSamplesByChannel\}/)
  assert.doesNotMatch(relays, /latestRelayLatencySamples\(account\.latency_samples \?\? \[\], (30|60)\)/)
})
