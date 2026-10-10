import type { ComponentProps } from "react"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

type TableActionButtonProps = Omit<ComponentProps<typeof Button>, "variant" | "size"> & {
  destructive?: boolean
}

export function TableActionButton({ destructive = false, className, ...props }: TableActionButtonProps) {
  return (
    <Button
      type="button"
      variant="outline"
      size="icon"
      className={cn("size-8 shrink-0", destructive && "border-danger/30 text-danger hover:bg-danger/10 hover:text-danger", className)}
      {...props}
    />
  )
}
