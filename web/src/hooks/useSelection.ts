import { useCallback, useMemo, useState } from 'react'

// useSelection tracks selected IDs, limited to the ones currently listed so
// items that disappear (removed, filtered out) never stay selected.
export function useSelection(visibleIds: string[]) {
  const [picked, setPicked] = useState<Set<string>>(new Set())
  const selected = useMemo(() => new Set(visibleIds.filter((id) => picked.has(id))), [visibleIds, picked])

  const toggle = useCallback((id: string, on: boolean) => {
    setPicked((prev) => {
      const next = new Set(prev)
      if (on) next.add(id)
      else next.delete(id)
      return next
    })
  }, [])
  const set = useCallback((ids: string[]) => setPicked(new Set(ids)), [])
  const clear = useCallback(() => setPicked(new Set()), [])
  return { selected, toggle, set, clear }
}
