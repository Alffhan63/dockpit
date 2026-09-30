import { useEffect, useState } from 'react'
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
import type { Container } from '@/lib/api'

export function RemoveContainerDialog({
  container,
  onOpenChange,
  onConfirm,
}: {
  container?: Container
  onOpenChange: (open: boolean) => void
  onConfirm: (c: Container, force: boolean) => void
}) {
  const [force, setForce] = useState(false)
  const running = container?.state === 'running'
  useEffect(() => setForce(false), [container])

  return (
    <AlertDialog open={!!container} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Remove {container?.name}?</AlertDialogTitle>
          <AlertDialogDescription>
            The container is deleted. Its image and volumes are kept. Press and hold the button to confirm.
          </AlertDialogDescription>
        </AlertDialogHeader>
        {running && (
          <div className="flex items-center justify-between gap-3 rounded-md border p-3">
            <Label htmlFor="force" className="flex-col items-start gap-0.5">
              <span>Stop and remove</span>
              <span className="text-xs font-normal text-muted-foreground">The container is running.</span>
            </Label>
            <Switch id="force" checked={force} onCheckedChange={setForce} />
          </div>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <HoldButton
            variant="destructive"
            disabled={running && !force}
            onConfirm={() => {
              if (container) onConfirm(container, force)
              onOpenChange(false)
            }}
          >
            Hold to remove
          </HoldButton>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
