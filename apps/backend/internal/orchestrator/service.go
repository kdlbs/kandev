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
	"database/sql"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func isNoActiveTurnError(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

// Common errors
var (
	ErrServiceAlreadyRunning = errors.New("service is already running")
	ErrServiceNotRunning     = errors.New("service is not running")
)

// ServiceConfig holds orchestrator service configuration
type ServiceConfig struct {
	Scheduler                     scheduler.SchedulerConfig
	QueueSize                     int
	QueueGroup                    string
	ClaudeBackgroundPromptHandoff bool

	// ClaudeMidTurnSteering enables delivering a prompt into a still-generating
	// turn for an agent that advertised prompt queueing. Independent of
	// ClaudeBackgroundPromptHandoff, which covers the foreground-idle handoff.
	ClaudeMidTurnSteering bool
}

// AttachmentReader is the narrow attachment-store seam needed when the
// orchestrator delivers claimed descriptors to a passthrough agent.
// Keeping this interface local avoids coupling the coordinator to lifecycle's
// concrete package while preserving the same structural contract.
type AttachmentReader interface {
	OpenClaimed(ctx context.Context, id, taskID, sessionID string) (io.ReadCloser, string, string, int64, error)
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
	PublishTaskUpdated(ctx context.Context, task *models.Task, oldWorkflowIDs ...string)
	PublishTaskStateChanged(ctx context.Context, task *models.Task, oldState v1.TaskState)
	// PublishTaskActivityIfChanged recomputes the task-level MOST-ACTIVE-WINS
	// activity aggregate and emits task.updated only when its three-state value
	// changed — including a generating↔background flip that leaves the coarse
	// state unchanged.
	PublishTaskActivityIfChanged(ctx context.Context, taskID string)
}

type taskQueuePromotionPublisher interface {
	PublishTaskQueuePromoted(ctx context.Context, task *models.Task)
}

// WorkflowMeta is the subset of workflow fields needed at step entry
// (agent profile default + optional workflow-level prompt).
type WorkflowMeta struct {
	AgentProfileID string
	Prompt         string
}

// WorkflowStepGetter retrieves workflow step information for prompt building.
type WorkflowStepGetter interface {
	GetStep(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error)
	GetNextStepByPosition(ctx context.Context, workflowID string, currentPosition int) (*wfmodels.WorkflowStep, error)
	GetPreviousStepByPosition(ctx context.Context, workflowID string, currentPosition int) (*wfmodels.WorkflowStep, error)
	// GetWorkflowMeta returns agent profile id and prompt for a workflow in one
	// read. Step-entry paths that need both fields should call this once.
	GetWorkflowMeta(ctx context.Context, workflowID string) (WorkflowMeta, error)
}

// AgentFamilyResolver maps a hand-written agent family reference onto the
// canonical IDs of every agent that answers to it. Exactly one candidate
// resolves the reference; none means it names no known agent; more than one
// means it is ambiguous and must not be guessed. The candidates themselves —
// not just how many there are — decide whether an ambiguous reference concerns
// a particular session at all. Implemented by registry.Registry.
type AgentFamilyResolver interface {
	ResolveFamilyIDs(name string) []string
}

// PromptReferenceExpander resolves "@name" saved-prompt references embedded in
// an effective prompt and returns both the expanded prompt and the exact
// server-generated block content. Implemented by promptservice.Service.
type PromptReferenceExpander interface {
	AppendReferenceExpansionsWithContext(
		ctx context.Context,
		prompt string,
		log *zap.Logger,
	) (expandedPrompt, trustedContext string)
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
	ListTaskWorkspaceFolders(ctx context.Context, taskID string) ([]*models.TaskWorkspaceFolder, error)
	CreateTaskSession(ctx context.Context, session *models.TaskSession) error
	UpdateTaskSession(ctx context.Context, session *models.TaskSession) error
	ListActiveTaskSessions(ctx context.Context) ([]*models.TaskSession, error)
	ListActiveTaskSessionsByTaskID(ctx context.Context, taskID string) ([]*models.TaskSession, error)
	ListTaskSessionWorktrees(ctx context.Context, sessionID string) ([]*models.TaskEnvironmentRepo, error)
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
	ClaimPromptableTaskSessionIfActive(ctx context.Context, id string) (models.PromptableTaskSessionClaim, error)
	UpdateTaskSessionBaseCommit(ctx context.Context, id string, baseCommitSHA string) error
	GetTaskSessionByTaskAndAgent(ctx context.Context, taskID, agentInstanceID string) (*models.TaskSession, error)
	UpdateTaskSessionWorktreeBranch(ctx context.Context, sessionID, branch string) error
	UpdateSessionReviewStatus(ctx context.Context, sessionID string, status string) error
	UpdateSessionMetadata(ctx context.Context, sessionID string, metadata map[string]interface{}) error
	SetSessionMetadataKey(ctx context.Context, sessionID, key string, value interface{}) error
	SetSessionMetadataKeyIfAbsent(ctx context.Context, sessionID, key string, value interface{}) (bool, error)
	UpdateSessionContextWindow(ctx context.Context, sessionID string, contextWindow map[string]interface{}) (int64, error)
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
	// ClaimTaskTitleSession atomically assigns the first eligible session to a
	// pending task title. The second return value reports a new claim rather
	// than an idempotent same-owner observation.
	ClaimTaskTitleSession(ctx context.Context, taskID, sessionID string) (owned bool, newlyClaimed bool, err error)
	UpdateTask(ctx context.Context, task *models.Task) error
	// SetTaskMetadataKey / RemoveTaskMetadataKey are concurrent-key-safe JSON
	// patch helpers on tasks.metadata (implemented by the sqlite/Postgres
	// repository). Used by startup reconciliation to mark interrupted tasks
	// and by the session-start funnel to clear the marker.
	SetTaskMetadataKey(ctx context.Context, taskID, key string, value interface{}) error
	// SetTaskMetadataKeyIfNotArchived writes one metadata key only when the
	// task row still has archived_at IS NULL (single statement — the archive
	// check and the write cannot be separated by a concurrent archive).
	SetTaskMetadataKeyIfNotArchived(ctx context.Context, taskID, key string, value interface{}) (bool, error)
	RemoveTaskMetadataKey(ctx context.Context, taskID, key string) (bool, error)
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

// ClaimTaskTitleSession claims the first-turn generated-title handoff for a
// task. The repository owns the compare-and-set; the orchestrator publishes a
// task update only when this call creates a new owner.
func (s *Service) ClaimTaskTitleSession(ctx context.Context, taskID, sessionID string) (bool, error) {
	if taskID == "" || sessionID == "" {
		return false, nil
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return false, err
	}
	if !models.IsAgentTitlePending(task.Metadata) {
		return false, nil
	}
	owned, newlyClaimed, err := s.repo.ClaimTaskTitleSession(ctx, taskID, sessionID)
	if err != nil {
		return false, err
	}
	if newlyClaimed {
		claimedTask, reloadErr := s.repo.GetTask(ctx, taskID)
		if reloadErr != nil {
			return false, reloadErr
		}
		s.publishTaskUpdated(ctx, claimedTask)
	}
	return owned, nil
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

	// sessionAccessCheck enforces per-user workspace scoping on the
	// session-keyed WS actions. Nil = unscoped. See SetSessionAccessChecker.
	sessionAccessCheck func(ctx context.Context, sessionID string) error

	// taskAccessCheck is the task-keyed sibling of sessionAccessCheck, for
	// entry points that name a task rather than a session (session.launch,
	// session.ensure). Nil = unscoped.
	taskAccessCheck func(ctx context.Context, taskID string) error

	// titleBranchRuntime performs the lifecycle-owned Git branch rename after
	// an agent resolves a prompt-first task title. It is optional for tests and
	// installations that do not configure an agent runtime.
	titleBranchRuntime titleBranchRuntime

	// Workflow step getter for prompt building
	workflowStepGetter WorkflowStepGetter

	// Resolves the agent family names written in configure_session rules onto
	// canonical agent IDs. Nil-safe: when unset, rule matching falls back to an
	// exact string comparison.
	agentFamilyResolver AgentFamilyResolver

	// Prompt reference expander for resolving "@name" saved-prompt
	// references in the effective workflow-step prompt. Nil-safe: when
	// unset, buildWorkflowPrompt leaves the prompt unchanged.
	promptExpander PromptReferenceExpander

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

	// Native code review. When set, buildWorkflowCallbacks registers the
	// run_code_review on_enter action. Nil-safe: without it the action kind
	// simply has no callback and the engine treats it as a no-op.
	reviewRunner ReviewRunner

	// GitHub service for PR auto-detection on push
	githubService GitHubService
	// ciAutomationInFlight prevents PR feedback and task-PR update events from
	// racing duplicate auto-fix prompts or merge calls for the same PR.
	ciAutomationInFlight sync.Map

	// GitLab MR lifecycle notification automation. Nil-safe: without it,
	// gitlab.task_mr.updated events are observed but no lifecycle prompt is
	// ever evaluated.
	gitlabMRAutomation taskMRAgentAutomationService
	// mrAutomationInFlight prevents overlapping poll ticks from running the
	// lifecycle evaluation pass twice for the same (task, repository, iid).
	mrAutomationInFlight sync.Map

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
	gitlabService      GitLabWatchService
	gitlabReviewSource *GitLabReviewWatcherSource
	gitlabIssueSource  *GitLabIssueWatcherSource
	// Azure DevOps watcher service + sources for work-item and pull-request
	// polling events. The integration publishes provider-native matches; the
	// shared watcher coordinator owns task creation and throttling.
	azureDevOpsService     AzureDevOpsWatchService
	azureWorkItemSource    *AzureDevOpsWorkItemWatcherSource
	azurePullRequestSource *AzureDevOpsPullRequestWatcherSource

	// gitlabMRLinkService auto-links merge requests opened outside Kandev's
	// Create-PR action, mirroring what githubService does for PRs. Separate
	// from gitlabService (the review/issue watch surface) — see
	// GitLabMRLinkService's doc comment.
	gitlabMRLinkService GitLabMRLinkService

	// Repository resolver for cloning + finding/creating repos for review tasks
	repositoryResolver RepositoryResolver

	// Automation service for handling automation triggers
	automationService AutomationService

	// Worktree reaper — reclaims the workspaces of automation runs that have
	// aged out of the per-automation retention window. Nil-safe: most tests
	// construct the service without one, and an install with no worktree
	// manager simply keeps every run's checkout. Set via SetWorktreeManager.
	worktreeReaper automationWorktreeReaper

	// unreclaimedWorkspaces holds the automation runs whose workspace removal
	// was attempted and did not actually free the directory. Retention selects
	// candidates by "still has a live worktree row", and a failed removal can
	// leave a run with no live row and a full checkout, invisible to every
	// later sweep — this is how it gets retried instead of written off. See
	// queueAutomationWorkspaceReclaim.
	unreclaimedWorkspaces   map[string]struct{}
	unreclaimedWorkspacesMu sync.Mutex

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

	// dispatchingQueued tracks the pre-acceptance reservation for the exact
	// queued message handed to an async worker. acceptedQueuedDispatch keeps
	// the same ownership visible after the worker claims RUNNING until its turn
	// settles, so Send Now cannot cancel or duplicate a successor that FIFO has
	// already accepted. The two maps are managed by queued_dispatch.go and are
	// arbitrated through cancelInFlight.
	dispatchingQueued      sync.Map
	acceptedQueuedDispatch sync.Map

	// afterReadyLifecycleReservation is a deterministic test seam for the
	// narrow interval after handleAgentReady releases its per-session guard
	// and before it starts a deferred durable-lifecycle dispatch.
	afterReadyLifecycleReservation func()

	// foregroundActivity tracks, per session, whether the open turn is actively
	// generating in the foreground or only waiting on a spawned background task
	// (subagent / run-in-background shell). Keyed sessionID -> *turnActivity;
	// see turn_activity.go. This is best-effort accounting state and does not
	// relax the coarse RUNNING admission policy.
	// foregroundActivityMu protects record identity: lookups lock the selected
	// record before releasing it, and execution teardown uses the same order
	// before detaching an unused record.
	foregroundActivityMu sync.Mutex
	foregroundActivity   map[string]*turnActivity
	// activityPublicationGuards serialize validation and event delivery by
	// session across turnActivity record generations. Entries are reference
	// counted and reclaimed when the last publisher/retirer releases them.
	activityPublicationGuardsMu sync.Mutex
	activityPublicationGuards   map[string]*activityPublicationGuard

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

	// steerInFlight tracks sessions with an unacknowledged mid-turn steer.
	// The spec allows at most one in-flight steer per session; a second attempt
	// while one is outstanding queues instead. Keyed by sessionID, cleared when
	// the steer dispatch is accepted or fails.
	//
	// Also read by the non-cancelling queue-drain paths (see isSteerInFlight):
	// SteerTask releases the per-session message-queue admission lock before
	// its own blocking dispatch, so a message queued in that window — with the
	// turn completing in the same window — must not let an ordinary drain
	// dispatch it ahead of the steer that was admitted first.
	steerInFlight sync.Map
	// Session reset flags: sessionID -> true while resetAgentContext is restarting process.
	// Used to suppress stale ready events and avoid draining queued prompts mid-reset.
	resetInProgressSessions sync.Map
	// contextWindowGuards serializes live context usage writes and gives each
	// session a generation boundary that reset_agent_context can invalidate.
	contextWindowGuards sync.Map

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
	// (the user cancel button, with duplicate and cross-source calls joined by
	// the unified cancellation operation registry), the natural turn-completion/boot-ready drain
	// decision in handleAgentReady / handleAgentBootReady (blocking while an
	// active cancellation marker yields),
	// or QueueAndInterruptForPeerMessage (blocking Lock — must wait rather
	// than work around a busy lock with an unguarded insert; see its doc
	// comment), or an agent stream handler persisting output (blocking Lock so
	// cancellation cannot commit while a stream side effect is in flight).
	// All of these must go through the same per-session guard — a second,
	// independent lock for any of them would defeat the mutual exclusion the
	// others rely on to avoid racing each other's take-and-dispatch decision
	// for the same session.
	//
	// Entries are reference-counted (acquireCancelInFlightGuard /
	// releaseCancelInFlightGuard) and pruned once nobody holds a reference,
	// so this stays bounded by concurrently-active sessions rather than
	// growing by one permanent entry per session ever created over a
	// long-lived backend's lifetime.
	cancelInFlight map[string]*cancelInFlightGuard

	// cancellationOperations is the single per-session ownership boundary for
	// every lifecycle cancellation source: explicit user cancel, silent
	// clarification recovery, and peer-message interrupts. The owner keeps the
	// operation in this map across the lifecycle wait and source-specific
	// reconciliation so all other state claimants either join it or defer.
	cancellationOperationsMu sync.Mutex
	cancellationOperations   map[string]*cancelOperation

	// cancelOperations tracks cancellation intent separately from the shared
	// cancelInFlight mutex. Stream and lifecycle handlers use that mutex to
	// serialize their side effects, but they are not themselves cancellations;
	// treating every mutex holder as a cancellation would make agent.boot_ready
	// discard a legitimate boot signal while a stream frame is being persisted.
	// Values hold the reference count, process-local transition revision, and
	// publication queue for each session. Entries remain after the count reaches
	// zero so a later operation cannot reuse an older revision. The single mutex
	// makes each count transition and its queued publication one atomic state change; event-bus
	// sends happen after releasing it so one slow bus cannot delay other sessions.
	cancelOperationsMu sync.Mutex
	cancelOperations   map[string]*cancellationOperationState

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

	// sendNowWorkers owns the asynchronous replacement handoffs. The context
	// is cancelled during Stop so a claimed durable source gets a bounded
	// recovery attempt before shutdown returns.
	sendNowMu      sync.Mutex
	sendNowCtx     context.Context
	sendNowCancel  context.CancelFunc
	sendNowStopped bool
	sendNowWorkers sync.WaitGroup
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

	usesDefaultEphemeralQueue := msgQueue == nil
	if usesDefaultEphemeralQueue {
		msgQueue = messagequeue.NewServiceMemory(log)
	}
	// Task-repository mutations purge the shared SQLite lifecycle queue in the
	// same transaction. Only the fallback in-memory queue needs the post-commit
	// callback; registering a supplied production queue here would purge the
	// next generation accepted after an archive/unarchive race.
	if usesDefaultEphemeralQueue {
		registrar, ok := repo.(interface {
			SetTaskQueuePurger(func(context.Context, string))
		})
		if ok {
			registrar.SetTaskQueuePurger(func(ctx context.Context, taskID string) {
				if _, err := msgQueue.PurgeTask(ctx, taskID); err != nil {
					svcLogger.Warn("failed to purge ephemeral task queue after task lifecycle mutation",
						zap.String("task_id", taskID), zap.Error(err))
				}
			})
		}
	}

	// Create the service (watcher will be created after we have handlers)
	sendNowCtx, sendNowCancel := context.WithCancel(context.Background())
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
		sendNowCtx:                   sendNowCtx,
		sendNowCancel:                sendNowCancel,
	}
	exec.SetOnContextWindowReset(s.clearContextWindowForReset)

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
	exec.SetOnEarlyLaunchTaskStateReconcile(s.reconcileTaskStateForEarlyLaunchFailure)
	exec.SetOnSessionStateChange(func(ctx context.Context, taskID, sessionID string, state models.TaskSessionState, errorMessage string) error {
		s.updateTaskSessionState(ctx, taskID, sessionID, state, errorMessage, true)
		return nil
	})
	exec.SetOnSessionStateTransition(s.transitionTaskSessionState)
	exec.SetOnSessionStarting(func(
		ctx context.Context,
		taskID string,
		session *models.TaskSession,
		expectedState models.TaskSessionState,
		promoteTask bool,
	) error {
		return s.setSessionStarting(ctx, taskID, session, expectedState, promoteTask)
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
		OnAgentStalled:         s.handleAgentStalled,
		OnAgentStopped:         s.handleAgentStopped,
		OnAgentStreamEvent:     s.handleAgentStreamEvent,
		OnACPSessionCreated:    s.handleACPSessionCreated,
		OnPermissionRequest:    s.handlePermissionRequest,
		OnGitEvent:             s.handleGitEvent,
		OnContextWindowUpdated: s.handleContextWindowUpdated,
		OnTaskMoved:            s.handleTaskMoved,
		OnTaskQueuePromoted:    s.handleTaskQueuePromoted,
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

// SetAttachmentReader wires the backend attachment store into passthrough
// prompt delivery so claimed descriptors can be streamed into the workspace.
func (s *Service) SetAttachmentReader(reader AttachmentReader) {
	s.executor.SetAttachmentReader(reader)
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

// RepositoryHostCloner is the narrow host-materialization contract. It returns
// only the persisted local path; clone credentials remain private to the
// orchestrator executor pipeline.
type RepositoryHostCloner interface {
	EnsureRepositoryCloned(context.Context, *models.Repository) (string, error)
}

// EnsureRepositoryCloned delegates host cloning through the executor's
// authenticated clone pipeline.
func (s *Service) EnsureRepositoryCloned(ctx context.Context, repository *models.Repository) (string, error) {
	return s.executor.EnsureRepositoryCloned(ctx, repository)
}

// SetGitHubCredentialBroker configures renewable workspace GitHub credentials
// for executor git and gh operations.
func (s *Service) SetGitHubCredentialBroker(issuer executor.GitHubCredentialLeaseIssuer, endpoint string) {
	s.executor.SetGitHubCredentialBroker(issuer, endpoint)
}

// SetAgentctlBinaryPath configures the host agentctl executable used by
// managed Git during Local and Worktree preparation.
func (s *Service) SetAgentctlBinaryPath(path string) {
	s.executor.SetAgentctlBinaryPath(path)
}

// SetTaskGitCredentialPolicyResolver configures non-secret workspace policy lookup.
func (s *Service) SetTaskGitCredentialPolicyResolver(resolver executor.TaskGitCredentialPolicyResolver) {
	s.executor.SetTaskGitCredentialPolicyResolver(resolver)
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

// SetSessionAccessChecker installs the per-user workspace scoping check used by
// the session-keyed WS actions (task session status, session PR check). Those
// resolve sessions through the orchestrator's own repo handle rather than the
// task service, so they do not inherit its authorize* checks and must ask here.
//
// The checker must return nil for contexts without a request identity, so
// internal callers (event bus, schedulers) stay unscoped. Nil leaves every
// session-keyed action unscoped, which is the pre-auth behavior.
func (s *Service) SetSessionAccessChecker(check func(ctx context.Context, sessionID string) error) {
	s.sessionAccessCheck = check
}

// SetTaskAccessChecker installs the task-keyed sibling of
// SetSessionAccessChecker, used by the entry points that name a task rather
// than a session. Same contract: nil for identity-less internal callers.
func (s *Service) SetTaskAccessChecker(check func(ctx context.Context, taskID string) error) {
	s.taskAccessCheck = check
}

// authorizeSession applies the configured per-user session check. No-op when
// unwired.
func (s *Service) authorizeSession(ctx context.Context, sessionID string) error {
	if s.sessionAccessCheck == nil || sessionID == "" {
		return nil
	}
	return s.sessionAccessCheck(ctx, sessionID)
}

// SessionTaskID returns the task that owns a session, or "" when the session
// is unknown. Used to enrich message.queue.status_changed events with the
// task_id so task-scoped consumers (e.g. the status summary projector) can
// refresh per-task queued-prompt counts.
func (s *Service) SessionTaskID(ctx context.Context, sessionID string) (string, error) {
	if sessionID == "" || s.repo == nil {
		return "", nil
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if session == nil {
		return "", nil
	}
	return session.TaskID, nil
}

// authorizeTask applies the configured per-user task check. No-op when unwired.
func (s *Service) authorizeTask(ctx context.Context, taskID string) error {
	if s.taskAccessCheck == nil || taskID == "" {
		return nil
	}
	return s.taskAccessCheck(ctx, taskID)
}

// authorizeTaskSessionPair guards an entry point that accepts BOTH a task and a
// session ID.
//
// Checking only the session is not enough: the caller can pass one of their own
// sessions to satisfy that check while pointing taskID at another user's task,
// which the method then uses for its task-scoped work (GetTaskPR, repository
// resolution, PR-watch creation, recoverable-failure handling). Both IDs are
// authorized, and the pair is required to be consistent so the two arguments
// cannot describe different tasks.
//
// The pair check is skipped when the session cannot be loaded: by then both IDs
// are already authorized, so a mismatch is a caller bug rather than a
// cross-user leak, and failing here would change behavior for identity-less
// internal callers.
func (s *Service) authorizeTaskSessionPair(ctx context.Context, taskID, sessionID string) error {
	if err := s.authorizeSession(ctx, sessionID); err != nil {
		return err
	}
	if err := s.authorizeTask(ctx, taskID); err != nil {
		return err
	}
	if taskID == "" || sessionID == "" || s.repo == nil {
		return nil
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return nil //nolint:nilerr // both IDs authorized; consistency is best-effort
	}
	if session.TaskID != taskID {
		return fmt.Errorf("session %s does not belong to task %s", sessionID, taskID)
	}
	return nil
}

// publishTaskUpdated forwards to the configured TaskEventPublisher.
// No-op when the publisher isn't wired (tests, or before SetTaskEventPublisher
// has been called during startup). Pass the pre-move workflow ID when the
// task's workflow changed (e.g. a cross-workflow transition) so the event
// carries old_workflow_id.
func (s *Service) publishTaskUpdated(ctx context.Context, task *models.Task, oldWorkflowIDs ...string) {
	if s.taskEvents == nil || task == nil {
		return
	}
	s.taskEvents.PublishTaskUpdated(ctx, task, oldWorkflowIDs...)
}

func (s *Service) publishTaskQueuePromoted(ctx context.Context, task *models.Task) {
	if s.taskEvents == nil || task == nil {
		return
	}
	if publisher, ok := s.taskEvents.(taskQueuePromotionPublisher); ok {
		publisher.PublishTaskQueuePromoted(ctx, task)
	}
}

func (s *Service) publishTaskStateChanged(ctx context.Context, task *models.Task, oldState v1.TaskState) {
	if s.taskEvents == nil || task == nil {
		return
	}
	s.taskEvents.PublishTaskStateChanged(ctx, task, oldState)
}

// publishTaskActivityIfChanged forwards a per-session activity flip to the task
// service, which recomputes the task-level aggregate and emits task.updated only
// when the aggregated value actually changes. No-op when the publisher isn't wired.
func (s *Service) publishTaskActivityIfChanged(ctx context.Context, taskID string) {
	if s.taskEvents == nil || taskID == "" {
		return
	}
	s.taskEvents.PublishTaskActivityIfChanged(ctx, taskID)
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

// SetAgentFamilyResolver sets the collaborator that maps the agent family names
// written in configure_session rules onto canonical agent IDs.
//
// If not set: a rule matches only when its agent_name is byte-identical to the
// session's stored agent family, and no rule can be reported as a typo or as
// ambiguous because nothing is knowable about an agent family without it.
func (s *Service) SetAgentFamilyResolver(resolver AgentFamilyResolver) {
	s.agentFamilyResolver = resolver
}

// SetPromptReferenceExpander sets the collaborator used to resolve "@name"
// saved-prompt references embedded in workflow-step prompts.
//
// If not set: buildWorkflowPrompt leaves any "@name" references in the
// effective prompt unexpanded.
func (s *Service) SetPromptReferenceExpander(e PromptReferenceExpander) {
	s.promptExpander = e
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
	store := newWorkflowStore(s.repo, s.workflowStepGetter, s.agentManager, s.publishTaskUpdated, s.logger, s.publishTaskMoved, s.publishTaskQueuePromoted)
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

// SetReviewRunner wires the native code-review runner, enabling the
// run_code_review workflow step action.
func (s *Service) SetReviewRunner(runner ReviewRunner) {
	s.reviewRunner = runner
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
	turnID, _ := s.startTurnForSessionWithOwnership(ctx, sessionID)
	return turnID
}

// startTurnForSessionWithOwnership returns whether this dispatch created the
// active turn. Callers that must compensate a failed dispatch may only close a
// turn they created; an adopted turn belongs to an earlier dispatch.
func (s *Service) startTurnForSessionWithOwnership(ctx context.Context, sessionID string) (string, bool) {
	if s.turnService == nil {
		return "", false
	}

	if turnIDVal, ok := s.activeTurns.Load(sessionID); ok {
		if turnID, ok := turnIDVal.(string); ok && turnID != "" {
			return turnID, false
		}
	}

	if turn, err := s.turnService.GetActiveTurn(ctx, sessionID); turn != nil {
		s.activeTurns.Store(sessionID, turn.ID)
		return turn.ID, false
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
		return "", false
	}

	s.activeTurns.Store(sessionID, turn.ID)
	return turn.ID, true
}

// completeTurnForSession closes any open turn for the session.
//
// The DB is the source of truth — activeTurns is just an in-memory cache that
// can drift (see startTurnForSession) or be wiped by a backend restart. We
// query the DB for any open turn and close it. Loops to mop up multiple
// zombies (e.g. left over from before this fix) with a small sanity bound.
func (s *Service) completeTurnForSession(ctx context.Context, sessionID string) {
	s.clearAcceptedQueuedDispatch(sessionID)
	s.completeTurnForTaskSession(ctx, "", sessionID)
}

func (s *Service) completeTurnForTaskSession(ctx context.Context, taskID, sessionID string) {
	// Stream-only completion paths call this helper directly rather than the
	// session wrapper. Clear the accepted queued-dispatch ownership here too,
	// otherwise a completed Send Now/FIFO successor can permanently block the
	// next queue action.
	s.clearAcceptedQueuedDispatch(sessionID)
	if err := s.completeTurnForTaskSessionChecked(ctx, taskID, sessionID); err != nil {
		s.logger.Warn("failed to reconcile active turn",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

// completeTurnForTaskSessionChecked closes every open turn for a session and
// returns an error unless the turn service confirms that none remain. The
// regular lifecycle paths use completeTurnForTaskSession, which preserves
// their best-effort behavior; cancellation uses this checked variant because
// workflow completion must not run while the cancelled turn is still open.
func (s *Service) completeTurnForTaskSessionChecked(ctx context.Context, taskID, sessionID string) error {
	return s.completeTurnForTaskSessionCheckedOwned(ctx, taskID, sessionID, "")
}

// completeTurnForTaskSessionCheckedOwned is the cancellation-specific form of
// turn settlement. When expectedTurnID is non-empty, it closes only that turn
// and fails closed if a different turn is active. This prevents a late cancel
// response from sweeping a successor turn that started after the cancelled
// turn's terminal frames were delivered.
func (s *Service) completeTurnForTaskSessionCheckedOwned(ctx context.Context, taskID, sessionID, expectedTurnID string) error {
	if err := s.verifyExpectedTurnOwnership(ctx, sessionID, expectedTurnID); err != nil {
		return err
	}

	// Foreground ownership ends with the turn, but detached background work can
	// outlive it. Preserve those registrations for accounting; full cleanup
	// belongs to execution/session teardown paths.
	s.yieldForegroundAndPublish(ctx, taskID, sessionID, foregroundYieldTurnCompletion)

	if s.turnService == nil {
		return nil
	}

	if expectedTurnID != "" {
		return s.completeExpectedTurn(ctx, sessionID, expectedTurnID)
	}

	return s.completeAllTurns(ctx, sessionID)
}

func (s *Service) verifyExpectedTurnOwnership(ctx context.Context, sessionID, expectedTurnID string) error {
	if s.turnService == nil || expectedTurnID == "" {
		return nil
	}
	turn, err := s.turnService.GetActiveTurn(ctx, sessionID)
	if err != nil {
		if isNoActiveTurnError(err) {
			return nil
		}
		return fmt.Errorf("look up captured active turn: %w", err)
	}
	if turn != nil && turn.ID != expectedTurnID {
		return fmt.Errorf("captured cancelled turn %s was superseded by active turn %s", expectedTurnID, turn.ID)
	}
	return nil
}

func (s *Service) completeExpectedTurn(ctx context.Context, sessionID, expectedTurnID string) error {
	turn, err := s.turnService.GetActiveTurn(ctx, sessionID)
	if err != nil {
		if isNoActiveTurnError(err) {
			return nil
		}
		return fmt.Errorf("look up captured active turn: %w", err)
	}
	if turn == nil {
		return nil
	}
	if turn.ID != expectedTurnID {
		return fmt.Errorf("captured cancelled turn %s was superseded by active turn %s", expectedTurnID, turn.ID)
	}
	if err := s.turnService.CompleteTurn(ctx, expectedTurnID); err != nil {
		return fmt.Errorf("complete captured turn %s: %w", expectedTurnID, err)
	}
	active, err := s.turnService.GetActiveTurn(ctx, sessionID)
	if err != nil {
		if isNoActiveTurnError(err) {
			s.activeTurns.CompareAndDelete(sessionID, expectedTurnID)
			return nil
		}
		return fmt.Errorf("verify captured turn closure: %w", err)
	}
	if active == nil {
		s.activeTurns.CompareAndDelete(sessionID, expectedTurnID)
		return nil
	}
	if active.ID != expectedTurnID {
		return fmt.Errorf("captured cancelled turn %s was superseded by active turn %s", expectedTurnID, active.ID)
	}
	return fmt.Errorf("captured cancelled turn %s remains open", expectedTurnID)
}

func (s *Service) completeAllTurns(ctx context.Context, sessionID string) error {
	s.activeTurns.Delete(sessionID)

	const maxIterations = 16
	closed := 0
	for closed < maxIterations {
		turn, err := s.turnService.GetActiveTurn(ctx, sessionID)
		if err != nil {
			if isNoActiveTurnError(err) {
				return nil
			}
			return fmt.Errorf("look up active turn: %w", err)
		}
		if turn == nil {
			return nil
		}
		if err := s.turnService.CompleteTurn(ctx, turn.ID); err != nil {
			// GetActiveTurn returns the latest open turn — retrying here
			// would just hit the same row and loop. Bail; the next
			// completeTurnForSession call will pick it up.
			return fmt.Errorf("complete turn %s: %w", turn.ID, err)
		}
		closed++
	}
	// Verify after the cap. Closing exactly maxIterations turns and then
	// finding the session clean is a successful reconciliation, not a runaway.
	turn, err := s.turnService.GetActiveTurn(ctx, sessionID)
	if err != nil {
		if isNoActiveTurnError(err) {
			return nil
		}
		return fmt.Errorf("verify active turn closure: %w", err)
	}
	if turn != nil {
		return fmt.Errorf("active turn %s remains after %d closure attempts", turn.ID, maxIterations)
	}
	return nil
}

// completeTurnIfCurrent closes turnID only when it is still sessionID's active
// turn. Lifecycle delivery uses this after a visible-message persistence
// failure so it cannot complete a turn that existed before this dispatch or a
// successor created concurrently.
func (s *Service) completeTurnIfCurrent(ctx context.Context, sessionID, turnID string) {
	if s.turnService == nil || turnID == "" {
		return
	}
	turn, err := s.turnService.GetActiveTurn(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to look up active turn after lifecycle message persistence failure",
			zap.String("session_id", sessionID), zap.Error(err))
		return
	}
	if turn == nil || turn.ID != turnID {
		return
	}
	if err := s.turnService.CompleteTurn(ctx, turnID); err != nil {
		s.logger.Warn("failed to complete lifecycle turn after message persistence failure",
			zap.String("session_id", sessionID), zap.String("turn_id", turnID), zap.Error(err))
		return
	}
	s.activeTurns.CompareAndDelete(sessionID, turnID)
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
		if isNoActiveTurnError(err) {
			return "", nil
		}
		return "", err
	}
	if turn == nil {
		return "", nil
	}
	return turn.ID, nil
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
	s.resetSendNowWorkers()

	// Reconcile session state from persisted runtime state on startup.
	// This does NOT launch any agent processes — sessions are recovered lazily
	// when the user opens them (via task.session.status → task.session.resume).
	s.reconcileSessionsOnStartup(ctx)
	if s.workflowStore != nil {
		s.workflowStore.ReconcileQueuedTasks(ctx)
	}

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

	// Subscribe to Azure DevOps watcher events
	s.subscribeAzureDevOpsEvents()

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

	// Reconcile queued tasks when WIP limits or feeder settings change.
	s.subscribeWorkflowQueueEvents()

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
	s.stopSendNowWorkers()

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

	report := newStartupCleanupReport()
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
		s.reconcileOneSessionOnStartup(ctx, running, report)
	}
	report.flush(s.logger)

	// Pre-poll remote executor status so task lists show accurate state
	if len(remoteRecords) > 0 && s.agentManager != nil {
		s.agentManager.PollRemoteStatusForRecords(ctx, remoteRecords)
	}
}

// reconcileOneSessionOnStartup adjusts DB state for a single session without launching agents.
func (s *Service) reconcileOneSessionOnStartup(ctx context.Context, running *models.ExecutorRunning, report *startupCleanupReport) {
	sessionID := running.SessionID
	if sessionID == "" {
		return
	}

	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		if isTaskSessionNotFound(err) {
			s.handleMissingSessionOnStartup(ctx, running, report)
			return
		}
		s.logger.Warn("failed to load session for reconciliation; preserving executor record",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}

	previousState := session.State

	// Handle terminal and never-started states (reuse existing cleanup logic)
	if skip := s.handleTerminalSessionOnStartup(ctx, session, running, previousState, report); skip {
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

	s.reconcileActiveSessionOnStartup(ctx, running, sessionID, previousState)
}

func (s *Service) reconcileActiveSessionOnStartup(
	ctx context.Context,
	running *models.ExecutorRunning,
	sessionID string,
	previousState models.TaskSessionState,
) {
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

	// Mark the task interrupted when its session was mid-turn when the backend
	// died (STARTING/RUNNING) so task-list surfaces can show the red
	// interruption icon. WAITING_FOR_INPUT sessions were idle, not interrupted.
	// The write is archive-atomic (SetTaskMetadataKeyIfNotArchived), so an
	// archive that commits after this check cannot leave a stale marker on an
	// archived task. The marker is cleared when a session of the task next
	// enters STARTING/RUNNING (see updateTaskSessionStateWithHook).
	if running.TaskID != "" && (previousState == models.TaskSessionStateStarting || previousState == models.TaskSessionStateRunning) {
		if _, setErr := s.repo.SetTaskMetadataKeyIfNotArchived(ctx, running.TaskID, models.MetaKeyInterruptedAt, time.Now().UTC().Format(time.RFC3339)); setErr != nil {
			s.logger.Warn("failed to mark task interrupted on startup",
				zap.String("task_id", running.TaskID),
				zap.Error(setErr))
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

func (s *Service) handleMissingSessionOnStartup(ctx context.Context, running *models.ExecutorRunning, report *startupCleanupReport) {
	sessionID := running.SessionID
	executionID := strings.TrimSpace(running.AgentExecutionID)
	if executionID == "" || s.agentManager == nil {
		decision := startupCleanupDecision{
			disposition:    startupCleanupDispositionPreserved,
			liveness:       processLivenessClass(s.rowLiveness(running)),
			stopErrorClass: "missing_stop_handle",
			expected:       true,
		}
		report.recordExpected(running, decision)
		s.logger.Debug("executor record has no stoppable runtime handle; preserving record",
			startupCleanupFields(running, decision)...)
		return
	}
	if err := s.agentManager.StopAgentWithReason(ctx, executionID, "startup missing session cleanup", true); err != nil {
		// A not-found stop for a runtime whose row is a confirmed-dead LOCAL
		// process means the runtime is already gone — treat it as an idempotent
		// successful stop and prune/repair the row under the resume-safety
		// invariant, so the orphan row does not survive restarts forever. Any
		// other error (alive/unknown/remote row, or a non-not-found failure) is
		// preserved and left for a later attempt.
		decision := classifyStartupCleanup(err, s.rowLiveness(running))
		if !decision.proceed {
			if decision.expected {
				report.recordExpected(running, decision)
				s.logger.Debug("failed to stop missing-session runtime; preserving executor record",
					startupCleanupFields(running, decision)...)
			} else {
				s.logger.Warn("failed to stop missing-session runtime; preserving executor record",
					startupCleanupFields(running, decision)...)
			}
			return
		}
		s.logger.Info("missing-session runtime already gone (confirmed dead); pruning executor record",
			startupCleanupFields(running, decision)...)
		s.pruneOrRepairExecutorRow(ctx, running, models.TaskSessionStateCancelled)
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
func (s *Service) handleTerminalSessionOnStartup(ctx context.Context, session *models.TaskSession, running *models.ExecutorRunning, previousState models.TaskSessionState, report *startupCleanupReport) bool {
	sessionID := session.ID
	switch previousState {
	case models.TaskSessionStateCompleted, models.TaskSessionStateCancelled:
		s.logger.Info("session in terminal state; stopping runtime before cleaning up executor record",
			zap.String("session_id", sessionID),
			zap.String("task_id", session.TaskID),
			zap.String("state", string(previousState)))
		s.abandonOpenTurnsOnStartup(ctx, sessionID, "terminal session cleanup")
		if !s.stopRuntimeForStartupCleanup(ctx, running, "startup terminal session cleanup", report) {
			return true
		}
		// Resume-safety invariant: prune the row only if it is not still resumable
		// (no resume_token). A terminal session that kept a resume_token — e.g. an
		// office run that COMPLETED a turn but stays resumable — is repaired in
		// place instead of deleted.
		s.pruneOrRepairExecutorRow(ctx, running, previousState)
		return true
	case models.TaskSessionStateFailed:
		s.handleFailedSessionOnStartup(ctx, session, running, report)
		return true
	}
	return false
}

// handleFailedSessionOnStartup handles a failed session during startup recovery.
func (s *Service) handleFailedSessionOnStartup(ctx context.Context, session *models.TaskSession, running *models.ExecutorRunning, report *startupCleanupReport) {
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
		if !s.stopRuntimeForStartupCleanup(ctx, running, "startup failed session cleanup", report) {
			return
		}
		// Prune only subject to the resume-safety invariant (a lingering
		// resume_token is repaired in place rather than deleted).
		s.pruneOrRepairExecutorRow(ctx, running, models.TaskSessionStateFailed)
	}
}

// stopRuntimeForStartupCleanup stops a session's still-registered runtime handle
// during startup reconciliation and reports whether the caller may proceed to
// prune/repair the executor row. It mirrors handleMissingSessionOnStartup's
// classification so terminal and non-resumable-failed reconciliation stop
// leaking stale rows: a not-found stop for a confirmed-dead LOCAL runtime means
// the runtime is already gone and cleanup proceeds; any other error
// (alive/unknown/remote row, or a non-not-found failure) preserves the row for a
// later attempt. A row with no stoppable handle proceeds only when its local
// process liveness is confirmed dead; alive and Unknown rows remain durable.
func (s *Service) stopRuntimeForStartupCleanup(ctx context.Context, running *models.ExecutorRunning, reason string, report *startupCleanupReport) bool {
	executionID := strings.TrimSpace(running.AgentExecutionID)
	if executionID == "" || s.agentManager == nil {
		liveness := s.rowLiveness(running)
		decision := startupCleanupDecision{
			disposition:    startupCleanupDispositionPreserved,
			liveness:       processLivenessClass(liveness),
			stopErrorClass: "missing_stop_handle",
			expected:       true,
		}
		if liveness == models.ProcessLivenessDead {
			decision.disposition = startupCleanupDispositionAlreadyAbsent
			decision.proceed = true
			decision.expected = false
		}
		if !decision.proceed {
			report.recordExpected(running, decision)
			s.logger.Debug("session runtime has no stoppable handle; preserving executor record",
				append(startupCleanupFields(running, decision), zap.String("reason", reason))...)
			return false
		}
		s.logger.Info("session runtime is confirmed dead without a stop handle; proceeding to prune/repair",
			append(startupCleanupFields(running, decision), zap.String("reason", reason))...)
		return true
	}
	err := s.agentManager.StopAgentWithReason(ctx, executionID, reason, true)
	if err == nil {
		return true
	}
	decision := classifyStartupCleanup(err, s.rowLiveness(running))
	if !decision.proceed {
		fields := append(startupCleanupFields(running, decision), zap.String("reason", reason))
		if decision.expected {
			report.recordExpected(running, decision)
			s.logger.Debug("failed to stop session runtime during startup cleanup; preserving executor record", fields...)
		} else {
			s.logger.Warn("failed to stop session runtime during startup cleanup; preserving executor record", fields...)
		}
		return false
	}
	s.logger.Info("session runtime already gone (confirmed dead) during startup cleanup; proceeding to prune/repair",
		append(startupCleanupFields(running, decision), zap.String("reason", reason))...)
	return true
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

func isMissingBranchError(err error) bool {
	for current := err; current != nil; current = errors.Unwrap(current) {
		if isMissingBranchMessage(current.Error()) {
			return true
		}
	}
	return false
}

func isMissingBranchMessage(message string) bool {
	msg := strings.ToLower(message)
	if strings.Contains(msg, "couldn't find remote ref") {
		return true
	}
	if strings.Contains(msg, "pathspec") && strings.Contains(msg, "did not match") &&
		!strings.Contains(msg, "fetch branch failed") {
		return true
	}

	const missingBranchMarker = "not found locally or on remote"
	markerIndex := strings.Index(msg, missingBranchMarker)
	if markerIndex < 0 || !strings.Contains(msg[:markerIndex], "branch") {
		return false
	}

	// The worktree layer appends the underlying fetch failure after this marker.
	// Only the marker by itself is evidence that the remote branch is missing.
	detail := strings.TrimSpace(msg[markerIndex+len(missingBranchMarker):])
	return detail == ""
}

var (
	quotedBranchPattern   = regexp.MustCompile(`branch "([^"]+)"`)
	remoteRefPattern      = regexp.MustCompile(`remote ref ([^\s]+)`)
	pathspecBranchPattern = regexp.MustCompile(`pathspec '([^']+)'`)
)

const launchFailurePRLookupTimeout = time.Second

func extractMissingBranchName(err error) string {
	branch := ""
	for current := err; current != nil; current = errors.Unwrap(current) {
		msg := current.Error()
		if match := quotedBranchPattern.FindStringSubmatch(msg); len(match) == 2 {
			branch = strings.TrimSpace(match[1])
			continue
		}
		if match := remoteRefPattern.FindStringSubmatch(msg); len(match) == 2 {
			branch = strings.TrimSpace(match[1])
			continue
		}
		if match := pathspecBranchPattern.FindStringSubmatch(msg); len(match) == 2 {
			branch = strings.TrimSpace(match[1])
		}
	}
	return branch
}

const missingPRBranchRecoveryClaimKey = "missing_pr_branch_recovery_claimed"

type failedSessionMetadataClaimer interface {
	SetSessionMetadataKeyIfAbsentIfState(
		ctx context.Context,
		sessionID, key string,
		value interface{},
		expectedState models.TaskSessionState,
	) (bool, error)
}

type failedSessionMetadataClaimReleaser interface {
	RemoveSessionMetadataKeyIfState(
		ctx context.Context,
		sessionID, key string,
		expectedState models.TaskSessionState,
	) (bool, error)
}

func (s *Service) matchingTaskPRState(ctx context.Context, taskID, repositoryID, branch string) string {
	repositoryID = strings.TrimSpace(repositoryID)
	if s.githubService == nil || repositoryID == "" || branch == "" {
		return ""
	}
	lookupCtx, cancel := context.WithTimeout(ctx, launchFailurePRLookupTimeout)
	defer cancel()
	prsByTask, err := s.githubService.ListTaskPRs(lookupCtx, []string{taskID})
	if err != nil {
		s.logger.Debug("failed to load task PR state for missing branch guidance",
			zap.String("task_id", taskID),
			zap.Error(err))
		return ""
	}
	prs := prsByTask[taskID]
	if state, found := taskPRState(prs, repositoryID, branch); found {
		return state
	}
	state, found := taskPRState(prs, "", branch)
	if !found || !s.hasOnlyTaskRepository(lookupCtx, taskID, repositoryID) {
		return ""
	}
	return state
}

func taskPRState(prs []*github.TaskPR, repositoryID, branch string) (string, bool) {
	matched := false
	state := ""
	for _, pr := range prs {
		if pr == nil || strings.TrimSpace(pr.RepositoryID) != repositoryID || strings.TrimSpace(pr.HeadBranch) != branch {
			continue
		}
		if matched {
			return "", true
		}
		matched = true
		state = strings.ToLower(strings.TrimSpace(pr.State))
		if state != githubPRStateOpen && state != githubPRStateClosed && state != githubPRStateMerged {
			return "", true
		}
	}
	return state, matched
}

func (s *Service) hasOnlyTaskRepository(ctx context.Context, taskID, repositoryID string) bool {
	store, ok := s.repo.(interface {
		ListTaskRepositories(context.Context, string) ([]*models.TaskRepository, error)
	})
	if !ok {
		return false
	}
	repositories, err := store.ListTaskRepositories(ctx, taskID)
	if err != nil {
		s.logger.Debug("failed to load task repositories for legacy PR guidance",
			zap.String("task_id", taskID),
			zap.Error(err))
		return false
	}
	return len(repositories) == 1 && repositories[0] != nil &&
		strings.TrimSpace(repositories[0].RepositoryID) == repositoryID
}

// repositoryForMissingBranchFailure derives the failed repository from the
// task's checkout configuration when environment preparation failed before the
// executor constructed its repository-scoped launch request. It deliberately
// returns no match for an empty or ambiguous branch so destructive recovery
// actions remain scoped to the repository that actually failed.
func (s *Service) repositoryForMissingBranchFailure(ctx context.Context, taskID, branch string) (string, string) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return "", branch
	}
	store, ok := s.repo.(interface {
		ListTaskRepositories(context.Context, string) ([]*models.TaskRepository, error)
	})
	if !ok {
		return "", branch
	}
	repositories, err := store.ListTaskRepositories(ctx, taskID)
	if err != nil {
		s.logger.Debug("failed to load task repositories for missing-branch guidance",
			zap.String("task_id", taskID), zap.Error(err))
		return "", branch
	}

	prNumber := missingBranchPRNumber(branch)
	var repositoryID string
	resolvedBranch := branch
	for _, repository := range repositories {
		if repository == nil || !taskRepositoryMatchesMissingBranch(repository, branch, prNumber) {
			continue
		}
		if repositoryID != "" {
			return "", branch
		}
		repositoryID = strings.TrimSpace(repository.RepositoryID)
		if checkoutBranch := strings.TrimSpace(repository.CheckoutBranch); checkoutBranch != "" {
			resolvedBranch = checkoutBranch
		}
	}
	return repositoryID, resolvedBranch
}

var missingBranchPRRefPattern = regexp.MustCompile(`^pull/(\d+)/head$`)

func missingBranchPRNumber(branch string) int {
	match := missingBranchPRRefPattern.FindStringSubmatch(strings.TrimSpace(branch))
	if len(match) != 2 {
		return 0
	}
	number, err := strconv.Atoi(match[1])
	if err != nil || number <= 0 {
		return 0
	}
	return number
}

func taskRepositoryMatchesMissingBranch(repository *models.TaskRepository, branch string, prNumber int) bool {
	if strings.TrimSpace(repository.CheckoutBranch) == branch {
		return true
	}
	return prNumber > 0 && taskRepositoryPRNumber(repository.Metadata) == prNumber
}

func taskRepositoryPRNumber(metadata map[string]interface{}) int {
	if metadata == nil {
		return 0
	}
	switch value := metadata["pr_number"].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		if value > 0 && value == float64(int(value)) {
			return int(value)
		}
	}
	return 0
}

// handleSessionLaunchFailed creates branch guidance only after the caller has
// persisted FAILED. The claim is stored on the session because prepare, start,
// and resume may race.
func (s *Service) handleSessionLaunchFailed(ctx context.Context, taskID, sessionID, repositoryID string, launchErr error) {
	if s.messageCreator == nil || !isMissingBranchError(launchErr) {
		return
	}
	recoveryCtx := context.WithoutCancel(ctx)
	rawBranch := extractMissingBranchName(launchErr)
	displayBranch := routingerr.Sanitize(rawBranch)
	resolvedRepositoryID, resolvedBranch := s.repositoryForMissingBranchFailure(recoveryCtx, taskID, rawBranch)
	if repositoryID == "" {
		repositoryID = resolvedRepositoryID
	}
	if repositoryID != "" && repositoryID == resolvedRepositoryID {
		rawBranch = resolvedBranch
		displayBranch = routingerr.Sanitize(resolvedBranch)
	}
	prState := s.matchingTaskPRState(recoveryCtx, taskID, repositoryID, rawBranch)
	if prState == githubPRStateOpen {
		return
	}
	claimer, ok := s.repo.(failedSessionMetadataClaimer)
	if !ok {
		s.logger.Warn("session repository cannot claim missing-branch recovery",
			zap.String("task_id", taskID), zap.String("session_id", sessionID))
		return
	}
	claimed, err := claimer.SetSessionMetadataKeyIfAbsentIfState(
		recoveryCtx, sessionID, missingPRBranchRecoveryClaimKey, true, models.TaskSessionStateFailed,
	)
	if err != nil {
		s.logger.Warn("failed to claim missing-branch recovery",
			zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
		return
	}
	if !claimed {
		return
	}
	if err := s.createMissingPRBranchRecoveryMessage(
		recoveryCtx, taskID, sessionID, displayBranch, prState,
	); err != nil {
		releaser, ok := s.repo.(failedSessionMetadataClaimReleaser)
		if !ok {
			s.logger.Warn("session repository cannot release missing-branch recovery claim after message failure",
				zap.String("task_id", taskID), zap.String("session_id", sessionID))
			return
		}
		if _, releaseErr := releaser.RemoveSessionMetadataKeyIfState(
			recoveryCtx, sessionID, missingPRBranchRecoveryClaimKey, models.TaskSessionStateFailed,
		); releaseErr != nil {
			s.logger.Warn("failed to release missing-branch recovery claim after message failure",
				zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(releaseErr))
		}
	}
}

func (s *Service) createMissingPRBranchRecoveryMessage(
	ctx context.Context,
	taskID, sessionID, branch, prState string,
) error {
	content := "Kandev couldn't fetch the requested branch from the configured repository. Verify the task's repository branch or PR link, then retry."
	if branch != "" {
		content = "Kandev couldn't fetch branch \"" + branch + "\" from the configured repository. Verify the task's repository branch or PR link, then retry."
	}
	authoritativeMissingBranch := prState == githubPRStateClosed || prState == githubPRStateMerged
	if authoritativeMissingBranch {
		content = "This task references a PR branch that no longer exists on remote."
		if branch != "" {
			content = "The remote PR branch \"" + branch + "\" no longer exists."
		}
	}
	metadata := map[string]interface{}{
		"variant":        "warning",
		"failure_kind":   "branch_fetch_failed",
		"missing_branch": branch,
	}
	if authoritativeMissingBranch {
		metadata["failure_kind"] = "missing_pr_branch"
		metadata["actions"] = []map[string]interface{}{
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
		}
	}
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
		return err
	}
	s.suppressToast.Store(sessionID, true)
	return nil
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
