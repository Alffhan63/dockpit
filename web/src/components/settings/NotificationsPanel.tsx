import { useEffect, useState } from 'react'
import { Loader2, Send } from 'lucide-react'
import { toast } from 'sonner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { usePoll } from '@/hooks/usePoll'
import { api, type NotifyConfig } from '@/lib/api'

function Field({ id, label, hint, children }: { id: string; label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      {children}
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  )
}

function RuleSwitch({ id, label, hint, checked, onChange }: { id: string; label: string; hint: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <div className="flex items-center justify-between gap-4 py-2">
      <Label htmlFor={id} className="flex-col items-start gap-0.5">
        <span>{label}</span>
        <span className="text-xs font-normal text-muted-foreground">{hint}</span>
      </Label>
      <Switch id={id} checked={checked} onCheckedChange={onChange} />
    </div>
  )
}

export function NotificationsPanel() {
  const stored = usePoll(api.notifications, 3_600_000, 'notifications')
  const [cfg, setCfg] = useState<NotifyConfig>()
  const [busy, setBusy] = useState<'save' | 'test'>()
  const [error, setError] = useState<string>()
  useEffect(() => {
    if (stored.data) setCfg(stored.data)
  }, [stored.data])

  if (!cfg) return <Skeleton className="h-96 rounded-xl" />

  const set = (fn: (c: NotifyConfig) => void) => {
    const next = structuredClone(cfg)
    fn(next)
    setCfg(next)
  }
  const num = (v: string) => Math.max(0, Math.min(100, Number(v) || 0))

  const save = async () => {
    setBusy('save')
    setError(undefined)
    try {
      setCfg(await api.saveNotifications(cfg))
      toast.success('Notification settings saved')
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(undefined)
    }
  }
  const test = async () => {
    setBusy('test')
    setError(undefined)
    try {
      await api.testNotifications(cfg.channels)
      toast.success('Test message sent')
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(undefined)
    }
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>Alerts</CardTitle>
          <CardDescription>Get a message when something breaks, even when the dashboard is closed.</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between gap-4">
            <Label htmlFor="notify-enabled" className="flex-col items-start gap-0.5">
              <span>Send notifications</span>
              <span className="text-xs font-normal text-muted-foreground">Needs at least one channel below.</span>
            </Label>
            <Switch id="notify-enabled" checked={cfg.enabled} onCheckedChange={(v) => set((c) => (c.enabled = v))} />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Channels</CardTitle>
          <CardDescription>Use one or several. Secrets are stored on the controller and shown hidden.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          <fieldset className="space-y-3">
            <legend className="mb-1 text-sm font-medium">Telegram</legend>
            <div className="grid gap-3 sm:grid-cols-2">
              <Field id="tg-token" label="Bot token" hint="From @BotFather.">
                <Input id="tg-token" autoComplete="off" value={cfg.channels.telegram.bot_token} onChange={(e) => set((c) => (c.channels.telegram.bot_token = e.target.value))} placeholder="123456:ABC…" />
              </Field>
              <Field id="tg-chat" label="Chat ID" hint="Message your bot first, then use its chat id.">
                <Input id="tg-chat" autoComplete="off" value={cfg.channels.telegram.chat_id} onChange={(e) => set((c) => (c.channels.telegram.chat_id = e.target.value))} placeholder="123456789" />
              </Field>
            </div>
          </fieldset>
          <fieldset className="space-y-3">
            <legend className="mb-1 text-sm font-medium">ntfy</legend>
            <div className="grid gap-3 sm:grid-cols-3">
              <Field id="ntfy-server" label="Server" hint="Empty = https://ntfy.sh">
                <Input id="ntfy-server" value={cfg.channels.ntfy.server} onChange={(e) => set((c) => (c.channels.ntfy.server = e.target.value))} placeholder="https://ntfy.sh" />
              </Field>
              <Field id="ntfy-topic" label="Topic">
                <Input id="ntfy-topic" value={cfg.channels.ntfy.topic} onChange={(e) => set((c) => (c.channels.ntfy.topic = e.target.value))} placeholder="my-cockpit-alerts" />
              </Field>
              <Field id="ntfy-token" label="Access token" hint="Optional.">
                <Input id="ntfy-token" autoComplete="off" value={cfg.channels.ntfy.token ?? ''} onChange={(e) => set((c) => (c.channels.ntfy.token = e.target.value))} />
              </Field>
            </div>
          </fieldset>
          <fieldset className="space-y-3">
            <legend className="mb-1 text-sm font-medium">Webhook</legend>
            <Field id="webhook-url" label="URL" hint='Receives POST {"title","body","level"} as JSON.'>
              <Input id="webhook-url" autoComplete="off" value={cfg.channels.webhook.url} onChange={(e) => set((c) => (c.channels.webhook.url = e.target.value))} placeholder="https://…" />
            </Field>
          </fieldset>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>What to alert on</CardTitle>
        </CardHeader>
        <CardContent className="divide-y">
          <RuleSwitch id="r-offline" label="Host offline" hint="Agent disconnected for more than 2 minutes." checked={cfg.rules.host_offline} onChange={(v) => set((c) => (c.rules.host_offline = v))} />
          <RuleSwitch id="r-health" label="Container unhealthy" hint="A healthcheck fails on two checks in a row." checked={cfg.rules.container_unhealthy} onChange={(v) => set((c) => (c.rules.container_unhealthy = v))} />
          <RuleSwitch id="r-exit" label="Container crashed" hint="Stopped with an error or out of memory. Stops you do from the dashboard are ignored." checked={cfg.rules.container_exit} onChange={(v) => set((c) => (c.rules.container_exit = v))} />
          <RuleSwitch id="r-loop" label="Restart loop" hint="3 or more restarts within 10 minutes." checked={cfg.rules.restart_loop} onChange={(v) => set((c) => (c.rules.restart_loop = v))} />
          <div className="grid gap-3 pt-3 sm:grid-cols-2">
            <Field id="r-disk" label="Disk usage above (%)" hint="0 turns it off.">
              <Input id="r-disk" type="number" min={0} max={100} value={cfg.rules.disk_percent} onChange={(e) => set((c) => (c.rules.disk_percent = num(e.target.value)))} />
            </Field>
            <Field id="r-mem" label="Memory usage above (%)" hint="0 turns it off.">
              <Input id="r-mem" type="number" min={0} max={100} value={cfg.rules.mem_percent} onChange={(e) => set((c) => (c.rules.mem_percent = num(e.target.value)))} />
            </Field>
          </div>
        </CardContent>
      </Card>

      {error && (
        <Alert variant="destructive" role="alert">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <div className="flex flex-wrap gap-2">
        <Button onClick={save} disabled={!!busy}>
          {busy === 'save' && <Loader2 className="animate-spin" />} Save
        </Button>
        <Button variant="outline" onClick={test} disabled={!!busy}>
          {busy === 'test' ? <Loader2 className="animate-spin" /> : <Send />} Send test message
        </Button>
      </div>
    </div>
  )
}
