package main

import (
	"sync"

	"github.com/pizzasaurusrex/homecast/internal/config"
)

// fileConfigStore wraps a *config.Config loaded from disk and persists every
// UpsertDevice back to the same path atomically (config.Save uses
// tempfile + rename). A single mutex serialises reads and writes so concurrent
// /api/devices/*/enable requests cannot interleave a partial save.
type fileConfigStore struct {
	mu   sync.Mutex
	path string
	cfg  *config.Config
}

func newFileConfigStore(path string) (*fileConfigStore, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	return &fileConfigStore{path: path, cfg: cfg}, nil
}

// Snapshot returns a by-value copy of the config, with the Devices slice
// defensively copied so the caller cannot mutate internal state.
func (s *fileConfigStore) Snapshot() config.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := *s.cfg
	out.Devices = append([]config.Device(nil), s.cfg.Devices...)
	return out
}

// UpsertDevice adds or replaces the device with the same ID and persists the
// new config to disk. If the Save fails the in-memory state is left
// untouched, so the next request sees the last-known-good snapshot.
func (s *fileConfigStore) UpsertDevice(d config.Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := *s.cfg
	next.Devices = append([]config.Device(nil), s.cfg.Devices...)

	found := false
	for i, existing := range next.Devices {
		if existing.ID == d.ID {
			next.Devices[i] = d
			found = true
			break
		}
	}
	if !found {
		next.Devices = append(next.Devices, d)
	}

	if err := next.Save(s.path); err != nil {
		return err
	}
	s.cfg = &next
	return nil
}
