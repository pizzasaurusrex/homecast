# homecast — Plan

A small Raspberry Pi service that exposes your Google Home / Nest speakers as
AirPlay targets, so you can play Apple Music (or anything else on iOS) to them
with native AirPlay controls. Thin wrapper around
[philippe44/AirConnect](https://github.com/philippe44/AirConnect) that adds
lifecycle management, a web dashboard, and a one-line installer.

> **Fresh session? Start here.** The [Progress](#progress) section below is the
> living status log — it records which milestones are complete, what's in
> flight, and any open questions. Every commit that advances project state
> updates it, so this file alone is enough to pick up without prior context.

---

## Progress

Status log, newest first. Every PR that touches code updates this section in
the same commit. Use merged PR links as the audit trail.

### Milestones

| Milestone | Status         | Notes                                             |
|-----------|----------------|---------------------------------------------------|
| M1        | ✅ done (2026-04-22) | Skeleton, CI, cross-compile release. [v0.0.1](https://github.com/pizzasaurusrex/homecast/releases/tag/v0.0.1) |
| M2        | ✅ done (2026-04-22) | config, discovery, bridge packages, `--dry-run` end-to-end works against real Google Homes. Coverage ≥80% on every package. |
| M3        | ✅ done (2026-04-23) | HTTP API + embedded web UI. All 4 slices merged ([PR #3](https://github.com/pizzasaurusrex/homecast/pull/3)–[#6](https://github.com/pizzasaurusrex/homecast/pull/6)). Pi E2E smoke-test is manual (requires hardware). |
| M4        | ✅ done (2026-04-23) | Installer, systemd, Docker-based integration test. [v0.1.0](https://github.com/pizzasaurusrex/homecast/releases/tag/v0.1.0) |
| M5        | ⏳ pending     | "Now Playing" indicator — parse AirConnect logs to surface which speaker is active in the UI |
| M6        | ⏳ pending     | Bluetooth speaker support — expose paired Bluetooth speakers as AirPlay targets via shairport-sync + bluealsa |
| M7        | ⏳ stretch     | iOS Shortcuts pack                                |

### Recent fixes

- **CI lint unblock** — [PR #1](https://github.com/pizzasaurusrex/homecast/pull/1): golangci-lint binary was built with Go 1.24 but go.mod targets 1.25; switched to `go install` + added `.golangci.yml`.

### Next actions (in order)

1. Manual E2E on the Pi: flash Pi OS, run the one-line installer, prove an iPhone can AirPlay to a Google Home end-to-end (smoke-test checklist in README).
2. M5: "Now Playing" indicator.
3. M6: Bluetooth speaker support.
4. M7 (stretch): iOS Shortcuts pack.

### Deferred follow-ups

- **mDNS TTL cache for `/api/devices`** — every GET currently triggers a 3 s mDNS browse. A polling UI will flood the LAN and stall each call. Add a short TTL cache (10–30 s) with an explicit `?refresh=1` bypass. File once slice 4 lands on a real Pi so we have real traffic to measure.
- **Bridge auto-restart on crash** — `config.AirConnect.AutoRestart` is parsed but `bridge.Supervisor` does not yet honour it. Today a crashed AirConnect stays down until the user hits /api/bridge/restart. Add an exponential-backoff watcher in the supervisor; this was originally an M2 target that slipped.
- **`/api/config` leaks filesystem paths** — the full config snapshot including `AirConnect.BinaryPath` is returned. On a hostile LAN this aids path-based attacks. For v2 consider either omitting sensitive fields or restricting the endpoint to loopback. Low priority while the LAN-only assumption holds.
- **`Supervisor.Stop` double-SIGTERM on concurrent callers** — if two goroutines race to call `Stop` while the child is running, both enter the `StateStopping` branch, send SIGTERM, and wait on the same `done` channel. Both unblock safely when the child exits and ESRCH is suppressed, so there is no observable bug today. An early-return on `StateStopping` would make the intent explicit and prevent a spurious second signal if the process happens to reuse its PID before the second SIGTERM is sent. Low priority — only one caller (`runServer`) drives Stop in practice.

### Open questions still deferred

- Hostname: rely on `homecast.local` via avahi (Pi OS ships it) or document alternative?
- API auth: none in v1 (LAN assumption). Add token header in v2?
- Logging format: `log/slog` JSON; acceptable for operators?

## 1. Problem

Apple Music can AirPlay to Samsung TVs/soundbars (Samsung licensed AirPlay 2)
but not to Google Home / Nest speakers (Google never licensed AirPlay, pushes
its own Cast protocol instead). AirConnect solves the protocol bridging but is
a bare binary with a config file — not friendly to set up, monitor, or tweak.

homecast makes it trivial:

1. Flash an SD card, run one install command on the Pi.
2. Open `http://homecast.local` from any device on your network.
3. See discovered Google Homes, toggle which ones to expose, restart the
   bridge, view logs.
4. Pick up your iPhone, open Apple Music, AirPlay to the living-room Home.

## 2. Non-goals

- **Not** a reimplementation of AirPlay or Google Cast protocols. We wrap
  AirConnect, with a fork-and-maintain fallback if upstream goes dark.
- **Not** a cloud service. Everything runs on the user's LAN.
- **Not** a macOS / Windows / generic-Linux app for v1. Target is Raspberry Pi
  with Raspberry Pi OS (Debian-based). Other platforms are easy follow-ups.
- **Not** a replacement for the Apple Music app. Music control stays in the
  native app via AirPlay.
- **Not** an iOS app in v1. iOS Shortcuts pack is a v2 stretch goal.

## 3. Architecture

```
┌─────────────┐     AirPlay/ALAC      ┌──────────────────────────┐
│   iPhone    │ ────────────────────> │      Raspberry Pi        │
│ Apple Music │                        │  ┌──────────────────┐   │
└─────────────┘                        │  │    AirConnect    │   │
                                       │  │   (subprocess)   │   │
                                       │  └────────┬─────────┘   │
                                       │           │ manages     │
                                       │  ┌────────┴─────────┐   │
                                       │  │   homecast (Go)  │   │
                                       │  │  - supervisor    │   │
                                       │  │  - mDNS discovery│   │
                                       │  │  - HTTP API + UI │   │
                                       │  └──────────────────┘   │
                                       │           │ Google Cast │
                                       └───────────┼─────────────┘
                                                   ▼
                                          ┌─────────────────┐
                                          │  Google Home /  │
                                          │     Nest        │
                                          └─────────────────┘
```

`homecast` is **not** in the audio path. It supervises AirConnect, discovers
devices, writes AirConnect's config file, and serves the UI. The audio goes
iPhone → AirConnect → Google Home directly.

### Components

- **AirConnect binary** — downloaded at install time from philippe44's official
  releases. Managed as a subprocess by `homecast`.
- **homecast daemon** (Go, single static binary):
  - Process supervisor for AirConnect (start/stop/restart, log capture,
    crash-restart with backoff)
  - mDNS discovery of `_googlecast._tcp` devices
  - Config file generator for AirConnect (XML)
  - HTTP API + embedded web UI
- **Web UI** — minimal SPA served at `/`. Plain HTML/CSS/vanilla JS, no
  framework. Embedded via Go's `embed` so the binary is self-contained.
- **systemd unit** — keeps `homecast` running across reboots.
- **Installer script** — `install.sh` downloads latest homecast + AirConnect,
  installs systemd unit, starts service.

### Data flow (happy path)

1. User flashes Pi, runs `curl -fsSL https://.../install.sh | sh`
2. `homecast.service` starts. Daemon comes up, begins mDNS discovery.
3. First-run state: no devices enabled. User opens `http://homecast.local`,
   toggles their speakers on.
4. Daemon writes AirConnect config, starts AirConnect.
5. AirConnect registers each enabled Google Home as an AirPlay receiver via
   mDNS (`_raop._tcp`).
6. iPhone sees them in the AirPlay picker, user plays music.

### HTTP API surface

| Method | Path                          | Purpose                              |
|--------|-------------------------------|--------------------------------------|
| GET    | `/api/status`                 | Bridge state, uptime, AirConnect ver |
| GET    | `/api/devices`                | Discovered Cast devices + enabled    |
| POST   | `/api/devices/:id/enable`     | Expose device as AirPlay target      |
| POST   | `/api/devices/:id/disable`    | Stop exposing                         |
| POST   | `/api/bridge/restart`         | Restart AirConnect                    |
| GET    | `/api/logs?tail=N`            | Recent AirConnect log lines           |
| GET    | `/api/config`                 | Current config (debug)                |

No auth in v1 — single-home LAN assumption. Token auth is a v2 consideration.

### Config

`/etc/homecast/config.yaml`:

```yaml
server:
  listen: "0.0.0.0:8080"
airconnect:
  binary_path: /usr/local/lib/homecast/aircast
  log_path: /var/log/homecast/aircast.log
  auto_restart: true
devices:
  - id: "kitchen-home._googlecast._tcp.local."
    name: "Kitchen Home"
    enabled: true
```

## 4. Repository layout

```
homecast/
├── PLAN.md                   (this file)
├── README.md                 (quickstart, added in M1)
├── LICENSE                   (MIT)
├── go.mod
├── go.sum
├── cmd/
│   └── homecast/
│       └── main.go
├── internal/
│   ├── config/               (load/save/validate yaml)
│   ├── discovery/            (mDNS Google Cast discovery)
│   ├── bridge/               (AirConnect subprocess + XML config gen)
│   ├── api/                  (HTTP handlers, server)
│   ├── logs/                 (log tailing)
│   └── web/                  (embedded static UI assets)
│       ├── web.go
│       └── static/
│           ├── index.html
│           ├── style.css
│           └── app.js
├── scripts/
│   ├── install.sh
│   └── uninstall.sh
├── systemd/
│   └── homecast.service
├── test/
│   └── integration/
│       └── install_test.sh   (Docker-based installer test)
└── .github/
    └── workflows/
        ├── ci.yml
        └── release.yml
```

Every `internal/` package ships with its own `_test.go`. Target ≥80%
statement coverage, enforced in CI.

## 5. Milestones

Sizing is "weekend-ish evenings" — not calendar days.

### M1 — Skeleton + CI (first evening)

- Initialize Go module `github.com/<user>/homecast`
- `cmd/homecast/main.go` prints version; `--version` flag works
- MIT LICENSE, initial README
- GitHub Actions `ci.yml`: `go test`, `go vet`, `gofmt -l`, `golangci-lint`,
  coverage upload
- GitHub Actions `release.yml`: cross-compile for `linux/armv7`, `linux/arm64`,
  `linux/amd64` on tag push; publish binaries + SHA256 checksums
- Repo pushed public

**Done when:** CI green on main, tagged `v0.0.1` release produces downloadable
armv7 + arm64 binaries.

### M2 — Core daemon (second evening)

- `internal/config`: YAML load/save, schema validation, defaults
- `internal/discovery`: mDNS browse for `_googlecast._tcp`, returns structured
  device list, tested with mock mDNS responses
- `internal/bridge`: subprocess manager — start/stop/restart, stdout/stderr
  capture, exponential backoff restart on crash, AirConnect XML config
  generation from our YAML
- Unit tests for each package, ≥80% coverage

**Done when:** `homecast --dry-run` prints discovered devices and the
AirConnect config it would write, without actually launching AirConnect.

### M3 — HTTP API + Web UI (third evening)

- `internal/api`: Gorilla-style router (or `net/http` + `http.ServeMux` — stdlib
  preferred), JSON handlers per API spec above
- Web UI: single page, three sections (status / devices / logs), vanilla JS
  fetching from `/api/*`
- Go `embed` for static assets
- `httptest`-based handler tests
- Manual E2E: run on the Pi, AirPlay from iPhone to a Google Home end-to-end

**Done when:** an iPhone can AirPlay Apple Music to a Google Home via the
bridge, controlled via the web UI from another device.

### M4 — Installer + systemd + release polish (fourth evening)

- `scripts/install.sh`: detects arch, downloads latest homecast release +
  latest AirConnect `aircast` binary, installs systemd unit, enables + starts
  service, prints URL to visit
- `scripts/uninstall.sh`: reverses M4
- `systemd/homecast.service`: `After=network-online.target`, `Restart=always`
- `test/integration/install_test.sh`: runs installer inside a Docker container
  (ubuntu:22.04 + systemd), asserts the service comes up healthy. AirConnect
  binary is mocked here — we test the installer, not AirConnect itself.
- README quickstart with one-line install command

**Done when:** fresh Pi OS install → one curl command → working bridge,
reproducible from scratch, integration test gates CI.

### M5 — "Now Playing" indicator

AirConnect logs connection and playback events to stdout. We already capture
this in the ring-buffer log tailing. This milestone surfaces the active speaker
in the UI.

- Parse the AirConnect log ring buffer for connection events (e.g. lines
  containing "connected" or "playing" alongside a device name / UDN).
- Expose an `activeSpeaker` field in `GET /api/status` (null when nothing is
  playing; device name + ID when a connection is active).
- Update the web UI to show a "now playing" badge next to the active device in
  the devices list and in the status panel.
- Unit-test the log parser against real AirConnect log lines captured from a Pi
  session (add representative lines as fixtures).

**Note:** the exact log format must be verified against a live AirConnect run
before implementing the parser — add fixture lines from the Pi log first.

**Done when:** the UI badge updates within one poll cycle of an iPhone
connecting to a speaker.

### M6 — Bluetooth speaker support

Expose Bluetooth A2DP speakers paired with the Pi as additional AirPlay targets,
alongside the existing Google Home / Cast devices.

#### Approach

AirConnect only bridges AirPlay ↔ Google Cast. A separate bridge is needed for
Bluetooth:

1. Pair the Pi with each Bluetooth speaker (one-time manual step via
   `bluetoothctl`).
2. Run `bluealsa` (BlueZ ALSA plugin) to create a virtual ALSA sink for each
   paired speaker.
3. Run one `shairport-sync` instance per speaker, configured to output to that
   speaker's bluealsa ALSA sink. Each instance registers itself on the LAN as a
   separate AirPlay receiver.
4. homecast manages the `shairport-sync` processes the same way it manages
   AirConnect — start/stop/restart, log capture, supervised restarts.

#### homecast changes required

- New `bluetooth` section in `config.yaml`: list of paired device MAC addresses
  with friendly names.
- Extend `internal/bridge` (or add `internal/bluetooth`) to supervise one
  `shairport-sync` child per configured device.
- Extend the devices list and enable/disable API to cover Bluetooth devices
  alongside Cast devices.
- `install.sh`: detect if `bluealsa` and `shairport-sync` are available; offer
  to install them (or document as a prerequisite).
- UI: label devices by type (Cast vs. Bluetooth) so the user can tell them apart.

#### Security assessment

Bluetooth does **not** open a new inbound network port or change firewall rules —
it is a radio protocol separate from the LAN. The incremental attack surface is:

| Surface | Risk | Mitigation |
|---------|------|------------|
| BlueZ system service | Runs with elevated privileges; vulnerabilities in BlueZ could be exploited locally | BlueZ is the standard Linux Bluetooth stack, well-audited, kept up to date by Pi OS |
| Paired device persistence | BlueZ stores paired devices in `/var/lib/bluetooth`; a compromised Pi could reconnect to previously paired speakers | Pairing requires physical proximity and explicit `bluetoothctl pair` — no auto-pairing |
| `shairport-sync` AirPlay listener | New AirPlay receivers on the LAN (same attack surface as AirConnect) | No new exposure type; same LAN-only assumption as the rest of homecast |
| Auto-connect on boot | Pi may reconnect to a paired speaker without user action | Expected behavior; document it. User can unpair via `bluetoothctl remove` |

**Net verdict:** safe for a home LAN. No new network holes. The BlueZ / shairport-sync
processes are well-maintained open-source projects with good security track records.
Primary operational concern is Bluetooth range (~10 m, degrades through walls) and
the Pi's weak onboard radio — a USB Bluetooth dongle is recommended for reliability.

**Done when:** an iPhone can AirPlay to a paired Bluetooth speaker via homecast,
with the speaker appearing in the devices list and toggle working.

### M7 — iOS Shortcuts pack (stretch, post-v1)

- Shortcuts for: enable/disable each speaker, restart bridge, show status
- Shortcuts call the homecast API over local network
- Siri phrases like "Hey Siri, enable kitchen speaker"
- Published as downloadable `.shortcut` files in the repo

## 6. Testing strategy

- **Unit** — `go test ./...` on every package in `internal/`. Table-driven where
  it fits. Coverage ≥80%, reported in CI, visible in PR comments.
- **Integration** — `httptest` for the API layer (real handlers, real routes,
  fake dependencies). Docker-based installer test for the release path.
- **Manual E2E** — run on the Pi with a real Google Home and iPhone. Documented
  smoke-test checklist in the README; no automation (would require real
  hardware).
- **Linting** — `gofmt`, `go vet`, `golangci-lint` (default config + revive,
  gocyclo, errcheck, ineffassign).
- **Race detector** — `go test -race` in CI.

## 7. CI/CD

### `.github/workflows/ci.yml`

Triggers: push, pull_request.

Jobs:
- `lint` — gofmt check, `go vet`, `golangci-lint run`
- `test` — matrix over Go 1.22 and 1.23; `go test -race -cover ./...`;
  upload coverage to Codecov
- `build` — `go build` for `linux/armv7` and `linux/arm64` to catch
  cross-compile regressions
- `integration` — run `test/integration/install_test.sh` in Docker

### `.github/workflows/release.yml`

Triggers: tag push matching `v*`.

- Cross-compile for `linux/armv7`, `linux/arm64`, `linux/amd64`
- Generate SHA256 checksums
- Create GitHub release with binaries + checksums attached
- `install.sh` reads `latest` release from the GitHub API

### Versioning

Semver. `v0.x` while we're pre-stable; `v1.0.0` when M1-M4 are production-quiet
for 2+ weeks on the maintainer's own Pi.

## 8. Risks & mitigations

| Risk                                       | Likelihood | Mitigation                                                      |
|--------------------------------------------|------------|-----------------------------------------------------------------|
| AirConnect upstream abandoned              | Low-Med    | MIT licensed — fork-and-maintain path is available              |
| Apple breaks AirPlay compatibility         | Low        | Same as above; philippe44 has historically kept up              |
| Pi 3 underpowered for transcoding          | Very Low   | AirConnect runs fine on Pi 3 per upstream docs                  |
| mDNS issues on VLAN/mesh-Wi-Fi setups      | Medium     | Document in README; consider mDNS reflector guidance for v2     |
| First-run discovery empty (nothing to toggle) | Low     | UI shows a helpful "no devices found yet" state + troubleshooting link |
| Binary release pipeline regressions        | Low        | Integration test in CI catches installer breakage pre-tag       |

## 9. Open questions

1. **Hostname** — default to `homecast.local` via avahi/mDNS? Most Pi OS images
   ship with avahi; we can assume it but document.
2. **Config format** — YAML chosen; reconsider if operators push back.
3. **Auth on the API** — none in v1. v2 could add a shared-token header for
   users with hostile roommates. Document the assumption clearly.
4. **Logging** — structured JSON logs from the daemon (useful) vs. plain text
   (simpler). Going with `log/slog` JSON output, pretty by default in TTY.
5. **Metrics** — Prometheus `/metrics` endpoint for v2? Low priority.

## 10. Out of scope for v1 (explicitly)

- AirPlay 2 multi-room sync (AirConnect is AirPlay 1 for Cast devices)
- Spotify Connect / other sources (AirConnect does this too but keep scope tight)
- Windows / macOS host support
- Grouped Google Home targets via our UI (configurable in AirConnect directly,
  UI wrapper is v2)
- Automatic AirConnect version updates (users re-run `install.sh` for now)
