import { useEffect, useRef, useState, type ComponentProps } from 'react'
import { Button } from '@/components/ui/button'
import { tap } from '@/lib/haptics'
import { cn } from '@/lib/utils'

// HoldButton needs the button to be held down before it fires, so a stray tap
// on a phone cannot delete anything. It works with a finger, mouse, or the
// keyboard (hold Enter or Space).
export function HoldButton({
  onConfirm,
  holdMs = 1000,
  children,
  className,
  disabled,
  ...props
}: Omit<ComponentProps<typeof Button>, 'onClick'> & { onConfirm: () => void; holdMs?: number }) {
  const [progress, setProgress] = useState(0)
  const raf = useRef(0)
  const start = useRef(0)
  const fired = useRef(false)

  const stop = () => {
    cancelAnimationFrame(raf.current)
    start.current = 0
    setProgress(0)
  }
  const begin = () => {
    if (disabled || start.current) return
    fired.current = false
    start.current = performance.now()
    const step = (now: number) => {
      const p = Math.min((now - start.current) / holdMs, 1)
      setProgress(p)
      if (p >= 1) {
        fired.current = true
        stop()
        tap()
        onConfirm()
        return
      }
      raf.current = requestAnimationFrame(step)
    }
    raf.current = requestAnimationFrame(step)
  }
  useEffect(() => () => cancelAnimationFrame(raf.current), [])

  return (
    <Button
      type="button"
      className={cn('relative touch-none overflow-hidden select-none', className)}
      disabled={disabled}
      aria-describedby={undefined}
      onPointerDown={(e) => {
        e.currentTarget.setPointerCapture(e.pointerId)
        begin()
      }}
      onPointerUp={stop}
      onPointerCancel={stop}
      onPointerLeave={stop}
      onKeyDown={(e) => {
        if ((e.key === 'Enter' || e.key === ' ') && !e.repeat) {
          e.preventDefault()
          begin()
        }
      }}
      onKeyUp={(e) => {
        if (e.key === 'Enter' || e.key === ' ') stop()
      }}
      onBlur={stop}
      onContextMenu={(e) => e.preventDefault()}
      {...props}
    >
      <span
        aria-hidden
        className="absolute inset-y-0 left-0 bg-white/30"
        style={{ width: `${progress * 100}%` }}
      />
      <span className="relative">{progress > 0 ? 'Keep holding…' : children}</span>
    </Button>
  )
}
