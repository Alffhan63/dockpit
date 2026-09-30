# Deploying

Target layout: the controller on one Linux server behind the nginx you already
run, and an agent on every Docker host (including the controller's own server,
if it runs containers).

## 1. Build

```sh
make release
```

This produces `bin/cockpit-server-linux-amd64` (UI embedded) and all
`bin/cockpit-agent-*` binaries. Use the `-arm64` variants for ARM servers.

## 2. Controller

On the controller server:

```sh
sudo install -m 755 cockpit-server-linux-amd64 /usr/local/bin/cockpit-server
sudo useradd --system --no-create-home --shell /usr/sbin/nologin cockpit
sudo cp deploy/cockpit-server.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now cockpit-server      # creates /var/lib/cockpit

# Set the dashboard password (min 12 characters)
sudo -u cockpit COCKPIT_DB=/var/lib/cockpit/cockpit.db cockpit-server passwd
```

The service listens on `127.0.0.1:8080` only.

## 3. nginx

1. Point a DNS record (e.g. `cockpit.example.com`) at the server.
2. Copy `deploy/nginx-cockpit.conf` into your sites, replace the server name
   and certificate paths (e.g. from certbot).
3. If your nginx does not define `$connection_upgrade`, add the `map` block
   from the top of that file to the `http` context.
4. `sudo nginx -t && sudo systemctl reload nginx`.

The WebSocket headers, `Host $http_host`, `X-Real-IP` and `X-Forwarded-Proto`
lines are required: they make agent connections and log streams work, keep
the session cookie `Secure`, and give login rate limiting the real client IP.

Open `https://cockpit.example.com` and sign in.

**Install on your phone:** Android/Chrome shows "Install app" (also in the
header); on iPhone open the site in Safari → Share → Add to Home Screen.
Installing needs HTTPS, which nginx provides. New versions reach installed
apps automatically after you upgrade the controller.

## 4. Agents

In the dashboard, **Add host** → name it → copy the token (shown once).
Alternatively: `sudo -u cockpit COCKPIT_DB=/var/lib/cockpit/cockpit.db cockpit-server host add "Linux Server"`.

### With the installer (recommended)

The binaries must be published as a GitHub release first (see the README,
"Releases"). Then on each host:

```sh
curl -fsSL https://cockpit.example.com/install.sh | sh -s -- --url https://cockpit.example.com
```

Paste the token when asked. What it does:

| | Linux | macOS |
|---|---|---|
| Binary | `/usr/local/bin/cockpit-agent` | `~/.local/bin/cockpit-agent` |
| Config | `/etc/cockpit-agent.env` (root, 600) | `~/Library/LaunchAgents/com.dockpit.agent.plist` (600) |
| Service | systemd `cockpit-agent`, user `cockpit-agent` in group `docker` | launchd agent for your user |
| Logs | `journalctl -u cockpit-agent -f` | `~/Library/Logs/cockpit-agent.log` |

The download is verified against the release's `checksums.txt`; a mismatch
aborts before anything is installed. Run it again to upgrade: the installed
URL, token and CA are kept. Pass `--token` to switch tokens, and
`--uninstall` to remove everything.

Without systemd (e.g. Alpine/OpenRC, containers) the binary and env file are
installed and the script tells you what to run.

Options: `--version v1.2.3`, `--binary ./cockpit-agent-linux-arm64` (hosts
without GitHub access), `--ca-file ca.pem` (self-signed controller),
`--token` / `COCKPIT_TOKEN` for automation (prefer the prompt: a token on the
command line ends up in shell history).

### Manually

Linux: copy `cockpit-agent-linux-<arch>` to `/usr/local/bin/cockpit-agent`,
create `/etc/cockpit-agent.env` (mode 600) with `COCKPIT_CONTROLLER_URL` and
`COCKPIT_TOKEN`, and install `deploy/cockpit-agent.service`.

macOS: edit and load `deploy/com.dockpit.agent.plist` (see the comment in it).

## Operations

- **Revoke a host:** remove it in the UI (or `cockpit-server host rm ID`).
  Its agent is disconnected and can no longer authenticate.
- **Lost or leaked token:** host page → "New agent token", then update the
  agent's env file and restart it.
- **Change the password:** `cockpit-server passwd` (ends all sessions).
- **Backup:** `/var/lib/cockpit/cockpit.db` (hosts, token hashes, sessions).
  Use `sqlite3 cockpit.db ".backup backup.db"` while running.
- **Self-signed controller:** set `COCKPIT_CA_FILE=/path/to/ca.pem` on agents.
- **Upgrade:** replace the binaries and restart the services. Agents reconnect
  on their own.
