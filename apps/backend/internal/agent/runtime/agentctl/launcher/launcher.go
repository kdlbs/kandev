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
	"strings"
	"sync"
	"time"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	commonconfig "github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// Launcher manages an agentctl subprocess.
type Launcher struct {
	binaryPath       string
	host             string
	port             int
	logger           *logger.Logger
	onUnexpectedExit func()
	startupConfig    commonconfig.AgentctlStartupConfig

	cmd    *exec.Cmd
	exited chan struct{}
	mu     sync.Mutex

	// For clean shutdown
	stopping bool

	// Parent liveness pipe: write-end kept open by the launcher.
	// When the backend dies (even SIGKILL), the kernel closes this FD,
	// breaking the pipe. agentctl detects the break and self-terminates.
	parentPipe *os.File

	// jobHandle holds a platform-specific kernel object used to enforce
	// "kill the child if the parent dies" without relying on the agentctl
	// process noticing on its own. On Windows it stores a Job Object handle
	// (see lifecycle_windows.go) and is accessed atomically. Unused on Unix
	// — there, the inherited pipe in launcher_pipe_unix.go covers the same
	// role; we keep the field on the shared struct so platform-specific
	// install/release methods (lifecycle_{unix,windows}.go) compile against
	// the same Launcher type.
	jobHandle uintptr //nolint:unused // referenced from lifecycle_windows.go

	// authToken is retrieved via handshake after agentctl starts.
	// agentctl generates its own token; the launcher retrieves it using the bootstrap nonce.
	authToken string
}

// Config holds configuration for the launcher.
type Config struct {
	BinaryPath       string // Path to agentctl binary (auto-detected if empty)
	Host             string // Host to bind to (default: localhost)
	Port             int    // Control port (default: 39429)
	OnUnexpectedExit func() // Called once when the child exits without Stop.
	StartupConfig    commonconfig.AgentctlStartupConfig
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
		binaryPath:       cfg.BinaryPath,
		host:             cfg.Host,
		port:             cfg.Port,
		onUnexpectedExit: cfg.OnUnexpectedExit,
		startupConfig:    cfg.StartupConfig,
		logger:           log.WithFields(zap.String("component", "agentctl-launcher")),
		exited:           make(chan struct{}),
	}
}

// BinaryPath returns the resolved path to the agentctl binary.
func (l *Launcher) BinaryPath() string {
	return l.binaryPath
}

// Port returns the actual port agentctl is running on.
// This may differ from the configured port if fallback port selection was used.
func (l *Launcher) Port() int {
	return l.port
}

// Pid returns the OS process id of the running agentctl control-server, or 0 if
// it has not started yet. This is the host-local liveness handle recorded in
// executors_running.local_pid for local/standalone rows (#1597 truthful executor rows).
func (l *Launcher) Pid() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cmd == nil || l.cmd.Process == nil {
		return 0
	}
	return l.cmd.Process.Pid
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
		l.forceKill(l.cmd.Process.Pid)
		l.closeParentPipe()
		l.releaseChildLifecycle()
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

	// Inject bootstrap nonce and the resolved child contract. Remove inherited
	// copies first so a managed child cannot observe a conflicting host value.
	overrides := []string{"AGENTCTL_BOOTSTRAP_NONCE=" + nonce}
	if l.startupConfig.Configured {
		encoded, err := commonconfig.EncodeAgentctlStartupConfig(l.startupConfig)
		if err != nil {
			return err
		}
		overrides = append(overrides, commonconfig.InternalAgentctlStartupConfigEnv+"="+encoded)
	}
	l.cmd.Env = environmentWithOverrides(os.Environ(), overrides...)
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

	// Bind the child to a kill-on-parent-exit kernel primitive (Job Object on
	// Windows, no-op on Unix where the inherited liveness pipe already covers
	// this). Failure is non-fatal: agentctl still works, but a parent crash
	// may leak an agentctl.exe that holds the control port (issue #892).
	if err := l.installChildLifecycle(l.cmd); err != nil {
		l.logger.Warn("failed to install child lifecycle protection; agentctl may outlive a parent crash",
			zap.Error(err))
	}

	l.logger.Info("agentctl process started", zap.Int("pid", l.cmd.Process.Pid))

	go l.pipeOutput("stdout", bufio.NewScanner(stdout))
	go l.pipeOutput("stderr", bufio.NewScanner(stderr))
	go l.monitorExit()

	return nil
}

func environmentWithOverrides(base []string, overrides ...string) []string {
	keys := make(map[string]struct{}, len(overrides))
	for _, entry := range overrides {
		key, _, ok := strings.Cut(entry, "=")
		if ok && key != "" {
			keys[key] = struct{}{}
		}
	}
	result := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key, _, ok := strings.Cut(entry, "=")
		if ok {
			if _, overridden := keys[key]; overridden {
				continue
			}
		}
		result = append(result, entry)
	}
	return append(result, overrides...)
}

// performHandshake retrieves agentctl's self-generated auth token using the nonce.
// On failure it kills the process and closes the liveness pipe.
func (l *Launcher) performHandshake(ctx context.Context, nonce string) (string, error) {
	ctl := agentctlclient.NewControlClient(l.host, l.port, l.logger)
	token, err := ctl.Handshake(ctx, nonce)
	if err != nil {
		l.forceKill(l.cmd.Process.Pid)
		l.closeParentPipe()
		l.releaseChildLifecycle()
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
	l.logger.Debug("agentctl subprocess stop requested", zap.Int("pid", pid))

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
		l.releaseChildLifecycle()
		return nil
	case <-ctx.Done():
		l.logger.Warn("graceful shutdown timed out, force killing")
		l.forceKill(pid)
		// Wait a bit for the kill to take effect
		select {
		case <-l.exited:
			l.releaseChildLifecycle()
			return nil
		case <-time.After(1 * time.Second):
			l.releaseChildLifecycle()
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

// pipeOutput reads from a scanner and logs each line. stdout is diagnostic
// noise (ACP travels over the socket, not stdout) so it stays at DEBUG.
// stderr carries the child's own structured logs; rather than flatten every
// line to WARN — which turns routine child INFO/DEBUG into shutdown noise in
// the parent log — it is forwarded at the level the child already tagged it
// with. Lines without a recognizable level fall back to WARN so unstructured
// panics/tracebacks stay visible.
func (l *Launcher) pipeOutput(name string, scanner *bufio.Scanner) {
	for scanner.Scan() {
		line := scanner.Text()
		if name != "stderr" {
			l.logger.Debug(line, zap.String("stream", name))
			continue
		}
		switch childLogLevel(line) {
		case "DEBUG":
			l.logger.Debug(line, zap.String("stream", name))
		case "INFO":
			l.logger.Info(line, zap.String("stream", name))
		case "ERROR", "FATAL", "PANIC", "DPANIC":
			l.logger.Error(line, zap.String("stream", name))
		default:
			l.logger.Warn(line, zap.String("stream", name))
		}
	}
}

// childLogLevel extracts a recognized level from the agentctl child's trusted
// structured log formats. It returns the uppercased level ("INFO"/"WARN"/…)
// or "" when the line does not match one of those formats.
func childLogLevel(line string) string {
	// A well-formed console record is "<ts>\t<LEVEL>\t<caller>\t<msg>", so the
	// level token must be bounded by at least a following caller field. Requiring
	// three segments rejects truncated lines (e.g. "<ts>\t<token>") whose second
	// field is not actually a level, so they fall back to WARN.
	fields := strings.SplitN(line, "\t", 4)
	if len(fields) >= 3 {
		if level := recognizedChildLogLevel(stripANSI(fields[1])); level != "" {
			return level
		}
	}

	if level := childSlogTextLevel(line); level != "" {
		return level
	}

	// slog.Default uses the standard log package's date/time prefix followed by
	// the level and message, for example "2026/08/12 10:00:00 INFO message".
	defaultFields := strings.Fields(line)
	if len(defaultFields) < 3 {
		return ""
	}
	if _, err := time.Parse("2006/01/02 15:04:05", defaultFields[0]+" "+defaultFields[1]); err != nil {
		return ""
	}
	return recognizedChildLogLevel(stripANSI(defaultFields[2]))
}

func childSlogTextLevel(line string) string {
	fields, ok := splitSlogTextFields(line)
	if !ok || len(fields) < 3 || !strings.HasPrefix(fields[0], "time=") ||
		!strings.HasPrefix(fields[1], "level=") {
		return ""
	}
	for _, field := range fields[2:] {
		if strings.HasPrefix(field, "msg=") {
			return recognizedChildLogLevel(stripANSI(strings.TrimPrefix(fields[1], "level=")))
		}
	}
	return ""
}

func splitSlogTextFields(line string) ([]string, bool) {
	fields := make([]string, 0, 4)
	start := -1
	inQuotes := false
	escaped := false
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if start < 0 {
			if ch == ' ' || ch == '\t' {
				continue
			}
			start = i
		}
		if inQuotes {
			switch {
			case escaped:
				escaped = false
			case ch == '\\':
				escaped = true
			case ch == '"':
				inQuotes = false
			}
			continue
		}
		switch ch {
		case '"':
			inQuotes = true
		case ' ', '\t':
			fields = append(fields, line[start:i])
			start = -1
		}
	}
	if inQuotes || escaped {
		return nil, false
	}
	if start >= 0 {
		fields = append(fields, line[start:])
	}
	return fields, true
}

func recognizedChildLogLevel(level string) string {
	level = strings.ToUpper(level)
	switch level {
	case "DEBUG", "INFO", "WARN", "ERROR", "FATAL", "PANIC", "DPANIC":
		return level
	default:
		return ""
	}
}

// stripANSI removes ANSI SGR escape sequences (e.g. the color codes the
// console encoder wraps the level token in) so the bare token can be matched.
func stripANSI(s string) string {
	for {
		start := strings.IndexByte(s, 0x1b)
		if start < 0 {
			return s
		}
		end := strings.IndexByte(s[start:], 'm')
		if end < 0 {
			// Unterminated escape: leave the raw ESC byte in place rather than
			// dropping the tail, so a truncated token (e.g. "INFO\x1b[34") does
			// not collapse into a valid level and instead falls back to WARN.
			return s
		}
		s = s[:start] + s[start+end+1:]
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
	if !stopping && l.onUnexpectedExit != nil {
		l.onUnexpectedExit()
	}

	close(l.exited)
}
