import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"
// @ts-expect-error Node 22 native TypeScript tests require the explicit .ts extension.
import { channelSectionVisible, defaultSystemSettings } from "./system-settings.ts"

const source = (file: string) => readFileSync(new URL(file, import.meta.url), "utf8")

test("\u76d1\u63a7\u6307\u6807\u5361\u7247\u5185\u63d0\u4f9b\u53ef\u4fdd\u5b58\u7684\u6309\u94ae", () => {
  const page = source("../app/system-settings-page.tsx")
  const card = page.match(/<Card id="monitor-metrics"[\s\S]*?<\/Card>/)?.[0]
  assert.ok(card)
  assert.match(card, /<Button[\s\S]*?disabled=\{saving \|\| query\.loading\}[\s\S]*?onClick=\{\(\) => void save\(\)\}[\s\S]*?\u4fdd\u5b58\u76d1\u63a7\u8bbe\u7f6e[\s\S]*?<\/Button>/)
})

test("\u540e\u53f0\u4efb\u52a1\u4fdd\u5b58\u6309\u94ae\u4f7f\u7528\u4efb\u52a1\u8bbe\u7f6e\u6587\u6848", () => {
  const page = source("../app/system-settings-page.tsx")
  const card = page.match(/<Card id="background-tasks"[\s\S]*?<\/Card>/)?.[0]
  assert.ok(card)
  assert.match(card, /onClick=\{\(\) => void saveSyncSettings\(\)\}[\s\S]*?\u4fdd\u5b58\u4efb\u52a1\u8bbe\u7f6e[\s\S]*?<\/Button>/)
  assert.doesNotMatch(card, /\u4fdd\u5b58\u540c\u6b65\u8bbe\u7f6e/)
})

test("\u672a\u8bfb\u5230\u914d\u7f6e\u65f6\u6240\u6709\u6e20\u9053\u5b50\u533a\u57df\u5747\u4e0d\u663e\u793a", () => {
  for (const section of ["recent", "stability", "consumption_trend", "groups", "rate_changes"] as const) {
    assert.equal(channelSectionVisible(null, section), false)
    assert.equal(channelSectionVisible({ ...defaultSystemSettings, channels_page_enabled: false }, section), false)
  }
})

test("\u5b50\u533a\u57df\u540c\u65f6\u9075\u5faa\u9875\u9762\u3001\u7236\u533a\u57df\u548c\u81ea\u8eab\u5f00\u5173", () => {
  const fields = { recent: "channels_show_recent_latency", stability: "channels_show_stability", consumption_trend: "channels_show_consumption", groups: "channels_show_groups", rate_changes: "channels_show_rate_changes" } as const
  for (const [section, field] of Object.entries(fields)) {
    assert.equal(channelSectionVisible(defaultSystemSettings, section as keyof typeof fields), true)
    assert.equal(channelSectionVisible({ ...defaultSystemSettings, [field]: false }, section as keyof typeof fields), false)
    if (section !== "rate_changes") {
      assert.equal(channelSectionVisible({ ...defaultSystemSettings, channels_show_channel_info: false }, section as keyof typeof fields), false)
    }
  }
})

test("\u914d\u7f6e\u672a\u5c31\u7eea\u65f6\u53ea\u9690\u85cf\u53d7\u63a7\u5bfc\u822a\u5165\u53e3\uff0c\u4e0d\u963b\u6b62\u5176\u4ed6\u4e1a\u52a1\u9875", () => {
  const header = source("../components/monitor/monitor-header.tsx")
  const queries = source("queries.ts")
  const main = source("../src/main.tsx")
  const shell = source("../components/app-shell.tsx")
  assert.doesNotMatch(header, /defaultSystemSettings/)
  assert.equal(/useContext\(SystemSettingsContext\)/.test(queries), true)
  assert.equal(/useContext\(SyncSettingsContext\)/.test(queries), true)
  assert.equal(/<SettingsProvider>/.test(main), true)
  assert.equal(/systemSettings\.data\?\.channels_page_enabled\s*===\s*true/.test(header), true)
  assert.equal(/systemSettings\.data\?\.operations_management_enabled\s*===\s*true/.test(header), true)
  assert.equal(/<Outlet\s*\/>/.test(shell), true)
  assert.equal(/systemSettings\.data\s*\?\s*<Outlet/.test(shell), false)
})

test("\u8bbe\u7f6e\u8868\u5355\u4ec5\u5728\u5b9e\u9645\u914d\u7f6e\u5c31\u7eea\u540e\u5c55\u793a\u5e76\u4ee5\u5176\u4e3a\u521d\u503c", () => {
  const page = source("../app/system-settings-page.tsx")
  assert.equal(/defaultSystemSettings/.test(page), false)
  assert.equal(/const\s+settings\s*=\s*settingsDraft\s*\?\?\s*query\.data/.test(page), true)
  assert.equal(/if\s*\(!query\.data\s*\|\|\s*!settings\)[\s\S]*?ConfigurationLoading/.test(page), true)
  assert.equal(/!syncQuery\.data\s*\|\|\s*!syncSettings\s*\?[\s\S]*?ConfigurationLoading/.test(page), true)
})

test("\u6e20\u9053\u4e24\u79cd\u89c6\u56fe\u5747\u9075\u5faa\u5b9e\u9645\u914d\u7f6e\uff0c\u4fdd\u7559\u672c\u5730\u5217\u504f\u597d", () => {
  const cards = source("../components/monitor/channel-cards.tsx")
  assert.equal(/uiSettings\s*=\s*defaultSystemSettings|uiSettings\?:/.test(cards), false)
  assert.equal(/new Set\(\[\.\.\.preferredColumns\]\.filter/.test(cards), true)
  assert.equal(/columnVisibilitySections\[column\][\s\S]*channelSectionVisible\(uiSettings, section\)/.test(cards), true)
  assert.equal(/channelSectionVisible\(uiSettings, "rate_changes"\)\s*&&\s*c\.latest_ratio_changed_at/.test(cards), true)

  const kpi = source("../components/monitor/kpi-row.tsx")
  const changes = source("../components/monitor/multiplier-changes.tsx")
  assert.equal(/channels_show_rate_changes/.test(kpi), true)
  assert.equal(/channels_show_rate_changes/.test(changes), true)
})

test("\u914d\u7f6e\u5c31\u7eea\u524d\u540e\u4e0d\u6539\u53d8\u500d\u7387\u9762\u677f\u7684 Hook \u8c03\u7528\u987a\u5e8f", () => {
  const changes = source("../components/monitor/multiplier-changes.tsx")
  const visibilityGuard = changes.indexOf("if (systemSettings.data?.channels_show_rate_changes !== true) return null")
  assert.ok(visibilityGuard > changes.indexOf("const channelMap = useMemo("))
})
