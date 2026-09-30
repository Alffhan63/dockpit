import type { ReactNode } from 'react'
import { Trash2, X } from 'lucide-react'
import { Button } from '@/components/ui/button'

// SelectionBar offers quick-select shortcuts, and bulk actions once
// something is selected.
export function SelectionBar({
  count,
  noun,
  shortcuts,
  onClear,
  onRemove,
}: {
  count: number
  noun: string
  shortcuts: ReactNode
  onClear: () => void
  onRemove: () => void
}) {
  if (count === 0) {
    return <div className="flex flex-wrap items-center gap-2">{shortcuts}</div>
  }
  return (
    <div className="flex flex-wrap items-center gap-2 rounded-lg border border-primary/30 bg-primary/5 px-2 py-1.5 duration-150 animate-in fade-in">
      <span className="px-1 text-sm font-medium">
        {count} {noun}
        {count === 1 ? '' : 's'} selected
      </span>
      <Button variant="ghost" size="sm" onClick={onClear}>
        <X /> Clear
      </Button>
      <Button variant="destructive" size="sm" className="ml-auto" onClick={onRemove}>
        <Trash2 /> Remove {count}
      </Button>
    </div>
  )
}
