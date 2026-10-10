import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22 native TypeScript tests require the explicit .ts extension.
import { pickRelayCompareBucket } from "./relay-compare-bucket.ts"

function bucket(start: string, end: string, requestCount: number) {
  return { start, end, request_count: requestCount }
}

test("exact start wins over neighbouring buckets", () => {
  const buckets = [
    bucket("2026-09-22T05:24:54Z", "2026-09-22T17:14:56Z", 1),
    bucket("2026-09-22T17:14:56Z", "2026-09-23T05:04:00Z", 4),
  ]
  assert.equal(pickRelayCompareBucket(buckets, "2026-09-22T17:14:56Z")?.request_count, 4)
})

test("minute level drift between channel and relay windows still resolves the same slot", () => {
  const relayBuckets = [
    bucket("2026-09-22T05:24:54Z", "2026-09-22T17:13:57Z", 2),
    bucket("2026-09-22T17:13:57Z", "2026-09-23T05:03:00Z", 7),
  ]
  assert.equal(pickRelayCompareBucket(relayBuckets, "2026-09-22T17:14:56Z")?.request_count, 7)
})

test("missing start falls back to the newest bucket with requests", () => {
  const buckets = [
    bucket("2026-09-17T07:14:24Z", "2026-09-17T19:03:27Z", 61),
    bucket("2026-09-17T19:03:27Z", "2026-09-18T06:52:30Z", 0),
  ]
  assert.equal(pickRelayCompareBucket(buckets)?.request_count, 61)
})

test("empty or fully idle series returns the last bucket or null", () => {
  assert.equal(pickRelayCompareBucket([]), null)
  assert.equal(pickRelayCompareBucket(undefined), null)
  const idle = [bucket("2026-09-22T05:24:54Z", "2026-09-22T17:13:57Z", 0)]
  assert.equal(pickRelayCompareBucket(idle)?.request_count, 0)
})
