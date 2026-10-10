import type { ComponentProps, ReactNode } from "react"
import { CardHeader } from "@/components/ui/card"
import { cn } from "@/lib/utils"

interface ListPanelHeaderProps extends Omit<ComponentProps<"div">, "title"> {
  title: ReactNode
  description?: ReactNode
  summary?: ReactNode
  actions?: ReactNode
  meta?: ReactNode
  secondary?: ReactNode
  secondaryPlacement?: "row" | "inline"
  actionsPlacement?: "header" | "secondary"
}

export function ListPanelHeader({ title, description, summary, actions, meta, secondary, secondaryPlacement = "row", actionsPlacement = "header", children, className, ...props }: ListPanelHeaderProps) {
  const inlineSecondary = secondaryPlacement === "inline" && secondary
  const inlineSummaryLayout = Boolean(inlineSecondary && summary)
  const headerActions = actionsPlacement === "header" ? actions : null
  const secondaryActions = actionsPlacement === "secondary" ? actions : null

  return (
    <CardHeader className={cn("list-panel-controls flex flex-col items-stretch px-4 py-3", secondaryPlacement === "row" && "gap-2", className)} {...props}>
      <div data-slot="list-panel-header-row" className={cn("flex min-w-0 flex-wrap justify-between gap-x-4 gap-y-2", summary ? "items-end" : "items-center", inlineSummaryLayout && "grid grid-cols-[minmax(0,1fr)_auto] 2xl:grid-cols-[minmax(0,1fr)_max-content]")}>
        <div data-slot="list-panel-heading" className={cn("min-w-0 flex-1 basis-48", inlineSummaryLayout && "col-start-1 row-start-1 2xl:row-span-2")}>
          {title}
          {description}
          {summary ? <div data-slot="list-panel-summary" className="mt-2 flex min-w-0 max-w-full flex-wrap items-center gap-3">{summary}</div> : null}
        </div>
        {inlineSecondary || headerActions || meta ? (
          <div data-slot="list-panel-toolbar" className={cn("ml-auto flex min-w-0 max-w-full flex-wrap items-center justify-end gap-2 [&_[data-slot=select-trigger]]:h-11 sm:[&_[data-slot=select-trigger]]:h-9", !inlineSecondary && (summary || secondary || !actions ? "flex-[0_1_auto]" : "flex-[4_1_32rem]"), inlineSummaryLayout && "col-span-2 col-start-1 row-start-2 w-full 2xl:col-span-1 2xl:col-start-2 2xl:w-auto", inlineSecondary && !inlineSummaryLayout && "flex-[1_1_auto] 2xl:flex-[0_1_auto]", inlineSecondary && "2xl:flex-nowrap")}>
            {inlineSecondary ? <div data-slot="list-panel-secondary-toolbar" className="flex min-w-0 flex-1 flex-wrap items-center gap-2 2xl:flex-nowrap">{secondary}{secondaryActions}</div> : null}
            {headerActions}
            {!inlineSummaryLayout ? meta : null}
          </div>
        ) : null}
        {inlineSummaryLayout && meta ? <div data-slot="list-panel-meta" className="col-start-2 row-start-1 ml-auto flex min-w-0 max-w-full self-start items-center justify-end">{meta}</div> : null}
      </div>
      {secondaryPlacement === "row" && (secondary || secondaryActions) ? <div data-slot="list-panel-secondary-toolbar" className="flex min-w-0 flex-wrap items-center gap-2 border-t border-border pt-2">{secondary}{secondaryActions}</div> : null}
      {children}
    </CardHeader>
  )
}
