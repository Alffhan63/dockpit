import { useEffect, useState } from 'react'
import { Loader2 } from 'lucide-react'
import { toast } from 'sonner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { usePoll } from '@/hooks/usePoll'
import { api, type PruneSchedule } from '@/lib/api'

const days = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']

export function AutomationPanel() {
  const stored = usePoll(api.pruneSchedule, 3_600_000, 'prune')
  const [s, setS] = useState<PruneSchedule>()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()
  useEffect(() => {
    if (stored.data) setS(stored.data)
  }, [stored.data])
  if (!s) return <Skeleton className="h-64 rounded-xl" />

  const save = async () => {
    setBusy(true)
    setError(undefined)
    try {
      setS(await api.savePruneSchedule(s))
      toast.success('Schedule saved')
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>Weekly cleanup</CardTitle>
          <CardDescription>
            Runs on every online host. It only removes untagged images and build cache. Containers and volumes are never touched. Each run is written to the audit log.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between gap-4">
            <Label htmlFor="prune-on" className="flex-col items-start gap-0.5">
              <span>Run automatically</span>
            </Label>
            <Switch id="prune-on" checked={s.enabled} onCheckedChange={(v) => setS({ ...s, enabled: v })} />
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="prune-day">Day</Label>
              <Select value={String(s.weekday)} onValueChange={(v) => setS({ ...s, weekday: Number(v) })}>
                <SelectTrigger id="prune-day" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {days.map((d, i) => (
                    <SelectItem key={d} value={String(i)}>
                      {d}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="prune-hour">Time (controller time zone)</Label>
              <Select value={String(s.hour)} onValueChange={(v) => setS({ ...s, hour: Number(v) })}>
                <SelectTrigger id="prune-hour" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {Array.from({ length: 24 }, (_, h) => (
                    <SelectItem key={h} value={String(h)}>
                      {String(h).padStart(2, '0')}:00
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <div className="flex items-center justify-between gap-4">
            <Label htmlFor="prune-dangling">Untagged (dangling) images</Label>
            <Switch id="prune-dangling" checked={s.dangling_images} onCheckedChange={(v) => setS({ ...s, dangling_images: v })} />
          </div>
          <div className="flex items-center justify-between gap-4">
            <Label htmlFor="prune-cache">Build cache</Label>
            <Switch id="prune-cache" checked={s.build_cache} onCheckedChange={(v) => setS({ ...s, build_cache: v })} />
          </div>
          {error && (
            <Alert variant="destructive" role="alert">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
          <Button onClick={save} disabled={busy}>
            {busy && <Loader2 className="animate-spin" />} Save
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Backups</CardTitle>
          <CardDescription>
            The controller copies its database (hosts, token hashes, settings, history) every night after 03:00 into
            <code className="mx-1 font-mono">data/backups/</code>and keeps the last 7. Copy that folder off the server to survive a disk failure.
          </CardDescription>
        </CardHeader>
      </Card>
    </div>
  )
}
