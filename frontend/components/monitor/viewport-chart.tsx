"use client"

import { memo, useEffect, useRef, useState, type ReactNode } from "react"

const ChartContent = memo(function ChartContent({ children }: { children: ReactNode; visible: boolean }) {
  return <>{children}</>
}, (previous, next) => !next.visible || previous.children === next.children)

/** 离屏时延后首次挂载，已展示的图表保留状态，并在回到视口时更新数据。 */
export function ViewportChart({ children, placeholder }: { children: ReactNode; placeholder: ReactNode }) {
  const container = useRef<HTMLDivElement>(null)
  const [visible, setVisible] = useState(false)
  const [activated, setActivated] = useState(false)

  useEffect(() => {
    if (typeof IntersectionObserver === "undefined") {
      setVisible(true)
      setActivated(true)
      return
    }
    const observer = new IntersectionObserver(([entry]) => {
      setVisible(entry.isIntersecting)
      if (entry.isIntersecting) setActivated(true)
    }, { rootMargin: "120px 0px" })
    if (container.current) observer.observe(container.current)
    return () => observer.disconnect()
  }, [])

  return <div ref={container} className="min-w-0" data-slot="viewport-chart">
    {activated ? <ChartContent visible={visible}>{children}</ChartContent> : placeholder}
  </div>
}
