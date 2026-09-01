#!/usr/bin/env bash
# Install whatsapp-mcp as a systemd service on a Linux server.
#
# Deliberately isolated so it cannot collide with anything already on the box:
#
#   own user      whatsappmcp, no login shell, no sudo
#   own directory /var/lib/whatsapp-mcp, mode 700, owned by that user
#   own port      8787 by default, bound to 127.0.0.1 only
#   own service   whatsapp-mcp.service
#
# It binds to loopback, so nothing is exposed to the internet by this script.
# Put it behind your existing reverse proxy, or reach it over an SSH tunnel.
#
# Usage:  sudo bash install.sh [--port 8787]

set -euo pipefail

PORT=8787
while [[ $# -gt 0 ]]; do
  case $1 in
    --port) PORT="$2"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 1 ;;
  esac
done

USER_NAME=whatsappmcp
DATA_DIR=/var/lib/whatsapp-mcp
BIN=/usr/local/bin/whatsapp-mcp

[[ $EUID -eq 0 ]] || { echo "run with sudo" >&2; exit 1; }

# Refuse rather than fight for a port something else already holds.
if ss -ltn 2>/dev/null | grep -q ":${PORT} "; then
  echo "port ${PORT} is already in use. Pick another with --port." >&2
  exit 1
fi

# The binary is expected at ${BIN} already. Cross compile it on your machine
# and scp it up, which avoids installing a Go toolchain on the server:
#
#   CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o whatsapp-mcp ./cmd/whatsapp-mcp
#   scp whatsapp-mcp root@your-server:/usr/local/bin/whatsapp-mcp
#
# If Go is present and the repo is public, this builds it in place instead.
if [[ ! -x "$BIN" ]]; then
  if command -v go >/dev/null; then
    echo "==> building from source"
    GOBIN=/usr/local/bin go install github.com/navidmoazzez/whatsapp-mcp/cmd/whatsapp-mcp@latest
  else
    echo "no binary at ${BIN} and no Go toolchain. Upload the binary first, see the comment above." >&2
    exit 1
  fi
fi
chmod +x "$BIN"

echo "==> creating the service user and data directory"
id -u "$USER_NAME" >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin "$USER_NAME"
mkdir -p "$DATA_DIR"
chown "$USER_NAME:$USER_NAME" "$DATA_DIR"
chmod 700 "$DATA_DIR"

# A token is mandatory. Generate one and keep it out of the unit file, which is
# world readable, by putting it in a 600 environment file instead.
ENV_FILE=/etc/whatsapp-mcp.env
if [[ ! -f "$ENV_FILE" ]]; then
  TOKEN="wamcp_$(head -c32 /dev/urandom | xxd -p -c64)"
  printf 'WAMCP_TOKEN=%s\n' "$TOKEN" > "$ENV_FILE"
  chmod 600 "$ENV_FILE"
  chown "$USER_NAME:$USER_NAME" "$ENV_FILE"
fi

echo "==> writing the systemd unit"
cat > /etc/systemd/system/whatsapp-mcp.service <<UNIT
[Unit]
Description=WhatsApp MCP server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${USER_NAME}
Group=${USER_NAME}
EnvironmentFile=${ENV_FILE}
ExecStart=${BIN} --http 127.0.0.1:${PORT} --token \${WAMCP_TOKEN} --data-dir ${DATA_DIR}
Restart=always
RestartSec=5

# Hardening. This process holds your WhatsApp session keys, so it gets no more
# of the system than it needs.
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${DATA_DIR}
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictAddressFamilies=AF_INET AF_INET6
MemoryMax=512M

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable whatsapp-mcp >/dev/null 2>&1 || true

cat <<DONE

Installed. It is NOT running yet, because WhatsApp has to be linked first and
that needs a QR code you scan in a terminal.

  1. Link it, once:

       sudo -u ${USER_NAME} ${BIN} login --data-dir ${DATA_DIR}

     Scan the QR with WhatsApp, Settings, Linked Devices, Link a Device.

  2. Start it:

       sudo systemctl start whatsapp-mcp
       systemctl status whatsapp-mcp

  3. Your bearer token:

       sudo cat ${ENV_FILE}

It listens on 127.0.0.1:${PORT} only. Nothing is exposed to the internet until
you point a reverse proxy at it.

DONE
