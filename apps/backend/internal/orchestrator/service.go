// Package orchestrator provides the main orchestrator service that coordinates
// task execution across agents. It manages:
//
//   - Task queuing and scheduling via the Scheduler
//   - Agent lifecycle through the AgentManager
//   - Event handling and propagation
//   - Session management and resume
//
// The orchestrator acts as the central coordinator between the task service,
// agent lifecycle manager, and the event bus.
package orchestrator

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Common errors
var (
	ErrServiceAlreadyRunning = errors.New("service is already running")
	ErrServiceNotRunning     = errors.New("service is not running")
)

// ServiceConfig holds orchestrator service configuration
type ServiceConfig struct {
	Scheduler  scheduler.SchedulerConfig
	QueueSize  int
	QueueGroup string
}

// DefaultServiceConfig returns default configuration
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		Scheduler:  scheduler.DefaultSchedulerConfig(),
		QueueSize:  1000,
		QueueGroup: "orchestrator",
	}
}

// MessageCreator is an interface for creating messages on tasks
type MessageCreator interface {
	CreateAgentMessage(ctx context.Context, taskID, content, agentSessionID, turnID string) error
	CreateUserMessage(ctx context.Context, taskID, content, agentSessionID, turnID string, metadata map[string]interface{}) error
	// CreateToolCallMessage creates a message for a tool call.
	// normalized contains the typed tool payload data.
	// parentToolCallID is the parent Task tool call ID for subagent nesting (empty for top-level).
	CreateToolCallMessage(ctx context.Context, taskID, toolCallID, parentToolCallID, title, status, agentSessionID, turnID string, normalized *streams.NormalizedPayload) error
	// UpdateToolCallMessage updates a tool call message's status and optionally its normalized data.
	// If the message doesn't exist, it creates it using taskID, turnID, and msgType.
	// normalized contains the typed tool payload data.
	// parentToolCallID is the parent Task tool call ID for subagent nesting (empty for top-level).
	UpdateToolCallMessage(ctx context.Context, taskID, toolCallID, parentToolCallID, status, result, agentSessionID, title, turnID, msgType string, normalized *streams.NormalizedPayload) error
	CreateSessionMessage(ctx context.Context, taskID, content, agentSessionID, messageType, turnID string, metadata map[string]interface{}, requestsInput bool) error
	CreatePermissionRequestMessage(ctx context.Context, taskID, sessionID, pendingID, toolCallID, title, turnID string, options []map[string]interface{}, actionType string, actionDetails map[string]interface{}) (string, error)
	UpdatePermissionMessage(ctx context.Context, sessionID, pendingID string, status models.PermissionStatus) error
	// CreateAgentMessageStreaming creates a new agent message with a pre-generated ID for streaming updates
	CreateAgentMessageStreaming(ctx context.Context, messageID, taskID, content, agentSessionID, turnID string) error
	// AppendAgentMessage appends additional content to an existing streaming message
	AppendAgentMessage(ctx context.Context, messageID, additionalContent string) error
	// CreateThinkingMessageStreaming creates a new thinking message with a pre-generated ID for streaming updates
	CreateThinkingMessageStreaming(ctx context.Context, messageID, taskID, content, agentSessionID, turnID string) error
	// AppendThinkingMessage appends additional content to an existing streaming thinking message
	AppendThinkingMessage(ctx context.Context, messageID, additionalContent string) error
	// InvalidateModelCache clears any cached model for a session, forcing the next
	// message to re-read the model from the DB. Called after model switches.
	InvalidateModelCache(sessionID string)
}

// TurnService is an interface for managing session turns
type TurnService interface {
	StartTurn(ctx context.Context, sessionID string) (*models.Turn, error)
	CompleteTurn(ctx context.Context, turnID string) error
	GetTurn(ctx context.Context, turnID string) (*models.Turn, error)
	GetActiveTurn(ctx context.Context, sessionID string) (*models.Turn, error)
	UpdateTurn(ctx context.Context, turn *models.Turn) error
	// AbandonOpenTurns buries any open turns for a session by setting
	// completed_at = started_at (zero duration), so a subsequent prompt starts
	// a fresh turn instead of adopting one that was orphaned by a crash or
	// restart. Used on session resume.
	AbandonOpenTurns(ctx context.Context, sessionID string) error
}

// TaskEventPublisher is the orchestrator's collaborator for publishing
// task.updated events. Implemented by the task service (which owns the rich
// payload build — session counts, primary session info, repositories,
// metadata, parent_id). The orchestrator does not construct task.updated
// payloads itself.
type TaskEventPublisher interface {
	PublishTaskUpdated(ctx context.Context, task *models.Task)
	PublishTaskStateChanged(ctx context.Context, task *models.Task, oldState v1.TaskState)
}

// WorkflowStepGetter retrieves workflow step information for prompt building.
type WorkflowStepGetter interface {
	GetStep(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error)
	GetNextStepByPosition(ctx context.Context, workflowID string, currentPosition int) (*wfmodels.WorkflowStep, error)
	GetPreviousStepByPosition(ctx context.Context, workflowID string, currentPosition int) (*wfmodels.WorkflowStep, error)
	GetWorkflowAgentProfileID(ctx context.Context, workflowID string) (string, error)
}

// repoStore is the repository interface accepted by NewService.
// It covers both the orchestrator's own needs (sessionExecutorStore) and
// the executor package's needs (executor.executorStore).
type repoStore interface {
	sessionExecutorStore
	// Additional methods needed by executor
	UpdateTaskState(ctx context.Context, id string, state v1.TaskState) error
	// UpdateTaskStateIfCurrentIn — see executor.executorStore's doc comment;
	// required here because repo (this interface) is what NewService passes
	// to executor.NewExecutor.
	UpdateTaskStateIfCurrentIn(ctx context.Context, id string, state v1.TaskState, allowed []v1.TaskState) (v1.TaskState, bool, error)
	// UpdateTaskStateIfNotArchived — see executor.executorStore's doc comment;
	// required here for the same reason as UpdateTaskStateIfCurrentIn above.
	UpdateTaskStateIfNotArchived(ctx context.Context, id string, state v1.TaskState) (v1.TaskState, bool, error)
	GetPrimaryTaskRepository(ctx context.Context, taskID string) (*models.TaskRepository, error)
	ListTaskRepositories(ctx context.Context, taskID string) ([]*models.TaskRepository, error)
	CreateTaskSession(ctx context.Context, session *models.TaskSession) error
	UpdateTaskSession(ctx context.Context, session *models.TaskSession) error
	ListActiveTaskSessions(ctx context.Context) ([]*models.TaskSession, error)
	ListActiveTaskSessionsByTaskID(ctx context.Context, taskID string) ([]*models.TaskSession, error)
	CreateTaskSessionWorktree(ctx context.Context, sessionWorktree *models.TaskSessionWorktree) error
	ListTaskSessionWorktrees(ctx context.Context, sessionID string) ([]*models.TaskSessionWorktree, error)
	ListSessionsWithBranches(ctx context.Context) ([]models.SessionBranchInfo, error)
	GetRepository(ctx context.Context, id string) (*models.Repository, error)
	UpdateRepository(ctx context.Context, repository *models.Repository) error
	ListRepositories(ctx context.Context, workspaceID string) ([]*models.Repository, error)
	GetExecutorProfile(ctx context.Context, id string) (*models.ExecutorProfile, error)
	// Multi-repo task environment children
	CreateTaskEnvironmentRepo(ctx context.Context, repo *models.TaskEnvironmentRepo) error
	ListTaskEnvironmentRepos(ctx context.Context, envID string) ([]*models.TaskEnvironmentRepo, error)
	UpdateTaskEnvironmentRepo(ctx context.Context, repo *models.TaskEnvironmentRepo) error
	// Session history + plan (for context handover)
	GetTaskPlan(ctx context.Context, taskID string) (*models.TaskPlan, error)
}

// sessionExecutorStore is the minimal repository interface needed by the orchestrator service.
type sessionExecutorStore interface {
	// Session
	GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error)
	GetActiveTaskSessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error)
	ListActiveTaskSessionsByTaskID(ctx context.Context, taskID string) ([]*models.TaskSession, error)
	SetSessionPrimary(ctx context.Context, sessionID string) error
	RenameTaskSession(ctx context.Context, id, name string) error
	UpdateTaskSession(ctx context.Context, session *models.TaskSession) error
	UpdateTaskSessionIfCurrentState(ctx context.Context, session *models.TaskSession, expected models.TaskSessionState) (bool, error)
	UpdateTaskSessionState(ctx context.Context, id string, state models.TaskSessionState, errorMessage string) error
	UpdateTaskSessionBaseCommit(ctx context.Context, id string, baseCommitSHA string) error
	GetTaskSessionByTaskAndAgent(ctx context.Context, taskID, agentInstanceID string) (*models.TaskSession, error)
	UpdateTaskSessionWorktreeBranch(ctx context.Context, sessionID, branch string) error
	UpdateSessionReviewStatus(ctx context.Context, sessionID string, status string) error
	UpdateSessionWorkflowStep(ctx context.Context, sessionID, stepID string) error
	UpdateSessionMetadata(ctx context.Context, sessionID string, metadata map[string]interface{}) error
	SetSessionMetadataKey(ctx context.Context, sessionID, key string, value interface{}) error
	SetSessionMetadataKeyIfAbsent(ctx context.Context, sessionID, key string, value interface{}) (bool, error)
	SetSessionACPSessionID(ctx context.Context, sessionID, acpSessionID string) (bool, error)
	// Executor running state
	ListExecutorsRunning(ctx context.Context) ([]*models.ExecutorRunning, error)
	UpsertExecutorRunning(ctx context.Context, running *models.ExecutorRunning) error
	GetExecutorRunningBySessionID(ctx context.Context, sessionID string) (*models.ExecutorRunning, error)
	DeleteExecutorRunningBySessionID(ctx context.Context, sessionID string) error
	HasExecutorRunningRow(ctx context.Context, sessionID string) (bool, error)
	UpdateResumeToken(ctx context.Context, sessionID, expectedExecID, resumeToken, lastMessageUUID string) error
	UpdateExecutorRunningStatus(ctx context.Context, sessionID, status string) error
	// RepairExecutorRunningDead repairs a row in place (status=stopped, local_pid
	// cleared, last_seen re-stamped) while preserving resume_token/worktree — used
	// by reconciliation to honor the resume-safety invariant instead of deleting a
	// resumable row.
	RepairExecutorRunningDead(ctx context.Context, sessionID string) error
	// Executor
	GetExecutor(ctx context.Context, id string) (*models.Executor, error)
	// Task
	GetTask(ctx context.Context, id string) (*models.Task, error)
	UpdateTask(ctx context.Context, task *models.Task) error
	ListChildCompletionRows(ctx context.Context, parentID string) ([]models.ChildCompletionRow, error)
	// Git snapshots and commits
	GetLatestGitSnapshot(ctx context.Context, sessionID string) (*models.GitSnapshot, error)
	CreateGitSnapshot(ctx context.Context, snapshot *models.GitSnapshot) error
	DeleteLiveMonitorSnapshots(ctx context.Context, sessionID string) error
	UpsertLatestLiveGitSnapshot(ctx context.Context, snapshot *models.GitSnapshot) error
	CreateSessionCommit(ctx context.Context, commit *models.SessionCommit) error
	GetSessionCommits(ctx context.Context, sessionID string) ([]*models.SessionCommit, error)
	DeleteSessionCommit(ctx context.Context, id string) error
	// Session listing + delete
	ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error)
	ListNonTerminalSessionsByAgentInstance(ctx context.Context, agentInstanceID string) ([]*models.TaskSession, error)
	DeleteTaskSession(ctx context.Context, id string) error
	// Messages — used by resume to backfill the initial user prompt when a
	// prior launch failed before recordInitialMessage ran.
	ListMessages(ctx context.Context, sessionID string) ([]*models.Message, error)
	// Pending clarification rows — durable guard for on_turn_complete while the user is answering.
	FindPendingClarificationMessagesBySessionID(ctx context.Context, sessionID string) ([]*models.Message, error)
	// Workspace
	GetWorkspace(ctx context.Context, id string) (*models.Workspace, error)
	// Task environment
	GetTaskEnvironment(ctx context.Context, id string) (*models.TaskEnvironment, error)
	GetTaskEnvironmentByTaskID(ctx context.Context, taskID string) (*models.TaskEnvironment, error)
	CreateTaskEnvironment(ctx context.Context, env *models.TaskEnvironment) error
	UpdateTaskEnvironment(ctx context.Context, env *models.TaskEnvironment) error
}

// Service is the main orchestrator service
type Service struct {
	config       ServiceConfig
	logger       *logger.Logger
	eventBus     bus.EventBus
	taskRepo     scheduler.TaskRepository
	repo         sessionExecutorStore
	agentManager executor.AgentManagerClient

	// Components
	queue     *queue.TaskQueue
	executor  *executor.Executor
	scheduler *scheduler.Scheduler
	watcher   *watcher.Watcher

	// Message queue service for queueing messages while agent is running
	messageQueue *messagequeue.Service

	// Message creator for saving agent responses
	messageCreator MessageCreator

	// Turn service for managing session turns
	turnService TurnService

	// Task event publisher for emitting task.updated events.
	// Task service owns the rich payload; orchestrator delegates.
	taskEvents TaskEventPublisher

	// Workflow step getter for prompt building
	workflowStepGetter WorkflowStepGetter

	// Workflow engine for typed state-machine evaluation of step transitions
	workflowEngine *engine.Engine
	workflowStore  *workflowStore
	// childCompletionLocks serializes duplicate on_children_completed deliveries.
	childCompletionLocksMu sync.Mutex
	childCompletionLocks   map[string]*childCompletionOperationLock
	// onProcessOnEnterComplete is a package-test hook for synchronizing with
	// applyEngineTransition's asynchronous processOnEnter goroutine.
	onProcessOnEnterComplete func()
	// engineOptions are applied each time initWorkflowEngine runs. Wired
	// from cmd/kandev (Phase 3.2) to plug Phase 2 ADR-0004 dependencies
	// — RunQueueAdapter, ParticipantStore, DecisionStore, and the CEO /
	// Primary agent resolvers — without coupling the orchestrator to
	// the office or runs packages.
	engineOptions []engine.Option

	// Phase 2 (ADR-0004) callback dependencies, set via dedicated
	// setters from cmd/kandev. When set, buildWorkflowCallbacks
	// registers the queue_run / clear_decisions /
	// queue_run_for_each_participant callbacks with these
	// dependencies. Nil-safe: missing deps simply skip registration.
	engineRunQueue     engine.RunQueueAdapter
	engineParticipants engine.ParticipantStore
	engineDecisions    engine.DecisionStore
	enginePrimary      engine.PrimaryAgentResolver
	engineCEOResolver  engine.CEOAgentResolver
	// Phase 8 dependencies — also nil-safe.
	engineTaskCreator      engine.TaskCreator
	engineWorkflowSwitcher engine.WorkflowSwitcher

	// GitHub service for PR auto-detection on push
	githubService GitHubService
	// ciAutomationInFlight prevents PR feedback and task-PR update events from
	// racing duplicate auto-fix prompts or merge calls for the same PR.
	ciAutomationInFlight sync.Map

	// Office task-handoffs materializer (phase 6 wiring) — invoked from
	// PrepareTaskSession to flip workspace groups to materialized once
	// their owner task has a session with worktree details. Optional;
	// nil-safe.
	workspaceMaterializer WorkspaceMaterializer

	// Review task creator for auto-creating tasks from review watch PRs
	reviewTaskCreator ReviewTaskCreator

	// Issue task creator for auto-creating tasks from issue watch events
	issueTaskCreator IssueTaskCreator

	// Watcher dispatch coordinator: single seam for the watcher→task pipeline.
	// Constructed lazily once issueTaskCreator is wired (see SetIssueTaskCreator).
	watcherCoordinator *WatcherDispatchCoordinator

	// Watcher throttle state. watcherTaskCount counts committed open watcher-
	// created tasks (DB-backed source of truth across restarts). pendingByWatch
	// tracks in-process events whose dedup row has not yet been written —
	// without it, a burst of events read the same stale COUNT(*) before any
	// goroutine commits, and the cap is silently overshot. Both reads and the
	// pending increment happen under watcherMu to prevent the burst race.
	watcherTaskCount WatcherTaskCounter
	watcherMu        sync.Mutex
	pendingByWatch   map[string]int
	// watcherSaturated tracks per-watch whether the last gate result was
	// "deferred", so we log the state-transition Warn ("cap reached" /
	// "cap cleared") only once per transition instead of every event.
	watcherSaturated map[string]bool

	// profileLookup answers "is this agent profile still live?" for the
	// dispatch pre-flight. Set via SetProfileLookup from main; nil-safe so
	// the legacy code path (and tests without profile wiring) keep working.
	profileLookup ProfileLookup
	// repoChecker answers "does this bound repository still exist?" for the
	// dispatch pre-flight. Set via SetRepositoryChecker from main; nil-safe.
	repoChecker RepositoryChecker
	// modelInfoLookup resolves optional model metadata from models.dev. Nil-safe;
	// ACP context-window events remain authoritative when they include a size.
	modelInfoMu           sync.RWMutex
	modelInfoLookup       ModelInfoLookup
	runtimeModelBySession sync.Map

	// Jira service for issue watch dedup operations
	jiraService JiraService
	// jiraSource adapts jiraService onto WatcherSource. Built once in
	// SetJiraService so handlers don't allocate per bus event.
	jiraSource *JiraWatcherSource

	// Linear service for issue watch dedup operations
	linearService LinearService
	// linearSource adapts linearService onto WatcherSource. Built once in
	// SetLinearService so handlers don't allocate per bus event.
	linearSource *LinearWatcherSource

	// Sentry service for issue watch dedup operations
	sentryService SentryService
	// sentrySource adapts sentryService onto WatcherSource. Built once in
	// SetSentryService so handlers don't allocate per bus event.
	sentrySource *SentryWatcherSource
	// GitLab service + task creators for auto-creating tasks from review /
	// issue watch events. When the task creators are nil the events are
	// logged but no tasks are created — matches the GitHub flow when a
	// workspace has no task creator wired.
	gitlabService           *gitlab.Service
	gitlabReviewTaskCreator GitLabReviewTaskCreator
	gitlabIssueTaskCreator  GitLabIssueTaskCreator

	// Repository resolver for cloning + finding/creating repos for review tasks
	repositoryResolver RepositoryResolver

	// Automation service for handling automation triggers
	automationService AutomationService

	// Worktree manager — used to clean up ephemeral worktrees for run-mode
	// automation tasks immediately on completion rather than waiting for
	// the 24h Office GC. Nil-safe.
	worktreeMgr *worktree.Manager

	// Clarification canceller — cancels pending clarifications when agent's turn completes
	clarificationCanceller ClarificationCanceller

	// Push tracker: "<sessionID>|<repository_name>" -> last known ahead count.
	// The repository_name segment is required for multi-repo: each repo emits
	// its own git status events, and keying by sessionID alone made events
	// from different repos overwrite each other's ahead counts (so only one
	// push got detected per session). Single-repo / repo-less sessions key
	// against an empty repository_name.
	pushTracker sync.Map

	// gitSnapshotCache throttles per-session writes of the live git status
	// snapshot (used by handleGitStatusUpdate -> persistGitStatusSnapshot).
	gitSnapshotCache *gitSnapshotCache

	// Active turns map: sessionID -> turnID
	activeTurns sync.Map

	// dispatchingQueued tracks, per session, the entry ID of whichever
	// queued message was most recently taken and handed off to the async
	// executeQueuedMessage goroutine, but hasn't yet reached promptTask's
	// own setSessionRunning call — the only point at which session.State
	// itself starts correctly reporting "busy" for that dispatch. Without
	// this, a second take-and-dispatch decision for the same session (a
	// workflow drain, a manual drain, or another parent interrupt) could
	// acquire the cancelInFlight guard in that gap, see the still-idle
	// session.State, and dispatch a second, unrelated queued entry before
	// the first dispatch's turn has even started.
	//
	// Stores the entry ID (string), not a bool: a newer dispatch for the
	// same session (e.g. a second parent interrupt cancelling and
	// re-taking) always supersedes an older one that's still settling —
	// markQueuedDispatchInFlight unconditionally overwrites. Each
	// goroutine must reconfirm it still owns *its own* entry ID
	// (isCurrentQueuedDispatch) immediately before calling
	// setSessionRunning, and may only clear the marker via a
	// compare-and-delete keyed on that same entry ID
	// (clearQueuedDispatchInFlightIfCurrent) — otherwise a superseded
	// goroutine finishing late could delete a newer dispatch's marker out
	// from under it, or proceed to prompt after it no longer owns the
	// session. Non-cancelling take-and-dispatch paths
	// (drainQueuedMessageForPromptableSessionLocked, takeIfPromptableLocked)
	// instead use isQueuedDispatchInFlight, which only asks "is *anything*
	// still settling" — they have no cancel to supersede it with, so any
	// in-flight dispatch at all is reason enough to defer.
	dispatchingQueued sync.Map

	// taskRuntimeStateMu serializes task-state flips derived from session
	// runtime state. Without it, a completion/cancel path can check for active
	// sibling sessions just before another handler marks one RUNNING, then
	// clobber the task back to REVIEW while work is active.
	taskRuntimeStateMu sync.Mutex

	// completedExecutions records execution IDs that have reached a terminal
	// agent lifecycle event. Buffered stream/tool events for these executions
	// must not wake their session back to RUNNING after the terminal path makes
	// it promptable again. Entries expire after a short grace window so the
	// guard does not grow without bound in long-running backend processes.
	completedExecutions sync.Map

	// executionTeardownClaims arbitrates detached runtime teardown by
	// "<session_id>::<execution_id>". Coordinator stop requests graceful
	// teardown while terminal-event cleanup requests force teardown; the first
	// intent accepted under the session's cancelInFlight guard owns that
	// execution. Claims expire with the same bounded grace period used for
	// completed-execution stream markers.
	executionTeardownClaims sync.Map

	// Session reset flags: sessionID -> true while resetAgentContext is restarting process.
	// Used to suppress stale ready events and avoid draining queued prompts mid-reset.
	resetInProgressSessions sync.Map

	// suppressToast: sessionID -> true. Set by failure handlers that create
	// guidance messages with actions. Cleared by updateTaskSessionState which
	// adds suppress_toast to the WS event so the frontend skips the error toast.
	suppressToast sync.Map

	// clarificationWatchdogs tracks active clarification primary-path resume watchdogs.
	// key: "<session_id>::<pending_id>", value: *clarificationWatchdogEntry
	clarificationWatchdogs sync.Map

	// clarificationWatchdogTimeout controls how long to wait for post-clarification
	// activity before triggering fallback resume.
	clarificationWatchdogTimeout time.Duration

	// cancelInFlightMu guards cancelInFlight's map structure (insert/
	// lookup/remove of *cancelInFlightGuard entries) — held only briefly
	// for that bookkeeping, never across a caller's actual per-session
	// critical section (which is guarded by the entry's own mutex, handed
	// to callers by acquireCancelInFlightGuard).
	cancelInFlightMu sync.Mutex
	// cancelInFlight holds one *cancelInFlightGuard per session with an
	// active reference — an in-progress or waiting claim from CancelAgent
	// (the user cancel button — TryLock, dedup impatient retries), the
	// natural turn-completion/boot-ready drain decision in handleAgentReady
	// / handleAgentBootReady (TryLock, skip if a cancel/interrupt owns it),
	// or QueueAndInterruptForPeerMessage (blocking Lock — must wait rather
	// than work around a busy lock with an unguarded insert; see its doc
	// comment). All of these must go through the same per-session guard —
	// a second, independent lock for any of them would defeat the mutual
	// exclusion the others rely on to avoid racing each other's
	// take-and-dispatch decision for the same session.
	//
	// Entries are reference-counted (acquireCancelInFlightGuard /
	// releaseCancelInFlightGuard) and pruned once nobody holds a reference,
	// so this stays bounded by concurrently-active sessions rather than
	// growing by one permanent entry per session ever created over a
	// long-lived backend's lifetime.
	cancelInFlight map[string]*cancelInFlightGuard

	// transientRetries tracks in-progress transient-provider-error (529
	// Overloaded) retry loops. key: sessionID, value: *transientRetryEntry.
	// A backoff timer per session re-drives the failed prompt; cancelled on
	// success, user-cancel, or service shutdown.
	transientRetries sync.Map

	// lastTurnPrompt caches the most recent outbound prompt per session so a
	// transient-failure retry can re-drive the same turn without the caller's
	// context. key: sessionID, value: capturedPrompt. Replaced every turn.
	lastTurnPrompt sync.Map

	// Service state
	mu        sync.RWMutex
	running   bool
	startedAt time.Time
}

// Status contains orchestrator status information
type Status struct {
	Running        bool      `json:"running"`
	ActiveAgents   int       `json:"active_agents"`
	QueuedTasks    int       `json:"queued_tasks"`
	TotalProcessed int64     `json:"total_processed"`
	TotalFailed    int64     `json:"total_failed"`
	UptimeSeconds  int64     `json:"uptime_seconds"`
	LastHeartbeat  time.Time `json:"last_heartbeat"`
}

// NewService creates a new orchestrator service. msgQueue is the persistent
// message queue service (constructed externally so cmd/kandev can wire it to
// the shared SQLite pool); pass nil to default to an in-memory queue, which
// suits tests but loses entries on restart.
func NewService(
	cfg ServiceConfig,
	eventBus bus.EventBus,
	agentManager executor.AgentManagerClient,
	taskRepo scheduler.TaskRepository,
	repo repoStore,
	shellPrefs executor.ShellPreferenceProvider,
	secretStore secrets.SecretStore,
	msgQueue *messagequeue.Service,
	log *logger.Logger,
) *Service {
	svcLogger := log.WithFields(zap.String("component", "orchestrator"))

	// Create the task queue with configured size
	taskQueue := queue.NewTaskQueue(cfg.QueueSize)

	// Create the executor with the agent manager client and repository for persistent sessions
	execCfg := executor.ExecutorConfig{
		ShellPrefs:  shellPrefs,
		SecretStore: secretStore,
	}
	exec := executor.NewExecutor(agentManager, repo, log, execCfg)

	// Create the scheduler with queue, executor, and task repository
	sched := scheduler.NewScheduler(taskQueue, exec, taskRepo, log, cfg.Scheduler)

	if msgQueue == nil {
		msgQueue = messagequeue.NewServiceMemory(log)
	}

	// Create the service (watcher will be created after we have handlers)
	s := &Service{
		config:                       cfg,
		logger:                       svcLogger,
		eventBus:                     eventBus,
		taskRepo:                     taskRepo,
		repo:                         repo,
		agentManager:                 agentManager,
		queue:                        taskQueue,
		executor:                     exec,
		scheduler:                    sched,
		messageQueue:                 msgQueue,
		clarificationWatchdogTimeout: 15 * time.Second,
		gitSnapshotCache:             newGitSnapshotCache(),
	}

	// Wire executor state changes through the orchestrator so events are published
	// (e.g. WebSocket notifications to the frontend). Must be set after service
	// construction so the session callback can reference s.updateTaskSessionState.
	// UpdateTaskStateIfNotArchived (not the unconditional UpdateTaskState) so
	// every runtime-driven task-state write the executor makes through this
	// single callback — IN_PROGRESS on agent start/resume, FAILED on launch
	// error — is atomic against archived_at. Each caller's own archived
	// guard (e.g. writeTaskInProgressForRuntime) reads the task before this
	// fires; ArchiveTask can commit in that window without an intervening
	// state write to invalidate the guard, so only this conditional write
	// closes the race (PR #1706 review).
	exec.SetOnTaskStateChange(func(ctx context.Context, taskID string, state v1.TaskState) error {
		updated, err := taskRepo.UpdateTaskStateIfNotArchived(ctx, taskID, state)
		if err != nil {
			return err
		}
		if !updated {
			s.logger.Debug("skipping runtime task state write for archived task",
				zap.String("task_id", taskID),
				zap.String("state", string(state)))
			return nil
		}
		s.processParentChildrenCompletedForTaskState(ctx, taskID, state)
		return nil
	})
	exec.SetOnTaskRuntimeStateReconcile(s.reconcileTaskStateForRuntime)
	exec.SetOnSessionStateChange(func(ctx context.Context, taskID, sessionID string, state models.TaskSessionState, errorMessage string) error {
		s.updateTaskSessionState(ctx, taskID, sessionID, state, errorMessage, true)
		return nil
	})
	exec.SetOnSessionStateTransition(s.transitionTaskSessionState)
	exec.SetOnSessionStarting(func(ctx context.Context, taskID string, session *models.TaskSession, promoteTask bool) error {
		return s.setSessionStarting(ctx, taskID, session, promoteTask)
	})
	exec.SetOnExecutionCleanupClaim(s.claimForcedExecutionCleanup)
	exec.SetOnExecutionStopOwnerRegistration(s.RegisterExecutionStopOwner)
	exec.SetOnTaskReviewStateReconcile(func(ctx context.Context, taskID, completedSessionID string) {
		s.writeTaskReviewState(ctx, taskID, completedSessionID)
	})
	exec.SetOnLaunchFailed(s.handleSessionLaunchFailed)
	exec.SetOnAgentStartFailed(s.handleAgentStartFailed)
	if caps, ok := agentManager.(executor.ExecutorTypeCapabilities); ok {
		exec.SetCapabilities(caps)
	}

	// Create the watcher with event handlers that wire everything together
	handlers := watcher.EventHandlers{
		OnTaskDeleted:          s.handleTaskDeleted,
		OnTaskStateChanged:     s.handleTaskStateChanged,
		OnAgentRunning:         s.handleAgentRunning,
		OnAgentBootReady:       s.handleAgentBootReady,
		OnAgentReady:           s.handleAgentReady,
		OnAgentCompleted:       s.handleAgentCompleted,
		OnAgentFailed:          s.handleAgentFailed,
		OnAgentStopped:         s.handleAgentStopped,
		OnAgentStreamEvent:     s.handleAgentStreamEvent,
		OnACPSessionCreated:    s.handleACPSessionCreated,
		OnPermissionRequest:    s.handlePermissionRequest,
		OnGitEvent:             s.handleGitEvent,
		OnContextWindowUpdated: s.handleContextWindowUpdated,
		OnTaskMoved:            s.handleTaskMoved,
	}
	s.watcher = watcher.NewWatcher(eventBus, handlers, cfg.QueueGroup, log)

	return s
}

// SetMessageCreator sets the message creator for saving agent responses to the database.
//
// This must be called before starting the orchestrator if you want agent messages, tool calls,
// and streaming content to be persisted to the database. The MessageCreator interface provides
// methods for creating and updating messages associated with task sessions.
//
// The MessageCreator is typically the task service, which owns the message persistence logic.
// Event handlers in the orchestrator call these methods when agent events occur:
//   - AgentStreamEvent → CreateAgentMessage, AppendAgentMessage
//   - Tool calls → CreateToolCallMessage, UpdateToolCallMessage
//   - Permission requests → CreatePermissionRequestMessage
//
// When to call: During orchestrator initialization, after creating the task service.
//
// If not set: Agent messages won't be saved to the database (events will still be published).
func (s *Service) SetMessageCreator(mc MessageCreator) {
	s.messageCreator = mc
}

// SetOnPrimarySessionSet sets a callback on the executor for when the first session
// of a task is marked primary. Used to publish a task.updated event so the frontend
// receives the primary_session_id.
func (s *Service) SetOnPrimarySessionSet(fn executor.PrimarySessionSetFunc) {
	s.executor.SetOnPrimarySessionSet(fn)
}

// SetRepoCloner sets the repository cloner and updater on the executor, enabling automatic
// cloning of provider-backed repositories (e.g. from a GitHub URL) when they are launched
// for local/worktree execution and have no local path yet.
func (s *Service) SetRepoCloner(cloner executor.RepoCloner, updater executor.RepoUpdater) {
	s.executor.SetRepoCloner(cloner, updater)
}

// SetTurnService sets the turn service for tracking conversation turns.
//
// A "turn" represents a single conversation round-trip: user prompt → agent response.
// The TurnService tracks turn timing and duration for analytics and UI display (e.g., showing
// how long each agent response took).
//
// The TurnService is typically the task service, which owns turn persistence logic.
// The orchestrator calls these methods:
//   - StartTurn: When agent begins processing a prompt
//   - CompleteTurn: When agent finishes and returns to ready state
//   - GetActiveTurn: To associate messages with current turn
//
// When to call: During orchestrator initialization, after creating the task service.
//
// If not set: Turns won't be tracked (orchestrator continues functioning normally, but
// no timing data is recorded and turn IDs in messages will be empty).
func (s *Service) SetTurnService(turnService TurnService) {
	s.turnService = turnService
}

// SetTaskEventPublisher wires the publisher used for task.updated events.
//
// The task service is the canonical publisher: it loads session counts,
// primary session info, repositories, metadata, and parent_id, and emits a
// single rich payload. Orchestrator code paths that mutate a task (workflow
// transitions, primary session assignment, workflow-step moves) delegate to
// this publisher instead of constructing their own partial payloads.
//
// If not set: orchestrator task.updated publishers no-op (nothing is emitted
// on those paths). Task service's own publishTaskEvent calls are unaffected.
func (s *Service) SetTaskEventPublisher(publisher TaskEventPublisher) {
	s.taskEvents = publisher
}

// publishTaskUpdated forwards to the configured TaskEventPublisher.
// No-op when the publisher isn't wired (tests, or before SetTaskEventPublisher
// has been called during startup).
func (s *Service) publishTaskUpdated(ctx context.Context, task *models.Task) {
	if s.taskEvents == nil || task == nil {
		return
	}
	s.taskEvents.PublishTaskUpdated(ctx, task)
}

func (s *Service) publishTaskStateChanged(ctx context.Context, task *models.Task, oldState v1.TaskState) {
	if s.taskEvents == nil || task == nil {
		return
	}
	s.taskEvents.PublishTaskStateChanged(ctx, task, oldState)
}

func (s *Service) publishTaskMoved(ctx context.Context, task *models.Task, fromWorkflowID, fromStepID, toStepID, sessionID string) {
	if s.eventBus == nil || task == nil {
		return
	}
	data := map[string]interface{}{
		"task_id":                   task.ID,
		"from_workflow_id":          fromWorkflowID,
		"to_workflow_id":            task.WorkflowID,
		"from_step_id":              fromStepID,
		"to_step_id":                toStepID,
		"session_id":                sessionID,
		"workflow_id":               task.WorkflowID,
		"task_description":          task.Description,
		"parent_id":                 task.ParentID,
		"assignee_agent_profile_id": task.AssigneeAgentProfileID,
	}
	event := bus.NewEvent(events.TaskMoved, "orchestrator", data)
	if err := s.eventBus.Publish(ctx, events.TaskMoved, event); err != nil {
		s.logger.Error("failed to publish pulled task.moved event",
			zap.String("task_id", task.ID), zap.Error(err))
	}
}

// StepRequiresCompletionSignal reports whether the workflow step bound to taskID
// has `auto_advance_requires_signal = true` (ADR 0015). Used by sysprompt
// injection sites to decide whether to expose the `step_complete_kandev` MCP
// tool to the agent. Returns false (without error) when the getter is unset,
// the task has no workflow step, or the step lookup fails — the caller treats
// "unknown" the same as "no signal required" so a flaky workflow lookup never
// silently enables the tool.
//
// Call sites that already have the task or step ID in scope should prefer
// [WorkflowStepRequiresCompletionSignal] to skip the extra GetTask round-trip.
func (s *Service) StepRequiresCompletionSignal(ctx context.Context, taskID string) bool {
	if s.workflowStepGetter == nil {
		return false
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return false
	}
	return s.WorkflowStepRequiresCompletionSignal(ctx, task.WorkflowStepID)
}

// WorkflowStepRequiresCompletionSignal reports whether the given workflow step
// has `auto_advance_requires_signal = true` (ADR 0015). Cheaper alternative to
// [StepRequiresCompletionSignal] for callers that already loaded the task and
// can pass `task.WorkflowStepID` directly — avoids a redundant GetTask round-trip
// at hot first-turn launch sites. Same "unknown ⇒ false" contract.
func (s *Service) WorkflowStepRequiresCompletionSignal(ctx context.Context, stepID string) bool {
	if s.workflowStepGetter == nil || stepID == "" {
		return false
	}
	step, err := s.workflowStepGetter.GetStep(ctx, stepID)
	if err != nil || step == nil {
		return false
	}
	return step.AutoAdvanceRequiresSignal
}

// SetWorkflowStepGetter sets the workflow step getter for prompt building.
//
// When workflow_step_id is provided to StartTask, the orchestrator uses this getter
// to retrieve the step's prompt_prefix, prompt_suffix, and plan_mode settings to
// build the effective prompt.
//
// If not set: workflow_step_id in StartTask is ignored and the prompt is used as-is.
func (s *Service) SetWorkflowStepGetter(getter WorkflowStepGetter) {
	s.workflowStepGetter = getter
	s.initWorkflowEngine()
}

// ClarificationCanceller detaches in-memory clarification waiters when an agent's
// turn completes while questions are still pending in the DB.
type ClarificationCanceller interface {
	DetachSessionAndNotify(ctx context.Context, sessionID string) int
}

// SetClarificationCanceller sets the canceller for cleaning up pending clarifications on turn complete.
func (s *Service) SetClarificationCanceller(c ClarificationCanceller) {
	s.clarificationCanceller = c
}

// initWorkflowEngine creates the workflow engine with store and callbacks.
// Called after the workflow step getter is set. Safe to call multiple times —
// previously-applied engine options are re-applied on reinit.
func (s *Service) initWorkflowEngine() {
	if s.workflowStepGetter == nil {
		return
	}
	store := newWorkflowStore(s.repo, s.workflowStepGetter, s.agentManager, s.publishTaskUpdated, s.logger, s.publishTaskMoved)
	callbacks := buildWorkflowCallbacks(s)
	s.workflowStore = store
	s.workflowEngine = engine.New(store, callbacks, s.engineOptions...)
}

// SetEngineRunQueue wires the engine's RunQueueAdapter dependency. Used
// both as an engine option (so the engine can resolve queue_run targets)
// and as a callback dependency (so QueueRunCallback can enqueue runs).
func (s *Service) SetEngineRunQueue(adapter engine.RunQueueAdapter) {
	s.engineRunQueue = adapter
	s.engineOptions = append(s.engineOptions, engine.WithRunQueue(adapter))
	s.reinitWorkflowEngine()
}

// SetEngineParticipantStore wires the engine's ParticipantStore.
func (s *Service) SetEngineParticipantStore(store engine.ParticipantStore) {
	s.engineParticipants = store
	s.engineOptions = append(s.engineOptions, engine.WithParticipantStore(store))
	s.reinitWorkflowEngine()
}

// SetEngineDecisionStore wires the engine's DecisionStore.
func (s *Service) SetEngineDecisionStore(store engine.DecisionStore) {
	s.engineDecisions = store
	s.engineOptions = append(s.engineOptions, engine.WithDecisionStore(store))
	s.reinitWorkflowEngine()
}

// SetEngineCEOResolver wires the engine's CEOAgentResolver.
func (s *Service) SetEngineCEOResolver(resolver engine.CEOAgentResolver) {
	s.engineCEOResolver = resolver
	s.engineOptions = append(s.engineOptions, engine.WithCEOAgentResolver(resolver))
	s.reinitWorkflowEngine()
}

// SetPrimaryAgentResolver wires the engine's PrimaryAgentResolver. The
// engine has no constructor option for this — the resolver is consumed
// by QueueRunCallback at registration time, so the orchestrator
// re-runs initWorkflowEngine to pick it up.
func (s *Service) SetPrimaryAgentResolver(resolver engine.PrimaryAgentResolver) {
	s.enginePrimary = resolver
	s.reinitWorkflowEngine()
}

// SetEngineTaskCreator wires the engine's TaskCreator dependency for
// the create_child_task action. The orchestrator captures it both as an
// engine option and as a callback dependency.
func (s *Service) SetEngineTaskCreator(creator engine.TaskCreator) {
	s.engineTaskCreator = creator
	s.engineOptions = append(s.engineOptions, engine.WithTaskCreator(creator))
	s.reinitWorkflowEngine()
}

// SetEngineWorkflowSwitcher wires the engine's WorkflowSwitcher for the
// switch_workflow action.
func (s *Service) SetEngineWorkflowSwitcher(switcher engine.WorkflowSwitcher) {
	s.engineWorkflowSwitcher = switcher
	s.engineOptions = append(s.engineOptions, engine.WithWorkflowSwitcher(switcher))
	s.reinitWorkflowEngine()
}

func (s *Service) reinitWorkflowEngine() {
	if s.workflowStepGetter != nil {
		s.initWorkflowEngine()
	}
}

// WorkflowEngine returns the workflow engine, or nil if it has not been
// initialised yet (the workflow step getter must be set first).
//
// Exposed for cross-package wiring (e.g. office service plumbing the
// engine into its dispatcher). The engine itself is safe for concurrent
// HandleTrigger calls.
func (s *Service) WorkflowEngine() *engine.Engine {
	return s.workflowEngine
}

// startTurnForSession ensures the session has an active turn and returns its ID.
//
// Idempotent: if an open turn already exists (either tracked in activeTurns or
// only present in the DB — the latter happens when service.CreateMessage lazily
// started a turn for an inbound user message before the orchestrator's prompt
// cycle began, or when the backend restarted with open turns in the DB), it is
// adopted rather than duplicated. A new turn is created only when none exists.
//
// This avoids the classic dual-creation leak: user message → service.CreateMessage
// starts turn X (DB only) → PromptTask → startTurnForSession → would create turn Y
// (DB + activeTurns), leaving X open forever because nothing tracks it.
func (s *Service) startTurnForSession(ctx context.Context, sessionID string) string {
	if s.turnService == nil {
		return ""
	}

	if turnIDVal, ok := s.activeTurns.Load(sessionID); ok {
		if turnID, ok := turnIDVal.(string); ok && turnID != "" {
			return turnID
		}
	}

	if turn, err := s.turnService.GetActiveTurn(ctx, sessionID); turn != nil {
		s.activeTurns.Store(sessionID, turn.ID)
		return turn.ID
	} else if err != nil {
		// A real DB read failure here would otherwise be silently dropped, and
		// we'd fall through to StartTurn — potentially writing a duplicate next
		// to an existing open turn we couldn't see. Log it; the next sweep via
		// completeTurnForSession will mop up any duplicate.
		s.logger.Warn("failed to look up active turn before starting a new one",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}

	turn, err := s.turnService.StartTurn(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to start turn",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return ""
	}

	s.activeTurns.Store(sessionID, turn.ID)
	return turn.ID
}

// completeTurnForSession closes any open turn for the session.
//
// The DB is the source of truth — activeTurns is just an in-memory cache that
// can drift (see startTurnForSession) or be wiped by a backend restart. We
// query the DB for any open turn and close it. Loops to mop up multiple
// zombies (e.g. left over from before this fix) with a small sanity bound.
func (s *Service) completeTurnForSession(ctx context.Context, sessionID string) {
	if s.turnService == nil {
		return
	}

	s.activeTurns.Delete(sessionID)

	const maxIterations = 16
	closed := 0
	for closed < maxIterations {
		turn, err := s.turnService.GetActiveTurn(ctx, sessionID)
		if err != nil {
			s.logger.Warn("failed to look up active turn",
				zap.String("session_id", sessionID),
				zap.Error(err))
			return
		}
		if turn == nil {
			return
		}
		if err := s.turnService.CompleteTurn(ctx, turn.ID); err != nil {
			// GetActiveTurn returns the latest open turn — retrying here
			// would just hit the same row and loop. Bail; the next
			// completeTurnForSession call will pick it up.
			s.logger.Warn("failed to complete turn; will retry on next sweep",
				zap.String("session_id", sessionID),
				zap.String("turn_id", turn.ID),
				zap.Error(err))
			return
		}
		closed++
	}
	// Only warn if turns are *still* accumulating after the cap. Closing
	// exactly maxIterations turns and then finding the session clean is not a
	// runaway.
	if turn, err := s.turnService.GetActiveTurn(ctx, sessionID); err == nil && turn != nil {
		s.logger.Warn("completeTurnForSession iteration cap hit; possible turn close loop",
			zap.String("session_id", sessionID),
			zap.Int("max_iterations", maxIterations))
	}
}

// getActiveTurnID returns the active turn ID for a session.
// If no active turn exists and the session ID is provided, it will start a new turn.
// This ensures messages always have a valid turn ID even in edge cases like resumed sessions.
func (s *Service) getActiveTurnID(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	turnIDVal, ok := s.activeTurns.Load(sessionID)
	if ok {
		turnID, _ := turnIDVal.(string)
		if turnID != "" {
			return turnID
		}
	}
	// No active turn exists - start one lazily
	// This handles edge cases like resumed sessions or race conditions
	return s.startTurnForSession(context.Background(), sessionID)
}

// peekActiveTurnID returns sessionID's currently active turn ID, or "" if
// none, WITHOUT creating one when none exists — unlike getActiveTurnID,
// this never calls startTurnForSession. Used by QueueAndInterruptForPeerMessage
// and handleAgentReady to snapshot "which turn is this?" before contending
// for the per-session cancelInFlight guard, then re-check it once the guard
// is held to detect whether a *different* turn has started in the
// meantime (e.g. a workflow transition auto-started a successor while the
// caller waited) — see their doc comments for the races this closes.
//
// Deliberately reads through to turnService.GetActiveTurn (the DB) rather
// than only the activeTurns in-memory cache: completeTurnForSession's own
// doc comment notes the cache can drift or miss turns that exist only in
// the DB (e.g. a lazily-started turn from service.CreateMessage), and
// missing a genuinely-active successor here would defeat the safety check
// callers rely on this for. Returns ("", nil) when turnService is not
// wired — callers must treat that as "no turn identity can be
// established" and fall back to their pre-existing (turn-unaware)
// behavior, not as a confirmed empty turn.
func (s *Service) peekActiveTurnID(ctx context.Context, sessionID string) (string, error) {
	if sessionID == "" || s.turnService == nil {
		return "", nil
	}
	turn, err := s.turnService.GetActiveTurn(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if turn == nil {
		return "", nil
	}
	return turn.ID, nil
}

// markQueuedDispatchInFlight records that entryID — a specific queued
// message — has been taken and handed off to the async
// executeQueuedMessage goroutine for sessionID. Unconditionally
// overwrites any previous entry for the session: a newer dispatch always
// supersedes an older one that's still settling. See the
// Service.dispatchingQueued field doc comment for why this exists:
// session.State alone doesn't reflect "busy" until that goroutine's own
// promptTask call reaches setSessionRunning, several DB round-trips
// later — and for why the token must be the specific entry ID, not a bare
// bool.
func (s *Service) markQueuedDispatchInFlight(sessionID, entryID string) {
	if sessionID == "" {
		return
	}
	s.dispatchingQueued.Store(sessionID, entryID)
}

// clearQueuedDispatchInFlightIfCurrent removes sessionID's in-flight
// marker only if it still names entryID — a compare-and-delete so a
// goroutine whose own dispatch has since been superseded by a newer one
// (a different entryID overwrote the marker) can never clear the
// *newer* dispatch's marker out from under it. Called by
// executeQueuedMessage via defer so the marker is cleared on every exit
// path (success, transient requeue, superseded, or lost/dropped message)
// — but only when this goroutine is still the current owner.
func (s *Service) clearQueuedDispatchInFlightIfCurrent(sessionID, entryID string) {
	if sessionID == "" {
		return
	}
	s.dispatchingQueued.CompareAndDelete(sessionID, entryID)
}

// isCurrentQueuedDispatch reports whether entryID is still the most
// recently handed-off in-flight dispatch for sessionID — i.e. no *other*
// dispatch has superseded it since markQueuedDispatchInFlight was called
// for it. A goroutine that owns entryID must confirm this immediately
// before calling setSessionRunning (see promptTask's claimDispatch
// parameter); if it no longer owns the token, a different dispatch has
// already taken over the session and this one must not proceed.
func (s *Service) isCurrentQueuedDispatch(sessionID, entryID string) bool {
	if sessionID == "" {
		return false
	}
	v, ok := s.dispatchingQueued.Load(sessionID)
	if !ok {
		return false
	}
	current, _ := v.(string)
	return current == entryID
}

// isQueuedDispatchInFlight reports whether *any* queued message is
// currently in the handoff window for sessionID, regardless of which one
// — see markQueuedDispatchInFlight. Checked by
// drainQueuedMessageForPromptableSessionLocked and takeIfPromptableLocked
// before either takes and directly dispatches another entry on the same
// session without a cancel to supersede whatever is already settling; a
// genuine cancel (cancelAndTakeForPeerMessage) is exempt — see its own
// doc comment for why it may proceed regardless and simply overwrite the
// token via markQueuedDispatchInFlight.
func (s *Service) isQueuedDispatchInFlight(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	_, ok := s.dispatchingQueued.Load(sessionID)
	return ok
}

func (s *Service) setSessionResetInProgress(sessionID string, inProgress bool) {
	if sessionID == "" {
		return
	}
	if inProgress {
		s.resetInProgressSessions.Store(sessionID, true)
		return
	}
	s.resetInProgressSessions.Delete(sessionID)
}

func (s *Service) isSessionResetInProgress(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	v, ok := s.resetInProgressSessions.Load(sessionID)
	if !ok {
		return false
	}
	inProgress, _ := v.(bool)
	return inProgress
}

// Start starts all orchestrator components
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return ErrServiceAlreadyRunning
	}
	s.running = true
	s.startedAt = time.Now()
	s.mu.Unlock()

	s.logger.Info("starting orchestrator service")

	// Reconcile session state from persisted runtime state on startup.
	// This does NOT launch any agent processes — sessions are recovered lazily
	// when the user opens them (via task.session.status → task.session.resume).
	s.reconcileSessionsOnStartup(ctx)

	// Start the watcher first to begin receiving events
	if err := s.watcher.Start(ctx); err != nil {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		return err
	}

	// Start the scheduler processing loop
	if err := s.scheduler.Start(ctx); err != nil {
		if stopErr := s.watcher.Stop(); stopErr != nil {
			s.logger.Warn("failed to stop watcher after scheduler start failure", zap.Error(stopErr))
		}
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		return err
	}

	// Subscribe to GitHub integration events
	s.subscribeGitHubEvents()

	// Subscribe to GitLab integration events
	s.subscribeGitLabEvents()

	// Subscribe to JIRA integration events
	s.subscribeJiraEvents()

	// Subscribe to Linear integration events
	s.subscribeLinearEvents()

	// Subscribe to Sentry integration events
	s.subscribeSentryEvents()

	// Subscribe to automation events
	s.subscribeAutomationEvents()

	// Subscribe to clarification events (cancel-and-resume flow)
	s.subscribeClarificationEvents()

	// Subscribe to prepare events (persist result in session metadata)
	s.subscribePrepareEvents()

	// Subscribe to ADR-0015 step-completion signals (out-of-band path:
	// signal arrives after turn-end).
	s.subscribeStepCompletionEvents()

	s.logger.Info("orchestrator service started successfully")
	return nil
}

// Stop stops all orchestrator components
func (s *Service) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return ErrServiceNotRunning
	}
	s.running = false
	s.mu.Unlock()

	s.logger.Info("stopping orchestrator service")

	// Stop components in reverse order
	var errs []error

	if err := s.scheduler.Stop(); err != nil {
		s.logger.Error("failed to stop scheduler", zap.Error(err))
		errs = append(errs, err)
	}

	if err := s.watcher.Stop(); err != nil {
		s.logger.Error("failed to stop watcher", zap.Error(err))
		errs = append(errs, err)
	}

	s.cancelAllClarificationWatchdogs()
	s.cancelAllTransientRetries()

	if len(errs) > 0 {
		return errs[0]
	}

	s.logger.Info("orchestrator service stopped successfully")
	return nil
}

// reconcileSessionsOnStartup adjusts database state for sessions that were active before restart.
// It does NOT launch any agent processes — sessions are recovered lazily when the user opens them
// (via task.session.status → task.session.resume or by sending a prompt).
//
// Strategy:
//
//  1. Terminal states (Completed/Cancelled/Failed) → stop any persisted runtime handle,
//     then clean up executor record only after a confirmed stop
//  2. Never-started (Created) → clean up executor record
//  3. Active states (Starting/Running/WaitingForInput) → set session to WAITING_FOR_INPUT,
//     clear stale execution IDs, fix task state, preserve ExecutorRunning record
//  4. Pre-poll remote executor status for remote runtimes (sprites, remote_docker)
//
// Called by: Start() method during orchestrator initialization.
func (s *Service) reconcileSessionsOnStartup(ctx context.Context) {
	runningExecutors, err := s.repo.ListExecutorsRunning(ctx)
	if err != nil {
		s.logger.Warn("failed to list executors running on startup", zap.Error(err))
		return
	}
	if len(runningExecutors) == 0 {
		s.logger.Info("no executors to reconcile on startup")
		return
	}

	s.logger.Info("reconciling sessions on startup (lazy recovery)", zap.Int("count", len(runningExecutors)))

	var remoteRecords []executor.RemoteStatusPollRequest
	for _, running := range runningExecutors {
		if models.IsRemoteExecutorType(models.ExecutorType(running.Runtime)) {
			remoteRecords = append(remoteRecords, executor.RemoteStatusPollRequest{
				SessionID:        running.SessionID,
				Runtime:          running.Runtime,
				AgentExecutionID: running.AgentExecutionID,
				ContainerID:      running.ContainerID,
				Metadata:         running.Metadata,
			})
		}
		s.reconcileOneSessionOnStartup(ctx, running)
	}

	// Pre-poll remote executor status so task lists show accurate state
	if len(remoteRecords) > 0 && s.agentManager != nil {
		s.agentManager.PollRemoteStatusForRecords(ctx, remoteRecords)
	}
}

// reconcileOneSessionOnStartup adjusts DB state for a single session without launching agents.
func (s *Service) reconcileOneSessionOnStartup(ctx context.Context, running *models.ExecutorRunning) {
	sessionID := running.SessionID
	if sessionID == "" {
		return
	}

	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		if isTaskSessionNotFound(err) {
			s.handleMissingSessionOnStartup(ctx, running)
			return
		}
		s.logger.Warn("failed to load session for reconciliation; preserving executor record",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}

	previousState := session.State

	// Handle terminal and never-started states (reuse existing cleanup logic)
	if skip := s.handleTerminalSessionOnStartup(ctx, session, running, previousState); skip {
		return
	}
	if previousState == models.TaskSessionStateCreated {
		s.logger.Info("session was never started; cleaning up",
			zap.String("session_id", sessionID))
		// A never-started session has no conversation to lose, so its row is
		// prunable — but route through the invariant so the rare case of a
		// Created row that still carries a resume_token is repaired, not deleted.
		s.pruneOrRepairExecutorRow(ctx, running, previousState)
		return
	}

	// IDLE is the office "between turns" resting state — agent process
	// and executor are already torn down, conversation is preserved
	// in the existing executors_running row for the next run. There
	// is nothing to recover; flipping to WAITING_FOR_INPUT here would
	// (a) lie about the session having pending user input and (b) make
	// the chat UI render as "working" because the office session shape
	// uses IDLE specifically to avoid that.
	if previousState == models.TaskSessionStateIdle {
		// Keep the IDLE session state (no flip to WAITING_FOR_INPUT), but still
		// make the ROW true: an office turn writes IDLE and then tears down, so a
		// crash/restart in that window leaves a row claiming status=running with a
		// dead local_pid. If the local process is confirmed dead, repair the row
		// in place — resume_token/worktree are preserved (RowMustBePreserved
		// treats IDLE as resumable). Remote rows report Unknown and are untouched.
		if s.rowLiveness(running) == models.ProcessLivenessDead {
			s.repairDeadRowLiveness(ctx, running)
		}
		s.logger.Info("session reconciled for lazy recovery (idle, no state change)",
			zap.String("session_id", sessionID),
			zap.String("task_id", running.TaskID),
			zap.Bool("has_resume_token", running.ResumeToken != ""),
			zap.Bool("has_worktree", running.WorktreePath != ""))
		return
	}

	// Active states: STARTING, RUNNING, WAITING_FOR_INPUT
	// Set session to WAITING_FOR_INPUT (idle, ready for lazy resume when user opens it)
	if previousState != models.TaskSessionStateWaitingForInput {
		if err := s.repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateWaitingForInput, ""); err != nil {
			s.logger.Warn("failed to set session to WAITING_FOR_INPUT on startup",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
	}
	s.abandonOpenTurnsOnStartup(ctx, sessionID, "active session reconciled to waiting")

	// PRESERVE executors_running.agent_execution_id post-restart. The in-memory
	// process is gone, but the stored ID still serves as a "this session was
	// previously launched" marker that drives resume detection in
	// applyRunningRecordToResumeRequest → isResumedSession → ResumePassthroughSession
	// (which adds the --resume flag). Clearing it here makes the next launch take
	// the fresh-start path and skip the resume CLI flag, so passthrough TUIs lose
	// their conversation context after every backend restart. The stale ID is
	// harmless: persistExecutorRunning UPSERTs it atomically alongside the
	// in-memory Add on the next launch, closing the divergence window we set out
	// to fix in this refactor.

	// Ensure task is in REVIEW state (not stuck IN_PROGRESS)
	if running.TaskID != "" {
		task, taskErr := s.repo.GetTask(ctx, running.TaskID)
		if taskErr == nil && task != nil && task.State == v1.TaskStateInProgress && !taskArchived(task) {
			// UpdateTaskStateIfCurrentIn (not the unconditional UpdateTaskState)
			// so the write is atomic against archived_at: the taskArchived guard
			// above reads the row before this call, and ArchiveTask can commit in
			// that window without changing task.State, so only an archive-aware
			// conditional write closes the race.
			if _, updateErr := s.taskRepo.UpdateTaskStateIfCurrentIn(ctx, running.TaskID, v1.TaskStateReview, []v1.TaskState{v1.TaskStateInProgress}); updateErr != nil {
				s.logger.Warn("failed to update task to REVIEW on startup",
					zap.String("task_id", running.TaskID),
					zap.Error(updateErr))
			}
		}
	}

	// PRESERVE the ExecutorRunning record — it holds the resume token and worktree info
	// needed for lazy recovery when the user opens the session. But make the row
	// TRUE: after a restart the spawned process is normally gone, so if the local
	// liveness handle is confirmed dead, repair the row (status=stopped, local_pid
	// cleared) so it never keeps claiming a live process (#1597 expected behavior).
	// resume_token/worktree are preserved by the repair. Remote rows report Unknown
	// and are left to their own runtime's status poll.
	if s.rowLiveness(running) == models.ProcessLivenessDead {
		s.repairDeadRowLiveness(ctx, running)
	}

	s.logger.Info("session reconciled for lazy recovery",
		zap.String("session_id", sessionID),
		zap.String("task_id", running.TaskID),
		zap.String("previous_state", string(previousState)),
		zap.Bool("has_resume_token", running.ResumeToken != ""),
		zap.Bool("has_worktree", running.WorktreePath != ""))
}

func (s *Service) handleMissingSessionOnStartup(ctx context.Context, running *models.ExecutorRunning) {
	sessionID := running.SessionID
	executionID := strings.TrimSpace(running.AgentExecutionID)
	if executionID == "" || s.agentManager == nil {
		s.logger.Warn("executor record has no session and no stoppable runtime handle; preserving record",
			zap.String("session_id", sessionID),
			zap.String("task_id", running.TaskID))
		return
	}
	if err := s.agentManager.StopAgentWithReason(ctx, executionID, "startup missing session cleanup", true); err != nil {
		s.logger.Warn("failed to stop missing-session runtime; preserving executor record",
			zap.String("session_id", sessionID),
			zap.String("task_id", running.TaskID),
			zap.String("execution_id", executionID),
			zap.Error(err))
		return
	}
	if err := s.repo.DeleteExecutorRunningBySessionID(ctx, sessionID); err != nil {
		s.logger.Warn("failed to remove executor record for missing session",
			zap.String("session_id", sessionID),
			zap.String("task_id", running.TaskID),
			zap.Error(err))
	}
}

func (s *Service) abandonOpenTurnsOnStartup(ctx context.Context, sessionID, reason string) {
	if s.turnService == nil {
		return
	}
	s.activeTurns.Delete(sessionID)
	if err := s.turnService.AbandonOpenTurns(ctx, sessionID); err != nil {
		s.logger.Warn("failed to abandon open turns during startup reconciliation",
			zap.String("session_id", sessionID),
			zap.String("reason", reason),
			zap.Error(err))
	}
}

func isTaskSessionNotFound(err error) bool {
	return errors.Is(err, models.ErrTaskSessionNotFound)
}

// handleTerminalSessionOnStartup processes sessions in terminal states during startup.
// Returns true if the session should be skipped (no further processing needed).
func (s *Service) handleTerminalSessionOnStartup(ctx context.Context, session *models.TaskSession, running *models.ExecutorRunning, previousState models.TaskSessionState) bool {
	sessionID := session.ID
	switch previousState {
	case models.TaskSessionStateCompleted, models.TaskSessionStateCancelled:
		s.logger.Info("session in terminal state; stopping runtime before cleaning up executor record",
			zap.String("session_id", sessionID),
			zap.String("task_id", session.TaskID),
			zap.String("state", string(previousState)))
		s.abandonOpenTurnsOnStartup(ctx, sessionID, "terminal session cleanup")
		executionID := strings.TrimSpace(running.AgentExecutionID)
		if executionID != "" && s.agentManager != nil {
			if err := s.agentManager.StopAgentWithReason(ctx, executionID, "startup terminal session cleanup", true); err != nil {
				s.logger.Warn("failed to stop terminal session runtime; preserving executor record",
					zap.String("session_id", sessionID),
					zap.String("execution_id", executionID),
					zap.Error(err))
				return true
			}
		}
		// Resume-safety invariant: prune the row only if it is not still resumable
		// (no resume_token). A terminal session that kept a resume_token — e.g. an
		// office run that COMPLETED a turn but stays resumable — is repaired in
		// place instead of deleted.
		s.pruneOrRepairExecutorRow(ctx, running, previousState)
		return true
	case models.TaskSessionStateFailed:
		s.handleFailedSessionOnStartup(ctx, session, running)
		return true
	}
	return false
}

// handleFailedSessionOnStartup handles a failed session during startup recovery.
func (s *Service) handleFailedSessionOnStartup(ctx context.Context, session *models.TaskSession, running *models.ExecutorRunning) {
	sessionID := session.ID
	s.abandonOpenTurnsOnStartup(ctx, sessionID, "failed session cleanup")
	// If session failed, ensure task is in REVIEW state (not stuck IN_PROGRESS)
	if session.TaskID != "" {
		task, taskErr := s.repo.GetTask(ctx, session.TaskID)
		if taskErr == nil && task != nil && task.State == v1.TaskStateInProgress && !taskArchived(task) {
			s.logger.Info("fixing task state: session failed but task still IN_PROGRESS",
				zap.String("task_id", session.TaskID),
				zap.String("session_id", sessionID))
			// UpdateTaskStateIfCurrentIn (not the unconditional UpdateTaskState)
			// so the write is atomic against archived_at — see the matching
			// comment in reconcileOneSessionOnStartup above.
			if _, updateErr := s.taskRepo.UpdateTaskStateIfCurrentIn(ctx, session.TaskID, v1.TaskStateReview, []v1.TaskState{v1.TaskStateInProgress}); updateErr != nil {
				s.logger.Warn("failed to update task state to REVIEW",
					zap.String("task_id", session.TaskID),
					zap.Error(updateErr))
			}
		}
	}
	if canResumeRunning(running) {
		s.logger.Info("preserving executor record for resumable failed session",
			zap.String("session_id", sessionID),
			zap.String("task_id", session.TaskID))
	} else {
		s.logger.Info("stopping failed session runtime before cleaning up executor record",
			zap.String("session_id", sessionID),
			zap.String("task_id", session.TaskID))
		executionID := strings.TrimSpace(running.AgentExecutionID)
		if executionID != "" && s.agentManager != nil {
			if err := s.agentManager.StopAgentWithReason(ctx, executionID, "startup failed session cleanup", true); err != nil {
				s.logger.Warn("failed to stop failed session runtime; preserving executor record",
					zap.String("session_id", sessionID),
					zap.String("execution_id", executionID),
					zap.Error(err))
				return
			}
		}
		// Prune only subject to the resume-safety invariant (a lingering
		// resume_token is repaired in place rather than deleted).
		s.pruneOrRepairExecutorRow(ctx, running, models.TaskSessionStateFailed)
	}
}

func canResumeRunning(running *models.ExecutorRunning) bool {
	if running == nil || running.ResumeToken == "" {
		return false
	}
	if running.Resumable {
		return true
	}
	return models.IsAlwaysResumableRuntime(running.Runtime)
}

func isMissingMergedPRBranchError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "couldn't find remote ref") ||
		(strings.Contains(msg, "not found locally or on remote") && strings.Contains(msg, "branch")) ||
		(strings.Contains(msg, "pathspec") && strings.Contains(msg, "did not match"))
}

var (
	quotedBranchPattern   = regexp.MustCompile(`branch "([^"]+)"`)
	remoteRefPattern      = regexp.MustCompile(`remote ref ([^\s]+)`)
	pathspecBranchPattern = regexp.MustCompile(`pathspec '([^']+)'`)
)

func extractMissingBranchName(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if match := quotedBranchPattern.FindStringSubmatch(msg); len(match) == 2 {
		return strings.TrimSpace(match[1])
	}
	if match := remoteRefPattern.FindStringSubmatch(msg); len(match) == 2 {
		return strings.TrimSpace(match[1])
	}
	if match := pathspecBranchPattern.FindStringSubmatch(msg); len(match) == 2 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

func (s *Service) handleSessionLaunchFailed(ctx context.Context, taskID, sessionID string, launchErr error) {
	if s.messageCreator == nil || !isMissingMergedPRBranchError(launchErr) {
		return
	}

	branch := extractMissingBranchName(launchErr)
	content := "This task references a PR branch that no longer exists on remote (likely merged and deleted)."
	if branch != "" {
		content = "The remote PR branch \"" + branch + "\" no longer exists (likely merged and deleted)."
	}
	metadata := map[string]interface{}{
		"variant":        "warning",
		"failure_kind":   "missing_pr_branch",
		"missing_branch": branch,
		"actions": []map[string]interface{}{
			{
				actionMetaKeyType:    "archive_task",
				actionMetaKeyLabel:   "Archive task",
				actionMetaKeyTooltip: "Keep task history and hide it from active work",
				actionMetaKeyIcon:    "archive",
				actionMetaKeyTestID:  "missing-branch-archive-button",
			},
			{
				actionMetaKeyType:    "delete_task",
				actionMetaKeyLabel:   "Delete task",
				actionMetaKeyTooltip: "Permanently remove this task",
				"variant":            "destructive",
				actionMetaKeyIcon:    "trash",
				actionMetaKeyTestID:  "missing-branch-delete-button",
			},
		},
	}
	s.suppressToast.Store(sessionID, true)
	msgCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.messageCreator.CreateSessionMessage(
		msgCtx,
		taskID,
		content,
		sessionID,
		string(v1.MessageTypeStatus),
		"", // No turn — agent never started, avoid lazily creating a synthetic turn.
		metadata,
		false,
	); err != nil {
		s.logger.Warn("failed to create missing PR branch launch failure message",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

// IsRunning returns true if the service is running
func (s *Service) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// GetStatus returns the orchestrator status
func (s *Service) GetStatus() *Status {
	s.mu.RLock()
	running := s.running
	startedAt := s.startedAt
	s.mu.RUnlock()

	queueStatus := s.scheduler.GetQueueStatus()

	var uptimeSeconds int64
	if running {
		uptimeSeconds = int64(time.Since(startedAt).Seconds())
	}

	return &Status{
		Running:        running,
		ActiveAgents:   queueStatus.ActiveExecutions,
		QueuedTasks:    queueStatus.QueuedTasks,
		TotalProcessed: queueStatus.TotalProcessed,
		TotalFailed:    queueStatus.TotalFailed,
		UptimeSeconds:  uptimeSeconds,
		LastHeartbeat:  time.Now(),
	}
}

// GetMessageQueue returns the message queue service
func (s *Service) GetMessageQueue() *messagequeue.Service {
	return s.messageQueue
}

// GetEventBus returns the event bus
func (s *Service) GetEventBus() bus.EventBus {
	return s.eventBus
}
