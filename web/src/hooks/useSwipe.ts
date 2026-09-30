import { useEffect, useRef, type RefObject } from 'react'

const MIN_DISTANCE = 60 // px, well past a finger wobble on a tap
const MAX_VERTICAL_RATIO = 0.5 // mostly horizontal
const MIN_VELOCITY = 0.15 // px/ms, filters slow drags that are really scrolls

/** useSwipe calls onSwipe for a quick horizontal swipe on el. Like Kashly it
 * only listens passively and decides at touchend, so scrolling and taps are
 * never intercepted. Native listeners (not React's) keep touches inside
 * dialogs, which render in portals, from counting. Elements marked
 * data-no-swipe (e.g. horizontally scrollable areas) are ignored. */
export function useSwipe(el: RefObject<HTMLElement | null>, onSwipe: (dir: 'left' | 'right') => void) {
  const cb = useRef(onSwipe)
  cb.current = onSwipe

  useEffect(() => {
    const node = el.current
    if (!node) return
    let start: { x: number; y: number; t: number } | null = null

    const onStart = (e: TouchEvent) => {
      const target = e.target as Element | null
      start =
        e.touches.length === 1 && !target?.closest('[data-no-swipe]')
          ? { x: e.touches[0].clientX, y: e.touches[0].clientY, t: Date.now() }
          : null
    }
    const onEnd = (e: TouchEvent) => {
      const begin = start
      start = null
      if (!begin) return
      const t = e.changedTouches[0]
      const dx = t.clientX - begin.x
      const dy = t.clientY - begin.y
      if (Math.abs(dx) < MIN_DISTANCE) return
      if (Math.abs(dy) > Math.abs(dx) * MAX_VERTICAL_RATIO) return
      if (Math.abs(dx) / Math.max(Date.now() - begin.t, 1) < MIN_VELOCITY) return
      cb.current(dx < 0 ? 'left' : 'right')
    }
    node.addEventListener('touchstart', onStart, { passive: true })
    node.addEventListener('touchend', onEnd, { passive: true })
    return () => {
      node.removeEventListener('touchstart', onStart)
      node.removeEventListener('touchend', onEnd)
    }
  }, [el])
}
