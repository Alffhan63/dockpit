import { TriangleAlert } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { CopyButton } from './CopyButton'

// AgentSetup shows a freshly issued token and how to install the agent.
// The install command deliberately leaves the token out: the script asks for
// it with hidden input, so it never lands in shell history.
export function AgentSetup({ token }: { token: string }) {
  const origin = location.origin
  const install = `curl -fsSL ${origin}/install.sh | sh -s -- --url ${origin}`
  return (
    // min-w-0: dialog content is a grid, and the long token must not widen it.
    <div className="min-w-0 space-y-4">
      <Alert>
        <TriangleAlert />
        <AlertDescription>
          This token is shown only once. It grants Docker access on the host, so keep it private. If you lose it,
          issue a new one.
        </AlertDescription>
      </Alert>
      <div className="space-y-1.5">
        <div className="text-sm font-medium">1. Copy the agent token</div>
        <div className="flex items-center gap-1 rounded-md border bg-muted/50 py-1 pr-1 pl-3">
          <code className="flex-1 truncate font-mono text-xs">{token}</code>
          <CopyButton value={token} label="Copy token" />
        </div>
      </div>
      <div className="space-y-1.5">
        <div className="text-sm font-medium">2. Run on the host, then paste the token when asked</div>
        <div className="relative rounded-md border bg-muted/50">
          <pre className="overflow-x-auto p-3 pr-10 font-mono text-xs leading-relaxed">{install}</pre>
          <div className="absolute top-1 right-1">
            <CopyButton value={install} label="Copy install command" />
          </div>
        </div>
        <p className="text-xs text-muted-foreground">
          Linux installs a systemd service (uses sudo); macOS installs a launchd agent for your user. The agent
          connects out to this controller, so the host needs no open ports.
        </p>
      </div>
    </div>
  )
}
