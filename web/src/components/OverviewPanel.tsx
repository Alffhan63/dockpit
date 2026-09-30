import { AlertTriangle, Boxes, CheckCircle2, Server } from 'lucide-react'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { href } from '@/hooks/useRoute'
import type { HostOverview, Problem } from '@/lib/api'
import { cn } from '@/lib/utils'
import { StatusBadge } from './StatusBadge'

const kindLabel: Record<Problem['kind'], string> = {
  unhealthy: 'Unhealthy',
  restarting: 'Restart loop',
  crashed: 'Crashed',
  oom: 'Out of memory',
}

interface Issue {
  key: string
  host: HostOverview
  title: string
  detail?: string
  tone: 'red' | 'amber' | 'gray'
  label: string
  to: string
}

// issuesOf turns the per-host summaries into one list, worst first.
function issuesOf(hosts: HostOverview[]): Issue[] {
  const out: Issue[] = []
  for (const h of hosts) {
    if (!h.online) {
      out.push({ key: `${h.id}/offline`, host: h, title: `${h.name} is offline`, tone: 'gray', label: 'Offline', to: href({ page: 'host', hostId: h.id }) })
      continue
    }
    for (const p of h.problems) {
      out.push({
        key: `${h.id}/${p.kind}/${p.id}`,
        host: h,
        title: p.container,
        detail: p.detail,
        tone: p.kind === 'restarting' ? 'amber' : 'red',
        label: kindLabel[p.kind],
        to: href({ page: 'logs', hostId: h.id, container: p.id }),
      })
    }
    if (h.disk >= 90) {
      out.push({ key: `${h.id}/disk`, host: h, title: `${h.name}: disk ${h.disk.toFixed(0)}% full`, tone: 'amber', label: 'Disk', to: href({ page: 'host', hostId: h.id, tab: 'cleanup' }) })
    }
    if (h.mem >= 90) {
      out.push({ key: `${h.id}/mem`, host: h, title: `${h.name}: memory ${h.mem.toFixed(0)}% used`, tone: 'amber', label: 'Memory', to: href({ page: 'host', hostId: h.id }) })
    }
  }
  const rank = { red: 0, amber: 1, gray: 2 }
  return out.sort((a, b) => rank[a.tone] - rank[b.tone])
}

function Tile({ icon: Icon, label, value, sub, tone }: { icon: typeof Server; label: string; value: string; sub?: string; tone?: 'ok' | 'bad' }) {
  return (
    <Card size="sm">
      <CardContent className="flex items-center gap-3">
        <span
          className={cn(
            'flex size-9 shrink-0 items-center justify-center rounded-lg',
            tone === 'bad' ? 'bg-destructive/15 text-destructive' : tone === 'ok' ? 'bg-success/15 text-success' : 'bg-muted text-muted-foreground',
          )}
        >
          <Icon className="size-5" aria-hidden />
        </span>
        <div className="min-w-0">
          <div className="text-2xl leading-none font-semibold tabular-nums">{value}</div>
          <div className="mt-1 truncate text-xs text-muted-foreground">
            {label}
            {sub && <span> · {sub}</span>}
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

// OverviewPanel is the at-a-glance strip on top of the dashboard: totals, and
// whatever needs attention listed first.
export function OverviewPanel({ hosts, loading }: { hosts?: HostOverview[]; loading: boolean }) {
  if (loading || !hosts) {
    return (
      <div className="grid gap-3 sm:grid-cols-3">
        {[0, 1, 2].map((i) => (
          <Skeleton key={i} className="h-[70px] rounded-xl" />
        ))}
      </div>
    )
  }
  if (hosts.length === 0) return null

  const online = hosts.filter((h) => h.online)
  const running = hosts.reduce((n, h) => n + h.running, 0)
  const total = hosts.reduce((n, h) => n + h.containers, 0)
  const issues = issuesOf(hosts)

  return (
    <section aria-label="Overview" className="space-y-3">
      <div className="grid gap-3 sm:grid-cols-3">
        <Tile icon={Server} label="Hosts online" value={`${online.length}/${hosts.length}`} tone={online.length === hosts.length ? 'ok' : 'bad'} />
        <Tile icon={Boxes} label="Containers running" value={String(running)} sub={`${total} total`} />
        <Tile
          icon={issues.length ? AlertTriangle : CheckCircle2}
          label={issues.length ? 'Need attention' : 'All systems normal'}
          value={String(issues.length)}
          tone={issues.length ? 'bad' : 'ok'}
        />
      </div>
      {issues.length > 0 && (
        <Card size="sm" className="border-destructive/30">
          <CardContent>
            <h2 className="mb-1 text-sm font-medium">Needs attention</h2>
            <ul className="divide-y" aria-live="polite">
              {issues.slice(0, 8).map((i) => (
                <li key={i.key}>
                  <a href={i.to} className="flex flex-wrap items-center gap-x-3 gap-y-1 py-2 text-sm hover:bg-muted/50 focus-visible:bg-muted/50 focus-visible:outline-none">
                    <StatusBadge tone={i.tone}>{i.label}</StatusBadge>
                    <span className="min-w-0 truncate font-medium">{i.title}</span>
                    {i.detail && <span className="text-xs text-muted-foreground">{i.detail}</span>}
                    {!i.title.startsWith(i.host.name) && <span className="ml-auto text-xs text-muted-foreground">{i.host.name}</span>}
                  </a>
                </li>
              ))}
            </ul>
            {issues.length > 8 && <p className="pt-1 text-xs text-muted-foreground">and {issues.length - 8} more</p>}
          </CardContent>
        </Card>
      )}
    </section>
  )
}
