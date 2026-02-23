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
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
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
	UpdatePermissionMessage(ctx context.Context, sessionID, pendingID, status string) error
	// CreateAgentMessageStreaming creates a new agent message with a pre-generated ID for streaming updates
	CreateAgentMessageStreaming(ctx context.Context, messageID, taskID, content, agentSessionID, turnID string) error
	// AppendAgentMessage appends additional content to an existing streaming message
	AppendAgentMessage(ctx context.Context, messageID, additionalContent string) error
	// CreateThinkingMessageStreaming creates a new thinking message with a pre-generated ID for streaming updates
	CreateThinkingMessageStreaming(ctx context.Context, messageID, taskID, content, agentSessionID, turnID string) error
	// AppendThinkingMessage appends additional content to an existing streaming thinking message
	AppendThinkingMessage(ctx context.Context, messageID, additionalContent string) error
}

// TurnService is an interface for managing session turns
type TurnService interface {
	StartTurn(ctx context.Context, sessionID string) (*models.Turn, error)
	CompleteTurn(ctx context.Context, turnID string) error
	GetActiveTurn(ctx context.Context, sessionID string) (*models.Turn, error)
}

// WorkflowStepGetter retrieves workflow step information for prompt building.
type WorkflowStepGetter interface {
	GetStep(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error)
	GetNextStepByPosition(ctx context.Context, workflowID string, currentPosition int) (*wfmodels.WorkflowStep, error)
	GetPreviousStepByPosition(ctx context.Context, workflowID string, currentPosition int) (*wfmodels.WorkflowStep, error)
}

// repoStore is the repository interface accepted by NewService.
// It covers both the orchestrator's own needs (sessionExecutorStore) and
// the executor package's needs (executor.executorStore).
type repoStore interface {
	sessionExecutorStore
	// Additional methods needed by executor
	UpdateTaskState(ctx context.Context, id string, state v1.TaskState) error
	GetPrimaryTaskRepository(ctx context.Context, taskID string) (*models.TaskRepository, error)
	CreateTaskSession(ctx context.Context, session *models.TaskSession) error
	UpdateTaskSession(ctx context.Context, session *models.TaskSession) error
	SetSessionPrimary(ctx context.Context, sessionID string) error
	ListActiveTaskSessions(ctx context.Context) ([]*models.TaskSession, error)
	ListActiveTaskSessionsByTaskID(ctx context.Context, taskID string) ([]*models.TaskSession, error)
	CreateTaskSessionWorktree(ctx context.Context, sessionWorktree *models.TaskSessionWorktree) error
	GetRepository(ctx context.Context, id string) (*models.Repository, error)
	GetExecutorProfile(ctx context.Context, id string) (*models.ExecutorProfile, error)
	GetWorkspace(ctx context.Context, id string) (*models.Workspace, error)
}

// sessionExecutorStore is the minimal repository interface needed by the orchestrator service.
type sessionExecutorStore interface {
	// Session
	GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error)
	UpdateTaskSession(ctx context.Context, session *models.TaskSession) error
	UpdateTaskSessionState(ctx context.Context, id string, state models.TaskSessionState, errorMessage string) error
	ClearSessionExecutionID(ctx context.Context, id string) error
	UpdateSessionWorkflowStep(ctx context.Context, sessionID string, stepID string) error
	UpdateSessionReviewStatus(ctx context.Context, sessionID string, status string) error
	// Executor running state
	ListExecutorsRunning(ctx context.Context) ([]*models.ExecutorRunning, error)
	UpsertExecutorRunning(ctx context.Context, running *models.ExecutorRunning) error
	GetExecutorRunningBySessionID(ctx context.Context, sessionID string) (*models.ExecutorRunning, error)
	DeleteExecutorRunningBySessionID(ctx context.Context, sessionID string) error
	// Executor
	GetExecutor(ctx context.Context, id string) (*models.Executor, error)
	// Task
	GetTask(ctx context.Context, id string) (*models.Task, error)
	UpdateTask(ctx context.Context, task *models.Task) error
	// Git snapshots and commits
	GetLatestGitSnapshot(ctx context.Context, sessionID string) (*models.GitSnapshot, error)
	CreateGitSnapshot(ctx context.Context, snapshot *models.GitSnapshot) error
	CreateSessionCommit(ctx context.Context, commit *models.SessionCommit) error
	GetSessionCommits(ctx context.Context, sessionID string) ([]*models.SessionCommit, error)
	DeleteSessionCommit(ctx context.Context, id string) error
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

	// Workflow step getter for prompt building
	workflowStepGetter WorkflowStepGetter

	// Workflow engine for typed state-machine evaluation of step transitions
	workflowEngine *engine.Engine
	workflowStore  *workflowStore

	// GitHub service for PR auto-detection on push
	githubService GitHubService

	// Review task creator for auto-creating tasks from review watch PRs
	reviewTaskCreator ReviewTaskCreator

	// Repository resolver for cloning + finding/creating repos for review tasks
	repositoryResolver RepositoryResolver

	// Push tracker: sessionID -> last known ahead count
	pushTracker sync.Map

	// Active turns map: sessionID -> turnID
	activeTurns sync.Map

	// Session reset flags: sessionID -> true while resetAgentContext is restarting process.
	// Used to suppress stale ready events and avoid draining queued prompts mid-reset.
	resetInProgressSessions sync.Map

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

// NewService creates a new orchestrator service
func NewService(
	cfg ServiceConfig,
	eventBus bus.EventBus,
	agentManager executor.AgentManagerClient,
	taskRepo scheduler.TaskRepository,
	repo repoStore,
	shellPrefs executor.ShellPreferenceProvider,
	secretStore secrets.SecretStore,
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

	// Create the message queue service
	msgQueue := messagequeue.NewService(log)

	// Create the service (watcher will be created after we have handlers)
	s := &Service{
		config:       cfg,
		logger:       svcLogger,
		eventBus:     eventBus,
		taskRepo:     taskRepo,
		repo:         repo,
		agentManager: agentManager,
		queue:        taskQueue,
		executor:     exec,
		scheduler:    sched,
		messageQueue: msgQueue,
	}

	// Wire executor state changes through the orchestrator so events are published
	// (e.g. WebSocket notifications to the frontend). Must be set after service
	// construction so the session callback can reference s.updateTaskSessionState.
	exec.SetOnTaskStateChange(func(ctx context.Context, taskID string, state v1.TaskState) error {
		return taskRepo.UpdateTaskState(ctx, taskID, state)
	})
	exec.SetOnSessionStateChange(func(ctx context.Context, taskID, sessionID string, state models.TaskSessionState, errorMessage string) error {
		s.updateTaskSessionState(ctx, taskID, sessionID, state, errorMessage, true)
		return nil
	})

	// Create the watcher with event handlers that wire everything together
	handlers := watcher.EventHandlers{
		OnAgentRunning:         s.handleAgentRunning,
		OnAgentReady:           s.handleAgentReady,
		OnAgentCompleted:       s.handleAgentCompleted,
		OnAgentFailed:          s.handleAgentFailed,
		OnAgentStopped:         s.handleAgentStopped,
		OnAgentStreamEvent:     s.handleAgentStreamEvent,
		OnACPSessionCreated:    s.handleACPSessionCreated,
		OnPermissionRequest:    s.handlePermissionRequest,
		OnGitEvent:             s.handleGitEvent,
		OnContextWindowUpdated: s.handleContextWindowUpdated,
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

// initWorkflowEngine creates the workflow engine with store and callbacks.
// Called after the workflow step getter is set. Safe to call multiple times.
func (s *Service) initWorkflowEngine() {
	if s.workflowStepGetter == nil {
		return
	}
	store := newWorkflowStore(s.repo, s.workflowStepGetter, s.agentManager, s.eventBus, s.logger)
	callbacks := buildWorkflowCallbacks(s)
	s.workflowStore = store
	s.workflowEngine = engine.New(store, callbacks)
}

// startTurnForSession starts a new turn for the session and stores it.
func (s *Service) startTurnForSession(ctx context.Context, sessionID string) string {
	if s.turnService == nil {
		return ""
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

// completeTurnForSession completes the active turn for the session.
func (s *Service) completeTurnForSession(ctx context.Context, sessionID string) {
	if s.turnService == nil {
		return
	}

	turnIDVal, ok := s.activeTurns.LoadAndDelete(sessionID)
	if !ok {
		return
	}

	turnID, ok := turnIDVal.(string)
	if !ok || turnID == "" {
		return
	}

	if err := s.turnService.CompleteTurn(ctx, turnID); err != nil {
		s.logger.Warn("failed to complete turn",
			zap.String("session_id", sessionID),
			zap.String("turn_id", turnID),
			zap.Error(err))
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
//  1. Terminal states (Completed/Cancelled/Failed) → clean up executor record
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
		if isRemoteRuntime(running.Runtime) {
			remoteRecords = append(remoteRecords, executor.RemoteStatusPollRequest{
				SessionID:        running.SessionID,
				Runtime:          running.Runtime,
				AgentExecutionID: running.AgentExecutionID,
				ContainerID:      running.ContainerID,
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
		s.logger.Warn("failed to load session for reconciliation",
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
		if err := s.repo.DeleteExecutorRunningBySessionID(ctx, sessionID); err != nil {
			s.logger.Warn("failed to remove executor record for unstarted session",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
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

	// Clear stale execution references (agent process is gone after restart)
	_ = s.repo.ClearSessionExecutionID(ctx, sessionID)

	// Ensure task is in REVIEW state (not stuck IN_PROGRESS)
	if running.TaskID != "" {
		task, taskErr := s.taskRepo.GetTask(ctx, running.TaskID)
		if taskErr == nil && task != nil && task.State == v1.TaskStateInProgress {
			if updateErr := s.taskRepo.UpdateTaskState(ctx, running.TaskID, v1.TaskStateReview); updateErr != nil {
				s.logger.Warn("failed to update task to REVIEW on startup",
					zap.String("task_id", running.TaskID),
					zap.Error(updateErr))
			}
		}
	}

	// PRESERVE the ExecutorRunning record — it holds the resume token and worktree info
	// needed for lazy recovery when the user opens the session.

	s.logger.Info("session reconciled for lazy recovery",
		zap.String("session_id", sessionID),
		zap.String("task_id", running.TaskID),
		zap.String("previous_state", string(previousState)),
		zap.Bool("has_resume_token", running.ResumeToken != ""),
		zap.Bool("has_worktree", running.WorktreePath != ""))
}

// isRemoteRuntime checks if a runtime string corresponds to a remote executor type.
func isRemoteRuntime(runtime string) bool {
	return runtime == string(models.ExecutorTypeSprites) || runtime == string(models.ExecutorTypeRemoteDocker)
}

// handleTerminalSessionOnStartup processes sessions in terminal states during startup.
// Returns true if the session should be skipped (no further processing needed).
func (s *Service) handleTerminalSessionOnStartup(ctx context.Context, session *models.TaskSession, running *models.ExecutorRunning, previousState models.TaskSessionState) bool {
	sessionID := session.ID
	switch previousState {
	case models.TaskSessionStateCompleted, models.TaskSessionStateCancelled:
		s.logger.Info("session in terminal state; cleaning up executor record",
			zap.String("session_id", sessionID),
			zap.String("task_id", session.TaskID),
			zap.String("state", string(previousState)))
		if err := s.repo.DeleteExecutorRunningBySessionID(ctx, sessionID); err != nil {
			s.logger.Warn("failed to remove executor record for terminal session",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
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
	// If session failed, ensure task is in REVIEW state (not stuck IN_PROGRESS)
	if session.TaskID != "" {
		task, taskErr := s.taskRepo.GetTask(ctx, session.TaskID)
		if taskErr == nil && task.State == v1.TaskStateInProgress {
			s.logger.Info("fixing task state: session failed but task still IN_PROGRESS",
				zap.String("task_id", session.TaskID),
				zap.String("session_id", sessionID))
			if updateErr := s.taskRepo.UpdateTaskState(ctx, session.TaskID, v1.TaskStateReview); updateErr != nil {
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
		s.logger.Info("cleaning up executor record for non-resumable failed session",
			zap.String("session_id", sessionID),
			zap.String("task_id", session.TaskID))
		if err := s.repo.DeleteExecutorRunningBySessionID(ctx, sessionID); err != nil {
			s.logger.Warn("failed to remove executor record for failed session",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
	}
}

func canResumeRunning(running *models.ExecutorRunning) bool {
	if running == nil || running.ResumeToken == "" {
		return false
	}
	if running.Resumable {
		return true
	}
	return running.Runtime == string(models.ExecutorTypeSprites)
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
