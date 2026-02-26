// Package process - VscodeManager manages a code-server subprocess for VS Code web access.
package process

import (
	"bufio"
	"context"
	"encoding/json"
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
	resolvedPath string // resolved code-server binary path (set after startup)
	status       VscodeStatus
	err          string
	message      string
	mu           sync.Mutex
	cancelStart  context.CancelFunc // cancels startAsync goroutine
	stopCh       chan struct{}
	doneCh       chan struct{}
	stopped      bool // guards stopCh against double-close
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

	go v.startAsync(ctx)
}

// startAsync runs the full startup sequence in a background goroutine.
func (v *VscodeManager) startAsync(ctx context.Context) {
	// Write theme settings before starting
	v.writeThemeSettings()

	// Resolve the binary, auto-installing if needed (the slow part)
	v.setMessage("Installing code-server (this may take a moment)...")
	resolvedPath, err := tools.ResolveBinary(ctx, v.command, nil, v.installStrategy, v.logger)
	if err != nil {
		v.setError(fmt.Sprintf("code-server binary not available: %s", err))
		return
	}
	v.mu.Lock()
	v.resolvedPath = resolvedPath
	v.mu.Unlock()

	// Allocate a random port via the OS to avoid collisions
	// with other kandev instances or concurrent sessions.
	port, err := allocatePort()
	if err != nil {
		v.setError(fmt.Sprintf("failed to allocate port: %s", err))
		return
	}
	v.mu.Lock()
	v.port = port
	v.mu.Unlock()

	v.setStatus(VscodeStatusStarting)
	v.setMessage("Starting code-server...")

	if err := v.startProcess(ctx, resolvedPath); err != nil {
		v.setError(fmt.Sprintf("code-server failed to start: %s", err))
		return
	}

	v.setStatus(VscodeStatusRunning)
	v.setMessage("")
	v.logger.Info("code-server started",
		zap.Int("port", v.port),
		zap.Int("pid", v.cmd.Process.Pid))
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
func (v *VscodeManager) startProcess(ctx context.Context, binaryPath string) error {
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
	v.cmd = exec.Command(binaryPath, args...)
	v.cmd.Dir = workDir
	v.workDir = workDir
	// Give code-server its own process group so Stop() can kill the entire
	// process tree (main process + Node.js workers) without affecting agentctl.
	setProcGroup(v.cmd)

	stderr, err := v.cmd.StderrPipe()
	if err != nil {
		v.mu.Unlock()
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := v.cmd.Start(); err != nil {
		v.mu.Unlock()
		return fmt.Errorf("failed to start code-server: %w", err)
	}

	v.stopCh = make(chan struct{})
	v.doneCh = make(chan struct{})
	v.mu.Unlock()

	// Read stderr in background for logging
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			v.logger.Debug("code-server", zap.String("line", scanner.Text()))
		}
	}()

	// Wait for process exit in background
	pid := v.cmd.Process.Pid
	go func() {
		defer close(v.doneCh)
		waitErr := v.cmd.Wait()

		v.mu.Lock()
		defer v.mu.Unlock()

		if waitErr != nil && v.status != VscodeStatusStopped {
			v.status = VscodeStatusError
			v.err = waitErr.Error()
			v.logger.Error("code-server exited with error",
				zap.Int("pid", pid), zap.Error(waitErr))
		} else {
			v.status = VscodeStatusStopped
			v.logger.Info("code-server exited", zap.Int("pid", pid))
		}
	}()

	// Wait for code-server to become ready
	if err := v.waitForReady(ctx); err != nil {
		_ = v.Stop(ctx)
		return fmt.Errorf("code-server failed to become ready: %w", err)
	}

	return nil
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

// Stop stops the code-server process.
func (v *VscodeManager) Stop(ctx context.Context) error {
	v.mu.Lock()
	if v.status == VscodeStatusStopped {
		v.mu.Unlock()
		return nil
	}

	v.logger.Info("stopping code-server")
	v.status = VscodeStatusStopped

	// Cancel any in-progress startAsync goroutine (e.g. slow binary download).
	if v.cancelStart != nil {
		v.cancelStart()
		v.cancelStart = nil
	}

	if v.stopCh != nil && !v.stopped {
		close(v.stopCh)
		v.stopped = true
	}

	cmd := v.cmd
	doneCh := v.doneCh
	v.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		pid := cmd.Process.Pid
		v.logger.Info("stopping code-server process group",
			zap.Int("pid", pid), zap.Int("pgid", pid))
		logChildProcesses(v.logger, pid)

		// Phase 1: Graceful SIGTERM to the entire process group.
		if err := terminateProcessGroup(pid); err != nil {
			v.logger.Warn("failed to send SIGTERM to code-server group",
				zap.Int("pgid", pid), zap.Error(err))
		}

		// Phase 2: Wait for graceful exit, then escalate to SIGKILL.
		if doneCh != nil {
			select {
			case <-doneCh:
				v.logger.Info("code-server stopped gracefully", zap.Int("pid", pid))
				return nil
			case <-ctx.Done():
				v.logger.Warn("context cancelled during SIGTERM wait, force killing",
					zap.Int("pgid", pid))
			case <-time.After(5 * time.Second):
				v.logger.Warn("code-server did not exit after SIGTERM, force killing",
					zap.Int("pgid", pid))
			}

			if err := killProcessGroup(pid); err != nil {
				v.logger.Warn("failed to force kill code-server group",
					zap.Int("pgid", pid), zap.Error(err))
			}
			select {
			case <-doneCh:
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
				v.logger.Error("code-server still alive after SIGKILL",
					zap.Int("pid", pid))
			}
		}
	} else if doneCh != nil {
		// No process but doneCh exists (startup cancelled before process started)
		select {
		case <-doneCh:
		case <-ctx.Done():
		case <-time.After(2 * time.Second):
		}
	}

	v.logger.Info("code-server stopped")
	return nil
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
	ipcSocket, err := findVscodeIPCSocket()
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

	return socks[0].path, nil
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
