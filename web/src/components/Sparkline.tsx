import { useId } from 'react'
import { cn } from '@/lib/utils'

// Sparkline is a tiny trend line. It is decorative: the exact value is always
// shown as text next to it, so it is hidden from assistive technology.
export function Sparkline({
  values,
  max = 100,
  width = 72,
  height = 22,
  className,
  color = 'var(--chart-cpu)',
}: {
  values: number[]
  max?: number
  width?: number
  height?: number
  className?: string
  color?: string
}) {
  const id = useId()
  if (values.length < 2) {
    return <span aria-hidden className={cn('inline-block', className)} style={{ width, height }} />
  }
  const top = Math.max(max, ...values)
  const x = (i: number) => (i / (values.length - 1)) * (width - 2) + 1
  const y = (v: number) => height - 2 - (Math.max(v, 0) / top) * (height - 4)
  const line = values.map((v, i) => `${i ? 'L' : 'M'}${x(i).toFixed(1)},${y(v).toFixed(1)}`).join('')
  const area = `${line}L${x(values.length - 1).toFixed(1)},${height}L${x(0).toFixed(1)},${height}Z`
  return (
    <svg aria-hidden width={width} height={height} className={cn('inline-block shrink-0', className)}>
      <defs>
        <linearGradient id={id} x1="0" x2="0" y1="0" y2="1">
          <stop offset="0" stopColor={color} stopOpacity={0.3} />
          <stop offset="1" stopColor={color} stopOpacity={0} />
        </linearGradient>
      </defs>
      <path d={area} fill={`url(#${id})`} />
      <path d={line} fill="none" stroke={color} strokeWidth={1.5} strokeLinejoin="round" strokeLinecap="round" />
    </svg>
  )
}
