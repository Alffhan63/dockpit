# Architecture

## Components

| Component | Runs on | Role |
|---|---|---|
| `server/` controller (Go) | one Linux server, behind nginx | API, embedded UI, SQLite state, holds agent connections |
| `agent/` (Go, static binary) | every Docker host | talks to local Docker, samples metrics, answers the controller |
| `web/` (React, Vite, shadcn/ui) | browser | dashboard, embedded into the controller binary at build time |

`go.work` ties the Go modules together. The controller imports the wire types
from `dockpit/agent/protocol`, so each message has one definition.

## Connection model: agents dial out

Each agent keeps one WebSocket to `wss://<controller>/agent/connect`, with
`Authorization: Bearer <token>`. Consequences:

- Docker hosts (home Mac mini, STB, servers behind NAT) need no inbound port.
- The agent, which is root-equivalent on its host, has no network surface.
- The controller is the only public component, and sits behind nginx + login.

The agent pings every 20s (below nginx's default 60s idle timeout) and
reconnects with exponential backoff (1s to 30s).

## Wire protocol (`agent/protocol`)

JSON messages on the agent socket:

| type | direction | purpose |
|---|---|---|
| `hello` | agent → controller | first message: OS, arch, Docker and agent version |
| `request` | controller → agent | `{id, method, params}` |
| `stream` | agent → controller | one chunk of a streaming result (logs) |
| `response` | agent → controller | ends a request: `result`, or `error` + optional HTTP-style `code` |
| `cancel` | controller → agent | stop working on request `id` |

Methods: `containers.list|start|stop|restart|remove|logs`, `host.status`.
Requests run concurrently and are matched by id. When a browser closes a log
view, or an HTTP request times out, the controller sends `cancel` and the
agent stops the Docker call.

**Backpressure:** each stream may queue 64 chunks on the controller. A browser
that cannot keep up gets its stream ended ("too slow, reconnect") instead of
stalling the agent's shared connection.

## Agent internals

- `internal/docker`: minimal Docker Engine client over the unix socket (no
  SDK): list, start/stop/restart/remove, inspect, logs (8-byte frame demux for
  non-TTY containers, line splitting, 16 KiB line cap), info, one-shot stats.
  The socket path is resolved per connection, so Docker may start after the agent.
- `internal/metrics`: host metrics via gopsutil (Linux and macOS, no cgo) every
  5s. Container stats are sampled only while someone viewed containers in the
  last 2 minutes; CPU % is computed from two consecutive one-shot samples, the
  same formula as `docker stats`.
- `internal/rpc`: maps methods to Docker calls, validates every container
  reference again (the agent owns the socket), batches log lines (200 lines or
  100 ms).
- `internal/transport`: connection loop, cancellation, TLS (`COCKPIT_CA_FILE`).

## Controller internals

- `internal/storage`: SQLite (pure-Go driver). Tables: `hosts` (token as
  SHA-256), `sessions` (id as SHA-256), `settings` (admin bcrypt hash).
- `internal/hosts`: live agent registry, request/stream multiplexing,
  registration (slug IDs, 256-bit tokens).
- `internal/api`: HTTP routes, session auth, CSRF header check, login rate
  limit (5 failures / 15 min per client IP), log WebSocket bridge.
- `internal/webui`: the built UI via `go:embed`.

## Security model

| Risk area | Control |
|---|---|
| Docker API exposure | Docker socket is only reached locally by the agent; `DOCKER_HOST=tcp://` is refused |
| Arbitrary execution | Fixed method set; no exec, no shell, no raw Docker passthrough |
| Input validation | Host IDs `^[a-z0-9][a-z0-9-]{0,62}$`, container refs `^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`, checked on controller and agent |
| Agent authentication | Per-host random token, stored hashed, revocable; rotation disconnects the agent |
| Transport | Agent refuses unencrypted URLs except loopback; TLS verification always on |
| Dashboard access | bcrypt password, rate-limited login, 30-day session in `HttpOnly`, `Secure` (behind TLS), `SameSite=Strict` cookie |
| CSRF / CSWSH | `X-Requested-With` header on writes; WebSocket Origin must match Host |
| XSS / framing | CSP `default-src 'self'`, `X-Frame-Options: DENY`, no inline scripts |
| Secrets in logs | Tokens and passwords are never logged |

`X-Real-IP` and `X-Forwarded-Proto` are trusted only from loopback, i.e. from
an nginx on the same machine.
