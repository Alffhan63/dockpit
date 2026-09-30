import { useEffect, useMemo, useRef, useState } from 'react'
import {
  ArrowLeft,
  Boxes,
  Cpu,
  Database,
  EllipsisVertical,
  HardDrive,
  FolderGit2,
  KeyRound,
  Layers,
  MemoryStick,
  RefreshCw,
  Search,
  Sparkles,
  Trash2,
} from 'lucide-react'
import { toast } from 'sonner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardAction, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { AgentSetup } from '@/components/AgentSetup'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { CleanupPanel } from '@/components/CleanupPanel'
import { ContainerTable } from '@/components/ContainerTable'
import { ImageList } from '@/components/ImageList'
import { ProjectList } from '@/components/ProjectList'
import { PullToRefresh } from '@/components/PullToRefresh'
import { HostStatusBadge } from '@/components/StatusBadge'
import { VolumeList } from '@/components/VolumeList'
import { HistoryCard } from '@/components/HistoryCard'
import { IpList } from '@/components/IpList'
import { usePoll } from '@/hooks/usePoll'
import { HOST_TABS, href, type HostTab } from '@/hooks/useRoute'
import { useSwipe } from '@/hooks/useSwipe'
import { api, ApiError } from '@/lib/api'
import { formatBytes, formatPercent, formatUptime, osLabel, percent, timeAgo } from '@/lib/format'
import { tap } from '@/lib/haptics'
import { cn } from '@/lib/utils'

export function HostPage({ hostId, tab }: { hostId: string; tab: HostTab }) {
  const host = usePoll(() => api.host(hostId), 5000, hostId)
  const online = host.data?.online ?? false
  const status = usePoll(() => api.hostStatus(hostId), 5000, online ? hostId : null)
  // Only the visible tab polls.
  const needsContainers = tab === 'containers' || tab === 'projects'
  const containers = usePoll(() => api.containers(hostId), 5000, online && needsContainers ? hostId : null)
  const images = usePoll(() => api.images(hostId), 10000, online && tab === 'images' ? hostId : null)
  // Volume sizes come from system df, so poll gently.
  const volumes = usePoll(() => api.volumes(hostId), 30000, online && tab === 'volumes' ? hostId : null)
  const tabList = useRef<HTMLDivElement>(null)
  useEffect(() => {
    tabList.current?.querySelector('[data-state="active"]')?.scrollIntoView({ block: 'nearest', inline: 'nearest' })
  }, [tab])

  // Tabs: tap, or swipe left/right on phones (like Kashly). The URL holds the
  // tab so reload and back work; replace() keeps tab flips out of history.
  const swipeArea = useRef<HTMLDivElement>(null)
  const prevTab = useRef(tab)
  const slide = Math.sign(HOST_TABS.indexOf(tab) - HOST_TABS.indexOf(prevTab.current))
  useEffect(() => {
    prevTab.current = tab
  }, [tab])
  const goTab = (t: HostTab) => location.replace(href({ page: 'host', hostId, tab: t }))
  useSwipe(swipeArea, (dir) => {
    const next = HOST_TABS[HOST_TABS.indexOf(tab) + (dir === 'left' ? 1 : -1)]
    if (next) {
      tap()
      goTab(next)
    }
  })

  const [volumeQuery, setVolumeQuery] = useState('')
  const visibleVolumes = useMemo(() => {
    const q = volumeQuery.trim().toLowerCase()
    return (volumes.data ?? [])
      .filter((v) => !q || v.name.toLowerCase().includes(q) || (v.compose_project ?? '').toLowerCase().includes(q))
      .sort((a, b) => a.containers - b.containers || b.size - a.size)
  }, [volumes.data, volumeQuery])

  const [imageQuery, setImageQuery] = useState('')
  const visibleImages = useMemo(() => {
    const q = imageQuery.trim().toLowerCase()
    return (images.data ?? [])
      .filter((i) => !q || i.id.includes(q) || i.tags.some((t) => t.toLowerCase().includes(q)))
      .sort((a, b) => b.created - a.created)
  }, [images.data, imageQuery])

  const [query, setQuery] = useState('')
  const [showStopped, setShowStopped] = useState(true)
  const [confirm, setConfirm] = useState<'rotate' | 'delete'>()
  const [newToken, setNewToken] = useState<string>()

  const visible = useMemo(() => {
    const q = query.trim().toLowerCase()
    return (containers.data ?? [])
      .filter((c) => showStopped || c.state === 'running')
      .filter((c) => !q || [c.name, c.image, c.id, c.compose_project ?? ''].some((v) => v.toLowerCase().includes(q)))
      .sort((a, b) => Number(b.state === 'running') - Number(a.state === 'running') || a.name.localeCompare(b.name))
  }, [containers.data, query, showStopped])

  if (host.error instanceof ApiError && host.error.status === 404) {
    return (
      <div className="space-y-4">
        <BackLink />
        <Alert>
          <AlertDescription>Host “{hostId}” does not exist.</AlertDescription>
        </Alert>
      </div>
    )
  }

  const h = host.data
  const m = status.data?.metrics
  const d = status.data?.docker

  const rotate = async () => {
    try {
      setNewToken(await api.rotateToken(hostId))
      host.refresh()
    } catch (err) {
      toast.error((err as Error).message)
    }
  }
  const remove = async () => {
    try {
      await api.deleteHost(hostId)
      toast.success(`Removed ${h?.name ?? hostId}`)
      location.hash = '#/'
    } catch (err) {
      toast.error((err as Error).message)
    }
  }

  const refreshAll = () =>
    Promise.all([host.refresh(), status.refresh(), containers.refresh(), images.refresh(), volumes.refresh()])

  return (
    <PullToRefresh onRefresh={refreshAll}>
      <div ref={swipeArea} className="space-y-6">
        <BackLink />
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0 space-y-1">
            <div className="flex items-center gap-3">
              <h1 className="text-2xl font-semibold tracking-tight">{h?.name ?? <Skeleton className="h-8 w-40" />}</h1>
              {h && <HostStatusBadge online={h.online} />}
            </div>
            {h && (
              <p className="text-sm text-muted-foreground">
                {h.info.os ? `${osLabel(h.info.os)} / ${h.info.arch}` : 'Agent never connected'}
                {h.info.docker_version && ` · Docker ${h.info.docker_version}`}
                {d?.operating_system && ` (${d.operating_system})`}
                {h.info.agent_version && ` · agent ${h.info.agent_version}`}
                {!h.online && h.last_seen_at && ` · last seen ${timeAgo(h.last_seen_at)}`}
              </p>
            )}
            {h && (
              <IpList className="pt-1" publicIP={h.public_ip} addresses={status.data?.addresses ?? h.info.addresses} />
            )}
          </div>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline" size="icon" aria-label="Host actions">
                <EllipsisVertical />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={() => setConfirm('rotate')}>
                <KeyRound /> New agent token…
              </DropdownMenuItem>
              <DropdownMenuItem variant="destructive" onClick={() => setConfirm('delete')}>
                <Trash2 /> Remove host…
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>

        {h && !h.online && (
          <Alert>
            <AlertDescription>
              The agent is not connected. Containers and metrics appear once it connects to this controller.
            </AlertDescription>
          </Alert>
        )}

        {h?.online && (
          <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
            <Stat
              icon={<Cpu />}
              label="CPU"
              value={m ? formatPercent(m.cpu_percent) : undefined}
              detail={m && `${m.cpu_cores} cores`}
            />
            <Stat
              icon={<MemoryStick />}
              label="Memory"
              value={m ? formatPercent(percent(m.mem_used, m.mem_total)) : undefined}
              detail={m && `${formatBytes(m.mem_used)} of ${formatBytes(m.mem_total)}`}
            />
            <Stat
              icon={<HardDrive />}
              label="Disk"
              value={m?.disk_total ? formatPercent(percent(m.disk_used, m.disk_total)) : undefined}
              detail={m?.disk_total ? `${formatBytes(m.disk_used)} of ${formatBytes(m.disk_total)}` : undefined}
            />
            <Stat
              icon={<Boxes />}
              label="Containers"
              value={d ? `${d.running} / ${d.containers}` : undefined}
              detail={m && `running · up ${formatUptime(m.uptime_seconds)}`}
            />
          </div>
        )}

        {h?.online && <HistoryCard hostId={hostId} />}

        {h?.online && (
          <div>
            <Tabs value={tab} onValueChange={(v) => goTab(v as HostTab)}>
              {/* Scrolls sideways on narrow phones; data-no-swipe keeps that from flipping tabs. */}
              <TabsList
                ref={tabList}
                data-no-swipe
                className="w-full justify-start overflow-x-auto [scrollbar-width:none] sm:w-auto [&::-webkit-scrollbar]:hidden"
              >
                <TabsTrigger value="containers" className={tabClass}>
                  <Boxes /> Containers
                  {d && <span className="hidden text-muted-foreground tabular-nums sm:inline">{d.containers}</span>}
                </TabsTrigger>
                <TabsTrigger value="projects" className={tabClass}>
                  <FolderGit2 /> Projects
                </TabsTrigger>
                <TabsTrigger value="images" className={tabClass}>
                  <Layers /> Images
                  {d && <span className="hidden text-muted-foreground tabular-nums sm:inline">{d.images}</span>}
                </TabsTrigger>
                <TabsTrigger value="volumes" className={tabClass}>
                  <Database /> Volumes
                </TabsTrigger>
                <TabsTrigger value="cleanup" className={tabClass}>
                  <Sparkles /> Cleanup
                </TabsTrigger>
              </TabsList>
            </Tabs>
            <div
              key={tab}
              className={cn(
                'mt-4 duration-200 animate-in fade-in',
                slide > 0 && 'slide-in-from-right-8',
                slide < 0 && 'slide-in-from-left-8',
              )}
            >
              {tab === 'projects' ? (
                containers.error ? (
                  <Alert variant="destructive">
                    <AlertDescription>{containers.error.message}</AlertDescription>
                  </Alert>
                ) : containers.loading ? (
                  <ListSkeleton />
                ) : (
                  <ProjectList hostId={hostId} containers={containers.data ?? []} onChanged={refreshAll} />
                )
              ) : tab === 'volumes' ? (
                <Card>
                  <CardHeader>
                    <CardTitle>Volumes</CardTitle>
                    <CardAction>
                      <Button variant="ghost" size="icon-sm" aria-label="Refresh" onClick={() => volumes.refresh()}>
                        <RefreshCw />
                      </Button>
                    </CardAction>
                    <div className="col-span-full pt-2">
                      <div className="relative w-full max-w-xs">
                        <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
                        <Input
                          placeholder="Filter by name or project…"
                          value={volumeQuery}
                          onChange={(e) => setVolumeQuery(e.target.value)}
                          className="pl-8"
                        />
                      </div>
                    </div>
                  </CardHeader>
                  <CardContent>
                    {volumes.error ? (
                      <Alert variant="destructive">
                        <AlertDescription>{volumes.error.message}</AlertDescription>
                      </Alert>
                    ) : volumes.loading ? (
                      <ListSkeleton />
                    ) : visibleVolumes.length === 0 ? (
                      <p className="py-8 text-center text-sm text-muted-foreground">No volumes match.</p>
                    ) : (
                      <VolumeList hostId={hostId} volumes={visibleVolumes} onChanged={volumes.refresh} />
                    )}
                  </CardContent>
                </Card>
              ) : tab === 'cleanup' ? (
                <CleanupPanel hostId={hostId} onChanged={() => status.refresh()} />
              ) : tab === 'containers' ? (
                <Card>
                  <CardHeader>
                    <CardTitle>Containers</CardTitle>
                    <CardAction>
                      <Button variant="ghost" size="icon-sm" aria-label="Refresh" onClick={() => containers.refresh()}>
                        <RefreshCw />
                      </Button>
                    </CardAction>
                    <div className="col-span-full flex flex-wrap items-center gap-4 pt-2">
                      <div className="relative w-full max-w-xs">
                        <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
                        <Input
                          placeholder="Filter by name, image, project…"
                          value={query}
                          onChange={(e) => setQuery(e.target.value)}
                          className="pl-8"
                        />
                      </div>
                      <div className="flex items-center gap-2">
                        <Switch id="stopped" checked={showStopped} onCheckedChange={setShowStopped} />
                        <Label htmlFor="stopped">Show stopped</Label>
                      </div>
                    </div>
                  </CardHeader>
                  <CardContent>
                    {containers.error ? (
                      <Alert variant="destructive">
                        <AlertDescription>{containers.error.message}</AlertDescription>
                      </Alert>
                    ) : containers.loading ? (
                      <ListSkeleton />
                    ) : visible.length === 0 ? (
                      <p className="py-8 text-center text-sm text-muted-foreground">No containers match.</p>
                    ) : (
                      <ContainerTable hostId={hostId} containers={visible} onChanged={containers.refresh} />
                    )}
                  </CardContent>
                </Card>
              ) : (
                <Card>
                  <CardHeader>
                    <CardTitle>Images</CardTitle>
                    <CardAction>
                      <Button variant="ghost" size="icon-sm" aria-label="Refresh" onClick={() => images.refresh()}>
                        <RefreshCw />
                      </Button>
                    </CardAction>
                    <div className="col-span-full pt-2">
                      <div className="relative w-full max-w-xs">
                        <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
                        <Input
                          placeholder="Filter by tag or ID…"
                          value={imageQuery}
                          onChange={(e) => setImageQuery(e.target.value)}
                          className="pl-8"
                        />
                      </div>
                    </div>
                  </CardHeader>
                  <CardContent>
                    {images.error ? (
                      <Alert variant="destructive">
                        <AlertDescription>{images.error.message}</AlertDescription>
                      </Alert>
                    ) : images.loading ? (
                      <ListSkeleton />
                    ) : visibleImages.length === 0 ? (
                      <p className="py-8 text-center text-sm text-muted-foreground">No images match.</p>
                    ) : (
                      <ImageList
                        hostId={hostId}
                        images={visibleImages}
                        onChanged={() => Promise.all([images.refresh(), status.refresh()])}
                      />
                    )}
                  </CardContent>
                </Card>
              )}
            </div>
          </div>
        )}

        <ConfirmDialog
          open={confirm === 'rotate'}
          onOpenChange={(o) => !o && setConfirm(undefined)}
          title="Issue a new agent token?"
          description="The current token stops working immediately and the agent disconnects until you restart it with the new token."
          confirm="Issue token"
          onConfirm={rotate}
        />
        <ConfirmDialog
          open={confirm === 'delete'}
          onOpenChange={(o) => !o && setConfirm(undefined)}
          title={`Remove ${h?.name ?? hostId}?`}
          description="The host is removed from the dashboard and its token is revoked. Containers on the host are not touched."
          confirm="Remove host"
          destructive
          onConfirm={remove}
        />
        <Dialog open={!!newToken} onOpenChange={(o) => !o && setNewToken(undefined)}>
          <DialogContent className="sm:max-w-lg">
            <DialogHeader>
              <DialogTitle>New agent token</DialogTitle>
            </DialogHeader>
            {newToken && <AgentSetup token={newToken} />}
            <DialogFooter>
              <Button onClick={() => setNewToken(undefined)}>Done</Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </PullToRefresh>
  )
}

const tabClass = 'shrink-0 px-2 text-xs sm:px-2.5 sm:text-sm'

function ListSkeleton() {
  return (
    <div className="space-y-2">
      {[0, 1, 2, 3].map((i) => (
        <Skeleton key={i} className="h-10 w-full" />
      ))}
    </div>
  )
}

function BackLink() {
  return (
    <Button variant="ghost" size="sm" className="-ml-2" asChild>
      <a href="#/">
        <ArrowLeft /> Hosts
      </a>
    </Button>
  )
}

function Stat({
  icon,
  label,
  value,
  detail,
}: {
  icon: React.ReactNode
  label: string
  value?: string
  detail?: string
}) {
  return (
    <Card size="sm">
      <CardContent className="space-y-1">
        <div className="flex items-center gap-2 text-sm text-muted-foreground [&_svg]:size-4">
          {icon}
          {label}
        </div>
        {value === undefined ? (
          <Skeleton className="h-7 w-20" />
        ) : (
          <div className="text-xl font-semibold tabular-nums sm:text-2xl">{value}</div>
        )}
        <div className="h-4 truncate text-xs text-muted-foreground">{detail}</div>
      </CardContent>
    </Card>
  )
}
