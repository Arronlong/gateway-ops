import { createContext, useCallback, useContext, useEffect, useRef, useState, type PointerEvent as ReactPointerEvent, type ReactNode, type RefObject } from "react"
import { Trash2 } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { useConfirm } from "@/components/ui/confirm-dialog"
import { apiFetch } from "@/lib/api"
import type { PelicanResult } from "@/lib/api-types"
import { deletePelicanResults, getPelicanSelection, pelicanSelectionRect, updatePelicanSelection, type PelicanSelectionRect } from "@/lib/pelican-selection"
import { pelicanResultPath } from "@/lib/pelican-sources"
import { pelicanThumbnailPool } from "@/lib/pelican-thumbnails"

type SelectionEntry = {element: HTMLDivElement; result: () => PelicanResult}
type SelectionContext = {
  selected: Set<string>
  removed: Set<string>
  availableKeys: ReadonlySet<string>
  busy: boolean
  toggle: (key: string) => void
  select: (keys: readonly string[], checked: boolean) => void
  register: (key: string, entry: SelectionEntry) => () => void
}
const PelicanSelectionContext = createContext<SelectionContext | null>(null)

export function usePelicanResultSelection(result: PelicanResult, ref: RefObject<HTMLDivElement | null>) {
  const context = useContext(PelicanSelectionContext)
  const latest = useRef(result)
  latest.current = result
  const key = pelicanResultPath(result)
  const register = context?.register
  const removed = context?.removed.has(key) ?? false
  useEffect(() => {
    if (!register || !ref.current || removed) return
    return register(key, {element: ref.current, result: () => latest.current})
  }, [key, register, ref, removed, result.status])
  return context ? {checked: context.selected.has(key), busy: context.busy, removed, toggle: () => context.toggle(key)} : null
}

export function PelicanGroupSelection({results, name}: {results: readonly PelicanResult[]; name: string}) {
  const context = useContext(PelicanSelectionContext)
  if (!context) return null
  const keys = [...new Set(results.map(result => pelicanResultPath(result)))].filter(key => context.availableKeys.has(key) && !context.removed.has(key))
  const allSelected = keys.length > 0 && keys.every(key => context.selected.has(key))
  const anySelected = keys.some(key => context.selected.has(key))
  return <div className="inline-flex shrink-0 items-center gap-1" role="group" aria-label={`${name}选择当前筛选结果`}>
    <Button type="button" variant="outline" size="sm" className="h-7 px-2 text-[10px]" aria-label={`${name}全选当前筛选结果`} disabled={context.busy || !keys.length || allSelected} onClick={() => context.select(keys, true)}>全选</Button>
    <Button type="button" variant="ghost" size="sm" className="h-7 px-2 text-[10px]" aria-label={`${name}全不选当前筛选结果`} disabled={context.busy || !anySelected} onClick={() => context.select(keys, false)}>全不选</Button>
  </div>
}

export function PelicanSelection({children, onRefresh}: {children: ReactNode; onRefresh: () => Promise<unknown>}) {
  const entries = useRef(new Map<string, SelectionEntry>())
  const [available, setAvailable] = useState(0)
  const [selected, setSelected] = useState<Set<string>>(() => new Set())
  const [removed, setRemoved] = useState<Set<string>>(() => new Set())
  const [busy, setBusy] = useState(false)
  const [progress, setProgress] = useState({done: 0, total: 0})
  const [box, setBox] = useState<PelicanSelectionRect | null>(null)
  const busyRef = useRef(false), suppressClick = useRef(false)
  const stopDrag = useRef<(() => void) | null>(null)
  const clickResetTimer = useRef<number | null>(null)
  const {confirm, dialog} = useConfirm()

  const register = useCallback((key: string, entry: SelectionEntry) => {
    entries.current.set(key, entry)
    setAvailable(entries.current.size)
    return () => {
      if (entries.current.get(key) !== entry) return
      entries.current.delete(key)
      setAvailable(entries.current.size)
      setSelected(current => {
        if (!current.has(key)) return current
        const next = new Set(current)
        next.delete(key)
        return next
      })
    }
  }, [])
  useEffect(() => () => {
    stopDrag.current?.()
    if (clickResetTimer.current !== null) window.clearTimeout(clickResetTimer.current)
  }, [])

  function toggle(key: string) {
    if (busyRef.current || !entries.current.has(key)) return
    setSelected(current => {
      const next = new Set(current)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  function select(keys: readonly string[], checked: boolean) {
    if (busyRef.current) return
    const available = keys.filter(key => entries.current.has(key))
    setSelected(current => updatePelicanSelection(current, available, checked))
  }

  function beginSelection(event: ReactPointerEvent<HTMLDivElement>) {
    if (busyRef.current || event.button !== 0 || event.pointerType !== "mouse") return
    const target = event.target as HTMLElement
    if (target.closest("a,button:not([data-pelican-preview]),input,select,textarea,[role=checkbox],[role=combobox],[role=switch]")) return
    stopDrag.current?.()
    if (clickResetTimer.current !== null) {
      window.clearTimeout(clickResetTimer.current)
      clickResetTimer.current = null
    }
    suppressClick.current = false
    const start = {x: event.clientX, y: event.clientY}
    const original = new Set(selected)
    const initial = event.ctrlKey || event.metaKey || event.shiftKey ? original : new Set<string>()
    const pointerID = event.pointerId
    let moved = false
    let cancelled = false
    const move = (pointer: PointerEvent) => {
      if (pointer.pointerId !== pointerID) return
      if ((pointer.buttons & 1) === 0) {cancel(); return}
      if (cancelled) return
      if (!moved && Math.hypot(pointer.clientX - start.x, pointer.clientY - start.y) < 6) return
      moved = true
      suppressClick.current = true
      pointer.preventDefault()
      const rect = pelicanSelectionRect(start, {x: pointer.clientX, y: pointer.clientY})
      setBox(rect)
      const bounds = [...entries.current].map(([key, entry]) => {
        const card = entry.element.getBoundingClientRect()
        const strip = entry.element.closest("[data-pelican-results]")?.getBoundingClientRect() ?? card
        // 横向滚动条外的作品不参与框选。
        return {key, rect: {left: Math.max(card.left, strip.left, 0), right: Math.min(card.right, strip.right, window.innerWidth), top: Math.max(card.top, strip.top, 0), bottom: Math.min(card.bottom, strip.bottom, window.innerHeight)}}
      })
      setSelected(getPelicanSelection(bounds, rect, initial))
    }
    const cleanup = () => {
      window.removeEventListener("pointermove", move)
      window.removeEventListener("pointerup", finish)
      window.removeEventListener("pointercancel", pointerCancel)
      window.removeEventListener("keydown", escape)
      window.removeEventListener("blur", cancel)
      stopDrag.current = null
    }
    const reset = () => {
      cleanup()
      setBox(null)
      clickResetTimer.current = window.setTimeout(() => {
        suppressClick.current = false
        clickResetTimer.current = null
      }, 0)
    }
    const finish = (pointer: PointerEvent) => {if (pointer.pointerId === pointerID) reset()}
    const cancelSelection = () => {
      if (cancelled) return
      cancelled = true
      if (moved) setSelected(original)
      setBox(null)
    }
    const cancel = () => {cancelSelection(); reset()}
    const pointerCancel = (pointer: PointerEvent) => {if (pointer.pointerId === pointerID) cancel()}
    // Escape 仅取消选区，松键前保留监听和点击抑制。
    const escape = (key: KeyboardEvent) => {if (key.key === "Escape") cancelSelection()}
    window.addEventListener("pointermove", move, {passive: false})
    window.addEventListener("pointerup", finish)
    window.addEventListener("pointercancel", pointerCancel)
    window.addEventListener("keydown", escape)
    window.addEventListener("blur", cancel)
    stopDrag.current = cleanup
  }

  async function removeSelected() {
    if (busyRef.current) return
    const results = [...selected].flatMap(key => {
      const result = entries.current.get(key)?.result()
      return result ? [result] : []
    })
    if (!results.length) return
    busyRef.current = true
    setBusy(true)
    setProgress({done: 0, total: results.length})
    try {
      if (!await confirm({title: `删除选中的 ${results.length} 份测智作品？`, description: "将永久删除所选渠道、账号和分组中的作品及结果，操作不可恢复。", confirmLabel: "确认批量删除", destructive: true})) return
      const outcome = await deletePelicanResults(results, async (path, result) => {
        try {
          await apiFetch(path, {method: "DELETE"})
          pelicanThumbnailPool.delete(`${path}?v=${result.finished_at}`)
        } finally {
          setProgress(current => ({...current, done: current.done + 1}))
        }
      })
      setRemoved(current => new Set([...current, ...outcome.deleted.map(result => pelicanResultPath(result))]))
      setSelected(new Set(outcome.failed.map(({result}) => pelicanResultPath(result))))
      if (outcome.failed.length) {
        const error = outcome.failed[0].error
        toast.error(`已删除 ${outcome.deleted.length} 份，${outcome.failed.length} 份删除失败，已保留勾选${error instanceof Error ? `：${error.message}` : ""}`)
      } else toast.success(`已删除 ${outcome.deleted.length} 份测智作品`)
      try {await onRefresh()} catch {toast.warning("作品列表刷新失败，请稍后刷新。")}
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "批量删除失败")
    } finally {
      busyRef.current = false
      setBusy(false)
    }
  }

  return <PelicanSelectionContext.Provider value={{selected, removed, availableKeys: new Set(entries.current.keys()), busy, toggle, select, register}}>
    <div className="min-w-0 space-y-3">
      <div className="flex flex-wrap items-center gap-2" role="toolbar" aria-label="测智作品批量操作" aria-busy={busy}>
        <span className="text-xs text-muted-foreground" aria-live="polite">{busy ? `删除中 ${progress.done}/${progress.total}` : `已选 ${selected.size} 份`}</span>
        <Button type="button" variant="outline" size="sm" disabled={busy || !available} onClick={() => setSelected(new Set(entries.current.keys()))}>全选当前结果</Button>
        <Button type="button" variant="ghost" size="sm" disabled={busy || !selected.size} onClick={() => setSelected(new Set())}>清空选择</Button>
        <Button type="button" variant="destructive" size="sm" disabled={busy || !selected.size} onClick={() => void removeSelected()}><Trash2 className="mr-1 size-3.5"/>批量删除{selected.size ? ` (${selected.size})` : ""}</Button>
        <span className="text-[10px] text-muted-foreground">拖动框选，Ctrl / ⌘ 可追加选择</span>
      </div>
      <div className="min-w-0 select-none space-y-5" onPointerDownCapture={beginSelection} onDragStart={event => event.preventDefault()} onClickCapture={event => {
        if (suppressClick.current) {event.preventDefault(); event.stopPropagation(); suppressClick.current = false}
      }}>{children}</div>
    </div>
    {box ? <div aria-hidden="true" className="pointer-events-none fixed z-50 border border-brand bg-brand/15" style={{left: box.left, top: box.top, width: box.right - box.left, height: box.bottom - box.top}}/> : null}
    {dialog}
  </PelicanSelectionContext.Provider>
}
