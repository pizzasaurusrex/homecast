package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/pizzasaurusrex/homecast/internal/api"
	"github.com/pizzasaurusrex/homecast/internal/bridge"
	"github.com/pizzasaurusrex/homecast/internal/discovery"
	"github.com/pizzasaurusrex/homecast/internal/logs"
	"github.com/pizzasaurusrex/homecast/internal/web"
)

const (
	logBufferLines    = 1000
	httpShutdownGrace = 5 * time.Second
	bridgeStopGrace   = 5 * time.Second
	httpReadHeader    = 5 * time.Second
	httpReadTimeout   = 30 * time.Second
	httpWriteTimeout  = 30 * time.Second
)

// doServe loads config, wires the supervisor + API + UI, and blocks until
// ctx is cancelled (signal from main) or the server errors out. The context
// passed to bridge.Supervisor.Start is detached from ctx so SIGINT does not
// immediately kill the AirConnect child; we drive the child via Stop during
// graceful shutdown instead.
func doServe(ctx context.Context, stdout, stderr io.Writer, configPath string, disc discovery.Discoverer) error {
	store, err := newFileConfigStore(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	cfg := store.Snapshot()

	logBuf := logs.NewBuffer(logBufferLines)
	sup := bridge.NewSupervisor(cfg.AirConnect.BinaryPath, nil, logBuf, cfg.AirConnect.AutoRestart)
	if err := sup.Start(context.Background()); err != nil {
		// Non-fatal: the UI still wants to be reachable so the operator can
		// inspect logs / fix config / hit restart.
		fmt.Fprintf(stderr, "homecast: supervisor failed to start AirConnect (%v); continuing so UI stays up\n", err)
	}
	sup.Watch(ctx)

	mux := buildServeMux(store, disc, sup, logBuf)

	srv := &http.Server{
		Addr:              cfg.Server.Listen,
		Handler:           mux,
		ReadHeaderTimeout: httpReadHeader,
		ReadTimeout:       httpReadTimeout,
		WriteTimeout:      httpWriteTimeout,
	}

	listener, err := net.Listen("tcp", cfg.Server.Listen)
	if err != nil {
		_ = sup.Stop(bridgeStopGrace)
		return fmt.Errorf("listen %s: %w", cfg.Server.Listen, err)
	}
	fmt.Fprintf(stdout, "homecast: listening on http://%s\n", listener.Addr())

	return runServer(ctx, stderr, srv, listener, sup)
}

// runServer is split out so tests can drive it with a pre-bound listener on
// localhost:0 without going through config loading.
func runServer(ctx context.Context, stderr io.Writer, srv *http.Server, listener net.Listener, sup *bridge.Supervisor) error {
	serveErr := make(chan error, 1)
	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case <-ctx.Done():
		fmt.Fprintln(stderr, "homecast: shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), httpShutdownGrace)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintf(stderr, "homecast: http shutdown: %v\n", err)
		}
		if err := sup.Stop(bridgeStopGrace); err != nil {
			fmt.Fprintf(stderr, "homecast: supervisor stop: %v\n", err)
		}
		<-serveErr
		return nil
	case err := <-serveErr:
		// Server exited on its own (bind failure, etc.). Still tear down the
		// supervisor so we do not leak AirConnect.
		_ = sup.Stop(bridgeStopGrace)
		return err
	}
}

// buildServeMux composes the /api/* JSON router with the embedded web UI.
// Exposed as a separate function so it can be tested in isolation without
// having to stand up a real listener.
func buildServeMux(store api.ConfigStore, disc discovery.Discoverer, sup api.Supervisor, logBuf api.LogTailer) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/", api.NewHandler(api.Options{
		Config:     store,
		Discoverer: disc,
		Supervisor: sup,
		Logs:       logBuf,
	}))
	mux.Handle("/", web.Handler())
	return securityHeaders(mux)
}

// securityHeaders wraps h with defensive HTTP response headers. The CSP
// matches the embedded UI: self-hosted scripts, styles, and fetch() only —
// no inline scripts, no external resources.
func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy",
			"default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; base-uri 'self'")
		h.ServeHTTP(w, r)
	})
}
