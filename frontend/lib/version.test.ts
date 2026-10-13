import assert from "node:assert/strict"
import test from "node:test"
// @ts-expect-error Node 22 的原生 TypeScript 测试需要显式 .ts 扩展名。
import { GITHUB_LICENSE_URL, GITHUB_REPOSITORY_URL, versionCheckMessage } from "./version.ts"

test("project links use the Arronlong repository", () => {
  assert.equal(GITHUB_REPOSITORY_URL, "https://github.com/Arronlong/gateway-ops")
  assert.equal(GITHUB_LICENSE_URL, "https://github.com/Arronlong/gateway-ops/blob/main/LICENSE")
})

test("检查失败或没有发布版本时不显示已是最新版", () => {
  assert.equal(versionCheckMessage(null), "尚未检查更新")
  assert.equal(versionCheckMessage({ status: "check_failed" }), "检查更新失败，请稍后重试")
  assert.equal(versionCheckMessage({ status: "no_release" }), "尚未发布正式版本")
})

test("按后端状态区分新版、当前最新版和开发版本", () => {
  assert.equal(versionCheckMessage({ status: "update_available", latest_version: "1.1.0" }), "发现新版本 v1.1.0")
  assert.equal(versionCheckMessage({ status: "up_to_date" }), "当前已是最新稳定版")
  assert.equal(versionCheckMessage({ status: "current_newer" }), "当前版本高于最新稳定版")
})
