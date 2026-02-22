// Package shell provides an embedded PTY-based shell session for agentctl.
// The shell session is automatically created when agentctl starts and destroyed when it stops.
package shell

import (
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
	pty *os.File
	cmd *exec.Cmd

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
}

// Maximum size of output buffer (16KB should be enough for recent history)
const maxOutputBufferSize = 16 * 1024

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

// detectShell returns the appropriate shell for the current OS
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
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell, []string{"-l"} // Login shell
	}

	// Try common shells
	shells := []string{"/bin/bash", "/bin/zsh", "/bin/sh"}
	for _, sh := range shells {
		if _, err := os.Stat(sh); err == nil {
			return sh, []string{"-l"}
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
	return []string{"-l"}
}

// start initializes and starts the shell process
func (s *Session) start(cfg Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cmd = exec.Command(s.shell, s.shellArgs...)
	s.cmd.Dir = cfg.WorkDir
	s.cmd.Env = buildShellEnv(cfg.WorkDir)

	// Start with PTY
	var err error
	s.pty, err = pty.StartWithSize(s.cmd, &pty.Winsize{
		Cols: uint16(cfg.Cols),
		Rows: uint16(cfg.Rows),
	})
	if err != nil {
		return fmt.Errorf("failed to start PTY: %w", err)
	}

	s.running = true
	s.startedAt = time.Now()

	s.logger.Info("shell session started",
		zap.String("shell", s.shell),
		zap.String("cwd", cfg.WorkDir),
		zap.Int("pid", s.cmd.Process.Pid))

	// Start output reader
	go s.readOutput()

	// Wait for process exit
	go s.waitForExit()

	return nil
}

// Stop gracefully stops the shell session.
// Called automatically when agentctl stops.
func (s *Session) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	s.stopping = true // Mark as stopping to prevent respawn
	s.mu.Unlock()

	s.logger.Info("stopping shell session")

	// Signal stop
	close(s.stopCh)

	// Close PTY (sends SIGHUP to process on Unix)
	if s.pty != nil {
		_ = s.pty.Close()
	}

	// Wait for exit with timeout
	select {
	case <-s.doneCh:
		s.logger.Info("shell session stopped gracefully")
	case <-time.After(5 * time.Second):
		s.logger.Warn("shell session stop timeout, force killing")
		if s.cmd != nil && s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
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
func (s *Session) waitForExit() {
	if s.cmd != nil {
		_ = s.cmd.Wait()
	}

	s.mu.Lock()
	stopping := s.stopping
	s.running = false
	s.mu.Unlock()

	// If Stop() was called, don't respawn - just signal done
	if stopping {
		s.logger.Info("shell process exited (stopping)")
		close(s.doneCh)
		return
	}

	s.logger.Info("shell process exited unexpectedly, respawning...")

	// Small delay before respawn to avoid tight loop on repeated failures
	time.Sleep(100 * time.Millisecond)

	// Respawn the shell
	if err := s.respawn(); err != nil {
		s.logger.Error("failed to respawn shell", zap.Error(err))
		close(s.doneCh)
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

	s.cmd = exec.Command(s.shell, s.shellArgs...)
	s.cmd.Dir = s.workDir
	s.cmd.Env = buildShellEnv(s.workDir)

	// Start with PTY
	var err error
	s.pty, err = pty.StartWithSize(s.cmd, &pty.Winsize{
		Cols: uint16(s.config.Cols),
		Rows: uint16(s.config.Rows),
	})
	if err != nil {
		return fmt.Errorf("failed to start PTY: %w", err)
	}

	s.running = true
	s.startedAt = time.Now()

	s.logger.Info("shell session respawned",
		zap.String("shell", s.shell),
		zap.String("cwd", s.workDir),
		zap.Int("pid", s.cmd.Process.Pid))

	// Start output reader
	go s.readOutput()

	// Wait for process exit (recursive respawn on exit)
	go s.waitForExit()

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
