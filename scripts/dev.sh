#!/usr/bin/env bash
# Runs controller, an agent for this machine and the web UI together for
# local development. Ctrl-C stops everything.
set -euo pipefail
cd "$(dirname "$0")/.."

if lsof -nP -iTCP:8080 -sTCP:LISTEN >/dev/null 2>&1; then
  echo "port 8080 is already in use; stop that process first" >&2
  exit 1
fi

make server agent
[ -d web/node_modules ] || (cd web && npm install)

export COCKPIT_DB="${COCKPIT_DB:-data/cockpit.db}"
mkdir -p "$(dirname "$COCKPIT_DB")"
server=./bin/cockpit-server

if ! "$server" passwd -check 2>/dev/null; then
  if [ -t 0 ]; then
    echo "No dashboard password yet. Choose one:"
    "$server" passwd
  else
    echo "WARNING: no dashboard password yet. Set one with:" >&2
    echo "  COCKPIT_DB=$COCKPIT_DB $server passwd" >&2
  fi
fi

# Register this machine as host "local" once; issue a fresh token every run.
token=$("$server" host token local -q 2>/dev/null || "$server" host add local -q)

trap 'trap - EXIT; kill 0' INT TERM EXIT

"$server" &
server_pid=$!
until curl -sf 127.0.0.1:8080/healthz >/dev/null; do
  kill -0 "$server_pid" 2>/dev/null || { echo "controller failed to start" >&2; exit 1; }
  sleep 0.2
done

COCKPIT_CONTROLLER_URL=http://127.0.0.1:8080 COCKPIT_TOKEN="$token" ./bin/cockpit-agent &
(cd web && npx vite) &

echo
echo "  Dashboard: http://127.0.0.1:5173"
echo "  API:       http://127.0.0.1:8080"
echo
wait
