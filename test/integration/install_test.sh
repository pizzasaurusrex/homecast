#!/usr/bin/env bash
# Docker-based installer integration test.
#
# Builds a minimal Ubuntu image with a fake systemctl shim, runs install.sh
# against mocked download URLs (served by a local Python HTTP server), then
# asserts the expected file layout and config content.
#
# Usage: bash test/integration/install_test.sh
# Requirements: docker (running), bash 4+

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
IMAGE="homecast-install-test:local"
PASS=0
FAIL=0

log()  { printf '\033[1;34m[TEST]\033[0m %s\n' "$*"; }
ok()   { printf '\033[1;32m[PASS]\033[0m %s\n' "$*"; PASS=$((PASS + 1)); }
fail() { printf '\033[1;31m[FAIL]\033[0m %s\n' "$*"; FAIL=$((FAIL + 1)); }

# ---------------------------------------------------------------------------
# 1. Build the test Docker image
# ---------------------------------------------------------------------------
log "Building Docker image..."
docker build -q -t "$IMAGE" -f - "$REPO_ROOT" <<'DOCKERFILE'
FROM ubuntu:22.04
ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update -qq && apt-get install -y -qq \
    curl ca-certificates python3 adduser \
    && rm -rf /var/lib/apt/lists/*

# Fake systemctl: records calls, always succeeds.
RUN printf '#!/bin/sh\necho "systemctl $*" >> /tmp/systemctl.log\n' \
    > /usr/bin/systemctl && chmod +x /usr/bin/systemctl

COPY scripts/install.sh /install.sh
COPY systemd/homecast.service /homecast.service.src
RUN chmod +x /install.sh
DOCKERFILE

# ---------------------------------------------------------------------------
# 2. Run the installer inside Docker with mocked downloads
#    - HOMECAST_DOWNLOAD_URL  points at a local HTTP server that serves a
#      fake homecast binary (a shell stub that prints its version).
#    - AIRCAST_DOWNLOAD_URL   points at the same server for a fake aircast.
#    Both servers are just `python3 -m http.server` from a temp dir.
# ---------------------------------------------------------------------------
log "Creating mock binaries..."
MOCK_DIR="$(mktemp -d)"
trap 'rm -rf "$MOCK_DIR"' EXIT

# Fake homecast binary: responds to --version
printf '#!/bin/sh\necho "homecast v0.0.0-test"\n' > "$MOCK_DIR/homecast"
chmod +x "$MOCK_DIR/homecast"

# Fake aircast binary: just a stub
printf '#!/bin/sh\necho "aircast stub"\n' > "$MOCK_DIR/aircast"
chmod +x "$MOCK_DIR/aircast"

log "Running installer in Docker..."
CONTAINER_OUTPUT=$(docker run --rm \
    -v "$MOCK_DIR:/mock" \
    "$IMAGE" \
    bash -c '
        set -e
        # Provide mocked binaries instead of real downloads
        export HOMECAST_BINARY_URL="file:///mock/homecast"
        export AIRCAST_BINARY_URL="file:///mock/aircast"
        export SKIP_SYSTEMD_ENABLE=1
        /install.sh 2>&1
    ')

echo "$CONTAINER_OUTPUT"

# ---------------------------------------------------------------------------
# 3. Run assertions in a fresh container that inspects the filesystem
# ---------------------------------------------------------------------------
log "Running assertions..."
ASSERT_OUTPUT=$(docker run --rm \
    -v "$MOCK_DIR:/mock" \
    "$IMAGE" \
    bash -c '
        set -e
        export HOMECAST_BINARY_URL="file:///mock/homecast"
        export AIRCAST_BINARY_URL="file:///mock/aircast"
        export SKIP_SYSTEMD_ENABLE=1
        /install.sh >/dev/null 2>&1

        echo "CHECK:homecast-bin:$(test -x /usr/local/bin/homecast && echo ok || echo missing)"
        echo "CHECK:aircast-bin:$(test -x /usr/local/lib/homecast/aircast && echo ok || echo missing)"
        echo "CHECK:service-file:$(test -f /etc/systemd/system/homecast.service && echo ok || echo missing)"
        echo "CHECK:config-file:$(test -f /etc/homecast/config.yaml && echo ok || echo missing)"
        echo "CHECK:log-dir:$(test -d /var/log/homecast && echo ok || echo missing)"
        echo "CHECK:homecast-user:$(id -u homecast >/dev/null 2>&1 && echo ok || echo missing)"
        echo "CHECK:config-binary-path:$(grep -q "binary_path: /usr/local/lib/homecast/aircast" /etc/homecast/config.yaml && echo ok || echo missing)"
        echo "CHECK:config-log-path:$(grep -q "log_path: /var/log/homecast/aircast.log" /etc/homecast/config.yaml && echo ok || echo missing)"
        echo "CHECK:systemctl-enable:$(grep -q "enable homecast" /tmp/systemctl.log 2>/dev/null && echo ok || echo missing)"
        echo "CHECK:systemctl-start:$(grep -q "start homecast" /tmp/systemctl.log 2>/dev/null && echo ok || echo missing)"
    ')

# Parse and report assertions
while IFS= read -r line; do
    if [[ "$line" == CHECK:* ]]; then
        name="${line#CHECK:}"
        key="${name%%:*}"
        value="${name##*:}"
        if [[ "$value" == "ok" ]]; then
            ok "$key"
        else
            fail "$key"
        fi
    fi
done <<< "$ASSERT_OUTPUT"

# ---------------------------------------------------------------------------
# 4. Summary
# ---------------------------------------------------------------------------
echo ""
printf 'Results: \033[1;32m%d passed\033[0m, \033[1;31m%d failed\033[0m\n' "$PASS" "$FAIL"
[[ "$FAIL" -eq 0 ]]
