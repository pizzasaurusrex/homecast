# homecast

Expose Google Home / Nest speakers as AirPlay targets from a Raspberry Pi. Thin
wrapper around [philippe44/AirConnect](https://github.com/philippe44/AirConnect)
that adds device discovery, a web dashboard, and a one-line installer.

**Status:** pre-alpha. See [PLAN.md](./PLAN.md) for the full roadmap and
architecture.

## Install

Coming in M4. For now, see the plan.

## Develop

### Run locally (Mac)

```sh
# Build
go build ./cmd/homecast

# Start the server (UI + API) using the sample config
./homecast --serve --config airconnect.yaml
# → homecast: listening on http://0.0.0.0:8080
```

Open [http://localhost:8080](http://localhost:8080) in your browser.

> **Note:** The AirConnect binary path in `airconnect.yaml` (`/usr/local/lib/homecast/aircast`)
> does not exist on Mac. The supervisor will log a warning and skip starting AirConnect,
> but the web UI and all API endpoints remain fully functional for development.

### Dry-run (device discovery only)

```sh
# Discover Cast devices on the network and print the AirConnect config that would be generated
./homecast --dry-run
# or with a saved config:
./homecast --dry-run --config airconnect.yaml
```

### Tests

```sh
go test ./...
```

## License

MIT. See [LICENSE](./LICENSE).

AirConnect is a separate project by [philippe44](https://github.com/philippe44/AirConnect),
also MIT-licensed. homecast downloads it at install time rather than
redistributing it, so users always get the upstream's latest build.
