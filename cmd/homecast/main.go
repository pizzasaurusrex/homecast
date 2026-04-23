package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pizzasaurusrex/homecast/internal/bridge"
	"github.com/pizzasaurusrex/homecast/internal/config"
	"github.com/pizzasaurusrex/homecast/internal/devices"
	"github.com/pizzasaurusrex/homecast/internal/discovery"
	"github.com/pizzasaurusrex/homecast/internal/version"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr, discovery.New()); err != nil {
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, disc discovery.Discoverer) error {
	fs := flag.NewFlagSet("homecast", flag.ContinueOnError)
	fs.SetOutput(stderr)
	showVersion := fs.Bool("version", false, "print version and exit")
	dryRun := fs.Bool("dry-run", false, "discover devices and print the AirConnect config that would be generated")
	serve := fs.Bool("serve", false, "run the HTTP daemon: API + embedded web UI + AirConnect supervisor")
	configPath := fs.String("config", "", "path to config file (default: built-in defaults for --dry-run; required for --serve)")
	discoverTimeout := fs.Duration("discover-timeout", 3*time.Second, "mDNS discovery timeout for --dry-run")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		fmt.Fprintln(stdout, version.String())
		return nil
	}
	if *dryRun {
		return doDryRun(stdout, stderr, disc, *configPath, *discoverTimeout)
	}
	if *serve {
		if *configPath == "" {
			fmt.Fprintln(stderr, "homecast: --serve requires --config <path> so device toggles can persist")
			return errors.New("--serve requires --config")
		}
		return doServe(ctx, stdout, stderr, *configPath, disc)
	}
	fmt.Fprintln(stderr, "homecast: no command given (try --version, --dry-run, or --serve)")
	return errors.New("no command")
}

func doDryRun(stdout, stderr io.Writer, disc discovery.Discoverer, configPath string, timeout time.Duration) error {
	cfg, err := loadOrDefault(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	fmt.Fprintln(stdout, "Discovering Google Cast devices...")
	found, err := disc.Browse(context.Background(), timeout)
	if err != nil {
		fmt.Fprintf(stderr, "discovery error: %v\n", err)
	}
	if len(found) == 0 {
		fmt.Fprintln(stdout, "  (none found)")
	} else {
		for _, d := range found {
			fmt.Fprintf(stdout, "  - %s  id=%s  addrs=%v\n", d.Name, d.ID, d.Addrs)
		}
	}

	merged := mergeForXML(cfg, found)
	fmt.Fprintln(stdout, "\nGenerated AirConnect config:")
	xmlBytes, err := bridge.GenerateAirConnectXML(merged)
	if err != nil {
		return fmt.Errorf("generate aircast config: %w", err)
	}
	if _, err := stdout.Write(xmlBytes); err != nil {
		return err
	}
	return nil
}

func loadOrDefault(path string) (*config.Config, error) {
	if path == "" {
		return config.Default(), nil
	}
	return config.Load(path)
}

// mergeForXML returns a *Config whose Devices list is the canonical merge of
// saved + discovered produced by internal/devices.Merge, projected back to
// config.Device rows for aircast XML generation.
func mergeForXML(cfg *config.Config, discovered []discovery.Device) *config.Config {
	merged := devices.Merge(cfg.Devices, discovered)
	savedByID := make(map[string]struct{}, len(cfg.Devices))
	for _, d := range cfg.Devices {
		savedByID[d.ID] = struct{}{}
	}
	out := *cfg
	out.Devices = make([]config.Device, 0, len(merged))
	for _, m := range merged {
		enabled := m.Enabled
		if m.Discovered {
			if _, wasSaved := savedByID[m.ID]; !wasSaved {
				enabled = true
			}
		}
		out.Devices = append(out.Devices, config.Device{
			ID:      m.ID,
			Name:    m.Name,
			Enabled: enabled,
		})
	}
	return &out
}
