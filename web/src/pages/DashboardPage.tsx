import { useMemo, useState } from 'react'
import { Eye, Server } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { AddHostDialog } from '@/components/AddHostDialog'
import { HostCard } from '@/components/HostCard'
import { OverviewPanel } from '@/components/OverviewPanel'
import { PullToRefresh } from '@/components/PullToRefresh'
import { useLocalPref } from '@/hooks/useLocalPref'
import { usePoll } from '@/hooks/usePoll'
import { api } from '@/lib/api'

interface HostPrefs {
  pinned: string[]
  hidden: string[]
}

export function DashboardPage() {
  const hosts = usePoll(api.hosts, 5000, 'hosts')
  const overview = usePoll(api.overview, 10_000, 'overview')
  const [prefs, setPrefs] = useLocalPref<HostPrefs>('cockpit:host-prefs', { pinned: [], hidden: [] })
  const [showHidden, setShowHidden] = useState(false)
  const list = hosts.data ?? []
  const online = list.filter((h) => h.online).length

  // Pinned first, then hosts that need attention (offline or with problems),
  // then by name. Hidden hosts leave the grid but still count in the alerts.
  const { shown, hiddenCount } = useMemo(() => {
    const byId = new Map((overview.data ?? []).map((o) => [o.id, o]))
    const rank = (id: string, isOnline: boolean) => {
      if (prefs.pinned.includes(id)) return 0
      const o = byId.get(id)
      return !isOnline || (o?.problems.length ?? 0) > 0 ? 1 : 2
    }
    const sorted = [...list].sort(
      (a, b) => rank(a.id, a.online) - rank(b.id, b.online) || a.name.localeCompare(b.name),
    )
    const hidden = sorted.filter((h) => prefs.hidden.includes(h.id))
    return { shown: showHidden ? sorted : sorted.filter((h) => !prefs.hidden.includes(h.id)), hiddenCount: hidden.length }
  }, [list, overview.data, prefs, showHidden])

  const toggle = (key: keyof HostPrefs, id: string) =>
    setPrefs((p) => ({ ...p, [key]: p[key].includes(id) ? p[key].filter((x) => x !== id) : [...p[key], id] }))

  return (
    <PullToRefresh onRefresh={hosts.refresh}>
      <div className="space-y-6">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <h1 className="text-2xl font-semibold tracking-tight">Hosts</h1>
            <p className="text-sm text-muted-foreground">
              {hosts.data ? `${online} online · ${list.length - online} offline` : 'Loading…'}
            </p>
          </div>
          <div className="flex items-center gap-2">
            {hiddenCount > 0 && (
              <Button variant="ghost" size="sm" aria-pressed={showHidden} onClick={() => setShowHidden((v) => !v)}>
                <Eye /> {showHidden ? 'Hide' : 'Show'} {hiddenCount} hidden
              </Button>
            )}
            <AddHostDialog onAdded={hosts.refresh} />
          </div>
        </div>

        <OverviewPanel hosts={overview.data} loading={overview.loading && !overview.error} />

        {hosts.error && (
          <Alert variant="destructive">
            <AlertDescription>Could not load hosts: {hosts.error.message}</AlertDescription>
          </Alert>
        )}

        {hosts.loading ? (
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {[0, 1, 2].map((i) => (
              <Skeleton key={i} className="h-56 rounded-xl" />
            ))}
          </div>
        ) : list.length === 0 ? (
          <Card>
            <CardContent className="flex flex-col items-center gap-3 py-12 text-center">
              <Server className="size-10 text-muted-foreground" />
              <div>
                <p className="font-medium">No hosts yet</p>
                <p className="text-sm text-muted-foreground">
                  Add a host, then start its agent with the token you get.
                </p>
              </div>
              <AddHostDialog onAdded={hosts.refresh} />
            </CardContent>
          </Card>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {shown.map((h) => (
              <div key={h.id} className={prefs.hidden.includes(h.id) ? 'opacity-50' : undefined}>
                <HostCard
                  host={h}
                  overview={overview.data?.find((o) => o.id === h.id)}
                  pinned={prefs.pinned.includes(h.id)}
                  isHidden={prefs.hidden.includes(h.id)}
                  onTogglePin={() => toggle('pinned', h.id)}
                  onHide={() => toggle('hidden', h.id)}
                />
              </div>
            ))}
            {shown.length === 0 && (
              <p className="col-span-full py-8 text-center text-sm text-muted-foreground">
                All hosts are hidden. Use “Show hidden” above to bring them back.
              </p>
            )}
          </div>
        )}
      </div>
    </PullToRefresh>
  )
}
