import { Server } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { AddHostDialog } from '@/components/AddHostDialog'
import { HostCard } from '@/components/HostCard'
import { PullToRefresh } from '@/components/PullToRefresh'
import { usePoll } from '@/hooks/usePoll'
import { api } from '@/lib/api'

export function DashboardPage() {
  const hosts = usePoll(api.hosts, 5000, 'hosts')
  const list = hosts.data ?? []
  const online = list.filter((h) => h.online).length

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
          <AddHostDialog onAdded={hosts.refresh} />
        </div>

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
            {list.map((h) => (
              <HostCard key={h.id} host={h} />
            ))}
          </div>
        )}
      </div>
    </PullToRefresh>
  )
}
