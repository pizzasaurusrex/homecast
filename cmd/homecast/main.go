package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/pizzasaurusrex/homecast/internal/bridge"
	"github.com/pizzasaurusrex/homecast/internal/config"
	"github.com/pizzasaurusrex/homecast/internal/discovery"
	"github.com/pizzasaurusrex/homecast/internal/version"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr, discovery.New()); err != nil {
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer, disc discovery.Discoverer) error {
	fs := flag.NewFlagSet("homecast", flag.ContinueOnError)
	fs.SetOutput(stderr)
	showVersion := fs.Bool("version", false, "print version and exit")
	dryRun := fs.Bool("dry-run", false, "discover devices and print the AirConnect config that would be generated")
	configPath := fs.String("config", "", "path to config file (default: built-in defaults)")
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
	fmt.Fprintln(stderr, "homecast: no command given (try --version or --dry-run)")
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

	merged := mergeDevices(cfg, found)
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

// mergeDevices returns a new *Config whose Devices is the union of the
// configured devices and the freshly discovered ones; discovered devices not
// present in the saved config are added as disabled (user opts in via UI).
func mergeDevices(cfg *config.Config, discovered []discovery.Device) *config.Config {
	byID := make(map[string]config.Device, len(cfg.Devices)+len(discovered))
	for _, d := range cfg.Devices {
		byID[d.ID] = d
	}
	for _, d := range discovered {
		if _, ok := byID[d.ID]; !ok {
			byID[d.ID] = config.Device{ID: d.ID, Name: d.Name, Enabled: false}
		}
	}
	out := *cfg
	out.Devices = make([]config.Device, 0, len(byID))
	for _, d := range cfg.Devices {
		out.Devices = append(out.Devices, byID[d.ID])
		delete(byID, d.ID)
	}
	for _, d := range discovered {
		if dev, ok := byID[d.ID]; ok {
			out.Devices = append(out.Devices, dev)
			delete(byID, d.ID)
		}
	}
	return &out
}
