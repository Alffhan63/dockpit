import { useCallback, useEffect, useState } from 'react'
import { Loader2 } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { api, type AuditEntry } from '@/lib/api'

const labels: Record<string, string> = {
  'containers.start': 'Started container',
  'containers.stop': 'Stopped container',
  'containers.restart': 'Restarted container',
  'containers.remove': 'Removed container',
  'images.remove': 'Removed image',
  'volumes.remove': 'Removed volume',
  'system.prune': 'Cleaned up',
  'prune.scheduled': 'Scheduled cleanup',
  'hosts.create': 'Added host',
  'hosts.delete': 'Removed host',
  'hosts.token': 'New agent token',
  'auth.login': 'Signed in',
  'auth.login.failed': 'Failed sign-in',
  'auth.2fa.enable': 'Turned on 2FA',
  'auth.2fa.disable': 'Turned off 2FA',
  'settings.notifications': 'Changed notification settings',
  'settings.notifications.test': 'Sent test notification',
  'settings.prune': 'Changed cleanup schedule',
}

export function AuditPanel() {
  const [entries, setEntries] = useState<AuditEntry[]>()
  const [more, setMore] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()

  const load = useCallback(async (before?: number) => {
    setBusy(true)
    try {
      const page = await api.audit(before)
      setEntries((prev) => (before ? [...(prev ?? []), ...page] : page))
      setMore(page.length === 50)
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }, [])
  useEffect(() => {
    load()
  }, [load])

  if (error) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{error}</AlertDescription>
      </Alert>
    )
  }
  if (!entries) return <Skeleton className="h-64 rounded-xl" />

  return (
    <Card>
      <CardContent className="space-y-3">
        {entries.length === 0 ? (
          <p className="py-6 text-center text-sm text-muted-foreground">Nothing recorded yet.</p>
        ) : (
          <Table aria-label="Audit log">
            <TableHeader>
              <TableRow>
                <TableHead>When</TableHead>
                <TableHead>Action</TableHead>
                <TableHead>Target</TableHead>
                <TableHead className="hidden sm:table-cell">From</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {entries.map((e) => (
                <TableRow key={e.id}>
                  <TableCell className="text-xs whitespace-nowrap tabular-nums">{new Date(e.time).toLocaleString()}</TableCell>
                  <TableCell>{labels[e.action] ?? e.action}</TableCell>
                  <TableCell className="max-w-56 truncate font-mono text-xs" title={[e.host_id, e.target, e.detail].filter(Boolean).join(' · ')}>
                    {[e.host_id, e.target].filter(Boolean).join(' / ') || '—'}
                    {e.detail && <span className="text-muted-foreground"> · {e.detail}</span>}
                  </TableCell>
                  <TableCell className="hidden font-mono text-xs text-muted-foreground sm:table-cell">{e.actor}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
        {more && (
          <Button variant="outline" size="sm" disabled={busy} onClick={() => load(entries.at(-1)?.id)}>
            {busy && <Loader2 className="animate-spin" />} Load more
          </Button>
        )}
      </CardContent>
    </Card>
  )
}
