import { useCallback, useState } from 'react'

// useLocalPref is useState that survives reloads. localStorage can be missing,
// full or blocked (private windows), so every access is guarded and the value
// simply lives in memory when it cannot be saved.
export function useLocalPref<T>(key: string, initial: T): [T, (v: T | ((prev: T) => T)) => void] {
  const [value, setValue] = useState<T>(() => {
    try {
      const raw = localStorage.getItem(key)
      return raw === null ? initial : (JSON.parse(raw) as T)
    } catch {
      return initial
    }
  })
  const set = useCallback(
    (next: T | ((prev: T) => T)) => {
      setValue((prev) => {
        const v = typeof next === 'function' ? (next as (p: T) => T)(prev) : next
        try {
          localStorage.setItem(key, JSON.stringify(v))
        } catch {
          // Keep working without persistence.
        }
        return v
      })
    },
    [key],
  )
  return [value, set]
}
