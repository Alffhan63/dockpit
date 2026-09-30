import { useEffect } from 'react'
import { Bell, CheckCircle2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { usePoll } from '@/hooks/usePoll'
import { api } from '@/lib/api'
import { issuesOf } from './OverviewPanel'
import { StatusBadge } from './StatusBadge'

const TITLE = 'Docker Cockpit'

// AlertsBell lives in the header on every page: a red dot and a count when
// something needs attention, and the list one tap away. The count also goes
// into the tab title and, in an installed app, onto the app icon.
export function AlertsBell() {
  const overview = usePoll(api.overview, 15_000, 'alerts-bell')
  const issues = overview.data ? issuesOf(overview.data) : []
  const count = issues.length

  useEffect(() => {
    document.title = count > 0 ? `(${count}) ${TITLE}` : TITLE
    const nav = navigator as Navigator & { setAppBadge?: (n?: number) => Promise<void>; clearAppBadge?: () => Promise<void> }
    if (count > 0) nav.setAppBadge?.(count).catch(() => {})
    else nav.clearAppBadge?.().catch(() => {})
  }, [count])

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="relative"
          aria-label={count > 0 ? `Alerts: ${count} need attention` : 'Alerts: all systems normal'}
          title="Alerts"
        >
          <Bell />
          {count > 0 && (
            <span
              aria-hidden
              className="absolute -top-0.5 -right-0.5 flex min-w-4 items-center justify-center rounded-full bg-destructive px-1 text-[10px] leading-4 font-semibold text-white tabular-nums"
            >
              {count > 9 ? '9+' : count}
            </span>
          )}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-80 max-w-[calc(100vw-2rem)]">
        <DropdownMenuLabel>{count > 0 ? `${count} need attention` : 'Alerts'}</DropdownMenuLabel>
        {count === 0 ? (
          <div className="flex items-center gap-2 px-2 py-3 text-sm text-muted-foreground">
            <CheckCircle2 className="size-4 text-success" aria-hidden /> All systems normal.
          </div>
        ) : (
          issues.slice(0, 8).map((i) => (
            <DropdownMenuItem key={i.key} asChild>
              <a href={i.to} className="flex flex-wrap items-center gap-x-2 gap-y-1">
                <StatusBadge tone={i.tone}>{i.label}</StatusBadge>
                <span className="min-w-0 truncate">{i.title}</span>
                {!i.title.startsWith(i.host.name) && <span className="ml-auto text-xs text-muted-foreground">{i.host.name}</span>}
              </a>
            </DropdownMenuItem>
          ))
        )}
        {count > 8 && <div className="px-2 py-1 text-xs text-muted-foreground">and {count - 8} more on the dashboard</div>}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
