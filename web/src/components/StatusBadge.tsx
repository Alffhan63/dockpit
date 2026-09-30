import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

// Status colors come from the palette tokens (success/warning/destructive/info).
const tones = {
  green: 'bg-success/15 text-success',
  amber: 'bg-warning/15 text-warning',
  red: 'bg-destructive/15 text-destructive',
  gray: 'bg-muted text-muted-foreground',
  blue: 'bg-info/15 text-info',
} as const

export type Tone = keyof typeof tones

export function StatusBadge({ tone, children, pulse }: { tone: Tone; children: React.ReactNode; pulse?: boolean }) {
  return (
    <Badge variant="secondary" className={cn('gap-1.5 border-0 font-medium', tones[tone])}>
      <span className={cn('size-1.5 rounded-full bg-current', pulse && 'animate-pulse')} />
      {children}
    </Badge>
  )
}

export function HostStatusBadge({ online }: { online: boolean }) {
  return online ? <StatusBadge tone="green">Online</StatusBadge> : <StatusBadge tone="gray">Offline</StatusBadge>
}

export function containerTone(state: string): Tone {
  switch (state) {
    case 'running':
      return 'green'
    case 'restarting':
    case 'paused':
    case 'created':
      return 'amber'
    case 'dead':
      return 'red'
    default:
      return 'gray'
  }
}
