import { useMemo, useState } from 'react'
import { FolderGit2, Loader2, Play, Square, Trash2 } from 'lucide-react'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { href } from '@/hooks/useRoute'
import { api, type Container, type Image } from '@/lib/api'
import { removeMany, runMany } from '@/lib/bulkToast'
import { formatBytes, formatPercent } from '@/lib/format'
import { cn } from '@/lib/utils'
import { StatusBadge } from './StatusBadge'

interface Project {
  name: string
  dir?: string
  containers: Container[]
  running: number
}

// groupProjects groups containers by their docker compose project label.
function groupProjects(containers: Container[]): { projects: Project[]; standalone: number } {
  const byName = new Map<string, Project>()
  let standalone = 0
  for (const c of containers) {
    if (!c.compose_project) {
      standalone++
      continue
    }
    let p = byName.get(c.compose_project)
    if (!p) {
      p = { name: c.compose_project, dir: c.compose_dir, containers: [], running: 0 }
      byName.set(p.name, p)
    }
    p.containers.push(c)
    if (c.state === 'running') p.running++
  }
  const projects = [...byName.values()]
  for (const p of projects) {
    p.containers.sort((a, b) => (a.compose_service ?? a.name).localeCompare(b.compose_service ?? b.name))
  }
  // Running projects first, then by name.
  projects.sort((a, b) => Number(b.running > 0) - Number(a.running > 0) || a.name.localeCompare(b.name))
  return { projects, standalone }
}

// exclusiveImages returns the images that only this project's containers use.
function exclusiveImages(p: Project, images: Image[]): Image[] {
  const uses = new Map<string, number>()
  for (const c of p.containers) uses.set(c.image_id, (uses.get(c.image_id) ?? 0) + 1)
  return images.filter((img) => img.containers > 0 && uses.get(img.id) === img.containers)
}

export function ProjectList({
  hostId,
  containers,
  onChanged,
}: {
  hostId: string
  containers: Container[]
  onChanged: () => void
}) {
  const { projects, standalone } = useMemo(() => groupProjects(containers), [containers])
  const [busy, setBusy] = useState<Record<string, boolean>>({})
  const [removing, setRemoving] = useState<{ project: Project; images: Image[] }>()

  const withBusy = async (p: Project, fn: () => Promise<unknown>) => {
    setBusy((b) => ({ ...b, [p.name]: true }))
    try {
      await fn()
    } finally {
      setBusy((b) => ({ ...b, [p.name]: false }))
      onChanged()
    }
  }
  const startAll = (p: Project) =>
    withBusy(p, () =>
      runMany(
        p.containers.filter((c) => c.state !== 'running'),
        (c) => c.name,
        { doing: 'Starting', done: 'Started', noun: 'container' },
        (c) => api.containerAction(hostId, c.id, 'start'),
      ),
    )
  const stopAll = (p: Project) =>
    withBusy(p, () =>
      runMany(
        p.containers.filter((c) => c.state === 'running'),
        (c) => c.name,
        { doing: 'Stopping', done: 'Stopped', noun: 'container' },
        (c) => api.containerAction(hostId, c.id, 'stop'),
      ),
    )
  const askRemove = (p: Project) =>
    withBusy(p, async () => {
      // Image usage is needed to offer removing the project's own images.
      const images = await api.images(hostId).catch(() => [])
      setRemoving({ project: p, images: exclusiveImages(p, images) })
    })
  const remove = (p: Project, images: Image[]) =>
    withBusy(p, async () => {
      setRemoving(undefined)
      const failed = await removeMany(
        p.containers,
        (c) => c.name,
        'container',
        (c) => api.removeContainer(hostId, c.id, true),
      )
      // Images go only once their containers are gone; the agent re-checks
      // usage, so an image something else still uses is refused, not removed.
      if (failed.length === 0 && images.length > 0) {
        await removeMany(
          images,
          (i) => i.tags[0] ?? i.id.slice(7, 19),
          'image',
          (i) => api.removeImage(hostId, i.id),
        )
      }
    })

  if (projects.length === 0) {
    return (
      <Card>
        <CardContent className="flex flex-col items-center gap-2 py-10 text-center">
          <FolderGit2 className="size-8 text-muted-foreground" />
          <p className="font-medium">No compose projects</p>
          <p className="text-sm text-muted-foreground">
            Containers started with <code className="font-mono">docker compose up</code> show up here, grouped per
            project.
          </p>
        </CardContent>
      </Card>
    )
  }

  return (
    <div className="space-y-3">
      <div className="grid gap-3 lg:grid-cols-2">
        {projects.map((p) => (
          <ProjectCard
            key={p.name}
            hostId={hostId}
            p={p}
            busy={busy[p.name]}
            onStart={() => startAll(p)}
            onStop={() => stopAll(p)}
            onRemove={() => askRemove(p)}
          />
        ))}
      </div>
      {standalone > 0 && (
        <p className="text-sm text-muted-foreground">
          {standalone} container{standalone === 1 ? ' is' : 's are'} not part of a compose project; see{' '}
          <a href={href({ page: 'host', hostId, tab: 'containers' })} className="text-foreground underline">
            Containers
          </a>
          .
        </p>
      )}
      <RemoveProjectDialog target={removing} onOpenChange={(o) => !o && setRemoving(undefined)} onConfirm={remove} />
    </div>
  )
}

function ProjectCard({
  hostId,
  p,
  busy,
  onStart,
  onStop,
  onRemove,
}: {
  hostId: string
  p: Project
  busy?: boolean
  onStart: () => void
  onStop: () => void
  onRemove: () => void
}) {
  const running = p.containers.filter((c) => c.state === 'running')
  const cpu = running.reduce((n, c) => n + (c.cpu_percent ?? 0), 0)
  const mem = running.reduce((n, c) => n + (c.mem_usage ?? 0), 0)
  const all = p.running === p.containers.length

  return (
    <Card size="sm">
      <CardHeader>
        <div className="flex items-start justify-between gap-2">
          <CardTitle className="truncate font-mono">{p.name}</CardTitle>
          <StatusBadge tone={all ? 'green' : p.running > 0 ? 'amber' : 'gray'}>
            {p.running}/{p.containers.length} running
          </StatusBadge>
        </div>
        {p.dir && (
          <p className="truncate font-mono text-xs text-muted-foreground" title={p.dir}>
            {p.dir}
          </p>
        )}
      </CardHeader>
      <CardContent>
        <ul className="flex flex-wrap gap-1.5">
          {p.containers.map((c) => (
            <li key={c.id}>
              <a
                href={href({ page: 'logs', hostId, container: c.id })}
                title={`${c.name}: ${c.status}`}
                className="inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 font-mono text-xs hover:bg-muted"
              >
                <span
                  className={cn(
                    'size-1.5 rounded-full',
                    c.state === 'running'
                      ? 'bg-success'
                      : c.state === 'restarting'
                        ? 'bg-warning'
                        : 'bg-muted-foreground',
                  )}
                />
                {c.compose_service ?? c.name}
              </a>
            </li>
          ))}
        </ul>
      </CardContent>
      <CardFooter className="flex-wrap gap-2">
        <span className="text-xs text-muted-foreground tabular-nums">
          {running.length > 0 ? `CPU ${formatPercent(cpu)} · RAM ${formatBytes(mem)}` : 'Stopped'}
        </span>
        <div className="ml-auto flex items-center gap-1">
          {busy ? (
            <Loader2 className="m-2 size-4 animate-spin text-muted-foreground" />
          ) : (
            <>
              {!all && (
                <Button variant="outline" size="sm" onClick={onStart}>
                  <Play /> Start all
                </Button>
              )}
              {p.running > 0 && (
                <Button variant="outline" size="sm" onClick={onStop}>
                  <Square /> Stop all
                </Button>
              )}
              <Button
                variant="ghost"
                size="sm"
                className="text-muted-foreground hover:text-destructive"
                onClick={onRemove}
              >
                <Trash2 /> Remove
              </Button>
            </>
          )}
        </div>
      </CardFooter>
    </Card>
  )
}

function RemoveProjectDialog({
  target,
  onOpenChange,
  onConfirm,
}: {
  target?: { project: Project; images: Image[] }
  onOpenChange: (open: boolean) => void
  onConfirm: (p: Project, images: Image[]) => void
}) {
  const [withImages, setWithImages] = useState(false)
  const p = target?.project
  const images = target?.images ?? []
  const imageSize = images.reduce((n, i) => n + i.size, 0)

  return (
    <AlertDialog
      open={!!target}
      onOpenChange={(o) => {
        if (!o) setWithImages(false)
        onOpenChange(o)
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Remove project {p?.name}?</AlertDialogTitle>
          <AlertDialogDescription>
            {p && p.running > 0
              ? `Its ${p.running} running container${p.running === 1 ? ' is' : 's are'} stopped, then all ${p.containers.length} are removed.`
              : `All ${p?.containers.length ?? 0} containers are removed.`}{' '}
            Volumes and networks are kept, so the data survives a later <code>docker compose up</code>.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <ul className="max-h-32 space-y-0.5 overflow-y-auto rounded-md border bg-muted/40 p-2 font-mono text-xs">
          {p?.containers.map((c) => (
            <li key={c.id} className="truncate">
              {c.name}
            </li>
          ))}
        </ul>
        {images.length > 0 && (
          <div className="flex items-center justify-between gap-3 rounded-md border p-3">
            <Label htmlFor="with-images" className="flex-col items-start gap-0.5">
              <span>
                Also remove {images.length} image{images.length === 1 ? '' : 's'} ({formatBytes(imageSize)})
              </span>
              <span className="text-xs font-normal text-muted-foreground">
                Only images no other container uses: {images.map((i) => i.tags[0] ?? '<none>').join(', ')}
              </span>
            </Label>
            <Switch id="with-images" checked={withImages} onCheckedChange={setWithImages} />
          </div>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={() => p && onConfirm(p, withImages ? images : [])}>
            Remove project
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
