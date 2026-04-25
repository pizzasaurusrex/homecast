package bridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

type State string

const (
	StateStopped  State = "stopped"
	StateRunning  State = "running"
	StateStopping State = "stopping"
)

var ErrAlreadyRunning = errors.New("supervisor already running")

type Supervisor struct {
	binary      string
	args        []string
	logOut      io.Writer
	autoRestart bool
	initBackoff time.Duration // default time.Second; override in tests

	mu           sync.Mutex
	cmd          *exec.Cmd
	state        State
	done         chan struct{}
	startedAt    time.Time
	restartCount int

	crashNotify chan struct{}
	watchOnce   sync.Once
}

func NewSupervisor(binary string, args []string, logOut io.Writer, autoRestart bool) *Supervisor {
	if logOut == nil {
		logOut = io.Discard
	}
	return &Supervisor{
		binary:      binary,
		args:        append([]string(nil), args...),
		logOut:      logOut,
		autoRestart: autoRestart,
		initBackoff: time.Second,
		state:       StateStopped,
		crashNotify: make(chan struct{}, 1),
	}
}

func (s *Supervisor) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// StartedAt returns the wall-clock time the currently-running child started,
// or the zero time when the supervisor is not running.
func (s *Supervisor) StartedAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startedAt
}

// RestartCount returns the number of times Watch has successfully restarted
// the child process after an unexpected exit.
func (s *Supervisor) RestartCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.restartCount
}

func (s *Supervisor) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.state == StateRunning || s.state == StateStopping {
		s.mu.Unlock()
		return ErrAlreadyRunning
	}
	cmd := exec.CommandContext(ctx, s.binary, s.args...)
	cmd.Stdout = s.logOut
	cmd.Stderr = s.logOut
	if err := cmd.Start(); err != nil {
		s.mu.Unlock()
		return fmt.Errorf("start %s: %w", s.binary, err)
	}
	done := make(chan struct{})
	s.cmd = cmd
	s.done = done
	s.state = StateRunning
	s.startedAt = time.Now()
	s.mu.Unlock()

	go func() {
		_ = cmd.Wait()
		s.mu.Lock()
		crashed := s.state == StateRunning // StateStopping means deliberate Stop()
		s.state = StateStopped
		s.cmd = nil
		s.startedAt = time.Time{}
		s.mu.Unlock()
		close(done)
		if crashed {
			select {
			case s.crashNotify <- struct{}{}:
			default:
			}
		}
	}()
	return nil
}

func (s *Supervisor) Stop(timeout time.Duration) error {
	s.mu.Lock()
	if s.state == StateStopped {
		s.mu.Unlock()
		return nil
	}
	s.state = StateStopping
	cmd := s.cmd
	done := s.done
	s.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("SIGTERM: %w", err)
	}
	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("SIGKILL: %w", err)
		}
		<-done
		return nil
	}
}

// Restart stops the child with the given timeout and starts a fresh one.
// The new child is always started with context.Background() so it runs until
// explicitly stopped — callers must not pass a short-lived context expecting
// it to bound the child's lifetime.
func (s *Supervisor) Restart(timeout time.Duration) error {
	if err := s.Stop(timeout); err != nil {
		return err
	}
	return s.Start(context.Background())
}

// Watch starts a background goroutine that restarts the supervised process
// after unexpected exits using exponential backoff. It is a no-op when
// autoRestart is false. Watch is idempotent — calling it multiple times starts
// only one watcher goroutine. The watcher stops when ctx is cancelled.
func (s *Supervisor) Watch(ctx context.Context) {
	if !s.autoRestart {
		return
	}
	s.watchOnce.Do(func() {
		go s.watchLoop(ctx)
	})
}

func (s *Supervisor) watchLoop(ctx context.Context) {
	const maxBackoff = 2 * time.Minute
	backoff := s.initBackoff
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.crashNotify:
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		// Use context.Background() so the child is not killed by ctx cancellation
		// during shutdown — Stop() drives the graceful teardown instead.
		if err := s.Start(context.Background()); err == nil {
			s.mu.Lock()
			s.restartCount++
			s.mu.Unlock()
		} else {
			// Binary not found or other hard error; re-enqueue so we keep trying.
			select {
			case s.crashNotify <- struct{}{}:
			default:
			}
		}
		backoff = min(backoff*2, maxBackoff)
	}
}
