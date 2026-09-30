import { useEffect, useState } from 'react'
import { Loader2, ShieldCheck, ShieldOff } from 'lucide-react'
import QRCode from 'qrcode'
import { toast } from 'sonner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { usePoll } from '@/hooks/usePoll'
import { api } from '@/lib/api'

function CodeInput({ id, value, onChange }: { id: string; value: string; onChange: (v: string) => void }) {
  return (
    <Input
      id={id}
      inputMode="numeric"
      autoComplete="one-time-code"
      maxLength={6}
      placeholder="123456"
      value={value}
      onChange={(e) => onChange(e.target.value.replace(/\D/g, ''))}
      className="w-40 text-center font-mono tracking-[0.3em]"
    />
  )
}

export function SecurityPanel() {
  const state = usePoll(api.twoFA, 3_600_000, '2fa')
  const [setup, setSetup] = useState<{ secret: string; uri: string }>()
  const [qr, setQr] = useState<string>()
  const [code, setCode] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()

  useEffect(() => {
    if (!setup) return setQr(undefined)
    QRCode.toDataURL(setup.uri, { margin: 1, width: 192 }).then(setQr, () => setQr(undefined))
  }, [setup])

  if (!state.data) return <Skeleton className="h-48 rounded-xl" />
  const enabled = state.data.enabled

  const run = async (fn: () => Promise<void>) => {
    setBusy(true)
    setError(undefined)
    try {
      await fn()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          {enabled ? <ShieldCheck className="size-5 text-success" aria-hidden /> : <ShieldOff className="size-5 text-muted-foreground" aria-hidden />}
          Two-factor authentication
        </CardTitle>
        <CardDescription>
          {enabled
            ? 'On. Signing in needs your password and a code from your authenticator app.'
            : 'Off. Add a second step to sign in using an authenticator app (Google Authenticator, Authy, 1Password…).'}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {error && (
          <Alert variant="destructive" role="alert">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {!enabled && !setup && (
          <Button
            onClick={() =>
              run(async () => {
                setSetup(await api.twoFASetup())
                setCode('')
              })
            }
            disabled={busy}
          >
            {busy && <Loader2 className="animate-spin" />} Set up
          </Button>
        )}

        {!enabled && setup && (
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault()
              run(async () => {
                await api.twoFAEnable(code)
                setSetup(undefined)
                setCode('')
                toast.success('Two-factor authentication is on')
                await state.refresh()
              })
            }}
          >
            <ol className="list-decimal space-y-3 pl-5 text-sm">
              <li>
                Scan this with your authenticator app, or type the key by hand.
                <div className="mt-2 flex flex-wrap items-center gap-4">
                  {qr ? <img src={qr} alt="QR code for the authenticator app" width={192} height={192} className="rounded-md bg-white p-1" /> : <Skeleton className="size-48" />}
                  <div className="min-w-0 space-y-1">
                    <div className="text-xs text-muted-foreground">Key</div>
                    <code className="block font-mono text-sm break-all select-all">{setup.secret}</code>
                    <a href={setup.uri} className="text-xs text-primary underline underline-offset-2">
                      Open in authenticator app
                    </a>
                  </div>
                </div>
              </li>
              <li>
                <Label htmlFor="enable-code" className="mb-1.5 inline-block font-normal">
                  Enter the 6-digit code it shows to confirm.
                </Label>
                <div>
                  <CodeInput id="enable-code" value={code} onChange={setCode} />
                </div>
              </li>
            </ol>
            <p className="text-xs text-muted-foreground">
              Lost your phone? On the server run <code className="font-mono">cockpit-server 2fa-reset</code>.
            </p>
            <div className="flex gap-2">
              <Button type="submit" disabled={busy || code.length !== 6}>
                {busy && <Loader2 className="animate-spin" />} Turn on
              </Button>
              <Button type="button" variant="ghost" onClick={() => setSetup(undefined)}>
                Cancel
              </Button>
            </div>
          </form>
        )}

        {enabled && (
          <form
            className="space-y-3"
            onSubmit={(e) => {
              e.preventDefault()
              run(async () => {
                await api.twoFADisable(password, code)
                setPassword('')
                setCode('')
                toast.success('Two-factor authentication is off')
                await state.refresh()
              })
            }}
          >
            <p className="text-sm text-muted-foreground">To turn it off, confirm with your password and a current code.</p>
            <div className="flex flex-wrap items-end gap-3">
              <div className="space-y-1.5">
                <Label htmlFor="disable-pw">Password</Label>
                <Input id="disable-pw" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} className="w-56" />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="disable-code">Code</Label>
                <CodeInput id="disable-code" value={code} onChange={setCode} />
              </div>
              <Button type="submit" variant="destructive" disabled={busy || !password || code.length !== 6}>
                {busy && <Loader2 className="animate-spin" />} Turn off
              </Button>
            </div>
          </form>
        )}
      </CardContent>
    </Card>
  )
}
