import { useEffect, useState } from 'react'

// Minimal hash router: #/, #/hosts/:id[/tab], #/hosts/:id/logs/:container
export type HostTab = 'containers' | 'projects' | 'images' | 'volumes' | 'cleanup'
export type SettingsTab = 'notifications' | 'security' | 'automation' | 'audit'
export const SETTINGS_TABS: SettingsTab[] = ['notifications', 'security', 'automation', 'audit']

// Left-to-right order, which is also the swipe order.
export const HOST_TABS: HostTab[] = ['containers', 'projects', 'images', 'volumes', 'cleanup']

export type Route =
  | { page: 'dashboard' }
  | { page: 'host'; hostId: string; tab?: HostTab }
  | { page: 'logs'; hostId: string; container: string }
  | { page: 'project-logs'; hostId: string; project: string }
  | { page: 'settings'; tab?: SettingsTab }

function parse(hash: string): Route {
  const parts = hash.replace(/^#\/?/, '').split('/').filter(Boolean).map(decodeURIComponent)
  if (parts[0] === 'settings') {
    return { page: 'settings', tab: SETTINGS_TABS.find((t) => t === parts[1]) ?? 'notifications' }
  }
  if (parts[0] === 'hosts' && parts[1]) {
    if (parts[2] === 'logs' && parts[3]) return { page: 'logs', hostId: parts[1], container: parts[3] }
    if (parts[2] === 'project-logs' && parts[3]) return { page: 'project-logs', hostId: parts[1], project: parts[3] }
    const tab = HOST_TABS.find((t) => t === parts[2]) ?? 'containers'
    return { page: 'host', hostId: parts[1], tab }
  }
  return { page: 'dashboard' }
}

export function href(route: Route): string {
  switch (route.page) {
    case 'dashboard':
      return '#/'
    case 'host':
      return `#/hosts/${encodeURIComponent(route.hostId)}${route.tab && route.tab !== 'containers' ? `/${route.tab}` : ''}`
    case 'logs':
      return `#/hosts/${encodeURIComponent(route.hostId)}/logs/${encodeURIComponent(route.container)}`
    case 'project-logs':
      return `#/hosts/${encodeURIComponent(route.hostId)}/project-logs/${encodeURIComponent(route.project)}`
    case 'settings':
      return `#/settings${route.tab && route.tab !== 'notifications' ? `/${route.tab}` : ''}`
  }
}

export function useRoute(): Route {
  const [route, setRoute] = useState(() => parse(location.hash))
  useEffect(() => {
    const onChange = () => setRoute(parse(location.hash))
    window.addEventListener('hashchange', onChange)
    return () => window.removeEventListener('hashchange', onChange)
  }, [])
  return route
}
