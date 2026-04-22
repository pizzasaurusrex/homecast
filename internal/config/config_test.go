package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultIsValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("default config should be valid, got: %v", err)
	}
}

func TestValidateRejectsEmptyListen(t *testing.T) {
	c := Default()
	c.Server.Listen = ""
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for empty listen")
	}
}

func TestValidateRejectsBadListen(t *testing.T) {
	c := Default()
	c.Server.Listen = "no-port"
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for listen without port")
	}
}

func TestValidateRejectsEmptyBinaryPath(t *testing.T) {
	c := Default()
	c.AirConnect.BinaryPath = ""
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for empty binary path")
	}
}

func TestValidateRejectsDuplicateDeviceID(t *testing.T) {
	c := Default()
	c.Devices = []Device{
		{ID: "dup", Name: "A", Enabled: true},
		{ID: "dup", Name: "B", Enabled: false},
	}
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("expected duplicate error, got: %v", err)
	}
}

func TestValidateRejectsEmptyDeviceFields(t *testing.T) {
	cases := []struct {
		name  string
		dev   Device
		field string
	}{
		{"empty id", Device{ID: "", Name: "x"}, "id"},
		{"empty name", Device{ID: "x", Name: ""}, "name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := Default()
			c.Devices = []Device{tc.dev}
			err := c.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("expected error mentioning %q, got: %v", tc.field, err)
			}
		})
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	orig := Default()
	orig.Devices = []Device{
		{ID: "kitchen._googlecast._tcp.local.", Name: "Kitchen Home", Enabled: true},
		{ID: "office._googlecast._tcp.local.", Name: "Office Nest", Enabled: false},
	}
	if err := orig.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(loaded.Devices))
	}
	if loaded.Devices[0].Name != "Kitchen Home" || !loaded.Devices[0].Enabled {
		t.Errorf("device[0] mismatch: %+v", loaded.Devices[0])
	}
	if loaded.Server.Listen != orig.Server.Listen {
		t.Errorf("listen mismatch: got %q want %q", loaded.Server.Listen, orig.Server.Listen)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load("/nonexistent/path/homecast.yaml"); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadBadYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("::: not yaml :::"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestLoadInvalidContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.yaml")
	content := "server:\n  listen: ''\nairconnect:\n  binary_path: /x\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestSaveRejectsInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	c := Default()
	c.Server.Listen = ""
	if err := c.Save(filepath.Join(dir, "config.yaml")); err == nil {
		t.Fatal("expected Save to return validation error")
	}
}

func TestSaveToNonexistentDir(t *testing.T) {
	if err := Default().Save("/nonexistent/dir/homecast.yaml"); err == nil {
		t.Fatal("expected error saving to nonexistent directory")
	}
}

func TestSaveToReadOnlyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Skipf("cannot make dir read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if err := Default().Save(filepath.Join(dir, "config.yaml")); err == nil {
		t.Fatal("expected error saving to read-only dir")
	}
}

func TestSaveAtomicity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := Default().Save(path); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file leaked: %s", e.Name())
		}
	}
}
