package api

import (
	"net/http"
	"strconv"

	"github.com/pizzasaurusrex/homecast/internal/bridge"
)

func (s *server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	state := s.opts.Supervisor.State()
	started := s.opts.Supervisor.StartedAt()

	data := map[string]interface{}{
		"bridgeState":       string(state),
		"startedAt":         nil,
		"uptimeSeconds":     0,
		"airconnectVersion": nil,
	}
	if state == bridge.StateRunning && !started.IsZero() {
		data["startedAt"] = started.UTC().Format("2006-01-02T15:04:05Z07:00")
		data["uptimeSeconds"] = int(s.opts.Now().Sub(started).Seconds())
	}
	writeJSON(w, http.StatusOK, data)
}

func (s *server) handleBridgeRestart(w http.ResponseWriter, r *http.Request) {
	if err := s.opts.Supervisor.Restart(r.Context(), s.opts.RestartTimeout); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"restarted": true})
}

func (s *server) handleLogs(w http.ResponseWriter, r *http.Request) {
	tail := defaultLogTail
	if q := r.URL.Query().Get("tail"); q != "" {
		n, err := strconv.Atoi(q)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "tail must be a non-negative integer")
			return
		}
		tail = n
	}
	if tail > maxLogTail {
		tail = maxLogTail
	}
	lines := s.opts.Logs.Tail(tail)
	writeJSON(w, http.StatusOK, map[string]any{"lines": lines, "count": len(lines)})
}

func (s *server) handleConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.opts.Config.Snapshot())
}
