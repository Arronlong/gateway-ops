"use client"

import { useEffect, useRef, useState } from "react"
import { Plus } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { apiFetch } from "@/lib/api"
import type { ProxyConfig } from "@/lib/api-types"

interface ProxySelectProps {
  value: string
  onChange: (value: string) => void
  onPendingChange: (pending: boolean) => void
  disabled?: boolean
}

export function ProxySelect({ value, onChange, onPendingChange, disabled }: ProxySelectProps) {
  const [proxies, setProxies] = useState<ProxyConfig[]>([])
  const [loading, setLoading] = useState(true)
  const [creating, setCreating] = useState(false)
  const [saving, setSaving] = useState(false)
  const [url, setURL] = useState("")
  const [error, setError] = useState<string | null>(null)
  const mounted = useRef(false)

  useEffect(() => {
    mounted.current = true
    const controller = new AbortController()
    apiFetch<ProxyConfig[]>("/proxies", { signal: controller.signal })
      .then((items) => setProxies((current) => [...items, ...current.filter((item) => !items.some((loaded) => loaded.id === item.id))]))
      .catch((error: Error) => { if (!controller.signal.aborted) setError(error.message || "读取代理失败") })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => { mounted.current = false; controller.abort() }
  }, [])

  useEffect(() => {
    onPendingChange(creating || saving)
    return () => onPendingChange(false)
  }, [creating, saving, onPendingChange])

  async function createProxy() {
    if (saving || !url.trim()) return
    setSaving(true)
    setError(null)
    try {
      const proxy = await apiFetch<ProxyConfig>("/proxies", { method: "POST", body: JSON.stringify({ url: url.trim() }) })
      if (!mounted.current) return
      setProxies((items) => [...items.filter((item) => item.id !== proxy.id), proxy])
      onChange(String(proxy.id))
      setCreating(false)
      setURL("")
    } catch (error) {
      if (mounted.current) setError(error instanceof Error ? error.message : "新增代理失败")
    } finally {
      if (mounted.current) setSaving(false)
    }
  }

  return (
    <div className="min-w-0 space-y-1.5">
      <Label htmlFor="channel-proxy">代理</Label>
      <div className="flex min-w-0 gap-2">
        <Select value={value || "none"} onValueChange={(value) => {
          // 动态加入选项时原生 select 会短暂报告空值；清除绑定只通过 none。
          if (value) onChange(value === "none" ? "" : value)
        }} disabled={disabled || saving}>
          <SelectTrigger id="channel-proxy" className="min-w-0 flex-1"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="none">不使用代理</SelectItem>
            {value && !proxies.some((proxy) => String(proxy.id) === value) ? <SelectItem value={value}>{loading ? "正在读取代理…" : `代理 #${value}（不可用）`}</SelectItem> : null}
            {proxies.map((proxy) => <SelectItem key={proxy.id} value={String(proxy.id)}>{proxy.display_url}</SelectItem>)}
          </SelectContent>
        </Select>
        <Button type="button" variant="outline" className="shrink-0 gap-1" disabled={disabled || creating} onClick={() => { setCreating(true); setError(null) }}><Plus className="size-3.5" />新增代理</Button>
      </div>
      {creating ? (
        <div className="space-y-2 rounded-md border border-border p-2">
          <Input aria-label="代理地址" placeholder="http://user:password@127.0.0.1:7890" value={url} onChange={(event) => setURL(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") { event.preventDefault(); void createProxy() } }} autoComplete="off" spellCheck={false} disabled={disabled || saving} />
          <div className="flex justify-end gap-2">
            <Button type="button" variant="ghost" size="sm" disabled={saving} onClick={() => { setCreating(false); setURL(""); setError(null) }}>取消</Button>
            <Button type="button" size="sm" disabled={disabled || saving || !url.trim()} onClick={() => void createProxy()}>{saving ? "保存中…" : "添加并选用"}</Button>
          </div>
        </div>
      ) : null}
      {error ? <p role="alert" className="text-xs text-danger">{error}</p> : null}
    </div>
  )
}
