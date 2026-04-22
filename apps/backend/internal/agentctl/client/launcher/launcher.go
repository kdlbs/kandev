// Package launcher provides functionality to spawn and manage agentctl as a subprocess.
// This is used in standalone mode when kandev wants to manage the agentctl lifecycle
// rather than requiring the user to start it separately.
package launcher

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	agentctlclient "github.com/kandev/kandev/internal/agentctl/client"
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

	// Parent liveness pipe: write-end kept open by the launcher.
	// When the backend dies (even SIGKILL), the kernel closes this FD,
	// breaking the pipe. agentctl detects the break and self-terminates.
	parentPipe *os.File

	// authToken is retrieved via handshake after agentctl starts.
	// agentctl generates its own token; the launcher retrieves it using the bootstrap nonce.
	authToken string
}

// Config holds configuration for the launcher.
type Config struct {
	BinaryPath string // Path to agentctl binary (auto-detected if empty)
	Host       string // Host to bind to (default: localhost)
	Port       int    // Control port (default: 39429)
}

// New creates a new Launcher.
func New(cfg Config, log *logger.Logger) *Launcher {
	if cfg.Host == "" {
		cfg.Host = "localhost"
	}
	if cfg.Port == 0 {
		cfg.Port = 39429
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

// AuthToken returns the auth token retrieved via handshake.
// Used by the backend to authenticate requests to agentctl.
func (l *Launcher) AuthToken() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.authToken
}

// generateNonce creates a cryptographically random 32-byte hex-encoded nonce.
func generateNonce() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate bootstrap nonce: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// agentctlBinaryName returns the platform-appropriate binary name.
func agentctlBinaryName() string {
	if runtime.GOOS == "windows" {
		return "agentctl.exe"
	}
	return "agentctl"
}

// findAgentctlBinary attempts to locate the agentctl binary.
func findAgentctlBinary() string {
	name := agentctlBinaryName()

	// 1. Check same directory as current executable
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// 2. Check PATH
	if path, err := exec.LookPath(name); err == nil {
		return path
	}

	// 3. Check common development locations
	candidates := []string{
		filepath.Join(".", "bin", name),
		filepath.Join(".", name),
		filepath.Join("..", name),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			if abs, err := filepath.Abs(candidate); err == nil {
				return abs
			}
			return candidate
		}
	}

	return name // Fall back to PATH lookup at runtime
}

// Start spawns the agentctl subprocess and waits for it to become healthy.
func (l *Launcher) Start(ctx context.Context) error {
	l.mu.Lock()
	if l.cmd != nil {
		l.mu.Unlock()
		return fmt.Errorf("agentctl already running")
	}

	if err := l.ensurePortAvailable(); err != nil {
		l.mu.Unlock()
		return fmt.Errorf("port %d not available: %w", l.port, err)
	}

	nonce, err := generateNonce()
	if err != nil {
		l.mu.Unlock()
		return err
	}

	if err := l.buildAndStartProcess(nonce); err != nil {
		l.mu.Unlock()
		return err
	}
	// Release the lock before blocking so monitorExit() can close l.exited,
	// enabling the fast-fail path in waitForHealthy if agentctl crashes.
	l.mu.Unlock()

	if err := l.waitForHealthy(ctx); err != nil {
		if killErr := l.cmd.Process.Kill(); killErr != nil {
			l.logger.Warn("failed to kill agentctl process after failed health check", zap.Error(killErr))
		}
		l.closeParentPipe()
		return fmt.Errorf("agentctl failed to become healthy: %w", err)
	}

	token, err := l.performHandshake(ctx, nonce)
	if err != nil {
		return err
	}

	l.mu.Lock()
	l.authToken = token
	l.mu.Unlock()

	l.logger.Info("agentctl is healthy and authenticated")
	return nil
}

// buildAndStartProcess sets up and launches the agentctl subprocess.
func (l *Launcher) buildAndStartProcess(nonce string) error {
	l.logger.Info("starting agentctl subprocess",
		zap.String("binary", l.binaryPath),
		zap.Int("port", l.port),
		zap.String("host", l.host))

	// Use exec.Command (not CommandContext) so we control shutdown via Stop().
	// CommandContext sends SIGKILL on cancellation, preventing graceful shutdown.
	l.cmd = exec.Command(l.binaryPath, fmt.Sprintf("-port=%d", l.port))

	// Inject bootstrap nonce; agentctl generates its own auth token.
	l.cmd.Env = append(os.Environ(), "AGENTCTL_BOOTSTRAP_NONCE="+nonce)
	l.cmd.SysProcAttr = buildSysProcAttr()

	pipeWrite, err := setupLivenessPipe(l.cmd)
	if err != nil {
		return err
	}

	stdout, err := l.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := l.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := l.cmd.Start(); err != nil {
		closePipeOnStartFailure(pipeWrite, l.cmd)
		return fmt.Errorf("failed to start agentctl: %w", err)
	}

	closeChildPipeEnd(l.cmd)
	l.parentPipe = pipeWrite

	l.logger.Info("agentctl process started", zap.Int("pid", l.cmd.Process.Pid))

	go l.pipeOutput("stdout", bufio.NewScanner(stdout))
	go l.pipeOutput("stderr", bufio.NewScanner(stderr))
	go l.monitorExit()

	return nil
}

// performHandshake retrieves agentctl's self-generated auth token using the nonce.
// On failure it kills the process and closes the liveness pipe.
func (l *Launcher) performHandshake(ctx context.Context, nonce string) (string, error) {
	ctl := agentctlclient.NewControlClient(l.host, l.port, l.logger)
	token, err := ctl.Handshake(ctx, nonce)
	if err != nil {
		if killErr := l.cmd.Process.Kill(); killErr != nil {
			l.logger.Warn("failed to kill agentctl after handshake failure", zap.Error(killErr))
		}
		l.closeParentPipe()
		return "", fmt.Errorf("agentctl handshake failed: %w", err)
	}
	return token, nil
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

	// Close the liveness pipe first — this signals agentctl that the parent
	// is shutting down, complementing the SIGTERM that follows.
	l.closeParentPipe()

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

// closeParentPipe closes and nils the parent liveness pipe if open.
// Safe for concurrent use.
func (l *Launcher) closeParentPipe() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closeParentPipeLocked()
}

// closeParentPipeLocked is like closeParentPipe but assumes mu is already held.
func (l *Launcher) closeParentPipeLocked() {
	if l.parentPipe != nil {
		_ = l.parentPipe.Close()
		l.parentPipe = nil
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
