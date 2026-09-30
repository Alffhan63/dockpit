import { useState } from 'react'
import { Plus } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api } from '@/lib/api'
import { AgentSetup } from './AgentSetup'

export function AddHostDialog({ onAdded }: { onAdded: () => void }) {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [token, setToken] = useState<string>()
  const [busy, setBusy] = useState(false)

  const reset = (o: boolean) => {
    setOpen(o)
    if (!o) {
      setName('')
      setToken(undefined)
    }
  }

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      const res = await api.createHost(name)
      setToken(res.token)
      onAdded()
    } catch (err) {
      toast.error((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={reset}>
      <DialogTrigger asChild>
        <Button>
          <Plus /> Add host
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        {token ? (
          <>
            <DialogHeader>
              <DialogTitle>Host added</DialogTitle>
              <DialogDescription>Start the agent on {name} to bring it online.</DialogDescription>
            </DialogHeader>
            <AgentSetup token={token} />
            <DialogFooter>
              <Button onClick={() => reset(false)}>Done</Button>
            </DialogFooter>
          </>
        ) : (
          <form onSubmit={submit} className="space-y-4">
            <DialogHeader>
              <DialogTitle>Add host</DialogTitle>
              <DialogDescription>Register a Docker host. You will get a token for its agent.</DialogDescription>
            </DialogHeader>
            <div className="space-y-2">
              <Label htmlFor="host-name">Name</Label>
              <Input
                id="host-name"
                placeholder="Mac mini"
                value={name}
                onChange={(e) => setName(e.target.value)}
                maxLength={64}
                autoFocus
              />
            </div>
            <DialogFooter>
              <Button type="submit" disabled={busy || !name.trim()}>
                {busy ? 'Adding…' : 'Add host'}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}
