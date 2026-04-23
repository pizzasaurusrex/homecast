// Package devices unifies the saved-config + mDNS-discovery device views so
// the CLI (`--dry-run`) and the HTTP API (`/api/devices`) agree on exactly
// which speakers exist, which ones came from config, and which were seen
// on the LAN. Before this package both call sites had near-duplicate merge
// loops that silently diverged on edge cases (e.g. empty-name mDNS records).
package devices

import (
	"github.com/pizzasaurusrex/homecast/internal/config"
	"github.com/pizzasaurusrex/homecast/internal/discovery"
)

// Merged is the canonical row produced by merging a saved config with the
// latest mDNS browse. Callers project this into whatever shape they need:
// config.Device (for aircast XML) or an API view struct (for the UI).
type Merged struct {
	ID         string
	Name       string
	Enabled    bool
	Discovered bool
	Addrs      []string
}

// Merge returns the union of saved and discovered devices.
//
// Order: saved devices first, in input order, each tagged with Discovered/Addrs
// from mDNS when present; then any discovered-only devices in the order mDNS
// returned them. Discovered rows with an empty ID or empty Name are dropped —
// they are malformed and would fail config.Validate on save, and produce
// broken aircast XML if written.
func Merge(saved []config.Device, discovered []discovery.Device) []Merged {
	discByID := make(map[string]discovery.Device, len(discovered))
	for _, d := range discovered {
		if d.ID == "" || d.Name == "" {
			continue
		}
		if _, dup := discByID[d.ID]; dup {
			continue
		}
		discByID[d.ID] = d
	}

	out := make([]Merged, 0, len(saved)+len(discovered))
	seen := make(map[string]struct{}, len(saved)+len(discovered))

	for _, s := range saved {
		m := Merged{ID: s.ID, Name: s.Name, Enabled: s.Enabled}
		if d, ok := discByID[s.ID]; ok {
			m.Discovered = true
			m.Addrs = append([]string(nil), d.Addrs...)
		}
		out = append(out, m)
		seen[s.ID] = struct{}{}
	}

	for _, d := range discovered {
		if d.ID == "" || d.Name == "" {
			continue
		}
		if _, already := seen[d.ID]; already {
			continue
		}
		seen[d.ID] = struct{}{}
		out = append(out, Merged{
			ID:         d.ID,
			Name:       d.Name,
			Enabled:    false,
			Discovered: true,
			Addrs:      append([]string(nil), d.Addrs...),
		})
	}
	return out
}
