"use client"

import { Component, type ErrorInfo, type ReactNode } from "react"
import { AlertTriangle, RefreshCw, RotateCcw } from "lucide-react"
import { Button } from "@/components/ui/button"

interface PageErrorBoundaryProps {
  children: ReactNode
  resetKey?: string
}

interface PageErrorBoundaryState {
  error: Error | null
}

export class PageErrorBoundary extends Component<PageErrorBoundaryProps, PageErrorBoundaryState> {
  state: PageErrorBoundaryState = { error: null }

  static getDerivedStateFromError(error: Error): PageErrorBoundaryState {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("Page rendering failed", error, info.componentStack)
  }

  componentDidUpdate(previous: PageErrorBoundaryProps) {
    if (this.state.error && previous.resetKey !== this.props.resetKey) {
      this.setState({ error: null })
    }
  }

  render() {
    if (!this.state.error) return this.props.children

    return (
      <section role="alert" className="mx-auto my-6 flex w-full max-w-2xl flex-col gap-4 rounded-xl border border-danger/25 bg-card p-6 text-card-foreground">
        <div className="flex items-start gap-3">
          <AlertTriangle className="mt-0.5 size-5 shrink-0 text-danger" aria-hidden="true" />
          <div className="min-w-0">
            <h1 className="text-base font-semibold">页面加载出错</h1>
            <p className="mt-1 text-sm text-muted-foreground">当前页面发生渲染异常，可以重试或重新加载。错误详情已保留，方便定位问题。</p>
          </div>
        </div>
        <details className="rounded-md bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
          <summary className="cursor-pointer font-medium">查看错误详情</summary>
          <p className="mt-2 break-words font-mono">{this.state.error.message}</p>
        </details>
        <div className="flex flex-wrap gap-2">
          <Button type="button" variant="outline" onClick={() => this.setState({ error: null })}><RotateCcw className="size-4" />重试当前页面</Button>
          <Button type="button" onClick={() => window.location.reload()}><RefreshCw className="size-4" />重新加载</Button>
        </div>
      </section>
    )
  }
}
