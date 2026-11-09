const assert = require("node:assert/strict")
const fs = require("node:fs")
const path = require("node:path")
const vm = require("node:vm")
const test = require("node:test")
const ts = require("../frontend/node_modules/typescript")

const source = fs.readFileSync(path.join(__dirname, "../frontend/components/relay/pelican-selection.tsx"), "utf8")
const start = source.indexOf("  function beginSelection(")
const end = source.indexOf("  async function removeSelected()", start)
assert.ok(start >= 0 && end > start)
const code = ts.transpileModule(source.slice(start, end), {
  compilerOptions: {target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.None},
}).outputText

// 执行组件中的真实手势函数；模拟事件和定时器，不依赖浏览器布局。
function setup() {
  const handlers = new Map(), timers = new Map()
  let timerID = 0
  const state = {selected: new Set(["original"]), box: null, writes: 0}
  const context = {
    busyRef: {current: false}, suppressClick: {current: false}, stopDrag: {current: null},
    clickResetTimer: {current: null},
    selected: state.selected, entries: {current: new Map()},
    window: {
      innerWidth: 1440, innerHeight: 900,
      addEventListener(name, fn) {
        if (!handlers.has(name)) handlers.set(name, new Set())
        handlers.get(name).add(fn)
      },
      removeEventListener(name, fn) {
        handlers.get(name)?.delete(fn)
        if (!handlers.get(name)?.size) handlers.delete(name)
      },
      setTimeout(fn) {timers.set(++timerID, fn); return timerID},
      clearTimeout(id) {timers.delete(id)},
    },
    setBox(value) {state.box = value},
    setSelected(value) {state.selected = value; state.writes++},
    pelicanSelectionRect: (a, b) => ({left: Math.min(a.x, b.x), top: Math.min(a.y, b.y), right: Math.max(a.x, b.x), bottom: Math.max(a.y, b.y)}),
    getPelicanSelection: () => new Set(["dragged"]),
  }
  vm.createContext(context)
  vm.runInContext(code, context)
  const emit = (name, values = {}) => {
    for (const fn of [...(handlers.get(name) || [])]) {
      fn({pointerId: 1, buttons: 1, clientX: 30, clientY: 30, preventDefault() {}, ...values})
    }
  }
  const flush = () => {
    const pending = [...timers.values()]
    timers.clear()
    pending.forEach(fn => fn())
  }
  const begin = () => context.beginSelection({
    button: 0, pointerType: "mouse", target: {closest: () => null},
    clientX: 10, clientY: 10, pointerId: 1,
  })
  begin()
  return {context, handlers, state, emit, flush, begin}
}

test("Escape 恢复选择，但松键前仍抑制点击且不再更新选框", () => {
  const h = setup()
  h.emit("pointermove")
  h.emit("keydown", {key: "Escape"})
  h.flush()
  assert.deepEqual([...h.state.selected], ["original"])
  assert.equal(h.state.box, null)
  assert.equal(h.context.suppressClick.current, true)
  assert.equal(h.handlers.has("pointerup"), true)
  const writes = h.state.writes
  h.emit("pointermove", {clientX: 80})
  assert.equal(h.state.writes, writes)
  assert.equal(h.state.box, null)
  h.emit("pointerup", {buttons: 0})
  assert.equal(h.context.suppressClick.current, true)
  assert.equal(h.handlers.size, 0)
  h.flush()
  assert.equal(h.context.suppressClick.current, false)
})

test("窗口失焦立即恢复选择并移除全部手势监听", () => {
  const h = setup()
  h.emit("pointermove")
  h.emit("blur")
  assert.equal(h.handlers.size, 0)
  assert.equal(h.state.box, null)
  assert.deepEqual([...h.state.selected], ["original"])
  const writes = h.state.writes
  h.emit("pointermove", {buttons: 0})
  assert.equal(h.state.writes, writes)
  h.flush()
  assert.equal(h.context.suppressClick.current, false)
})

test("未收到 pointerup 时，无左键的移动取消手势而非继续框选", () => {
  const h = setup()
  h.emit("pointermove")
  h.emit("pointermove", {buttons: 2, clientX: 80})
  assert.equal(h.handlers.size, 0)
  assert.equal(h.state.box, null)
  assert.deepEqual([...h.state.selected], ["original"])
})

test("其他指针的取消事件不影响当前手势", () => {
  const h = setup()
  h.emit("pointermove")
  h.emit("pointercancel", {pointerId: 2})
  assert.notEqual(h.state.box, null)
  assert.equal(h.handlers.has("pointerup"), true)
  h.emit("pointercancel")
  assert.equal(h.handlers.size, 0)
  assert.deepEqual([...h.state.selected], ["original"])
})

test("普通点击不被抑制，正常拖动保留选中结果", () => {
  const h = setup()
  h.emit("pointerup", {buttons: 0})
  assert.equal(h.context.suppressClick.current, false)
  h.flush()
  h.begin()
  h.emit("pointermove")
  h.emit("pointerup", {buttons: 0})
  assert.deepEqual([...h.state.selected], ["dragged"])
  assert.equal(h.state.box, null)
  assert.equal(h.context.suppressClick.current, true)
  h.flush()
  assert.equal(h.context.suppressClick.current, false)
})

test("上一手势的延迟清理不能取消新手势的点击抑制", () => {
  const h = setup()
  h.emit("pointermove")
  h.emit("pointerup", {buttons: 0})
  h.begin()
  h.emit("pointermove")
  h.flush()
  assert.equal(h.context.suppressClick.current, true)
})

test("卸载清理移除全部手势监听", () => {
  const h = setup()
  h.emit("pointermove")
  h.context.stopDrag.current()
  assert.equal(h.handlers.size, 0)
})
