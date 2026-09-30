import { useCallback, useEffect, useRef, useState } from 'react'

interface PollState<T> {
  data?: T
  error?: Error
  loading: boolean
  refresh: () => Promise<void>
}

// usePoll calls fn now and every intervalMs while the tab is visible.
// key identifies what is being fetched: changing it resets the state,
// null pauses polling.
export function usePoll<T>(fn: () => Promise<T>, intervalMs: number, key: string | null): PollState<T> {
  const [data, setData] = useState<T>()
  const [error, setError] = useState<Error>()
  const [loading, setLoading] = useState(true)
  const fnRef = useRef(fn)
  fnRef.current = fn
  const keyRef = useRef(key)
  keyRef.current = key

  const refresh = useCallback(async () => {
    if (keyRef.current === null) return
    try {
      setData(await fnRef.current())
      setError(undefined)
    } catch (e) {
      setError(e as Error)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    setData(undefined)
    setError(undefined)
    setLoading(key !== null)
    if (key === null) return
    refresh()
    const id = setInterval(() => {
      if (document.visibilityState === 'visible') refresh()
    }, intervalMs)
    return () => clearInterval(id)
  }, [key, intervalMs, refresh])

  return { data, error, loading, refresh }
}
