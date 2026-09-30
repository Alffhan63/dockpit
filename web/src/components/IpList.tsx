import { toast } from 'sonner'
import type { Address } from '@/lib/api'
import { cn } from '@/lib/utils'

// Private ranges the agent also treats as private: RFC 1918, 100.64/10
// (Tailscale, carrier NAT) and loopback.
function isPrivateV4(ip: string): boolean {
  const [a, b] = ip.split('.').map(Number)
  return (
    a === 10 ||
    a === 127 ||
    (a === 172 && b >= 16 && b <= 31) ||
    (a === 192 && b === 168) ||
    (a === 100 && b >= 64 && b <= 127)
  )
}

interface Chip {
  label: string
  ip: string
  public: boolean
}

// chips merges the address the controller sees with the host's interface
// addresses: public ones first, each IP once.
function chips(publicIP: string | undefined, addresses: Address[] = []): Chip[] {
  const out: Chip[] = []
  const seen = new Set<string>()
  const add = (c: Chip) => {
    if (!seen.has(c.ip)) {
      seen.add(c.ip)
      out.push(c)
    }
  }
  if (publicIP && !isPrivateV4(publicIP)) add({ label: 'Public', ip: publicIP, public: true })
  for (const a of addresses) if (a.scope === 'public') add({ label: `Public (${a.interface})`, ip: a.ip, public: true })
  for (const a of addresses) if (a.scope === 'private') add({ label: a.interface, ip: a.ip, public: false })
  // Agent and controller on the same private network: say how it connects.
  // Loopback (agent on the controller's own machine) says nothing useful.
  if (publicIP && isPrivateV4(publicIP) && !publicIP.startsWith('127.'))
    add({ label: 'via', ip: publicIP, public: false })
  return out
}

async function copy(ip: string) {
  try {
    await navigator.clipboard.writeText(ip)
    toast.success(`Copied ${ip}`)
  } catch {
    toast.error('Could not copy')
  }
}

// IpList shows the host's IPv4 addresses as small chips; a click copies one.
export function IpList({
  publicIP,
  addresses,
  limit,
  className,
}: {
  publicIP?: string
  addresses?: Address[]
  limit?: number
  className?: string
}) {
  const all = chips(publicIP, addresses)
  if (all.length === 0) return null
  const shown = limit ? all.slice(0, limit) : all
  return (
    <div className={cn('flex flex-wrap items-center gap-1.5', className)}>
      {shown.map((c) => (
        <button
          key={c.ip}
          type="button"
          onClick={(e) => {
            // Inside clickable cards: copy, do not navigate.
            e.preventDefault()
            e.stopPropagation()
            copy(c.ip)
          }}
          title={`Copy ${c.ip}`}
          className="inline-flex items-center gap-1.5 rounded-md border bg-muted/40 px-1.5 py-0.5 text-xs hover:bg-muted"
        >
          <span className={cn('size-1.5 rounded-full', c.public ? 'bg-info' : 'bg-muted-foreground')} />
          <span className="text-muted-foreground">{c.label}</span>
          <span className="font-mono">{c.ip}</span>
        </button>
      ))}
      {limit && all.length > limit && <span className="text-xs text-muted-foreground">+{all.length - limit}</span>}
    </div>
  )
}
