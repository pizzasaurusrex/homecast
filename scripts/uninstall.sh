#!/usr/bin/env bash
# homecast uninstaller — reverses install.sh
#
# Usage: sudo bash scripts/uninstall.sh
# Pass --purge to also remove /etc/homecast/config.yaml and /var/log/homecast/

set -euo pipefail

HOMECAST_BIN="/usr/local/bin/homecast"
AIRCAST_BIN="/usr/local/lib/homecast/aircast"
CONFIG_DIR="/etc/homecast"
LOG_DIR="/var/log/homecast"
SERVICE_FILE="/etc/systemd/system/homecast.service"
HOMECAST_USER="homecast"

info()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
error() { printf '\033[1;31mError:\033[0m %s\n' "$*" >&2; exit 1; }

[[ "$(id -u)" -eq 0 ]] || error "This script must be run as root (try: sudo bash uninstall.sh)"

PURGE=0
for arg in "$@"; do
    [[ "$arg" == "--purge" ]] && PURGE=1
done

# Stop and disable service
if systemctl is-active homecast &>/dev/null; then
    info "Stopping homecast service..."
    systemctl stop homecast
fi
if systemctl is-enabled homecast &>/dev/null; then
    info "Disabling homecast service..."
    systemctl disable homecast
fi

# Remove unit file and reload
if [[ -f "$SERVICE_FILE" ]]; then
    info "Removing systemd unit..."
    rm -f "$SERVICE_FILE"
    systemctl daemon-reload
fi

# Remove binaries
info "Removing binaries..."
rm -f "$HOMECAST_BIN" "$AIRCAST_BIN"
rmdir --ignore-fail-on-non-empty "$(dirname "$AIRCAST_BIN")"

# Remove system user
if id "$HOMECAST_USER" &>/dev/null; then
    info "Removing user $HOMECAST_USER..."
    userdel "$HOMECAST_USER"
fi

# Optionally purge config and logs
if [[ "$PURGE" -eq 1 ]]; then
    info "Purging config and logs..."
    rm -rf "$CONFIG_DIR" "$LOG_DIR"
else
    printf '\n\033[1;33mNote:\033[0m Config (%s) and logs (%s) were preserved.\n' "$CONFIG_DIR" "$LOG_DIR"
    printf 'Run with --purge to remove them.\n\n'
fi

info "homecast uninstalled."
