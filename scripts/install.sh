#!/usr/bin/env bash
# homecast installer
# Downloads the latest homecast binary and AirConnect aircast binary,
# installs them with a systemd service, and starts the daemon.
#
# Usage: curl -fsSL https://raw.githubusercontent.com/pizzasaurusrex/homecast/main/scripts/install.sh | sh
#
# Environment overrides (for testing):
#   HOMECAST_BINARY_URL  — full URL (or file://) for the homecast binary
#   AIRCAST_BINARY_URL   — full URL (or file://) for the aircast binary
#   SKIP_SYSTEMD_ENABLE  — set to 1 to skip systemctl enable/start

set -euo pipefail

HOMECAST_REPO="pizzasaurusrex/homecast"
AIRCONNECT_REPO="philippe44/AirConnect"

HOMECAST_BIN="/usr/local/bin/homecast"
AIRCAST_BIN="/usr/local/lib/homecast/aircast"
CONFIG_FILE="/etc/homecast/config.yaml"
LOG_DIR="/var/log/homecast"
SERVICE_FILE="/etc/systemd/system/homecast.service"
HOMECAST_USER="homecast"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
info()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
error() { printf '\033[1;31mError:\033[0m %s\n' "$*" >&2; exit 1; }

need_root() {
    [[ "$(id -u)" -eq 0 ]] || error "This script must be run as root (try: sudo bash install.sh)"
}

detect_arch() {
    local machine
    machine="$(uname -m)"
    case "$machine" in
        armv7l)  echo "armv7"  ;;
        aarch64) echo "arm64"  ;;
        x86_64)  echo "amd64"  ;;
        *)       error "Unsupported architecture: $machine" ;;
    esac
}

# AirConnect uses different arch names than we do.
aircast_arch() {
    case "$1" in
        arm64) echo "aarch64" ;;
        armv7) echo "arm"     ;;
        amd64) echo "x86_64"  ;;
        *)     error "No aircast arch mapping for: $1" ;;
    esac
}

latest_release_asset() {
    local repo="$1" pattern="$2"
    local url
    url=$(curl -fsSL "https://api.github.com/repos/${repo}/releases/latest" \
        | grep -o '"browser_download_url": "[^"]*'"${pattern}"'[^"]*"' \
        | head -1 \
        | grep -o 'https://[^"]*')
    [[ -n "$url" ]] || error "Could not find release asset matching '${pattern}' in ${repo}"
    echo "$url"
}

download() {
    local url="$1" dest="$2"
    if [[ "$url" == file://* ]]; then
        cp "${url#file://}" "$dest"
    else
        curl -fsSL -o "$dest" "$url"
    fi
}

# AirConnect ships a single ZIP; extract the binary for our arch.
download_aircast() {
    local arch="$1" dest="$2"
    local ac_arch zip_url zip_tmp entry
    ac_arch="$(aircast_arch "$arch")"
    entry="aircast-linux-${ac_arch}"

    info "Fetching latest AirConnect release..."
    zip_url="$(latest_release_asset "$AIRCONNECT_REPO" "AirConnect-.*\.zip")"

    zip_tmp="$(mktemp --suffix=.zip)"
    info "Downloading AirConnect..."
    curl -fsSL -o "$zip_tmp" "$zip_url"
    unzip -p "$zip_tmp" "$entry" > "$dest"
    rm -f "$zip_tmp"
    chmod +x "$dest"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
need_root

# Stop the service before replacing binaries — Linux refuses to overwrite
# executables that are currently mapped into a running process ("Text file busy").
if systemctl is-active --quiet homecast 2>/dev/null; then
    info "Stopping homecast service..."
    systemctl stop homecast
fi

ARCH="$(detect_arch)"
info "Detected architecture: $ARCH"

# Resolve download URLs
if [[ -z "${HOMECAST_BINARY_URL:-}" ]]; then
    info "Fetching latest homecast release..."
    HOMECAST_BINARY_URL="$(latest_release_asset "$HOMECAST_REPO" "homecast-.*-linux-${ARCH}")"
fi


# Create directories
info "Creating directories..."
mkdir -p "$(dirname "$AIRCAST_BIN")" "$LOG_DIR" "$(dirname "$CONFIG_FILE")"

# Create system user (no login shell, no home)
if ! id "$HOMECAST_USER" &>/dev/null; then
    info "Creating user $HOMECAST_USER..."
    useradd --system --no-create-home --shell /usr/sbin/nologin "$HOMECAST_USER"
fi

# Download homecast binary
info "Downloading homecast..."
TMP_HC="$(mktemp)"
download "$HOMECAST_BINARY_URL" "$TMP_HC"
chmod +x "$TMP_HC"
mv "$TMP_HC" "$HOMECAST_BIN"

# Download aircast binary
if [[ -n "${AIRCAST_BINARY_URL:-}" ]]; then
    TMP_AC="$(mktemp)"
    download "$AIRCAST_BINARY_URL" "$TMP_AC"
    chmod +x "$TMP_AC"
    mv "$TMP_AC" "$AIRCAST_BIN"
else
    download_aircast "$ARCH" "$AIRCAST_BIN"
fi

# Write default config (only if not already present)
if [[ ! -f "$CONFIG_FILE" ]]; then
    info "Writing default config to $CONFIG_FILE..."
    cat > "$CONFIG_FILE" <<EOF
server:
  listen: "0.0.0.0:8080"
airconnect:
  binary_path: ${AIRCAST_BIN}
  log_path: ${LOG_DIR}/aircast.log
  auto_restart: true
devices: []
EOF
fi

# Set ownership
chown -R "$HOMECAST_USER:$HOMECAST_USER" "$LOG_DIR" "$(dirname "$CONFIG_FILE")"

# Install systemd unit
info "Installing systemd service..."
cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=homecast — AirPlay bridge for Google Home / Nest speakers
Documentation=https://github.com/pizzasaurusrex/homecast
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${HOMECAST_USER}
ExecStart=${HOMECAST_BIN} --serve --config ${CONFIG_FILE}
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=homecast

[Install]
WantedBy=multi-user.target
EOF

# Enable and start (skip in test environments)
if [[ "${SKIP_SYSTEMD_ENABLE:-0}" != "1" ]]; then
    info "Enabling and starting homecast service..."
    systemctl daemon-reload
    systemctl enable homecast
    systemctl start homecast
    systemctl status homecast --no-pager
else
    # Still record calls so tests can assert against them
    systemctl enable homecast
    systemctl start homecast
fi

info "Done! Open http://$(hostname -I | awk '{print $1}'):8080 in your browser."
