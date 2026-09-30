import { Loader2 } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { usePoll } from '@/hooks/usePoll'
import { api, type Container } from '@/lib/api'
import { formatBytes } from '@/lib/format'
import { HealthBadge, StatusBadge, containerTone } from './StatusBadge'

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="grid grid-cols-[7rem_1fr] gap-3 py-1.5 text-sm">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="min-w-0 break-words">{children}</dd>
    </div>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="space-y-1">
      <h3 className="text-xs font-semibold tracking-wide text-muted-foreground uppercase">{title}</h3>
      {children}
    </section>
  )
}

const mono = 'font-mono text-xs'

// ContainerDetailDialog is a read-only inspect view. Secret-looking
// environment values are masked by the agent and never reach the browser.
export function ContainerDetailDialog({
  hostId,
  container,
  onOpenChange,
}: {
  hostId: string
  container?: Container
  onOpenChange: (open: boolean) => void
}) {
  const detail = usePoll(() => api.inspect(hostId, container!.id), 15000, container ? `${hostId}/${container.id}` : null)
  const d = detail.data

  return (
    <Dialog open={!!container} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="truncate font-mono">{container?.name}</DialogTitle>
          <DialogDescription className="truncate">{container?.image}</DialogDescription>
        </DialogHeader>
        {detail.error && (
          <Alert variant="destructive">
            <AlertDescription>{detail.error.message}</AlertDescription>
          </Alert>
        )}
        {!d && !detail.error && (
          <div className="flex items-center gap-2 py-6 text-sm text-muted-foreground" role="status">
            <Loader2 className="size-4 animate-spin" /> Loading…
          </div>
        )}
        {d && (
          <div className="space-y-5">
            <Section title="State">
              <dl className="divide-y">
                <Row label="Status">
                  <span className="flex flex-wrap items-center gap-2">
                    <StatusBadge tone={containerTone(d.state)}>{d.state}</StatusBadge>
                    <HealthBadge health={d.health} />
                    {d.oom_killed && <StatusBadge tone="red">out of memory</StatusBadge>}
                  </span>
                </Row>
                {d.health_output && (
                  <Row label="Healthcheck">
                    <pre className={`${mono} whitespace-pre-wrap`}>{d.health_output}</pre>
                  </Row>
                )}
                {d.state !== 'running' && <Row label="Exit code">{d.exit_code}</Row>}
                {d.error && <Row label="Error">{d.error}</Row>}
                <Row label="Restarts">
                  {d.restart_count}
                  {d.restart_policy && <span className="text-muted-foreground"> · policy {d.restart_policy}</span>}
                </Row>
                {d.started_at && <Row label="Started">{new Date(d.started_at).toLocaleString()}</Row>}
                {d.finished_at && d.state !== 'running' && (
                  <Row label="Finished">{new Date(d.finished_at).toLocaleString()}</Row>
                )}
                <Row label="Command">
                  <code className={mono}>{d.command || '—'}</code>
                </Row>
                <Row label="Limits">
                  {d.mem_limit ? `${formatBytes(d.mem_limit)} memory` : 'no memory limit'}
                  {d.cpus ? ` · ${d.cpus} CPUs` : ''}
                </Row>
                <Row label="Log driver">
                  <code className={mono}>{d.log_driver}</code>
                </Row>
              </dl>
            </Section>

            <Section title={`Environment (${d.env.length})`}>
              {d.env.length === 0 ? (
                <p className="text-sm text-muted-foreground">None.</p>
              ) : (
                <div className="max-h-56 overflow-auto rounded-md border">
                  <table className="w-full text-left text-xs">
                    <caption className="sr-only">Environment variables. Secret values are hidden.</caption>
                    <tbody className="divide-y">
                      {d.env.map((e) => (
                        <tr key={e.key}>
                          <th scope="row" className="w-2/5 px-2 py-1 align-top font-mono font-medium break-all">
                            {e.key}
                          </th>
                          <td
                            className={`px-2 py-1 font-mono break-all ${e.masked ? 'text-muted-foreground' : ''}`}
                            title={e.masked ? 'Hidden by the agent' : undefined}
                          >
                            {e.value}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
              <p className="text-xs text-muted-foreground">Values that look like secrets are hidden on the host.</p>
            </Section>

            <Section title={`Mounts (${d.mounts.length})`}>
              {d.mounts.length === 0 ? (
                <p className="text-sm text-muted-foreground">None.</p>
              ) : (
                <ul className="space-y-1">
                  {d.mounts.map((m) => (
                    <li key={m.destination} className={`${mono} break-all`}>
                      <span className="text-muted-foreground">{m.type} </span>
                      {m.name || m.source} <span className="text-muted-foreground">→</span> {m.destination}
                      {m.read_only && <span className="text-muted-foreground"> (read-only)</span>}
                    </li>
                  ))}
                </ul>
              )}
            </Section>

            <Section title={`Networks (${d.networks.length})`}>
              {d.networks.length === 0 ? (
                <p className="text-sm text-muted-foreground">None.</p>
              ) : (
                <ul className="space-y-1">
                  {d.networks.map((n) => (
                    <li key={n.name} className={mono}>
                      {n.name} <span className="text-muted-foreground">{n.ip}</span>
                    </li>
                  ))}
                </ul>
              )}
            </Section>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
