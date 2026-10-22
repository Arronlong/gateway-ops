import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"

const source = (file: string) => readFileSync(new URL(file, import.meta.url), "utf8")

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
