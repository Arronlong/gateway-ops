import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"

test("\u9875\u9762\u6e32\u67d3\u5f02\u5e38\u4fdd\u7559\u6062\u590d\u754c\u9762\u548c\u9519\u8bef\u8bc1\u636e", () => {
  const text = readFileSync(new URL("./page-error-boundary.tsx", import.meta.url), "utf8")
  assert.match(text, /getDerivedStateFromError/)
  assert.match(text, /componentDidCatch/)
  assert.match(text, /componentDidUpdate/)
  assert.match(text, /role="alert"/)
  assert.match(text, /window.location.reload\(\)/)
  const shell = readFileSync(new URL("../app-shell.tsx", import.meta.url), "utf8")
  assert.match(shell, /<PageErrorBoundary[^>]*resetKey=/)
  assert.match(shell, /<Outlet\s*\/>/)
  const main = readFileSync(new URL("../../src/main.tsx", import.meta.url), "utf8")
  assert.match(main, /<PageErrorBoundary>/)
})
