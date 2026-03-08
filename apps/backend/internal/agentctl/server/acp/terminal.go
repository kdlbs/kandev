package acp

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"

	"go.uber.org/zap"
)

// terminal represents a single ACP terminal (one command execution).
type terminal struct {
	cmd    *exec.Cmd
	output *terminalOutput
	doneCh chan struct{} // closed when the process exits
	mu     sync.Mutex
	killed bool
}

// terminalOutput is a thread-safe, bounded output buffer.
type terminalOutput struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func newTerminalOutput(limit int) *terminalOutput {
	if limit <= 0 {
		limit = 1024 * 1024 // 1MB default
	}
	return &terminalOutput{limit: limit}
}

func (o *terminalOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	n, err := o.buf.Write(p)
	if o.buf.Len() > o.limit {
		excess := o.buf.Len() - o.limit
		o.buf.Next(excess)
		o.truncated = true
	}
	return n, err
}

func (o *terminalOutput) snapshot() (string, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.String(), o.truncated
}

// TerminalManager manages ACP terminal instances.
type TerminalManager struct {
	logger *zap.Logger
	nextID atomic.Int64

	mu        sync.RWMutex
	terminals map[string]*terminal
}

// NewTerminalManager creates a new terminal manager.
func NewTerminalManager(logger *zap.Logger) *TerminalManager {
	return &TerminalManager{
		logger:    logger,
		terminals: make(map[string]*terminal),
	}
}

// Create starts a command in a new terminal and returns its ID.
func (m *TerminalManager) Create(command string, args []string, cwd string, env map[string]string, outputByteLimit int) (string, error) {
	id := fmt.Sprintf("t-%d", m.nextID.Add(1))

	cmd := exec.Command(command, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	if len(env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	out := newTerminalOutput(outputByteLimit)
	cmd.Stdout = out
	cmd.Stderr = out

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start command %q: %w", command, err)
	}

	t := &terminal{
		cmd:    cmd,
		output: out,
		doneCh: make(chan struct{}),
	}

	// Wait for exit in background and close doneCh.
	go func() {
		_ = cmd.Wait()
		close(t.doneCh)
	}()

	m.mu.Lock()
	m.terminals[id] = t
	m.mu.Unlock()

	m.logger.Debug("terminal created",
		zap.String("terminal_id", id),
		zap.String("command", command),
		zap.Strings("args", args),
	)
	return id, nil
}

// Output returns the current output of a terminal.
func (m *TerminalManager) Output(id string) (output string, truncated bool, exitCode *int, signal *string, err error) {
	m.mu.RLock()
	t, ok := m.terminals[id]
	m.mu.RUnlock()
	if !ok {
		return "", false, nil, nil, fmt.Errorf("terminal %q not found", id)
	}

	output, truncated = t.output.snapshot()

	// Check if process has exited.
	select {
	case <-t.doneCh:
		code, sig := exitStatus(t.cmd)
		return output, truncated, code, sig, nil
	default:
		return output, truncated, nil, nil, nil
	}
}

// WaitForExit blocks until the terminal's command exits.
func (m *TerminalManager) WaitForExit(id string) (exitCode *int, signal *string, err error) {
	m.mu.RLock()
	t, ok := m.terminals[id]
	m.mu.RUnlock()
	if !ok {
		return nil, nil, fmt.Errorf("terminal %q not found", id)
	}

	<-t.doneCh
	code, sig := exitStatus(t.cmd)
	return code, sig, nil
}

// Kill sends SIGTERM to the terminal's process without releasing resources.
func (m *TerminalManager) Kill(id string) error {
	m.mu.RLock()
	t, ok := m.terminals[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("terminal %q not found", id)
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.killed {
		return nil
	}
	t.killed = true

	if t.cmd.Process != nil {
		_ = t.cmd.Process.Signal(syscall.SIGTERM)
	}
	return nil
}

// Release kills the terminal (if still running) and removes it.
func (m *TerminalManager) Release(id string) error {
	m.mu.Lock()
	t, ok := m.terminals[id]
	if ok {
		delete(m.terminals, id)
	}
	m.mu.Unlock()
	if !ok {
		return nil
	}

	t.mu.Lock()
	if !t.killed && t.cmd.Process != nil {
		_ = t.cmd.Process.Signal(syscall.SIGKILL)
	}
	t.mu.Unlock()

	m.logger.Debug("terminal released", zap.String("terminal_id", id))
	return nil
}

// ReleaseAll kills and removes all terminals.
func (m *TerminalManager) ReleaseAll() {
	m.mu.Lock()
	terminals := m.terminals
	m.terminals = make(map[string]*terminal)
	m.mu.Unlock()

	for id, t := range terminals {
		if t.cmd.Process != nil {
			_ = t.cmd.Process.Signal(syscall.SIGKILL)
		}
		m.logger.Debug("terminal released (shutdown)", zap.String("terminal_id", id))
	}
}

// exitStatus extracts exit code and signal from a completed command.
func exitStatus(cmd *exec.Cmd) (*int, *string) {
	state := cmd.ProcessState
	if state == nil {
		return nil, nil
	}
	ws, ok := state.Sys().(syscall.WaitStatus)
	if !ok {
		code := state.ExitCode()
		return &code, nil
	}
	if ws.Signaled() {
		sig := ws.Signal().String()
		return nil, &sig
	}
	code := ws.ExitStatus()
	return &code, nil
}
