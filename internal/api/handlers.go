package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/pizzasaurusrex/homecast/internal/bridge"
)

// statusView is the JSON shape of /api/status. StartedAt is a pointer so the
// "stopped" case emits explicit null rather than a zero-valued timestamp.
//
// TODO(slice-4): populate AirconnectVersion once bridge.Supervisor exposes it.
// Omitted from the response until then so JS clients don't learn to ignore
// a permanently-null field.
type statusView struct {
	BridgeState   string  `json:"bridgeState"`
	StartedAt     *string `json:"startedAt"`
	UptimeSeconds int     `json:"uptimeSeconds"`
}

func (s *server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	state := s.opts.Supervisor.State()
	started := s.opts.Supervisor.StartedAt()

	view := statusView{BridgeState: string(state)}
	if state == bridge.StateRunning && !started.IsZero() {
		ts := started.UTC().Format(time.RFC3339)
		view.StartedAt = &ts
		view.UptimeSeconds = int(s.opts.Now().Sub(started).Seconds())
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *server) handleBridgeRestart(w http.ResponseWriter, _ *http.Request) {
	if err := s.opts.Supervisor.Restart(s.opts.RestartTimeout); err != nil {
		s.internalError(w, "failed to restart bridge", err)
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
	snap := s.opts.Config.Snapshot()
	writeJSON(w, http.StatusOK, snap)
}
