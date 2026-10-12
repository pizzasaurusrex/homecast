# homecast

Expose Google Home / Nest speakers as AirPlay targets from a Raspberry Pi. Thin
wrapper around [philippe44/AirConnect](https://github.com/philippe44/AirConnect)
that adds device discovery, a web dashboard, and a one-line installer.

**Status:** pre-alpha. See [PLAN.md](./PLAN.md) for the full roadmap and
architecture.

## Install

Flash a Raspberry Pi with [Raspberry Pi OS](https://www.raspberrypi.com/software/),
SSH in, and run:

```sh
curl -fsSL https://raw.githubusercontent.com/pizzasaurusrex/homecast/main/scripts/install.sh | sudo bash
```

The script will:
1. Detect your Pi's architecture (armv7 / arm64)
2. Download the latest `homecast` and `aircast` binaries
3. Create a `homecast` system user and write a default config to `/etc/homecast/config.yaml`
4. Install and start a systemd service

When it finishes, open `http://homecast.local:8080` (or the IP it prints) in any
browser on the same network. Toggle your Google Home / Nest speakers on, then pick
them from the AirPlay menu on your iPhone.

### Uninstall

```sh
curl -fsSL https://raw.githubusercontent.com/pizzasaurusrex/homecast/main/scripts/uninstall.sh | sudo bash
# Add --purge to also remove config and logs
```

### Verify your install

After the installer finishes, run through this checklist:

- [ ] `systemctl status homecast` shows `active (running)`
- [ ] `http://homecast.local:8080` loads the dashboard in a browser
- [ ] The dashboard lists your Google Home / Nest speakers under Devices
- [ ] Toggling a device on and clicking Restart Bridge starts AirConnect
- [ ] An iPhone sees the speaker in the AirPlay picker (Control Centre → AirPlay)
- [ ] Audio plays through the speaker from Apple Music

If a speaker doesn't appear, check that the Pi and speaker are on the same subnet
and that mDNS traffic isn't blocked by your router.

## Troubleshooting

### The Pi restarts or drops off the network under load

If the Pi reboots during `apt` upgrades or installs, disappears from the network
for a minute at a time, or comes back with empty config files or a corrupted SD
card, suspect the power supply or cable before the Pi itself. A weak supply holds
5V while the Pi idles and sags as soon as it does real work.

Check the firmware's power flags:

```sh
vcgencmd get_throttled
```

| Value | Meaning |
|---|---|
| `throttled=0x0` | Power has been fine since boot |
| `0x50000` | Under-voltage and throttling have happened since boot |
| `0x50005` | Under-voltage and throttling are happening right now |
| `0x80000` / `0x80008` | Soft temperature limit only (normal on a Pi 3B+ under load, not a power problem) |

The kernel log records the same events:

```sh
sudo dmesg | grep -i voltage
# hwmon hwmon1: Undervoltage detected!
```

A solid red power LED does not rule this out: the Pi can reset under load while
the LED stays lit. A hard reset also wipes the flags, so a clean `0x0` straight
after an unexplained reboot proves nothing. Re-check after putting the Pi under
load.

To fix it, use a supply rated for your model (5.1V 2.5A micro-USB for a Pi 3,
5.1V 3A USB-C for a Pi 4) with a short, thick cable or one that's attached to the
supply. Phone chargers and long thin cables are the usual culprits.

## Develop

### Run locally (Mac) — full e2e

**1. Build the binary**

```sh
go build ./cmd/homecast
```

**2. Download AirConnect for macOS**

```sh
curl -L -o /tmp/airconnect.zip \
  https://github.com/philippe44/AirConnect/releases/download/1.9.3/AirConnect-1.9.3.zip

# Apple Silicon (M1/M2/M3/M4) — use the static build to avoid macOS libcrypto issues:
unzip -j /tmp/airconnect.zip aircast-macos-arm64-static -d .
mv aircast-macos-arm64-static aircast

# Intel Mac (x86_64):
# unzip -j /tmp/airconnect.zip aircast-macos-x86_64-static -d .
# mv aircast-macos-x86_64-static aircast

chmod +x aircast
xattr -d com.apple.quarantine aircast 2>/dev/null || true  # remove Gatekeeper quarantine if needed
```

**3. Create a local config**

Copy the sample config and point it at your local binary:

```sh
cp airconnect.yaml airconnect.local.yaml
```

Edit `airconnect.local.yaml` and update `binary_path`:

```yaml
airconnect:
  binary_path: ./aircast   # ← change this line
```

`airconnect.local.yaml` is gitignored — safe to customize without committing.

**4. Discover your Cast devices** (optional, to populate the `devices` list)

```sh
./homecast --dry-run --config airconnect.local.yaml
```

**5. Start the server**

```sh
./homecast --serve --config airconnect.local.yaml
# → homecast: listening on http://[::]:8080
```

Open [http://localhost:8080](http://localhost:8080) in your browser.

### Tests

```sh
go test ./...
```

## License

MIT. See [LICENSE](./LICENSE).

AirConnect is a separate project by [philippe44](https://github.com/philippe44/AirConnect),
also MIT-licensed. homecast downloads it at install time rather than
redistributing it, so users always get the upstream's latest build.
