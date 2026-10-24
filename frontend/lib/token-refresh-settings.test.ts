import assert from "node:assert/strict"
import test from "node:test"
import {readFileSync} from "node:fs"

test("\u6e20\u9053 Token \u5237\u65b0\u4f7f\u7528\u7edf\u4e00\u9891\u7387\u4e0b\u62c9\u6846\u5e76\u4fdd\u7559\u5df2\u5b58\u9891\u7387",()=>{
 const page=readFileSync(new URL("../app/system-settings-page.tsx",import.meta.url),"utf8")
 assert.match(page,/task="channel_token_refresh"/)
 assert.match(page,/channel_token_refresh_enabled: syncSettings.channel_token_refresh_enabled/)
 assert.match(page,/channel_token_refresh_interval_minutes: syncSettings.channel_token_refresh_interval_minutes/)
 assert.match(page,/value=\{tokenRefreshInterval\}\s+options=\{tokenRefreshIntervals\}/)
 assert.match(page,/formatOption=\{formatMinutes\}/)
 assert.ok(page.includes("[1, ...channelSyncIntervals, tokenRefreshInterval]"))
 assert.ok(!page.includes("customMinutes"))
})
