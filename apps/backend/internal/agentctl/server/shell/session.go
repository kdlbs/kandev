// Package shell provides an embedded PTY-based shell session for agentctl.
// The shell session is automatically created when agentctl starts and destroyed when it stops.
package shell

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// Session represents the single embedded shell session for an agentctl instance.
// It is automatically created when agentctl starts and destroyed when it stops.
type Session struct {
	logger    *logger.Logger
	workDir   string
	shell     string   // Shell command (detected based on OS)
	shellArgs []string // Shell arguments
	config    Config   // Stored config for respawn

	// PTY and process
	pty       *os.File
	cmd       *exec.Cmd
	lifecycle shellProcessLifecycleHandle
	reapErr   error

	// State
	running   bool
	startedAt time.Time
	mu        sync.RWMutex

	// Output subscribers (WebSocket connections)
	subscribers map[chan<- []byte]struct{}
	subMu       sync.RWMutex

	// Ring buffer for recent output (for new subscribers)
	outputBuffer []byte
	bufferMu     sync.RWMutex

	// Lifecycle
	stopCh   chan struct{}
	doneCh   chan struct{}
	stopping bool // true when Stop() has been called (don't respawn)

	killGroupFn     func(*os.Process) error
	waitGroupExitFn func(*os.Process) bool
	beforeRespawn   func()
	afterStopClaim  func()
}

// Maximum size of output buffer (16KB should be enough for recent history)
const maxOutputBufferSize = 16 * 1024
const shellStopGrace = 250 * time.Millisecond
const shellReapGrace = 2 * time.Second

func killAndWaitShellCommand(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	_ = cmd.Process.Kill()
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-time.After(shellReapGrace):
		return fmt.Errorf("shell process %d was not reaped after failed startup", cmd.Process.Pid)
	}
}

// Config holds shell session configuration
type Config struct {
	WorkDir      string   // Working directory (default: workspace)
	Cols         int      // Initial terminal columns (default: 80)
	Rows         int      // Initial terminal rows (default: 24)
	ShellCommand string   // Optional shell command override
	ShellArgs    []string // Optional shell args override
}

// DefaultConfig returns the default shell configuration
func DefaultConfig(workDir string) Config {
	return Config{
		WorkDir: workDir,
		Cols:    80,
		Rows:    24,
	}
}

// Status represents the shell status
type Status struct {
	Running   bool
	Pid       int
	Shell     string
	Cwd       string
	StartedAt time.Time
}

// detectShell returns the appropriate shell for the current OS.
// It validates that the shell exists before returning it.
func detectShell() (string, []string) {
	if runtime.GOOS == "windows" {
		// Prefer PowerShell if available
		if _, err := exec.LookPath("pwsh.exe"); err == nil {
			return "pwsh.exe", []string{"-NoLogo", "-NoExit"}
		}
		if _, err := exec.LookPath("powershell.exe"); err == nil {
			return "powershell.exe", []string{"-NoLogo", "-NoExit"}
		}
		return "cmd.exe", nil
	}

	// Unix-like systems (Linux, macOS)
	// Check $SHELL but only use it if the shell actually exists
	// (host's $SHELL may not be installed in Docker containers).
	// No -l flag: Debian's /etc/profile (and /etc/profile.d/*) can overwrite
	// PATH, dropping container-set entries like /data/.npm-global/bin where
	// agent CLIs (claude, codex, ...) are installed.
	if shell := os.Getenv("SHELL"); shell != "" {
		if _, err := exec.LookPath(shell); err == nil {
			return shell, nil
		}
	}

	// Try common shells
	shells := []string{"/bin/bash", "/bin/zsh", "/bin/sh"}
	for _, sh := range shells {
		if _, err := os.Stat(sh); err == nil {
			return sh, nil
		}
	}

	return "/bin/sh", nil
}

// NewSession creates and starts a new shell session.
// This is called automatically when agentctl starts.
func NewSession(cfg Config, log *logger.Logger) (*Session, error) {
	shell, args := detectShell()
	if cfg.ShellCommand != "" {
		shell = cfg.ShellCommand
		if len(cfg.ShellArgs) > 0 {
			args = cfg.ShellArgs
		} else {
			args = defaultShellArgs(shell)
		}
	}

	s := &Session{
		logger:      log.WithFields(zap.String("component", "shell")),
		workDir:     cfg.WorkDir,
		shell:       shell,
		shellArgs:   args,
		config:      cfg,
		subscribers: make(map[chan<- []byte]struct{}),
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}

	if err := s.start(cfg); err != nil {
		return nil, err
	}

	return s, nil
}

func defaultShellArgs(shellCommand string) []string {
	if runtime.GOOS == "windows" {
		lower := strings.ToLower(shellCommand)
		if strings.Contains(lower, "pwsh") || strings.Contains(lower, "powershell") {
			return []string{"-NoLogo", "-NoExit"}
		}
		return nil
	}
	return nil
}

// start initializes and starts the shell process
func (s *Session) start(cfg Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	doneCh := make(chan struct{})

	s.cmd = exec.Command(s.shell, s.shellArgs...)
	s.cmd.Dir = cfg.WorkDir
	s.cmd.Env = buildShellEnv(cfg.WorkDir)
	configureShellProcess(s.cmd)

	// Start with PTY
	var err error
	s.pty, err = pty.StartWithSize(s.cmd, &pty.Winsize{
		Cols: uint16(cfg.Cols),
		Rows: uint16(cfg.Rows),
	})
	if err != nil {
		return fmt.Errorf("failed to start PTY: %w", err)
	}
	lifecycle, lifecycleErr := installShellProcessLifecycle(s.cmd)
	if lifecycleErr != nil {
		_ = s.pty.Close()
		reapErr := killAndWaitShellCommand(s.cmd)
		return errors.Join(fmt.Errorf("failed to install shell process lifecycle: %w", lifecycleErr), reapErr)
	}

	s.running = true
	s.startedAt = time.Now()
	s.doneCh = doneCh
	s.lifecycle = lifecycle
	s.reapErr = nil

	s.logger.Info("shell session started",
		zap.String("shell", s.shell),
		zap.String("cwd", cfg.WorkDir),
		zap.Int("pid", s.cmd.Process.Pid))

	// Start output reader
	go s.readOutput()

	// Wait for process exit
	go s.waitForExit(s.cmd, doneCh, lifecycle)

	return nil
}

// Stop gracefully stops the shell session.
// Called automatically when agentctl stops.
func (s *Session) Stop() error {
	s.mu.Lock()
	cmd := s.cmd
	doneCh := s.doneCh
	var process *os.Process
	if cmd != nil {
		process = cmd.Process
	}
	pid := 0
	if process != nil {
		pid = process.Pid
	}
	lifecycle := s.lifecycle
	if !s.running {
		s.stopping = true
		doneCh := s.doneCh
		s.mu.Unlock()
		if s.afterStopClaim != nil {
			s.afterStopClaim()
		}
		if doneCh != nil {
			select {
			case <-doneCh:
				return s.cleanupShellProcessGroup(process, pid, "stop_retry", lifecycle)
			case <-time.After(shellReapGrace):
				return fmt.Errorf("shell process was not reaped after stop")
			}
		}
		return nil
	}
	s.running = false
	s.stopping = true // Mark as stopping to prevent respawn
	s.mu.Unlock()

	s.logger.Info("stopping shell session",
		zap.String("shell", s.shell),
		zap.String("cwd", s.workDir),
		zap.Int("pid", pid),
		zap.Duration("grace", shellStopGrace))
	s.logger.Debug("shell session stop requested",
		zap.Int("pid", pid),
		zap.String("shell", s.shell),
		zap.String("cwd", s.workDir))

	// Signal stop
	close(s.stopCh)

	// Close PTY (sends SIGHUP to process on Unix)
	if s.pty != nil {
		s.logger.Debug("shell session PTY close requested", zap.Int("pid", pid))
		_ = s.pty.Close()
	}

	// Wait for exit with timeout
	select {
	case <-doneCh:
		s.logger.Info("shell session stopped gracefully")
		return s.cleanupShellProcessGroup(process, pid, "leader_exited", lifecycle)
	case <-time.After(shellStopGrace):
		s.logger.Warn("shell session stop timeout, force killing",
			zap.Int("pid", pid),
			zap.Duration("grace", shellStopGrace))
		if err := s.cleanupShellProcessGroup(process, pid, "stop_timeout", lifecycle); err != nil {
			return err
		}
	}
	if pid == 0 {
		return nil
	}

	select {
	case <-doneCh:
		return nil
	case <-time.After(shellReapGrace):
		return fmt.Errorf("shell process %d was not reaped after force kill", pid)
	}
}

func (s *Session) cleanupShellProcessGroup(
	process *os.Process,
	pid int,
	reason string,
	lifecycle shellProcessLifecycleHandle,
) error {
	if process == nil {
		return nil
	}
	s.logger.Debug("shell session process group cleanup requested",
		zap.Int("pid", pid),
		zap.String("reason", reason))
	if ownsShellProcessLifecycle(lifecycle) {
		if err := reapShellProcessLifecycle(lifecycle); err != nil {
			return fmt.Errorf("reap shell process job: %w", err)
		}
		s.mu.Lock()
		if s.lifecycle == lifecycle {
			s.lifecycle = shellProcessLifecycleHandle{}
			s.reapErr = nil
		}
		s.mu.Unlock()
		return nil
	}
	killGroup := killShellProcessGroup
	if s.killGroupFn != nil {
		killGroup = s.killGroupFn
	}
	if err := killGroup(process); err != nil {
		s.logger.Debug("shell session process group cleanup failed",
			zap.Int("pid", pid),
			zap.String("reason", reason),
			zap.Error(err))
		return err
	}
	if s.waitGroupExitFn != nil {
		if !s.waitGroupExitFn(process) {
			return fmt.Errorf("shell process group %d remains alive after force kill", pid)
		}
		return nil
	}
	deadline := time.NewTimer(shellReapGrace)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for shellProcessGroupAlive(process) {
		select {
		case <-deadline.C:
			return fmt.Errorf("shell process group %d remains alive after force kill", pid)
		case <-ticker.C:
		}
	}
	return nil
}

// Write sends input to the shell
func (s *Session) Write(data []byte) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.running || s.pty == nil {
		return 0, fmt.Errorf("shell not running")
	}

	return s.pty.Write(data)
}

// Resize changes the terminal dimensions of the shell PTY.
func (s *Session) Resize(cols, rows uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running || s.pty == nil {
		return fmt.Errorf("shell not running")
	}

	// Store for respawn
	s.config.Cols = int(cols)
	s.config.Rows = int(rows)

	return pty.Setsize(s.pty, &pty.Winsize{Cols: cols, Rows: rows})
}

// Subscribe adds a subscriber for shell output
func (s *Session) Subscribe(ch chan<- []byte) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	s.subscribers[ch] = struct{}{}
}

// Unsubscribe removes a subscriber
func (s *Session) Unsubscribe(ch chan<- []byte) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	delete(s.subscribers, ch)
}

// Status returns the current shell status
func (s *Session) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pid := 0
	if s.cmd != nil && s.cmd.Process != nil {
		pid = s.cmd.Process.Pid
	}

	return Status{
		Running:   s.running,
		Pid:       pid,
		Shell:     s.shell,
		Cwd:       s.workDir,
		StartedAt: s.startedAt,
	}
}

// readOutput continuously reads from PTY and broadcasts to subscribers
func (s *Session) readOutput() {
	buf := make([]byte, 4096)

	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		n, err := s.pty.Read(buf)
		if err != nil {
			if err != io.EOF {
				s.logger.Debug("shell read error", zap.Error(err))
			}
			return
		}

		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			s.broadcast(data)
		}
	}
}

// broadcast sends data to all subscribers and stores in buffer
func (s *Session) broadcast(data []byte) {
	// Store in ring buffer for new subscribers
	s.appendToBuffer(data)

	s.subMu.RLock()
	defer s.subMu.RUnlock()

	for ch := range s.subscribers {
		select {
		case ch <- data:
		default:
			// Subscriber channel full, skip
		}
	}
}

// appendToBuffer adds data to the output ring buffer
func (s *Session) appendToBuffer(data []byte) {
	s.bufferMu.Lock()
	defer s.bufferMu.Unlock()

	s.outputBuffer = append(s.outputBuffer, data...)

	// Trim if exceeds max size (keep the most recent data)
	if len(s.outputBuffer) > maxOutputBufferSize {
		s.outputBuffer = s.outputBuffer[len(s.outputBuffer)-maxOutputBufferSize:]
	}
}

// GetBufferedOutput returns the buffered recent output
func (s *Session) GetBufferedOutput() []byte {
	s.bufferMu.RLock()
	defer s.bufferMu.RUnlock()

	if len(s.outputBuffer) == 0 {
		return nil
	}

	// Return a copy to avoid race conditions
	result := make([]byte, len(s.outputBuffer))
	copy(result, s.outputBuffer)
	return result
}

// waitForExit waits for the shell process to exit and respawns if not stopping
func (s *Session) waitForExit(cmd *exec.Cmd, doneCh chan struct{}, lifecycle shellProcessLifecycleHandle) {
	if cmd != nil {
		_ = cmd.Wait()
	}
	pid := 0
	var process *os.Process
	if cmd != nil {
		process = cmd.Process
	}
	if process != nil {
		pid = process.Pid
	}
	reapErr := s.cleanupShellProcessGroup(process, pid, "leader_exited", lifecycle)

	s.mu.Lock()
	stopping := s.stopping
	s.running = false
	s.reapErr = reapErr
	s.mu.Unlock()
	if reapErr != nil {
		s.logger.Error("shell process tree was not reaped", zap.Int("pid", pid), zap.Error(reapErr))
		close(doneCh)
		return
	}

	// If Stop() was called, don't respawn - just signal done
	if stopping {
		s.logger.Info("shell process exited (stopping)")
		close(doneCh)
		return
	}

	s.logger.Info("shell process exited unexpectedly, respawning...")

	// Small delay before respawn to avoid tight loop on repeated failures.
	if s.beforeRespawn != nil {
		s.beforeRespawn()
	} else {
		time.Sleep(100 * time.Millisecond)
	}

	// Respawn the shell
	if err := s.respawn(); err != nil {
		s.logger.Error("failed to respawn shell", zap.Error(err))
		close(doneCh)
		return
	}
	s.mu.Lock()
	stoppedBeforeRespawn := s.stopping && !s.running
	s.mu.Unlock()
	if stoppedBeforeRespawn {
		close(doneCh)
	}
}

// respawn restarts the shell process after an unexpected exit
func (s *Session) respawn() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if we're stopping (Stop() called during respawn)
	if s.stopping {
		return nil
	}

	// Close old PTY to prevent file descriptor leak.
	// Don't nil out s.pty — readOutput() may still be reading from it
	// concurrently without the mutex. Closing is sufficient: any in-flight
	// Read() will return an error and the goroutine will exit.
	if s.pty != nil {
		_ = s.pty.Close()
	}

	doneCh := make(chan struct{})
	s.cmd = exec.Command(s.shell, s.shellArgs...)
	s.cmd.Dir = s.workDir
	s.cmd.Env = buildShellEnv(s.workDir)
	configureShellProcess(s.cmd)

	// Start with PTY
	var err error
	s.pty, err = pty.StartWithSize(s.cmd, &pty.Winsize{
		Cols: uint16(s.config.Cols),
		Rows: uint16(s.config.Rows),
	})
	if err != nil {
		return fmt.Errorf("failed to start PTY: %w", err)
	}
	lifecycle, lifecycleErr := installShellProcessLifecycle(s.cmd)
	if lifecycleErr != nil {
		_ = s.pty.Close()
		reapErr := killAndWaitShellCommand(s.cmd)
		return errors.Join(fmt.Errorf("failed to install shell process lifecycle: %w", lifecycleErr), reapErr)
	}

	s.running = true
	s.startedAt = time.Now()
	s.doneCh = doneCh
	s.lifecycle = lifecycle
	s.reapErr = nil

	s.logger.Info("shell session respawned",
		zap.String("shell", s.shell),
		zap.String("cwd", s.workDir),
		zap.Int("pid", s.cmd.Process.Pid))

	// Start output reader
	go s.readOutput()

	// Wait for process exit (recursive respawn on exit)
	go s.waitForExit(s.cmd, doneCh, lifecycle)

	return nil
}

// buildShellEnv creates the environment for the shell process
func buildShellEnv(workDir string) []string {
	env := os.Environ()

	// Set working directory related vars
	env = append(env, "PWD="+workDir)

	// Set terminal type
	env = append(env, "TERM=xterm-256color")
	env = append(env, "LANG=C.UTF-8")
	env = append(env, "LC_ALL=C.UTF-8")

	return env
}
