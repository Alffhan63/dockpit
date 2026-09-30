import { useState } from 'react'
import { Loader2, Trash2 } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { useSelection } from '@/hooks/useSelection'
import { api, type Image } from '@/lib/api'
import { removeMany } from '@/lib/bulkToast'
import { formatBytes, timeAgo } from '@/lib/format'
import { cn } from '@/lib/utils'
import { BulkRemoveDialog } from './BulkRemoveDialog'
import { SelectionBar } from './SelectionBar'
import { StatusBadge } from './StatusBadge'

const shortId = (id: string) => id.replace(/^sha256:/, '').slice(0, 12)
const label = (img: Image) => img.tags[0] ?? `<none> ${shortId(img.id)}`

// ImageList shows local images. Images used by any container, even a
// stopped one, cannot be selected: the agent refuses to remove them.
export function ImageList({ hostId, images, onChanged }: { hostId: string; images: Image[]; onChanged: () => void }) {
  const [busy, setBusy] = useState<Record<string, boolean>>({})
  const [confirm, setConfirm] = useState<Image[]>()
  const removable = images.filter((i) => i.containers === 0)
  const sel = useSelection(removable.map((i) => i.id))
  const chosen = removable.filter((i) => sel.selected.has(i.id))
  const dangling = removable.filter((i) => i.tags.length === 0)
  const total = images.reduce((n, i) => n + i.size, 0)
  const reclaimable = removable.reduce((n, i) => n + i.size, 0)

  const remove = async (targets: Image[]) => {
    setConfirm(undefined)
    setBusy((b) => ({ ...b, ...Object.fromEntries(targets.map((i) => [i.id, true])) }))
    await removeMany(targets, label, 'image', (i) => api.removeImage(hostId, i.id))
    setBusy((b) => ({ ...b, ...Object.fromEntries(targets.map((i) => [i.id, false])) }))
    sel.clear()
    onChanged()
  }

  const all = removable.length > 0 && chosen.length === removable.length
  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">
        {images.length} images · {formatBytes(total)} total ·{' '}
        <span className="text-foreground">{formatBytes(reclaimable)}</span> unused
      </p>
      <SelectionBar
        count={chosen.length}
        noun="image"
        onClear={sel.clear}
        onRemove={() => setConfirm(chosen)}
        shortcuts={
          <>
            <Button
              variant="outline"
              size="sm"
              disabled={removable.length === 0}
              onClick={() => sel.set(removable.map((i) => i.id))}
            >
              Select unused ({removable.length})
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={dangling.length === 0}
              onClick={() => sel.set(dangling.map((i) => i.id))}
            >
              Select dangling ({dangling.length})
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
                  onCheckedChange={(v) => sel.set(v === true ? removable.map((i) => i.id) : [])}
                />
              </TableHead>
              <TableHead>Image</TableHead>
              <TableHead>ID</TableHead>
              <TableHead className="text-right">Size</TableHead>
              <TableHead>Created</TableHead>
              <TableHead>Used by</TableHead>
              <TableHead className="w-0" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {images.map((img) => (
              <TableRow key={img.id} data-state={sel.selected.has(img.id) ? 'selected' : undefined}>
                <TableCell>
                  <SelectBox img={img} sel={sel} />
                </TableCell>
                <TableCell className="max-w-80">
                  <Tags img={img} />
                </TableCell>
                <TableCell className="font-mono text-xs text-muted-foreground">{shortId(img.id)}</TableCell>
                <TableCell className="text-right tabular-nums">{formatBytes(img.size)}</TableCell>
                <TableCell className="text-sm">{timeAgo(img.created)}</TableCell>
                <TableCell>
                  <Usage img={img} />
                </TableCell>
                <TableCell>
                  <RemoveButton img={img} busy={busy[img.id]} onClick={() => setConfirm([img])} />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      <ul className="-mx-1 divide-y md:hidden">
        {images.map((img) => (
          <li
            key={img.id}
            className={cn('flex items-start gap-3 px-1 py-3', sel.selected.has(img.id) && 'bg-primary/5')}
          >
            <SelectBox img={img} sel={sel} className="mt-1" />
            <div className="min-w-0 flex-1 space-y-1">
              <Tags img={img} />
              <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                <span className="tabular-nums">{formatBytes(img.size)}</span>
                <span className="font-mono">{shortId(img.id)}</span>
                <span>{timeAgo(img.created)}</span>
                <Usage img={img} />
              </div>
            </div>
            <RemoveButton img={img} busy={busy[img.id]} onClick={() => setConfirm([img])} large />
          </li>
        ))}
      </ul>

      <BulkRemoveDialog
        open={!!confirm}
        onOpenChange={(o) => !o && setConfirm(undefined)}
        title={`Remove ${confirm?.length ?? 0} image${confirm?.length === 1 ? '' : 's'}?`}
        description="Each image is deleted with all of its tags. You can pull it again later."
        names={(confirm ?? []).map((i) => (i.tags.length ? i.tags.join(', ') : label(i)))}
        onConfirm={() => confirm && remove(confirm)}
      />
    </div>
  )
}

function SelectBox({
  img,
  sel,
  className,
}: {
  img: Image
  sel: { selected: Set<string>; toggle: (id: string, on: boolean) => void }
  className?: string
}) {
  const box = (
    <Checkbox
      className={className}
      aria-label={`Select ${label(img)}`}
      disabled={img.containers > 0}
      checked={sel.selected.has(img.id)}
      onCheckedChange={(v) => sel.toggle(img.id, v === true)}
    />
  )
  if (img.containers === 0) return box
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className={cn('inline-flex', className)}>{box}</span>
      </TooltipTrigger>
      <TooltipContent>In use: remove its containers first</TooltipContent>
    </Tooltip>
  )
}

function Tags({ img }: { img: Image }) {
  if (img.tags.length === 0) {
    return (
      <span className="flex items-center gap-2 text-muted-foreground">
        <span className="font-mono text-sm">&lt;none&gt;</span>
        <Badge variant="outline" className="font-normal">
          dangling
        </Badge>
      </span>
    )
  }
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-1.5">
      <span className="truncate font-mono text-sm font-medium">{img.tags[0]}</span>
      {img.tags.slice(1).map((t) => (
        <Badge key={t} variant="outline" className="max-w-48 truncate font-mono font-normal">
          {t}
        </Badge>
      ))}
    </div>
  )
}

function Usage({ img }: { img: Image }) {
  if (img.containers === 0) return <StatusBadge tone="gray">unused</StatusBadge>
  return (
    <StatusBadge tone={img.running > 0 ? 'green' : 'amber'}>
      {img.containers} container{img.containers === 1 ? '' : 's'}
      {img.running > 0 && ` · ${img.running} running`}
    </StatusBadge>
  )
}

function RemoveButton({
  img,
  busy,
  onClick,
  large,
}: {
  img: Image
  busy?: boolean
  onClick: () => void
  large?: boolean
}) {
  if (busy) return <Loader2 className="mx-2 my-2 size-4 animate-spin text-muted-foreground" />
  return (
    <Button
      variant="ghost"
      size={large ? 'icon' : 'icon-sm'}
      aria-label={`Remove ${label(img)}`}
      disabled={img.containers > 0}
      onClick={onClick}
      className="text-muted-foreground hover:text-destructive"
    >
      <Trash2 />
    </Button>
  )
}
