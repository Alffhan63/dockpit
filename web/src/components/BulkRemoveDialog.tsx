import { useEffect, useState, type ReactNode } from 'react'
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { HoldButton } from '@/components/HoldButton'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'

// BulkRemoveDialog confirms removing several items. When some of them need
// force (running containers), the user must opt in explicitly.
export function BulkRemoveDialog({
  open,
  onOpenChange,
  title,
  description,
  names,
  forceCount = 0,
  forceLabel,
  onConfirm,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description: ReactNode
  names: string[]
  forceCount?: number
  forceLabel?: string
  onConfirm: (force: boolean) => void
}) {
  const [force, setForce] = useState(false)
  useEffect(() => {
    if (open) setForce(false)
  }, [open])
  const shown = names.slice(0, 8)
  const count = force || forceCount === 0 ? names.length : names.length - forceCount

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>
        <ul className="max-h-40 space-y-0.5 overflow-y-auto rounded-md border bg-muted/40 p-2 font-mono text-xs">
          {shown.map((n) => (
            <li key={n} className="truncate">
              {n}
            </li>
          ))}
          {names.length > shown.length && (
            <li className="text-muted-foreground">and {names.length - shown.length} more…</li>
          )}
        </ul>
        {forceCount > 0 && (
          <div className="flex items-center justify-between gap-3 rounded-md border p-3">
            <Label htmlFor="bulk-force" className="flex-col items-start gap-0.5">
              <span>{forceLabel}</span>
              <span className="text-xs font-normal text-muted-foreground">
                Otherwise those {forceCount} are skipped.
              </span>
            </Label>
            <Switch id="bulk-force" checked={force} onCheckedChange={setForce} />
          </div>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <HoldButton
            variant="destructive"
            disabled={count === 0}
            onConfirm={() => {
              onConfirm(force)
              onOpenChange(false)
            }}
          >
            Hold to remove {count}
          </HoldButton>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
