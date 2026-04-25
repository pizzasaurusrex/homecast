package bridge

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

const sleepBin = "/bin/sleep"

func TestSupervisorStartStop(t *testing.T) {
	sup := NewSupervisor(sleepBin, []string{"30"}, io.Discard, false)
	if got := sup.State(); got != StateStopped {
		t.Fatalf("initial state = %v, want Stopped", got)
	}
	if err := sup.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := sup.State(); got != StateRunning {
		t.Fatalf("after Start, state = %v, want Running", got)
	}
	if err := sup.Stop(2 * time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := sup.State(); got != StateStopped {
		t.Fatalf("after Stop, state = %v, want Stopped", got)
	}
}

func TestSupervisorStartTwiceReturnsError(t *testing.T) {
	sup := NewSupervisor(sleepBin, []string{"30"}, io.Discard, false)
	if err := sup.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = sup.Stop(2 * time.Second) })

	err := sup.Start(context.Background())
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second Start = %v, want ErrAlreadyRunning", err)
	}
}

func TestSupervisorStopWhenNotRunningIsNoop(t *testing.T) {
	sup := NewSupervisor(sleepBin, []string{"30"}, io.Discard, false)
	if err := sup.Stop(time.Second); err != nil {
		t.Fatalf("Stop on stopped supervisor returned error: %v", err)
	}
}

func TestSupervisorStartNonexistentBinary(t *testing.T) {
	sup := NewSupervisor("/definitely/does/not/exist/binary", nil, io.Discard, false)
	if err := sup.Start(context.Background()); err == nil {
		t.Fatal("expected error starting nonexistent binary")
	}
	if got := sup.State(); got != StateStopped {
		t.Errorf("failed Start left state = %v, want Stopped", got)
	}
}

func TestSupervisorStartedAt(t *testing.T) {
	sup := NewSupervisor(sleepBin, []string{"30"}, io.Discard, false)
	if got := sup.StartedAt(); !got.IsZero() {
		t.Fatalf("initial StartedAt = %v, want zero", got)
	}
	before := time.Now()
	if err := sup.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = sup.Stop(2 * time.Second) })
	after := time.Now()

	got := sup.StartedAt()
	if got.IsZero() {
		t.Fatal("StartedAt after Start returned zero time")
	}
	if got.Before(before) || got.After(after) {
		t.Errorf("StartedAt = %v, want in [%v, %v]", got, before, after)
	}

	if err := sup.Stop(2 * time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := sup.StartedAt(); !got.IsZero() {
		t.Errorf("StartedAt after Stop = %v, want zero", got)
	}
}

func TestSupervisorRestart(t *testing.T) {
	sup := NewSupervisor(sleepBin, []string{"30"}, io.Discard, false)
	if err := sup.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = sup.Stop(2 * time.Second) })

	if err := sup.Restart(context.Background(), 2*time.Second); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if got := sup.State(); got != StateRunning {
		t.Fatalf("after Restart, state = %v, want Running", got)
	}
}

func TestSupervisorSIGKILLFallback(t *testing.T) {
	// `sleep` exits immediately on SIGTERM, so to exercise the SIGKILL
	// fallback path we'd need a process that ignores SIGTERM. Skip for now
	// and revisit in M3 with a dedicated fake binary if needed.
	t.Skip("sleep responds to SIGTERM; SIGKILL path exercised in M3")
}

func TestSupervisorCapturesOutput(t *testing.T) {
	var buf bytes.Buffer
	sup := NewSupervisor("/bin/echo", []string{"hello-supervisor"}, &buf, false)
	if err := sup.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// echo exits quickly; wait for supervisor to observe it.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if sup.State() == StateStopped {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := buf.String(); got != "hello-supervisor\n" {
		t.Errorf("captured output = %q, want %q", got, "hello-supervisor\n")
	}
}

func TestSupervisorWatch_RestartsOnCrash(t *testing.T) {
	// /bin/echo exits immediately — each run is an unexpected exit (crash).
	sup := NewSupervisor("/bin/echo", []string{"watch-test"}, io.Discard, true)
	sup.initBackoff = 50 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sup.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	sup.Watch(ctx)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if sup.RestartCount() >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := sup.RestartCount(); got < 2 {
		t.Errorf("expected ≥2 auto-restarts, got %d", got)
	}
}

func TestSupervisorWatch_NoopWhenAutoRestartFalse(t *testing.T) {
	sup := NewSupervisor("/bin/echo", []string{"no-restart"}, io.Discard, false)
	sup.initBackoff = 50 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := sup.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	sup.Watch(ctx)

	// Wait for echo to exit and a bit extra to confirm no restart happens.
	time.Sleep(300 * time.Millisecond)

	if got := sup.RestartCount(); got != 0 {
		t.Errorf("expected 0 restarts with autoRestart=false, got %d", got)
	}
	if got := sup.State(); got != StateStopped {
		t.Errorf("expected Stopped, got %v", got)
	}
}

func TestSupervisorWatch_StopsOnContextCancel(t *testing.T) {
	sup := NewSupervisor("/bin/echo", []string{"cancel-test"}, io.Discard, true)
	sup.initBackoff = 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())

	if err := sup.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	sup.Watch(ctx)

	// Let a couple of restarts happen, then cancel.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if sup.RestartCount() >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()

	// Give the watcher a moment to observe cancellation, then verify
	// RestartCount stops growing.
	time.Sleep(100 * time.Millisecond)
	countAfterCancel := sup.RestartCount()
	time.Sleep(300 * time.Millisecond)
	if got := sup.RestartCount(); got > countAfterCancel+1 {
		t.Errorf("restart count kept growing after ctx cancel: %d → %d", countAfterCancel, got)
	}
}

func TestSupervisorWatch_DeliberateStopDoesNotRestart(t *testing.T) {
	sup := NewSupervisor(sleepBin, []string{"30"}, io.Discard, true)
	sup.initBackoff = 50 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := sup.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	sup.Watch(ctx)

	if err := sup.Stop(2 * time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// After deliberate Stop, watcher must not restart the process.
	time.Sleep(300 * time.Millisecond)
	if got := sup.RestartCount(); got != 0 {
		t.Errorf("expected 0 restarts after deliberate Stop, got %d", got)
	}
	if got := sup.State(); got != StateStopped {
		t.Errorf("expected Stopped after deliberate Stop, got %v", got)
	}
}
