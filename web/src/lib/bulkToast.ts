import { toast } from 'sonner'
import { runBulk } from './bulk'
import { tap } from './haptics'

interface Verbs {
  doing: string // "Removing"
  done: string // "Removed"
  noun: string // "container"
}

const plural = (n: number, noun: string) => `${n} ${noun}${n === 1 ? '' : 's'}`

// runMany applies fn to every item with a live progress toast, then
// summarises what worked and what did not. It resolves to the failures.
export async function runMany<T>(
  items: T[],
  label: (item: T) => string,
  verbs: Verbs,
  fn: (item: T) => Promise<void>,
): Promise<T[]> {
  if (items.length === 0) return []
  const id = toast.loading(`${verbs.doing} 0/${plural(items.length, verbs.noun)}…`)
  const results = await runBulk(items, fn, (done) =>
    toast.loading(`${verbs.doing} ${done}/${plural(items.length, verbs.noun)}…`, { id }),
  )
  const failed = results.filter((r) => r.error)
  const ok = results.length - failed.length
  if (failed.length === 0) {
    tap()
    toast.success(`${verbs.done} ${plural(ok, verbs.noun)}`, { id })
    return []
  }
  const detail = failed
    .slice(0, 3)
    .map((f) => `${label(f.item)}: ${f.error!.message}`)
    .join('\n')
  toast.error(`${verbs.done} ${ok}, failed ${failed.length}`, {
    id,
    description: detail + (failed.length > 3 ? `\n…and ${failed.length - 3} more` : ''),
    duration: 10000,
  })
  return failed.map((f) => f.item)
}

export function removeMany<T>(
  items: T[],
  label: (item: T) => string,
  noun: string,
  remove: (item: T) => Promise<void>,
) {
  return runMany(items, label, { doing: 'Removing', done: 'Removed', noun }, remove)
}
