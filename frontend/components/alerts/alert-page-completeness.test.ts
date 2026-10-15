import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"

const read = (file: string) => readFileSync(new URL(file, import.meta.url), "utf8")

test("预警中心保留四个分区和指标区域", () => {
  const source = read("../../app/notifications-page.tsx")
  assert.match(source, /预警事件/)
  assert.match(source, /订阅规则/)
  assert.match(source, /通知渠道/)
  assert.match(source, /发送记录/)
  assert.match(source, /待处理事件/)
  assert.match(source, /运行中规则/)
})

test("现有通知路径和预警路径共用页面", () => {
  const source = read("../../src/main.tsx")
  assert.match(source, /path="notifications" element=\{<NotificationsPage \/>\}/)
  assert.match(source, /path="alerts" element=\{<NotificationsPage \/>\}/)
})
