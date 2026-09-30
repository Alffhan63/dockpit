import { useEffect, useSyncExternalStore } from 'react'

export type Theme = 'light' | 'dark' | 'system'
const KEY = 'cockpit-theme'

function load(): Theme {
  try {
    const v = localStorage.getItem(KEY)
    if (v === 'light' || v === 'dark') return v
  } catch {
    // storage unavailable (private mode): fall back to system
  }
  return 'system'
}

// One shared theme for every component that asks for it.
let current: Theme = load()
const listeners = new Set<() => void>()
const mq = matchMedia('(prefers-color-scheme: dark)')

function resolve(t: Theme): 'light' | 'dark' {
  return t === 'system' ? (mq.matches ? 'dark' : 'light') : t
}

function apply() {
  const dark = resolve(current) === 'dark'
  document.documentElement.classList.toggle('dark', dark)
  // Status bar color of the installed app follows the theme.
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', dark ? '#0B0F14' : '#F6F8FA')
  listeners.forEach((l) => l())
}

function setTheme(t: Theme) {
  current = t
  try {
    localStorage.setItem(KEY, t)
  } catch {
    // ignore
  }
  apply()
}

function subscribe(l: () => void) {
  listeners.add(l)
  return () => listeners.delete(l)
}

export function useTheme() {
  const theme = useSyncExternalStore(subscribe, () => current)
  const resolved = useSyncExternalStore(subscribe, () => resolve(current))
  useEffect(() => {
    mq.addEventListener('change', apply)
    return () => mq.removeEventListener('change', apply)
  }, [])
  return { theme, resolved, setTheme }
}
