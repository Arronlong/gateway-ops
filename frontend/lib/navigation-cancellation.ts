export interface NavigationClickIntent {
  button: number
  metaKey: boolean
  ctrlKey: boolean
  shiftKey: boolean
  altKey: boolean
  href: string
  target?: string
  download?: string | boolean
}

export function shouldCancelNavigation(intent: NavigationClickIntent, currentHref: string) {
  if (intent.button !== 0 || intent.metaKey || intent.ctrlKey || intent.shiftKey || intent.altKey) return false
  if (intent.target && intent.target.toLowerCase() !== "_self") return false
  if (intent.download) return false

  try {
    const current = new URL(currentHref)
    const next = new URL(intent.href, current)
    if (next.origin !== current.origin) return false
    return next.pathname !== current.pathname || next.search !== current.search
  } catch {
    return false
  }
}

export function installNavigationCancellation(cancel: () => void) {
  const onClick = (event: MouseEvent) => {
    if (event.defaultPrevented || !(event.target instanceof Element)) return
    const anchor = event.target.closest<HTMLAnchorElement>("a[href]")
    if (!anchor) return
    if (shouldCancelNavigation({
      button: event.button,
      metaKey: event.metaKey,
      ctrlKey: event.ctrlKey,
      shiftKey: event.shiftKey,
      altKey: event.altKey,
      href: anchor.href,
      target: anchor.target,
      download: anchor.download,
    }, window.location.href)) {
      cancel()
    }
  }
  const onPopState = () => cancel()

  document.addEventListener("click", onClick, true)
  window.addEventListener("popstate", onPopState)
  return () => {
    document.removeEventListener("click", onClick, true)
    window.removeEventListener("popstate", onPopState)
  }
}
