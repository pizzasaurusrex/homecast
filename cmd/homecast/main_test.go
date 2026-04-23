package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/pizzasaurusrex/homecast/internal/config"
	"github.com/pizzasaurusrex/homecast/internal/discovery"
)

func TestRunVersionFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), []string{"--version"}, &stdout, &stderr, discovery.Fake{}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "homecast") {
		t.Errorf("expected version output to contain 'homecast', got %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("expected empty stderr, got %q", stderr.String())
	}
}

func TestRunNoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), nil, &stdout, &stderr, discovery.Fake{}); err == nil {
		t.Fatal("expected error when no args given, got nil")
	}
	if !strings.Contains(stderr.String(), "--dry-run") || !strings.Contains(stderr.String(), "--serve") {
		t.Errorf("expected stderr to hint at --dry-run and --serve, got %q", stderr.String())
	}
}

func TestRunUnknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), []string{"--nope"}, &stdout, &stderr, discovery.Fake{}); err == nil {
		t.Fatal("expected error for unknown flag, got nil")
	}
}

func TestRunDryRunEmptyDiscovery(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(context.Background(), []string{"--dry-run"}, &stdout, &stderr, discovery.Fake{})
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "(none found)") {
		t.Errorf("expected '(none found)' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "<aircast>") {
		t.Errorf("expected aircast XML in output, got:\n%s", out)
	}
}

func TestRunDryRunWithDiscoveredDevices(t *testing.T) {
	var stdout, stderr bytes.Buffer
	disc := discovery.Fake{Devices: []discovery.Device{
		{ID: "kitchen-id", Name: "Kitchen Home", Addrs: []string{"192.168.1.10"}},
	}}
	err := run(context.Background(), []string{"--dry-run"}, &stdout, &stderr, disc)
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Kitchen Home") {
		t.Errorf("expected discovered device in output, got:\n%s", out)
	}
	if !strings.Contains(out, "<udn>kitchen-id</udn>") {
		t.Errorf("expected device udn in generated XML, got:\n%s", out)
	}
	if !strings.Contains(out, "<enabled>0</enabled>") {
		t.Errorf("expected discovered device to be disabled-by-default in XML, got:\n%s", out)
	}
}

func TestRunDryRunWithConfigPath(t *testing.T) {
	// Exercises the loadOrDefault non-default branch.
	cfg := config.Default()
	cfg.Devices = []config.Device{{ID: "seeded", Name: "Seeded", Enabled: true}}
	path := writeConfigFile(t, cfg)

	var stdout, stderr bytes.Buffer
	err := run(context.Background(), []string{"--dry-run", "--config", path}, &stdout, &stderr, discovery.Fake{})
	if err != nil {
		t.Fatalf("dry-run with config: %v (stderr=%s)", err, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Seeded") {
		t.Errorf("expected seeded device in output, got:\n%s", out)
	}
	if !strings.Contains(out, "<enabled>1</enabled>") {
		t.Errorf("expected saved enabled device to survive merge, got:\n%s", out)
	}
}

func TestRunServeWithoutConfigErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(context.Background(), []string{"--serve"}, &stdout, &stderr, discovery.Fake{})
	if err == nil {
		t.Fatal("expected --serve without --config to error, got nil")
	}
	if !strings.Contains(stderr.String(), "--config") {
		t.Errorf("expected hint about --config in stderr, got %q", stderr.String())
	}
}
