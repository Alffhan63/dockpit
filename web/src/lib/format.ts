export function formatBytes(n: number): string {
  if (!n) return '0 B'
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  const i = Math.min(Math.floor(Math.log(n) / Math.log(1024)), units.length - 1)
  const v = n / 1024 ** i
  return `${v >= 100 || i === 0 ? v.toFixed(0) : v.toFixed(1)} ${units[i]}`
}

export function percent(used: number, total: number): number {
  return total > 0 ? (used / total) * 100 : 0
}

export function formatPercent(p: number | undefined): string {
  if (p === undefined) return '—'
  return `${p < 10 ? p.toFixed(1) : p.toFixed(0)}%`
}

export function formatUptime(seconds: number): string {
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (d > 0) return `${d}d ${h}h`
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })

export function timeAgo(date: Date | string | number): string {
  const t = typeof date === 'number' ? date * 1000 : new Date(date).getTime()
  const s = Math.round((t - Date.now()) / 1000)
  const abs = Math.abs(s)
  if (abs < 60) return rtf.format(s, 'second')
  if (abs < 3600) return rtf.format(Math.round(s / 60), 'minute')
  if (abs < 86400) return rtf.format(Math.round(s / 3600), 'hour')
  if (abs < 86400 * 30) return rtf.format(Math.round(s / 86400), 'day')
  return new Date(t).toLocaleDateString()
}

export function osLabel(os: string): string {
  return ({ darwin: 'macOS', linux: 'Linux' } as Record<string, string>)[os] ?? (os || 'unknown')
}
