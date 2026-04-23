package api

import (
	"context"
	"net/http"

	"github.com/pizzasaurusrex/homecast/internal/config"
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
	found, _ := s.browse(r.Context())
	writeJSON(w, http.StatusOK, mergeDevices(cfg.Devices, found))
}

func (s *server) handleDeviceEnable(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cfg := s.opts.Config.Snapshot()

	for _, d := range cfg.Devices {
		if d.ID == id {
			if err := s.opts.Config.UpsertDevice(config.Device{ID: d.ID, Name: d.Name, Enabled: true}); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"id": id, "enabled": true})
			return
		}
	}

	found, _ := s.browse(r.Context())
	for _, d := range found {
		if d.ID == id {
			if err := s.opts.Config.UpsertDevice(config.Device{ID: d.ID, Name: d.Name, Enabled: true}); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"id": id, "enabled": true})
			return
		}
	}

	writeError(w, http.StatusNotFound, "device not found in saved config or discovery")
}

func (s *server) handleDeviceDisable(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cfg := s.opts.Config.Snapshot()

	for _, d := range cfg.Devices {
		if d.ID == id {
			if err := s.opts.Config.UpsertDevice(config.Device{ID: d.ID, Name: d.Name, Enabled: false}); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"id": id, "enabled": false})
			return
		}
	}
	writeError(w, http.StatusNotFound, "device not saved; nothing to disable")
}

func (s *server) browse(ctx context.Context) ([]discovery.Device, error) {
	if s.opts.Discoverer == nil {
		return nil, nil
	}
	return s.opts.Discoverer.Browse(ctx, s.opts.DiscoverTimeout)
}

func mergeDevices(saved []config.Device, discovered []discovery.Device) []deviceView {
	discByID := make(map[string]discovery.Device, len(discovered))
	for _, d := range discovered {
		discByID[d.ID] = d
	}

	seen := make(map[string]struct{}, len(saved)+len(discovered))
	out := make([]deviceView, 0, len(saved)+len(discovered))

	for _, d := range saved {
		disc, ok := discByID[d.ID]
		view := deviceView{ID: d.ID, Name: d.Name, Enabled: d.Enabled, Discovered: ok}
		if ok {
			view.Addrs = append([]string(nil), disc.Addrs...)
		}
		out = append(out, view)
		seen[d.ID] = struct{}{}
	}
	for _, d := range discovered {
		if _, already := seen[d.ID]; already {
			continue
		}
		out = append(out, deviceView{
			ID:         d.ID,
			Name:       d.Name,
			Enabled:    false,
			Discovered: true,
			Addrs:      append([]string(nil), d.Addrs...),
		})
	}
	return out
}
