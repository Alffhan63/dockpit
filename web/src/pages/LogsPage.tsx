import { useLayoutEffect, useMemo, useRef, useState } from 'react'
import {
  ArrowDown,
  ArrowLeft,
  ChevronDown,
  Download,
  Eraser,
  Pause,
  Play,
  RefreshCw,
  Regex,
  Search,
  TriangleAlert,
} from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { READABLE_LOG_DRIVERS } from '@/components/ContainerTable'
import { StatusBadge, type Tone } from '@/components/StatusBadge'
import { useLogStream, type Line, type LogTarget, type StreamStatus } from '@/hooks/useLogStream'
import { usePoll } from '@/hooks/usePoll'
import { href } from '@/hooks/useRoute'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'

const statusView: Record<StreamStatus, { tone: Tone; label: string }> = {
  connecting: { tone: 'blue', label: 'Connecting' },
  live: { tone: 'green', label: 'Live' },
  ended: { tone: 'gray', label: 'Ended' },
  error: { tone: 'red', label: 'Disconnected' },
}

const ERROR_RE = /\b(error|err|fatal|panic|exception|critical|traceback|failed|failure)\b/i
const WARN_RE = /\b(warn|warning|deprecated)\b/i
type Level = 'error' | 'warn' | 'info'
const levelOf = (l: Line): Level => (l.s === 'stderr' || ERROR_RE.test(l.m) ? 'error' : WARN_RE.test(l.m) ? 'warn' : 'info')

// A few readable tints to tell containers apart in a merged project log.
const SRC_COLORS = ['text-sky-400', 'text-emerald-400', 'text-violet-400', 'text-amber-400', 'text-pink-400', 'text-cyan-400']

export function LogsPage({ hostId, container, project }: { hostId: string; container?: string; project?: string }) {
  const [tail, setTail] = useState(500)
  const [follow, setFollow] = useState(true)
  const [timestamps, setTimestamps] = useState(false)
  const [query, setQuery] = useState('')
  const [regex, setRegex] = useState(false)
  const [cursor, setCursor] = useState<number>()

  const host = usePoll(() => api.host(hostId), 30000, hostId)
  // Resolve names (the route carries IDs) and, for a project, its containers.
  const containers = usePoll(() => api.containers(hostId), 30000, host.data?.online ? hostId : null)

  const targets: LogTarget[] = useMemo(() => {
    if (!project) return container ? [{ ref: container, label: container.slice(0, 12) }] : []
    return (containers.data ?? [])
      .filter((c) => c.compose_project === project)
      .map((c) => ({ ref: c.id, label: c.compose_service || c.name }))
  }, [project, container, containers.data])

  const stream = useLogStream(hostId, targets, tail)
  const single = !project ? containers.data?.find((c) => c.id === container || c.name === container) : undefined
  const title = project ? project : (single?.name ?? container?.slice(0, 12) ?? '')
  const colorOf = useMemo(() => {
    const m = new Map(targets.map((t, i) => [t.label, SRC_COLORS[i % SRC_COLORS.length]]))
    return (label?: string) => (label ? m.get(label) : undefined)
  }, [targets])

  const scroller = useRef<HTMLDivElement>(null)

  const matcher = useMemo(() => {
    const q = query.trim()
    if (!q) return { test: undefined as ((s: string) => boolean) | undefined }
    if (regex) {
      try {
        const re = new RegExp(q, 'i')
        return { test: (s: string) => re.test(s) }
      } catch (e) {
        return { test: undefined, error: (e as Error).message }
      }
    }
    const lower = q.toLowerCase()
    return { test: (s: string) => s.toLowerCase().includes(lower) }
  }, [query, regex])

  const visible = useMemo(
    () => (matcher.test ? stream.lines.filter((l) => matcher.test!(l.m)) : stream.lines),
    [stream.lines, matcher],
  )

  useLayoutEffect(() => {
    const el = scroller.current
    if (follow && el) el.scrollTop = el.scrollHeight
  }, [visible, follow])

  // Scrolling up pauses following; scrolling back to the bottom resumes it.
  const onScroll = () => {
    const el = scroller.current
    if (!el) return
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40
    if (atBottom !== follow) setFollow(atBottom)
  }

  const errorCount = useMemo(() => visible.reduce((n, l) => n + (levelOf(l) === 'error' ? 1 : 0), 0), [visible])
  const jumpToError = (dir: 1 | -1) => {
    const errs = visible.filter((l) => levelOf(l) === 'error')
    if (errs.length === 0) return
    let next: Line | undefined
    if (cursor === undefined) next = dir === 1 ? errs[0] : errs.at(-1)
    else if (dir === 1) next = errs.find((l) => l.id > cursor) ?? errs[0]
    else next = [...errs].reverse().find((l) => l.id < cursor) ?? errs.at(-1)
    if (!next) return
    setFollow(false)
    setCursor(next.id)
    requestAnimationFrame(() =>
      scroller.current?.querySelector(`[data-line="${next.id}"]`)?.scrollIntoView({ block: 'center' }),
    )
  }

  const text = () =>
    visible
      .map((l) => `${l.t ? l.t + ' ' : ''}${l.src ? `[${l.src}] ` : ''}${l.m}`)
      .join('\n')
  const download = () => {
    const url = URL.createObjectURL(new Blob([text() + '\n'], { type: 'text/plain' }))
    const a = document.createElement('a')
    a.href = url
    a.download = `${title || 'container'}-logs.txt`
    a.click()
    URL.revokeObjectURL(url)
  }

  const s = statusView[stream.status]
  const driver = single?.log_driver
  const unreadable = driver && !READABLE_LOG_DRIVERS.includes(driver)
  const empty = visible.length === 0 && stream.status !== 'connecting'

  return (
    <div className="flex h-full flex-col gap-3 sm:gap-4">
      <div className="flex flex-wrap items-center gap-3">
        <Button variant="ghost" size="sm" className="-ml-2" asChild>
          <a href={href({ page: 'host', hostId, tab: project ? 'projects' : undefined })}>
            <ArrowLeft /> {host.data?.name ?? hostId}
          </a>
        </Button>
        <h1 className="truncate font-mono text-lg font-semibold">
          {title}
          {project && <span className="ml-2 font-sans text-sm font-normal text-muted-foreground">{targets.length} containers</span>}
        </h1>
        <StatusBadge tone={s.tone} pulse={stream.status === 'live'}>
          {s.label}
        </StatusBadge>
        {stream.error && <span className="text-sm text-destructive">{stream.error}</span>}
        {(stream.status === 'error' || stream.status === 'ended') && (
          <Button variant="outline" size="sm" onClick={stream.reconnect}>
            <RefreshCw /> Reconnect
          </Button>
        )}
      </div>

      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <div className="relative w-full max-w-xs">
          <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            aria-label={regex ? 'Filter lines by regular expression' : 'Filter lines'}
            aria-invalid={!!matcher.error}
            placeholder={regex ? 'Regular expression…' : 'Filter lines…'}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className={cn('pr-9 pl-8', regex && 'font-mono')}
          />
          <Button
            variant={regex ? 'secondary' : 'ghost'}
            size="icon-sm"
            className="absolute top-1/2 right-1 -translate-y-1/2"
            aria-label="Use regular expression"
            aria-pressed={regex}
            title="Regular expression"
            onClick={() => setRegex((r) => !r)}
          >
            <Regex />
          </Button>
        </div>
        <div className="flex items-center gap-2">
          <Label className="text-muted-foreground">Tail</Label>
          <Select value={String(tail)} onValueChange={(v) => setTail(Number(v))}>
            <SelectTrigger size="sm" className="w-24" aria-label="Number of past lines">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {[100, 500, 1000, 5000].map((n) => (
                <SelectItem key={n} value={String(n)}>
                  {n}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex items-center gap-2">
          <Switch id="ts" checked={timestamps} onCheckedChange={setTimestamps} />
          <Label htmlFor="ts">Timestamps</Label>
        </div>
        <div className="ml-auto flex flex-wrap items-center gap-1">
          <Button variant="ghost" size="sm" disabled={errorCount === 0} onClick={() => jumpToError(1)} title="Jump to the next error line">
            <TriangleAlert /> Next error{errorCount > 0 && <span className="tabular-nums">({errorCount})</span>}
          </Button>
          <Button variant="ghost" size="icon-sm" disabled={errorCount === 0} onClick={() => jumpToError(-1)} aria-label="Previous error">
            <ChevronDown className="rotate-180" />
          </Button>
          <Button variant="ghost" size="sm" onClick={() => setFollow((f) => !f)}>
            {follow ? <Pause /> : <Play />} {follow ? 'Pause' : 'Follow'}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            disabled={visible.length === 0}
            onClick={() => navigator.clipboard.writeText(text()).then(() => toast.success('Copied'), () => toast.error('Could not copy'))}
          >
            Copy
          </Button>
          <Button variant="ghost" size="sm" disabled={visible.length === 0} onClick={download}>
            <Download /> Download
          </Button>
          <Button variant="ghost" size="sm" onClick={stream.clear}>
            <Eraser /> Clear
          </Button>
        </div>
      </div>
      {matcher.error && (
        <p role="alert" className="-mt-2 text-xs text-destructive">
          Invalid expression: {matcher.error}
        </p>
      )}

      <div className="relative min-h-0 flex-1">
        <div
          ref={scroller}
          onScroll={onScroll}
          role="log"
          aria-label={`Logs of ${title}`}
          tabIndex={0}
          className="h-full overflow-auto rounded-lg border border-[#263241] bg-[#0B0F14] py-2 font-mono text-xs leading-5 text-[#E5E7EB] focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
        >
          {visible.length === 0 ? (
            <div className="space-y-2 px-4 py-2 text-[#8B98A8]">
              <p>{stream.status === 'connecting' ? 'Connecting…' : query ? 'No lines match.' : 'No output yet.'}</p>
              {empty && !query && unreadable && (
                <p className="text-[#FCD34D]">
                  This container uses the “{driver}” logging driver, which Docker cannot read back, so there is nothing to show
                  here. Use json-file, local or journald to see logs in the dashboard.
                </p>
              )}
              {empty && !query && !unreadable && (
                <p>
                  The container has written nothing to stdout or stderr. If the app logs to a file inside the container, it will not
                  appear here.
                </p>
              )}
            </div>
          ) : (
            visible.map((l) => {
              const level = levelOf(l)
              return (
                <div
                  key={l.id}
                  data-line={l.id}
                  className={cn(
                    'border-l-2 px-3 break-all whitespace-pre-wrap hover:bg-white/5',
                    level === 'error' && 'border-[#EF4444]/70 text-[#FCA5A5]',
                    level === 'warn' && 'border-[#F59E0B]/70 text-[#FCD34D]',
                    level === 'info' && 'border-transparent',
                    cursor === l.id && 'bg-white/10 ring-1 ring-[#EF4444]/60 ring-inset',
                  )}
                >
                  {timestamps && l.t && <span className="mr-3 text-[#8B98A8] select-none">{formatTime(l.t)}</span>}
                  {l.src && <span className={cn('mr-2 font-semibold select-none', colorOf(l.src))}>{l.src}</span>}
                  {l.m || ' '}
                </div>
              )
            })
          )}
        </div>
        {!follow && (
          <Button size="sm" className="absolute right-4 bottom-4 shadow-lg" onClick={() => setFollow(true)}>
            <ArrowDown /> Jump to latest
          </Button>
        )}
      </div>
      <div className="flex justify-between text-xs text-muted-foreground">
        <span>
          {visible.length.toLocaleString()} lines{query && ` matching “${query}”`} · keeps the last 5,000
        </span>
        <span className="flex items-center gap-3">
          <span className="flex items-center gap-1.5">
            <span className="size-2 rounded-sm bg-[#EF4444]" /> error / stderr
          </span>
          <span className="flex items-center gap-1.5">
            <span className="size-2 rounded-sm bg-[#F59E0B]" /> warning
          </span>
        </span>
      </div>
    </div>
  )
}

function formatTime(t: string): string {
  const d = new Date(t)
  return isNaN(d.getTime()) ? t : d.toLocaleTimeString(undefined, { hour12: false }) + '.' + String(d.getMilliseconds()).padStart(3, '0')
}
