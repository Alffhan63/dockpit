import { useCallback, useEffect, useState } from 'react'
import { Toaster } from '@/components/ui/sonner'
import { TooltipProvider } from '@/components/ui/tooltip'
import { AppShell } from '@/components/AppShell'
import { useRoute } from '@/hooks/useRoute'
import { api, UNAUTHORIZED_EVENT } from '@/lib/api'
import { DashboardPage } from '@/pages/DashboardPage'
import { HostPage } from '@/pages/HostPage'
import { LoginPage } from '@/pages/LoginPage'
import { LogsPage } from '@/pages/LogsPage'
import { SettingsPage } from '@/pages/SettingsPage'

type Session = { authenticated: boolean; password_set: boolean }

export function App() {
  const [session, setSession] = useState<Session>()
  const [error, setError] = useState<string>()
  const route = useRoute()

  const check = useCallback(() => {
    api
      .session()
      .then((s) => {
        setSession(s)
        setError(undefined)
      })
      .catch((e: Error) => setError(e.message))
  }, [])

  useEffect(() => {
    check()
    const onUnauthorized = () => setSession((s) => (s ? { ...s, authenticated: false } : s))
    window.addEventListener(UNAUTHORIZED_EVENT, onUnauthorized)
    return () => window.removeEventListener(UNAUTHORIZED_EVENT, onUnauthorized)
  }, [check])

  const logout = async () => {
    await api.logout().catch(() => {})
    setSession((s) => (s ? { ...s, authenticated: false } : s))
  }

  let content
  if (error) {
    content = (
      <div className="flex min-h-svh items-center justify-center p-4 text-sm text-destructive">
        Controller unreachable: {error}
      </div>
    )
  } else if (!session) {
    content = null
  } else if (!session.authenticated) {
    content = <LoginPage passwordSet={session.password_set} onLogin={check} />
  } else {
    content = (
      <AppShell onLogout={logout} routeKey={route.page === 'host' ? `host/${route.hostId}` : route.page === 'settings' ? 'settings' : location.hash} fill={route.page === 'logs' || route.page === 'project-logs'}>
        {route.page === 'dashboard' && <DashboardPage />}
        {route.page === 'host' && <HostPage key={route.hostId} hostId={route.hostId} tab={route.tab ?? 'containers'} />}
        {route.page === 'settings' && <SettingsPage tab={route.tab ?? 'notifications'} />}
        {route.page === 'project-logs' && (
          <LogsPage key={`${route.hostId}/project/${route.project}`} hostId={route.hostId} project={route.project} />
        )}
        {route.page === 'logs' && (
          <LogsPage key={`${route.hostId}/${route.container}`} hostId={route.hostId} container={route.container} />
        )}
      </AppShell>
    )
  }

  return (
    <TooltipProvider>
      {content}
      <Toaster richColors position="top-center" />
    </TooltipProvider>
  )
}
