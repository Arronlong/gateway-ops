import type { ReactNode } from "react"
import { RotateCcw, SlidersHorizontal } from "lucide-react"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

interface ListFilterGroup {
  label: string
  value: string
  options: { value: string; label: string }[]
  onValueChange: (value: string) => void
}

interface ListFilterMenuProps {
  groups: ListFilterGroup[]
  activeCount: number
  onReset: () => void
  children?: ReactNode
}

export function ListFilterMenu({ groups, activeCount, onReset, children }: ListFilterMenuProps) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button type="button" variant="outline" size="sm" className="h-11 shrink-0 gap-1.5 text-xs sm:h-9" aria-label={"\u7b5b\u9009"}>
          <SlidersHorizontal className="size-3.5" />{"\u7b5b\u9009"}
          {activeCount > 0 ? <span className="rounded-full bg-brand/10 px-1.5 py-0.5 text-[10px] font-semibold text-brand">{activeCount}</span> : null}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="max-h-[min(70dvh,480px)] w-64 max-w-[calc(100vw-2rem)] overflow-y-auto">
        <DropdownMenuLabel>{"\u7b5b\u9009\u6761\u4ef6"}</DropdownMenuLabel>
        <DropdownMenuSeparator />
        {children ? <><div className="space-y-2 px-2 py-2" onKeyDown={(event) => { if (event.key !== "Escape") event.stopPropagation() }}>{children}</div><DropdownMenuSeparator /></> : null}
        {groups.map((group) => (
          <div key={group.label}>
            <DropdownMenuLabel className="text-xs text-muted-foreground">{group.label}</DropdownMenuLabel>
            <DropdownMenuRadioGroup value={group.value} onValueChange={group.onValueChange} aria-label={group.label}>
              {group.options.map((option) => <DropdownMenuRadioItem key={option.value} value={option.value} className="min-h-11 text-xs sm:min-h-9">{option.label}</DropdownMenuRadioItem>)}
            </DropdownMenuRadioGroup>
            <DropdownMenuSeparator />
          </div>
        ))}
        <DropdownMenuItem className="min-h-11 text-xs sm:min-h-9" onSelect={onReset}><RotateCcw className="size-3.5" />{"\u91cd\u7f6e\u7b5b\u9009"}</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
