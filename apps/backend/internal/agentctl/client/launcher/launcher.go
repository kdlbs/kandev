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
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// Launcher manages an agentctl subprocess.
type Launcher struct {
	binaryPath string
	host       string
	port       int
	logger     *logger.Logger

	cmd    *exec.Cmd
	exited chan struct{}
	mu     sync.Mutex

	// For clean shutdown
	stopping bool
}

// Config holds configuration for the launcher.
type Config struct {
	BinaryPath string // Path to agentctl binary (auto-detected if empty)
	Host       string // Host to bind to (default: localhost)
	Port       int    // Control port (default: 9999)
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
		binaryPath: cfg.BinaryPath,
		host:       cfg.Host,
		port:       cfg.Port,
		logger:     log.WithFields(zap.String("component", "agentctl-launcher")),
		exited:     make(chan struct{}),
	}
}

// Port returns the actual port agentctl is running on.
// This may differ from the configured port if fallback port selection was used.
func (l *Launcher) Port() int {
	return l.port
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

	// Ensure port is available (diagnoses and cleans up stale processes if needed)
	if err := l.ensurePortAvailable(); err != nil {
		return fmt.Errorf("port %d not available: %w", l.port, err)
	}

	l.logger.Info("starting agentctl subprocess",
		zap.String("binary", l.binaryPath),
		zap.Int("port", l.port),
		zap.String("host", l.host))

	// Build command with flags
	// Note: We use exec.Command (not CommandContext) because we want to control
	// shutdown ourselves via Stop(). CommandContext sends SIGKILL on context
	// cancellation which prevents graceful shutdown.
	l.cmd = exec.Command(l.binaryPath,
		fmt.Sprintf("-port=%d", l.port),
	)

	// Inherit environment from parent process
	l.cmd.Env = os.Environ()

	// Set process attributes:
	// - Pdeathsig on Linux: kernel sends SIGTERM to child when parent dies.
	// - Setpgid: create new process group so Ctrl+C doesn't propagate directly.
	l.cmd.SysProcAttr = buildSysProcAttr()

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
		if killErr := l.cmd.Process.Kill(); killErr != nil {
			l.logger.Warn("failed to kill agentctl process after failed health check", zap.Error(killErr))
		}
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

	// Send graceful stop signal (SIGTERM on Unix, interrupt on Windows)
	if err := l.gracefulStop(pid); err != nil {
		return err
	}

	// Wait for process to exit or context timeout
	select {
	case <-l.exited:
		l.logger.Info("agentctl stopped gracefully")
		return nil
	case <-ctx.Done():
		l.logger.Warn("graceful shutdown timed out, force killing")
		l.forceKill(pid)
		// Wait a bit for the kill to take effect
		select {
		case <-l.exited:
			return nil
		case <-time.After(1 * time.Second):
			return fmt.Errorf("agentctl did not exit after force kill")
		}
	}
}



// checkPortAvailable verifies the given port is not in use.
// It checks by attempting a wildcard bind (matching what agentctl does with ":port").
func checkPortAvailable(port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	return ln.Close()
}

// findFreePort asks the OS for an available port by binding to :0.
func findFreePort() (int, error) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port, nil
}

// ensurePortAvailable checks if the configured port is free. If not, it
// immediately falls back to an OS-assigned free port.
func (l *Launcher) ensurePortAvailable() error {
	if err := checkPortAvailable(l.port); err == nil {
		return nil
	}

	originalPort := l.port

	l.logger.Info("port already in use, selecting a free port",
		zap.Int("port", l.port))

	freePort, err := findFreePort()
	if err != nil {
		return fmt.Errorf("port %d is in use and failed to find alternative: %w", originalPort, err)
	}
	l.logger.Info("using alternative port",
		zap.Int("original_port", originalPort),
		zap.Int("new_port", freePort))
	l.port = freePort
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
		// Check if process already exited (e.g. port bind failure)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-l.exited:
			return fmt.Errorf("agentctl exited unexpectedly during startup (check logs above for bind errors)")
		default:
		}

		resp, err := client.Get(healthURL)
		if err == nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				l.logger.Debug("failed to close health response body", zap.Error(closeErr))
			}
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			l.logger.Debug("health check returned non-200",
				zap.Int("status", resp.StatusCode))
		}

		l.logger.Debug("waiting for agentctl to be healthy",
			zap.Duration("backoff", backoff),
			zap.Error(err))

		// Wait with early exit on process death
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-l.exited:
			return fmt.Errorf("agentctl exited during health check (check logs above for bind errors)")
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	return fmt.Errorf("timeout waiting for agentctl to become healthy")
}

// pipeOutput reads from a scanner and logs each line.
// stderr is logged at WARN level for visibility; stdout remains at DEBUG.
func (l *Launcher) pipeOutput(name string, scanner *bufio.Scanner) {
	for scanner.Scan() {
		line := scanner.Text()
		if name == "stderr" {
			l.logger.Warn(line, zap.String("stream", name))
		} else {
			l.logger.Debug(line, zap.String("stream", name))
		}
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
			zap.Int("pid", l.cmd.Process.Pid),
			zap.Int("exit_code", l.cmd.ProcessState.ExitCode()))
	} else if !stopping {
		l.logger.Info("agentctl exited",
			zap.Int("pid", l.cmd.Process.Pid),
			zap.Int("exit_code", l.cmd.ProcessState.ExitCode()))
	}

	close(l.exited)
}
