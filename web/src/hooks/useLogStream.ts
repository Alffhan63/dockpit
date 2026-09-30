import { useCallback, useEffect, useRef, useState } from 'react'
import { api, type LogLine } from '@/lib/api'

export type StreamStatus = 'connecting' | 'live' | 'ended' | 'error'

const MAX_LINES = 5000

export interface Line extends LogLine {
  id: number
}

// useLogStream follows a container's logs over WebSocket, keeping the last
// MAX_LINES lines.
export function useLogStream(hostId: string, container: string, tail: number) {
  const [lines, setLines] = useState<Line[]>([])
  const [status, setStatus] = useState<StreamStatus>('connecting')
  const [error, setError] = useState<string>()
  const [generation, setGeneration] = useState(0)
  const nextId = useRef(0)

  useEffect(() => {
    setLines([])
    setStatus('connecting')
    setError(undefined)
    const ws = new WebSocket(api.logsURL(hostId, container, tail))
    let finished = false

    ws.onopen = () => setStatus('live')
    ws.onmessage = (msg) => {
      const ev = JSON.parse(msg.data) as { type: string; lines?: LogLine[]; error?: string }
      if (ev.type === 'lines' && ev.lines) {
        const incoming = ev.lines.map((l) => ({ ...l, id: nextId.current++ }))
        setLines((prev) => {
          const next = prev.concat(incoming)
          return next.length > MAX_LINES ? next.slice(next.length - MAX_LINES) : next
        })
      } else if (ev.type === 'end') {
        finished = true
        setStatus('ended')
      } else if (ev.type === 'error') {
        finished = true
        setStatus('error')
        setError(ev.error)
      }
    }
    ws.onclose = () => {
      if (!finished) {
        setStatus('error')
        setError((e) => e ?? 'connection to controller lost')
      }
    }
    return () => {
      finished = true
      ws.close()
    }
  }, [hostId, container, tail, generation])

  const reconnect = useCallback(() => setGeneration((g) => g + 1), [])
  const clear = useCallback(() => setLines([]), [])
  return { lines, status, error, reconnect, clear }
}
