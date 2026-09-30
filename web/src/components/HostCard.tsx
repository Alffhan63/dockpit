import { Boxes, Clock } from 'lucide-react'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { usePoll } from '@/hooks/usePoll'
import { href } from '@/hooks/useRoute'
import { api, type Host } from '@/lib/api'
import { formatBytes, formatUptime, osLabel, percent, timeAgo } from '@/lib/format'
import { MetricBar } from './MetricBar'
import { HostStatusBadge } from './StatusBadge'

export function HostCard({ host }: { host: Host }) {
  const status = usePoll(() => api.hostStatus(host.id), 5000, host.online ? host.id : null)
  const m = status.data?.metrics
  const d = status.data?.docker

  return (
    <a href={href({ page: 'host', hostId: host.id })} className="group block rounded-xl focus-visible:outline-none">
      <Card className="h-full transition-colors group-hover:ring-primary/40 group-focus-visible:ring-2 group-focus-visible:ring-ring">
        <CardHeader>
          <div className="flex items-start justify-between gap-2">
            <CardTitle className="truncate">{host.name}</CardTitle>
            <HostStatusBadge online={host.online} />
          </div>
          <CardDescription>
            {host.info.os ? `${osLabel(host.info.os)} / ${host.info.arch}` : 'Agent never connected'}
            {host.info.docker_version && ` · Docker ${host.info.docker_version}`}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {!host.online ? (
            <p className="text-sm text-muted-foreground">
              {host.last_seen_at ? `Last seen ${timeAgo(host.last_seen_at)}` : 'Waiting for the agent to connect.'}
            </p>
          ) : !m || m.sampled_at === 0 ? (
            <>
              <Skeleton className="h-6 w-full" />
              <Skeleton className="h-6 w-full" />
              <Skeleton className="h-6 w-full" />
            </>
          ) : (
            <>
              <MetricBar label="CPU" value={m.cpu_percent} detail={`${m.cpu_cores} cores`} />
              <MetricBar
                label="Memory"
                value={percent(m.mem_used, m.mem_total)}
                detail={`${formatBytes(m.mem_used)} / ${formatBytes(m.mem_total)}`}
              />
              {m.disk_total > 0 && (
                <MetricBar
                  label="Disk"
                  value={percent(m.disk_used, m.disk_total)}
                  detail={`${formatBytes(m.disk_used)} / ${formatBytes(m.disk_total)}`}
                />
              )}
            </>
          )}
        </CardContent>
        {host.online && d && m && (
          <CardFooter className="gap-4 text-sm text-muted-foreground">
            <span className="flex items-center gap-1.5">
              <Boxes className="size-4" />
              <span className="font-medium text-foreground tabular-nums">{d.running}</span> / {d.containers} running
            </span>
            <span className="ml-auto flex items-center gap-1.5">
              <Clock className="size-4" /> up {formatUptime(m.uptime_seconds)}
            </span>
          </CardFooter>
        )}
      </Card>
    </a>
  )
}
