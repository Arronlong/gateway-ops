import { useEffect, useState } from "react"
import { ExternalLink, GitBranch, RefreshCw } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { apiFetch } from "@/lib/api"
import { versionCheckMessage, type VersionInfo, type VersionCheckResult } from "@/lib/version"

export function VersionCard() {
  const [info, setInfo] = useState<VersionInfo | null>(null)
  const [infoError, setInfoError] = useState(false)
  const [result, setResult] = useState<VersionCheckResult | null>(null)
  const [checking, setChecking] = useState(false)
  const [checkError, setCheckError] = useState(false)

  useEffect(() => {
    const controller = new AbortController()
    void apiFetch<VersionInfo>("/version", { signal: controller.signal })
      .then(setInfo)
      .catch(() => { if (!controller.signal.aborted) setInfoError(true) })
    return () => controller.abort()
  }, [])

  async function checkUpdates() {
    setChecking(true)
    setCheckError(false)
    setResult(null)
    try {
      if (!info) {
        setInfo(await apiFetch<VersionInfo>("/version"))
        setInfoError(false)
      }
      setResult(await apiFetch<VersionCheckResult>("/version/check"))
    } catch {
      setCheckError(true)
    } finally {
      setChecking(false)
    }
  }

  const statusText = checking ? "正在查询 GitHub…" : checkError ? "检查更新失败，请检查网络或稍后重试" : versionCheckMessage(result)

  return (
    <Card id="version" className="gap-0 border border-border py-0 shadow-none">
      <CardHeader className="flex flex-row items-center justify-between gap-3 border-b border-border px-4 py-3">
        <CardTitle className="flex items-center gap-2 text-sm font-semibold"><GitBranch className="size-4 text-brand" />版本与更新</CardTitle>
        <Button type="button" variant="outline" size="sm" disabled={checking} onClick={() => void checkUpdates()}>
          <RefreshCw className={checking ? "size-3.5 animate-spin" : "size-3.5"} />检查更新
        </Button>
      </CardHeader>
      <CardContent className="space-y-3 px-4 py-4 text-sm">
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
          <span>当前版本：<strong className="font-mono">{info ? `v${info.version}` : infoError ? "读取失败" : "读取中…"}</strong></span>
          {info?.channel === "development" ? <span className="text-xs text-muted-foreground">开发构建</span> : null}
          {info?.channel === "prerelease" ? <span className="text-xs text-muted-foreground">预发布构建</span> : null}
        </div>
        {info?.revision && info.revision !== "unknown" ? <p className="text-xs text-muted-foreground">提交：<code>{info.revision.slice(0, 12)}</code>{info.build_time !== "unknown" ? ` · 构建时间：${new Date(info.build_time).toLocaleString("zh-CN")}` : ""}</p> : null}
        <div className="flex flex-wrap items-center gap-3" role="status" aria-live="polite">
          <span className={checkError ? "text-destructive" : result?.update_available ? "font-medium text-brand" : "text-muted-foreground"}>{statusText}</span>
          {result?.release_url ? <a className="inline-flex items-center gap-1 text-brand hover:underline" href={result.release_url} target="_blank" rel="noopener noreferrer">查看发布说明<ExternalLink className="size-3.5" /></a> : null}
        </div>
        <p className="text-xs leading-5 text-muted-foreground">检查 GitHub 上的最新稳定版；发现更新后，请按部署方式拉取指定版本并重启应用。查询结果缓存 5 分钟。</p>
      </CardContent>
    </Card>
  )
}
