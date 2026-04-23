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
	binary string
	args   []string
	logOut io.Writer

	mu        sync.Mutex
	cmd       *exec.Cmd
	state     State
	done      chan struct{}
	startedAt time.Time
}

func NewSupervisor(binary string, args []string, logOut io.Writer) *Supervisor {
	if logOut == nil {
		logOut = io.Discard
	}
	return &Supervisor{
		binary: binary,
		args:   append([]string(nil), args...),
		logOut: logOut,
		state:  StateStopped,
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
		s.state = StateStopped
		s.cmd = nil
		s.startedAt = time.Time{}
		s.mu.Unlock()
		close(done)
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

func (s *Supervisor) Restart(ctx context.Context, timeout time.Duration) error {
	if err := s.Stop(timeout); err != nil {
		return err
	}
	return s.Start(ctx)
}
