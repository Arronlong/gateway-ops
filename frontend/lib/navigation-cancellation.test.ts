import assert from "node:assert/strict"
import test from "node:test"

// @ts-expect-error Node 22 原生 TypeScript 测试需要显式 .ts 扩展名。
import { shouldCancelNavigation } from "./navigation-cancellation.ts"

const baseURL = "https://gateway.example/relay-stations"

function click(overrides: Partial<Parameters<typeof shouldCancelNavigation>[0]> = {}) {
  return {
    button: 0,
    metaKey: false,
    ctrlKey: false,
    shiftKey: false,
    altKey: false,
    href: "https://gateway.example/channels",
    target: "",
    download: false,
    ...overrides,
  }
}

test("cancels query requests before an internal route change", () => {
  assert.equal(shouldCancelNavigation(click(), baseURL), true)
})

test("keeps current-page, modified, external, and new-tab clicks untouched", () => {
  assert.equal(shouldCancelNavigation(click({ href: baseURL }), baseURL), false)
  assert.equal(shouldCancelNavigation(click({ ctrlKey: true }), baseURL), false)
  assert.equal(shouldCancelNavigation(click({ href: "https://other.example/channels" }), baseURL), false)
  assert.equal(shouldCancelNavigation(click({ target: "_blank" }), baseURL), false)
  assert.equal(shouldCancelNavigation(click({ download: true }), baseURL), false)
})
