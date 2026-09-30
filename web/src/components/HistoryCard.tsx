import { useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent, type PointerEvent } from 'react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { usePoll } from '@/hooks/usePoll'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { api, type HistoryPoint, type MetricRange } from '@/lib/api'

type Range = 'live' | MetricRange
const RANGES: { value: Range; label: string; seconds: number; tickEvery: number }[] = [
  { value: 'live', label: 'Last 3 hours', seconds: 3 * 3600, tickEvery: 3600 },
  { value: '24h', label: 'Last 24 hours', seconds: 24 * 3600, tickEvery: 4 * 3600 },
  { value: '7d', label: 'Last 7 days', seconds: 7 * 86400, tickEvery: 86400 },
  { value: '30d', label: 'Last 30 days', seconds: 30 * 86400, tickEvery: 5 * 86400 },
]
const HEIGHT = 128
const PAD = { top: 10, right: 44, bottom: 22, left: 34 }

type Key = 'cpu' | 'mem'
const SERIES: { key: Key; label: string; color: string }[] = [
  { key: 'cpu', label: 'CPU', color: 'var(--chart-cpu)' },
  { key: 'mem', label: 'Memory', color: 'var(--chart-mem)' },
]

const clock = (t: number, seconds = false) =>
  new Date(t * 1000).toLocaleTimeString(undefined, {
    hour: '2-digit',
    minute: '2-digit',
    ...(seconds && { second: '2-digit' }),
    hour12: false,
  })
const day = (t: number) => new Date(t * 1000).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
const pct = (v: number) => `${v < 10 ? v.toFixed(1) : v.toFixed(0)}%`

// HistoryCard shows host CPU and memory as two small
// multiples on the same 0-100% scale with a shared crosshair. The readout
// above the charts shows the latest values, or the hovered moment's.
export function HistoryCard({ hostId }: { hostId: string }) {
  // The live view comes from the agent (30 s steps, kept in its memory). Longer
  // ranges come from the controller, which stores one sample a minute.
  const [range, setRange] = useState<Range>('live')
  const cfg = RANGES.find((r) => r.value === range)!
  const live = usePoll(() => api.history(hostId), 30_000, range === 'live' ? hostId : null)
  const stored = usePoll(() => api.metrics(hostId, range as MetricRange), 60_000, range === 'live' ? null : `${hostId}/${range}`)
  const history = range === 'live' ? live : stored
  const points: HistoryPoint[] = useMemo(
    () => (range === 'live' ? (live.data?.points ?? []) : (stored.data?.points ?? [])),
    [range, live.data, stored.data],
  )
  const step = (range === 'live' ? live.data?.step_seconds : stored.data?.bucket_seconds) ?? 30
  const [hover, setHover] = useState<number | null>(null) // index into points
  const long = cfg.seconds > 24 * 3600

  const end = useMemo(() => Math.max(Date.now() / 1000, points.at(-1)?.t ?? 0), [points])
  const start = end - cfg.seconds
  const shown = hover !== null ? points[hover] : points.at(-1)
  const since = points[0] && points[0].t > start + Math.max(10 * 60, cfg.seconds * 0.05) ? points[0].t : null
  const stamp = (t: number, seconds = false) => (cfg.seconds > 24 * 3600 ? `${day(t)} ${clock(t)}` : clock(t, seconds))

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between gap-3">
          <CardTitle>{cfg.label}</CardTitle>
          <Select value={range} onValueChange={(v) => { setHover(null); setRange(v as Range) }}>
            <SelectTrigger size="sm" className="w-36" aria-label="Time range">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {RANGES.map((r) => (
                <SelectItem key={r.value} value={r.value}>
                  {r.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <CardDescription className="flex flex-wrap items-baseline gap-x-4 gap-y-1">
          {shown ? (
            <>
              <span className="tabular-nums">{hover !== null ? stamp(shown.t, true) : 'Now'}</span>
              {SERIES.map((s) => (
                <span key={s.key} className="flex items-center gap-1.5">
                  <span className="h-0.5 w-3 rounded-full" style={{ background: s.color }} />
                  <span className="font-semibold text-foreground tabular-nums">{pct(shown[s.key])}</span>
                  <span>{s.label}</span>
                </span>
              ))}
              {since && hover === null && <span>collecting since {stamp(since)}</span>}
            </>
          ) : history.error ? (
            <span className="text-destructive">{history.error.message}</span>
          ) : (
            <span>{history.loading ? 'Loading…' : 'Collecting data… the first points appear within a minute.'}</span>
          )}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="grid gap-4 sm:grid-cols-2">
          {SERIES.map((s) => (
            <MiniChart
              key={s.key}
              label={s.label}
              color={s.color}
              value={(p) => p[s.key]}
              points={points}
              step={step}
              start={start}
              end={end}
              hover={hover}
              onHover={setHover}
              windowLabel={cfg.label.toLowerCase()}
              tickEvery={cfg.tickEvery}
              long={long}
            />
          ))}
        </div>
        <HistoryTable points={points} label={cfg.label.toLowerCase()} stamp={stamp} every={Math.max(600, cfg.seconds / 18)} />
      </CardContent>
    </Card>
  )
}

function useWidth() {
  const ref = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(0)
  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    const ro = new ResizeObserver(([e]) => setWidth(e.contentRect.width))
    ro.observe(el)
    setWidth(el.getBoundingClientRect().width)
    return () => ro.disconnect()
  }, [])
  return { ref, width }
}

function MiniChart({
  label,
  color,
  value,
  points,
  step,
  start,
  end,
  hover,
  onHover,
  windowLabel,
  tickEvery,
  long,
}: {
  windowLabel: string
  tickEvery: number
  long: boolean
  label: string
  color: string
  value: (p: HistoryPoint) => number
  points: HistoryPoint[]
  step: number
  start: number
  end: number
  hover: number | null
  onHover: (i: number | null) => void
}) {
  const { ref, width } = useWidth()
  const plotW = Math.max(width - PAD.left - PAD.right, 1)
  const plotH = HEIGHT - PAD.top - PAD.bottom
  const x = (t: number) => PAD.left + ((t - start) / (end - start)) * plotW
  const y = (v: number) => PAD.top + (1 - Math.min(Math.max(v, 0), 100) / 100) * plotH
  const baseline = y(0)

  // Break the line where samples are missing (agent was offline).
  const { line, area } = useMemo(() => {
    let line = ''
    let area = ''
    let seg: HistoryPoint[] = []
    const flush = () => {
      if (seg.length === 0) return
      const d = seg.map((p, i) => `${i ? 'L' : 'M'}${x(p.t).toFixed(1)},${y(value(p)).toFixed(1)}`).join('')
      line += d
      area += `${d}L${x(seg.at(-1)!.t).toFixed(1)},${baseline}L${x(seg[0].t).toFixed(1)},${baseline}Z`
      seg = []
    }
    for (const p of points) {
      if (p.t < start) continue
      if (seg.length && p.t - seg.at(-1)!.t > step * 2.5) flush()
      seg.push(p)
    }
    flush()
    return { line, area }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [points, width, start, end, step])

  const ticks = useMemo(() => {
    const out: number[] = []
    for (let t = Math.ceil(start / tickEvery) * tickEvery; t <= end; t += tickEvery) out.push(t)
    return out
  }, [start, end, tickEvery])

  const nearest = (clientX: number, rect: DOMRect) => {
    if (points.length === 0) return null
    const t = start + ((clientX - rect.left - PAD.left) / plotW) * (end - start)
    let lo = 0
    let hi = points.length - 1
    while (lo < hi) {
      const mid = (lo + hi) >> 1
      if (points[mid].t < t) lo = mid + 1
      else hi = mid
    }
    if (lo > 0 && Math.abs(points[lo - 1].t - t) < Math.abs(points[lo].t - t)) lo--
    return lo
  }
  const onMove = (e: PointerEvent<SVGSVGElement>) =>
    onHover(nearest(e.clientX, e.currentTarget.getBoundingClientRect()))
  const onKey = (e: KeyboardEvent<SVGSVGElement>) => {
    if (points.length === 0) return
    const i = hover ?? points.length - 1
    if (e.key === 'ArrowLeft') onHover(Math.max(0, i - 1))
    else if (e.key === 'ArrowRight') onHover(Math.min(points.length - 1, i + 1))
    else if (e.key === 'Escape') onHover(null)
    else return
    e.preventDefault()
  }

  const last = points.at(-1)
  const hp = hover !== null ? points[hover] : null

  return (
    // data-no-swipe: dragging across the chart scrubs; it must not flip tabs.
    <div ref={ref} data-no-swipe>
      <div className="mb-1 text-xs text-muted-foreground">{label}</div>
      {width > 0 && (
        <svg
          width={width}
          height={HEIGHT}
          role="img"
          aria-label={`${label}, ${windowLabel}${last ? `, now ${pct(value(last))}` : ''}. Use arrow keys to step through values.`}
          tabIndex={0}
          className="touch-pan-y rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
          onPointerMove={onMove}
          onPointerDown={onMove}
          onPointerLeave={() => onHover(null)}
          onKeyDown={onKey}
          onBlur={() => onHover(null)}
        >
          {[0, 50, 100].map((v) => (
            <g key={v}>
              <line x1={PAD.left} x2={PAD.left + plotW} y1={y(v)} y2={y(v)} stroke="var(--border)" strokeWidth={1} />
              <text
                x={PAD.left - 6}
                y={y(v) + 3.5}
                textAnchor="end"
                className="fill-muted-foreground text-[10px] tabular-nums"
              >
                {v}%
              </text>
            </g>
          ))}
          {ticks.map((t) => (
            <text
              key={t}
              x={x(t)}
              y={HEIGHT - 6}
              textAnchor="middle"
              className="fill-muted-foreground text-[10px] tabular-nums"
            >
              {long ? day(t) : clock(t)}
            </text>
          ))}
          <path d={area} fill={color} fillOpacity={0.1} />
          <path d={line} fill="none" stroke={color} strokeWidth={2} strokeLinejoin="round" strokeLinecap="round" />
          {last && !hp && (
            <>
              <circle cx={x(last.t)} cy={y(value(last))} r={4} fill={color} stroke="var(--card)" strokeWidth={2} />
              <text
                x={x(last.t) + 8}
                y={y(value(last)) + 4}
                className="fill-foreground text-[11px] font-medium tabular-nums"
              >
                {pct(value(last))}
              </text>
            </>
          )}
          {hp && (
            <>
              <line
                x1={x(hp.t)}
                x2={x(hp.t)}
                y1={PAD.top}
                y2={baseline}
                stroke="var(--muted-foreground)"
                strokeOpacity={0.6}
                strokeWidth={1}
              />
              <circle cx={x(hp.t)} cy={y(value(hp))} r={4} fill={color} stroke="var(--card)" strokeWidth={2} />
            </>
          )}
        </svg>
      )}
    </div>
  )
}

// HistoryTable is the non-visual view of the charts, one row per 10 minutes.
function HistoryTable({
  points,
  label,
  stamp,
  every,
}: {
  points: HistoryPoint[]
  label: string
  stamp: (t: number) => string
  every: number
}) {
  const rows = useMemo(() => {
    const out: HistoryPoint[] = []
    let next = 0
    for (const p of points) {
      if (p.t >= next) {
        out.push(p)
        next = p.t + every
      }
    }
    return out
  }, [points, every])
  return (
    <table className="sr-only">
      <caption>Host CPU and memory, {label}</caption>
      <thead>
        <tr>
          <th>Time</th>
          <th>CPU</th>
          <th>Memory</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((p) => (
          <tr key={p.t}>
            <td>{stamp(p.t)}</td>
            <td>{pct(p.cpu)}</td>
            <td>{pct(p.mem)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
