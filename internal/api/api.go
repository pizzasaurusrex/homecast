// Package api serves homecast's HTTP JSON API and embedded web UI.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/pizzasaurusrex/homecast/internal/bridge"
	"github.com/pizzasaurusrex/homecast/internal/config"
	"github.com/pizzasaurusrex/homecast/internal/discovery"
)

// ConfigStore is the subset of config operations the api needs. Snapshot
// returns by value so callers cannot mutate internal state.
type ConfigStore interface {
	Snapshot() config.Config
	UpsertDevice(d config.Device) error
}

// Supervisor is the subset of bridge.Supervisor the api talks to.
type Supervisor interface {
	State() bridge.State
	StartedAt() time.Time
	Restart(timeout time.Duration) error
}

// LogTailer returns the most recent N log lines, oldest first.
type LogTailer interface {
	Tail(n int) []string
}

// Options configures a new API handler.
type Options struct {
	Config          ConfigStore
	Discoverer      discovery.Discoverer
	Supervisor      Supervisor
	Logs            LogTailer
	Logger          *slog.Logger
	DiscoverTimeout time.Duration
	RestartTimeout  time.Duration
	Now             func() time.Time
	// OnBeforeRestart is called before the supervisor is restarted. Use it to
	// regenerate the AirConnect XML config from the current saved device list.
	// If it returns an error the restart is aborted.
	OnBeforeRestart func() error
}

const (
	defaultLogTail = 200
	maxLogTail     = 2000
)

// NewHandler returns an http.Handler that serves the /api/* routes.
func NewHandler(opts Options) http.Handler {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.DiscoverTimeout == 0 {
		opts.DiscoverTimeout = 3 * time.Second
	}
	if opts.RestartTimeout == 0 {
		opts.RestartTimeout = 5 * time.Second
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	s := &server{opts: opts}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/devices", s.handleDevices)
	mux.HandleFunc("POST /api/devices/{id}/enable", s.handleDeviceEnable)
	mux.HandleFunc("POST /api/devices/{id}/disable", s.handleDeviceDisable)
	mux.HandleFunc("POST /api/bridge/restart", s.handleBridgeRestart)
	mux.HandleFunc("GET /api/logs", s.handleLogs)
	mux.HandleFunc("GET /api/config", s.handleConfig)
	return mux
}

type server struct {
	opts Options
}

// envelope is the JSON response wrapper. Ok is always set; Data is populated on
// success, Error on failure.
type envelope struct {
	Ok    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Ok: true, Data: data})
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Ok: false, Error: msg})
}

// internalError logs err server-side and returns a generic 500 to the client so
// filesystem paths and similar details don't leak into browser responses.
func (s *server) internalError(w http.ResponseWriter, clientMsg string, err error, logAttrs ...any) {
	attrs := append([]any{"err", err}, logAttrs...)
	s.opts.Logger.Error(clientMsg, attrs...)
	writeError(w, http.StatusInternalServerError, clientMsg)
}
