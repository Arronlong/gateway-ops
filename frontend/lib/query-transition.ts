import { startTransition } from "react"

export type QueryTransitionScheduler = (callback: () => void) => void

export function scheduleQueryUpdate(
  update: () => void,
  scheduler: QueryTransitionScheduler = startTransition,
) {
  scheduler(update)
}
