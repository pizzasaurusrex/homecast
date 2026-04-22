# homecast

Expose Google Home / Nest speakers as AirPlay targets from a Raspberry Pi. Thin
wrapper around [philippe44/AirConnect](https://github.com/philippe44/AirConnect)
that adds device discovery, a web dashboard, and a one-line installer.

**Status:** pre-alpha. See [PLAN.md](./PLAN.md) for the full roadmap and
architecture.

## Install

Coming in M4. For now, see the plan.

## Develop

```sh
go test ./...
go build ./cmd/homecast
./homecast --version
```

## License

MIT. See [LICENSE](./LICENSE).

AirConnect is a separate project by [philippe44](https://github.com/philippe44/AirConnect),
also MIT-licensed. homecast downloads it at install time rather than
redistributing it, so users always get the upstream's latest build.
