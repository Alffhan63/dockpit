#!/bin/sh
# Docker Cockpit agent installer for Linux (systemd) and macOS (launchd).
#
#   curl -fsSL https://cockpit.example.com/install.sh | sh -s -- --url https://cockpit.example.com
#
# The script downloads the agent for this OS/arch from GitHub Releases,
# verifies its SHA-256 checksum, and installs it as a service. It asks for the
# agent token (from "Add host" in the dashboard) without echoing it, so the
# token does not end up in shell history.
#
# Re-run it to upgrade: the installed URL and token are kept, so no token is
# needed. Pass --token to switch tokens. Remove with --uninstall.

set -eu

# GitHub repository that publishes the releases. Change this to your fork.
REPO="${COCKPIT_REPO:-Alffhan63/dockpit}"
DOWNLOAD_BASE="${COCKPIT_DOWNLOAD_BASE:-https://github.com/$REPO/releases}"

URL="${COCKPIT_CONTROLLER_URL:-}"
TOKEN="${COCKPIT_TOKEN:-}"
VERSION="${COCKPIT_VERSION:-latest}"
BINARY=""
CA_FILE=""
UNINSTALL=0

LABEL="com.dockpit.agent"
SERVICE="cockpit-agent"
AGENT_USER="cockpit-agent"

usage() {
	cat >&2 <<'EOF'
Usage: install.sh [options]

  --url URL        controller URL, e.g. https://cockpit.example.com
                   (or COCKPIT_CONTROLLER_URL)
  --token TOKEN    agent token (or COCKPIT_TOKEN). Prompted for if missing.
  --version TAG    release to install, e.g. v1.0.0 (default: latest)
  --binary PATH    install this agent binary instead of downloading one
  --ca-file PATH   extra CA certificate to trust (self-signed controller)
  --uninstall      stop and remove the agent
  -h, --help       show this help
EOF
}

say() { printf '%s\n' "$*" >&2; }
step() { printf '==> %s\n' "$*" >&2; }
die() {
	say "error: $*"
	exit 1
}

while [ $# -gt 0 ]; do
	case "$1" in
	--url) [ $# -ge 2 ] || die "--url needs a value"; URL=$2; shift 2 ;;
	--token) [ $# -ge 2 ] || die "--token needs a value"; TOKEN=$2; shift 2 ;;
	--version) [ $# -ge 2 ] || die "--version needs a value"; VERSION=$2; shift 2 ;;
	--binary) [ $# -ge 2 ] || die "--binary needs a value"; BINARY=$2; shift 2 ;;
	--ca-file) [ $# -ge 2 ] || die "--ca-file needs a value"; CA_FILE=$2; shift 2 ;;
	--uninstall) UNINSTALL=1; shift ;;
	-h | --help) usage; exit 0 ;;
	*) usage; die "unknown option: $1" ;;
	esac
done

# --- platform ---------------------------------------------------------------

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
linux | darwin) ;;
*) die "unsupported OS: $OS (Linux and macOS only)" ;;
esac

case "$(uname -m)" in
x86_64 | amd64) ARCH=amd64 ;;
aarch64 | arm64) ARCH=arm64 ;;
armv7* | armv8l) ARCH=arm ;;
*) die "unsupported CPU architecture: $(uname -m)" ;;
esac
if [ "$OS" = darwin ] && [ "$ARCH" = arm ]; then
	die "unsupported platform: darwin/arm"
fi

SUDO=""
if [ "$OS" = linux ] && [ "$(id -u)" -ne 0 ]; then
	command -v sudo >/dev/null 2>&1 || die "run as root or install sudo"
	SUDO="sudo"
fi
if [ "$OS" = darwin ] && [ "$(id -u)" -eq 0 ]; then
	die "on macOS run this as your normal user, not root: Docker Desktop runs per user"
fi

TMP=$(mktemp -d)
cleanup() {
	# Restore echo if we were interrupted while reading the token.
	if (: </dev/tty) 2>/dev/null; then stty echo </dev/tty 2>/dev/null || true; fi
	rm -rf "$TMP"
}
trap cleanup EXIT INT TERM

# --- helpers ----------------------------------------------------------------

fetch() { # url dest
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL --proto '=https' --tlsv1.2 "$1" -o "$2"
	elif command -v wget >/dev/null 2>&1; then
		wget -q -O "$2" "$1"
	else
		die "curl or wget is required"
	fi
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | cut -d' ' -f1
	else
		die "sha256sum or shasum is required to verify the download"
	fi
}

# /dev/tty can exist without being usable (CI, docker run without -t).
have_tty() { (: </dev/tty) 2>/dev/null; }

xml_escape() {
	printf '%s' "$1" | sed -e 's/&/\&amp;/g' -e 's/</\&lt;/g' -e 's/>/\&gt;/g'
}

# --- uninstall --------------------------------------------------------------

uninstall_linux() {
	if command -v systemctl >/dev/null 2>&1; then
		$SUDO systemctl disable --now "$SERVICE" 2>/dev/null || true
		$SUDO rm -f "/etc/systemd/system/$SERVICE.service"
		$SUDO systemctl daemon-reload 2>/dev/null || true
	fi
	$SUDO rm -f /usr/local/bin/cockpit-agent /etc/cockpit-agent.env /etc/cockpit-agent-ca.pem
	if id "$AGENT_USER" >/dev/null 2>&1; then
		$SUDO userdel "$AGENT_USER" 2>/dev/null || $SUDO deluser "$AGENT_USER" 2>/dev/null || true
	fi
}

uninstall_darwin() {
	launchctl bootout "gui/$(id -u)/$LABEL" 2>/dev/null || true
	rm -f "$HOME/Library/LaunchAgents/$LABEL.plist" "$HOME/.local/bin/cockpit-agent"
	rm -rf "$HOME/.config/cockpit-agent"
}

if [ "$UNINSTALL" -eq 1 ]; then
	step "Removing the Docker Cockpit agent"
	"uninstall_$OS"
	say "Done. Remove the host in the dashboard to revoke its token."
	exit 0
fi

# --- inputs -----------------------------------------------------------------

# Upgrades: reuse the installed URL, token and CA unless new ones are given,
# so re-running the one-line installer needs no token (it is shown only once).
LINUX_ENV=/etc/cockpit-agent.env
LINUX_CA=/etc/cockpit-agent-ca.pem
MAC_PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
MAC_CA="$HOME/.config/cockpit-agent/ca.pem"
existing() { # key -> value from the installed config, or nothing
	if [ "$OS" = linux ]; then
		$SUDO sh -c "[ -f $LINUX_ENV ] && sed -n 's/^$1=//p' $LINUX_ENV" 2>/dev/null || true
	elif [ -f "$MAC_PLIST" ]; then
		plutil -extract "EnvironmentVariables.$1" raw "$MAC_PLIST" 2>/dev/null || true
	fi
}
if [ -z "$TOKEN" ]; then
	TOKEN=$(existing COCKPIT_TOKEN)
	[ -n "$TOKEN" ] && step "Keeping the installed agent token"
fi
if [ -z "$URL" ]; then
	URL=$(existing COCKPIT_CONTROLLER_URL)
fi
if [ -z "$CA_FILE" ] && [ -n "$(existing COCKPIT_CA_FILE)" ]; then
	CA_FILE=$(existing COCKPIT_CA_FILE)
fi

if [ -z "$URL" ] && have_tty; then
	printf 'Controller URL (e.g. https://cockpit.example.com): ' >/dev/tty
	IFS= read -r URL </dev/tty
fi
URL=${URL%/}
case "$URL" in
https://*) ;;
http://localhost* | http://127.0.0.1* | http://\[::1\]*) ;;
http://*) die "controller URL must use https:// (plain http is only allowed for localhost)" ;;
*) die "missing or invalid --url" ;;
esac

if [ -z "$TOKEN" ]; then
	have_tty || die "no token: pass --token or set COCKPIT_TOKEN"
	# Turn echo off before showing the prompt so a fast paste is never echoed.
	stty -echo </dev/tty
	printf 'Agent token (from "Add host", input hidden): ' >/dev/tty
	IFS= read -r TOKEN </dev/tty
	stty echo </dev/tty
	printf '\n' >/dev/tty
fi
case "$TOKEN" in
*[!0-9a-f]* | "") die "invalid token: expected the 64-character token from the dashboard" ;;
esac
[ "${#TOKEN}" -eq 64 ] || die "invalid token: expected 64 characters, got ${#TOKEN}"

if [ -n "$CA_FILE" ] && ! { [ -r "$CA_FILE" ] || $SUDO test -r "$CA_FILE"; }; then
	die "cannot read --ca-file $CA_FILE"
fi

# Early, friendly check. The agent itself verifies TLS when it connects.
if command -v curl >/dev/null 2>&1; then
	set -- -fsS --max-time 10
	[ -n "$CA_FILE" ] && set -- "$@" --cacert "$CA_FILE"
	if ! curl "$@" "$URL/healthz" >/dev/null 2>&1; then
		say "warning: $URL/healthz is not reachable from this host; installing anyway"
	fi
fi

# --- binary -----------------------------------------------------------------

ASSET="cockpit-agent-$OS-$ARCH"
if [ -n "$BINARY" ]; then
	[ -f "$BINARY" ] || die "no such file: $BINARY"
	step "Using local binary $BINARY"
	cp "$BINARY" "$TMP/$ASSET"
else
	if [ "$VERSION" = latest ]; then
		base="$DOWNLOAD_BASE/latest/download"
	else
		base="$DOWNLOAD_BASE/download/$VERSION"
	fi
	step "Downloading $ASSET ($VERSION)"
	fetch "$base/$ASSET" "$TMP/$ASSET" || die "download failed: $base/$ASSET"
	fetch "$base/checksums.txt" "$TMP/checksums.txt" || die "download failed: $base/checksums.txt"
	want=$(awk -v f="$ASSET" '$2 == f || $2 == "*" f { print $1 }' "$TMP/checksums.txt")
	[ -n "$want" ] || die "$ASSET is not listed in checksums.txt"
	got=$(sha256 "$TMP/$ASSET")
	[ "$got" = "$want" ] || die "checksum mismatch for $ASSET (expected $want, got $got)"
	step "Checksum verified"
fi
chmod 755 "$TMP/$ASSET"

# --- install: Linux ---------------------------------------------------------

install_linux() {
	step "Installing /usr/local/bin/cockpit-agent"
	$SUDO install -m 755 "$TMP/$ASSET" /usr/local/bin/cockpit-agent

	if ! id "$AGENT_USER" >/dev/null 2>&1; then
		step "Creating system user $AGENT_USER"
		$SUDO useradd --system --no-create-home --shell /usr/sbin/nologin "$AGENT_USER" 2>/dev/null ||
			$SUDO adduser -S -H -s /sbin/nologin "$AGENT_USER" ||
			die "could not create user $AGENT_USER"
	fi

	groups_line=""
	if getent group docker >/dev/null 2>&1; then
		groups_line="SupplementaryGroups=docker"
	else
		say "warning: no 'docker' group on this host; the agent may not be able to open the Docker socket"
	fi

	step "Writing /etc/cockpit-agent.env (mode 600)"
	umask 077
	{
		printf 'COCKPIT_CONTROLLER_URL=%s\n' "$URL"
		printf 'COCKPIT_TOKEN=%s\n' "$TOKEN"
		if [ -n "$CA_FILE" ]; then printf 'COCKPIT_CA_FILE=%s\n' "$LINUX_CA"; fi
	} >"$TMP/env"
	$SUDO install -m 600 "$TMP/env" /etc/cockpit-agent.env
	if [ -n "$CA_FILE" ] && [ "$CA_FILE" != "$LINUX_CA" ]; then
		$SUDO install -m 644 "$CA_FILE" "$LINUX_CA"
	fi

	if ! command -v systemctl >/dev/null 2>&1 || [ ! -d /run/systemd/system ]; then
		say ""
		say "systemd is not running here, so no service was installed."
		say "Start the agent with your init system, as user $AGENT_USER, using:"
		say "  env file: /etc/cockpit-agent.env"
		say "  command:  /usr/local/bin/cockpit-agent"
		return
	fi

	step "Installing systemd service $SERVICE"
	cat >"$TMP/unit" <<EOF
[Unit]
Description=Docker Cockpit agent
After=network-online.target docker.service
Wants=network-online.target

[Service]
User=$AGENT_USER
$groups_line
EnvironmentFile=/etc/cockpit-agent.env
ExecStart=/usr/local/bin/cockpit-agent
Restart=always
RestartSec=5
NoNewPrivileges=true
ProtectSystem=full
ProtectHome=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF
	$SUDO install -d -m 755 /etc/systemd/system
	$SUDO install -m 644 "$TMP/unit" "/etc/systemd/system/$SERVICE.service"
	$SUDO systemctl daemon-reload
	$SUDO systemctl enable "$SERVICE" >/dev/null 2>&1
	$SUDO systemctl restart "$SERVICE"

	sleep 3
	if $SUDO systemctl is-active --quiet "$SERVICE"; then
		step "Agent is running"
	else
		say "warning: the service is not active; recent logs:"
	fi
	$SUDO journalctl -u "$SERVICE" -n 5 --no-pager 2>/dev/null || true
	say ""
	say "Logs: journalctl -u $SERVICE -f"
}

# --- install: macOS ---------------------------------------------------------

install_darwin() {
	bin_dir="$HOME/.local/bin"
	conf_dir="$HOME/.config/cockpit-agent"
	plist="$HOME/Library/LaunchAgents/$LABEL.plist"
	log="$HOME/Library/Logs/cockpit-agent.log"

	step "Installing $bin_dir/cockpit-agent"
	mkdir -p "$bin_dir" "$HOME/Library/LaunchAgents" "$HOME/Library/Logs"
	install -m 755 "$TMP/$ASSET" "$bin_dir/cockpit-agent"
	xattr -d com.apple.quarantine "$bin_dir/cockpit-agent" 2>/dev/null || true

	ca_entry=""
	if [ -n "$CA_FILE" ]; then
		mkdir -p "$conf_dir"
		[ "$CA_FILE" = "$MAC_CA" ] || install -m 644 "$CA_FILE" "$MAC_CA"
		ca_entry="    <key>COCKPIT_CA_FILE</key>
    <string>$(xml_escape "$conf_dir/ca.pem")</string>"
	fi

	step "Writing $plist (mode 600)"
	umask 077
	cat >"$plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>$LABEL</string>
  <key>ProgramArguments</key>
  <array>
    <string>$(xml_escape "$bin_dir/cockpit-agent")</string>
  </array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>COCKPIT_CONTROLLER_URL</key>
    <string>$(xml_escape "$URL")</string>
    <key>COCKPIT_TOKEN</key>
    <string>$TOKEN</string>
$ca_entry
  </dict>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>$(xml_escape "$log")</string>
  <key>StandardErrorPath</key>
  <string>$(xml_escape "$log")</string>
</dict>
</plist>
EOF
	chmod 600 "$plist"

	step "Starting launchd agent $LABEL"
	launchctl bootout "gui/$(id -u)/$LABEL" 2>/dev/null || true
	launchctl bootstrap "gui/$(id -u)" "$plist"

	sleep 3
	tail -n 5 "$log" 2>/dev/null || true
	say ""
	say "Logs: tail -f $log"
}

"install_$OS"
say "Done. The host should show as Online in the dashboard within a few seconds."
