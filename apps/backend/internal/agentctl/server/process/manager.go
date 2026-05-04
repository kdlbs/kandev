// Package process manages the agent subprocess lifecycle
package process

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/shell"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	tools "github.com/kandev/kandev/internal/tools/installer"
	"go.uber.org/zap"
)

// Status represents the agent process status
type Status string

const (
	StatusStopped  Status = "stopped"
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusPaused   Status = "paused"
	StatusStopping Status = "stopping"
	StatusError    Status = "error"
)

// errorWrapper wraps an error so it can be stored in atomic.Value (which cannot store nil)
type errorWrapper struct {
	err error
}

// PendingPermission represents a permission request waiting for user response
type PendingPermission struct {
	ID         string
	Request    *adapter.PermissionRequest
	ResponseCh chan *adapter.PermissionResponse
	CreatedAt  time.Time
}

// PermissionNotification is sent when the agent requests permission
type PermissionNotification struct {
	PendingID     string                     `json:"pending_id"`
	SessionID     string                     `json:"session_id"`
	ToolCallID    string                     `json:"tool_call_id"`
	Title         string                     `json:"title"`
	Options       []adapter.PermissionOption `json:"options"`
	ActionType    string                     `json:"action_type,omitempty"`
	ActionDetails map[string]interface{}     `json:"action_details,omitempty"`
	CreatedAt     time.Time                  `json:"created_at"`
}

// defaultStderrBufferSize is the number of recent stderr lines to keep for error context
const defaultStderrBufferSize = 50

const agentTempDirRoot = "kandev-agent"

// Manager manages the agent subprocess
type Manager struct {
	cfg    *config.InstanceConfig
	logger *logger.Logger

	// Process state
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   io.ReadCloser
	stderr   io.ReadCloser
	status   atomic.Value // Status
	exitCode atomic.Int32
	exitErr  atomic.Value // error

	// Stderr buffering for error context
	stderrBuffer []string
	stderrMu     sync.RWMutex

	// Workspace tracker for git status and file changes
	workspaceTracker *WorkspaceTracker
	// repoTrackers holds per-repository trackers for multi-repo task roots.
	// Each tracker stamps RepositoryName onto its emitted events and shares
	// subscriber channels with the root via the Manager fan-out.
	// Empty for single-repo workspaces.
	repoTrackers []*WorkspaceTracker
	// workspaceTrackersBySubpath caches per-subpath trackers for multi-repo
	// task roots. Key is the cleaned subpath (relative to cfg.WorkDir). The
	// root tracker lives in workspaceTracker above.
	workspaceTrackersBySubpath map[string]*WorkspaceTracker
	workspaceTrackersMu        sync.Mutex

	// Script/process runner (dev server, setup, cleanup, custom)
	processRunner *ProcessRunner

	// Embedded shell session (auto-created when agent starts)
	shell *shell.Session

	// Per-terminal shell sessions for remote executor support
	shellMgr *shell.Manager

	// Protocol adapter for agent communication
	adapter    adapter.AgentAdapter
	adapterCfg *adapter.Config

	// Agent event notifications (protocol-agnostic)
	updatesCh chan adapter.AgentEvent

	// Pending permission requests waiting for user response
	pendingPermissions map[string]*PendingPermission
	permissionMu       sync.RWMutex

	// VS Code server manager (lazy-initialized on demand)
	vscode   *VscodeManager
	vscodeMu sync.Mutex

	// Git operator for git operations (lazy-initialized)
	gitOperator   *GitOperator
	gitOperatorMu sync.Mutex
	// gitOperatorsBySubpath caches per-subpath operators for multi-repo task
	// roots. Key is the cleaned subpath (relative to cfg.WorkDir); empty key
	// is reserved for the root operator and lives in gitOperator above.
	gitOperatorsBySubpath map[string]*GitOperator

	// Final command string (full command with all adapter args)
	finalCommand string

	// Synchronization
	mu      sync.RWMutex
	wg      sync.WaitGroup
	stopCh  chan struct{}
	doneCh  chan struct{}
	startMu sync.Mutex
}

// NewManager creates a new process manager
func NewManager(cfg *config.InstanceConfig, log *logger.Logger) *Manager {
	cfg.WorkDir = resolveExistingWorkDir(cfg.WorkDir, log.WithFields(zap.String("component", "process-manager")))
	m := &Manager{
		cfg:                cfg,
		logger:             log.WithFields(zap.String("component", "process-manager")),
		updatesCh:          make(chan adapter.AgentEvent, 100),
		pendingPermissions: make(map[string]*PendingPermission),
	}
	// Multi-repo task roots hold one git worktree per repository as siblings.
	// In that case build a per-repo tracker for each child so each emits its
	// own GitStatusUpdate (tagged with RepositoryName) and the changes panel
	// can show all repos. The root tracker covers the single-repo case via
	// preferGitRepoChildIfRootIsBare; we skip its fallback when we've already
	// detected a multi-repo root to avoid double-tracking the first repo.
	repoChildren := scanRepositorySubdirs(cfg.WorkDir)
	if len(repoChildren) >= 2 {
		// Multi-repo: root tracker bound to the bare task root (no fallback,
		// no events), plus one tracker per repo subdir.
		m.workspaceTracker = NewWorkspaceTrackerForRepo(cfg.WorkDir, "", log)
		for _, child := range repoChildren {
			m.repoTrackers = append(m.repoTrackers,
				NewWorkspaceTrackerForRepo(child.path, child.name, log))
		}
	} else {
		m.workspaceTracker = NewWorkspaceTracker(cfg.WorkDir, log)
	}
	m.processRunner = NewProcessRunner(m.workspaceTracker, log, cfg.ProcessBufferMaxBytes)
	m.shellMgr = shell.NewManager(cfg.WorkDir, log)
	m.status.Store(StatusStopped)
	m.exitCode.Store(-1)
	return m
}

// repositorySubdir is one git-repo child of a multi-repo task root.
type repositorySubdir struct {
	name string // directory basename (used as RepositoryName on emitted events)
	path string // absolute path to the repo subdir
}

// scanRepositorySubdirs returns the immediate child directories of workDir
// that are themselves git repositories or worktrees. Returns an empty slice
// when workDir doesn't exist, isn't readable, or contains zero git children.
// Used to detect multi-repo task roots at Manager construction.
func scanRepositorySubdirs(workDir string) []repositorySubdir {
	if workDir == "" {
		return nil
	}
	entries, err := os.ReadDir(workDir)
	if err != nil {
		return nil
	}
	var out []repositorySubdir
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		candidate := filepath.Join(workDir, entry.Name())
		if resolveGitIndexPath(candidate) == "" {
			continue
		}
		out = append(out, repositorySubdir{name: entry.Name(), path: candidate})
	}
	return out
}

// Status returns the current process status
func (m *Manager) Status() Status {
	return m.status.Load().(Status)
}

// ExitCode returns the exit code (-1 if not exited)
func (m *Manager) ExitCode() int {
	return int(m.exitCode.Load())
}

// ExitError returns the exit error if any
func (m *Manager) ExitError() error {
	if v := m.exitErr.Load(); v != nil {
		if w, ok := v.(errorWrapper); ok {
			return w.err
		}
	}
	return nil
}

// GetWorkspaceTracker returns the workspace tracker for git status and file monitoring
func (m *Manager) GetWorkspaceTracker() *WorkspaceTracker {
	return m.workspaceTracker
}

// SubscribeWorkspaceStream creates a single workspace stream subscriber and
// fans it out across the root tracker plus every per-repo tracker, so the
// caller receives events from all repositories on one channel. Use
// UnsubscribeWorkspaceStream to detach and close.
//
// For single-repo workspaces this is equivalent to
// GetWorkspaceTracker().SubscribeWorkspaceStream() — the per-repo list is
// empty and only the root tracker fires events.
func (m *Manager) SubscribeWorkspaceStream() types.WorkspaceStreamSubscriber {
	sub := make(types.WorkspaceStreamSubscriber, 100)
	m.workspaceTracker.AttachWorkspaceStreamSubscriber(sub)
	for _, t := range m.repoTrackers {
		t.AttachWorkspaceStreamSubscriber(sub)
	}
	return sub
}

// UnsubscribeWorkspaceStream detaches the subscriber from every tracker and
// closes the channel exactly once.
func (m *Manager) UnsubscribeWorkspaceStream(sub types.WorkspaceStreamSubscriber) {
	m.workspaceTracker.DetachWorkspaceStreamSubscriber(sub)
	for _, t := range m.repoTrackers {
		t.DetachWorkspaceStreamSubscriber(sub)
	}
	close(sub)
}

// GetWorkspaceTrackerFor returns a workspace tracker scoped to a sub-directory
// of the workspace. Used by multi-repo task roots where each repository lives
// at {WorkDir}/{subpath}. Empty subpath returns the root tracker.
//
// Per-subpath trackers are created lazily on first request and cached. They
// are NOT started (no polling goroutines) — the multi-repo path uses them
// only for synchronous git-status queries via GetCurrentGitStatus, which
// falls through to getGitStatus when no cache is present. This keeps the
// per-repo cost cheap while preserving the long-running polling behavior of
// the root tracker for the agent's primary workspace.
func (m *Manager) GetWorkspaceTrackerFor(subpath string) (*WorkspaceTracker, error) {
	cleaned, full, err := m.resolveSubpath(subpath)
	if err != nil {
		return nil, err
	}
	if cleaned == "" {
		return m.workspaceTracker, nil
	}

	m.workspaceTrackersMu.Lock()
	defer m.workspaceTrackersMu.Unlock()
	if m.workspaceTrackersBySubpath == nil {
		m.workspaceTrackersBySubpath = make(map[string]*WorkspaceTracker)
	}
	if t, ok := m.workspaceTrackersBySubpath[cleaned]; ok {
		return t, nil
	}
	t := NewWorkspaceTracker(full, m.logger)
	m.workspaceTrackersBySubpath[cleaned] = t
	return t, nil
}

// StartProcess runs a script/process with isolated stdout/stderr.
func (m *Manager) StartProcess(ctx context.Context, req StartProcessRequest) (*ProcessInfo, error) {
	if m.processRunner == nil {
		return nil, fmt.Errorf("process runner not available")
	}
	return m.processRunner.Start(ctx, req)
}

// StopProcess stops a running process by ID.
func (m *Manager) StopProcess(ctx context.Context, req StopProcessRequest) error {
	if m.processRunner == nil {
		return fmt.Errorf("process runner not available")
	}
	return m.processRunner.Stop(ctx, req)
}

// GetProcess returns a process by ID.
func (m *Manager) GetProcess(id string, includeOutput bool) (*ProcessInfo, bool) {
	if m.processRunner == nil {
		return nil, false
	}
	return m.processRunner.Get(id, includeOutput)
}

// ListProcesses returns processes for a session (or all if sessionID empty).
func (m *Manager) ListProcesses(sessionID string) []ProcessInfo {
	if m.processRunner == nil {
		return nil
	}
	return m.processRunner.List(sessionID)
}

// RepoSubpaths returns the subpath name (relative to cfg.WorkDir) for every
// per-repo tracker discovered at construction time. Empty for single-repo
// workspaces. Used by callers that want to fan an op out across repos.
func (m *Manager) RepoSubpaths() []string {
	out := make([]string, 0, len(m.repoTrackers))
	for _, t := range m.repoTrackers {
		if t.repositoryName != "" {
			out = append(out, t.repositoryName)
		}
	}
	return out
}

// SetWorkspacePollMode propagates a poll-mode change to the root tracker and
// every per-repo tracker, then forces a RefreshGitStatus on each non-paused
// tracker so a fresh snapshot reaches every subscriber. Without the refresh,
// monitorTick only pushes on detected change — multi-repo workspaces would
// leave the per-repo state map sparse (one repo present, the other missing)
// after a focus event, since the agent's initial pushes happen at boot and
// no replay path exists for clients that subscribe later.
func (m *Manager) SetWorkspacePollMode(ctx context.Context, mode PollMode) {
	m.workspaceTracker.SetPollMode(mode)
	for _, t := range m.repoTrackers {
		t.SetPollMode(mode)
	}
	if mode == PollModePaused {
		return
	}
	// Snapshot the tracker slice before launching the goroutine so a
	// concurrent Stop()/teardown that mutates m.repoTrackers can't race or
	// nil-deref the iteration. Refresh in background — RefreshGitStatus
	// blocks on git commands which can take seconds on large repos; the
	// HTTP caller shouldn't wait.
	root := m.workspaceTracker
	trackers := make([]*WorkspaceTracker, len(m.repoTrackers))
	copy(trackers, m.repoTrackers)
	go func() {
		root.RefreshGitStatus(ctx)
		for _, t := range trackers {
			t.RefreshGitStatus(ctx)
		}
	}()
}

// GitOperator returns the git operator for git operations against the
// workspace root. Lazy-initialized.
func (m *Manager) GitOperator() *GitOperator {
	m.gitOperatorMu.Lock()
	defer m.gitOperatorMu.Unlock()

	if m.gitOperator == nil {
		m.gitOperator = NewGitOperator(m.cfg.WorkDir, m.logger, m.workspaceTracker)
	}
	return m.gitOperator
}

// GitOperatorFor returns a git operator scoped to a sub-directory of the
// workspace. Used by multi-repo task roots where each repository lives at
// {WorkDir}/{subpath}. Empty subpath returns the root operator.
//
// The subpath is validated to prevent path-traversal: it must be a clean
// relative path with no parent-references and must resolve to an existing
// directory inside cfg.WorkDir.
func (m *Manager) GitOperatorFor(subpath string) (*GitOperator, error) {
	cleaned, full, err := m.resolveSubpath(subpath)
	if err != nil {
		return nil, err
	}
	if cleaned == "" {
		return m.GitOperator(), nil
	}

	m.gitOperatorMu.Lock()
	defer m.gitOperatorMu.Unlock()
	if m.gitOperatorsBySubpath == nil {
		m.gitOperatorsBySubpath = make(map[string]*GitOperator)
	}
	if op, ok := m.gitOperatorsBySubpath[cleaned]; ok {
		return op, nil
	}
	op := NewGitOperatorForRepo(full, cleaned, m.logger, m.workspaceTracker)
	m.gitOperatorsBySubpath[cleaned] = op
	return op, nil
}

// resolveSubpath normalises and validates a repo subpath relative to
// cfg.WorkDir. Returns ("", "", nil) for the root (empty/"."); otherwise
// returns the cleaned relative path and the absolute full path.
//
// Rejects: parent-references, absolute paths, paths containing "..",
// and paths that don't resolve to an existing directory.
func (m *Manager) resolveSubpath(subpath string) (string, string, error) {
	subpath = strings.TrimSpace(subpath)
	if subpath == "" || subpath == "." {
		return "", m.cfg.WorkDir, nil
	}

	cleaned := filepath.Clean(subpath)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "..") {
		return "", "", fmt.Errorf("invalid repo subpath: %q", subpath)
	}
	if filepath.IsAbs(cleaned) {
		return "", "", fmt.Errorf("repo subpath must be relative: %q", subpath)
	}
	for _, part := range strings.Split(cleaned, string(filepath.Separator)) {
		if part == ".." {
			return "", "", fmt.Errorf("repo subpath escapes workspace: %q", subpath)
		}
	}

	full := filepath.Join(m.cfg.WorkDir, cleaned)
	info, err := os.Stat(full)
	if err != nil {
		return "", "", fmt.Errorf("repo subpath not found: %w", err)
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("repo subpath is not a directory: %q", subpath)
	}
	return cleaned, full, nil
}

// JoinRepoPath validates a repo subpath and returns the workspace-relative
// path obtained by joining `subpath` and `path`. Empty `subpath` returns
// `path` unchanged (single-repo workspaces). Used by file content / update
// handlers to scope a per-repo path under the right repository directory
// before delegating to the workspace tracker.
func (m *Manager) JoinRepoPath(subpath, path string) (string, error) {
	cleaned, _, err := m.resolveSubpath(subpath)
	if err != nil {
		return "", err
	}
	if cleaned == "" {
		return path, nil
	}
	return filepath.Join(cleaned, path), nil
}

// Start starts the agent process
func (m *Manager) Start(ctx context.Context) error {
	m.startMu.Lock()
	defer m.startMu.Unlock()

	if m.Status() == StatusRunning || m.Status() == StatusStarting {
		return fmt.Errorf("agent is already running")
	}

	m.logger.Info("starting agent process",
		zap.String("protocol", string(m.cfg.Protocol)),
		zap.Strings("args", m.cfg.AgentArgs),
		zap.String("workdir", m.cfg.WorkDir),
		zap.Int("mcp_servers", len(m.cfg.McpServers)))

	m.status.Store(StatusStarting)
	m.exitCode.Store(-1)
	m.exitErr.Store(errorWrapper{err: nil})

	if len(m.cfg.AgentArgs) == 0 {
		m.status.Store(StatusError)
		return fmt.Errorf("no agent command configured")
	}

	if err := m.ensureAgentTempEnv(); err != nil {
		m.status.Store(StatusError)
		return err
	}

	// Build adapter config and create protocol adapter
	if err := m.buildAdapterConfig(); err != nil {
		m.status.Store(StatusError)
		return err
	}

	// One-shot adapters manage their own subprocess per prompt.
	// Skip process creation — the adapter spawns processes in Prompt().
	if oneShotAdapter, ok := m.adapter.(adapter.OneShotAdapter); ok && oneShotAdapter.IsOneShot() {
		return m.startOneShot()
	}

	// Assemble final command (does not start the process yet)
	if err := m.buildFinalCommand(); err != nil {
		m.status.Store(StatusError)
		return err
	}

	// Set up stdin/stdout/stderr pipes (must happen before process starts)
	if err := m.startProcessPipes(); err != nil {
		m.status.Store(StatusError)
		return err
	}

	// Start the subprocess now that pipes are connected
	if err := m.cmd.Start(); err != nil {
		m.status.Store(StatusError)
		return fmt.Errorf("failed to start agent: %w", err)
	}

	m.stopCh = make(chan struct{})
	m.doneCh = make(chan struct{})

	// Connect adapter to the process stdin/stdout pipes
	if err := m.adapter.Connect(m.stdin, m.stdout); err != nil {
		m.status.Store(StatusError)
		return fmt.Errorf("failed to connect adapter: %w", err)
	}

	// Start stderr reader and exit waiter
	m.wg.Add(2)
	go m.readStderr()
	go m.waitForExit()

	// Forward adapter updates to our channel
	m.wg.Add(1)
	go m.forwardUpdates()

	// Start workspace tracker with background context (not tied to HTTP request)
	m.workspaceTracker.Start(context.Background())
	for _, t := range m.repoTrackers {
		t.Start(context.Background())
	}

	// Auto-create shell session if enabled
	m.startAgentShell()

	m.status.Store(StatusRunning)
	m.logger.Info("agent process started", zap.Int("pid", m.cmd.Process.Pid))

	return nil
}

// startOneShot initialises a one-shot adapter without spawning a long-lived subprocess.
// The adapter manages its own per-prompt subprocess lifecycle internally.
func (m *Manager) startOneShot() error {
	m.stopCh = make(chan struct{})
	m.doneCh = make(chan struct{})

	// Forward adapter updates to our channel
	m.wg.Add(1)
	go m.forwardUpdates()

	// Start workspace tracker with background context (not tied to HTTP request)
	m.workspaceTracker.Start(context.Background())
	for _, t := range m.repoTrackers {
		t.Start(context.Background())
	}

	// Auto-create shell session if enabled
	m.startAgentShell()

	m.status.Store(StatusRunning)
	m.logger.Info("one-shot adapter started (no persistent subprocess)")
	return nil
}

// buildAdapterConfig constructs the adapter configuration and initialises the
// protocol adapter, including merging any adapter-provided environment variables.
func (m *Manager) buildAdapterConfig() error {
	mcpServers := make([]adapter.McpServerConfig, len(m.cfg.McpServers))
	for i, mcp := range m.cfg.McpServers {
		mcpServers[i] = adapter.McpServerConfig{
			Name:    mcp.Name,
			URL:     mcp.URL,
			Type:    mcp.Type,
			Command: mcp.Command,
			Args:    mcp.Args,
			Env:     mcp.Env,
			Headers: mcp.Headers,
		}
	}
	m.adapterCfg = &adapter.Config{
		WorkDir:        m.cfg.WorkDir,
		AutoApprove:    m.cfg.AutoApprovePermissions,
		ApprovalPolicy: m.cfg.ApprovalPolicy,
		McpServers:     mcpServers,
		AgentID:        m.cfg.AgentType, // From registry (e.g., "auggie", "amp", "claude-code")
		AssumeMcpSse:   m.cfg.AssumeMcpSse,
	}

	// Configure one-shot mode when a continue command is provided.
	// One-shot adapters (e.g., Amp) spawn a new subprocess per prompt.
	if m.cfg.ContinueCommand != "" {
		m.adapterCfg.OneShotConfig = &adapter.OneShotConfig{
			InitialArgs:  m.cfg.AgentArgs,
			ContinueArgs: config.ParseCommand(m.cfg.ContinueCommand),
			Env:          m.cfg.AgentEnv,
			WorkDir:      m.cfg.WorkDir,
		}
	}

	for i, srv := range mcpServers {
		m.logger.Debug("MCP server config",
			zap.Int("index", i),
			zap.String("name", srv.Name),
			zap.String("url", srv.URL),
			zap.String("type", srv.Type),
			zap.String("command", srv.Command))
	}

	if err := m.createAdapter(); err != nil {
		return fmt.Errorf("failed to create adapter: %w", err)
	}

	// Merge adapter-provided environment variables into the subprocess environment
	adapterEnv, err := m.adapter.PrepareEnvironment()
	if err != nil {
		m.logger.Warn("failed to prepare protocol environment", zap.Error(err))
	}
	for k, v := range adapterEnv {
		m.cfg.AgentEnv = append(m.cfg.AgentEnv, fmt.Sprintf("%s=%s", k, v))
	}
	return nil
}

// buildFinalCommand assembles the full command args and creates the exec.Cmd.
// The process group is set so child processes can be killed together.
func (m *Manager) buildFinalCommand() error {
	if err := m.ensureAgentTempEnv(); err != nil {
		return err
	}

	extraArgs := m.adapter.PrepareCommandArgs()

	cmdArgs := make([]string, 0, len(m.cfg.AgentArgs)-1+len(extraArgs))
	cmdArgs = append(cmdArgs, m.cfg.AgentArgs[1:]...)
	cmdArgs = append(cmdArgs, extraArgs...)

	m.finalCommand = strings.Join(append([]string{m.cfg.AgentArgs[0]}, cmdArgs...), " ")

	m.logger.Debug("final agent command",
		zap.String("binary", m.cfg.AgentArgs[0]),
		zap.Strings("args", cmdArgs),
		zap.Int("extra_args_count", len(extraArgs)))

	// NOTE: We intentionally don't use exec.CommandContext here because we don't want
	// the HTTP request context to kill the agent process when the request completes.
	m.cmd = exec.Command(m.cfg.AgentArgs[0], cmdArgs...)
	m.cmd.Dir = m.cfg.WorkDir
	m.cmd.Env = m.cfg.AgentEnv
	// Create a new process group so we can kill all child processes together.
	// This is important for adapters like OpenCode that spawn child processes
	// (npx -> sh -> node -> opencode binary).
	setProcGroup(m.cmd)

	m.logger.Info("agent command prepared",
		zap.Strings("args", m.cfg.AgentArgs),
		zap.Strings("extra_args", extraArgs),
		zap.String("workdir", m.cfg.WorkDir),
		zap.Int("env_count", len(m.cfg.AgentEnv)))

	return nil
}

func (m *Manager) ensureAgentTempEnv() error {
	dir := filepath.Join(os.TempDir(), agentTempDirRoot, agentTempDirName(m.cfg.SessionID, m.cfg.Port))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create agent temp dir: %w", err)
	}
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		m.cfg.AgentEnv = upsertEnvValue(m.cfg.AgentEnv, key, dir)
	}
	return nil
}

func agentTempDirName(sessionID string, port int) string {
	name := strings.TrimSpace(sessionID)
	if name == "" && port > 0 {
		name = fmt.Sprintf("port-%d", port)
	}
	if name == "" {
		return "default"
	}

	var b strings.Builder
	for _, r := range name {
		if isAgentTempDirRune(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	cleaned := strings.Trim(b.String(), "._-")
	if cleaned == "" {
		return "default"
	}
	return cleaned
}

func isAgentTempDirRune(r rune) bool {
	return r >= 'a' && r <= 'z' ||
		r >= 'A' && r <= 'Z' ||
		r >= '0' && r <= '9' ||
		r == '.' || r == '_' || r == '-'
}

func upsertEnvValue(env []string, key, value string) []string {
	prefix := key + "="
	next := make([]string, 0, len(env)+1)
	replaced := false
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			if !replaced {
				next = append(next, prefix+value)
				replaced = true
			}
			continue
		}
		next = append(next, item)
	}
	if !replaced {
		next = append(next, prefix+value)
	}
	return next
}

// startProcessPipes creates stdin, stdout, and stderr pipes for the agent subprocess.
// The pipes must be created before the process starts.
func (m *Manager) startProcessPipes() error {
	var err error
	m.stdin, err = m.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	m.stdout, err = m.cmd.StdoutPipe()
	if err != nil {
		_ = m.stdin.Close()
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	m.stderr, err = m.cmd.StderrPipe()
	if err != nil {
		_ = m.stdin.Close()
		_ = m.stdout.Close()
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}
	return nil
}

// startAgentShell auto-creates a shell session when ShellEnabled is configured.
// Failure is non-fatal: the agent can still work without a shell session.
func (m *Manager) startAgentShell() {
	if !m.cfg.ShellEnabled {
		return
	}
	shellCfg := shell.DefaultConfig(m.cfg.WorkDir)
	shellCfg.ShellCommand = preferredShellCommand(m.cfg.AgentEnv)
	shellSession, err := shell.NewSession(shellCfg, m.logger)
	if err != nil {
		m.logger.Warn("failed to create shell session", zap.Error(err))
		return
	}
	m.shell = shellSession
	m.logger.Info("shell session auto-created")
}

func preferredShellCommand(env []string) string {
	if value := lookupEnvValue(env, "AGENTCTL_SHELL_COMMAND"); value != "" {
		return value
	}
	return lookupEnvValue(env, "SHELL")
}

func lookupEnvValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

// Configure sets the agent command and optional environment variables.
// This must be called before Start() if the instance was created without a command.
// continueCommand is optional — when set, the adapter uses it for one-shot follow-up prompts.
func (m *Manager) Configure(command string, env map[string]string, approvalPolicy, continueCommand string) error {
	m.startMu.Lock()
	defer m.startMu.Unlock()

	if m.Status() == StatusRunning || m.Status() == StatusStarting {
		return fmt.Errorf("cannot configure while agent is running")
	}

	if command == "" {
		return fmt.Errorf("agent command cannot be empty")
	}

	// Parse the command string and update config
	args := config.ParseCommand(command)
	if len(args) == 0 {
		return fmt.Errorf("failed to parse agent command")
	}

	m.cfg.AgentCommand = command
	m.cfg.AgentArgs = args

	// Set approval policy if provided (for Codex)
	if approvalPolicy != "" {
		m.cfg.ApprovalPolicy = approvalPolicy
	}

	// Store continue command for one-shot adapters (e.g., Amp)
	if continueCommand != "" {
		m.cfg.ContinueCommand = continueCommand
	}

	// Merge additional env vars
	if len(env) > 0 {
		for k, v := range env {
			m.cfg.AgentEnv = append(m.cfg.AgentEnv, fmt.Sprintf("%s=%s", k, v))
		}
	}

	m.logger.Info("agent configured",
		zap.String("command", command),
		zap.Strings("args", args),
		zap.String("approval_policy", m.cfg.ApprovalPolicy),
		zap.String("continue_command", continueCommand),
		zap.Int("env_count", len(env)))

	return nil
}

// createAdapter creates the appropriate protocol adapter based on configuration.
// This should be called before starting the process so PrepareEnvironment can run.
func (m *Manager) createAdapter() error {
	protocol := m.cfg.Protocol
	if protocol == "" {
		return fmt.Errorf("protocol not specified in configuration")
	}

	m.logger.Debug("creating adapter", zap.String("protocol", string(protocol)))
	adpt, err := adapter.NewAdapter(protocol, m.adapterCfg, m.logger)
	if err != nil {
		return fmt.Errorf("failed to create adapter: %w", err)
	}
	m.adapter = adpt

	// Set stderr provider for adapters that support it (Codex, StreamJSON)
	if setter, ok := m.adapter.(adapter.StderrProviderSetter); ok {
		setter.SetStderrProvider(m)
	}

	// Set the permission handler
	m.adapter.SetPermissionHandler(m.handlePermissionRequest)

	return nil
}

// forwardUpdates forwards updates from the adapter to the manager's channel
func (m *Manager) forwardUpdates() {
	defer m.wg.Done()

	updatesCh := m.adapter.Updates()
	for {
		select {
		case update, ok := <-updatesCh:
			if !ok {
				return
			}
			select {
			case m.updatesCh <- update:
			default:
				m.logger.Warn("updates channel full, dropping notification")
			}
		case <-m.stopCh:
			return
		}
	}
}

// GetUpdates returns the channel for agent event notifications
func (m *Manager) GetUpdates() <-chan adapter.AgentEvent {
	return m.updatesCh
}

// SendErrorEvent sends an error event on the updates channel so the
// lifecycle manager (and ultimately the UI) learns about the failure.
func (m *Manager) SendErrorEvent(errorMessage string) {
	select {
	case m.updatesCh <- adapter.AgentEvent{
		Type:  adapter.EventTypeError,
		Error: errorMessage,
	}:
	default:
		m.logger.Warn("updates channel full, could not send error event")
	}
}

// GetAdapter returns the protocol adapter
func (m *Manager) GetAdapter() adapter.AgentAdapter {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.adapter
}

// GetSessionID returns the current session ID from the adapter.
// The adapter is the single source of truth for session ID.
func (m *Manager) GetSessionID() string {
	m.mu.RLock()
	a := m.adapter
	m.mu.RUnlock()

	if a != nil {
		return a.GetSessionID()
	}
	return ""
}

// Stop stops the agent process
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	status := m.Status()
	if status == StatusStopped || status == StatusStopping {
		m.logger.Info("Stop called but already stopped/stopping", zap.String("status", string(status)))
		return nil
	}

	m.logger.Info("stopping agent process - START")
	m.status.Store(StatusStopping)

	m.stopShellAndProcesses(ctx)
	m.closeAdapterAndStdin()
	m.killProcessGroupIfRequired()
	m.waitForProcessExit(ctx)

	m.status.Store(StatusStopped)
	m.logger.Info("stopping agent process - COMPLETE")
	return nil
}

// stopShellAndProcesses stops the shell session, VS Code, workspace processes, and workspace tracker.
func (m *Manager) stopShellAndProcesses(ctx context.Context) {
	// Stop VS Code server if running
	m.logger.Debug("stopping vscode server")
	if err := m.StopVscode(ctx); err != nil {
		m.logger.Debug("failed to stop vscode server", zap.Error(err))
	}
	m.logger.Debug("vscode server stopped")

	m.logger.Debug("stopping shell session")
	if m.shell != nil {
		if err := m.shell.Stop(); err != nil {
			m.logger.Debug("failed to stop shell session", zap.Error(err))
		}
		m.shell = nil
	}
	m.logger.Debug("shell session stopped")

	m.logger.Debug("stopping terminal shells")
	m.shellMgr.StopAll()
	m.logger.Debug("terminal shells stopped")

	// Stop all running workspace processes (dev server, setup, cleanup, custom).
	m.logger.Debug("stopping workspace processes")
	if m.processRunner != nil {
		if err := m.processRunner.StopAll(ctx); err != nil {
			m.logger.Debug("failed to stop workspace processes", zap.Error(err))
		}
	}
	m.logger.Debug("workspace processes stopped")

	m.logger.Debug("stopping workspace tracker")
	if m.workspaceTracker != nil {
		m.workspaceTracker.Stop()
	}
	for _, t := range m.repoTrackers {
		t.Stop()
	}
	m.logger.Debug("workspace tracker stopped")
}

// closeAdapterAndStdin closes the protocol adapter, the stop channel, and stdin.
func (m *Manager) closeAdapterAndStdin() {
	m.logger.Debug("closing adapter")
	if m.adapter != nil {
		if err := m.adapter.Close(); err != nil {
			m.logger.Debug("failed to close adapter", zap.Error(err))
		}
	}
	m.logger.Debug("adapter closed")

	m.logger.Debug("closing stop channel")
	if m.stopCh != nil {
		close(m.stopCh)
	}
	m.logger.Debug("stop channel closed")

	// Close stdin to signal EOF to agent
	m.logger.Debug("closing stdin")
	if m.stdin != nil {
		if err := m.stdin.Close(); err != nil {
			m.logger.Debug("failed to close stdin", zap.Error(err))
		}
	}
	m.logger.Debug("stdin closed")
}

// killProcessGroupIfRequired kills the entire process group for adapters (such as
// OpenCode) that run as HTTP servers and do not exit when stdin is closed.
func (m *Manager) killProcessGroupIfRequired() {
	if m.adapter == nil || !m.adapter.RequiresProcessKill() {
		return
	}
	if m.cmd == nil || m.cmd.Process == nil {
		return
	}
	// We kill the process group to ensure all child processes are killed too.
	// This is important because OpenCode spawns: npx -> sh -> node -> opencode binary
	pid := m.cmd.Process.Pid
	m.logger.Debug("killing process group", zap.Int("pgid", pid))
	if err := killProcessGroup(pid); err != nil {
		m.logger.Debug("failed to kill process group, trying single process", zap.Error(err))
		if err := m.cmd.Process.Kill(); err != nil {
			m.logger.Warn("failed to kill process", zap.Error(err))
		}
	}
}

// waitForProcessExit waits for all goroutines to finish, force-killing on context timeout.
func (m *Manager) waitForProcessExit(ctx context.Context) {
	m.logger.Debug("waiting for process to exit")
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		m.logger.Info("agent process stopped gracefully")
	case <-ctx.Done():
		if m.cmd != nil && m.cmd.Process != nil {
			m.logger.Warn("force killing agent process")
			if err := m.cmd.Process.Kill(); err != nil {
				m.logger.Warn("failed to kill agent process", zap.Error(err))
			}
		}
	}
}

// readStderr reads and logs stderr from the agent
func (m *Manager) readStderr() {
	defer m.wg.Done()

	scanner := bufio.NewScanner(m.stderr)
	for scanner.Scan() {
		line := scanner.Text()
		m.logger.Debug("agent stderr", zap.String("line", line))

		// Buffer the line for error context
		m.appendStderr(line)
	}

	if err := scanner.Err(); err != nil {
		m.logger.Debug("stderr reader error", zap.Error(err))
	}
}

// ansiEscapeRegex matches ANSI escape sequences
var ansiEscapeRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// stripANSI removes ANSI escape codes from a string
func stripANSI(s string) string {
	return ansiEscapeRegex.ReplaceAllString(s, "")
}

// appendStderr adds a line to the stderr ring buffer
func (m *Manager) appendStderr(line string) {
	m.stderrMu.Lock()
	defer m.stderrMu.Unlock()

	// Strip ANSI escape codes for cleaner display
	cleanLine := stripANSI(line)

	if len(m.stderrBuffer) >= defaultStderrBufferSize {
		// Ring buffer: drop oldest line
		m.stderrBuffer = m.stderrBuffer[1:]
	}
	m.stderrBuffer = append(m.stderrBuffer, cleanLine)
}

// GetRecentStderr returns a copy of the recent stderr lines
func (m *Manager) GetRecentStderr() []string {
	m.stderrMu.RLock()
	defer m.stderrMu.RUnlock()

	result := make([]string, len(m.stderrBuffer))
	copy(result, m.stderrBuffer)
	return result
}

// ClearStderrBuffer clears the stderr buffer (e.g., after successful operation)
func (m *Manager) ClearStderrBuffer() {
	m.stderrMu.Lock()
	defer m.stderrMu.Unlock()
	m.stderrBuffer = nil
}

// waitForExit waits for the process to exit
func (m *Manager) waitForExit() {
	defer m.wg.Done()
	defer close(m.doneCh)

	err := m.cmd.Wait()

	if err != nil {
		m.exitErr.Store(errorWrapper{err: err})
		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			m.exitCode.Store(int32(exitCode))
		}
		// Include recent stderr for better error diagnostics
		recentStderr := m.GetRecentStderr()
		m.logger.Error("agent process exited with error",
			zap.Error(err),
			zap.Int("exit_code", exitCode),
			zap.Strings("recent_stderr", recentStderr))

		// Send error event to the updates channel so UI can display it
		errorMsg := fmt.Sprintf("Agent process exited with code %d", exitCode)
		if len(recentStderr) > 0 {
			errorMsg = fmt.Sprintf("%s: %s", errorMsg, strings.Join(recentStderr, "; "))
		}
		select {
		case m.updatesCh <- adapter.AgentEvent{
			Type:  adapter.EventTypeError,
			Error: errorMsg,
			Data: map[string]any{
				"exit_code":     exitCode,
				"recent_stderr": recentStderr,
			},
		}:
		default:
			m.logger.Warn("updates channel full, could not send exit error event")
		}
	} else {
		m.exitCode.Store(0)
		m.logger.Info("agent process exited successfully")
	}

	m.status.Store(StatusStopped)
}

// GetFinalCommand returns the full command string that was used to start the agent process,
// including all adapter-added arguments (sandbox mode, MCP flags, etc.).
func (m *Manager) GetFinalCommand() string {
	return m.finalCommand
}

// GetProcessInfo returns information about the process
func (m *Manager) GetProcessInfo() map[string]interface{} {
	info := map[string]interface{}{
		"status":    string(m.Status()),
		"exit_code": m.ExitCode(),
	}

	if m.cmd != nil && m.cmd.Process != nil {
		info["pid"] = m.cmd.Process.Pid
	}

	if err := m.ExitError(); err != nil {
		info["exit_error"] = err.Error()
	}

	return info
}

// handlePermissionRequest handles permission requests from the agent
// It stores the pending request and waits for a response from the backend
func (m *Manager) handlePermissionRequest(ctx context.Context, req *adapter.PermissionRequest) (*adapter.PermissionResponse, error) {
	// Use the adapter-provided pending ID if available, otherwise generate one.
	// This ensures the ID sent to the frontend matches the one used for response lookup.
	// For OpenCode (per_xxx) and Claude Code (requestID), the adapter passes its ID
	// so we use the same ID throughout the permission flow.
	pendingID := req.PendingID
	if pendingID == "" {
		pendingID = fmt.Sprintf("%s-%s-%d", req.SessionID, req.ToolCallID, time.Now().UnixNano())
	}

	m.logger.Info("handling permission request",
		zap.String("pending_id", pendingID),
		zap.String("session_id", req.SessionID),
		zap.String("tool_call_id", req.ToolCallID),
		zap.String("title", req.Title),
		zap.Bool("auto_approve", m.cfg.AutoApprovePermissions))

	// If auto-approve is enabled, immediately approve with the first "allow" option
	if m.cfg.AutoApprovePermissions {
		return m.autoApprovePermission(req)
	}

	// Create pending permission with response channel
	pending := &PendingPermission{
		ID:         pendingID,
		Request:    req,
		ResponseCh: make(chan *adapter.PermissionResponse, 1),
		CreatedAt:  time.Now(),
	}

	// Store pending permission
	m.permissionMu.Lock()
	m.pendingPermissions[pendingID] = pending
	m.permissionMu.Unlock()

	// Clean up when done
	defer func() {
		m.permissionMu.Lock()
		delete(m.pendingPermissions, pendingID)
		m.permissionMu.Unlock()
	}()

	// Send notification to backend via updates channel
	m.sendPermissionNotification(pending)

	// Wait for response indefinitely - user may close and reopen the page
	select {
	case resp := <-pending.ResponseCh:
		m.logger.Info("received permission response",
			zap.String("pending_id", pendingID),
			zap.String("option_id", resp.OptionID),
			zap.Bool("cancelled", resp.Cancelled))
		if resp.Cancelled {
			m.sendPermissionCancelledNotification(pending)
		}
		return resp, nil
	case <-ctx.Done():
		m.logger.Warn("permission request context cancelled",
			zap.String("pending_id", pendingID))
		// Send cancellation notification so the backend can update the permission message status
		m.sendPermissionCancelledNotification(pending)
		return &adapter.PermissionResponse{Cancelled: true}, nil
	}
}

// autoApprovePermission automatically approves a permission request
// by selecting the first "allow" option, or the first option if no allow option exists
func (m *Manager) autoApprovePermission(req *adapter.PermissionRequest) (*adapter.PermissionResponse, error) {
	if len(req.Options) == 0 {
		m.logger.Warn("no options available for auto-approve, cancelling")
		return &adapter.PermissionResponse{Cancelled: true}, nil
	}

	// Find the first "allow" option
	var selectedOption *adapter.PermissionOption
	for i := range req.Options {
		opt := &req.Options[i]
		if opt.Kind == "allow_once" || opt.Kind == "allow_always" {
			selectedOption = opt
			break
		}
	}

	// If no allow option, use the first option
	if selectedOption == nil {
		selectedOption = &req.Options[0]
	}

	m.logger.Info("auto-approving permission request",
		zap.String("option_id", selectedOption.OptionID),
		zap.String("option_name", selectedOption.Name),
		zap.String("kind", selectedOption.Kind))

	return &adapter.PermissionResponse{
		OptionID: selectedOption.OptionID,
	}, nil
}

// sendPermissionNotification sends a permission request notification through the updates channel.
// Uses a blocking send with timeout to ensure delivery. If delivery fails within 5 seconds,
// auto-cancels the permission so the agent doesn't hang waiting for a response.
func (m *Manager) sendPermissionNotification(pending *PendingPermission) {
	// Convert options to streams.PermissionOption (types.PermissionOption is an alias)
	options := make([]streams.PermissionOption, len(pending.Request.Options))
	copy(options, pending.Request.Options)

	event := adapter.AgentEvent{
		Type:              adapter.EventTypePermissionRequest,
		SessionID:         pending.Request.SessionID,
		ToolCallID:        pending.Request.ToolCallID,
		PendingID:         pending.ID,
		PermissionTitle:   pending.Request.Title,
		PermissionOptions: options,
		ActionType:        pending.Request.ActionType,
		ActionDetails:     pending.Request.ActionDetails,
	}

	m.logger.Info("sending permission notification via updates channel",
		zap.String("pending_id", pending.ID),
		zap.String("title", pending.Request.Title),
		zap.String("action_type", pending.Request.ActionType))

	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case m.updatesCh <- event:
		// Sent successfully
	case <-timer.C:
		m.logger.Error("failed to deliver permission notification, auto-cancelling",
			zap.String("pending_id", pending.ID))
		select {
		case pending.ResponseCh <- &adapter.PermissionResponse{Cancelled: true}:
		default:
		}
	}
}

// sendPermissionCancelledNotification sends a notification that a permission request was cancelled.
// This happens when the context is cancelled (e.g., agent completes or user stops the task)
// before the user responds to the permission request.
func (m *Manager) sendPermissionCancelledNotification(pending *PendingPermission) {
	event := adapter.AgentEvent{
		Type:      adapter.EventTypePermissionCancelled,
		SessionID: pending.Request.SessionID,
		PendingID: pending.ID,
	}

	m.logger.Info("sending permission cancelled notification",
		zap.String("pending_id", pending.ID),
		zap.String("session_id", pending.Request.SessionID))

	select {
	case m.updatesCh <- event:
		// Sent successfully
	default:
		m.logger.Warn("updates channel full, dropping permission cancelled notification",
			zap.String("pending_id", pending.ID))
	}
}

// RespondToPermission responds to a pending permission request
func (m *Manager) RespondToPermission(pendingID string, optionID string, cancelled bool) error {
	m.permissionMu.RLock()
	pending, ok := m.pendingPermissions[pendingID]
	m.permissionMu.RUnlock()

	if !ok {
		return fmt.Errorf("pending permission not found: %s", pendingID)
	}

	m.logger.Info("responding to permission request",
		zap.String("pending_id", pendingID),
		zap.String("option_id", optionID),
		zap.Bool("cancelled", cancelled))

	// Send response (non-blocking since channel is buffered)
	select {
	case pending.ResponseCh <- &adapter.PermissionResponse{
		OptionID:  optionID,
		Cancelled: cancelled,
	}:
		return nil
	default:
		return fmt.Errorf("response channel full for pending permission: %s", pendingID)
	}
}

// CancelPendingPermissions cancels all pending permission requests.
// Called before sending a new prompt so the agent isn't blocked waiting for responses.
func (m *Manager) CancelPendingPermissions() {
	m.permissionMu.RLock()
	pending := make([]*PendingPermission, 0, len(m.pendingPermissions))
	for _, p := range m.pendingPermissions {
		pending = append(pending, p)
	}
	m.permissionMu.RUnlock()

	for _, p := range pending {
		m.logger.Info("cancelling pending permission before new prompt",
			zap.String("pending_id", p.ID))
		select {
		case p.ResponseCh <- &adapter.PermissionResponse{Cancelled: true}:
		default:
		}
	}
}

// Shell returns the embedded shell session, or nil if not available
func (m *Manager) Shell() *shell.Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.shell
}

// ShellManager returns the per-terminal shell session manager.
func (m *Manager) ShellManager() *shell.Manager {
	return m.shellMgr
}

// StartShell creates and starts the shell session independently of the agent process.
// This is used in passthrough mode where the agent runs directly via InteractiveRunner
// but we still need shell access for the workspace.
// Returns nil if shell is already started or if ShellEnabled is false.
func (m *Manager) StartShell() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Already running
	if m.shell != nil {
		return nil
	}

	// Shell disabled
	if !m.cfg.ShellEnabled {
		return nil
	}

	shellCfg := shell.DefaultConfig(m.cfg.WorkDir)
	shellCfg.ShellCommand = preferredShellCommand(m.cfg.AgentEnv)
	shellSession, err := shell.NewSession(shellCfg, m.logger)
	if err != nil {
		return fmt.Errorf("failed to create shell session: %w", err)
	}

	m.shell = shellSession
	m.logger.Info("shell session started independently")
	return nil
}

// StartVscode starts the code-server process on a random OS-assigned port.
// The VS Code server runs independently of the agent process.
// Start is non-blocking — the caller should poll VscodeInfo() for status updates.
func (m *Manager) StartVscode(_ context.Context, theme string) {
	m.vscodeMu.Lock()
	defer m.vscodeMu.Unlock()

	if m.vscode != nil {
		info := m.vscode.Info()
		if info.Status == VscodeStatusRunning || info.Status == VscodeStatusStarting || info.Status == VscodeStatusInstalling {
			return
		}
	}

	command := m.cfg.VscodeCommand
	if command == "" {
		command = "code-server"
	}

	strategy := codeServerInstallStrategy(m.logger)
	m.vscode = NewVscodeManager(command, m.cfg.WorkDir, theme, strategy, m.logger)
	m.vscode.Start()
}

// codeServerInstallStrategy returns a tarball strategy that auto-installs code-server.
func codeServerInstallStrategy(log *logger.Logger) tools.Strategy {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	installDir := filepath.Join(home, ".kandev", "tools", "code-server")

	return tools.NewGithubTarballStrategy(installDir, "code-server", tools.GithubTarballConfig{
		Owner:        "coder",
		Repo:         "code-server",
		Version:      "4.96.4",
		AssetPattern: "code-server-{version}-{os}-{arch}.tar.gz",
		BinaryPath:   "code-server-{version}-{os}-{arch}/bin/code-server",
		Targets: map[string]string{
			"darwin/arm64": "macos-arm64",
			"darwin/amd64": "macos-amd64",
			"linux/amd64":  "linux-amd64",
			"linux/arm64":  "linux-arm64",
		},
	}, log)
}

// StopVscode stops the code-server process if running.
func (m *Manager) StopVscode(ctx context.Context) error {
	m.vscodeMu.Lock()
	defer m.vscodeMu.Unlock()

	if m.vscode == nil {
		return nil
	}

	err := m.vscode.Stop(ctx)
	m.vscode = nil
	return err
}

// VscodeInfo returns the current VS Code server status.
func (m *Manager) VscodeInfo() VscodeInfo {
	m.vscodeMu.Lock()
	defer m.vscodeMu.Unlock()

	if m.vscode == nil {
		return VscodeInfo{Status: VscodeStatusStopped}
	}
	return m.vscode.Info()
}

// VscodeOpenFile opens a file in the running VS Code instance via the Remote CLI.
// If code-server is not running, it auto-starts it and waits for readiness.
//
// Note: there is a benign race between the needsStart check and WaitForRunning —
// another goroutine could stop vscode in between. WaitForRunning handles this
// correctly by returning an error for stopped/error states.
func (m *Manager) VscodeOpenFile(ctx context.Context, path string, line, col int) error {
	m.vscodeMu.Lock()
	needsStart := m.vscode == nil
	if !needsStart {
		info := m.vscode.Info()
		needsStart = info.Status == VscodeStatusStopped || info.Status == VscodeStatusError
	}
	m.vscodeMu.Unlock()

	if needsStart {
		m.logger.Info("auto-starting code-server for open-file request")
		m.StartVscode(ctx, "")
	}

	m.vscodeMu.Lock()
	vscode := m.vscode
	m.vscodeMu.Unlock()

	if vscode == nil {
		return fmt.Errorf("failed to initialize code-server")
	}

	if err := vscode.WaitForRunning(ctx); err != nil {
		return fmt.Errorf("code-server not ready: %w", err)
	}

	return vscode.OpenFile(ctx, path, line, col)
}
