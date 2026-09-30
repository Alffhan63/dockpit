// Types mirror dockpit/agent/protocol and the controller API.

export interface Address {
  interface: string
  ip: string
  scope: 'private' | 'public'
}

export interface HostInfo {
  os: string
  arch: string
  docker_version: string
  agent_version: string
  addresses?: Address[]
}

export interface Host {
  id: string
  name: string
  online: boolean
  created_at: string
  last_seen_at?: string
  connected_at?: string
  info: HostInfo
  // Where the agent connects from, as the controller sees it.
  public_ip?: string
}

export interface HostMetrics {
  cpu_percent: number
  cpu_cores: number
  mem_used: number
  mem_total: number
  disk_path: string
  disk_used: number
  disk_total: number
  uptime_seconds: number
  sampled_at: number
}

export interface DockerInfo {
  version: string
  operating_system: string
  containers: number
  running: number
  paused: number
  stopped: number
  images: number
  ncpu: number
  mem_total: number
}

export interface HostStatus {
  metrics: HostMetrics
  docker: DockerInfo
  addresses?: Address[]
}

export interface Port {
  ip?: string
  private_port: number
  public_port?: number
  type: string
}

export interface Container {
  id: string
  name: string
  image: string
  image_id: string
  state: string
  status: string
  created: number
  ports: Port[]
  compose_project?: string
  compose_service?: string
  compose_dir?: string
  cpu_percent?: number
  mem_usage?: number
  mem_limit?: number
}

export interface Image {
  id: string
  tags: string[]
  size: number
  created: number
  containers: number
  running: number
}

export interface DiskUsageItem {
  count: number
  active: number
  size: number
  reclaimable: number
}

export interface DiskUsage {
  images: DiskUsageItem
  containers: DiskUsageItem
  volumes: DiskUsageItem
  build_cache: DiskUsageItem
}

export interface HistoryPoint {
  t: number
  cpu: number
  mem: number
}

export interface HostHistory {
  step_seconds: number
  points: HistoryPoint[]
}

export interface Volume {
  name: string
  driver: string
  created_at?: string
  size: number // -1 when unknown
  containers: number
  running: number
  compose_project?: string
  anonymous: boolean
}

export type PruneKind = 'build-cache' | 'dangling-images'

export interface LogLine {
  s: 'stdout' | 'stderr'
  t?: string
  m: string
}

export type ContainerAction = 'start' | 'stop' | 'restart'

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message)
  }
}

// Fired when the session expires so the app can show the login page.
export const UNAUTHORIZED_EVENT = 'cockpit:unauthorized'

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: {
      Accept: 'application/json',
      // Required by the controller on state-changing requests (CSRF).
      'X-Requested-With': 'cockpit',
      ...(body !== undefined && { 'Content-Type': 'application/json' }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
    credentials: 'same-origin',
  })
  if (res.status === 204) return undefined as T
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    if (res.status === 401 && path !== '/api/v1/auth/login') {
      window.dispatchEvent(new Event(UNAUTHORIZED_EVENT))
    }
    throw new ApiError(res.status, data.error ?? `HTTP ${res.status}`)
  }
  return data as T
}

const host = (id: string) => `/api/v1/hosts/${encodeURIComponent(id)}`
const container = (hostId: string, ref: string) => `${host(hostId)}/containers/${encodeURIComponent(ref)}`

export const api = {
  session: () => request<{ authenticated: boolean; password_set: boolean }>('GET', '/api/v1/auth/session'),
  login: (password: string) => request<void>('POST', '/api/v1/auth/login', { password }),
  logout: () => request<void>('POST', '/api/v1/auth/logout'),

  hosts: () => request<{ hosts: Host[] }>('GET', '/api/v1/hosts').then((r) => r.hosts),
  host: (id: string) => request<Host>('GET', host(id)),
  hostStatus: (id: string) => request<HostStatus>('GET', `${host(id)}/status`),
  createHost: (name: string) => request<{ host: Host; token: string }>('POST', '/api/v1/hosts', { name }),
  deleteHost: (id: string) => request<void>('DELETE', host(id)),
  rotateToken: (id: string) => request<{ token: string }>('POST', `${host(id)}/token`).then((r) => r.token),

  containers: (hostId: string) =>
    request<{ containers: Container[] }>('GET', `${host(hostId)}/containers`).then((r) => r.containers),
  containerAction: (hostId: string, ref: string, action: ContainerAction) =>
    request<void>('POST', `${container(hostId, ref)}/${action}`),
  removeContainer: (hostId: string, ref: string, force: boolean) =>
    request<void>('DELETE', `${container(hostId, ref)}${force ? '?force=true' : ''}`),

  images: (hostId: string) =>
    request<{ images: Image[] }>('GET', `${host(hostId)}/images`).then((r) => r.images),
  removeImage: (hostId: string, id: string) =>
    request<void>('DELETE', `${host(hostId)}/images/${encodeURIComponent(id)}`),

  history: (hostId: string) => request<HostHistory>('GET', `${host(hostId)}/history`),
  volumes: (hostId: string) =>
    request<{ volumes: Volume[] }>('GET', `${host(hostId)}/volumes`).then((r) => r.volumes),
  removeVolume: (hostId: string, name: string) =>
    request<void>('DELETE', `${host(hostId)}/volumes/${encodeURIComponent(name)}`),
  disk: (hostId: string) => request<DiskUsage>('GET', `${host(hostId)}/disk`),
  prune: (hostId: string, kind: PruneKind) =>
    request<{ deleted: number; space_reclaimed: number }>('POST', `${host(hostId)}/prune/${kind}`),

  logsURL: (hostId: string, ref: string, tail: number) => {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
    return `${proto}//${location.host}${container(hostId, ref)}/logs?tail=${tail}`
  },
}
