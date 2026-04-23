# homecast

Expose Google Home / Nest speakers as AirPlay targets from a Raspberry Pi. Thin
wrapper around [philippe44/AirConnect](https://github.com/philippe44/AirConnect)
that adds device discovery, a web dashboard, and a one-line installer.

**Status:** pre-alpha. See [PLAN.md](./PLAN.md) for the full roadmap and
architecture.

## Install

Coming in M4. For now, see the plan.

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
