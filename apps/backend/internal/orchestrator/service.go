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
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Common errors
var (
	ErrServiceAlreadyRunning = errors.New("service is already running")
	ErrServiceNotRunning     = errors.New("service is not running")
)

// ServiceConfig holds orchestrator service configuration
type ServiceConfig struct {
	Scheduler       scheduler.SchedulerConfig
	QueueSize       int
	WorktreeEnabled bool // Whether to use Git worktrees for agent isolation
}

// DefaultServiceConfig returns default configuration
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		Scheduler:       scheduler.DefaultSchedulerConfig(),
		QueueSize:       1000,
		WorktreeEnabled: true,
	}
}

// MessageCreator is an interface for creating messages on tasks
type MessageCreator interface {
	CreateAgentMessage(ctx context.Context, taskID, content, agentSessionID, turnID string) error
	CreateUserMessage(ctx context.Context, taskID, content, agentSessionID, turnID string) error
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

// WorkflowStep contains workflow step data needed for prompt construction and transitions.
type WorkflowStep struct {
	ID               string
	Name             string
	StepType         string
	AutoStartAgent   bool
	PlanMode         bool
	RequireApproval  bool
	PromptPrefix     string
	PromptSuffix     string
	OnCompleteStepID string // Step to transition to when agent completes
	OnApprovalStepID string // Step to transition to when user approves
}

// WorkflowStepGetter retrieves workflow step information for prompt building.
type WorkflowStepGetter interface {
	GetStep(ctx context.Context, stepID string) (*WorkflowStep, error)
	// GetSourceStep finds the step that has on_complete_step_id pointing to the given step.
	// Used to find the "previous" step when moving back from a review step.
	GetSourceStep(ctx context.Context, boardID, targetStepID string) (*WorkflowStep, error)
}

// WorktreeRecreator handles worktree recreation when the directory is missing.
// This is used during session resume when the worktree was deleted (e.g., by the user).
type WorktreeRecreator interface {
	RecreateWorktree(ctx context.Context, req WorktreeRecreateRequest) (*WorktreeRecreateResult, error)
}

// WorktreeRecreateRequest contains parameters for recreating a worktree.
type WorktreeRecreateRequest struct {
	SessionID    string
	TaskID       string
	TaskTitle    string
	RepositoryID string
	BaseBranch   string
	WorktreeID   string
}

// WorktreeRecreateResult contains the result of worktree recreation.
type WorktreeRecreateResult struct {
	WorktreePath   string
	WorktreeBranch string
}

// Service is the main orchestrator service
type Service struct {
	config       ServiceConfig
	logger       *logger.Logger
	eventBus     bus.EventBus
	taskRepo     scheduler.TaskRepository
	repo         repository.Repository // Full repository for agent sessions
	agentManager executor.AgentManagerClient

	// Components
	queue     *queue.TaskQueue
	executor  *executor.Executor
	scheduler *scheduler.Scheduler
	watcher   *watcher.Watcher

	// Message creator for saving agent responses
	messageCreator MessageCreator

	// Turn service for managing session turns
	turnService TurnService

	// Workflow step getter for prompt building
	workflowStepGetter WorkflowStepGetter

	// Worktree recreator for recreating missing worktrees on resume
	worktreeRecreator WorktreeRecreator

	// Active turns map: sessionID -> turnID
	activeTurns sync.Map

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
	repo repository.Repository,
	shellPrefs executor.ShellPreferenceProvider,
	log *logger.Logger,
) *Service {
	svcLogger := log.WithFields(zap.String("component", "orchestrator"))

	// Create the task queue with configured size
	taskQueue := queue.NewTaskQueue(cfg.QueueSize)

	// Create the executor with the agent manager client and repository for persistent sessions
	execCfg := executor.ExecutorConfig{
		WorktreeEnabled: cfg.WorktreeEnabled,
		ShellPrefs:      shellPrefs,
	}
	exec := executor.NewExecutor(agentManager, repo, log, execCfg)

	// Create the scheduler with queue, executor, and task repository
	sched := scheduler.NewScheduler(taskQueue, exec, taskRepo, log, cfg.Scheduler)

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
	}

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
	s.watcher = watcher.NewWatcher(eventBus, handlers, log)

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
}

// SetWorktreeRecreator sets the worktree recreator for handling missing worktrees during resume.
//
// When resuming sessions on startup, if a worktree directory is missing, the orchestrator
// will attempt to recreate it using this recreator. If recreation succeeds, the session
// can be resumed normally. If not set, missing worktrees will cause session resume to fail.
func (s *Service) SetWorktreeRecreator(wr WorktreeRecreator) {
	s.worktreeRecreator = wr
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

	// Resume executors from persisted runtime state on startup.
	s.resumeExecutorsOnStartup(ctx)

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

// resumeExecutorsOnStartup recovers agent sessions that were active before server restart.
//
// This critical startup routine handles crash recovery and session persistence. When the server
// starts, it queries the database for sessions that were "running" when the server stopped,
// and attempts to restore them by relaunching agentctl and reconnecting to the workspace.
//
// Recovery Strategy:
//
//  1. Query database for sessions marked as "running" (persisted executor state)
//  2. For each session, determine recovery action based on previous state:
//
//     Terminal States (Completed/Failed/Cancelled):
//       - Clean up stale executor record from database
//       - Skip resume (session is already done)
//
//     Never-Started State (Created):
//       - Clean up executor record (session never actually ran)
//       - Skip resume (nothing to recover)
//
//     Active States (Starting/Running/WaitingForInput):
//       - Validate workspace/worktree still exists
//       - Launch new agentctl instance
//       - Restore session via ACP session/load (if resumable)
//       - Transition to WaitingForInput (ready for next prompt)
//
//  3. Handle resume failures gracefully:
//       - Mark session as Failed in database
//       - Clean up executor record
//       - Continue with next session (don't block other recoveries)
//
// Agent Resume Behavior:
//
//   - startAgent = (ResumeToken != "" && Resumable) determines if agent process starts
//   - If startAgent=true: Launches agent, loads conversation history via ACP
//   - If startAgent=false: Only workspace access (shell) is restored, no agent
//
// State Transitions:
//
//   - Session resume does NOT process any prompt (just loads history)
//   - No "complete" event is emitted after resume (only after prompt completion)
//   - Final state is always WaitingForInput (ready for user's next prompt)
//
// This runs synchronously during Start() before accepting new requests, ensuring all
// recoverable sessions are restored before the orchestrator begins processing new tasks.
//
// Called by: Start() method during orchestrator initialization.
func (s *Service) resumeExecutorsOnStartup(ctx context.Context) {
	runningExecutors, err := s.repo.ListExecutorsRunning(ctx)
	if err != nil {
		s.logger.Warn("failed to list executors running on startup", zap.Error(err))
		return
	}
	if len(runningExecutors) == 0 {
		s.logger.Info("no executors to resume on startup")
		return
	}

	s.logger.Info("resuming executors on startup", zap.Int("count", len(runningExecutors)))

	for _, running := range runningExecutors {
		sessionID := running.SessionID
		if sessionID == "" {
			continue
		}

		session, err := s.repo.GetTaskSession(ctx, sessionID)
		if err != nil {
			s.logger.Warn("failed to load session for resume",
				zap.String("session_id", sessionID),
				zap.Error(err))
			continue
		}

		previousState := session.State

		// Handle sessions in terminal states - clean up executor record and fix task state
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
			continue
		case models.TaskSessionStateFailed:
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
			// Preserve ExecutorRunning for resumable failed sessions so the user
			// can resume them later. Only clean up non-resumable failures.
			if running.ResumeToken != "" && running.Resumable {
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
			continue
		}

		// Handle sessions that never started - skip without failure
		if previousState == models.TaskSessionStateCreated {
			s.logger.Info("session was never started; skipping recovery",
				zap.String("session_id", sessionID))
			if err := s.repo.DeleteExecutorRunningBySessionID(ctx, sessionID); err != nil {
				s.logger.Warn("failed to remove executor record for unstarted session",
					zap.String("session_id", sessionID),
					zap.Error(err))
			}
			continue
		}

		startAgent := running.ResumeToken != "" && running.Resumable
		if !startAgent {
			s.logger.Debug("resuming workspace without agent session",
				zap.String("session_id", sessionID),
				zap.String("previous_state", string(previousState)),
				zap.Bool("has_resume_token", running.ResumeToken != ""),
				zap.Bool("resumable", running.Resumable))
		}

		// Validate worktree exists before resuming
		if err := validateSessionWorktrees(session); err != nil {
			s.logger.Info("worktree not found, attempting recreation",
				zap.String("session_id", sessionID),
				zap.Error(err))

			// Try to recreate the worktree
			if s.tryRecreateWorktree(ctx, session) {
				s.logger.Info("worktree recreated successfully, continuing with resume",
					zap.String("session_id", sessionID))
				// Re-validate after recreation to ensure it worked
				if err := validateSessionWorktrees(session); err != nil {
					s.logger.Warn("worktree validation still failed after recreation",
						zap.String("session_id", sessionID),
						zap.Error(err))
					_ = s.repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateFailed, "workspace recreation failed: "+err.Error())
					_ = s.repo.DeleteExecutorRunningBySessionID(ctx, sessionID)
					continue
				}
			} else {
				// Recreation failed or not available
				s.logger.Warn("worktree recreation failed; marking session as failed",
					zap.String("session_id", sessionID))
				_ = s.repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateFailed, "workspace not found and could not be restored")
				_ = s.repo.DeleteExecutorRunningBySessionID(ctx, sessionID)
				continue
			}
		}

		s.logger.Debug("resuming session on startup",
			zap.String("session_id", sessionID),
			zap.String("task_id", running.TaskID),
			zap.String("previous_state", string(previousState)),
			zap.String("resume_token", running.ResumeToken),
			zap.Bool("start_agent", startAgent))

		// Resume the session - this will:
		// 1. Create a new agentctl instance
		// 2. Start the agent process
		// 3. Initialize and load the session via ACP/Codex protocol
		//
		// ResumeSession will temporarily set state to STARTING during launch.
		// After resume, the agent is immediately ready for input (session is loaded,
		// no prompt is being processed). We set WAITING_FOR_INPUT directly because:
		// - Session resume just loads conversation history, doesn't process a prompt
		// - Agents don't send a "complete" event after session load (only after prompt completion)
		// - The agent is ready for the next user prompt immediately after session loads
		if _, err := s.executor.ResumeSession(ctx, session, startAgent); err != nil {
			s.logger.Warn("failed to resume executor on startup",
				zap.String("session_id", sessionID),
				zap.Error(err))
			_ = s.repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateFailed, err.Error())
			// Clear the stale execution ID to prevent "execution not found" errors
			// when the user tries to prompt after the failed resume
			_ = s.repo.ClearSessionExecutionID(ctx, sessionID)
			_ = s.repo.DeleteExecutorRunningBySessionID(ctx, sessionID)
			continue
		}

		if startAgent {
			// Set state to WAITING_FOR_INPUT - agent is ready for the next prompt
			if err := s.repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateWaitingForInput, ""); err != nil {
				s.logger.Warn("failed to set session state to WAITING_FOR_INPUT after resume",
					zap.String("session_id", sessionID),
					zap.Error(err))
			}
			// Also update task state to REVIEW since we're waiting for user input
			if err := s.taskRepo.UpdateTaskState(ctx, running.TaskID, v1.TaskStateReview); err != nil {
				s.logger.Warn("failed to set task state to REVIEW after resume",
					zap.String("task_id", running.TaskID),
					zap.Error(err))
			}
		}

		s.logger.Debug("session resumed successfully",
			zap.String("session_id", sessionID),
			zap.String("task_id", running.TaskID),
			zap.Bool("start_agent", startAgent),
			zap.String("state", string(models.TaskSessionStateWaitingForInput)))
	}
}

// tryRecreateWorktree attempts to recreate a missing worktree for a session.
// On success, it updates the session's worktree info and creates an info message.
// On failure, it marks the session as failed and creates an error message.
// Returns true if recreation succeeded and session can be resumed.
func (s *Service) tryRecreateWorktree(ctx context.Context, session *models.TaskSession) bool {
	if s.worktreeRecreator == nil {
		s.logger.Debug("worktree recreator not available, cannot recreate worktree",
			zap.String("session_id", session.ID))
		return false
	}

	if len(session.Worktrees) == 0 {
		s.logger.Debug("session has no worktrees to recreate",
			zap.String("session_id", session.ID))
		return false
	}

	wt := session.Worktrees[0]

	// Get task title for better branch naming
	var taskTitle string
	if task, err := s.taskRepo.GetTask(ctx, session.TaskID); err == nil {
		taskTitle = task.Title
	}

	s.logger.Info("attempting to recreate worktree",
		zap.String("session_id", session.ID),
		zap.String("task_id", session.TaskID),
		zap.String("worktree_id", wt.WorktreeID),
		zap.String("repository_id", wt.RepositoryID))

	req := WorktreeRecreateRequest{
		SessionID:    session.ID,
		TaskID:       session.TaskID,
		TaskTitle:    taskTitle,
		RepositoryID: wt.RepositoryID,
		BaseBranch:   session.BaseBranch,
		WorktreeID:   wt.WorktreeID,
	}

	result, err := s.worktreeRecreator.RecreateWorktree(ctx, req)
	if err != nil {
		s.logger.Warn("failed to recreate worktree",
			zap.String("session_id", session.ID),
			zap.String("worktree_id", wt.WorktreeID),
			zap.Error(err))

		// Create error message in chat
		if s.messageCreator != nil {
			metadata := map[string]interface{}{
				"variant": "error",
			}
			_ = s.messageCreator.CreateSessionMessage(
				ctx,
				session.TaskID,
				"Workspace could not be restored. The workspace directory was deleted and recreation failed: "+err.Error(),
				session.ID,
				string(v1.MessageTypeStatus),
				"", // no turn ID during startup
				metadata,
				false,
			)
		}
		return false
	}

	// Update session's worktree info
	session.Worktrees[0].WorktreePath = result.WorktreePath
	session.Worktrees[0].WorktreeBranch = result.WorktreeBranch

	s.logger.Info("worktree recreated successfully",
		zap.String("session_id", session.ID),
		zap.String("worktree_id", wt.WorktreeID),
		zap.String("new_path", result.WorktreePath))

	// Create info message in chat
	if s.messageCreator != nil {
		metadata := map[string]interface{}{
			"variant": "info",
		}
		_ = s.messageCreator.CreateSessionMessage(
			ctx,
			session.TaskID,
			"Workspace was automatically restored after server restart",
			session.ID,
			string(v1.MessageTypeStatus),
			"", // no turn ID during startup
			metadata,
			false,
		)
	}

	return true
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
