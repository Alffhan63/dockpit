import { useLayoutEffect, useMemo, useRef, useState } from 'react'
import { ArrowDown, ArrowLeft, Eraser, Pause, Play, RefreshCw, Search } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { StatusBadge, type Tone } from '@/components/StatusBadge'
import { useLogStream, type StreamStatus } from '@/hooks/useLogStream'
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

export function LogsPage({ hostId, container }: { hostId: string; container: string }) {
  const [tail, setTail] = useState(500)
  const [follow, setFollow] = useState(true)
  const [timestamps, setTimestamps] = useState(false)
  const [query, setQuery] = useState('')
  const stream = useLogStream(hostId, container, tail)
  const host = usePoll(() => api.host(hostId), 30000, hostId)
  // Resolve the container's name for the title; the route carries its ID.
  const containers = usePoll(() => api.containers(hostId), 30000, host.data?.online ? hostId : null)
  const name = containers.data?.find((c) => c.id === container || c.name === container)?.name ?? container.slice(0, 12)

  const scroller = useRef<HTMLDivElement>(null)
  const visible = useMemo(() => {
    const q = query.trim().toLowerCase()
    return q ? stream.lines.filter((l) => l.m.toLowerCase().includes(q)) : stream.lines
  }, [stream.lines, query])

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

  const s = statusView[stream.status]

  return (
    <div className="flex h-full flex-col gap-3 sm:gap-4">
      <div className="flex flex-wrap items-center gap-3">
        <Button variant="ghost" size="sm" className="-ml-2" asChild>
          <a href={href({ page: 'host', hostId })}>
            <ArrowLeft /> {host.data?.name ?? hostId}
          </a>
        </Button>
        <h1 className="truncate font-mono text-lg font-semibold">{name}</h1>
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
          <Input placeholder="Filter lines…" value={query} onChange={(e) => setQuery(e.target.value)} className="pl-8" />
        </div>
        <div className="flex items-center gap-2">
          <Label className="text-muted-foreground">Tail</Label>
          <Select value={String(tail)} onValueChange={(v) => setTail(Number(v))}>
            <SelectTrigger size="sm" className="w-24">
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
        <div className="ml-auto flex items-center gap-1">
          <Button variant="ghost" size="sm" onClick={() => setFollow((f) => !f)}>
            {follow ? <Pause /> : <Play />} {follow ? 'Pause' : 'Follow'}
          </Button>
          <Button variant="ghost" size="sm" onClick={stream.clear}>
            <Eraser /> Clear
          </Button>
        </div>
      </div>

      <div className="relative min-h-0 flex-1">
        <div
          ref={scroller}
          onScroll={onScroll}
          className="h-full overflow-auto rounded-lg border border-[#263241] bg-[#0B0F14] py-2 font-mono text-xs leading-5 text-[#E5E7EB]"
        >
          {visible.length === 0 ? (
            <div className="px-4 py-2 text-[#8B98A8]">
              {stream.status === 'connecting' ? 'Connecting…' : query ? 'No lines match.' : 'No output yet.'}
            </div>
          ) : (
            visible.map((l) => (
              <div
                key={l.id}
                className={cn(
                  'border-l-2 px-3 break-all whitespace-pre-wrap hover:bg-white/5',
                  l.s === 'stderr' ? 'border-[#EF4444]/70 text-[#FCA5A5]' : 'border-transparent',
                )}
              >
                {timestamps && l.t && <span className="mr-3 text-[#8B98A8] select-none">{formatTime(l.t)}</span>}
                {l.m || ' '}
              </div>
            ))
          )}
        </div>
        {!follow && (
          <Button
            size="sm"
            className="absolute right-4 bottom-4 shadow-lg"
            onClick={() => setFollow(true)}
          >
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
            <span className="size-2 rounded-sm bg-[#8B98A8]" /> stdout
          </span>
          <span className="flex items-center gap-1.5">
            <span className="size-2 rounded-sm bg-destructive" /> stderr
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
