package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pizzasaurusrex/homecast/internal/bridge"
	"github.com/pizzasaurusrex/homecast/internal/config"
	"github.com/pizzasaurusrex/homecast/internal/discovery"
)

// --- fakes ---------------------------------------------------------------

type fakeConfigStore struct {
	mu        sync.Mutex
	cfg       *config.Config
	upsertErr error
}

func (f *fakeConfigStore) Snapshot() config.Config {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := *f.cfg
	out.Devices = append([]config.Device(nil), f.cfg.Devices...)
	return out
}

func (f *fakeConfigStore) UpsertDevice(d config.Device) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.upsertErr != nil {
		return f.upsertErr
	}
	devices := append([]config.Device(nil), f.cfg.Devices...)
	for i, existing := range devices {
		if existing.ID == d.ID {
			devices[i] = d
			newCfg := *f.cfg
			newCfg.Devices = devices
			f.cfg = &newCfg
			return nil
		}
	}
	devices = append(devices, d)
	newCfg := *f.cfg
	newCfg.Devices = devices
	f.cfg = &newCfg
	return nil
}

type fakeSupervisor struct {
	state        bridge.State
	startedAt    time.Time
	restartErr   error
	restarts     int
	ctxErrAtCall error
}

func (f *fakeSupervisor) State() bridge.State  { return f.state }
func (f *fakeSupervisor) StartedAt() time.Time { return f.startedAt }
func (f *fakeSupervisor) Restart(ctx context.Context, _ time.Duration) error {
	f.ctxErrAtCall = ctx.Err()
	f.restarts++
	return f.restartErr
}

type fakeLogs struct{ lines []string }

func (f *fakeLogs) Tail(n int) []string {
	if n <= 0 || len(f.lines) == 0 {
		return []string{}
	}
	if n >= len(f.lines) {
		out := make([]string, len(f.lines))
		copy(out, f.lines)
		return out
	}
	out := make([]string, n)
	copy(out, f.lines[len(f.lines)-n:])
	return out
}

// silentLogger discards logs in tests unless a test captures them explicitly.
func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// --- helpers -------------------------------------------------------------

func newTestHandler(t *testing.T, opts Options) http.Handler {
	t.Helper()
	if opts.Config == nil {
		opts.Config = &fakeConfigStore{cfg: config.Default()}
	}
	if opts.Discoverer == nil {
		opts.Discoverer = discovery.Fake{}
	}
	if opts.Supervisor == nil {
		opts.Supervisor = &fakeSupervisor{state: bridge.StateStopped}
	}
	if opts.Logs == nil {
		opts.Logs = &fakeLogs{}
	}
	if opts.Logger == nil {
		opts.Logger = silentLogger()
	}
	if opts.DiscoverTimeout == 0 {
		opts.DiscoverTimeout = 50 * time.Millisecond
	}
	if opts.RestartTimeout == 0 {
		opts.RestartTimeout = 50 * time.Millisecond
	}
	if opts.Now == nil {
		opts.Now = func() time.Time { return time.Date(2026, 4, 22, 12, 0, 10, 0, time.UTC) }
	}
	return NewHandler(opts)
}

func decodeEnvelope(t *testing.T, body io.Reader) envelope {
	t.Helper()
	var env envelope
	if err := json.NewDecoder(body).Decode(&env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	return env
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// --- tests ---------------------------------------------------------------

func TestStatus_Running(t *testing.T) {
	startedAt := time.Date(2026, 4, 22, 11, 59, 50, 0, time.UTC)
	h := newTestHandler(t, Options{
		Supervisor: &fakeSupervisor{state: bridge.StateRunning, startedAt: startedAt},
	})
	w := do(t, h, http.MethodGet, "/api/status", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", w.Code, w.Body.String())
	}
	env := decodeEnvelope(t, w.Body)
	if !env.Ok {
		t.Fatalf("want ok=true, got envelope=%+v", env)
	}
	m := env.Data.(map[string]interface{})
	if m["bridgeState"] != "running" {
		t.Fatalf("bridgeState: got %v", m["bridgeState"])
	}
	if m["uptimeSeconds"].(float64) != 20 {
		t.Fatalf("uptimeSeconds: got %v want 20", m["uptimeSeconds"])
	}
	if m["startedAt"] != "2026-04-22T11:59:50Z" {
		t.Fatalf("startedAt: got %v", m["startedAt"])
	}
	if _, ok := m["airconnectVersion"]; ok {
		t.Fatalf("airconnectVersion key must not be present until populated; got %+v", m)
	}
}

func TestStatus_Stopped(t *testing.T) {
	h := newTestHandler(t, Options{
		Supervisor: &fakeSupervisor{state: bridge.StateStopped},
	})
	w := do(t, h, http.MethodGet, "/api/status", "")
	env := decodeEnvelope(t, w.Body)
	m := env.Data.(map[string]interface{})
	if m["bridgeState"] != "stopped" {
		t.Fatalf("bridgeState: got %v", m["bridgeState"])
	}
	if m["uptimeSeconds"].(float64) != 0 {
		t.Fatalf("uptimeSeconds: got %v want 0", m["uptimeSeconds"])
	}
	if m["startedAt"] != nil {
		t.Fatalf("startedAt: got %v want nil", m["startedAt"])
	}
}

func TestDevices_MergesSavedAndDiscovered(t *testing.T) {
	cfg := config.Default()
	cfg.Devices = []config.Device{
		{ID: "kitchen", Name: "Kitchen Home", Enabled: true},
		{ID: "stale", Name: "Stale Speaker", Enabled: false},
	}
	store := &fakeConfigStore{cfg: cfg}
	disc := discovery.Fake{Devices: []discovery.Device{
		{ID: "kitchen", Name: "Kitchen Home", Addrs: []string{"10.0.0.5"}},
		{ID: "living", Name: "Living Room", Addrs: []string{"10.0.0.6"}},
	}}
	h := newTestHandler(t, Options{Config: store, Discoverer: disc})

	w := do(t, h, http.MethodGet, "/api/devices", "")
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d body=%s", w.Code, w.Body.String())
	}
	env := decodeEnvelope(t, w.Body)
	list := env.Data.([]interface{})
	if len(list) != 3 {
		t.Fatalf("want 3 merged devices, got %d: %+v", len(list), list)
	}
	byID := map[string]map[string]interface{}{}
	for _, item := range list {
		d := item.(map[string]interface{})
		byID[d["id"].(string)] = d
	}
	if !byID["kitchen"]["enabled"].(bool) || !byID["kitchen"]["discovered"].(bool) {
		t.Errorf("kitchen: %+v", byID["kitchen"])
	}
	if byID["stale"]["enabled"].(bool) != false || byID["stale"]["discovered"].(bool) != false {
		t.Errorf("stale: %+v", byID["stale"])
	}
	if byID["living"]["enabled"].(bool) != false || byID["living"]["discovered"].(bool) != true {
		t.Errorf("living: %+v", byID["living"])
	}
}

func TestDevices_DiscoveryError_LogsAndReturnsSavedOnly(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	cfg := config.Default()
	cfg.Devices = []config.Device{{ID: "kitchen", Name: "Kitchen", Enabled: true}}
	store := &fakeConfigStore{cfg: cfg}
	disc := discovery.Fake{Err: errors.New("mdns flapped")}
	h := newTestHandler(t, Options{Config: store, Discoverer: disc, Logger: logger})

	w := do(t, h, http.MethodGet, "/api/devices", "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 even on discovery error, got %d body=%s", w.Code, w.Body.String())
	}
	env := decodeEnvelope(t, w.Body)
	list := env.Data.([]interface{})
	if len(list) != 1 {
		t.Fatalf("want 1 saved device, got %d", len(list))
	}
	d := list[0].(map[string]interface{})
	if d["discovered"].(bool) != false {
		t.Errorf("expected discovered=false when mDNS errored, got %v", d)
	}
	if !strings.Contains(logBuf.String(), "mdns flapped") {
		t.Errorf("expected discovery error to be logged, got: %s", logBuf.String())
	}
}

func TestDevices_NilDiscoverer_SavedOnly(t *testing.T) {
	cfg := config.Default()
	cfg.Devices = []config.Device{{ID: "kitchen", Name: "Kitchen", Enabled: true}}
	store := &fakeConfigStore{cfg: cfg}
	h := NewHandler(Options{
		Config:     store,
		Supervisor: &fakeSupervisor{state: bridge.StateStopped},
		Logs:       &fakeLogs{},
		Logger:     silentLogger(),
	})
	w := do(t, h, http.MethodGet, "/api/devices", "")
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d body=%s", w.Code, w.Body.String())
	}
	env := decodeEnvelope(t, w.Body)
	list := env.Data.([]interface{})
	if len(list) != 1 {
		t.Fatalf("want 1 saved device, got %d", len(list))
	}
}

func TestEnable_ExistingSavedDevice(t *testing.T) {
	cfg := config.Default()
	cfg.Devices = []config.Device{{ID: "kitchen", Name: "Kitchen Home", Enabled: false}}
	store := &fakeConfigStore{cfg: cfg}
	h := newTestHandler(t, Options{Config: store})

	w := do(t, h, http.MethodPost, "/api/devices/kitchen/enable", "")
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d body=%s", w.Code, w.Body.String())
	}
	snap := store.Snapshot()
	if !snap.Devices[0].Enabled {
		t.Fatalf("expected device enabled, got %+v", snap.Devices[0])
	}
}

func TestEnable_DiscoveredButNotSaved_Upserts(t *testing.T) {
	store := &fakeConfigStore{cfg: config.Default()}
	disc := discovery.Fake{Devices: []discovery.Device{
		{ID: "new-device", Name: "New Speaker"},
	}}
	h := newTestHandler(t, Options{Config: store, Discoverer: disc})

	w := do(t, h, http.MethodPost, "/api/devices/new-device/enable", "")
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d body=%s", w.Code, w.Body.String())
	}
	snap := store.Snapshot()
	if len(snap.Devices) != 1 || snap.Devices[0].ID != "new-device" || !snap.Devices[0].Enabled || snap.Devices[0].Name != "New Speaker" {
		t.Fatalf("expected upsert, got %+v", snap.Devices)
	}
}

func TestEnable_DiscoveredWithEmptyName_404(t *testing.T) {
	// A malformed mDNS record with no usable name must not be persisted —
	// Validate() rejects empty-name devices, and the file-backed store
	// (slice 4) would error on Save. Treat it as "not found".
	store := &fakeConfigStore{cfg: config.Default()}
	disc := discovery.Fake{Devices: []discovery.Device{
		{ID: "weirdo", Name: ""},
	}}
	h := newTestHandler(t, Options{Config: store, Discoverer: disc})
	w := do(t, h, http.MethodPost, "/api/devices/weirdo/enable", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("code: %d body=%s", w.Code, w.Body.String())
	}
	if len(store.Snapshot().Devices) != 0 {
		t.Fatalf("empty-name device must not be saved")
	}
}

func TestEnable_UnknownDevice_404(t *testing.T) {
	h := newTestHandler(t, Options{})
	w := do(t, h, http.MethodPost, "/api/devices/ghost/enable", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("code: %d body=%s", w.Code, w.Body.String())
	}
	env := decodeEnvelope(t, w.Body)
	if env.Ok || env.Error == "" {
		t.Fatalf("want error envelope, got %+v", env)
	}
}

func TestDisable_ExistingSaved(t *testing.T) {
	cfg := config.Default()
	cfg.Devices = []config.Device{{ID: "kitchen", Name: "Kitchen Home", Enabled: true}}
	store := &fakeConfigStore{cfg: cfg}
	h := newTestHandler(t, Options{Config: store})

	w := do(t, h, http.MethodPost, "/api/devices/kitchen/disable", "")
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d body=%s", w.Code, w.Body.String())
	}
	if store.Snapshot().Devices[0].Enabled {
		t.Fatalf("expected device disabled")
	}
}

func TestDisable_NotSaved_404(t *testing.T) {
	disc := discovery.Fake{Devices: []discovery.Device{{ID: "living", Name: "Living"}}}
	h := newTestHandler(t, Options{Discoverer: disc})
	w := do(t, h, http.MethodPost, "/api/devices/living/disable", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("code: %d body=%s", w.Code, w.Body.String())
	}
}

func TestEnable_UpsertError_500_GenericMessage(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	cfg := config.Default()
	cfg.Devices = []config.Device{{ID: "kitchen", Name: "Kitchen", Enabled: false}}
	store := &fakeConfigStore{cfg: cfg, upsertErr: errors.New("permission denied: /etc/homecast/config.yaml")}
	h := newTestHandler(t, Options{Config: store, Logger: logger})

	w := do(t, h, http.MethodPost, "/api/devices/kitchen/enable", "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("code: %d body=%s", w.Code, w.Body.String())
	}
	env := decodeEnvelope(t, w.Body)
	if strings.Contains(env.Error, "/etc/homecast") || strings.Contains(env.Error, "permission denied") {
		t.Errorf("raw error leaked to client: %q", env.Error)
	}
	if !strings.Contains(logBuf.String(), "permission denied") {
		t.Errorf("expected raw error to be logged server-side, got: %s", logBuf.String())
	}
}

func TestBridgeRestart_Success(t *testing.T) {
	sup := &fakeSupervisor{state: bridge.StateRunning}
	h := newTestHandler(t, Options{Supervisor: sup})
	w := do(t, h, http.MethodPost, "/api/bridge/restart", "")
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d body=%s", w.Code, w.Body.String())
	}
	if sup.restarts != 1 {
		t.Fatalf("expected 1 restart, got %d", sup.restarts)
	}
}

func TestBridgeRestart_SupervisorError_500_Generic(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	sup := &fakeSupervisor{state: bridge.StateRunning, restartErr: errors.New("exec: aircast: /usr/local/lib/homecast/aircast: no such file or directory")}
	h := newTestHandler(t, Options{Supervisor: sup, Logger: logger})
	w := do(t, h, http.MethodPost, "/api/bridge/restart", "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("code: %d body=%s", w.Code, w.Body.String())
	}
	env := decodeEnvelope(t, w.Body)
	if strings.Contains(env.Error, "/usr/local/lib") {
		t.Errorf("raw path leaked to client: %q", env.Error)
	}
	if !strings.Contains(logBuf.String(), "/usr/local/lib") {
		t.Errorf("expected raw error logged, got: %s", logBuf.String())
	}
}

func TestBridgeRestart_UsesDetachedContext(t *testing.T) {
	// If the client disconnects mid-restart, the supervisor's child process
	// (started via exec.CommandContext) must not get killed along with the
	// request. The handler must pass a context independent of r.Context().
	sup := &fakeSupervisor{state: bridge.StateRunning}
	h := newTestHandler(t, Options{Supervisor: sup})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := httptest.NewRequest(http.MethodPost, "/api/bridge/restart", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("code: %d body=%s", w.Code, w.Body.String())
	}
	if sup.ctxErrAtCall != nil {
		t.Fatalf("restart ran with cancelled context: %v", sup.ctxErrAtCall)
	}
}

func TestLogs_DefaultTail(t *testing.T) {
	logs := &fakeLogs{lines: []string{"a", "b", "c"}}
	h := newTestHandler(t, Options{Logs: logs})
	w := do(t, h, http.MethodGet, "/api/logs", "")
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d body=%s", w.Code, w.Body.String())
	}
	env := decodeEnvelope(t, w.Body)
	data := env.Data.(map[string]interface{})
	lines := data["lines"].([]interface{})
	if len(lines) != 3 {
		t.Fatalf("lines: %v", lines)
	}
}

func TestLogs_TailQuery(t *testing.T) {
	logs := &fakeLogs{lines: []string{"a", "b", "c", "d"}}
	h := newTestHandler(t, Options{Logs: logs})
	w := do(t, h, http.MethodGet, "/api/logs?tail=2", "")
	env := decodeEnvelope(t, w.Body)
	lines := env.Data.(map[string]interface{})["lines"].([]interface{})
	if len(lines) != 2 || lines[0] != "c" || lines[1] != "d" {
		t.Fatalf("lines: %v", lines)
	}
}

func TestLogs_InvalidTail_400(t *testing.T) {
	h := newTestHandler(t, Options{})
	w := do(t, h, http.MethodGet, "/api/logs?tail=notanumber", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("code: %d", w.Code)
	}
}

func TestLogs_TailCap(t *testing.T) {
	logs := &fakeLogs{}
	h := newTestHandler(t, Options{Logs: logs})
	w := do(t, h, http.MethodGet, "/api/logs?tail=999999", "")
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d body=%s", w.Code, w.Body.String())
	}
}

func TestConfig_ReturnsSnapshot(t *testing.T) {
	cfg := config.Default()
	cfg.Devices = []config.Device{{ID: "a", Name: "A", Enabled: true}}
	store := &fakeConfigStore{cfg: cfg}
	h := newTestHandler(t, Options{Config: store})
	w := do(t, h, http.MethodGet, "/api/config", "")
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d body=%s", w.Code, w.Body.String())
	}
	env := decodeEnvelope(t, w.Body)
	m := env.Data.(map[string]interface{})
	if _, ok := m["server"]; !ok {
		t.Fatalf("expected server in config, got %+v", m)
	}
}

func TestUnknownRoute_404(t *testing.T) {
	h := newTestHandler(t, Options{})
	w := do(t, h, http.MethodGet, "/api/does-not-exist", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("code: %d", w.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	h := newTestHandler(t, Options{})
	w := do(t, h, http.MethodPut, "/api/status", "")
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("code: %d", w.Code)
	}
}

func TestResponseIsJSON(t *testing.T) {
	h := newTestHandler(t, Options{})
	w := do(t, h, http.MethodGet, "/api/status", "")
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type: %q", ct)
	}
}
