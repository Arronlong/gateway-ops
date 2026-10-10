import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"

const source = readFileSync(new URL("./channel-cards.tsx", import.meta.url), "utf8")

test("\u6e20\u9053\u641c\u7d22\u4f7f\u7528\u53ef\u4e2d\u65ad\u66f4\u65b0\u4e14\u4e0d\u8986\u76d6\u8f93\u5165\u8349\u7a3f", () => {
  assert.match(source, /startTransition\(\(\) => onValueChange\(draft\)\)/)
  assert.doesNotMatch(source, /setDraft\(value\)/)
  assert.match(source, /channelSearchResetVersion/)
  assert.match(source, /if \(lastResetVersion\.current === resetVersion\) return/)
})
