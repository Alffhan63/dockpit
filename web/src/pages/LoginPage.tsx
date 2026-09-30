import { useState } from 'react'
import { Eye, EyeOff, TriangleAlert } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api } from '@/lib/api'

export function LoginPage({ passwordSet, onLogin }: { passwordSet: boolean; onLogin: () => void }) {
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string>()
  const [busy, setBusy] = useState(false)
  const [show, setShow] = useState(false)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError(undefined)
    try {
      await api.login(password)
      onLogin()
    } catch (err) {
      setError((err as Error).message)
      setPassword('')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex min-h-dvh items-center justify-center bg-background p-4 pt-[calc(env(safe-area-inset-top)+1rem)]">
      <Card className="w-full max-w-sm">
        <CardHeader className="items-center text-center">
          <img src="/icon-192.png" alt="" className="mx-auto mb-2 size-14 rounded-2xl" />
          <CardTitle className="text-xl text-primary">Docker Cockpit</CardTitle>
          <CardDescription>Sign in to manage your hosts</CardDescription>
        </CardHeader>
        <CardContent>
          {!passwordSet ? (
            <Alert>
              <TriangleAlert />
              <AlertTitle>No password set</AlertTitle>
              <AlertDescription>
                On the controller, run <code className="font-mono">cockpit-server passwd</code>, then reload.
              </AlertDescription>
            </Alert>
          ) : (
            <form onSubmit={submit} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="password">Password</Label>
                <div className="relative">
                  <Input
                    id="password"
                    type={show ? 'text' : 'password'}
                    autoComplete="current-password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    className="pr-9"
                    autoFocus
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    className="absolute top-1/2 right-1 -translate-y-1/2 text-muted-foreground"
                    aria-label={show ? 'Hide password' : 'Show password'}
                    onClick={() => setShow((v) => !v)}
                  >
                    {show ? <EyeOff /> : <Eye />}
                  </Button>
                </div>
              </div>
              {error && <p className="text-sm text-destructive">{error}</p>}
              <Button type="submit" className="w-full" disabled={busy || !password}>
                {busy ? 'Signing in…' : 'Sign in'}
              </Button>
            </form>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
