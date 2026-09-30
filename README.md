# Docker Cockpit

A small personal dashboard for several Docker hosts: see what runs where, read
logs live, start/stop/restart/remove containers. Built for a homelab
"try a repo, watch it, remove it" workflow, not as a Portainer replacement.

```
Browser ──HTTPS──▶ nginx ──▶ cockpit-server (controller, 127.0.0.1:8080, SQLite)
                                 ▲          ▲
                    wss, outbound, per-host token
                                 │          │
                          cockpit-agent   cockpit-agent      (one per Docker host)
                                 │
                          local Docker unix socket
```

Agents dial **out** to the controller, so Docker hosts need no open ports, and
the Docker API is never exposed over the network.

- [docs/architecture.md](docs/architecture.md): design and wire protocol
- [docs/deploy.md](docs/deploy.md): running it on your servers behind nginx

## Features

- **Hosts:** register hosts from the UI or CLI, each with its own revocable token.
  Online/offline status, OS/arch, Docker version, CPU, memory, disk, uptime,
  container counts.
- **Containers:** list with image, status, ports, compose project, CPU and
  memory. Start, stop, restart, remove (volumes are never removed; running
  containers need an explicit "stop and remove").
- **Bulk actions:** select containers (or "Select not running") and remove
  them in one go; running ones need an explicit "stop and remove".
- **Projects:** containers grouped per `docker compose` project with Start
  all / Stop all / Remove project (optionally with the images only that
  project uses). Volumes and networks are kept.
- **Images:** list with size and usage, bulk remove unused or dangling
  images. Images any container uses cannot be removed.
- **Volumes:** size, project and which containers mount each volume.
  Mounted volumes cannot be deleted; deleting an unused one requires typing
  its name (or "delete N volumes"), because the data is gone for good.
- **Cleanup:** `docker system df` summary; clear build cache and dangling
  images. Volumes are never cleaned up in bulk.
- **History:** host CPU and memory for the last 3 hours (30s resolution),
  kept in the agent's memory; restarting the agent starts it over.
- **Logs:** live over WebSocket with stdout/stderr separated, auto-scroll that
  pauses when you scroll up, filtering, timestamps, tail size.
- **Installable app (PWA):** add it to your phone's home screen for a
  full-screen app with its own icon. Phone layout with container cards,
  pull-to-refresh and light/dark themes. Only the app shell is cached; API
  data (containers, logs, tokens) never is.
- **Security:** dashboard login (bcrypt, rate limited, `HttpOnly`/`Secure`/
  `SameSite=Strict` session cookie, CSRF header), tokens stored only as hashes,
  TLS-only agent connections to non-local controllers, strict CSP, no shell or
  arbitrary Docker API access.

## Quick start (local)

Requirements: Go 1.22+, Node 20+, Docker running (on macOS: Docker Desktop).

```sh
make dev
```

The first run asks you to choose a dashboard password. Then open
**http://127.0.0.1:5173** (Vite, hot reload). `make dev` builds everything,
registers this machine as host `local`, starts controller + agent + UI, and
stops them all on Ctrl-C. Dev data lives in `data/cockpit.db`.

## Installing an agent

Every Docker host you want to see in the dashboard runs one small agent. The
agent connects **out** to the controller over `wss://`, so the host needs no
open ports and its Docker socket never leaves the machine.

### Before you start

1. **The controller is running and reachable over HTTPS**, e.g.
   `https://cockpit.example.com` behind nginx (see [docs/deploy.md](docs/deploy.md)).
   From the host, `curl https://cockpit.example.com/healthz` should print
   `{"status":"ok"}`.
2. **A release is published** on GitHub, because the installer downloads the
   agent binary from there. Publishing one is a tag push; the release workflow
   builds everything:

   ```sh
   git tag v0.1.0 && git push origin v0.1.0
   ```

   Wait for the *release* workflow to finish (Actions tab). Without a release,
   use the offline install below.
3. **Docker is installed and running** on the host (Linux dockerd, Docker
   Desktop, OrbStack or Colima).

### Install (one command)

1. In the dashboard, click **Add host**, give it a name (e.g. "Linux Server")
   and copy the **agent token**. It is shown only once.
2. On the host, run the command from the dialog:

   ```sh
   curl -fsSL https://cockpit.example.com/install.sh | sh -s -- --url https://cockpit.example.com
   ```

3. Paste the token when asked. Input is hidden, and the token never appears
   on the command line or in shell history.
4. Within a few seconds the host shows as **Online** in the dashboard.

The installer detects the OS and CPU (`amd64`, `arm64`, `arm`), downloads
`cockpit-agent-<os>-<arch>` from the latest release, **verifies it against the
release's `checksums.txt`** (a mismatch aborts before anything is installed),
and sets it up as a service:

| | Linux | macOS |
|---|---|---|
| Runs as | system user `cockpit-agent`, group `docker` | your user (Docker Desktop runs per user) |
| Binary | `/usr/local/bin/cockpit-agent` | `~/.local/bin/cockpit-agent` |
| Config (holds the token) | `/etc/cockpit-agent.env`, root, mode 600 | `~/Library/LaunchAgents/com.dockpit.agent.plist`, mode 600 |
| Service | systemd unit `cockpit-agent`, starts on boot | launchd agent, starts at login |
| Logs | `journalctl -u cockpit-agent -f` | `tail -f ~/Library/Logs/cockpit-agent.log` |

On Linux the script uses `sudo`; on macOS run it as your normal user, not root.

### Day to day

| Task | How |
|---|---|
| Check it runs | Linux: `systemctl status cockpit-agent` · macOS: `launchctl print gui/$(id -u)/com.dockpit.agent` |
| Upgrade | Run the same install command again. The installed URL, token and CA are kept, so no token is needed |
| Install a specific version | `... \| sh -s -- --url https://cockpit.example.com --version v0.1.0` |
| Change the token | Host page → **⋮ → New agent token**, then re-run the installer with `--token <new>` (or `COCKPIT_TOKEN=<new>`) |
| Uninstall | `curl -fsSL https://cockpit.example.com/install.sh \| sh -s -- --uninstall`, then remove the host in the dashboard |

A leaked token grants Docker access on that host: issue a new one right away
(the old one stops working and the agent disconnects).

### Other ways to install

- **Offline / no GitHub access:** copy the right binary from a machine with
  the repo (`make agents` builds all of them into `bin/`), then:

  ```sh
  sh install.sh --url https://cockpit.example.com --binary ./cockpit-agent-linux-arm64
  ```

- **Self-signed controller certificate:** add `--ca-file /path/to/ca.pem`; the
  agent trusts that CA in addition to the system roots. Verification stays on.
- **Automation:** pass `COCKPIT_TOKEN=... sh install.sh --url ...` or
  `--token`. Prefer the prompt for manual installs: a token on the command
  line ends up in shell history.
- **No systemd** (Alpine/OpenRC, containers, some routers/STBs): the script
  installs the binary and `/etc/cockpit-agent.env` and tells you what to run
  with your init system.
- **Fully manual:** see [docs/deploy.md](docs/deploy.md#manually).

### Troubleshooting

| Agent log says | Meaning / fix |
|---|---|
| `controller rejected the agent token` | Wrong token, or the host was removed / its token rotated. Issue a new token and re-run the installer. |
| `is not encrypted: use https://` | The URL must be `https://`; plain `http://` is only accepted for localhost. |
| `certificate signed by unknown authority` | The controller uses a private certificate: use Let's Encrypt or `--ca-file`. |
| `no Docker socket found` | Docker is not running on the host. The agent keeps retrying and recovers once it starts. |
| `permission denied` on `docker.sock` | Linux: the `docker` group is missing or the socket is not group-readable; check `ls -l /var/run/docker.sock`. |
| Host stays **Offline**, no errors | The host cannot reach the controller: check DNS, firewall and `curl https://<controller>/healthz` from the host. |
| Installer: `checksum mismatch` | The download is corrupt or was tampered with. Nothing was installed; retry, or check the release assets. |

## Build

```sh
make test      # go vet + go test -race (agent, server) + web type-check/build
make build     # web UI (embedded into the server), bin/cockpit-server, bin/cockpit-agent
make release   # + cockpit-server-linux-{amd64,arm64}, every agent binary, checksums.txt
```

Agent binaries: `cockpit-agent-{darwin-arm64,darwin-amd64,linux-amd64,linux-arm64,linux-arm}`.
All binaries are static (no cgo).

## Controller CLI

```sh
cockpit-server                  # run the controller
cockpit-server passwd           # set the dashboard password (prompt, or one line on stdin)
cockpit-server host add NAME    # register a host, print its token once (-q: token only)
cockpit-server host list
cockpit-server host token ID    # issue a new token, revoking the old one
cockpit-server host rm ID       # remove a host and revoke its token
```

## Configuration

Controller (`cockpit-server`):

| Variable | Default | Description |
|---|---|---|
| `COCKPIT_DB` | `cockpit.db` | SQLite database path |
| `COCKPIT_LISTEN` | `127.0.0.1:8080` | Listen address. Keep it on loopback behind nginx. |
| `COCKPIT_WEB_DIR` | embedded UI | Serve the UI from a directory instead |

Agent (`cockpit-agent`):

| Variable | Default | Description |
|---|---|---|
| `COCKPIT_CONTROLLER_URL` | (required) | e.g. `https://cockpit.example.com`. Plain `http://` only for localhost. |
| `COCKPIT_TOKEN` | (required) | Token from "Add host" or `cockpit-server host add` |
| `COCKPIT_CA_FILE` | system roots | Extra PEM CA to trust (self-signed controller) |
| `COCKPIT_DISK_PATH` | `/` | Filesystem to report disk usage for |
| `DOCKER_HOST` | auto-detect | Only `unix://` sockets are accepted |

The agent finds Docker Desktop, OrbStack, Colima and Linux sockets on its own,
and keeps running (reporting errors) if Docker starts later.

## API (v1)

All `/api/v1` routes except `auth/session` and `auth/login` need a session
cookie. Non-GET requests need `X-Requested-With: cockpit`.

| Method | Path | |
|---|---|---|
| GET | `/healthz` | liveness |
| GET | `/api/v1/auth/session` | `{authenticated, password_set}` |
| POST | `/api/v1/auth/login` | `{password}` → session cookie |
| POST | `/api/v1/auth/logout` | |
| GET / POST | `/api/v1/hosts` | list / register (`{name}` → `{host, token}`) |
| GET / DELETE | `/api/v1/hosts/{id}` | get / remove |
| POST | `/api/v1/hosts/{id}/token` | new token |
| GET | `/api/v1/hosts/{id}/status` | host metrics + Docker info |
| GET | `/api/v1/hosts/{id}/containers[?all=false]` | containers |
| POST | `/api/v1/hosts/{id}/containers/{c}/{start\|stop\|restart}` | |
| DELETE | `/api/v1/hosts/{id}/containers/{c}[?force=true]` | remove |
| WS | `/api/v1/hosts/{id}/containers/{c}/logs?tail=N` | `{type: lines\|end\|error}` events |
| GET | `/api/v1/hosts/{id}/images` | images with usage counts |
| DELETE | `/api/v1/hosts/{id}/images/{imageID}` | remove an unused image (by ID) |
| GET | `/api/v1/hosts/{id}/disk` | disk usage summary |
| GET | `/api/v1/hosts/{id}/history` | last 3h of host CPU/memory |
| GET | `/api/v1/hosts/{id}/volumes` | volumes with size and usage |
| DELETE | `/api/v1/hosts/{id}/volumes/{name}` | delete an unmounted volume |
| POST | `/api/v1/hosts/{id}/prune/{build-cache\|dangling-images}` | low-risk cleanup |
| WS | `/agent/connect` | agents only (Bearer token) |

Errors are `{"error": "..."}`: 400 invalid id, 401 login required, 403 CSRF,
404 unknown host/container, 409 conflicting state, 429 rate limited,
502 agent/Docker error, 503 host offline, 504 agent timeout.

## Project layout

```
agent/    Go agent: protocol/ (wire types, shared), internal/{docker,metrics,rpc,transport}
server/   Go controller: internal/{api,auth,hosts,storage,webui}
web/      React + Vite + Tailwind + shadcn/ui
deploy/   nginx, systemd and launchd files
install.sh  agent installer (also served by the controller at /install.sh)
scripts/  dev.sh
.github/  CI and release workflows
```

## Known limitations

- Single admin user; no roles.
- No image pulls, `docker compose` deployment or exec. Deploy experiments on
  the host as usual; the cockpit manages them afterwards.
- Metrics history covers 3 hours and lives in agent memory only.
- On macOS, container CPU/memory are relative to the Docker Desktop VM; host
  metrics are the Mac's.
