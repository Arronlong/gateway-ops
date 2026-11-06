import { useCallback, useEffect, useRef, useState } from "react"
import { useRefreshTick } from "@/lib/refresh-context"
import { fetchUsageIntegrity } from "@/lib/usage-integrity-request"
import type { UsageIntegrity } from "@/lib/usage-integrity"
import { usageIntegritySnapshot, usageIntegrityProgress } from "@/lib/usage-integrity"

interface IntegrityQueryState {
  path: string | null
  data: UsageIntegrity | null
  loading: boolean
  refreshing: boolean
  error: string | null
  progress: string
  startedAt: number
}

export function useUsageIntegrityQuery(path: string | null) {
  const [state, setState] = useState<IntegrityQueryState>({ path, data: null, loading: !!path, refreshing: !!path, error: null, progress: "", startedAt: Date.now() })
  const active = useRef<{ path: string; controller: AbortController; promise: Promise<void> } | null>(null)
  const tick = useRefreshTick()
  const previousTick = useRef(tick)
  const fallbackStartedAt = useRef({ path, startedAt: Date.now() })
  if (fallbackStartedAt.current.path !== path) fallbackStartedAt.current = { path, startedAt: Date.now() }

  const run = useCallback(() => {
    if (!path) return Promise.resolve()
    if (active.current?.path === path) return active.current.promise
    active.current?.controller.abort()
    const controller = new AbortController()
    setState(previous => ({ path, data: previous.path === path && previous.data ? {...previous.data,error:undefined,phase:"total",accounts:previous.data.accounts.map(account=>({...account,error:undefined}))} : null, loading: previous.path !== path || previous.data === null, refreshing: true, error: null, progress: "\u6b63\u5728\u6838\u5bf9\u603b\u6570\u2026", startedAt: Date.now() }))
    const promise = fetchUsageIntegrity(path, controller.signal, event => {
      if (controller.signal.aborted) return
      const snapshot = usageIntegritySnapshot(event.data)
      setState(previous => previous.path === path ? { ...previous, data: snapshot ?? previous.data, progress: usageIntegrityProgress(event) } : previous)
    }).then(data => {
      if (!controller.signal.aborted) setState(previous => ({ ...previous, data, loading: false, refreshing: false, error: null }))
    }).catch(error => {
      const message = error instanceof Error ? error.message : String(error)
      if (!controller.signal.aborted) setState(previous => ({ ...previous, data: previous.data ? { ...previous.data, phase: "interrupted", error: message } : null, loading: false, refreshing: false, error: message, progress: "" }))
    }).finally(() => {
      if (active.current?.controller === controller) active.current = null
    })
    active.current = { path, controller, promise }
    return promise
  }, [path])

  useEffect(() => {
    if (!path) {
      setState({ path: null, data: null, loading: false, refreshing: false, error: null, progress: "", startedAt: 0 })
      return
    }
    const timer = setTimeout(() => { void run() }, 0)
    return () => {
      clearTimeout(timer)
      if (active.current?.path === path) {
        active.current.controller.abort()
        active.current = null
      }
    }
  }, [path, run])

  useEffect(() => {
    if (previousTick.current === tick) return
    previousTick.current = tick
    if (state.data?.phase === "paused" && state.data.retry_at && Date.now()<new Date(state.data.retry_at).getTime()) return
    if (!active.current) void run()
  }, [tick, run, state.data])

  useEffect(()=>{
    if (!path || state.path !== path || state.data?.phase !== "paused" || state.data.retry_status !== 429 || !state.data.retry_at) return
    const timer=setTimeout(()=>{void run()},Math.max(0,new Date(state.data.retry_at).getTime()-Date.now())+250)
    return()=>clearTimeout(timer)
  },[path,state.path,state.data?.phase,state.data?.retry_status,state.data?.retry_at,run])

  const current = state.path === path ? state : { path, data: null, loading: !!path, refreshing: !!path, error: null, progress: "\u6b63\u5728\u6838\u5bf9\u603b\u6570\u2026", startedAt: fallbackStartedAt.current.startedAt }
  return { ...current, refetch: run }
}
