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
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/stepentry"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func isNoActiveTurnError(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

// Common errors
var (
	ErrServiceAlreadyRunning = errors.New("service is already running")
	ErrServiceNotRunning     = errors.New("service is not running")
	ErrRouteActionActiveTurn = errors.New("route actions require a settled turn")
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

	// OfficeSessionIdentity keys an Office task's session identity on the
	// run's own agent instead of the task's runner seat. Off by default;
	// enabling it exposes pre-existing duplicate (task_id, agent_profile_id)
	// rows until the companion unique-index fix has shipped.
	OfficeSessionIdentity bool
}

// AttachmentReader is the narrow attachment-store seam needed when the
// orchestrator delivers claimed descriptors to a passthrough agent.
// Keeping this interface local avoids coupling the coordinator to lifecycle's
// concrete package while preserving the same structural contract.
type AttachmentReader interface {
	OpenClaimed(ctx context.Context, id, taskID, sessionID string) (io.ReadCloser, string, string, int64, error)
}

// LaunchAttachmentClaimer is the narrow task-service seam used to bind staged
// attachment descriptors to an authorized task/session before launch dispatch.
type LaunchAttachmentClaimer interface {
	ClaimMessageAttachments(ctx context.Context, taskID, sessionID string, attachments []v1.MessageAttachment) error
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
	CreatePermissionRequestMessage(ctx context.Context, taskID, sessionID, requestID, pendingID, toolCallID, title, turnID string, options []map[string]interface{}, actionType string, actionDetails map[string]interface{}) (string, error)
	UpdatePermissionMessage(ctx context.Context, taskID, sessionID, requestID, pendingID string, status models.PermissionStatus) error
	ClaimPermissionResolution(ctx context.Context, request models.PermissionResolutionClaimRequest) (*models.PermissionResolutionClaimResult, error)
	FinalizePermissionResolution(ctx context.Context, request models.PermissionResolutionFinalizeRequest) (*models.PermissionResolutionFinalizeResult, error)
	GetPermissionResolutionAudit(ctx context.Context, taskID, sessionID, requestID, pendingID string) (*models.PermissionResolutionAudit, error)
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

// TransientRetryMessageService is the narrow task-service seam used to retire
// persisted retry status messages. The task service owns authorization and
// event-bus publication for both operations.
type TransientRetryMessageService interface {
	ListMessages(ctx context.Context, sessionID string) ([]*models.Message, error)
	DeleteMessage(ctx context.Context, id string) error
}

// SubagentContextRecorder persists a durable relational record of a subagent
// (Task tool) invocation observed on a tool-call frame. It returns nothing —
// a repository failure never fails the enclosing message write, turn, or
// agent stream (AC-27 in
// docs/specs/agents/requirements/subagent-context-persistence.md). Implemented by
// taskservice.Service via an adapter; optional, so an installation that
// never wires it behaves exactly as before.
type SubagentContextRecorder interface {
	RecordSubagentContext(ctx context.Context, req taskservice.RecordSubagentContextRequest)
}

// TurnService is an interface for managing session turns
type TurnService interface {
	StartTurn(ctx context.Context, sessionID string) (*models.Turn, error)
	ReserveTurn(
		ctx context.Context,
		sessionID string,
		recovery *models.PromptDispatchRecovery,
	) (*models.Turn, error)
	MarkReservedTurnDispatchAttempted(ctx context.Context, turn *models.Turn) error
	PublishReservedTurn(ctx context.Context, turn *models.Turn) error
	RollbackReservedTurn(ctx context.Context, sessionID, turnID string) (bool, error)
	ReconcileUnpublishedPromptTurns(ctx context.Context) (int, error)
	CompleteTurn(ctx context.Context, turnID string) error
	GetTurn(ctx context.Context, turnID string) (*models.Turn, error)
	GetActiveTurn(ctx context.Context, sessionID string) (*models.Turn, error)
	UpdateTurn(ctx context.Context, turn *models.Turn) error
	PatchTurnMetadata(ctx context.Context, sessionID, turnID string, updates map[string]interface{}) error
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

// FeederPullReconciler wakes task-service feeder pulls after a manual move's
// lifecycle has completed. The task service remains the owner of candidate
// selection and promotion rules.
type FeederPullReconciler interface {
	ReconcileFeederPulls(ctx context.Context, workflowID, feederStepID string)
}

type taskQueuePromotionPublisher interface {
	PublishTaskQueuePromoted(ctx context.Context, task *models.Task)
}

type TaskStopCleanupFunc func(ctx context.Context, taskID, reason string) error

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

// StepHistoryRecorder persists an ADR 0015 session-step transition audit
// row. Optional — when unset, the auto-advance and deferred move_task_kandev
// transition paths record nothing. Errors are logged and swallowed by the
// caller: the audit trail must never fail the transition it is recording.
type StepHistoryRecorder interface {
	CreateStepTransition(ctx context.Context, sessionID, fromStepID, toStepID string, trigger wfmodels.StepTransitionTrigger, actorID *string, metadata map[string]interface{}) error
}

type asyncStepHistoryRecorder interface {
	EnqueueStepTransition(sessionID, fromStepID, toStepID string, trigger wfmodels.StepTransitionTrigger, actorID *string, metadata map[string]interface{})
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

// DirectPromptPreparer canonicalizes a user-submitted structured prompt before
// the task service persists it. The returned trusted context is the exact
// server-generated content that may be preserved by system-context
// canonicalization.
type DirectPromptPreparer interface {
	PrepareDirectPrompt(ctx context.Context, prompt string, isPassthrough bool) (string, string)
}

// DirectPromptStarter starts a prepared direct-message session while retaining
// the trusted saved-prompt context prepared before message persistence.
type DirectPromptStarter interface {
	StartCreatedSessionWithPromptContext(
		ctx context.Context,
		taskID, sessionID, agentProfileID, prompt string,
		skipMessageRecord, planMode, autoStart bool,
		attachments []v1.MessageAttachment,
		references []v1.EntityReference,
		promptReferenceContext string,
	) (*executor.TaskExecution, error)
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
	// GetWorkspaceGroupOwnerTaskID resolves push-detected PR/MR associations
	// to the workspace-group owner task; see resolveEffectivePushTaskID.
	GetWorkspaceGroupOwnerTaskID(ctx context.Context, taskID string) (string, error)
}

// sessionExecutorStore is the minimal repository interface needed by the orchestrator service.
type sessionExecutorStore interface {
	// Session
	GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error)
	GetActiveTaskSessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error)
	ListActiveTaskSessionsByTaskID(ctx context.Context, taskID string) ([]*models.TaskSession, error)
	SetSessionPrimary(ctx context.Context, sessionID string) error
	SetSessionPrimaryIfNonterminal(ctx context.Context, sessionID string) (bool, error)
	RenameTaskSession(ctx context.Context, id, name string) error
	UpdateTaskSession(ctx context.Context, session *models.TaskSession) error
	UpdateTaskSessionIfCurrentState(ctx context.Context, session *models.TaskSession, expected models.TaskSessionState) (bool, error)
	UpdateTaskSessionStateIfCurrent(ctx context.Context, id string, expected, status models.TaskSessionState, errorMessage string) (bool, time.Time, error)
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
	CreateSessionCommit(ctx context.Context, commit *models.SessionCommit) (bool, error)
	GetSessionCommits(ctx context.Context, sessionID string) ([]*models.SessionCommit, error)
	DeleteSessionCommit(ctx context.Context, id string) error
	// Session listing + delete
	ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error)
	ListNonTerminalSessionsByAgentInstance(ctx context.Context, agentInstanceID string) ([]*models.TaskSession, error)
	DeleteTaskSession(ctx context.Context, id string) error
	// Messages — used by resume to backfill the initial user prompt when a
	// prior launch failed before recordInitialMessage ran.
	ListMessages(ctx context.Context, sessionID string) ([]*models.Message, error)
	// Session history + plan used to build a provider-neutral continuation.
	GetTaskPlan(ctx context.Context, taskID string) (*models.TaskPlan, error)
	// Pending clarification rows — durable guard for on_turn_complete while the user is answering.
	FindActiveClarificationMessagesBySessionID(ctx context.Context, sessionID string) ([]*models.Message, error)
	// Workspace
	GetWorkspace(ctx context.Context, id string) (*models.Workspace, error)
	// Task environment
	GetTaskEnvironment(ctx context.Context, id string) (*models.TaskEnvironment, error)
	GetTaskEnvironmentByTaskID(ctx context.Context, taskID string) (*models.TaskEnvironment, error)
	CreateTaskEnvironment(ctx context.Context, env *models.TaskEnvironment) error
	UpdateTaskEnvironment(ctx context.Context, env *models.TaskEnvironment) error
	DeleteTaskEnvironment(ctx context.Context, id string) error
	// Step-entry CAS markers (see internal/workflow/stepentry) — claim/complete
	// an engine-owned on_enter action at most once per step-entry.
	ClaimStepEntryMarker(ctx context.Context, entryID int64, position int, kind, operationID string, claimedAt time.Time) (bool, error)
	CompleteStepEntryMarker(ctx context.Context, entryID int64, position int, state stepentry.MarkerState, cause string, completedAt time.Time) error
	// GetStepEntryMarkerState reads back a marker's terminal (or in-progress)
	// state after a lost CAS claim, so the caller can tell "already failed"
	// apart from "already done" or "still in progress elsewhere."
	GetStepEntryMarkerState(ctx context.Context, entryID int64, position int) (state stepentry.MarkerState, cause string, found bool, err error)
	// ClearStepDecisionsAndCompleteMarker satisfies AC-B6: clears every
	// decision for (taskID, stepID) and completes the clear_decisions
	// marker in one database transaction, so a crash between the two can
	// never leave the marker showing in_progress after the delete already
	// committed. Only usable when the DecisionStore backing clear_decisions
	// shares this repository's writer DB — see the capability check at
	// dispatchClearDecisionsAtomic's call site.
	ClearStepDecisionsAndCompleteMarker(ctx context.Context, taskID, stepID string, entryID int64, position int, now time.Time) (int64, error)
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

type reservedPromptTurn struct {
	id           string
	done         chan struct{}
	mu           sync.Mutex
	resolved     bool
	accepted     bool
	afterResolve []reservedPromptCallback
}

type reservedPromptCallback struct {
	owner *reservedPromptCallbackOwner
	run   func(context.Context)
}

func newReservedPromptTurn(id string) *reservedPromptTurn {
	return &reservedPromptTurn{id: id, done: make(chan struct{})}
}

func (r *reservedPromptTurn) resolve(accepted bool) []reservedPromptCallback {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.resolved {
		return nil
	}
	r.resolved = true
	r.accepted = accepted
	close(r.done)
	callbacks := r.afterResolve
	r.afterResolve = nil
	return callbacks
}

func (r *reservedPromptTurn) wait(ctx context.Context) (bool, error) {
	select {
	case <-r.done:
		r.mu.Lock()
		accepted := r.accepted
		r.mu.Unlock()
		return accepted, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func (r *reservedPromptTurn) deferUntilResolved(callback reservedPromptCallback) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.resolved {
		return false
	}
	r.afterResolve = append(r.afterResolve, callback)
	return true
}

// reservedPromptCallbackOwner gives deferred reservation reconciliation one
// lifecycle owner. Stop prevents new launches, cancels the shared context, and
// waits until every callback has observed cancellation and returned.
type reservedPromptCallbackOwner struct {
	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	stopped bool
	workers sync.WaitGroup
}

func newReservedPromptCallbackOwner() *reservedPromptCallbackOwner {
	ctx, cancel := context.WithCancel(context.Background())
	return &reservedPromptCallbackOwner{ctx: ctx, cancel: cancel}
}

func (o *reservedPromptCallbackOwner) launch(callback func(context.Context)) bool {
	if o == nil || callback == nil {
		return false
	}
	o.mu.Lock()
	if o.stopped {
		o.mu.Unlock()
		return false
	}
	ctx := o.ctx
	o.workers.Add(1)
	o.mu.Unlock()
	go func() {
		defer o.workers.Done()
		callback(ctx)
	}()
	return true
}

func (o *reservedPromptCallbackOwner) stop() {
	if o == nil {
		return
	}
	o.mu.Lock()
	if !o.stopped {
		o.stopped = true
		o.cancel()
	}
	o.mu.Unlock()
	o.workers.Wait()
}

// Service is the main orchestrator service
type Service struct {
	config        ServiceConfig
	logger        *logger.Logger
	eventBus      bus.EventBus
	taskRepo      scheduler.TaskRepository
	repo          sessionExecutorStore
	promptTargets taskPullRequestTargetStore
	agentManager  executor.AgentManagerClient

	// Components
	queue     *queue.TaskQueue
	executor  *executor.Executor
	scheduler *scheduler.Scheduler
	watcher   *watcher.Watcher

	// Message queue service for queueing messages while agent is running
	messageQueue *messagequeue.Service

	// Message creator for saving agent responses
	messageCreator MessageCreator

	// transientRetryMessages owns durable cleanup of persisted retry notices.
	// It is optional for focused tests and pre-composition callers.
	transientRetryMessages TransientRetryMessageService

	// subagentContexts optionally persists a relational record of subagent
	// (Task tool) invocations recognized on the tool-call frame paths. Nil is
	// safe: both call sites guard on it. See SetSubagentContextRecorder.
	subagentContexts SubagentContextRecorder

	// agentProfileRecentUseRecorder optionally persists the task_create profile
	// selected by a deferred launch after its agent starts successfully.
	agentProfileRecentUseRecorder AgentProfileRecentUseRecorder

	// Turn service for managing session turns
	turnService TurnService

	// Task event publisher for emitting task.updated events.
	// Task service owns the rich payload; orchestrator delegates.
	taskEvents  TaskEventPublisher
	feederPulls FeederPullReconciler
	// taskStopCleanup reaps task-owned non-session runtimes before REVIEW.
	taskStopCleanup TaskStopCleanupFunc

	// launchAttachmentClaimer binds staged descriptors before any launch intent
	// can dispatch them to the runtime. Inline attachments need no claim.
	launchAttachmentClaimer LaunchAttachmentClaimer

	// sessionAccessCheck enforces per-user workspace scoping on the
	// session-keyed WS actions. Nil = unscoped. See SetSessionAccessChecker.
	sessionAccessCheck func(ctx context.Context, sessionID string) error

	// taskAccessCheck is the task-keyed sibling of sessionAccessCheck, for
	// entry points that name a task rather than a session (session.launch,
	// session.ensure). Nil = unscoped.
	taskAccessCheck func(ctx context.Context, taskID string) error

	// routeActionHandler is owned by the dynamic conductor composition. The
	// orchestrator only validates/authorizes the request and returns the
	// authoritative route snapshot; concrete and dynamic callers share this
	// seam.
	routeActionHandler func(context.Context, RouteActionRequest) (*RouteActionResult, error)

	// profileExecutionResolver is the shared logical-to-concrete profile
	// boundary used by task and workflow launch paths.
	profileExecutionResolver *agentruntime.ProfileExecutionResolver

	// dynamicRecovery owns durable reset/retry deadlines. Only pending states
	// are scheduled after restart; a retrying state means dispatch may have
	// crossed the process boundary and remains manual until reconciled.
	dynamicRecoveryMu     sync.Mutex
	dynamicRecoveryCtx    context.Context
	dynamicRecoveryCancel context.CancelFunc
	dynamicRecoveryTimers map[string]*time.Timer

	// titleBranchRuntime performs the lifecycle-owned Git branch rename after
	// an agent resolves a prompt-first task title. It is optional for tests and
	// installations that do not configure an agent runtime.
	titleBranchRuntime titleBranchRuntime

	// Workflow step getter for prompt building
	workflowStepGetter WorkflowStepGetter

	// stepHistoryRecorder persists the ADR 0015 audit row for orchestrator
	// step transitions: auto-advance (StepTransitionTriggerAutoComplete) and
	// the deferred move_task_kandev path (StepTransitionTriggerManual).
	// Optional.
	stepHistoryRecorder StepHistoryRecorder

	// Reads task dependency state for the auto-start gate and the
	// dependency-resolution reaction. Nil-safe: when unset, no task has
	// dependencies so nothing is gated.
	dependencyReader TaskDependencyReader

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
	// queuedMoveExitStart and queuedMoveExitComplete are package-test hooks
	// for the durable source-exit barrier.
	onQueuedMoveExitStart             func()
	onQueuedMoveExitComplete          func()
	onTaskQueuePromotionEntryComplete func()
	// onManualMoveLifecycleStart blocks the admitted manual-move lifecycle in
	// package tests so feeder promotion ordering can be asserted.
	onManualMoveLifecycleStart func()
	// onQueuedMessageExecutionComplete is a package-test hook that fires after
	// an asynchronous queued-message worker has released its dispatch
	// reservation. It lets tests wait for the full worker lifecycle instead of
	// observing only the prompt call.
	onQueuedMessageExecutionComplete func()
	// queuedMoveLifecycleLocks serializes source-exit work per task. The
	// completion marker remains durable so a restart can safely resume work.
	queuedMoveLifecycleLocks sync.Map
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

	// Review participant seats (REQ-OFFICE-REVIEW-SEATS-001) dependencies —
	// also nil-safe. buildWorkflowCallbacks registers ensure_participant_seat
	// once engineParticipantSeatWriter is set; engineParticipantSeatCaster
	// may still be nil at that point (wired separately once the Office seat
	// caster exists), in which case EnsureParticipantSeatCallback itself
	// reports ErrActionNotYetWired only for entries that actually need to
	// cast a new seat.
	engineParticipantSeatWriter engine.ParticipantSeatWriter
	engineParticipantSeatCaster engine.ParticipantSeatCaster

	// engineAgentProfiles wires the quorum guard's AgentProfileResolver
	// (REQ-OFFICE-REVIEW-SEATS-004.3): dropping a required seat whose agent
	// profile was deleted after the seat was cast. Also nil-safe — an
	// unwired resolver leaves every seat counted, matching prior behavior.
	engineAgentProfiles engine.AgentProfileResolver

	// Native code review. When set, buildWorkflowCallbacks registers the
	// run_code_review on_enter action. Nil-safe: without it the action kind
	// simply has no callback and the engine treats it as a no-op.
	reviewRunner ReviewRunner

	// GitHub service for PR auto-detection on push
	githubService GitHubService
	// ciAutomationInFlight serializes each PR's evaluation and coalesces one
	// follow-up request instead of dropping an event that arrives mid-run.
	ciAutomationInFlight ciAutomationCoordinator

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

	// taskLaunchRecoveryWorktree resolves live remote defaults for the explicit
	// task.launch.recover action. It is nil in isolated orchestrator tests.
	taskLaunchRecoveryWorktree taskLaunchRecoveryWorktree
	// taskLaunchRecoveryTasks is the task-service seam used by the same action
	// for exact base-branch writes, remote-branch validation, and terminal moves.
	taskLaunchRecoveryTasks taskLaunchRecoveryTaskService
	// taskLaunchRecoveryRepo carries the repository reads and repository-default
	// writes that are outside sessionExecutorStore's common contract.
	taskLaunchRecoveryRepo taskLaunchRecoveryRepository

	// idleReaper is the periodic background loop that calls
	// reclaimIdleSession on rows older than idleReaperMinIdle. Nil-safe:
	// callers that don't need the reaper (most tests, ephemeral
	// orchestrator instances) leave it nil and startIdleSessionReaper
	// / stopIdleSessionReaper no-op. See idle_session_reaper.go.
	idleReaper *idleSessionReaper

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
	// reservedPromptTurns keeps durably created turns private until agentctl
	// accepts their prompt. Each reservation broadcasts its accepted-or-rolled-
	// back result so a concurrent ready event can wait, then revalidate prompt
	// generation before touching turn or workflow state.
	reservedPromptTurns sync.Map
	// reservedPromptCallbacks owns the reconciliation callbacks released when
	// a prompt reservation settles. The owner is replaced on Start and drained
	// before scheduler/watcher teardown on Stop.
	reservedPromptCallbacksMu sync.Mutex
	reservedPromptCallbacks   *reservedPromptCallbackOwner

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
	// agentReadyReservationWaitTimeout is a deterministic test seam. Zero uses
	// the production timeout derived from the prompt dispatch budgets.
	agentReadyReservationWaitTimeout time.Duration

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

	// taskSessionErrorLocks serialize session deletion with retained-error
	// selection and publication for each task. Entries are reference-counted
	// and reclaimed after the last concurrent operation releases them.
	taskSessionErrorLocksMu sync.Mutex
	taskSessionErrorLocks   map[string]*taskSessionErrorGuard

	// completedExecutions records execution IDs that have reached a terminal
	// agent lifecycle event. Buffered stream/tool events for these executions
	// must not wake their session back to RUNNING after the terminal path makes
	// it promptable again. Entries expire after a short grace window so the
	// guard does not grow without bound in long-running backend processes.
	completedExecutions sync.Map

	// readyTurnMarks records, per (session, execution, prompt generation),
	// the turn ID handleAgentReady confirmed and is about to close via
	// completeTurnForSession. handleCompleteStreamEvent reads this snapshot
	// (on both the terminal and non-terminal path) instead of trusting live
	// active-turn state or the terminal-execution marker: agent.ready is
	// published before the complete-stream frame for the same completion
	// (see markReadyTurn's doc comment for the ordering guarantee and its
	// NATS-deployment caveat), so both of those alternatives can already
	// find the turn gone or stale by the time this runs. Entries are
	// consumed on read and expire after the same grace window as
	// completedExecutions so an unread entry cannot grow the map unbounded.
	// This map is per-process — see markReadyTurn's doc comment for why a
	// horizontally-scaled NATS deployment can miss cross-instance.
	readyTurnMarks sync.Map

	// readyTurnMarksZeroGen is readyTurnMarks' sibling for promptGeneration==0
	// completions (transports with no generation tracking at all), which share
	// one key per (session, execution) with no generation to disambiguate
	// them — so entries queue FIFO here instead of occupying a single slot in
	// readyTurnMarks. Guarded by readyTurnMarksZeroGenMu since sync.Map has no
	// atomic append. See markReadyTurn's doc comment.
	readyTurnMarksZeroGenMu sync.Mutex
	readyTurnMarksZeroGen   map[string][]readyTurnMark

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
	// sessionLifecycleLocks serializes context resets with every path that can
	// resume a session from executors_running. The lock is intentionally shared
	// by resetAgentContext and ensureSessionRunning so a lazy resume cannot read
	// the old token while a workflow reset is clearing it.
	// Entries are not deleted: deleting a lock can let a new caller create a
	// second mutex while an existing waiter still owns the old one.
	sessionLifecycleLocks sync.Map // map[sessionID]*sync.Mutex
	// turnCompletionLocks serializes processOnTurnCompleteViaEngineWithCause
	// per session. Several independent event sources (normal agent turn
	// completion, agent-exit, step_complete_kandev's out-of-band signal,
	// user cancellation) can each call it for the same session; without
	// serialization, two concurrent calls can both read the same
	// pre-transition engine state and each call applyEngineTransition,
	// allocating two independent workflow_step_entries rows for what should
	// be a single logical transition and double-dispatching that entry's
	// on_enter actions (clear_decisions, queue_run_for_each_participant).
	// Entries are not deleted — see sessionLifecycleLocks above for why.
	turnCompletionLocks sync.Map // map[sessionID]*sync.Mutex
	// turnCompletionConsumedGeneration records, per session, the
	// TaskSession.UpdatedAt of the most recent session snapshot that was
	// already let past acquireTurnCompletionCriticalSection. It closes the
	// gap turnCompletionLocks' contention signal cannot: two calls whose
	// entire lifetimes are fully sequential (the second's own pre-lock
	// GetTask already reflects the first's commit, so the WorkflowStepID
	// comparison passes trivially) never contend on the mutex at all. A
	// caller's session argument only carries a newer UpdatedAt than the
	// stored generation when something legitimately touched the session
	// (a new turn starting, cancellation reconciliation, ...) after the
	// last transition was consumed; a stale or identical snapshot — the
	// hallmark of a duplicate delivery for the same physical turn — is
	// rejected instead of being re-evaluated against the step the winner
	// already moved to. See acquireTurnCompletionCriticalSection.
	// Entries are not deleted — see sessionLifecycleLocks above for why.
	turnCompletionConsumedGeneration sync.Map // map[sessionID]time.Time
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

	// dynamicAttemptEvidence is keyed by logical session and stores the current
	// concrete or dynamic prompt attempt. It is replaced for every attempt, and
	// its execution ID and prompt generation fence late stream/lifecycle events
	// from a predecessor. Automatic recovery requires an explicit no-output,
	// no-effect result from this map.
	dynamicAttemptEvidence sync.Map

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

	// ciAutomationWorkers owns the asynchronous per-PR automation loops. The
	// service-owned context lets Stop cancel in-flight evaluations and prevents
	// workers from outliving the orchestrator across a restart.
	ciAutomationMu      sync.Mutex
	ciAutomationCtx     context.Context
	ciAutomationCancel  context.CancelFunc
	ciAutomationStopped bool
	ciAutomationWorkers sync.WaitGroup
}

type RouteAction string

const (
	RouteActionRetry      RouteAction = "retry"
	RouteActionTryNext    RouteAction = "try_next"
	RouteActionSkip       RouteAction = "skip"
	RouteActionCancelWait RouteAction = "cancel_wait"
	RouteActionStop       RouteAction = "stop"
)

type RouteActionRequest struct {
	SessionID          string
	Action             RouteAction
	ExpectedGeneration int64
}

type RouteActionResult struct {
	SessionID          string     `json:"session_id"`
	LogicalProfileID   string     `json:"logical_profile_id"`
	ExecutionProfileID string     `json:"execution_profile_id,omitempty"`
	RouteGeneration    int64      `json:"route_generation"`
	ProfileVersion     int64      `json:"profile_version"`
	State              string     `json:"state"`
	Reason             string     `json:"reason,omitempty"`
	ErrorCode          string     `json:"error_code,omitempty"`
	ErrorClass         string     `json:"error_class,omitempty"`
	CatalogueVersion   string     `json:"catalogue_version,omitempty"`
	RetryOrdinal       int64      `json:"retry_ordinal,omitempty"`
	Deadline           *time.Time `json:"deadline,omitempty"`
	PendingOutcome     string     `json:"pending_outcome,omitempty"`
}

// RouteActionConflictError carries the authoritative route snapshot when a
// client submits an old generation. Callers can render the returned snapshot
// and retry with its generation without issuing a second read request.
type RouteActionConflictError struct {
	Result *RouteActionResult
	Err    error
}

func (e *RouteActionConflictError) Error() string {
	if e == nil || e.Err == nil {
		return "route action conflict"
	}
	return e.Err.Error()
}

func (e *RouteActionConflictError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (s *Service) SetRouteActionHandler(handler func(context.Context, RouteActionRequest) (*RouteActionResult, error)) {
	s.routeActionHandler = handler
}

// SetProfileExecutionResolver wires the shared profile-kind boundary into
// task and workflow launches.
func (s *Service) SetProfileExecutionResolver(resolver *agentruntime.ProfileExecutionResolver) {
	s.profileExecutionResolver = resolver
}

func (s *Service) ApplyRouteAction(ctx context.Context, request RouteActionRequest) (*RouteActionResult, error) {
	if request.SessionID == "" {
		return nil, errors.New("session_id is required")
	}
	if request.Action != RouteActionRetry && request.Action != RouteActionTryNext &&
		request.Action != RouteActionSkip && request.Action != RouteActionCancelWait &&
		request.Action != RouteActionStop {
		return nil, fmt.Errorf("unsupported route action %q", request.Action)
	}
	if s.routeActionHandler == nil {
		return nil, errors.New("dynamic route actions are not configured")
	}
	if err := s.authorizeSession(ctx, request.SessionID); err != nil {
		return nil, err
	}
	lock, release := s.acquireCancelInFlightGuard(request.SessionID)
	lock.Lock()
	defer func() {
		lock.Unlock()
		release()
	}()
	if s.turnService != nil {
		activeTurn, err := s.turnService.GetActiveTurn(ctx, request.SessionID)
		if err != nil && !isNoActiveTurnError(err) {
			return nil, fmt.Errorf("check active turn for route action: %w", err)
		}
		if activeTurn != nil {
			return nil, ErrRouteActionActiveTurn
		}
	}
	return s.routeActionHandler(ctx, request)
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
	var taskLaunchRecoveryRepo taskLaunchRecoveryRepository
	if recoveryRepo, ok := repo.(taskLaunchRecoveryRepository); ok {
		taskLaunchRecoveryRepo = recoveryRepo
	}

	// Create the service (watcher will be created after we have handlers)
	sendNowCtx, sendNowCancel := context.WithCancel(context.Background())
	ciAutomationCtx, ciAutomationCancel := context.WithCancel(context.Background())
	s := &Service{
		config:                       cfg,
		logger:                       svcLogger,
		eventBus:                     eventBus,
		taskRepo:                     taskRepo,
		repo:                         repo,
		promptTargets:                repo,
		agentManager:                 agentManager,
		queue:                        taskQueue,
		executor:                     exec,
		scheduler:                    sched,
		messageQueue:                 msgQueue,
		taskLaunchRecoveryRepo:       taskLaunchRecoveryRepo,
		clarificationWatchdogTimeout: 15 * time.Second,
		gitSnapshotCache:             newGitSnapshotCache(),
		dynamicRecoveryTimers:        make(map[string]*time.Timer),
		reservedPromptCallbacks:      newReservedPromptCallbackOwner(),
		sendNowCtx:                   sendNowCtx,
		sendNowCancel:                sendNowCancel,
		ciAutomationCtx:              ciAutomationCtx,
		ciAutomationCancel:           ciAutomationCancel,
		idleReaper:                   newIdleSessionReaper(),
	}
	// Always publish queue-status after a task-scoped queue purge so the
	// status-summary projector zeros queued_prompt_count. Unlike the
	// ephemeral purger above, this must never call PurgeTask on the SQLite
	// queue — the rows are already gone in-transaction.
	if registrar, ok := repo.(interface {
		SetTaskQueuePurgeNotifier(func(context.Context, string))
	}); ok {
		registrar.SetTaskQueuePurgeNotifier(func(ctx context.Context, taskID string) {
			s.publishTaskQueueStatusEvent(ctx, taskID, "")
		})
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
	exec.SetLaunchFailureReviewEligibility(s.resolveLaunchFailureReviewEligibility)
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
		OnTaskCreated:          s.handleTaskCreated,
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

// SetTransientRetryMessageService wires the task service used to retire
// persisted transient-retry status messages.
func (s *Service) SetTransientRetryMessageService(service TransientRetryMessageService) {
	s.transientRetryMessages = service
}

// SetSubagentContextRecorder wires the optional subagent-context writer.
// If not set, subagent tool-call frames are recognized and rendered exactly
// as before; only the durable relational record is skipped.
func (s *Service) SetSubagentContextRecorder(r SubagentContextRecorder) {
	s.subagentContexts = r
}

// SetAttachmentReader wires the backend attachment store into passthrough
// prompt delivery so claimed descriptors can be streamed into the workspace.
func (s *Service) SetAttachmentReader(reader AttachmentReader) {
	s.executor.SetAttachmentReader(reader)
}

// SetLaunchAttachmentClaimer wires staged-descriptor admission into the
// unified session launch boundary.
func (s *Service) SetLaunchAttachmentClaimer(claimer LaunchAttachmentClaimer) {
	s.launchAttachmentClaimer = claimer
}

// SetOnPrimarySessionSet sets a callback on the executor for when the first session
// of a task is marked primary. Used to publish a task.updated event so the frontend
// receives the primary_session_id.
func (s *Service) SetOnPrimarySessionSet(fn executor.PrimarySessionSetFunc) {
	s.executor.SetOnPrimarySessionSet(fn)
}

// SetOnTaskEnvironmentReady wires task-level consumers after canonical
// environment persistence.
func (s *Service) SetOnTaskEnvironmentReady(fn executor.TaskEnvironmentReadyFunc) {
	s.executor.SetOnTaskEnvironmentReady(fn)
}

// SetOnTaskStopCleanup wires cleanup for task-owned runtimes that are not
// represented by active sessions, such as task-scoped language servers.
func (s *Service) SetOnTaskStopCleanup(fn TaskStopCleanupFunc) {
	s.taskStopCleanup = fn
}

// SetRepoCloner sets the repository cloner and updater on the executor, enabling automatic
// cloning of provider-backed repositories (e.g. from a GitHub URL) when they are launched
// for local/worktree execution and have no local path yet.
func (s *Service) SetRepoCloner(cloner executor.RepoCloner, updater executor.RepoUpdater) {
	s.executor.SetRepoCloner(cloner, updater)
}

// SetTaskRepositoryBaseBranchUpdater wires exact task-repository fallback
// self-healing into the executor.
func (s *Service) SetTaskRepositoryBaseBranchUpdater(updater executor.TaskRepositoryBaseBranchUpdater) {
	s.executor.SetTaskRepositoryBaseBranchUpdater(updater)
}

// SetPRBaseResolver wires best-effort pull-request base resolution at launch.
func (s *Service) SetPRBaseResolver(resolver executor.PRBaseResolver) {
	s.executor.SetPRBaseResolver(resolver)
}

// RepositoryHostCloner is the narrow host-materialization contract. It returns
// only the persisted local path; clone credentials remain private to the
// orchestrator executor pipeline.
type RepositoryHostCloner interface {
	EnsureRepositoryClonedForSession(
		context.Context, string, string, *models.Repository,
	) (string, error)
}

// EnsureRepositoryCloned delegates host cloning through the executor's
// authenticated clone pipeline.
func (s *Service) EnsureRepositoryCloned(ctx context.Context, repository *models.Repository) (string, error) {
	return s.executor.EnsureRepositoryCloned(ctx, repository)
}

// EnsureRepositoryClonedForSession delegates host cloning with exact task and
// session scope for plugin repository credentials.
func (s *Service) EnsureRepositoryClonedForSession(
	ctx context.Context, taskID, sessionID string, repository *models.Repository,
) (string, error) {
	return s.executor.EnsureRepositoryClonedForSession(ctx, taskID, sessionID, repository)
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
// This dependency must be set before Start. Startup reconciliation fails closed
// without durable turn access because it cannot safely classify unpublished
// prompt reservations.
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

// SetFeederPullReconciler wires the task-service reconciliation callback used
// after an admitted manual move lifecycle has completed.
func (s *Service) SetFeederPullReconciler(reconciler FeederPullReconciler) {
	s.feederPulls = reconciler
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
	queuePromotion := false
	if task.Metadata != nil {
		_, queuePromotion = task.Metadata[models.MetaKeyQueuePromotionPending]
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
		"wip_admitted":              task.WIPAdmitted,
		"queued_for_step_id":        task.QueuedForStepID,
		"queue_promotion":           queuePromotion,
	}
	if task.QueuedAt != nil {
		data["queued_at"] = task.QueuedAt.Format(time.RFC3339)
	} else {
		data["queued_at"] = nil
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

// SetStepHistoryRecorder wires the ADR 0015 audit-trail writer for
// auto-advance step transitions (engine and legacy on_turn_complete paths)
// and the deferred move_task_kandev path (applyPendingMove). Optional.
func (s *Service) SetStepHistoryRecorder(recorder StepHistoryRecorder) {
	s.stepHistoryRecorder = recorder
	// The transition store is created when the workflow step getter is wired.
	// Rebuild it so queue-promotion paths receive the same recorder.
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

// ClarificationCanceller updates pending clarification ownership when an agent
// turn detaches or its session becomes terminal.
type ClarificationCanceller interface {
	DetachSessionAndNotify(ctx context.Context, sessionID string) (int, error)
	ExpireSessionAndNotify(ctx context.Context, sessionID string) (int, error)
}

// SetClarificationCanceller sets the canceller for turn and terminal-session cleanup.
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
	store := newWorkflowStore(s.repo, s.workflowStepGetter, s.agentManager, s.publishTaskUpdated, s.logger, s.publishTaskMoved, s.publishTaskQueuePromoted, s.publishTaskStateChanged, s.stepHistoryRecorder)
	store.setGuardedTransitionLifecycle(s.applyGuardedTransitionLifecycle)
	callbacks := buildWorkflowCallbacks(s)
	s.workflowStore = store
	// AC-24/24a: the engine's own structured "guard did not fire" log needs
	// the service's logger. Passed here (rather than folded into
	// s.engineOptions via a Set* method) because s.logger is a stable
	// constructor-time field already in scope, unlike the optional
	// dependencies those methods wire in after Service creation.
	options := append([]engine.Option{engine.WithLogger(s.logger)}, s.engineOptions...)
	s.workflowEngine = engine.New(store, callbacks, options...)
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

// SetEngineParticipantSeatWriter wires the engine's ParticipantSeatWriter
// for the ensure_participant_seat action. Unlike ParticipantStore/
// DecisionStore, this is a pure callback-construction dependency — nothing
// else in the engine reads it — so it is not also appended as an
// engine.Option.
func (s *Service) SetEngineParticipantSeatWriter(writer engine.ParticipantSeatWriter) {
	s.engineParticipantSeatWriter = writer
	s.reinitWorkflowEngine()
}

// SetEngineParticipantSeatCaster wires the engine's ParticipantSeatCaster
// for the ensure_participant_seat action (REQ-002's casting resolution).
func (s *Service) SetEngineParticipantSeatCaster(caster engine.ParticipantSeatCaster) {
	s.engineParticipantSeatCaster = caster
	s.reinitWorkflowEngine()
}

// SetEngineAgentProfileResolver wires the engine's AgentProfileResolver for
// the quorum guard's REQ-OFFICE-REVIEW-SEATS-004.3 skip.
func (s *Service) SetEngineAgentProfileResolver(resolver engine.AgentProfileResolver) {
	s.engineAgentProfiles = resolver
	s.engineOptions = append(s.engineOptions, engine.WithAgentProfileResolver(resolver))
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
	turnID, created, _, err := s.startTurnForSessionWithOwnershipChecked(ctx, sessionID, false, nil)
	if err != nil {
		s.logger.Warn("failed to start turn",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return "", false
	}
	return turnID, created
}

// startTurnForSessionWithOwnershipChecked makes persistence failure part of
// prompt admission. A caller may reserve a newly created turn so turn.started
// is published only after the external dispatch is acknowledged.
func (s *Service) startTurnForSessionWithOwnershipChecked(
	ctx context.Context,
	sessionID string,
	reserve bool,
	recovery *models.PromptDispatchRecovery,
) (string, bool, *models.Turn, error) {
	if s.turnService == nil {
		return "", false, nil, nil
	}
	if reservedID := s.reservedPromptTurnID(sessionID); reservedID != "" {
		return "", false, nil, fmt.Errorf(
			"%w: session %s has unpublished reserved turn %s",
			ErrAgentPromptInProgress,
			sessionID,
			reservedID,
		)
	}

	if turnIDVal, ok := s.activeTurns.Load(sessionID); ok {
		if turnID, ok := turnIDVal.(string); ok && turnID != "" {
			s.bindAcceptedDispatchTurn(sessionID, turnID)
			return turnID, false, nil, nil
		}
	}

	turn, err := s.turnService.GetActiveTurn(ctx, sessionID)
	if err != nil {
		return "", false, nil, fmt.Errorf("look up active turn: %w", err)
	}
	if turn != nil {
		s.activeTurns.Store(sessionID, turn.ID)
		s.bindAcceptedDispatchTurn(sessionID, turn.ID)
		return turn.ID, false, nil, nil
	}

	if reserve {
		turn, err = s.turnService.ReserveTurn(ctx, sessionID, recovery)
	} else {
		turn, err = s.turnService.StartTurn(ctx, sessionID)
	}
	if err != nil {
		return "", false, nil, fmt.Errorf("persist turn: %w", err)
	}

	if reserve {
		s.reservedPromptTurns.Store(sessionID, newReservedPromptTurn(turn.ID))
		return turn.ID, true, turn, nil
	}
	s.activeTurns.Store(sessionID, turn.ID)
	s.bindAcceptedDispatchTurn(sessionID, turn.ID)
	return turn.ID, true, nil, nil
}

func (s *Service) reservedPromptTurnID(sessionID string) string {
	reservation := s.reservedPromptTurn(sessionID)
	if reservation == nil {
		return ""
	}
	return reservation.id
}

func (s *Service) reservedPromptTurn(sessionID string) *reservedPromptTurn {
	value, ok := s.reservedPromptTurns.Load(sessionID)
	if !ok {
		return nil
	}
	reservation, _ := value.(*reservedPromptTurn)
	return reservation
}

func (s *Service) resolveReservedPromptTurn(sessionID, turnID string, accepted bool) bool {
	reservation := s.reservedPromptTurn(sessionID)
	if reservation == nil || reservation.id != turnID {
		return false
	}
	s.reservedPromptTurns.CompareAndDelete(sessionID, reservation)
	callbacks := reservation.resolve(accepted)
	for _, callback := range callbacks {
		if callback.owner != nil {
			callback.owner.launch(callback.run)
		}
	}
	return true
}

func (s *Service) deferReservedPromptCallback(
	reservation *reservedPromptTurn,
	callback func(context.Context),
) bool {
	if reservation == nil || callback == nil {
		return false
	}
	s.reservedPromptCallbacksMu.Lock()
	owner := s.reservedPromptCallbacks
	if owner == nil {
		owner = newReservedPromptCallbackOwner()
		s.reservedPromptCallbacks = owner
	}
	s.reservedPromptCallbacksMu.Unlock()
	return reservation.deferUntilResolved(reservedPromptCallback{owner: owner, run: callback})
}

func (s *Service) resetReservedPromptCallbacks() {
	next := newReservedPromptCallbackOwner()
	s.reservedPromptCallbacksMu.Lock()
	previous := s.reservedPromptCallbacks
	s.reservedPromptCallbacks = next
	s.reservedPromptCallbacksMu.Unlock()
	previous.stop()
}

func (s *Service) stopReservedPromptCallbacks() {
	s.reservedPromptCallbacksMu.Lock()
	owner := s.reservedPromptCallbacks
	s.reservedPromptCallbacksMu.Unlock()
	owner.stop()
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
	s.completeTurnForTaskSessionWithSuccessorPolicy(ctx, taskID, sessionID, true)
}

// completeTurnForTaskSessionWithSuccessorPolicy reconciles active turns while
// optionally preserving an accepted replacement. Only a stream event that has
// been proven to belong to a superseded prompt may preserve that replacement;
// the current successor's own terminal event must settle it normally.
func (s *Service) completeTurnForTaskSessionWithSuccessorPolicy(
	ctx context.Context,
	taskID, sessionID string,
	preserveAcceptedSuccessor bool,
) {
	// Stream-only completion of a cancelled predecessor must not wipe a
	// Send Now / FIFO successor that has already claimed prompt ownership.
	// The ready-path wrapper (completeTurnForSession) still clears the
	// marker when the successor turn itself settles, so the next queue
	// action is not blocked forever.
	if preserveAcceptedSuccessor && s.acceptedDispatchInFlight(sessionID) {
		if successor := s.acceptedDispatchSuccessorTurn(sessionID); successor != "" {
			if err := s.completeTurnsExcept(ctx, sessionID, successor); err != nil {
				s.logger.Warn("failed to reconcile predecessor turn while successor dispatch is accepted",
					zap.String("session_id", sessionID),
					zap.String("successor_turn_id", successor),
					zap.Error(err))
			}
		}
		return
	}
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

func (s *Service) completeTurnsExcept(ctx context.Context, sessionID, keepTurnID string) error {
	if s.turnService == nil || keepTurnID == "" {
		return nil
	}

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
		if turn == nil || turn.ID == keepTurnID {
			s.activeTurns.Store(sessionID, keepTurnID)
			return nil
		}
		if err := s.turnService.CompleteTurn(ctx, turn.ID); err != nil {
			return fmt.Errorf("complete predecessor turn %s: %w", turn.ID, err)
		}
		closed++
	}
	active, err := s.turnService.GetActiveTurn(ctx, sessionID)
	if err != nil {
		if isNoActiveTurnError(err) {
			return fmt.Errorf("closed %d predecessor turns but successor %s was not found", closed, keepTurnID)
		}
		return fmt.Errorf("verify successor turn %s after closing %d predecessors: %w", keepTurnID, closed, err)
	}
	if active == nil {
		return fmt.Errorf("closed %d predecessor turns but successor %s was not found", closed, keepTurnID)
	}
	if active.ID != keepTurnID {
		return fmt.Errorf("closed %d predecessor turns but active turn is %s, want successor %s", closed, active.ID, keepTurnID)
	}
	s.activeTurns.Store(sessionID, keepTurnID)
	return nil
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
	if turnID := s.reservedPromptTurnID(sessionID); turnID != "" {
		return turnID
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

// acquireSessionLifecycleLock serializes operations that change or consume a
// session's persisted conversation identity. Keep the critical section around
// the complete reset/resume operation, including the bounded runtime wait, so
// no caller can launch with a token that a concurrent reset is invalidating.
func (s *Service) acquireSessionLifecycleLock(sessionID string) func() {
	if sessionID == "" {
		return func() {}
	}
	value, _ := s.sessionLifecycleLocks.LoadOrStore(sessionID, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	return lock.Unlock
}

// acquireTurnCompletionLock serializes on_turn_complete processing for a
// single session — see turnCompletionLocks' field comment for the race it
// closes. A caller with no session ID (defensive callers pass "" rather than
// skip the lock call) gets a no-op unlock and contended=false.
//
// contended reports whether another caller already held this session's lock
// at the moment this call tried to acquire it — i.e. another
// processOnTurnCompleteViaEngineWithCause call for the same session is (or
// very recently was) actively executing right now. That is a direct,
// timing-independent duplicate signal: unlike comparing two DB reads taken
// at different times (which a sufficiently late duplicate can pass
// trivially — see acquireTurnCompletionCriticalSection), lock contention can
// only be true when two calls for the same session genuinely overlapped.
func (s *Service) acquireTurnCompletionLock(sessionID string) (unlock func(), contended bool) {
	if sessionID == "" {
		return func() {}, false
	}
	value, _ := s.turnCompletionLocks.LoadOrStore(sessionID, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	if lock.TryLock() {
		return lock.Unlock, false
	}
	lock.Lock()
	return lock.Unlock, true
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
	s.resetReservedPromptCallbacks()
	s.resetSendNowWorkers()
	s.resetCIAutomationWorkers()

	// Reconcile session state from persisted runtime state on startup.
	// This does NOT launch any agent processes — sessions are recovered lazily
	// when the user opens them (via task.session.status → task.session.resume).
	if s.turnService == nil {
		err := errors.New("reconcile unpublished prompt turns on startup: turn service is unavailable")
		s.logger.Error("failed to reconcile unpublished prompt turns on startup", zap.Error(err))
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		return err
	}
	if err := s.reconcileUnpublishedPromptTurnsOnStartup(ctx); err != nil {
		s.logger.Error("failed to reconcile unpublished prompt turns on startup", zap.Error(err))
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		return err
	}
	s.reconcileExecutorSessionsOnStartup(ctx)
	if s.workflowStore != nil {
		s.workflowStore.ReconcileQueuedTasks(ctx)
	}
	s.reconcileTaskLifecycleTokens(ctx)
	// Chains whose predecessor completed while the process was down: the
	// dependencies_resolved event is in-memory and is not replayed, so without
	// this sweep a chain stalls silently across a restart. Runs after the WIP
	// reconciler so an admitted-by-promotion task is already eligible.
	s.reconcileDependencyLaunchesOnStartup(ctx)

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

	// Invalidate the compiled-step cache when workflow steps change.
	s.subscribeWorkflowStepCacheEvents()

	// Restore durable dynamic policy waits after the route and lifecycle
	// services are ready. Only un-dispatched pending states are scheduled.
	s.startDynamicPolicyRecovery(ctx)

	// Start the idle-session reaper last. It depends on s.repo
	// (already wired), s.agentManager (already wired), and the
	// turnService-cleared reconciler so a turnService == nil error
	// would already have aborted Start above. After this returns the
	// background goroutine owns the reclaim tick; Service.Stop joins it
	// before tearing down repo / agentManager.
	s.startIdleSessionReaper(ctx)

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
	s.stopDynamicPolicyRecovery()

	// Stop components in reverse order
	var errs []error
	s.stopIdleSessionReaper()
	s.stopReservedPromptCallbacks()

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
	s.stopCIAutomationWorkers()

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
// Tests and explicit reconciliation callers use this combined entrypoint;
// Start calls both phases directly so prompt/clarification recovery errors are fatal.
func (s *Service) reconcileSessionsOnStartup(ctx context.Context) {
	if err := s.reconcileUnpublishedPromptTurnsOnStartup(ctx); err != nil {
		s.logger.Error("failed to reconcile unpublished prompt turns on startup", zap.Error(err))
		return
	}
	s.reconcileExecutorSessionsOnStartup(ctx)
}

func (s *Service) reconcileUnpublishedPromptTurnsOnStartup(ctx context.Context) error {
	if s.turnService != nil {
		reconciled, reconcileErr := s.turnService.ReconcileUnpublishedPromptTurns(ctx)
		if reconcileErr != nil {
			return fmt.Errorf("reconcile unpublished prompt turns on startup: %w", reconcileErr)
		} else if reconciled > 0 {
			s.logger.Info("reconciled unpublished prompt turns on startup", zap.Int("count", reconciled))
		}
	}
	return nil
}

func (s *Service) reconcileExecutorSessionsOnStartup(ctx context.Context) {
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

	s.reconcileActiveSessionOnStartup(ctx, running, sessionID, previousState, session)
}

func (s *Service) reconcileActiveSessionOnStartup(
	ctx context.Context,
	running *models.ExecutorRunning,
	sessionID string,
	previousState models.TaskSessionState,
	session *models.TaskSession,
) {
	// Active states: STARTING, RUNNING, WAITING_FOR_INPUT
	// Set session to WAITING_FOR_INPUT (idle, ready for lazy resume when user opens it)
	if previousState != models.TaskSessionStateWaitingForInput {
		s.updateTaskSessionStateWithHook(
			ctx,
			running.TaskID,
			sessionID,
			models.TaskSessionStateWaitingForInput,
			"",
			false,
			nil,
			session,
		)
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

const launchFailurePRLookupTimeout = time.Second

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

// QueueUserPrompt persists a prompt that must wait for workflow WIP admission.
// The user message row is already written by the WebSocket handler, so the
// queue marker prevents the drain path from creating a duplicate row.
func (s *Service) QueueUserPrompt(
	ctx context.Context,
	taskID, sessionID, prompt, model string,
	planMode bool,
	attachments []v1.MessageAttachment,
	metadata map[string]interface{},
	userMessageRecorded bool,
) error {
	if s.messageQueue == nil {
		return errors.New("message queue is not configured")
	}
	queueMetadata := make(map[string]interface{}, len(metadata)+1)
	for key, value := range metadata {
		queueMetadata[key] = value
	}
	if userMessageRecorded {
		queueMetadata[metaKeyUserMessageRecorded] = true
	}
	if _, err := s.messageQueue.QueueMessageWithMetadata(
		ctx,
		sessionID,
		taskID,
		prompt,
		model,
		messagequeue.QueuedByUser,
		planMode,
		toQueuedAttachments(attachments),
		queueMetadata,
	); err != nil {
		return fmt.Errorf("queue user prompt: %w", err)
	}
	s.publishQueueStatusEvent(ctx, sessionID)

	// T2: enqueue-side fast-path drain. The user's WIP wait is a
	// first-class contract (the dispatcher gates on
	// ProcessOnTurnStart's Queued=true return value). Promoting a queued
	// prompt to dispatch while the task is still in WIP-wait would
	// bypass the admission path; the existing 4 drain triggers
	// (agent.ready / agent.boot_ready / processOnEnter / promptTask
	// tail) lack this guard too — T2 is defense-in-depth, matching the
	// queueFull-bound pattern rather than introducing a new escape.
	// Three SQL reads per enqueue (clarification guard, session, and task)
	// is small and acceptable for the user-message volume; the load-path hot
	// loop is already gated by the guarded clarification check and
	// isCancelInFlight / isQueuedDispatchInFlight / isSteerInFlight
	// in-flight checks (cheaper than the DB read).
	s.tryFastPathDrainAfterEnqueue(ctx, taskID, sessionID)
	return nil
}

// tryFastPathDrainAfterEnqueue is the T2 fast-path drain. After QueueUserPrompt
// has admitted a user message into the session's queue, this method
// decides whether the message is safe to dispatch right now. The
// decision is gated by a sequence of cheap-to-expensive checks in
// increasing cost order:
//
//  1. messageQueue is non-nil (cheap pointer check).
//  2. isCancelInFlight / isQueuedDispatchInFlight / isSteerInFlight
//     are false (in-memory atomic loads). These guard the next drain
//     from racing an in-flight sibling. False negatives are safe: the
//     next turn-end re-evaluates.
//  3. drainQueuedMessageForPromptableSessionWithTaskAdmission reloads the
//     task after acquiring the guard and immediately before reservation. A
//     task waiting in the WIP admission queue (QueuedForStepID != "" &&
//     !WIPAdmitted) remains parked.
//
// The first failure aborts the fast-path
// drain silently. Failures leave the queued message in place for the
// next turn-end drain — they never block user input.
//
// The guarded helper rechecks clarification ownership, session promptability,
// and task admission immediately before reservation. The four pre-existing
// drain triggers (agent.ready, agent.boot_ready, processOnEnter, promptTask
// tail) also lack a WIP check — T2 is defense-in-depth that matches the
// existing admission pattern at the queue boundary, rather than introducing a
// different escape path inside the dispatch lifecycle.
func (s *Service) tryFastPathDrainAfterEnqueue(ctx context.Context, taskID, sessionID string) {
	if s.messageQueue == nil {
		return
	}
	if s.isCancelInFlight(sessionID) || s.isQueuedDispatchInFlight(sessionID) || s.isSteerInFlight(sessionID) {
		return
	}
	const maxTaskAdmissionReadAttempts = 2
	for attempt := 0; attempt < maxTaskAdmissionReadAttempts; attempt++ {
		outcome := s.drainQueuedMessageForPromptableSessionWithTaskAdmission(ctx, taskID, sessionID)
		if outcome != queueDrainTaskAdmissionReadFailed {
			return
		}
		if attempt+1 == maxTaskAdmissionReadAttempts {
			s.logger.Warn("queue fast-path deferred after repeated task admission reads failed",
				zap.String("task_id", taskID), zap.String("session_id", sessionID))
			return
		}
	}
}

// GetEventBus returns the event bus
func (s *Service) GetEventBus() bus.EventBus {
	return s.eventBus
}
