package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pizzasaurusrex/homecast/internal/api"
	"github.com/pizzasaurusrex/homecast/internal/bridge"
	"github.com/pizzasaurusrex/homecast/internal/config"
	"github.com/pizzasaurusrex/homecast/internal/discovery"
	"github.com/pizzasaurusrex/homecast/internal/logs"
)

// --- fakes ---------------------------------------------------------------

type stubSupervisor struct {
	state     bridge.State
	startedAt time.Time
	restarts  int
}

func (s *stubSupervisor) State() bridge.State  { return s.state }
func (s *stubSupervisor) StartedAt() time.Time { return s.startedAt }
func (s *stubSupervisor) Restart(_ context.Context, _ time.Duration) error {
	s.restarts++
	s.startedAt = time.Now()
	return nil
}

// --- buildServeMux: routing and composition -----------------------------

func newTestMux(t *testing.T, store api.ConfigStore, disc discovery.Discoverer, sup api.Supervisor, buf api.LogTailer) http.Handler {
	t.Helper()
	if store == nil {
		cfg := config.Default()
		path := writeConfigFile(t, cfg)
		s, err := newFileConfigStore(path)
		if err != nil {
			t.Fatalf("config store: %v", err)
		}
		store = s
	}
	if disc == nil {
		disc = discovery.Fake{}
	}
	if sup == nil {
		sup = &stubSupervisor{state: bridge.StateStopped}
	}
	if buf == nil {
		buf = logs.NewBuffer(50)
	}
	return buildServeMux(store, disc, sup, buf)
}

func TestBuildServeMux_RoutesUIAndAPI(t *testing.T) {
	seed := config.Default()
	seed.Devices = []config.Device{{ID: "kitchen", Name: "Kitchen", Enabled: true}}
	path := writeConfigFile(t, seed)
	store, err := newFileConfigStore(path)
	if err != nil {
		t.Fatalf("config store: %v", err)
	}

	buf := logs.NewBuffer(50)
	fmt.Fprintln(buf, "seeded log line")

	mux := newTestMux(t, store,
		discovery.Fake{Devices: []discovery.Device{{ID: "kitchen", Name: "Kitchen", Addrs: []string{"10.0.0.5"}}}},
		&stubSupervisor{state: bridge.StateRunning, startedAt: time.Now().Add(-30 * time.Second)},
		buf,
	)

	ts := httptestServer(t, mux)
	defer ts.Close()

	// UI at "/"
	body, ct, code := httpGet(t, ts.URL+"/")
	if code != http.StatusOK {
		t.Fatalf("GET /: code=%d", code)
	}
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("GET /: content-type=%q", ct)
	}
	if !strings.Contains(body, "<title>homecast</title>") {
		t.Errorf("GET /: body did not include title; got first 200B: %q", head(body, 200))
	}

	// API
	body, ct, code = httpGet(t, ts.URL+"/api/status")
	if code != http.StatusOK {
		t.Fatalf("GET /api/status: %d", code)
	}
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("GET /api/status: content-type=%q", ct)
	}
	var envelope struct {
		Ok   bool `json:"ok"`
		Data struct {
			BridgeState string `json:"bridgeState"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("parse status body: %v (%s)", err, body)
	}
	if !envelope.Ok || envelope.Data.BridgeState != "running" {
		t.Errorf("status: %+v", envelope)
	}

	// devices merged view
	body, _, code = httpGet(t, ts.URL+"/api/devices")
	if code != http.StatusOK {
		t.Fatalf("GET /api/devices: %d", code)
	}
	if !strings.Contains(body, `"id":"kitchen"`) || !strings.Contains(body, `"discovered":true`) {
		t.Errorf("devices body missing expected fields: %s", body)
	}
}

func TestBuildServeMux_EnablePersists(t *testing.T) {
	path := writeConfigFile(t, config.Default())
	store, err := newFileConfigStore(path)
	if err != nil {
		t.Fatalf("config store: %v", err)
	}
	disc := discovery.Fake{Devices: []discovery.Device{
		{ID: "new-speaker", Name: "New Speaker", Addrs: []string{"10.0.0.9"}},
	}}
	mux := newTestMux(t, store, disc, nil, nil)
	ts := httptestServer(t, mux)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/devices/new-speaker/enable", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST enable: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("enable status: %d", resp.StatusCode)
	}

	reloaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(reloaded.Devices) != 1 || !reloaded.Devices[0].Enabled || reloaded.Devices[0].ID != "new-speaker" {
		t.Errorf("expected persisted enable, got %+v", reloaded.Devices)
	}
}

// --- doServe: end-to-end load + shutdown ---------------------------------

func TestDoServe_StartsAndShutsDown(t *testing.T) {
	seed := config.Default()
	seed.Server.Listen = "127.0.0.1:0"
	// Nonexistent binary: supervisor.Start fails, doServe must log and
	// continue so the UI stays reachable.
	seed.AirConnect.BinaryPath = "/definitely/does/not/exist/aircast"
	path := writeConfigFile(t, seed)

	ctx, cancel := context.WithCancel(context.Background())
	stdout := &syncBuf{}
	stderr := &syncBuf{}

	done := make(chan error, 1)
	go func() { done <- doServe(ctx, stdout, stderr, path, discovery.Fake{}) }()

	// Wait until doServe has printed its "listening on" banner before
	// cancelling, so we exercise the graceful-shutdown branch rather than
	// racing the bind.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(stdout.String(), "listening on") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(stdout.String(), "listening on") {
		t.Fatalf("doServe never announced listening; stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "supervisor failed to start") {
		t.Errorf("expected supervisor-start warning on stderr, got %q", stderr.String())
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("doServe returned %v after cancel", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("doServe did not return within 3s of ctx cancel")
	}
}

func TestDoServe_BadConfigPathReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stdout, stderr bytes.Buffer
	err := doServe(ctx, &stdout, &stderr, "/nonexistent/homecast.yaml", discovery.Fake{})
	if err == nil {
		t.Fatal("expected error for missing config path")
	}
}

// syncBuf is a concurrent-safe io.Writer for capturing goroutine output.
// bytes.Buffer is not goroutine-safe, and `go test -race` will flag the
// natural pattern (one goroutine writes, the test reads).
type syncBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// --- runServer: graceful shutdown ----------------------------------------

func TestRunServer_ShutsDownOnContextCancel(t *testing.T) {
	// Use a real supervisor but never start it, so Stop is a no-op and we
	// exercise the graceful-shutdown code path without needing aircast.
	sup := bridge.NewSupervisor("/bin/true", nil, io.Discard, false)

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())
	var stderr bytes.Buffer

	done := make(chan error, 1)
	go func() { done <- runServer(ctx, &stderr, srv, listener, sup) }()

	// Wait for the server to actually accept connections before we cancel.
	waitForListen(t, addr, 2*time.Second)

	// Hit it once to prove it is serving.
	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("pre-shutdown GET: %v", err)
	}
	_ = resp.Body.Close()

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runServer returned %v after cancel", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runServer did not return within 3s of ctx cancel")
	}
}

func TestRunServer_ReturnsServeError(t *testing.T) {
	sup := bridge.NewSupervisor("/bin/true", nil, io.Discard, false)
	srv := &http.Server{}

	// Close the listener immediately so Serve returns a real error (not
	// ErrServerClosed, which runServer swallows).
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	_ = listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stderr bytes.Buffer
	err = runServer(ctx, &stderr, srv, listener, sup)
	if err == nil {
		t.Fatal("expected serve error, got nil")
	}
	if errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("ErrServerClosed should be swallowed, got wrapper %v", err)
	}
}

// --- helpers -------------------------------------------------------------

type testServer struct {
	URL    string
	closer func()
}

func (ts *testServer) Close() { ts.closer() }

func httptestServer(t *testing.T, h http.Handler) *testServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: h}
	go func() { _ = srv.Serve(listener) }()
	waitForListen(t, listener.Addr().String(), 2*time.Second)
	return &testServer{
		URL: "http://" + listener.Addr().String(),
		closer: func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
		},
	}
}

func httpGet(t *testing.T, url string) (body, contentType string, code int) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b), resp.Header.Get("Content-Type"), resp.StatusCode
}

func waitForListen(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server at %s did not accept within %v", addr, timeout)
}

func head(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
