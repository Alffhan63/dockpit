import { useState } from 'react'
import { Boxes, Database, Hammer, Layers, Loader2, RefreshCw, Sparkles } from 'lucide-react'
import { toast } from 'sonner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import { usePoll } from '@/hooks/usePoll'
import { href } from '@/hooks/useRoute'
import { api, type DiskUsageItem, type PruneKind } from '@/lib/api'
import { formatBytes, percent } from '@/lib/format'
import { tap } from '@/lib/haptics'
import { ConfirmDialog } from './ConfirmDialog'

const prunes: Record<PruneKind, { title: string; description: string; confirm: string }> = {
  'build-cache': {
    title: 'Clear build cache?',
    description: 'Unused build cache is deleted. Nothing running is affected; the next image build may be slower.',
    confirm: 'Clear build cache',
  },
  'dangling-images': {
    title: 'Remove dangling images?',
    description: 'Untagged images that no container uses are deleted. Tagged images are kept.',
    confirm: 'Remove dangling images',
  },
}

// CleanupPanel shows `docker system df` and offers only low-risk cleanups:
// build cache and dangling images. Containers and volumes are shown but must
// be removed deliberately from their own tabs (volumes: not at all).
export function CleanupPanel({ hostId, onChanged }: { hostId: string; onChanged: () => void }) {
  const disk = usePoll(() => api.disk(hostId), 60_000, hostId)
  const [confirm, setConfirm] = useState<PruneKind>()
  const [running, setRunning] = useState<PruneKind>()
  const du = disk.data

  const prune = async (kind: PruneKind) => {
    setConfirm(undefined)
    setRunning(kind)
    const id = toast.loading(kind === 'build-cache' ? 'Clearing build cache…' : 'Removing dangling images…')
    try {
      const res = await api.prune(hostId, kind)
      tap()
      toast.success(`Freed ${formatBytes(res.space_reclaimed)}`, {
        id,
        description: `${res.deleted} ${kind === 'build-cache' ? 'cache entries' : 'images'} deleted`,
      })
      await disk.refresh()
      onChanged()
    } catch (err) {
      toast.error((err as Error).message, { id })
    } finally {
      setRunning(undefined)
    }
  }

  const total = du ? du.images.size + du.containers.size + du.volumes.size + du.build_cache.size : 0
  const reclaimable = du ? du.images.reclaimable + du.containers.reclaimable + du.build_cache.reclaimable : 0

  return (
    <Card>
      <CardHeader>
        <CardTitle>Disk usage</CardTitle>
        <CardDescription>
          {du
            ? `${formatBytes(total)} used by Docker · ${formatBytes(reclaimable)} reclaimable without touching volumes`
            : 'Measuring… this can take a few seconds on large hosts.'}
        </CardDescription>
        <CardAction>
          <Button variant="ghost" size="icon-sm" aria-label="Refresh" onClick={() => disk.refresh()}>
            <RefreshCw />
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent>
        {disk.error ? (
          <Alert variant="destructive">
            <AlertDescription>{disk.error.message}</AlertDescription>
          </Alert>
        ) : !du ? (
          <div className="grid gap-3 sm:grid-cols-2">
            {[0, 1, 2, 3].map((i) => (
              <Skeleton key={i} className="h-36 rounded-xl" />
            ))}
          </div>
        ) : (
          <div className="grid gap-3 sm:grid-cols-2">
            <UsageTile
              icon={<Hammer />}
              label="Build cache"
              item={du.build_cache}
              activeLabel="in use"
              action={
                <Button
                  size="sm"
                  disabled={du.build_cache.reclaimable === 0 || !!running}
                  onClick={() => setConfirm('build-cache')}
                >
                  {running === 'build-cache' ? <Loader2 className="animate-spin" /> : <Sparkles />}
                  Clear {formatBytes(du.build_cache.reclaimable)}
                </Button>
              }
            />
            <UsageTile
              icon={<Layers />}
              label="Images"
              item={du.images}
              activeLabel="used by containers"
              action={
                <div className="flex flex-wrap gap-2">
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={!!running}
                    onClick={() => setConfirm('dangling-images')}
                  >
                    {running === 'dangling-images' ? <Loader2 className="animate-spin" /> : <Sparkles />}
                    Remove dangling
                  </Button>
                  <Button size="sm" variant="ghost" asChild>
                    <a href={href({ page: 'host', hostId, tab: 'images' })}>Choose images…</a>
                  </Button>
                </div>
              }
            />
            <UsageTile
              icon={<Boxes />}
              label="Containers"
              item={du.containers}
              activeLabel="running"
              note="Writable layers only; images are counted above."
              action={
                <Button size="sm" variant="ghost" asChild>
                  <a href={href({ page: 'host', hostId, tab: 'containers' })}>Choose containers…</a>
                </Button>
              }
            />
            <UsageTile
              icon={<Database />}
              label="Volumes"
              item={du.volumes}
              activeLabel="in use"
              note="Volumes hold data, so they are never cleaned up in bulk here."
              action={
                <Button size="sm" variant="ghost" asChild>
                  <a href={href({ page: 'host', hostId, tab: 'volumes' })}>Review volumes…</a>
                </Button>
              }
            />
          </div>
        )}
      </CardContent>
      {confirm && (
        <ConfirmDialog
          open
          onOpenChange={(o) => !o && setConfirm(undefined)}
          title={prunes[confirm].title}
          description={prunes[confirm].description}
          confirm={prunes[confirm].confirm}
          onConfirm={() => prune(confirm)}
        />
      )}
    </Card>
  )
}

function UsageTile({
  icon,
  label,
  item,
  activeLabel,
  note,
  action,
}: {
  icon: React.ReactNode
  label: string
  item: DiskUsageItem
  activeLabel: string
  note?: string
  action?: React.ReactNode
}) {
  const share = percent(item.reclaimable, item.size)
  return (
    <div className="flex flex-col gap-3 rounded-xl border p-4">
      <div className="flex items-center gap-2 text-sm text-muted-foreground [&_svg]:size-4">
        {icon}
        {label}
        <span className="ml-auto tabular-nums">
          {item.count} · {item.active} {activeLabel}
        </span>
      </div>
      <div>
        <div className="text-2xl font-semibold tabular-nums">{formatBytes(item.size)}</div>
        <div className="text-xs text-muted-foreground tabular-nums">
          {formatBytes(item.reclaimable)} reclaimable ({share.toFixed(0)}%)
        </div>
      </div>
      <Progress value={share} className="h-1.5 [&>*]:bg-warning" />
      {note && <p className="text-xs text-muted-foreground">{note}</p>}
      {action && <div className="mt-auto">{action}</div>}
    </div>
  )
}
