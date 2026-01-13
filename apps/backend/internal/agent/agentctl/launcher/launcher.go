// Package launcher provides functionality to spawn and manage agentctl as a subprocess.
// This is used in standalone mode when kandev wants to manage the agentctl lifecycle
// rather than requiring the user to start it separately.
package launcher

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// Launcher manages an agentctl subprocess.
type Launcher struct {
	binaryPath             string
	host                   string
	port                   int
	autoApprovePermissions bool
	logger                 *logger.Logger

	cmd    *exec.Cmd
	exited chan struct{}
	mu     sync.Mutex

	// For clean shutdown
	stopping bool
}

// Config holds configuration for the launcher.
type Config struct {
	BinaryPath             string // Path to agentctl binary (auto-detected if empty)
	Host                   string // Host to bind to (default: localhost)
	Port                   int    // Control port (default: 9999)
	AutoApprovePermissions bool   // Auto-approve permission requests (default: true for standalone)
}

// New creates a new Launcher.
func New(cfg Config, log *logger.Logger) *Launcher {
	if cfg.Host == "" {
		cfg.Host = "localhost"
	}
	if cfg.Port == 0 {
		cfg.Port = 9999
	}
	if cfg.BinaryPath == "" {
		cfg.BinaryPath = findAgentctlBinary()
	}

	return &Launcher{
		binaryPath:             cfg.BinaryPath,
		host:                   cfg.Host,
		port:                   cfg.Port,
		autoApprovePermissions: cfg.AutoApprovePermissions,
		logger:                 log.WithFields(zap.String("component", "agentctl-launcher")),
		exited:                 make(chan struct{}),
	}
}

// findAgentctlBinary attempts to locate the agentctl binary.
func findAgentctlBinary() string {
	// 1. Check same directory as current executable
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "agentctl")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// 2. Check PATH
	if path, err := exec.LookPath("agentctl"); err == nil {
		return path
	}

	// 3. Check common development locations
	candidates := []string{
		"./bin/agentctl",
		"./agentctl",
		"../agentctl",
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			if abs, err := filepath.Abs(candidate); err == nil {
				return abs
			}
			return candidate
		}
	}

	return "agentctl" // Fall back to PATH lookup at runtime
}

// Start spawns the agentctl subprocess and waits for it to become healthy.
func (l *Launcher) Start(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.cmd != nil {
		return fmt.Errorf("agentctl already running")
	}

	// Check if port is available
	if err := l.checkPortAvailable(); err != nil {
		return fmt.Errorf("port %d not available: %w", l.port, err)
	}

	l.logger.Info("starting agentctl subprocess",
		zap.String("binary", l.binaryPath),
		zap.Int("port", l.port),
		zap.Bool("auto_approve_permissions", l.autoApprovePermissions))

	// Build command with standalone mode flags
	// Note: We use exec.Command (not CommandContext) because we want to control
	// shutdown ourselves via Stop(). CommandContext sends SIGKILL on context
	// cancellation which prevents graceful shutdown.
	l.cmd = exec.Command(l.binaryPath,
		"--mode=standalone",
		fmt.Sprintf("--control-port=%d", l.port),
	)

	// Set environment variables (inherit from parent + add agentctl-specific ones)
	l.cmd.Env = append(os.Environ(),
		fmt.Sprintf("AGENTCTL_AUTO_APPROVE_PERMISSIONS=%t", l.autoApprovePermissions),
	)

	// Set process attributes:
	// - Pdeathsig: kernel sends SIGTERM to child when parent dies (crash protection)
	// - Setpgid: create new process group so Ctrl+C doesn't propagate directly
	//   (we handle shutdown ourselves via Stop())
	l.cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGTERM,
		Setpgid:   true, // Don't inherit parent's process group
	}

	// Capture stdout and stderr
	stdout, err := l.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := l.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the process
	if err := l.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start agentctl: %w", err)
	}

	l.logger.Info("agentctl process started", zap.Int("pid", l.cmd.Process.Pid))

	// Pipe stdout/stderr to logger in background
	go l.pipeOutput("stdout", bufio.NewScanner(stdout))
	go l.pipeOutput("stderr", bufio.NewScanner(stderr))

	// Monitor process exit in background
	go l.monitorExit()

	// Wait for health check to pass
	if err := l.waitForHealthy(ctx); err != nil {
		// Kill the process if health check fails
		l.cmd.Process.Kill()
		return fmt.Errorf("agentctl failed to become healthy: %w", err)
	}

	l.logger.Info("agentctl is healthy and ready")
	return nil
}

// Stop gracefully shuts down the agentctl subprocess.
func (l *Launcher) Stop(ctx context.Context) error {
	l.mu.Lock()

	if l.cmd == nil || l.cmd.Process == nil {
		l.mu.Unlock()
		return nil
	}

	// Check if already exited
	select {
	case <-l.exited:
		l.mu.Unlock()
		l.logger.Info("agentctl already stopped")
		return nil
	default:
	}

	l.stopping = true
	pid := l.cmd.Process.Pid
	l.mu.Unlock()

	l.logger.Info("stopping agentctl subprocess", zap.Int("pid", pid))

	// Send SIGTERM for graceful shutdown
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		l.logger.Warn("failed to send SIGTERM, trying SIGKILL", zap.Error(err))
		_ = syscall.Kill(pid, syscall.SIGKILL)
		return err
	}

	// Wait for process to exit or context timeout
	select {
	case <-l.exited:
		l.logger.Info("agentctl stopped gracefully")
		return nil
	case <-ctx.Done():
		l.logger.Warn("graceful shutdown timed out, sending SIGKILL")
		_ = syscall.Kill(pid, syscall.SIGKILL)
		// Wait a bit for the kill to take effect
		select {
		case <-l.exited:
			return nil
		case <-time.After(2 * time.Second):
			return fmt.Errorf("agentctl did not exit after SIGKILL")
		}
	}
}

// Wait blocks until the agentctl process exits.
// Returns the exit error if any.
func (l *Launcher) Wait() error {
	<-l.exited
	if l.cmd != nil && l.cmd.ProcessState != nil {
		if !l.cmd.ProcessState.Success() {
			return fmt.Errorf("agentctl exited with code %d", l.cmd.ProcessState.ExitCode())
		}
	}
	return nil
}

// Running returns true if agentctl is currently running.
func (l *Launcher) Running() bool {
	select {
	case <-l.exited:
		return false
	default:
		return l.cmd != nil && l.cmd.Process != nil
	}
}

// Port returns the control port agentctl is listening on.
func (l *Launcher) Port() int {
	return l.port
}

// checkPortAvailable verifies the port is not in use.
func (l *Launcher) checkPortAvailable() error {
	addr := fmt.Sprintf("%s:%d", l.host, l.port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	ln.Close()
	return nil
}

// waitForHealthy polls the health endpoint until it succeeds or times out.
func (l *Launcher) waitForHealthy(ctx context.Context) error {
	healthURL := fmt.Sprintf("http://%s:%d/health", l.host, l.port)
	client := &http.Client{Timeout: 2 * time.Second}

	// Use exponential backoff: 100ms, 200ms, 400ms, 800ms, 1s, 1s, ...
	backoff := 100 * time.Millisecond
	maxBackoff := 1 * time.Second
	deadline := time.Now().Add(30 * time.Second)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-l.exited:
			return fmt.Errorf("agentctl exited unexpectedly during startup")
		default:
		}

		resp, err := client.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		l.logger.Debug("waiting for agentctl to be healthy",
			zap.Duration("backoff", backoff),
			zap.Error(err))

		time.Sleep(backoff)
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	return fmt.Errorf("timeout waiting for agentctl to become healthy")
}

// pipeOutput reads from a scanner and logs each line.
func (l *Launcher) pipeOutput(name string, scanner *bufio.Scanner) {
	for scanner.Scan() {
		line := scanner.Text()
		// Log at info level with agentctl prefix
		l.logger.Info(line, zap.String("stream", name))
	}
}

// monitorExit waits for the process to exit and signals via the exited channel.
func (l *Launcher) monitorExit() {
	err := l.cmd.Wait()

	l.mu.Lock()
	stopping := l.stopping
	l.mu.Unlock()

	if err != nil && !stopping {
		l.logger.Error("agentctl exited unexpectedly",
			zap.Error(err),
			zap.Int("exit_code", l.cmd.ProcessState.ExitCode()))
	} else if !stopping {
		l.logger.Info("agentctl exited",
			zap.Int("exit_code", l.cmd.ProcessState.ExitCode()))
	}

	close(l.exited)
}

