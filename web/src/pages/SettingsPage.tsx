import { ArrowLeft } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { AuditPanel } from '@/components/settings/AuditPanel'
import { AutomationPanel } from '@/components/settings/AutomationPanel'
import { NotificationsPanel } from '@/components/settings/NotificationsPanel'
import { SecurityPanel } from '@/components/settings/SecurityPanel'
import { SETTINGS_TABS, href, type SettingsTab } from '@/hooks/useRoute'

const titles: Record<SettingsTab, string> = {
  notifications: 'Notifications',
  security: 'Security',
  automation: 'Automation',
  audit: 'Audit log',
}

export function SettingsPage({ tab }: { tab: SettingsTab }) {
  return (
    <div className="mx-auto max-w-3xl space-y-4">
      <Button variant="ghost" size="sm" className="-ml-2" asChild>
        <a href={href({ page: 'dashboard' })}>
          <ArrowLeft /> Hosts
        </a>
      </Button>
      <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>
      <Tabs value={tab} onValueChange={(t) => location.replace(href({ page: 'settings', tab: t as SettingsTab }))}>
        <div className="-mx-1 overflow-x-auto px-1">
          <TabsList>
            {SETTINGS_TABS.map((t) => (
              <TabsTrigger key={t} value={t}>
                {titles[t]}
              </TabsTrigger>
            ))}
          </TabsList>
        </div>
      </Tabs>
      {tab === 'notifications' && <NotificationsPanel />}
      {tab === 'security' && <SecurityPanel />}
      {tab === 'automation' && <AutomationPanel />}
      {tab === 'audit' && <AuditPanel />}
    </div>
  )
}
