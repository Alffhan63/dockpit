import { useEffect, useRef, type ReactNode } from 'react'
import { Container, Download, LogOut, Settings } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { useInstallPrompt } from '@/hooks/useInstallPrompt'
import { ThemeToggle } from './ThemeToggle'
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
  const install = useInstallPrompt()
  const main = useRef<HTMLElement>(null)

  // <main> stays mounted across pages; start each page at the top.
  useEffect(() => {
    main.current?.scrollTo({ top: 0 })
  }, [routeKey])

  return (
    <div className="flex h-dvh flex-col bg-background text-foreground">
      <a
        href="#main"
        onClick={(e) => {
          // The hash router owns location.hash, so move focus by hand.
          e.preventDefault()
          main.current?.focus()
        }}
        className="sr-only z-50 rounded-md bg-primary px-3 py-2 text-primary-foreground focus:not-sr-only focus:fixed focus:top-2 focus:left-2"
      >
        Skip to content
      </a>
      <header className="z-30 shrink-0 border-b bg-card pt-[env(safe-area-inset-top)] pr-[env(safe-area-inset-right)] pl-[env(safe-area-inset-left)]">
        <div className="mx-auto flex h-14 max-w-7xl items-center gap-3 px-4">
          <a href="#/" className="flex items-center gap-2 font-semibold">
            <span className="flex size-7 items-center justify-center rounded-lg bg-primary text-primary-foreground">
              <Container className="size-4" />
            </span>
            <span className="text-primary">Docker Cockpit</span>
          </a>
          <div className="ml-auto flex items-center gap-1.5">
            {install && (
              <Button variant="outline" size="sm" onClick={install}>
                <Download /> <span className="hidden sm:inline">Install app</span>
              </Button>
            )}
            <Button variant="ghost" size="sm" asChild>
              <a href="#/settings" aria-label="Settings" title="Settings">
                <Settings />
              </a>
            </Button>
            <ThemeToggle />
            <Separator orientation="vertical" className="mx-1 h-5" />
            <Button
              variant="ghost"
              size="sm"
              className="active:not-aria-[haspopup]:translate-y-0"
              onClick={onLogout}
              aria-label="Log out"
              title="Log out"
            >
              <LogOut /> <span className="hidden sm:inline">Log out</span>
            </Button>
          </div>
        </div>
      </header>
      <main
        id="main"
        tabIndex={-1}
        ref={main}
        data-scroll-root
        className="min-h-0 flex-1 overflow-y-auto outline-none pr-[env(safe-area-inset-right)] pb-[env(safe-area-inset-bottom)] pl-[env(safe-area-inset-left)]"
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
