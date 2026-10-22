import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"

const source = (file: string) => readFileSync(new URL(file, import.meta.url), "utf8")

test("\u4f5c\u54c1\u7f29\u7565\u56fe\u548c\u8be6\u60c5\u90fd\u652f\u6301\u4e8c\u6b21\u786e\u8ba4\u5220\u9664", () => {
  const panel = source("../components/relay/pelican.tsx")
  assert.match(panel, /method:\s*"DELETE"/)
  assert.match(panel, /function PelicanDeleteButton/)
  assert.equal((panel.match(/<PelicanDeleteButton result=\{result\}/g) ?? []).length, 2)
  assert.match(panel, /await confirm\(/)
  assert.match(panel, /destructive:\s*true/)
  assert.match(panel, /requests\.current\.delete\(/)
  assert.match(panel, /setSelected\(/)
  assert.match(panel, /setRemovedIDs\(/)
})

test("\u7cfb\u7edf\u8bbe\u7f6e\u63d0\u4f9b\u5168\u5c40\u6d4b\u667a\u53c2\u6570\u548c\u4fdd\u5b58\u5165\u53e3", () => {
  const page = source("../app/system-settings-page.tsx")
  const card = source("../components/system/pelican-settings-card.tsx")
  assert.match(page, /<PelicanSettingsCard[\s\S]*?onSave=\{\(\) => void save\(\)\}/)
  assert.match(card, /id="pelican-settings"/)
  for (const field of ["model", "prompt", "reasoning_effort", "parallel_count", "cron_expression"]) {
    assert.ok(card.includes(`settings.pelican_${field}`), `missing global field ${field}`)
  }
  assert.match(card, /disabled=\{saving \|\| invalid\} onClick=\{onSave\}/)
  assert.doesNotMatch(card, /<Switch|pelican_enabled/)
})

test("\u8d26\u53f7\u4ec5\u63d0\u4ea4\u5f00\u5173\u4e14\u624b\u52a8\u6d4b\u8bd5\u4e0d\u4f9d\u8d56\u5b9a\u65f6\u542f\u7528", () => {
  const panel = source("../components/relay/pelican.tsx")
  assert.match(panel, /JSON\.stringify\(\{enabled\}\)/)
  assert.doesNotMatch(panel, /PelicanEditor|setEditing|context\.edit|config\?\.model/)
  assert.match(panel, /settings\.data\?\.pelican_model/)
  assert.match(panel, /disabled=\{active\|\|!configured\|\|!context\.data\}/)
  assert.match(panel, /to="\/settings#pelican-settings"/)
})
