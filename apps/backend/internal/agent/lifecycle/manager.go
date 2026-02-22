// Package lifecycle manages agent instance lifecycles including tracking,
// state transitions, and cleanup.
package lifecycle

import (
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/registry"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
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

	// ExecutorRegistry manages multiple runtimes (Docker, Standalone, etc.)
	// Each task can select its runtime based on executor type.
	executorRegistry *ExecutorRegistry

	// executorFallbackPolicy controls behavior when a requested runtime is unavailable.
	executorFallbackPolicy ExecutorFallbackPolicy

	// Refactored components for separation of concerns
	executionStore   *ExecutionStore        // Thread-safe execution tracking
	commandBuilder   *CommandBuilder        // Builds agent commands from registry config
	sessionManager   *SessionManager        // Handles ACP session initialization
	streamManager    *StreamManager         // Manages WebSocket streams
	eventPublisher   *EventPublisher        // Publishes lifecycle events
	containerManager *ContainerManager      // Manages Docker containers (optional, nil for non-Docker runtimes)
	historyManager   *SessionHistoryManager // Stores session history for context injection (fork_session pattern)

	// Workspace info provider for on-demand instance creation
	workspaceInfoProvider WorkspaceInfoProvider

	// bootMessageService creates boot messages displayed in chat during agent startup.
	bootMessageService BootMessageService

	// preparerRegistry maps executor types to environment preparers.
	preparerRegistry *PreparerRegistry

	// mcpHandler is the MCP request dispatcher for handling MCP requests
	// from agentctl instances through the agent stream.
	mcpHandler agentctl.MCPHandler

	// Background remote status polling
	remoteStatusPollInterval time.Duration
	remoteStatusMu           sync.RWMutex
	remoteStatusBySession    map[string]*RemoteStatus
	stopCh                   chan struct{}
	wg                       sync.WaitGroup
}

// NewManager creates a new lifecycle manager.
// The executorRegistry manages multiple runtimes (Docker, Standalone, etc.) for task-specific execution.
// The containerManager parameter is optional and used for Docker cleanup (pass nil for non-Docker runtimes).
// The fallbackPolicy controls behavior when a requested runtime is unavailable.
func NewManager(
	reg *registry.Registry,
	eventBus bus.EventBus,
	executorRegistry *ExecutorRegistry,
	containerManager *ContainerManager,
	credsMgr CredentialsManager,
	profileResolver ProfileResolver,
	mcpProvider McpConfigProvider,
	fallbackPolicy ExecutorFallbackPolicy,
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
	historyManager, err := NewSessionHistoryManager("", log)
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
		executionStore:           executionStore,
		commandBuilder:           commandBuilder,
		sessionManager:           sessionManager,
		eventPublisher:           eventPublisher,
		containerManager:         containerManager,
		historyManager:           historyManager,
		remoteStatusPollInterval: 60 * time.Second,
		remoteStatusBySession:    make(map[string]*RemoteStatus),
		stopCh:                   stopCh,
	}

	// Initialize stream manager with callbacks that delegate to manager methods
	// mcpHandler will be set later via SetMCPHandler
	mgr.streamManager = NewStreamManager(log, StreamCallbacks{
		OnAgentEvent:    mgr.handleAgentEvent,
		OnGitStatus:     mgr.handleGitStatusUpdate,
		OnGitCommit:     mgr.handleGitCommitCreated,
		OnGitReset:      mgr.handleGitResetDetected,
		OnFileChange:    mgr.handleFileChangeNotification,
		OnShellOutput:   mgr.handleShellOutput,
		OnShellExit:     mgr.handleShellExit,
		OnProcessOutput: mgr.handleProcessOutput,
		OnProcessStatus: mgr.handleProcessStatus,
	}, nil)

	// Set session manager dependencies for full orchestration
	sessionManager.SetDependencies(eventPublisher, mgr.streamManager, executionStore, historyManager)

	if executorRegistry != nil {
		mgr.logger.Info("initialized with runtimes", zap.Int("count", len(executorRegistry.List())))
	}

	return mgr
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
