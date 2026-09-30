import { useRef, useState, type ReactNode, type TouchEvent } from 'react'
import { RefreshCw } from 'lucide-react'
import { tap } from '@/lib/haptics'
import { cn } from '@/lib/utils'

const THRESHOLD = 70
const MAX_PULL = 100
const DRAG_RATIO = 0.5

/** Pull down to refresh, for pages inside AppShell's scrolling <main>. It only
 * tracks a gesture that starts with <main> scrolled to the top, so it never
 * fights normal scrolling. */
export function PullToRefresh({ onRefresh, children }: { onRefresh: () => Promise<unknown>; children: ReactNode }) {
  const [pull, setPull] = useState(0)
  const [refreshing, setRefreshing] = useState(false)
  const startY = useRef<number | null>(null)
  const root = useRef<HTMLDivElement>(null)

  const onTouchStart = (e: TouchEvent) => {
    const scroller = root.current?.closest<HTMLElement>('[data-scroll-root]')
    startY.current = !refreshing && e.touches.length === 1 && scroller?.scrollTop === 0 ? e.touches[0].clientY : null
  }
  const onTouchMove = (e: TouchEvent) => {
    if (startY.current === null) return
    const dy = e.touches[0].clientY - startY.current
    setPull(dy > 0 ? Math.min(dy * DRAG_RATIO, MAX_PULL) : 0)
  }
  const onTouchEnd = async () => {
    if (startY.current === null) return
    startY.current = null
    if (pull < THRESHOLD) {
      setPull(0)
      return
    }
    tap()
    setRefreshing(true)
    setPull(THRESHOLD)
    try {
      await onRefresh()
    } finally {
      setRefreshing(false)
      setPull(0)
    }
  }

  return (
    <div ref={root} onTouchStart={onTouchStart} onTouchMove={onTouchMove} onTouchEnd={onTouchEnd}>
      <div
        className="flex items-center justify-center overflow-hidden transition-[height] duration-200 ease-out"
        style={{ height: pull }}
      >
        <RefreshCw
          className={cn('size-5 text-primary', refreshing && 'animate-spin')}
          style={refreshing ? undefined : { transform: `rotate(${pull * 3}deg)` }}
        />
      </div>
      {children}
    </div>
  )
}
