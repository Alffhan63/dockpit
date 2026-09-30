import { useCallback, useEffect, useRef, useState } from 'react'
import { api, type LogLine } from '@/lib/api'

export type StreamStatus = 'connecting' | 'live' | 'ended' | 'error'

const MAX_LINES = 5000

export interface Line extends LogLine {
  id: number
  // Which container the line came from; set when following several at once.
  src?: string
}

export interface LogTarget {
  ref: string
  label: string
}

// useLogStream follows the logs of one or more containers over WebSocket,
// keeping the last MAX_LINES lines. With several targets the lines are merged
// by Docker's timestamps.
export function useLogStream(hostId: string, targets: LogTarget[], tail: number) {
  const [lines, setLines] = useState<Line[]>([])
  const [status, setStatus] = useState<StreamStatus>('connecting')
  const [error, setError] = useState<string>()
  const [generation, setGeneration] = useState(0)
  const nextId = useRef(0)
  const key = targets.map((t) => t.ref).join(',')

  useEffect(() => {
    setLines([])
    setStatus('connecting')
    setError(undefined)
    if (targets.length === 0) return
    const multi = targets.length > 1
    const states: StreamStatus[] = targets.map(() => 'connecting')
    const errors: (string | undefined)[] = targets.map(() => undefined)
    let closing = false

    const publish = () => {
      if (states.some((s) => s === 'live')) setStatus('live')
      else if (states.every((s) => s === 'connecting')) setStatus('connecting')
      else if (states.every((s) => s === 'error')) setStatus('error')
      else if (states.some((s) => s === 'connecting')) setStatus('connecting')
      else setStatus('ended')
      setError(errors.find(Boolean))
    }

    const sockets = targets.map((target, i) => {
      const ws = new WebSocket(api.logsURL(hostId, target.ref, tail))
      let finished = false
      ws.onopen = () => {
        states[i] = 'live'
        publish()
      }
      ws.onmessage = (msg) => {
        const ev = JSON.parse(msg.data) as { type: string; lines?: LogLine[]; error?: string }
        if (ev.type === 'lines' && ev.lines) {
          const incoming = ev.lines.map((l) => ({ ...l, id: nextId.current++, src: multi ? target.label : undefined }))
          setLines((prev) => {
            let next = prev.concat(incoming)
            if (multi) next = next.sort((a, b) => (a.t && b.t ? (a.t < b.t ? -1 : a.t > b.t ? 1 : 0) : a.id - b.id))
            return next.length > MAX_LINES ? next.slice(next.length - MAX_LINES) : next
          })
        } else if (ev.type === 'end') {
          finished = true
          states[i] = 'ended'
          publish()
        } else if (ev.type === 'error') {
          finished = true
          states[i] = 'error'
          errors[i] = multi ? `${target.label}: ${ev.error}` : ev.error
          publish()
        }
      }
      ws.onclose = () => {
        if (!finished && !closing) {
          states[i] = 'error'
          errors[i] ??= 'connection to controller lost'
          publish()
        }
      }
      return ws
    })
    return () => {
      closing = true
      sockets.forEach((ws) => ws.close())
    }
    // targets is derived from key
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hostId, key, tail, generation])

  const reconnect = useCallback(() => setGeneration((g) => g + 1), [])
  const clear = useCallback(() => setLines([]), [])
  return { lines, status, error, reconnect, clear }
}
