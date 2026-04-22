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
	sup := NewSupervisor(sleepBin, []string{"30"}, io.Discard)
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
	sup := NewSupervisor(sleepBin, []string{"30"}, io.Discard)
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
	sup := NewSupervisor(sleepBin, []string{"30"}, io.Discard)
	if err := sup.Stop(time.Second); err != nil {
		t.Fatalf("Stop on stopped supervisor returned error: %v", err)
	}
}

func TestSupervisorStartNonexistentBinary(t *testing.T) {
	sup := NewSupervisor("/definitely/does/not/exist/binary", nil, io.Discard)
	if err := sup.Start(context.Background()); err == nil {
		t.Fatal("expected error starting nonexistent binary")
	}
	if got := sup.State(); got != StateStopped {
		t.Errorf("failed Start left state = %v, want Stopped", got)
	}
}

func TestSupervisorRestart(t *testing.T) {
	sup := NewSupervisor(sleepBin, []string{"30"}, io.Discard)
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
	sup := NewSupervisor("/bin/echo", []string{"hello-supervisor"}, &buf)
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
