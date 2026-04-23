package api

import (
	"context"
	"net/http"

	"github.com/pizzasaurusrex/homecast/internal/config"
	"github.com/pizzasaurusrex/homecast/internal/devices"
	"github.com/pizzasaurusrex/homecast/internal/discovery"
)

// deviceView is the JSON shape returned by /api/devices. It merges saved and
// discovered state so the UI can show "saved but not present on the LAN".
type deviceView struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Enabled    bool     `json:"enabled"`
	Discovered bool     `json:"discovered"`
	Addrs      []string `json:"addrs,omitempty"`
}

func (s *server) handleDevices(w http.ResponseWriter, r *http.Request) {
	cfg := s.opts.Config.Snapshot()
	found := s.browse(r.Context())
	merged := devices.Merge(cfg.Devices, found)
	views := make([]deviceView, len(merged))
	for i, m := range merged {
		views[i] = deviceView{
			ID:         m.ID,
			Name:       m.Name,
			Enabled:    m.Enabled,
			Discovered: m.Discovered,
			Addrs:      m.Addrs,
		}
	}
	writeJSON(w, http.StatusOK, views)
}

func (s *server) handleDeviceEnable(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cfg := s.opts.Config.Snapshot()

	if d, ok := findSaved(cfg.Devices, id); ok {
		s.upsert(w, config.Device{ID: d.ID, Name: d.Name, Enabled: true})
		return
	}
	if d, ok := findDiscovered(s.browse(r.Context()), id); ok {
		s.upsert(w, config.Device{ID: d.ID, Name: d.Name, Enabled: true})
		return
	}
	writeError(w, http.StatusNotFound, "device not found in saved config or discovery")
}

func (s *server) handleDeviceDisable(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cfg := s.opts.Config.Snapshot()
	if d, ok := findSaved(cfg.Devices, id); ok {
		s.upsert(w, config.Device{ID: d.ID, Name: d.Name, Enabled: false})
		return
	}
	writeError(w, http.StatusNotFound, "device not saved; nothing to disable")
}

func (s *server) upsert(w http.ResponseWriter, d config.Device) {
	if err := s.opts.Config.UpsertDevice(d); err != nil {
		s.internalError(w, "failed to update device config", err, "deviceID", d.ID)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": d.ID, "enabled": d.Enabled})
}

// browse swallows and logs discovery errors so a transient mDNS failure does
// not turn a useful /api/devices response into a 5xx. Callers get whatever
// devices came back (possibly none).
func (s *server) browse(ctx context.Context) []discovery.Device {
	if s.opts.Discoverer == nil {
		return nil
	}
	devices, err := s.opts.Discoverer.Browse(ctx, s.opts.DiscoverTimeout)
	if err != nil {
		s.opts.Logger.Warn("mDNS discovery failed", "err", err)
	}
	return devices
}

func findSaved(devices []config.Device, id string) (config.Device, bool) {
	for _, d := range devices {
		if d.ID == id {
			return d, true
		}
	}
	return config.Device{}, false
}

// findDiscovered returns the first device with the given id that has both a
// non-empty ID and Name. Malformed mDNS entries are skipped — Validate()
// rejects empty-name devices, and the file-backed store would fail on Save.
func findDiscovered(devices []discovery.Device, id string) (discovery.Device, bool) {
	for _, d := range devices {
		if d.ID == id && d.Name != "" {
			return d, true
		}
	}
	return discovery.Device{}, false
}
