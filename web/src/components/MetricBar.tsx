import { Progress } from '@/components/ui/progress'
import { cn } from '@/lib/utils'

export function MetricBar({ label, value, detail }: { label: string; value: number; detail?: string }) {
  const v = Math.max(0, Math.min(100, value))
  return (
    <div className="space-y-1.5">
      <div className="flex items-baseline justify-between text-sm">
        <span className="text-muted-foreground">{label}</span>
        <span className="tabular-nums">
          <span className="font-medium">{v.toFixed(0)}%</span>
          {detail && <span className="ml-1.5 text-xs text-muted-foreground">{detail}</span>}
        </span>
      </div>
      <Progress
        value={v}
        className={cn(
          'h-1.5',
          v >= 90 ? '[&>*]:bg-destructive' : v >= 75 ? '[&>*]:bg-warning' : '[&>*]:bg-primary',
        )}
      />
    </div>
  )
}
