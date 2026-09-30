import { useState } from 'react'
import { EllipsisVertical, Info, Loader2, Play, Rows3, Rows4, RotateCw, ScrollText, Square, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { useLocalPref } from '@/hooks/useLocalPref'
import { usePoll } from '@/hooks/usePoll'
import { href } from '@/hooks/useRoute'
import { useSelection } from '@/hooks/useSelection'
import { api, type Container, type ContainerAction, type Port } from '@/lib/api'
import { formatBytes, formatPercent, timeAgo } from '@/lib/format'
import { removeMany } from '@/lib/bulkToast'
import { tap } from '@/lib/haptics'
import { cn } from '@/lib/utils'
import { BulkRemoveDialog } from './BulkRemoveDialog'
import { ContainerDetailDialog } from './ContainerDetailDialog'
import { Sparkline } from './Sparkline'
import { RemoveContainerDialog } from './RemoveContainerDialog'
import { SelectionBar } from './SelectionBar'
import { HealthBadge, StatusBadge, containerTone } from './StatusBadge'

function formatPort(p: Port): string {
  return p.public_port ? `${p.public_port}→${p.private_port}` : `${p.private_port}`
}

const verbs: Record<ContainerAction, string> = { start: 'Started', stop: 'Stopped', restart: 'Restarted' }

interface Actions {
  busy: Record<string, boolean>
  action: (c: Container, a: ContainerAction) => void
  remove: (c: Container) => void
  selected: Set<string>
  toggle: (id: string, on: boolean) => void
  details: (c: Container) => void
  sparks: Record<string, number[]>
}

// exitInfo describes why a stopped container is down.
function exitInfo(c: Container): string | undefined {
  if (c.state === 'running' || c.exit_code === undefined) return undefined
  if (c.oom_killed) return `out of memory (exit ${c.exit_code})`
  return c.exit_code === 0 ? undefined : `exit ${c.exit_code}`
}

// Why a container's logs may be empty: some Docker log drivers cannot be read back.
export const READABLE_LOG_DRIVERS = ['json-file', 'local', 'journald']

function Restarts({ c }: { c: Container }) {
  if (!c.restart_count) return null
  return (
    <span
      className={cn('text-xs tabular-nums', c.restart_count >= 3 ? 'text-warning' : 'text-muted-foreground')}
      title="Times Docker restarted this container"
    >
      ↻ {c.restart_count}
    </span>
  )
}

// ContainerTable shows containers as a table on wide screens and as cards on
// phones, sharing the same actions.
export function ContainerTable({
  hostId,
  containers,
  onChanged,
}: {
  hostId: string
  containers: Container[]
  onChanged: () => void
}) {
  const [busy, setBusy] = useState<Record<string, boolean>>({})
  const [removing, setRemoving] = useState<Container>()
  const [bulkOpen, setBulkOpen] = useState(false)
  const [inspecting, setInspecting] = useState<Container>()
  const [compact, setCompact] = useLocalPref('cockpit:compact-containers', false)
  const sparks = usePoll(() => api.sparks(hostId), 30_000, hostId)
  const sel = useSelection(containers.map((c) => c.id))
  const chosen = containers.filter((c) => sel.selected.has(c.id))
  const chosenRunning = chosen.filter((c) => c.state === 'running').length
  const notRunning = containers.filter((c) => c.state !== 'running')

  // undo, when given, is the action that reverses this one; the toast offers it.
  const run = async (c: Container, fn: () => Promise<void>, done: string, undo?: ContainerAction) => {
    setBusy((b) => ({ ...b, [c.id]: true }))
    try {
      await fn()
      tap()
      toast.success(
        `${done} ${c.name}`,
        undo && {
          duration: 8000,
          action: {
            label: 'Undo',
            onClick: () =>
              api
                .containerAction(hostId, c.id, undo)
                .then(onChanged)
                .catch((err: Error) => toast.error(`${c.name}: ${err.message}`)),
          },
        },
      )
      onChanged()
    } catch (err) {
      toast.error(`${c.name}: ${(err as Error).message}`)
    } finally {
      setBusy((b) => ({ ...b, [c.id]: false }))
    }
  }
  const actions: Actions = {
    busy,
    action: (c, a) =>
      run(c, () => api.containerAction(hostId, c.id, a), verbs[a], a === 'stop' ? 'start' : a === 'start' ? 'stop' : undefined),
    remove: setRemoving,
    selected: sel.selected,
    toggle: sel.toggle,
    details: setInspecting,
    sparks: sparks.data ?? {},
  }

  const removeSelected = async (force: boolean) => {
    const targets = force ? chosen : chosen.filter((c) => c.state !== 'running')
    setBulkOpen(false)
    setBusy((b) => ({ ...b, ...Object.fromEntries(targets.map((c) => [c.id, true])) }))
    await removeMany(
      targets,
      (c) => c.name,
      'container',
      (c) => api.removeContainer(hostId, c.id, force),
    )
    setBusy((b) => ({ ...b, ...Object.fromEntries(targets.map((c) => [c.id, false])) }))
    sel.clear()
    onChanged()
  }

  return (
    <div className="space-y-3">
      <SelectionBar
        count={chosen.length}
        noun="container"
        onClear={sel.clear}
        onRemove={() => setBulkOpen(true)}
        shortcuts={
          <Button
            variant="outline"
            size="sm"
            disabled={notRunning.length === 0}
            onClick={() => sel.set(notRunning.map((c) => c.id))}
          >
            Select not running ({notRunning.length})
          </Button>
        }
      />
      <div className="flex justify-end">
        <Button variant="ghost" size="sm" aria-pressed={compact} onClick={() => setCompact((v) => !v)}>
          {compact ? <Rows3 /> : <Rows4 />} {compact ? 'Comfortable' : 'Compact'}
        </Button>
      </div>
      <div className="hidden md:block">
        <WideTable hostId={hostId} containers={containers} actions={actions} onSelectAll={sel.set} compact={compact} />
      </div>
      <ul className="-mx-1 divide-y md:hidden">
        {containers.map((c) => (
          <ContainerCard key={c.id} hostId={hostId} c={c} actions={actions} compact={compact} />
        ))}
      </ul>
      <ContainerDetailDialog hostId={hostId} container={inspecting} onOpenChange={(o) => !o && setInspecting(undefined)} />
      <RemoveContainerDialog
        container={removing}
        onOpenChange={(o) => !o && setRemoving(undefined)}
        onConfirm={(c, force) => run(c, () => api.removeContainer(hostId, c.id, force), 'Removed')}
      />
      <BulkRemoveDialog
        open={bulkOpen}
        onOpenChange={setBulkOpen}
        title={`Remove ${chosen.length} container${chosen.length === 1 ? '' : 's'}?`}
        description="The containers are deleted. Their images and volumes are kept."
        names={chosen.map((c) => c.name)}
        forceCount={chosenRunning}
        forceLabel={`Also stop and remove ${chosenRunning} running`}
        onConfirm={removeSelected}
      />
    </div>
  )
}

function WideTable({
  hostId,
  containers,
  actions,
  onSelectAll,
  compact,
}: {
  hostId: string
  containers: Container[]
  actions: Actions
  onSelectAll: (ids: string[]) => void
  compact: boolean
}) {
  const all = containers.length > 0 && actions.selected.size === containers.length
  const some = actions.selected.size > 0 && !all
  return (
    <Table aria-label="Containers">
      <TableHeader>
        <TableRow>
          <TableHead className="w-8">
            <Checkbox
              aria-label="Select all"
              checked={all ? true : some ? 'indeterminate' : false}
              onCheckedChange={(v) => onSelectAll(v === true ? containers.map((c) => c.id) : [])}
            />
          </TableHead>
          <TableHead>Name</TableHead>
          <TableHead>Image</TableHead>
          <TableHead>Status</TableHead>
          <TableHead className="hidden lg:table-cell">Ports</TableHead>
          <TableHead className="text-right">CPU (30 min)</TableHead>
          <TableHead className="text-right">Memory</TableHead>
          <TableHead className="hidden lg:table-cell">Created</TableHead>
          <TableHead className="w-0" />
        </TableRow>
      </TableHeader>
      <TableBody>
        {containers.map((c) => {
          const running = c.state === 'running'
          return (
            <TableRow
              key={c.id}
              data-state={actions.selected.has(c.id) ? 'selected' : undefined}
              className={cn(!running && 'text-muted-foreground', compact && '[&>td]:py-1')}
            >
              <TableCell>
                <Checkbox
                  aria-label={`Select ${c.name}`}
                  checked={actions.selected.has(c.id)}
                  onCheckedChange={(v) => actions.toggle(c.id, v === true)}
                />
              </TableCell>
              <TableCell className="max-w-64">
                <div className="flex items-center gap-2">
                  <a
                    href={href({ page: 'logs', hostId, container: c.id })}
                    className="truncate font-medium text-foreground hover:underline"
                  >
                    {c.name}
                  </a>
                  {c.compose_project && (
                    <Badge variant="outline" className="shrink-0 font-normal">
                      {c.compose_project}
                    </Badge>
                  )}
                </div>
                <div className="flex items-center gap-2 font-mono text-xs text-muted-foreground">
                  {!compact && c.id.slice(0, 12)}
                  <Restarts c={c} />
                </div>
              </TableCell>
              <TableCell className="max-w-56 truncate font-mono text-xs" title={c.image}>
                {c.image}
              </TableCell>
              <TableCell>
                <div className="flex flex-col items-start gap-1">
                  <span className="flex flex-wrap items-center gap-1">
                    <StatusBadge tone={containerTone(c.state)}>{c.state}</StatusBadge>
                    <HealthBadge health={c.health} />
                  </span>
                  {(!compact || exitInfo(c)) && (
                    <span className="text-xs text-muted-foreground">
                      {!compact && c.status}
                      {exitInfo(c) && <span className="text-destructive"> · {exitInfo(c)}</span>}
                    </span>
                  )}
                </div>
              </TableCell>
              <TableCell className="hidden font-mono text-xs lg:table-cell">
                {c.ports.map(formatPort).join(', ') || '—'}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {running ? (
                  <span className="inline-flex items-center justify-end gap-2">
                    <Sparkline values={actions.sparks[c.id] ?? []} className="hidden xl:inline-block" width={56} height={18} />
                    {formatPercent(c.cpu_percent)}
                  </span>
                ) : (
                  '—'
                )}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {running && c.mem_usage ? formatBytes(c.mem_usage) : '—'}
              </TableCell>
              <TableCell className="hidden text-sm lg:table-cell">{timeAgo(c.created)}</TableCell>
              <TableCell>
                <RowActions hostId={hostId} c={c} actions={actions} />
              </TableCell>
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}

function ContainerCard({ hostId, c, actions, compact }: { hostId: string; c: Container; actions: Actions; compact: boolean }) {
  const running = c.state === 'running'
  return (
    <li className={cn('flex items-start gap-3 px-1', compact ? 'py-1.5' : 'py-3', actions.selected.has(c.id) && 'bg-primary/5')}>
      <Checkbox
        className="mt-1"
        aria-label={`Select ${c.name}`}
        checked={actions.selected.has(c.id)}
        onCheckedChange={(v) => actions.toggle(c.id, v === true)}
      />
      <a href={href({ page: 'logs', hostId, container: c.id })} className="min-w-0 flex-1 space-y-1">
        <div className="flex items-center gap-2">
          <span className={cn('truncate font-medium', !running && 'text-muted-foreground')}>{c.name}</span>
          <StatusBadge tone={containerTone(c.state)}>{c.state}</StatusBadge>
          <HealthBadge health={c.health} />
          <Restarts c={c} />
        </div>
        {!compact && <div className="truncate font-mono text-xs text-muted-foreground">{c.image}</div>}
        <div className="flex flex-wrap gap-x-3 gap-y-0.5 text-xs text-muted-foreground">
          {running ? (
            <>
              <span className="inline-flex items-center gap-1.5 tabular-nums">
                CPU {formatPercent(c.cpu_percent)}
                <Sparkline values={actions.sparks[c.id] ?? []} width={44} height={14} />
              </span>
              <span className="tabular-nums">RAM {c.mem_usage ? formatBytes(c.mem_usage) : '—'}</span>
            </>
          ) : (
            <span>
              {c.status}
              {exitInfo(c) && <span className="text-destructive"> · {exitInfo(c)}</span>}
            </span>
          )}
          {c.ports.length > 0 && <span className="font-mono">{c.ports.map(formatPort).join(', ')}</span>}
          {c.compose_project && <span>· {c.compose_project}</span>}
        </div>
      </a>
      <RowActions hostId={hostId} c={c} actions={actions} large />
    </li>
  )
}

function RowActions({
  hostId,
  c,
  actions,
  large,
}: {
  hostId: string
  c: Container
  actions: Actions
  large?: boolean
}) {
  const running = c.state === 'running'
  const size = large ? 'icon' : 'icon-sm'
  if (actions.busy[c.id]) {
    return <Loader2 className="mx-2 my-2 size-4 animate-spin text-muted-foreground" />
  }
  return (
    <div className="flex shrink-0 items-center justify-end gap-0.5">
      {running ? (
        <IconAction label="Stop" size={size} onClick={() => actions.action(c, 'stop')}>
          <Square />
        </IconAction>
      ) : (
        <IconAction label="Start" size={size} onClick={() => actions.action(c, 'start')}>
          <Play />
        </IconAction>
      )}
      {!large && (
        <IconAction label="Logs" size={size} href={href({ page: 'logs', hostId, container: c.id })}>
          <ScrollText />
        </IconAction>
      )}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size={size} aria-label="More actions">
            <EllipsisVertical />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {large && (
            <DropdownMenuItem asChild>
              <a href={href({ page: 'logs', hostId, container: c.id })}>
                <ScrollText /> Logs
              </a>
            </DropdownMenuItem>
          )}
          <DropdownMenuItem onClick={() => actions.details(c)}>
            <Info /> Details
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => actions.action(c, 'restart')}>
            <RotateCw /> Restart
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" onClick={() => actions.remove(c)}>
            <Trash2 /> Remove…
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}

function IconAction({
  label,
  size,
  onClick,
  href,
  children,
}: {
  label: string
  size: 'icon' | 'icon-sm'
  onClick?: () => void
  href?: string
  children: React.ReactNode
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        {href ? (
          <Button variant="ghost" size={size} aria-label={label} asChild>
            <a href={href}>{children}</a>
          </Button>
        ) : (
          <Button variant="ghost" size={size} aria-label={label} onClick={onClick}>
            {children}
          </Button>
        )}
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  )
}
