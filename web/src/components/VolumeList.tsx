import { useEffect, useState } from 'react'
import { Loader2, Trash2, TriangleAlert } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
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
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { useSelection } from '@/hooks/useSelection'
import { api, type Volume } from '@/lib/api'
import { removeMany } from '@/lib/bulkToast'
import { formatBytes, timeAgo } from '@/lib/format'
import { cn } from '@/lib/utils'
import { SelectionBar } from './SelectionBar'
import { StatusBadge } from './StatusBadge'

const size = (v: Volume) => (v.size < 0 ? '—' : formatBytes(v.size))
const shortName = (v: Volume) => (v.anonymous ? `${v.name.slice(0, 12)}…` : v.name)

// VolumeList shows volumes. Deleting one destroys its data, so: volumes any
// container mounts (running or not) cannot be selected, the agent refuses
// them too, and every delete needs a typed confirmation.
export function VolumeList({
  hostId,
  volumes,
  onChanged,
}: {
  hostId: string
  volumes: Volume[]
  onChanged: () => void
}) {
  const [busy, setBusy] = useState<Record<string, boolean>>({})
  const [confirm, setConfirm] = useState<Volume[]>()
  const removable = volumes.filter((v) => v.containers === 0)
  const sel = useSelection(removable.map((v) => v.name))
  const chosen = removable.filter((v) => sel.selected.has(v.name))
  const anonymous = removable.filter((v) => v.anonymous)
  const known = (vs: Volume[]) => vs.reduce((n, v) => n + Math.max(v.size, 0), 0)

  const remove = async (targets: Volume[]) => {
    setConfirm(undefined)
    setBusy((b) => ({ ...b, ...Object.fromEntries(targets.map((v) => [v.name, true])) }))
    await removeMany(targets, shortName, 'volume', (v) => api.removeVolume(hostId, v.name))
    setBusy((b) => ({ ...b, ...Object.fromEntries(targets.map((v) => [v.name, false])) }))
    sel.clear()
    onChanged()
  }

  const all = removable.length > 0 && chosen.length === removable.length
  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">
        {volumes.length} volumes · {formatBytes(known(volumes))} ·{' '}
        <span className="text-foreground">{formatBytes(known(removable))}</span> in {removable.length} unused
      </p>
      <SelectionBar
        count={chosen.length}
        noun="volume"
        onClear={sel.clear}
        onRemove={() => setConfirm(chosen)}
        shortcuts={
          <>
            <Button
              variant="outline"
              size="sm"
              disabled={removable.length === 0}
              onClick={() => sel.set(removable.map((v) => v.name))}
            >
              Select unused ({removable.length})
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={anonymous.length === 0}
              onClick={() => sel.set(anonymous.map((v) => v.name))}
            >
              Select anonymous ({anonymous.length})
            </Button>
          </>
        }
      />

      <div className="hidden md:block">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-8">
                <Checkbox
                  aria-label="Select all unused"
                  checked={all ? true : chosen.length > 0 ? 'indeterminate' : false}
                  onCheckedChange={(v) => sel.set(v === true ? removable.map((x) => x.name) : [])}
                />
              </TableHead>
              <TableHead>Volume</TableHead>
              <TableHead className="text-right">Size</TableHead>
              <TableHead>Created</TableHead>
              <TableHead>Used by</TableHead>
              <TableHead className="w-0" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {volumes.map((v) => (
              <TableRow key={v.name} data-state={sel.selected.has(v.name) ? 'selected' : undefined}>
                <TableCell>
                  <SelectBox v={v} sel={sel} />
                </TableCell>
                <TableCell className="max-w-96">
                  <NameCell v={v} />
                </TableCell>
                <TableCell className="text-right tabular-nums">{size(v)}</TableCell>
                <TableCell className="text-sm">{v.created_at ? timeAgo(v.created_at) : '—'}</TableCell>
                <TableCell>
                  <Usage v={v} />
                </TableCell>
                <TableCell>
                  <RemoveButton v={v} busy={busy[v.name]} onClick={() => setConfirm([v])} />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      <ul className="-mx-1 divide-y md:hidden">
        {volumes.map((v) => (
          <li
            key={v.name}
            className={cn('flex items-start gap-3 px-1 py-3', sel.selected.has(v.name) && 'bg-primary/5')}
          >
            <SelectBox v={v} sel={sel} className="mt-1" />
            <div className="min-w-0 flex-1 space-y-1">
              <NameCell v={v} />
              <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                <span className="tabular-nums">{size(v)}</span>
                {v.created_at && <span>{timeAgo(v.created_at)}</span>}
                <Usage v={v} />
              </div>
            </div>
            <RemoveButton v={v} busy={busy[v.name]} onClick={() => setConfirm([v])} large />
          </li>
        ))}
      </ul>

      <TypedRemoveDialog targets={confirm} onOpenChange={(o) => !o && setConfirm(undefined)} onConfirm={remove} />
    </div>
  )
}

function NameCell({ v }: { v: Volume }) {
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-1.5">
      <span className="truncate font-mono text-sm font-medium" title={v.name}>
        {shortName(v)}
      </span>
      {v.anonymous && (
        <Badge variant="outline" className="font-normal">
          anonymous
        </Badge>
      )}
      {v.compose_project && (
        <Badge variant="outline" className="font-normal">
          {v.compose_project}
        </Badge>
      )}
    </div>
  )
}

function SelectBox({
  v,
  sel,
  className,
}: {
  v: Volume
  sel: { selected: Set<string>; toggle: (id: string, on: boolean) => void }
  className?: string
}) {
  const box = (
    <Checkbox
      className={className}
      aria-label={`Select ${v.name}`}
      disabled={v.containers > 0}
      checked={sel.selected.has(v.name)}
      onCheckedChange={(c) => sel.toggle(v.name, c === true)}
    />
  )
  if (v.containers === 0) return box
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className={cn('inline-flex', className)}>{box}</span>
      </TooltipTrigger>
      <TooltipContent>Mounted by a container: remove the container first</TooltipContent>
    </Tooltip>
  )
}

function Usage({ v }: { v: Volume }) {
  if (v.containers === 0) return <StatusBadge tone="gray">unused</StatusBadge>
  return (
    <StatusBadge tone={v.running > 0 ? 'green' : 'amber'}>
      {v.containers} container{v.containers === 1 ? '' : 's'}
      {v.running > 0 && ` · ${v.running} running`}
    </StatusBadge>
  )
}

function RemoveButton({
  v,
  busy,
  onClick,
  large,
}: {
  v: Volume
  busy?: boolean
  onClick: () => void
  large?: boolean
}) {
  if (busy) return <Loader2 className="mx-2 my-2 size-4 animate-spin text-muted-foreground" />
  return (
    <Button
      variant="ghost"
      size={large ? 'icon' : 'icon-sm'}
      aria-label={`Delete ${v.name}`}
      disabled={v.containers > 0}
      onClick={onClick}
      className="text-muted-foreground hover:text-destructive"
    >
      <Trash2 />
    </Button>
  )
}

// TypedRemoveDialog makes deleting data deliberate: type the volume's name,
// or "delete N volumes" for several (or an anonymous one).
function TypedRemoveDialog({
  targets,
  onOpenChange,
  onConfirm,
}: {
  targets?: Volume[]
  onOpenChange: (open: boolean) => void
  onConfirm: (targets: Volume[]) => void
}) {
  const [typed, setTyped] = useState('')
  useEffect(() => setTyped(''), [targets])
  const vs = targets ?? []
  const phrase =
    vs.length === 1 && !vs[0].anonymous ? vs[0].name : `delete ${vs.length} volume${vs.length === 1 ? '' : 's'}`
  const total = vs.reduce((n, v) => n + Math.max(v.size, 0), 0)

  return (
    <AlertDialog open={!!targets} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            Delete {vs.length} volume{vs.length === 1 ? '' : 's'} ({formatBytes(total)})?
          </AlertDialogTitle>
          <AlertDialogDescription>
            No container uses {vs.length === 1 ? 'it' : 'them'} right now.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <Alert variant="destructive">
          <TriangleAlert />
          <AlertDescription>
            The data in {vs.length === 1 ? 'this volume' : 'these volumes'} is deleted permanently. There is no undo.
          </AlertDescription>
        </Alert>
        <ul className="max-h-32 space-y-0.5 overflow-y-auto rounded-md border bg-muted/40 p-2 font-mono text-xs">
          {vs.slice(0, 8).map((v) => (
            <li key={v.name} className="flex justify-between gap-3">
              <span className="truncate">{v.name}</span>
              <span className="shrink-0 text-muted-foreground">{size(v)}</span>
            </li>
          ))}
          {vs.length > 8 && <li className="text-muted-foreground">and {vs.length - 8} more…</li>}
        </ul>
        <div className="space-y-1.5">
          <Label htmlFor="typed-confirm" className="block text-sm font-normal">
            Type <code className="rounded bg-muted px-1 font-mono text-foreground">{phrase}</code> to confirm
          </Label>
          <Input
            id="typed-confirm"
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
            autoComplete="off"
            autoCapitalize="off"
            spellCheck={false}
            className="font-mono"
          />
        </div>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction variant="destructive" disabled={typed !== phrase} onClick={() => onConfirm(vs)}>
            Delete permanently
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
