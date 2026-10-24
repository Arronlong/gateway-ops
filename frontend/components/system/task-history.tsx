import { useEffect, useState } from "react"
import { ChevronLeft, FileText, History, RefreshCw } from "lucide-react"
import { apiFetch } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { cn } from "@/lib/utils"

interface TaskRun {
  id: number
  task: string
  status: "running" | "success" | "failed"
  started_at: string
  finished_at: string | null
  duration_ms: number
  summary: string
  logs?: string
}
interface TaskRunPage {
  runs: TaskRun[]
  total: number
  succeeded: number
  failed: number
  running: number
  page: number
  page_size: number
}

function formatTime(value: string | null) {
  if (!value) return "\u2014"
  return new Date(value).toLocaleString("zh-CN", { hour12: false })
}
function duration(run: TaskRun) {
  const ms = run.status === "running" ? Date.now() - new Date(run.started_at).getTime() : run.duration_ms
  if (ms < 1000) return `${Math.max(0, ms)} ms`
  return `${(ms / 1000).toFixed(1)} \u79d2`
}
const statusLabels = { running: "\u6267\u884c\u4e2d", success: "\u6210\u529f", failed: "\u5931\u8d25" }

export function TaskHistoryButton({ task, title }: { task: string; title: string }) {
  const [open, setOpen] = useState(false)
  return <>
    <Button type="button" variant="outline" size="sm" className="h-8 gap-1.5 border-brand/25 bg-brand/5 px-2.5 text-xs text-brand hover:bg-brand/10 hover:text-brand" aria-label={`${title}\u67e5\u770b\u6267\u884c\u8bb0\u5f55`} onClick={() => setOpen(true)}>
      <History className="size-3.5" />{"\u67e5\u770b\u6267\u884c\u8bb0\u5f55"}</Button>
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetContent className="w-full gap-0 sm:w-[760px] sm:max-w-[calc(100vw-2rem)]">
        <SheetHeader className="border-b border-border pr-12">
          <SheetTitle>{title}{"\u00b7\u6267\u884c\u8bb0\u5f55"}</SheetTitle>
          <SheetDescription>{"\u8bb0\u5f55\u529f\u80fd\u4e0a\u7ebf\u540e\u7684\u81ea\u52a8\u8c03\u5ea6\uff08\u542b\u542f\u52a8\u540c\u6b65\uff09\uff1b\u4e0d\u8865\u9020\u5386\u53f2\u6570\u636e\u3002"}</SheetDescription>
        </SheetHeader>
        {open && <TaskHistory task={task} />}
      </SheetContent>
    </Sheet>
  </>
}

function TaskHistory({ task }: { task: string }) {
  const [page, setPage] = useState(1)
  const [version, setVersion] = useState(0)
  const [data, setData] = useState<TaskRunPage | null>(null)
  const [selected, setSelected] = useState<number | null>(null)
  const [detail, setDetail] = useState<TaskRun | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [detailError, setDetailError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [detailLoading, setDetailLoading] = useState(false)
  const base = `/tasks/${encodeURIComponent(task)}/runs`

  useEffect(() => {
    let controller: AbortController | null = null
    let disposed = false
    let active = false
    const load = async () => {
      if (active) return
      active = true
      controller = new AbortController()
      try {
        const result = await apiFetch<TaskRunPage>(`${base}?page=${page}&page_size=20`, { signal: controller.signal })
        if (!disposed) { setData(result); setError(null) }
      } catch (cause) {
        if (!disposed && !controller.signal.aborted) setError(cause instanceof Error ? cause.message : "\u52a0\u8f7d\u6267\u884c\u8bb0\u5f55\u5931\u8d25")
      } finally { active = false; if (!disposed) setLoading(false) }
    }
    setLoading(true)
    setData(null)
    void load()
    const timer = setInterval(() => void load(), 5000)
    return () => { disposed = true; controller?.abort(); clearInterval(timer) }
  }, [base, page, version])

  useEffect(() => {
    setDetail(null)
    setDetailError(null)
    if (selected === null) return
    let controller: AbortController | null = null
    let disposed = false
    let active = false
    const load = async () => {
      if (active) return
      active = true
      controller = new AbortController()
      try {
        const run = await apiFetch<TaskRun>(`${base}/${selected}`, { signal: controller.signal })
        if (!disposed) { setDetail(run); setDetailError(null) }
      } catch (cause) {
        if (!disposed && !controller.signal.aborted) setDetailError(cause instanceof Error ? cause.message : "\u52a0\u8f7d\u65e5\u5fd7\u5931\u8d25")
      } finally { active = false; if (!disposed) setDetailLoading(false) }
    }
    setDetailLoading(true)
    void load()
    const timer = setInterval(() => void load(), 5000)
    return () => { disposed = true; controller?.abort(); clearInterval(timer) }
  }, [base, selected, version])

  return <div className="min-h-0 flex-1 overflow-y-auto p-4">
    <div className="mb-4 flex items-center justify-between gap-2">
      {selected !== null ? <Button variant="ghost" size="sm" onClick={() => setSelected(null)}>
        <ChevronLeft className="size-4" />{"\u8fd4\u56de\u6267\u884c\u5217\u8868"}</Button> : <span className="text-xs text-muted-foreground">{"\u6bcf 5 \u79d2\u81ea\u52a8\u5237\u65b0"}</span>}
      <Button type="button" variant="outline" size="sm" onClick={() => setVersion(value => value + 1)}><RefreshCw className="size-3.5" />{"\u5237\u65b0"}</Button>
    </div>
    {selected !== null ? <section aria-label={"\u5355\u6b21\u6267\u884c\u65e5\u5fd7"} className="space-y-3">
      <h3 className="text-sm font-semibold">{"\u6267\u884c"} #{selected}</h3>
      {detailLoading && <p role="status" className="text-sm text-muted-foreground">{"\u65e5\u5fd7\u52a0\u8f7d\u4e2d\u2026"}</p>}
      {detailError && <p role="alert" className="text-sm text-destructive">{detailError}</p>}
      {detail && <>
        <div className="space-y-1 text-xs text-muted-foreground">
          <p>{statusLabels[detail.status]} · {duration(detail)}</p>
          <p>{"\u5f00\u59cb\uff1a"}{formatTime(detail.started_at)}</p>
          <p>{"\u7ed3\u675f\uff1a"}{formatTime(detail.finished_at)}</p>
          <p className="break-words">{detail.summary}</p>
        </div>
        <pre className="max-h-[65vh] overflow-auto whitespace-pre-wrap break-words rounded-md border border-border bg-muted/30 p-3 font-mono text-xs leading-5">{detail.logs || (detail.status === "running" ? "\u4efb\u52a1\u6267\u884c\u4e2d\uff0c\u65e5\u5fd7\u5c06\u5728\u6267\u884c\u7ed3\u675f\u540e\u4fdd\u5b58\u3002" : "\u672c\u6b21\u6267\u884c\u65e0\u53ef\u7528\u65e5\u5fd7\u3002")}</pre>
      </>}
    </section> : <>
      {data && <div className="mb-4 grid grid-cols-2 gap-2 sm:grid-cols-4">{[["\u7d2f\u8ba1\u6267\u884c", data.total], ["\u6210\u529f", data.succeeded], ["\u5931\u8d25", data.failed], ["\u6267\u884c\u4e2d", data.running]].map(([label, count]) => <div key={String(label)} className="rounded-md border border-border p-3">
        <p className="text-xs text-muted-foreground">{label}</p>
        <p className="mt-1 text-lg font-semibold tabular-nums">{count}</p>
      </div>)}</div>}
      {loading && <p role="status" className="py-8 text-center text-sm text-muted-foreground">{"\u6267\u884c\u8bb0\u5f55\u52a0\u8f7d\u4e2d\u2026"}</p>}
      {error && <p role="alert" className="mb-3 text-sm text-destructive">{error}</p>}
      {data && data.total === 0 && <p className="py-12 text-center text-sm text-muted-foreground">{"\u6682\u65e0\u6267\u884c\u8bb0\u5f55\uff0c\u4efb\u52a1\u4e0b\u6b21\u81ea\u52a8\u6267\u884c\u540e\u4f1a\u663e\u793a\u5728\u8fd9\u91cc\u3002"}</p>}
      {data && data.total > 0 && <>
        <div className="overflow-x-auto rounded-md border border-border">
          <table className="w-full text-left text-xs">
            <thead className="bg-muted/50 text-muted-foreground">
              <tr>{["\u6267\u884c\u7f16\u53f7", "\u5f00\u59cb\u65f6\u95f4 / \u7ed3\u675f\u65f6\u95f4", "\u8017\u65f6", "\u7ed3\u679c", "\u64cd\u4f5c"].map((label, index) => <th key={label} className={cn("whitespace-nowrap p-3 font-medium", index === 0 && "hidden sm:table-cell")}>{label}</th>)}</tr>
            </thead>
            <tbody>{data.runs.map(run => <tr key={run.id} className="border-t border-border">
              <td className="hidden p-3 tabular-nums sm:table-cell">#{run.id}</td>
              <td className="whitespace-nowrap p-3 tabular-nums">
                <p>{formatTime(run.started_at)}</p>
                <p className="mt-1 text-muted-foreground">{formatTime(run.finished_at)}</p>
              </td>
              <td className="whitespace-nowrap p-3 tabular-nums">{duration(run)}</td>
              <td className="p-3">
                <span className={cn("whitespace-nowrap rounded px-1.5 py-0.5 font-medium", run.status === "success" ? "bg-success/10 text-success" : run.status === "failed" ? "bg-destructive/10 text-destructive" : "bg-brand/10 text-brand")}>{statusLabels[run.status]}</span>
              </td>
              <td className="p-2">
                <Button type="button" variant="ghost" size="sm" className="size-8 p-0 text-xs sm:w-auto sm:px-2.5" aria-label={`\u67e5\u770b\u6267\u884c ${run.id} \u7684\u65e5\u5fd7`} onClick={() => setSelected(run.id)}>
                  <FileText className="size-3.5" />
                  <span className="sr-only sm:not-sr-only">{"\u67e5\u770b\u65e5\u5fd7"}</span>
                </Button>
              </td>
            </tr>)}</tbody>
          </table>
        </div>
        <div className="mt-4 flex items-center justify-between gap-2 text-xs text-muted-foreground">
          <span>{"\u7b2c "}{data.page}{" / "}{Math.max(1, Math.ceil(data.total / data.page_size))}{" \u9875"}</span>
          <div className="flex gap-2">
            <Button variant="outline" size="sm" disabled={loading || page <= 1} onClick={() => setPage(value => value - 1)}>{"\u4e0a\u4e00\u9875"}</Button>
            <Button variant="outline" size="sm" disabled={loading || page * data.page_size >= data.total} onClick={() => setPage(value => value + 1)}>{"\u4e0b\u4e00\u9875"}</Button>
          </div>
        </div>
      </>}
    </>}
  </div>
}
