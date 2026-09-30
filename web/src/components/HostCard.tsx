import { Boxes, Clock, Eye, EyeOff, Star } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { usePoll } from '@/hooks/usePoll'
import { href } from '@/hooks/useRoute'
import { api, type Host, type HostOverview } from '@/lib/api'
import { formatBytes, formatUptime, osLabel, percent, timeAgo } from '@/lib/format'
import { cn } from '@/lib/utils'
import { isBehind } from '@/lib/version'
import { IpList } from './IpList'
import { MetricBar } from './MetricBar'
import { Sparkline } from './Sparkline'
import { HostStatusBadge, StatusBadge } from './StatusBadge'

export function HostCard({
  host,
  overview,
  pinned,
  isHidden,
  onTogglePin,
  onHide,
}: {
  host: Host
  overview?: HostOverview
  pinned?: boolean
  isHidden?: boolean
  onTogglePin?: () => void
  onHide?: () => void
}) {
  const status = usePoll(() => api.hostStatus(host.id), 5000, host.online ? host.id : null)
  const m = status.data?.metrics
  const d = status.data?.docker

  return (
    // The controls sit next to the link, not inside it: interactive elements
    // cannot be nested in an anchor.
    <div className="group/host relative">
      <div
        className={cn(
          'absolute -top-2 -right-2 z-10 flex gap-1 opacity-0 transition-opacity group-focus-within/host:opacity-100 group-hover/host:opacity-100 pointer-coarse:opacity-100',
          pinned && 'opacity-100',
        )}
      >
        {onTogglePin && (
          <Button
            variant="secondary"
            size="icon-sm"
            className="rounded-full shadow"
            aria-label={pinned ? `Unpin ${host.name}` : `Pin ${host.name} to the top`}
            aria-pressed={pinned}
            title={pinned ? 'Unpin' : 'Pin to top'}
            onClick={onTogglePin}
          >
            <Star className={cn(pinned && 'fill-current text-warning')} />
          </Button>
        )}
        {onHide && (
          <Button
            variant="secondary"
            size="icon-sm"
            className="rounded-full shadow"
            aria-label={isHidden ? `Show ${host.name} again` : `Hide ${host.name}`}
            title={isHidden ? 'Show on the dashboard again' : 'Hide from the dashboard'}
            onClick={onHide}
          >
            {isHidden ? <Eye /> : <EyeOff />}
          </Button>
        )}
      </div>
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
            {host.info.agent_version && (
              <span className="mt-0.5 flex flex-wrap items-center gap-x-1.5 gap-y-1">
                <span>Agent {host.info.agent_version}</span>
                {isBehind(host.info.agent_version, host.controller_version) && (
                  <StatusBadge tone="amber">update to {host.controller_version}</StatusBadge>
                )}
              </span>
            )}
          </CardDescription>
          <IpList
            className="pt-1"
            publicIP={host.public_ip}
            addresses={status.data?.addresses ?? host.info.addresses}
            limit={2}
          />
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
              <div className="flex items-end gap-3">
                <div className="min-w-0 flex-1">
                  <MetricBar label="CPU" value={m.cpu_percent} detail={`${m.cpu_cores} cores`} />
                </div>
                <Sparkline values={overview?.spark ?? []} width={64} height={24} />
              </div>
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
            {overview && overview.problems.length > 0 && (
              <StatusBadge tone="red">
                {overview.problems.length} issue{overview.problems.length === 1 ? '' : 's'}
              </StatusBadge>
            )}
            <span className="ml-auto flex items-center gap-1.5">
              <Clock className="size-4" /> up {formatUptime(m.uptime_seconds)}
            </span>
          </CardFooter>
        )}
      </Card>
      </a>
    </div>
  )
}
