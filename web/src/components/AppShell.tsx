import { useEffect, useRef, type ReactNode } from 'react'
import { Container, Download, LogOut, Monitor, Moon, Sun } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useInstallPrompt } from '@/hooks/useInstallPrompt'
import { useTheme } from '@/hooks/useTheme'
import { cn } from '@/lib/utils'

// AppShell fills the viewport: a fixed header and a scrolling <main>. In the
// installed app the header sits under the status bar (safe-area insets).
export function AppShell({
  children,
  routeKey,
  fill,
  onLogout,
}: {
  children: ReactNode
  routeKey: string
  // fill gives the page exactly the viewport height (the log viewer scrolls
  // internally); other pages grow and scroll <main>.
  fill?: boolean
  onLogout: () => void
}) {
  const { theme, resolved, setTheme } = useTheme()
  const install = useInstallPrompt()
  const main = useRef<HTMLElement>(null)

  // <main> stays mounted across pages; start each page at the top.
  useEffect(() => {
    main.current?.scrollTo({ top: 0 })
  }, [routeKey])

  return (
    <div className="flex h-dvh flex-col bg-background text-foreground">
      <header className="z-30 shrink-0 border-b bg-card pt-[env(safe-area-inset-top)] pr-[env(safe-area-inset-right)] pl-[env(safe-area-inset-left)]">
        <div className="mx-auto flex h-14 max-w-7xl items-center gap-3 px-4">
          <a href="#/" className="flex items-center gap-2 font-semibold">
            <span className="flex size-7 items-center justify-center rounded-lg bg-primary text-primary-foreground">
              <Container className="size-4" />
            </span>
            <span className="text-primary">Docker Cockpit</span>
          </a>
          <div className="ml-auto flex items-center gap-1">
            {install && (
              <Button variant="outline" size="sm" onClick={install}>
                <Download /> <span className="hidden sm:inline">Install app</span>
              </Button>
            )}
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="icon" aria-label="Menu">
                  {resolved === 'dark' ? <Moon /> : <Sun />}
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onClick={() => setTheme('light')} data-active={theme === 'light'}>
                  <Sun /> Light
                </DropdownMenuItem>
                <DropdownMenuItem onClick={() => setTheme('dark')} data-active={theme === 'dark'}>
                  <Moon /> Dark
                </DropdownMenuItem>
                <DropdownMenuItem onClick={() => setTheme('system')} data-active={theme === 'system'}>
                  <Monitor /> System
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem variant="destructive" onClick={onLogout}>
                  <LogOut /> Log out
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>
      </header>
      <main
        ref={main}
        data-scroll-root
        className="min-h-0 flex-1 overflow-y-auto pr-[env(safe-area-inset-right)] pb-[env(safe-area-inset-bottom)] pl-[env(safe-area-inset-left)]"
      >
        <div
          key={routeKey}
          className={cn(
            'mx-auto max-w-7xl p-4 duration-200 animate-in fade-in slide-in-from-bottom-1 md:p-6',
            fill ? 'h-full' : 'min-h-full',
          )}
        >
          {children}
        </div>
      </main>
    </div>
  )
}
