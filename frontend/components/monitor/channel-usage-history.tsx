import { useCallback, useEffect, useRef, useState } from "react"
import { History, Loader2 } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { TimeRangeFilter } from "@/components/ui/time-range-filter"
import { useTriggerRefresh } from "@/lib/refresh-context"
import { refetchUsageHistoryStream } from "@/lib/sync-stream"
import { serializeTimeRange, type TimeRangeSelection } from "@/lib/time-range"
import { usageIntegritySnapshot, usageIntegrityPause, usageIntegrityProgress, type UsageIntegrity } from "@/lib/usage-integrity"
import { UsageIntegrityReport } from "@/components/monitor/usage-integrity-report"
import type { Channel, RelayUsageRangePreset } from "@/lib/api-types"
import { cn } from "@/lib/utils"

const historyRanges = [
  { value: "today", label: "\u4eca\u5929" },
  { value: "24h", label: "24 \u5c0f\u65f6" },
  { value: "7d", label: "7 \u5929" },
  { value: "30d", label: "30 \u5929" },
  { value: "all", label: "\u5168\u90e8" },
]

export function ChannelUsageHistory({ channel, range, compact = false }: { channel: Channel; range: TimeRangeSelection<RelayUsageRangePreset>; compact?: boolean }) {
  const [open, setOpen] = useState(false)
  const [historyRange, setHistoryRange] = useState(range)
  const [running, setRunning] = useState(false)
  const [message, setMessage] = useState("")
  const [failed, setFailed] = useState(false)
  const [manualResult, setManualResult] = useState<UsageIntegrity | null>(null)
  const [manualStartedAt,setManualStartedAt]=useState(0)
  const [clock,setClock]=useState(Date.now())
  const activeRequest = useRef(false)
  const requestQuery = serializeTimeRange(historyRange)?.toString() ?? null
  const refresh = useTriggerRefresh()
  const data = manualResult
  const retryAt=data?.phase==="paused" && data.retry_at ? new Date(data.retry_at).getTime() : 0
  useEffect(()=>{
    if(!running && !retryAt)return
    setClock(Date.now())
    const timer=setInterval(()=>setClock(Date.now()),1000)
    return()=>clearInterval(timer)
  },[running,retryAt])
  const elapsed=Math.max(0,Math.floor((clock-manualStartedAt)/1000))
  const paused=usageIntegrityPause(data,clock)
  const checkError=manualResult?.error || ""

  const refetchHistory = useCallback(async () => {
    if (!requestQuery || activeRequest.current) return
    const bounds = new URLSearchParams(requestQuery)
    activeRequest.current = true
    setRunning(true)
    setManualResult(previous=>previous ? {...previous,phase:"locating",error:undefined,retry_at:undefined,accounts:previous.accounts.map(account=>({...account,error:undefined}))} : null)
    setManualStartedAt(Date.now())
    setFailed(false)
    setMessage("\u6b63\u5728\u6838\u5bf9\u5e76\u8865\u9f50\u5386\u53f2\u6570\u636e\u2026")
    let done = false
    let error = ""
    let pausedResult: UsageIntegrity | null = null
    try {
      await refetchUsageHistoryStream(channel.id, bounds, { onEvent: event => {
        setMessage(usageIntegrityProgress(event))
        const snapshot = usageIntegritySnapshot(event.data)
        if (snapshot) setManualResult(snapshot)
        if (snapshot?.phase === "paused") pausedResult=snapshot
        if (event.stage === "error" || event.ok === false) error = event.message
        if (event.stage === "done" && event.ok !== false) done = true
      } })
      if (error || !done) throw new Error(error || "\u672a\u6536\u5230\u5b8c\u6210\u72b6\u6001\uff0c\u8bf7\u68c0\u67e5\u540e\u91cd\u8bd5")
      toast.success("\u5386\u53f2\u6570\u636e\u6838\u5bf9\u4e0e\u8865\u9f50\u5b8c\u6210")
    } catch (error) {
      const text = error instanceof Error ? error.message : "\u5386\u53f2\u6570\u636e\u62c9\u53d6\u5931\u8d25"
      setMessage(pausedResult ? usageIntegrityPause(pausedResult) : text)
      setManualResult(previous=>previous?.phase==="paused" ? previous : previous ? {...previous,phase:"interrupted",error:text} : null)
      setFailed(!pausedResult)
      if (pausedResult) toast.message("\u4e0a\u6e38\u8bf7\u6c42\u5df2\u6682\u7f13\uff0c\u5df2\u5b8c\u6210\u7684\u4fee\u590d\u5df2\u4fdd\u5b58")
      else toast.error(text)
    } finally {
      activeRequest.current = false
      setRunning(false)
      refresh()
    }
  }, [channel.id, requestQuery, refresh])

  useEffect(() => {
    if (!open || running || manualResult?.phase !== "paused" || manualResult.retry_status !== 429 || !manualResult.retry_at) return
    const timer = setTimeout(() => { void refetchHistory() }, Math.max(0, new Date(manualResult.retry_at).getTime() - Date.now()) + 250)
    return () => clearTimeout(timer)
  }, [open, running, manualResult?.phase, manualResult?.retry_at, manualResult?.retry_status, refetchHistory])

  if (channel.balance_mode === "manual") return null

  return <>
    <div className={cn("flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1", compact ? "mt-1" : "ml-auto max-w-full")}>
      <Button variant="ghost" size="sm" className="h-7 max-w-full shrink-0 gap-1 px-1.5 text-[11px]" onClick={() => { setHistoryRange(range); setManualResult(null); setMessage(""); setFailed(false); setOpen(true) }}>
        <History className="size-3" />{"\u91cd\u65b0\u62c9\u53d6\u5386\u53f2"}
      </Button>
      {checkError || paused ? <p role={paused ? "status" : "alert"} className={cn("w-full break-words text-[11px]",paused ? "text-warning" : "text-danger")}>{paused || checkError}</p> : null}
    </div>
    <Dialog open={open} onOpenChange={next => { if (!running) setOpen(next) }}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-xl" onInteractOutside={event => { if (running) event.preventDefault() }}>
        <DialogHeader>
          <DialogTitle>{channel.name}{"\u00b7\u91cd\u65b0\u62c9\u53d6\u5386\u53f2\u6570\u636e"}</DialogTitle>
          <DialogDescription>{"\u9009\u597d\u65f6\u95f4\u8303\u56f4\u540e\u624b\u52a8\u5f00\u59cb\u6838\u5bf9\u5e76\u8865\u9f50\uff0c\u6bcf\u9875\u53d6\u5f97\u540e\u7acb\u5373\u4fdd\u5b58\uff1b\u4e2d\u65ad\u540e\u4ece\u5df2\u4fdd\u5b58\u7684\u8fdb\u5ea6\u7ee7\u7eed\u3002\u4e0a\u6e38\u5df2\u6e05\u7406\u7684\u65e5\u5fd7\u65e0\u6cd5\u6062\u590d\u3002"}</DialogDescription>
        </DialogHeader>
        <fieldset disabled={running} className="min-w-0">
          <TimeRangeFilter value={historyRange} options={historyRanges} onChange={next=>{
            if (next === historyRange) return
            setHistoryRange(next)
            setManualResult(null)
            setMessage("")
            setFailed(false)
          }} label={`\u5386\u53f2\u91cd\u62c9\u65f6\u95f4-${channel.id}`} />
        </fieldset>
        {data || running ? <UsageIntegrityReport result={data} loading={running} /> : null}
        <section className="min-w-0 rounded-md border border-border p-3 text-xs" aria-label={"\u5904\u7406\u8fdb\u5ea6"}>
          <h3 className="mb-2 font-medium">{"3. \u5904\u7406\u8fdb\u5ea6"}</h3>
          {message && !failed && !paused ? <p role="status" className="flex items-start gap-1 break-words text-muted-foreground">{running ? <Loader2 className="mt-0.5 size-3.5 shrink-0 animate-spin"/> : null}<span className="min-w-0 break-words">{message}{running ? ` \u00b7 \u5df2\u7528\u65f6 ${elapsed} \u79d2` : ""}</span></p> : null}
          {paused ? <p role="status" className="break-words text-warning">{paused}{manualResult?.retry_status===429 ? "\uff1b\u4fdd\u6301\u5f39\u7a97\u6253\u5f00\u5c06\u81ea\u52a8\u7ee7\u7eed" : ""}</p> : checkError || failed ? <p role="alert" className="break-words text-danger">{"\u5904\u7406\u672a\u5b8c\u6210\uff1a"}{failed ? message : checkError}</p> : null}
          {!running && !message && !checkError ? <p className="text-muted-foreground">{!data ? "\u8bf7\u9009\u62e9\u65f6\u95f4\u8303\u56f4\uff0c\u70b9\u51fb\u201c\u5f00\u59cb\u6838\u5bf9\u5e76\u8865\u9f50\u201d\u540e\u6267\u884c\u3002" : data.status==="mismatch" ? "\u6838\u5bf9\u5b8c\u6210\uff0c\u5dee\u5f02\u5c1a\u672a\u4fee\u590d\u3002" : "\u65e0\u9700\u4fee\u590d\u3002"}</p> : null}
        </section>
        <DialogFooter>
          <Button variant="outline" disabled={running} onClick={() => setOpen(false)}>{"\u5173\u95ed"}</Button>
          <Button disabled={running || retryAt>clock || requestQuery === null} onClick={() => void refetchHistory()}>{running ? <Loader2 className="size-3.5 animate-spin" /> : <History className="size-3.5" />}{running ? "\u6b63\u5728\u6838\u5bf9\u5e76\u8865\u9f50\u2026" : retryAt>clock ? "\u7b49\u5f85\u4e0a\u6e38\u6062\u590d" : failed || paused ? "\u7ee7\u7eed\u5904\u7406" : "\u5f00\u59cb\u6838\u5bf9\u5e76\u8865\u9f50"}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </>
}
