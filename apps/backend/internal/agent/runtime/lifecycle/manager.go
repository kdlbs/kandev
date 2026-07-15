// Package lifecycle manages agent instance lifecycles including tracking,
// state transitions, and cleanup.
package lifecycle

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"

	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/runtime/activity"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

// ExecutorFallbackPolicy controls behavior when a requested runtime is unavailable.
type ExecutorFallbackPolicy string

const (
	// ExecutorFallbackAllow silently falls back to the default runtime (current behavior).
	ExecutorFallbackAllow ExecutorFallbackPolicy = "allow"
	// ExecutorFallbackWarn falls back but logs a warning (current behavior, explicit).
	ExecutorFallbackWarn ExecutorFallbackPolicy = "warn"
	// ExecutorFallbackDeny returns an error if the requested runtime is unavailable.
	ExecutorFallbackDeny ExecutorFallbackPolicy = "deny"
)

// Manager manages agent instance lifecycles
type Manager struct {
	registry        *registry.Registry
	eventBus        bus.EventBus
	credsMgr        CredentialsManager
	profileResolver ProfileResolver
	worktreeMgr     *worktree.Manager
	mcpProvider     McpConfigProvider
	logger          *logger.Logger
	// dataDir is the kandev root directory. Misnamed for historical reasons:
	// cmd/kandev/agents.go passes cfg.ResolvedHomeDir() (the kandev root —
	// typically ~/.kandev) here, not ResolvedDataDir(). Used for:
	// - Session history storage (SessionHistoryManager)
	// - Ephemeral workspace creation (quick chat) at <root>/quick-chat/<sessionID>
	// - Scratch workspaces for repo-less tasks at <root>/tasks/<workspaceID>/<taskID>
	dataDir string

	// ExecutorRegistry manages multiple runtimes (Docker, Standalone, etc.)
	// Each task can select its runtime based on executor type.
	executorRegistry *ExecutorRegistry

	// executorFallbackPolicy controls behavior when a requested runtime is unavailable.
	executorFallbackPolicy ExecutorFallbackPolicy

	// Refactored components for separation of concerns
	executionStore *ExecutionStore        // Thread-safe execution tracking
	commandBuilder *CommandBuilder        // Builds agent commands from registry config
	sessionManager *SessionManager        // Handles ACP session initialization
	streamManager  *StreamManager         // Manages WebSocket streams
	eventPublisher *EventPublisher        // Publishes lifecycle events
	historyManager *SessionHistoryManager // Stores session history for context injection (fork_session pattern)

	// Workspace info provider for on-demand instance creation
	workspaceInfoProvider WorkspaceInfoProvider

	// bootMessageService creates boot messages displayed in chat during agent startup.
	bootMessageService BootMessageService

	// preparerRegistry maps executor types to environment preparers.
	preparerRegistry *PreparerRegistry

	// mcpHandler is the MCP request dispatcher for handling MCP requests
	// from agentctl instances through the agent stream.
	mcpHandler agentctl.MCPHandler

	// singleflight deduplicates concurrent GetOrEnsureExecution calls for the same session
	ensureExecutionGroup singleflight.Group

	// Background remote status polling
	remoteStatusPollInterval time.Duration
	remoteStatusMu           sync.RWMutex
	remoteStatusBySession    map[string]*RemoteStatus
	stopCh                   chan struct{}
	stopOnce                 sync.Once
	wg                       sync.WaitGroup
	// shuttingDown is flipped true when graceful shutdown begins (see
	// StopAllAgents) so handlers running in detached goroutines can
	// short-circuit work that would otherwise race the teardown and log
	// confusing errors against children already being stopped.
	shuttingDown atomic.Bool

	// pollAggregator routes hub session-mode events to agentctl. See
	// manager_subscription.go.
	pollAggregator *workspacePollAggregator

	// secretStore encrypts/decrypts runtime auth tokens (e.g., agentctl handshake tokens).
	// Used to persist tokens across backend restarts for remote executor recovery.
	secretStore secrets.SecretStore

	// runningWriter persists the executors_running row in lockstep with executionStore.
	// See SetExecutorRunningWriter and persistence.go. The lifecycle manager is the
	// only component allowed to write the lifecycle-owned columns of this table.
	runningWriter ExecutorRunningWriter

	// agentProfileReader resolves the full agent_profiles row (including the
	// office-enrichment fields added in ADR 0005 Wave A) for the launch-prep
	// SkillDeployer hook. Nil → skill deploy is skipped.
	agentProfileReader AgentProfileReader

	// skillDeployer materialises per-profile skills + custom prompt before
	// the agent process starts. Defaults to a no-op deployer; office wires
	// its concrete implementation via SetSkillDeployer.
	skillDeployer SkillDeployer

	// remediateNpxCache is the hook fired when the routing classifier
	// returns CodeNpxCacheCorrupted. NewManager wires routingerr.RemediateNpxCache;
	// tests override it to avoid touching the real filesystem.
	remediateNpxCache func(path string, log *zap.Logger) error

	// standaloneHostPID is the OS process id of the standalone agentctl
	// control-server this backend spawned on the local host. It is the
	// host-local liveness handle recorded in executors_running.local_pid for
	// local/standalone rows (see persistence.go / #1597 truthful executor rows).
	// 0 when unset (tests, or before the launcher wires it). Never used for
	// SSH/remote rows — their process lives on another host.
	standaloneHostPID atomic.Int64

	// managedGoCache provides the opt-in GOCACHE for host-local executions.
	// System storage wiring installs it after settings persistence is ready.
	managedGoCache ManagedGoCacheEnvironmentProvider

	activityCoordinator *activity.Coordinator
	activityMu          sync.Mutex
	activityLeases      map[string]*activity.TaskLease
	activityPending     map[string]uint64
	activityGeneration  uint64
}

// ManagedGoCacheEnvironmentProvider supplies the environment for one new
// local execution. Implementations must return an absolute GOCACHE path.
type ManagedGoCacheEnvironmentProvider interface {
	ExecutionEnvironment(ctx context.Context) (map[string]string, error)
}

// SetManagedGoCacheEnvironmentProvider wires install-wide managed cache settings.
func (m *Manager) SetManagedGoCacheEnvironmentProvider(provider ManagedGoCacheEnvironmentProvider) {
	m.managedGoCache = provider
}

// SetActivityCoordinator wires the install-wide host-resource activity gate.
// It is optional so embedded and test configurations retain legacy behavior.
func (m *Manager) SetActivityCoordinator(coordinator *activity.Coordinator) {
	m.activityMu.Lock()
	m.activityCoordinator = coordinator
	if m.activityLeases == nil {
		m.activityLeases = make(map[string]*activity.TaskLease)
	}
	if m.activityPending == nil {
		m.activityPending = make(map[string]uint64)
	}
	m.activityMu.Unlock()
	if m.executorRegistry == nil {
		return
	}
	backend, err := m.executorRegistry.GetBackend(executor.NameDocker)
	if err != nil {
		return
	}
	if dockerExecutor, ok := backend.(*DockerExecutor); ok {
		dockerExecutor.SetActivityCoordinator(coordinator)
	}
}

// SetStandaloneHostPID records the local agentctl control-server PID so
// local/standalone executor rows can carry a real host-local liveness handle.
// Wired during DI from the agentctl launcher (see backendapp). Safe to leave
// unset in tests that don't exercise the persistence path.
func (m *Manager) SetStandaloneHostPID(pid int) {
	m.standaloneHostPID.Store(int64(pid))
}

// NewManager creates a new lifecycle manager.
// The executorRegistry manages multiple runtimes (Docker, Standalone, etc.) for task-specific execution.
// The fallbackPolicy controls behavior when a requested runtime is unavailable.
func NewManager(
	reg *registry.Registry,
	eventBus bus.EventBus,
	executorRegistry *ExecutorRegistry,
	credsMgr CredentialsManager,
	profileResolver ProfileResolver,
	mcpProvider McpConfigProvider,
	fallbackPolicy ExecutorFallbackPolicy,
	dataDir string,
	log *logger.Logger,
) *Manager {
	componentLogger := log.WithFields(zap.String("component", "lifecycle-manager"))

	// Initialize command builder
	commandBuilder := NewCommandBuilder()

	// Create stop channel for graceful shutdown
	stopCh := make(chan struct{})

	// Initialize session manager
	sessionManager := NewSessionManager(log, stopCh)

	// Initialize event publisher
	eventPublisher := NewEventPublisher(eventBus, log)

	// Initialize execution store
	executionStore := NewExecutionStore()

	// Initialize session history manager for fork_session pattern (context injection)
	historyManager, err := NewSessionHistoryManager("", dataDir, log)
	if err != nil {
		log.Warn("failed to create session history manager, context injection disabled", zap.Error(err))
	}

	mgr := &Manager{
		registry:                 reg,
		eventBus:                 eventBus,
		executorRegistry:         executorRegistry,
		executorFallbackPolicy:   fallbackPolicy,
		credsMgr:                 credsMgr,
		profileResolver:          profileResolver,
		mcpProvider:              mcpProvider,
		logger:                   componentLogger,
		dataDir:                  dataDir,
		executionStore:           executionStore,
		commandBuilder:           commandBuilder,
		sessionManager:           sessionManager,
		eventPublisher:           eventPublisher,
		historyManager:           historyManager,
		remoteStatusPollInterval: 60 * time.Second,
		remoteStatusBySession:    make(map[string]*RemoteStatus),
		stopCh:                   stopCh,
		skillDeployer:            NoopSkillDeployer(),
		remediateNpxCache:        routingerr.RemediateNpxCache,
	}
	// Initialize stream manager with callbacks that delegate to manager methods
	// mcpHandler will be set later via SetMCPHandler.
	// stopCh is shared with the manager so workspace-stream backoff drains on Stop.
	mgr.streamManager = NewStreamManager(log, StreamCallbacks{
		OnAgentEvent:       mgr.handleAgentEvent,
		OnStreamDisconnect: mgr.handleStreamDisconnect,
		OnGitStatus:        mgr.handleGitStatusUpdate,
		OnGitCommit:        mgr.handleGitCommitCreated,
		OnGitReset:         mgr.handleGitResetDetected,
		OnBranchSwitch:     mgr.handleBranchSwitch,
		OnFileChange:       mgr.handleFileChangeNotification,
		OnShellOutput:      mgr.handleShellOutput,
		OnShellExit:        mgr.handleShellExit,
		OnProcessOutput:    mgr.handleProcessOutput,
		OnProcessStatus:    mgr.handleProcessStatus,
	}, nil, stopCh)

	// Set session manager dependencies for full orchestration
	sessionManager.SetDependencies(eventPublisher, mgr.streamManager, executionStore, historyManager)
	sessionManager.SetPromptStarter(mgr.BeginPrompt)

	mgr.pollAggregator = newWorkspacePollAggregator(mgr)

	if executorRegistry != nil {
		mgr.logger.Info("initialized with runtimes", zap.Int("count", len(executorRegistry.List())))
	}

	return mgr
}

// HandleSessionMode routes a session-level mode transition (from the gateway
// hub) into the per-workspace aggregator, which pushes the resulting
// workspace-effective mode to agentctl. See manager_subscription.go.
func (m *Manager) HandleSessionMode(sessionID string, mode WorkspacePollMode) {
	if m.pollAggregator == nil {
		return
	}
	m.pollAggregator.HandleSessionMode(sessionID, mode)
}

// flushCachedPollMode pushes any session mode the gateway cached before this
// execution was ready. Closes the pre-execution-focus race where the frontend
// sent session.focus during execution startup and the cached mode never
// reached agentctl. No-op when nothing was cached.
func (m *Manager) flushCachedPollMode(sessionID string) {
	if m.pollAggregator == nil {
		return
	}
	m.pollAggregator.FlushSessionMode(sessionID)
}

// SetWorktreeManager sets the worktree manager for Git worktree isolation.
//
// This must be called before launching agents if Git worktree support is enabled in the runtime.
// The worktree manager creates isolated Git working directories for each agent execution,
// allowing multiple agents to work on the same repository without conflicts.
//
// Call this during initialization, typically when setting up the orchestrator service.
// If not set, agents will work directly in the repository's main working directory.
func (m *Manager) SetWorktreeManager(worktreeMgr *worktree.Manager) {
	m.worktreeMgr = worktreeMgr
	// Register the worktree preparer so that executor type "worktree" gets
	// worktree-specific preparation (create git worktree, checkout PR branch)
	// instead of the generic local preparer.
	if m.preparerRegistry != nil {
		m.preparerRegistry.Register(models.ExecutorTypeWorktree, NewWorktreePreparer(worktreeMgr, m.logger))
	}
}

// WorktreeManager returns the configured worktree manager. Returns nil
// when worktree support has not been wired (legacy / tests). Used by
// the office task-handoffs cleaner to translate worktree IDs into
// disk operations.
func (m *Manager) WorktreeManager() *worktree.Manager {
	return m.worktreeMgr
}

// SetMCPHandler sets the MCP request handler for dispatching MCP tool calls.
//
// MCP requests from agents flow through the agent stream (WebSocket) to the backend,
// where they are dispatched to this handler. This enables agents to use MCP tools
// like listing workspaces, boards, tasks, and asking user questions.
//
// This must be called before agents start making MCP calls. Typically set during
// initialization after the MCP handlers are created.
func (m *Manager) SetMCPHandler(handler agentctl.MCPHandler) {
	m.mcpHandler = handler
	// Update the stream manager with the handler
	m.streamManager.mcpHandler = handler
}

// SetWorkspaceInfoProvider sets the provider for workspace information.
//
// The WorkspaceInfoProvider interface allows the lifecycle manager to dynamically create
// agent executions on-demand when the frontend connects to a session that doesn't have
// an active execution yet. This enables session resume after server restart or when
// accessing a session via URL (/task/[id]/[sessionId]).
//
// The provider must implement:
//   - GetWorkspaceInfoBySessionID(ctx, sessionID) - Returns workspace path, worktree info,
//     and MCP servers configured for the session
//
// This is typically called during initialization, with the task service as the provider.
// Without this, EnsureWorkspaceExecutionForSession will fail.
func (m *Manager) SetWorkspaceInfoProvider(provider WorkspaceInfoProvider) {
	m.workspaceInfoProvider = provider
}

// SetBootMessageService sets the service used to create boot messages in chat
// during agent startup. If not set, no boot messages will be created.
func (m *Manager) SetBootMessageService(svc BootMessageService) {
	m.bootMessageService = svc
}

// SetPreparerRegistry sets the registry of environment preparers.
func (m *Manager) SetPreparerRegistry(registry *PreparerRegistry) {
	m.preparerRegistry = registry
}

// SetSecretStore sets the secret store for encrypting runtime auth tokens.
func (m *Manager) SetSecretStore(store secrets.SecretStore) {
	m.secretStore = store
}

// SetAgentProfileReader wires the reader the launch-prep SkillDeployer uses
// to resolve full agent_profiles rows by id. Without it, the skill deploy
// step is silently skipped.
func (m *Manager) SetAgentProfileReader(reader AgentProfileReader) {
	m.agentProfileReader = reader
}

// SetSkillDeployer plugs in a concrete deployer that materialises per-profile
// skills + custom prompt into the workspace before launch. Default is a
// no-op deployer; office wires its real implementation here.
func (m *Manager) SetSkillDeployer(deployer SkillDeployer) {
	if deployer == nil {
		m.skillDeployer = NoopSkillDeployer()
		return
	}
	m.skillDeployer = deployer
}

// DockerClientProvider returns a function that lazily resolves the Docker client
// from the Docker executor in the registry. Returns nil if Docker is unavailable.
func (m *Manager) DockerClientProvider() func() *docker.Client {
	return func() *docker.Client {
		if m.executorRegistry == nil {
			return nil
		}
		backend, err := m.executorRegistry.GetBackend(executor.NameDocker)
		if err != nil {
			return nil
		}
		dockerExec, ok := backend.(*DockerExecutor)
		if !ok {
			return nil
		}
		return dockerExec.Client()
	}
}
