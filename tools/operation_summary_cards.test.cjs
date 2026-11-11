const assert = require("node:assert/strict")
const fs = require("node:fs")
const path = require("node:path")
const {createRequire} = require("node:module")
const test = require("node:test")
const vm = require("node:vm")

const frontendRequire = createRequire(path.resolve("frontend/package.json"))
const ts = frontendRequire("typescript")
const source = fs.readFileSync("frontend/components/operations/summary-cards.tsx", "utf8")
const {outputText} = ts.transpileModule(source, {compilerOptions: {module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX}})
const exportsObject = {}
vm.runInNewContext(`(function(require, exports) {${outputText}\n})`)(name => name.startsWith("@/") ? {} : frontendRequire(name), exportsObject)
const {multiplier, cny, usd} = exportsObject

test("\u5b9e\u65f6\u500d\u7387\u56db\u820d\u4e94\u5165\u5230\u56db\u4f4d\u5c0f\u6570", () => {
  assert.equal(multiplier(0.123456), "0.1235 \u500d")
  assert.equal(multiplier(1.23454), "1.2345 \u500d")
  assert.equal(multiplier(0.0001), "0.0001 \u500d")
  assert.equal(multiplier(-0.123456), "-0.1235 \u500d")
})

test("\u5b9e\u65f6\u500d\u7387\u7684\u96f6\u503c\u548c\u6574\u6570\u8865\u8db3\u56db\u4f4d\u5c0f\u6570", () => {
  assert.equal(multiplier(0), "0.0000 \u500d")
  assert.equal(multiplier(1), "1.0000 \u500d")
  assert.equal(multiplier(1.2), "1.2000 \u500d")
})

test("\u7f3a\u5931\u548c\u975e\u6709\u9650\u500d\u7387\u4fdd\u7559\u5360\u4f4d\u7b26", () => {
  for (const value of [null, undefined, NaN, Infinity, -Infinity]) assert.equal(multiplier(value), "\u2014")
})

test("\u91d1\u989d\u4fdd\u7559\u539f\u6709\u4e24\u4f4d\u5c0f\u6570", () => {
  assert.equal(cny(12.3456), "\u00a512.35")
  assert.equal(usd(12.3456), "$12.35")
})
