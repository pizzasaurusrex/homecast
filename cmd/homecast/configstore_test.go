package main

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/pizzasaurusrex/homecast/internal/config"
)

func writeConfigFile(t *testing.T, cfg *config.Config) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "homecast.yaml")
	if err := cfg.Save(path); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	return path
}

func TestFileConfigStore_LoadAndSnapshot(t *testing.T) {
	seed := config.Default()
	seed.Devices = []config.Device{{ID: "a", Name: "A", Enabled: true}}
	path := writeConfigFile(t, seed)

	store, err := newFileConfigStore(path)
	if err != nil {
		t.Fatalf("newFileConfigStore: %v", err)
	}

	snap := store.Snapshot()
	if len(snap.Devices) != 1 || snap.Devices[0].ID != "a" {
		t.Fatalf("snapshot: %+v", snap.Devices)
	}

	// Mutating the snapshot must not affect the store's backing state.
	snap.Devices[0].Enabled = false
	snap.Devices = append(snap.Devices, config.Device{ID: "rogue", Name: "Rogue", Enabled: true})

	snap2 := store.Snapshot()
	if len(snap2.Devices) != 1 {
		t.Errorf("caller mutation leaked into store: %+v", snap2.Devices)
	}
	if !snap2.Devices[0].Enabled {
		t.Errorf("caller mutation flipped enabled flag: %+v", snap2.Devices[0])
	}
}

func TestFileConfigStore_UpsertInsertsNewDevice(t *testing.T) {
	path := writeConfigFile(t, config.Default())
	store, err := newFileConfigStore(path)
	if err != nil {
		t.Fatalf("newFileConfigStore: %v", err)
	}
	if err := store.UpsertDevice(config.Device{ID: "new", Name: "New", Enabled: true}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	reloaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(reloaded.Devices) != 1 || reloaded.Devices[0].ID != "new" || !reloaded.Devices[0].Enabled {
		t.Fatalf("persisted state wrong: %+v", reloaded.Devices)
	}
}

func TestFileConfigStore_UpsertReplacesExisting(t *testing.T) {
	seed := config.Default()
	seed.Devices = []config.Device{{ID: "a", Name: "A", Enabled: false}}
	path := writeConfigFile(t, seed)

	store, err := newFileConfigStore(path)
	if err != nil {
		t.Fatalf("newFileConfigStore: %v", err)
	}
	if err := store.UpsertDevice(config.Device{ID: "a", Name: "A", Enabled: true}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	reloaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(reloaded.Devices) != 1 || !reloaded.Devices[0].Enabled {
		t.Fatalf("expected replace in place, got %+v", reloaded.Devices)
	}
}

func TestFileConfigStore_UpsertSaveErrorLeavesStateUnchanged(t *testing.T) {
	// Save() writes via a tempfile in the same directory and then renames
	// over the target. Making the directory read-only after the first load
	// forces the rename to fail, simulating a disk-full / permission event.
	seed := config.Default()
	seed.Devices = []config.Device{{ID: "a", Name: "A", Enabled: true}}
	path := writeConfigFile(t, seed)

	store, err := newFileConfigStore(path)
	if err != nil {
		t.Fatalf("newFileConfigStore: %v", err)
	}

	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err = store.UpsertDevice(config.Device{ID: "a", Name: "A", Enabled: false})
	if err == nil {
		t.Fatal("expected Save to fail on read-only dir, got nil")
	}
	snap := store.Snapshot()
	if !snap.Devices[0].Enabled {
		t.Errorf("in-memory state flipped despite Save failure: %+v", snap.Devices[0])
	}
}

func TestFileConfigStore_ConcurrentUpsertsSerialised(t *testing.T) {
	path := writeConfigFile(t, config.Default())
	store, err := newFileConfigStore(path)
	if err != nil {
		t.Fatalf("newFileConfigStore: %v", err)
	}

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			id := "dev-" + string(rune('a'+i))
			if err := store.UpsertDevice(config.Device{ID: id, Name: id, Enabled: true}); err != nil {
				t.Errorf("Upsert %s: %v", id, err)
			}
		}(i)
	}
	wg.Wait()

	reloaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(reloaded.Devices) != n {
		t.Errorf("expected %d devices after concurrent upserts, got %d", n, len(reloaded.Devices))
	}
}
