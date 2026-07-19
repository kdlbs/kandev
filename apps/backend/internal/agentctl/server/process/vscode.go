// Package process - VscodeManager manages a code-server subprocess for VS Code web access.
package process

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	tools "github.com/kandev/kandev/internal/tools/installer"
	"go.uber.org/zap"
)

// VscodeStatus represents the code-server process status.
type VscodeStatus string

const (
	VscodeStatusStopped    VscodeStatus = "stopped"
	VscodeStatusInstalling VscodeStatus = "installing"
	VscodeStatusStarting   VscodeStatus = "starting"
	VscodeStatusRunning    VscodeStatus = "running"
	VscodeStatusError      VscodeStatus = "error"
)

// VscodeInfo holds the current state of the VS Code server.
type VscodeInfo struct {
	Status  VscodeStatus `json:"status"`
	Port    int          `json:"port"`
	Error   string       `json:"error,omitempty"`
	Message string       `json:"message,omitempty"`
}

// VscodeManager manages a code-server subprocess.
type VscodeManager struct {
	command         string // code-server binary name or path
	workDir         string
	port            int
	theme           string         // "dark" or "light"
	installStrategy tools.Strategy // optional: auto-installs code-server if not found
	logger          *logger.Logger

	cmd          *exec.Cmd
	lifecycle    processLifecycleHandle
	reapErr      error
	resolvedPath string // resolved code-server binary path (set after startup)
	status       VscodeStatus
	err          string
	message      string
	mu           sync.Mutex
	cancelStart  context.CancelFunc // cancels startAsync goroutine
	startDone    chan struct{}      // closes when the current start generation exits
	stopCh       chan struct{}
	doneCh       chan struct{}
	stopped      bool // guards stopCh against double-close

	resolveBinaryFn    func(context.Context) (string, error)
	allocatePortFn     func() (int, error)
	beforeProcessStart func()
	afterStartCancel   func()
	groupAliveFn       func(int) bool
	terminateGroupFn   func(int) error
	killGroupFn        func(int) error
	waitGroupExitFn    func(context.Context, int) bool
}

// NewVscodeManager creates a new VS Code process manager.
// The port is allocated via the OS when Start is called.
func NewVscodeManager(
	command, workDir string,
	theme string,
	strategy tools.Strategy,
	log *logger.Logger,
) *VscodeManager {
	return &VscodeManager{
		command:         command,
		workDir:         workDir,
		theme:           theme,
		installStrategy: strategy,
		logger:          log.WithFields(zap.String("component", "vscode-manager")),
		status:          VscodeStatusStopped,
	}
}

// Start launches the code-server process asynchronously.
// It returns immediately after setting status to "installing" and spawns
// a background goroutine that resolves the binary, writes theme settings,
// and starts the process.
func (v *VscodeManager) Start() {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.status == VscodeStatusRunning || v.status == VscodeStatusStarting || v.status == VscodeStatusInstalling {
		return
	}

	v.setStatusLocked(VscodeStatusInstalling)
	v.setMessageLocked("Preparing code-server...")
	v.err = ""
	v.stopped = false

	ctx, cancel := context.WithCancel(context.Background())
	v.cancelStart = cancel
	startDone := make(chan struct{})
	v.startDone = startDone

	go v.startAsync(ctx, startDone)
}

// startAsync runs the full startup sequence in a background goroutine.
func (v *VscodeManager) startAsync(ctx context.Context, done chan struct{}) {
	defer close(done)
	// Write theme settings before starting
	v.writeThemeSettings()

	// Resolve the binary, auto-installing if needed (the slow part)
	if !v.commitStartState(ctx, done, func() {
		v.message = "Installing code-server (this may take a moment)..."
	}) {
		return
	}
	resolvedPath, err := v.resolveBinary(ctx)
	if err != nil {
		v.setStartError(ctx, done, fmt.Sprintf("code-server binary not available: %s", err))
		return
	}
	if !v.commitStartState(ctx, done, func() { v.resolvedPath = resolvedPath }) {
		return
	}

	// Allocate a random port via the OS to avoid collisions
	// with other kandev instances or concurrent sessions.
	port, err := v.allocateStartupPort()
	if err != nil {
		v.setStartError(ctx, done, fmt.Sprintf("failed to allocate port: %s", err))
		return
	}
	if !v.commitStartState(ctx, done, func() {
		v.port = port
		v.status = VscodeStatusStarting
		v.message = "Starting code-server..."
	}) {
		return
	}
	if v.beforeProcessStart != nil {
		v.beforeProcessStart()
	}
	if ctx.Err() != nil {
		return
	}

	if err := v.startProcess(ctx, done, resolvedPath); err != nil {
		v.setStartError(ctx, done, fmt.Sprintf("code-server failed to start: %s", err))
		return
	}
	if !v.commitStartState(ctx, done, func() {
		v.status = VscodeStatusRunning
		v.message = ""
	}) {
		return
	}
	v.mu.Lock()
	pid := 0
	startedPort := 0
	if v.startDone == done && v.cmd != nil && v.cmd.Process != nil {
		pid = v.cmd.Process.Pid
		startedPort = v.port
	}
	v.mu.Unlock()
	v.logger.Info("code-server started",
		zap.Int("port", startedPort),
		zap.Int("pid", pid))
}

func (v *VscodeManager) resolveBinary(ctx context.Context) (string, error) {
	if v.resolveBinaryFn != nil {
		return v.resolveBinaryFn(ctx)
	}
	return tools.ResolveBinary(ctx, v.command, nil, v.installStrategy, v.logger)
}

func (v *VscodeManager) allocateStartupPort() (int, error) {
	if v.allocatePortFn != nil {
		return v.allocatePortFn()
	}
	return allocatePort()
}

func (v *VscodeManager) commitStartState(ctx context.Context, done chan struct{}, commit func()) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if ctx.Err() != nil || v.startDone != done || v.status == VscodeStatusStopped {
		return false
	}
	commit()
	return true
}

func (v *VscodeManager) setStartError(ctx context.Context, done chan struct{}, message string) {
	v.commitStartState(ctx, done, func() {
		v.status = VscodeStatusError
		v.err = message
		v.message = ""
	})
}

// allocatePort finds a free port using the OS.
func allocatePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("failed to allocate port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port, nil
}

// startProcess creates and starts the code-server subprocess.
func (v *VscodeManager) startProcess(ctx context.Context, generationDone chan struct{}, binaryPath string) error {
	workDir := resolveExistingWorkDir(v.workDir, v.logger)
	bindAddr := fmt.Sprintf("0.0.0.0:%d", v.port)
	args := []string{
		"--bind-addr", bindAddr,
		"--auth", "none",
		"--disable-telemetry",
		"--disable-update-check",
		"--user-data-dir", v.userDataDir(),
	}
	// Append workDir as positional argument so code-server opens the folder.
	args = append(args, workDir)

	v.mu.Lock()
	if ctx.Err() != nil || v.startDone != generationDone || v.status == VscodeStatusStopped {
		v.mu.Unlock()
		return context.Canceled
	}
	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = workDir
	v.workDir = workDir
	// Give code-server its own process group so Stop() can kill the entire
	// process tree (main process + Node.js workers) without affecting agentctl.
	setManagedProcGroup(cmd)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		v.mu.Unlock()
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		v.mu.Unlock()
		return fmt.Errorf("failed to start code-server: %w", err)
	}
	lifecycle, lifecycleErr := installProcessLifecycle(cmd)
	if lifecycleErr != nil {
		reapErr := killAndWaitStartedCommand(cmd)
		v.mu.Unlock()
		return errors.Join(fmt.Errorf("failed to install code-server process lifecycle: %w", lifecycleErr), reapErr)
	}

	doneCh := make(chan struct{})
	v.cmd = cmd
	v.lifecycle = lifecycle
	v.reapErr = nil
	v.stopCh = make(chan struct{})
	v.doneCh = doneCh
	v.mu.Unlock()

	go v.readProcessStderr(stderr)
	go v.monitorProcess(cmd, doneCh)

	// Wait for code-server to become ready
	if err := v.waitForReady(ctx); err != nil {
		v.stopProcessAfterStartupFailure()
		return fmt.Errorf("code-server failed to become ready: %w", err)
	}

	return nil
}

func (v *VscodeManager) stopProcessAfterStartupFailure() {
	v.mu.Lock()
	cmd := v.cmd
	doneCh := v.doneCh
	v.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = killProcessGroup(cmd.Process.Pid)
	if doneCh != nil {
		select {
		case <-doneCh:
		case <-time.After(2 * time.Second):
		}
	}
}

func resolveExistingWorkDir(workDir string, log *logger.Logger) string {
	candidate := strings.TrimSpace(workDir)
	if candidate == "" {
		if cwd, err := os.Getwd(); err == nil && cwd != "" {
			return cwd
		}
		return "."
	}

	info, err := os.Stat(candidate)
	if err == nil && info.IsDir() {
		return candidate
	}

	// Walk up to the nearest existing parent directory.
	current := candidate
	for {
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		info, statErr := os.Stat(parent)
		if statErr == nil && info.IsDir() {
			if log != nil {
				log.Warn("vscode workdir missing; using nearest existing parent directory",
					zap.String("requested_workdir", candidate),
					zap.String("fallback_workdir", parent))
			}
			return parent
		}
		current = parent
	}

	if cwd, cwdErr := os.Getwd(); cwdErr == nil && cwd != "" {
		if log != nil {
			log.Warn("vscode workdir missing; using current directory fallback",
				zap.String("requested_workdir", candidate),
				zap.String("fallback_workdir", cwd))
		}
		return cwd
	}
	if log != nil {
		log.Warn("vscode workdir missing; using relative dot fallback",
			zap.String("requested_workdir", candidate))
	}
	return "."
}

// waitForReady polls the code-server port until it accepts connections.
func (v *VscodeManager) waitForReady(ctx context.Context) error {
	addr := fmt.Sprintf("127.0.0.1:%d", v.port)
	deadline := time.After(30 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timeout waiting for code-server on port %d", v.port)
		case <-v.doneCh:
			return fmt.Errorf("code-server exited before becoming ready")
		default:
			conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				return nil
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// WaitForRunning blocks until the code-server reaches "running" status.
// Returns immediately if already running. Returns an error if the status
// transitions to "error" or "stopped", or if the context is cancelled.
func (v *VscodeManager) WaitForRunning(ctx context.Context) error {
	for {
		v.mu.Lock()
		s := v.status
		errMsg := v.err
		v.mu.Unlock()

		switch s {
		case VscodeStatusRunning:
			return nil
		case VscodeStatusError:
			return fmt.Errorf("code-server failed to start: %s", errMsg)
		case VscodeStatusStopped:
			return fmt.Errorf("code-server is stopped")
		case VscodeStatusInstalling, VscodeStatusStarting:
			// still booting — wait and retry
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// Info returns the current VS Code server state.
func (v *VscodeManager) Info() VscodeInfo {
	v.mu.Lock()
	defer v.mu.Unlock()
	return VscodeInfo{
		Status:  v.status,
		Port:    v.port,
		Error:   v.err,
		Message: v.message,
	}
}

// Port returns the port code-server is bound to.
func (v *VscodeManager) Port() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.port
}

// userDataDir returns the path for code-server user data.
func (v *VscodeManager) userDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".kandev", "tools", "code-server-data")
}

// writeThemeSettings ensures VS Code settings.json contains our managed settings.
// Merges with any existing user settings, overwriting only the keys we manage.
func (v *VscodeManager) writeThemeSettings() {
	settingsDir := filepath.Join(v.userDataDir(), "User")
	settingsPath := filepath.Join(settingsDir, "settings.json")

	themeName := "Default Dark Modern"
	if v.theme == "light" {
		themeName = "Default Light Modern"
	}

	managed := map[string]any{
		"workbench.colorTheme":                             themeName,
		"security.workspace.trust.enabled":                 false,
		"workbench.startupEditor":                          "none",
		"workbench.enableExperiments":                      false,
		"workbench.accounts.experimental.showEntitlements": false,
		"settingsSync.enabled":                             false,
		"telemetry.enabled":                                false,
		"workbench.sideBar.location":                       "right",
		"window.commandCenter":                             false,
		"workbench.layoutControl.enabled":                  false,
		"editor.minimap.autohide":                          true,
	}

	// Read existing settings and merge our managed keys on top.
	settings := make(map[string]any)
	if existing, err := os.ReadFile(settingsPath); err == nil {
		_ = json.Unmarshal(existing, &settings)
	}
	for k, v := range managed {
		settings[k] = v
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		v.logger.Warn("failed to marshal theme settings", zap.Error(err))
		return
	}

	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		v.logger.Warn("failed to create settings dir", zap.Error(err))
		return
	}

	if err := os.WriteFile(settingsPath, data, 0o644); err != nil {
		v.logger.Warn("failed to write theme settings", zap.Error(err))
	}
}

// OpenFile opens a file in the running VS Code instance via the Remote CLI.
// It uses the code-server bundled Remote CLI script and the VSCODE_IPC_HOOK_CLI
// environment variable to communicate with the running VS Code process.
func (v *VscodeManager) OpenFile(ctx context.Context, path string, line, col int) error {
	v.mu.Lock()
	if v.status != VscodeStatusRunning {
		v.mu.Unlock()
		return fmt.Errorf("code-server is not running")
	}
	resolvedPath := v.resolvedPath
	workDir := v.workDir
	v.mu.Unlock()

	if resolvedPath == "" {
		return fmt.Errorf("code-server binary path not resolved")
	}

	// Resolve the Remote CLI script relative to the code-server binary.
	remoteCLI := resolveRemoteCLI(resolvedPath)
	if _, err := os.Stat(remoteCLI); err != nil {
		return fmt.Errorf("remote CLI not found at %s: %w", remoteCLI, err)
	}

	// Find the IPC socket for the running VS Code instance.
	// The IPC socket may not appear immediately after code-server's HTTP port
	// is ready, so poll for it with a timeout.
	ipcSocket, err := waitForVscodeIPCSocket(ctx, 15*time.Second)
	if err != nil {
		return fmt.Errorf("failed to find VS Code IPC socket: %w", err)
	}

	// Build absolute path if relative.
	absPath := path
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(workDir, absPath)
	}

	// Build the --goto argument: <path>:<line>:<col>
	gotoArg := absPath
	if line > 0 {
		gotoArg = fmt.Sprintf("%s:%d", absPath, line)
		if col > 0 {
			gotoArg = fmt.Sprintf("%s:%d", gotoArg, col)
		}
	}

	cmd := exec.CommandContext(ctx, remoteCLI, "--goto", gotoArg)
	cmd.Env = append(os.Environ(), "VSCODE_IPC_HOOK_CLI="+ipcSocket)
	cmd.Dir = workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("remote CLI failed: %w (output: %s)", err, strings.TrimSpace(string(output)))
	}

	v.logger.Info("opened file in VS Code",
		zap.String("path", absPath),
		zap.Int("line", line),
		zap.Int("col", col))
	return nil
}

// resolveRemoteCLI returns the path to the VS Code Remote CLI script
// relative to the resolved code-server binary.
// Layout: <binary-dir>/../lib/vscode/bin/remote-cli/code-{platform}.sh
func resolveRemoteCLI(binaryPath string) string {
	binDir := filepath.Dir(binaryPath) // .../bin/
	baseDir := filepath.Dir(binDir)    // .../code-server-x.y.z-os-arch/
	platform := runtime.GOOS           // "darwin" or "linux"
	return filepath.Join(baseDir, "lib", "vscode", "bin", "remote-cli", "code-"+platform+".sh")
}

// findVscodeIPCSocket searches /tmp for the most recent vscode-ipc-*.sock file.
// It validates each candidate by attempting a Unix socket connection to skip
// stale sockets left behind by crashed VS Code instances.
func findVscodeIPCSocket() (string, error) {
	tmpDir := os.TempDir()
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return "", fmt.Errorf("failed to read temp dir: %w", err)
	}

	type sockEntry struct {
		path    string
		modTime time.Time
	}
	var socks []sockEntry

	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "vscode-ipc-") || !strings.HasSuffix(e.Name(), ".sock") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		socks = append(socks, sockEntry{
			path:    filepath.Join(tmpDir, e.Name()),
			modTime: info.ModTime(),
		})
	}

	if len(socks) == 0 {
		return "", fmt.Errorf("no vscode-ipc-*.sock found in %s", tmpDir)
	}

	// Sort by modification time descending (most recent first).
	sort.Slice(socks, func(i, j int) bool {
		return socks[i].modTime.After(socks[j].modTime)
	})

	// Try each socket, most recent first, and return the first one that accepts
	// a connection. This skips stale sockets from crashed processes.
	for _, s := range socks {
		conn, dialErr := net.DialTimeout("unix", s.path, 500*time.Millisecond)
		if dialErr != nil {
			continue
		}
		_ = conn.Close()
		return s.path, nil
	}

	return "", fmt.Errorf("no live vscode-ipc-*.sock found in %s (%d stale sockets skipped)", tmpDir, len(socks))
}

// waitForVscodeIPCSocket polls for a live VS Code IPC socket until one is found
// or the timeout/context expires. Code-server may take several seconds after its
// HTTP port is ready before creating the IPC socket.
func waitForVscodeIPCSocket(ctx context.Context, timeout time.Duration) (string, error) {
	deadline := time.After(timeout)
	var lastErr error

	for {
		sock, err := findVscodeIPCSocket()
		if err == nil {
			return sock, nil
		}
		lastErr = err

		select {
		case <-ctx.Done():
			return "", fmt.Errorf("%w (last attempt: %v)", ctx.Err(), lastErr)
		case <-deadline:
			return "", lastErr
		case <-time.After(1 * time.Second):
		}
	}
}

// setStatus updates the status under lock.
func (v *VscodeManager) setStatus(s VscodeStatus) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.status = s
}

// setStatusLocked updates the status (caller must hold v.mu).
func (v *VscodeManager) setStatusLocked(s VscodeStatus) {
	v.status = s
}

// setMessage updates the message under lock.
func (v *VscodeManager) setMessage(msg string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.message = msg
}

// setMessageLocked updates the message (caller must hold v.mu).
func (v *VscodeManager) setMessageLocked(msg string) {
	v.message = msg
}

// setError sets error status with a message, under lock.
func (v *VscodeManager) setError(errMsg string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.status = VscodeStatusError
	v.err = errMsg
	v.message = ""
}
