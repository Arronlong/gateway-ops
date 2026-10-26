import type { ReactNode } from "react"
import { ProductIcon } from "@/components/product-branding"
import { Button } from "@/components/ui/button"

interface ConfigurationLoadingProps {
  query: { loading: boolean; error: string | null; refetch: () => Promise<void> }
  title?: ReactNode
  className?: string
}

export function ConfigurationLoading({ query, title = "\u7cfb\u7edf\u8bbe\u7f6e", className = "rounded-xl border border-border bg-card p-6" }: ConfigurationLoadingProps) {
  return (
    <section aria-busy={query.loading} className={className}>
      <h1 className="flex items-center gap-2 text-base font-semibold text-foreground"><ProductIcon/>{title}</h1>
      <div role={query.error ? "alert" : "status"} className="mt-2 text-sm text-muted-foreground">
        <p>{query.error ? "\u914d\u7f6e\u52a0\u8f7d\u5931\u8d25\uff0c\u8bf7\u91cd\u8bd5\u3002" : "\u6b63\u5728\u52a0\u8f7d\u5b9e\u9645\u914d\u7f6e\u2026"}</p>
        {query.error ? (
          <Button type="button" variant="outline" size="sm" className="mt-3" disabled={query.loading} onClick={() => void query.refetch().catch(() => {})}>
            {"\u91cd\u8bd5"}
          </Button>
        ) : null}
      </div>
    </section>
  )
}
