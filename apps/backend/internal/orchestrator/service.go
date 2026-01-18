// Package orchestrator provides the main orchestrator service that ties all components together.
package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/dto"
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
	CreateAgentMessage(ctx context.Context, taskID, content, agentSessionID string) error
	CreateUserMessage(ctx context.Context, taskID, content, agentSessionID string) error
	CreateToolCallMessage(ctx context.Context, taskID, toolCallID, title, status, agentSessionID string, args map[string]interface{}) error
	UpdateToolCallMessage(ctx context.Context, taskID, toolCallID, status, result, agentSessionID string) error
	CreateSessionMessage(ctx context.Context, taskID, content, agentSessionID, messageType string, metadata map[string]interface{}, requestsInput bool) error
	CreatePermissionRequestMessage(ctx context.Context, taskID, sessionID, pendingID, toolCallID, title string, options []map[string]interface{}, actionType string, actionDetails map[string]interface{}) (string, error)
	UpdatePermissionMessage(ctx context.Context, sessionID, pendingID, status string) error
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

	// Agent stream event handlers (for WebSocket streaming)
	streamHandlers []func(payload *lifecycle.AgentStreamEventPayload)
	streamMu       sync.RWMutex

	// Message creator for saving agent responses
	messageCreator MessageCreator

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
		MaxConcurrent:   cfg.Scheduler.MaxConcurrent,
		WorktreeEnabled: cfg.WorktreeEnabled,
		ShellPrefs:      shellPrefs,
	}
	exec := executor.NewExecutor(agentManager, repo, log, execCfg)

	// Create the scheduler with queue, executor, and task repository
	sched := scheduler.NewScheduler(taskQueue, exec, taskRepo, log, cfg.Scheduler)

	// Create the service (watcher will be created after we have handlers)
	s := &Service{
		config:         cfg,
		logger:         svcLogger,
		eventBus:       eventBus,
		taskRepo:       taskRepo,
		repo:           repo,
		agentManager:   agentManager,
		queue:          taskQueue,
		executor:       exec,
		scheduler:      sched,
		streamHandlers: make([]func(payload *lifecycle.AgentStreamEventPayload), 0),
	}

	// Create the watcher with event handlers that wire everything together
	handlers := watcher.EventHandlers{
		OnTaskStateChanged:  s.handleTaskStateChanged,
		OnAgentReady:        s.handleAgentReady,
		OnAgentCompleted:    s.handleAgentCompleted,
		OnAgentFailed:       s.handleAgentFailed,
		OnAgentStreamEvent:  s.handleAgentStreamEvent,
		OnACPSessionCreated: s.handleACPSessionCreated,
		OnPermissionRequest: s.handlePermissionRequest,
		OnGitStatusUpdated:  s.handleGitStatusUpdated,
	}
	s.watcher = watcher.NewWatcher(eventBus, handlers, log)

	return s
}

// SetMessageCreator sets the message creator for saving agent responses
func (s *Service) SetMessageCreator(mc MessageCreator) {
	s.messageCreator = mc
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

		// Handle sessions in terminal states - clean up executor record
		switch previousState {
		case models.TaskSessionStateCompleted, models.TaskSessionStateFailed, models.TaskSessionStateCancelled:
			s.logger.Info("session in terminal state; cleaning up executor record",
				zap.String("session_id", sessionID),
				zap.String("state", string(previousState)))
			if err := s.repo.DeleteExecutorRunningBySessionID(ctx, sessionID); err != nil {
				s.logger.Warn("failed to remove executor record for terminal session",
					zap.String("session_id", sessionID),
					zap.Error(err))
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

		// For sessions that were STARTING, RUNNING, or WAITING_FOR_INPUT:
		// We need a resume token to recover them
		if running.ResumeToken == "" {
			s.logger.Warn("no resume token available; marking session as failed",
				zap.String("session_id", sessionID),
				zap.String("previous_state", string(previousState)))
			_ = s.repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateFailed,
				"backend restarted and session cannot be resumed (no resume token)")
			_ = s.repo.DeleteExecutorRunningBySessionID(ctx, sessionID)
			continue
		}

		// Check if executor supports resume
		if !running.Resumable {
			s.logger.Warn("executor not resumable; marking session as failed",
				zap.String("session_id", sessionID))
			_ = s.repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateFailed,
				"executor does not support session resume")
			_ = s.repo.DeleteExecutorRunningBySessionID(ctx, sessionID)
			continue
		}

		// Validate worktree exists before resuming
		if err := validateSessionWorktrees(session); err != nil {
			s.logger.Warn("worktree validation failed; marking session as failed",
				zap.String("session_id", sessionID),
				zap.Error(err))
			_ = s.repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateFailed, err.Error())
			_ = s.repo.DeleteExecutorRunningBySessionID(ctx, sessionID)
			continue
		}

		s.logger.Info("resuming session on startup",
			zap.String("session_id", sessionID),
			zap.String("task_id", running.TaskID),
			zap.String("previous_state", string(previousState)),
			zap.String("resume_token", running.ResumeToken))

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
		if _, err := s.executor.ResumeSession(ctx, session, true); err != nil {
			s.logger.Warn("failed to resume executor on startup",
				zap.String("session_id", sessionID),
				zap.Error(err))
			_ = s.repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateFailed, err.Error())
			_ = s.repo.DeleteExecutorRunningBySessionID(ctx, sessionID)
			continue
		}

		// Set state to WAITING_FOR_INPUT - agent is ready for the next prompt
		if err := s.repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateWaitingForInput, ""); err != nil {
			s.logger.Warn("failed to set session state to WAITING_FOR_INPUT after resume",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}

		s.logger.Info("session resumed successfully",
			zap.String("session_id", sessionID),
			zap.String("task_id", running.TaskID),
			zap.String("state", string(models.TaskSessionStateWaitingForInput)))
	}
}

func validateSessionWorktrees(session *models.TaskSession) error {
	for _, wt := range session.Worktrees {
		if wt.WorktreePath == "" {
			continue
		}
		if _, err := os.Stat(wt.WorktreePath); err != nil {
			return fmt.Errorf("worktree path not found: %w", err)
		}
	}
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

// EnqueueTask manually adds a task to the queue
func (s *Service) EnqueueTask(ctx context.Context, task *v1.Task) error {
	s.logger.Info("manually enqueueing task",
		zap.String("task_id", task.ID),
		zap.String("title", task.Title))
	return s.scheduler.EnqueueTask(task)
}

// StartTask manually starts agent execution for a task
func (s *Service) StartTask(ctx context.Context, taskID string, agentProfileID string, priority int, prompt string) (*executor.TaskExecution, error) {
	s.logger.Info("manually starting task",
		zap.String("task_id", taskID),
		zap.String("agent_profile_id", agentProfileID),
		zap.Int("priority", priority),
		zap.Int("prompt_length", len(prompt)))

	if err := s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateScheduling); err != nil {
		s.logger.Warn("failed to update task state to SCHEDULING",
			zap.String("task_id", taskID),
			zap.Error(err))
	}

	// Fetch the task from the repository to get complete task info
	task, err := s.scheduler.GetTask(ctx, taskID)
	if err != nil {
		s.logger.Error("failed to fetch task for manual start",
			zap.String("task_id", taskID),
			zap.Error(err))
		return nil, err
	}

	// Override priority if provided in the request
	if priority > 0 {
		task.Priority = priority
	}

	// Use provided prompt, fall back to task description
	effectivePrompt := prompt
	if effectivePrompt == "" {
		effectivePrompt = task.Description
	}

	execution, err := s.executor.ExecuteWithProfile(ctx, task, agentProfileID, effectivePrompt)
	if err != nil {
		return nil, err
	}

	if execution.SessionID != "" {
		s.updateTaskSessionState(ctx, taskID, execution.SessionID, models.TaskSessionStateRunning, "", true)
		// Create user message for the initial prompt
		if s.messageCreator != nil && effectivePrompt != "" {
			if err := s.messageCreator.CreateUserMessage(ctx, taskID, effectivePrompt, execution.SessionID); err != nil {
				s.logger.Error("failed to create initial user message",
					zap.String("task_id", taskID),
					zap.Error(err))
			}
		}
	}

	if err := s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateInProgress); err != nil {
		s.logger.Warn("failed to update task state to IN_PROGRESS",
			zap.String("task_id", taskID),
			zap.Error(err))
	}

	return execution, nil
}

// ResumeTaskSession restarts a specific task session using its stored worktree.
func (s *Service) ResumeTaskSession(ctx context.Context, taskID, sessionID string) (*executor.TaskExecution, error) {
	s.logger.Info("resuming task session",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID))

	if exec, ok := s.executor.GetExecutionWithContext(ctx, taskID); ok && exec != nil {
		if exec.SessionID != "" && exec.SessionID != sessionID {
			return nil, executor.ErrExecutionAlreadyRunning
		}
	}

	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.TaskID != taskID {
		return nil, fmt.Errorf("task session does not belong to task")
	}
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil || running == nil || running.ResumeToken == "" || !running.Resumable {
		return nil, fmt.Errorf("session is not resumable")
	}
	if err := validateSessionWorktrees(session); err != nil {
		return nil, err
	}

	execution, err := s.executor.ResumeSession(ctx, session, true)
	if err != nil {
		_ = s.repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateFailed, err.Error())
		return nil, err
	}
	// Preserve persisted task/session state; resume should not mutate state/columns.
	execution.SessionState = v1.TaskSessionState(session.State)

	s.logger.Info("task session resumed and ready for input",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID))

	return execution, nil
}

// GetTaskSessionStatus returns the status of a task session including whether it's resumable
func (s *Service) GetTaskSessionStatus(ctx context.Context, taskID, sessionID string) (dto.TaskSessionStatusResponse, error) {
	s.logger.Info("checking task session status",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID))

	resp := dto.TaskSessionStatusResponse{
		SessionID: sessionID,
		TaskID:    taskID,
	}

	// 1. Load session from database
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		resp.Error = "session not found"
		return resp, nil
	}

	if session.TaskID != taskID {
		resp.Error = "session does not belong to task"
		return resp, nil
	}

	resp.State = string(session.State)
	resp.AgentProfileID = session.AgentProfileID

	// Extract resume token from executor runtime state.
	var resumeToken string
	if running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID); err == nil && running != nil {
		resumeToken = running.ResumeToken
		resp.ACPSessionID = resumeToken
		if running.Resumable {
			resp.IsResumable = true
		}
	}

	// Extract worktree info
	if len(session.Worktrees) > 0 {
		wt := session.Worktrees[0]
		if wt.WorktreePath != "" {
			resp.WorktreePath = &wt.WorktreePath
		}
		if wt.WorktreeBranch != "" {
			resp.WorktreeBranch = &wt.WorktreeBranch
		}
	}

	// 2. Check if we have an active execution in memory and agent is running
	if exec, ok := s.executor.GetExecutionWithContext(ctx, taskID); ok && exec != nil {
		resp.IsAgentRunning = true
		resp.NeedsResume = false
		return resp, nil
	}

	// 3. Session can be resumed if it has a resume token
	if resumeToken == "" {
		resp.IsAgentRunning = false
		resp.IsResumable = false
		resp.NeedsResume = false
		return resp, nil
	}

	// 4. Additional validations for resumption
	if session.AgentProfileID == "" {
		resp.Error = "session missing agent profile"
		resp.IsResumable = false
		return resp, nil
	}

	// Check if worktree exists (if one was used)
	if len(session.Worktrees) > 0 && session.Worktrees[0].WorktreePath != "" {
		if _, err := os.Stat(session.Worktrees[0].WorktreePath); err != nil {
			resp.Error = "worktree not found"
			resp.IsResumable = false
			return resp, nil
		}
	}

	resp.IsAgentRunning = false
	resp.NeedsResume = true
	resp.ResumeReason = "agent_not_running"

	return resp, nil
}

// StopTask stops agent execution for a task (stops all active sessions for the task)
func (s *Service) StopTask(ctx context.Context, taskID string, reason string, force bool) error {
	s.logger.Info("stopping task execution",
		zap.String("task_id", taskID),
		zap.String("reason", reason),
		zap.Bool("force", force))
	return s.executor.StopByTaskID(ctx, taskID, reason, force)
}

// StopSession stops agent execution for a specific session
func (s *Service) StopSession(ctx context.Context, sessionID string, reason string, force bool) error {
	s.logger.Info("stopping session execution",
		zap.String("session_id", sessionID),
		zap.String("reason", reason),
		zap.Bool("force", force))
	return s.executor.Stop(ctx, sessionID, reason, force)
}

// PromptResult contains the result of a prompt operation
type PromptResult struct {
	StopReason   string // The reason the agent stopped (e.g., "end_turn")
	AgentMessage string // The agent's accumulated response message
}

// PromptTask sends a follow-up prompt to a running agent for a task session.
func (s *Service) PromptTask(ctx context.Context, taskID, sessionID string, prompt string) (*PromptResult, error) {
	s.logger.Info("sending prompt to task agent",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.Int("prompt_length", len(prompt)))
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateRunning, "", true)
	result, err := s.executor.Prompt(ctx, taskID, sessionID, prompt)
	if err != nil {
		return nil, err
	}
	return &PromptResult{
		StopReason:   result.StopReason,
		AgentMessage: result.AgentMessage,
	}, nil
}

// RespondToPermission sends a response to a permission request for a session
func (s *Service) RespondToPermission(ctx context.Context, sessionID, pendingID, optionID string, cancelled bool) error {
	s.logger.Info("responding to permission request",
		zap.String("session_id", sessionID),
		zap.String("pending_id", pendingID),
		zap.String("option_id", optionID),
		zap.Bool("cancelled", cancelled))

	// Respond to the permission via agentctl
	if err := s.executor.RespondToPermission(ctx, sessionID, pendingID, optionID, cancelled); err != nil {
		return err
	}

	// Determine status based on response
	status := "approved"
	if cancelled {
		status = "rejected"
	}

	// Update the permission message with the new status
	if s.messageCreator != nil {
		if err := s.messageCreator.UpdatePermissionMessage(ctx, sessionID, pendingID, status); err != nil {
			s.logger.Warn("failed to update permission message status",
				zap.String("session_id", sessionID),
				zap.String("pending_id", pendingID),
				zap.String("status", status),
				zap.Error(err))
			// Don't fail the whole operation if message update fails
		}
	}

	return nil
}

// CompleteTask explicitly completes a task and stops all its agents
func (s *Service) CompleteTask(ctx context.Context, taskID string) error {
	s.logger.Info("completing task",
		zap.String("task_id", taskID))

	// Stop all agents for this task (which will trigger AgentCompleted events and update session states)
	if err := s.executor.StopByTaskID(ctx, taskID, "task completed by user", false); err != nil {
		// If agents are already stopped, just update the task state directly
		s.logger.Warn("failed to stop agents, updating task state directly",
			zap.String("task_id", taskID),
			zap.Error(err))
	}

	// Update task state to COMPLETED
	if err := s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateCompleted); err != nil {
		return fmt.Errorf("failed to update task state: %w", err)
	}

	s.logger.Info("task marked as COMPLETED",
		zap.String("task_id", taskID))
	return nil
}

// GetTaskExecution returns the current execution state for a task
func (s *Service) GetTaskExecution(taskID string) (*executor.TaskExecution, bool) {
	return s.executor.GetExecution(taskID)
}

// GetQueuedTasks returns tasks in the queue
func (s *Service) GetQueuedTasks() []*queue.QueuedTask {
	return s.queue.List()
}

// RegisterStreamHandler registers a handler for agent stream events (used by WebSocket streaming)
func (s *Service) RegisterStreamHandler(handler func(payload *lifecycle.AgentStreamEventPayload)) {
	s.streamMu.Lock()
	defer s.streamMu.Unlock()
	s.streamHandlers = append(s.streamHandlers, handler)
	s.logger.Debug("registered stream handler", zap.Int("total_handlers", len(s.streamHandlers)))
}

// UnregisterStreamHandler removes a stream handler
func (s *Service) UnregisterStreamHandler(handler func(payload *lifecycle.AgentStreamEventPayload)) {
	s.streamMu.Lock()
	defer s.streamMu.Unlock()

	// Find and remove the handler by comparing function pointers
	// Note: This uses a simple approach - in production you might want to use handler IDs
	for i := range s.streamHandlers {
		// We can't directly compare functions, so we'll keep the handler in the list
		// In a real implementation, you'd use unique handler IDs
		_ = i
	}
	s.logger.Debug("unregister stream handler called")
}

// broadcastStreamEvent broadcasts an agent stream event to all registered handlers
func (s *Service) broadcastStreamEvent(payload *lifecycle.AgentStreamEventPayload) {
	s.streamMu.RLock()
	handlers := make([]func(payload *lifecycle.AgentStreamEventPayload), len(s.streamHandlers))
	copy(handlers, s.streamHandlers)
	s.streamMu.RUnlock()

	for _, handler := range handlers {
		handler(payload)
	}
}

// Event handlers

// handleTaskStateChanged handles task state change events
func (s *Service) handleTaskStateChanged(ctx context.Context, data watcher.TaskEventData) {
	s.logger.Debug("handling task state changed",
		zap.String("task_id", data.TaskID))

	// No auto-enqueue on TODO; sessions are started explicitly with agent profile.
}

// handleAgentReady handles agent ready events (prompt completed, ready for follow-up)
// Now that both ACP and Codex Prompt() calls are synchronous, this event fires after
// the "complete" stream event. The state transition is already handled by handleAgentStreamEvent
// on the "complete" event, so this handler just logs the event for debugging.
func (s *Service) handleAgentReady(ctx context.Context, data watcher.AgentEventData) {
	s.logger.Debug("agent ready event received (state transition handled by complete event)",
		zap.String("task_id", data.TaskID),
		zap.String("agent_execution_id", data.AgentExecutionID))
}

func (s *Service) handleACPSessionCreated(ctx context.Context, data watcher.ACPSessionEventData) {
	if data.TaskID == "" || data.ACPSessionID == "" {
		return
	}

	sessionID := ""
	if exec, ok := s.executor.GetExecution(data.TaskID); ok && exec != nil {
		sessionID = exec.SessionID
	}
	if sessionID == "" {
		if session, err := s.repo.GetTaskSessionByTaskID(ctx, data.TaskID); err == nil && session != nil {
			sessionID = session.ID
		}
	}
	if sessionID == "" {
		s.logger.Warn("no task session found to store ACP session id",
			zap.String("task_id", data.TaskID))
		return
	}

	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to load task session for ACP session update",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}

	resumable := true
	if session.ExecutorID != "" {
		if executor, err := s.repo.GetExecutor(ctx, session.ExecutorID); err == nil && executor != nil {
			resumable = executor.Resumable
		}
	}

	running := &models.ExecutorRunning{
		ID:               session.ID,
		SessionID:        session.ID,
		TaskID:           session.TaskID,
		ExecutorID:       session.ExecutorID,
		Status:           "ready",
		Resumable:        resumable,
		ResumeToken:      data.ACPSessionID,
		AgentExecutionID: session.AgentExecutionID,
		ContainerID:      session.ContainerID,
	}
	if len(session.Worktrees) > 0 {
		running.WorktreeID = session.Worktrees[0].WorktreeID
		running.WorktreePath = session.Worktrees[0].WorktreePath
		running.WorktreeBranch = session.Worktrees[0].WorktreeBranch
	}

	if err := s.repo.UpsertExecutorRunning(ctx, running); err != nil {
		s.logger.Warn("failed to persist resume token for session",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}

	s.logger.Info("stored resume token for task session",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", sessionID),
		zap.String("resume_token", data.ACPSessionID))
}

// handleAgentCompleted handles agent completion events
func (s *Service) handleAgentCompleted(ctx context.Context, data watcher.AgentEventData) {
	s.logger.Info("handling agent completed",
		zap.String("task_id", data.TaskID),
		zap.String("agent_execution_id", data.AgentExecutionID))

	// Update scheduler and remove from queue
	s.scheduler.HandleTaskCompleted(data.TaskID, true)
	s.scheduler.RemoveTask(data.TaskID)

	// Move task to REVIEW state for user review
	// The user can then send a follow-up (moves back to IN_PROGRESS) or mark as COMPLETED
	if err := s.taskRepo.UpdateTaskState(ctx, data.TaskID, v1.TaskStateReview); err != nil {
		s.logger.Error("failed to update task state to REVIEW",
			zap.String("task_id", data.TaskID),
			zap.Error(err))
	} else {
		s.logger.Info("task moved to REVIEW state after agent completion",
			zap.String("task_id", data.TaskID))
	}
}

// handleAgentFailed handles agent failure events
func (s *Service) handleAgentFailed(ctx context.Context, data watcher.AgentEventData) {
	s.logger.Warn("handling agent failed",
		zap.String("task_id", data.TaskID),
		zap.String("agent_execution_id", data.AgentExecutionID),
		zap.String("error_message", data.ErrorMessage))

	// Trigger retry logic
	s.scheduler.HandleTaskCompleted(data.TaskID, false)
	if !s.scheduler.RetryTask(data.TaskID) {
		s.logger.Error("task failed and retry limit exceeded",
			zap.String("task_id", data.TaskID))

		// Move task to REVIEW state even on failure - user can decide to retry or close
		// This maintains the review cycle: user reviews the failure and decides next steps
		if err := s.taskRepo.UpdateTaskState(ctx, data.TaskID, v1.TaskStateReview); err != nil {
			s.logger.Error("failed to update task state to REVIEW after failure",
				zap.String("task_id", data.TaskID),
				zap.Error(err))
		} else {
			s.logger.Info("task moved to REVIEW state after failure (for user review)",
				zap.String("task_id", data.TaskID))
		}
	}
}

// handleAgentStreamEvent handles agent stream events (tool calls, message chunks, etc.)
func (s *Service) handleAgentStreamEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload == nil || payload.Data == nil {
		return
	}

	taskID := payload.TaskID
	sessionID := payload.SessionID
	eventType := payload.Data.Type

	s.logger.Debug("handling agent stream event",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("event_type", eventType))

	// Handle different event types
	switch eventType {
	case "tool_call":
		// Create tool call message when a tool call starts
		// If there's accumulated text, save it as an agent message first
		s.saveAgentTextIfPresent(ctx, payload)
		s.handleToolCallEvent(ctx, payload)

	case "tool_update":
		// Update tool call message when status changes
		s.handleToolUpdateEvent(ctx, payload)

	case "complete":
		// Save any accumulated text as an agent message
		s.saveAgentTextIfPresent(ctx, payload)
		// Now that both ACP and Codex Prompt() calls are synchronous (return when turn is done),
		// we can directly finalize the agent ready state here.
		// The "complete" event is now the single source of truth for turn completion.
		s.finalizeAgentReady(taskID, sessionID)

	case "error":
		// Handle error events
		if sessionID != "" && s.messageCreator != nil {
			if err := s.messageCreator.CreateSessionMessage(
				ctx,
				taskID,
				payload.Data.Error,
				sessionID,
				string(v1.MessageTypeError),
				map[string]interface{}{
					"provider":       "agent",
					"provider_agent": payload.AgentID,
				},
				false,
			); err != nil {
				s.logger.Error("failed to create error message",
					zap.String("task_id", taskID),
					zap.Error(err))
			}
		}
	}

	// Broadcast to all registered handlers (WebSocket streaming)
	s.broadcastStreamEvent(payload)
}

// handleToolCallEvent handles tool_call events and creates messages
func (s *Service) handleToolCallEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload.SessionID == "" {
		s.logger.Warn("missing session_id for tool_call",
			zap.String("task_id", payload.TaskID),
			zap.String("tool_call_id", payload.Data.ToolCallID))
		return
	}

	if s.messageCreator != nil {
		if err := s.messageCreator.CreateToolCallMessage(
			ctx,
			payload.TaskID,
			payload.Data.ToolCallID,
			payload.Data.ToolTitle,
			payload.Data.ToolStatus,
			payload.SessionID,
			payload.Data.ToolArgs,
		); err != nil {
			s.logger.Error("failed to create tool call message",
				zap.String("task_id", payload.TaskID),
				zap.String("tool_call_id", payload.Data.ToolCallID),
				zap.Error(err))
		} else {
			s.logger.Info("created tool call message",
				zap.String("task_id", payload.TaskID),
				zap.String("tool_call_id", payload.Data.ToolCallID))
		}

		s.updateTaskSessionState(ctx, payload.TaskID, payload.SessionID, models.TaskSessionStateRunning, "", false)
	}
}

// saveAgentTextIfPresent saves any accumulated agent text as an agent message
func (s *Service) saveAgentTextIfPresent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload.Data.Text == "" || payload.SessionID == "" {
		return
	}

	if s.messageCreator != nil {
		if err := s.messageCreator.CreateAgentMessage(ctx, payload.TaskID, payload.Data.Text, payload.SessionID); err != nil {
			s.logger.Error("failed to create agent message",
				zap.String("task_id", payload.TaskID),
				zap.Error(err))
		} else {
			s.logger.Info("created agent message",
				zap.String("task_id", payload.TaskID),
				zap.Int("message_length", len(payload.Data.Text)))
		}
	}
}

// handleToolUpdateEvent handles tool_update events and updates messages
func (s *Service) handleToolUpdateEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload.SessionID == "" {
		s.logger.Warn("missing session_id for tool_update",
			zap.String("task_id", payload.TaskID),
			zap.String("tool_call_id", payload.Data.ToolCallID))
		return
	}

	// Only update message when tool call completes or errors
	switch payload.Data.ToolStatus {
	case "complete", "completed", "error", "failed":
		if s.messageCreator != nil {
			result := ""
			if payload.Data.ToolResult != nil {
				if str, ok := payload.Data.ToolResult.(string); ok {
					result = str
				}
			}
			if err := s.messageCreator.UpdateToolCallMessage(
				ctx,
				payload.TaskID,
				payload.Data.ToolCallID,
				payload.Data.ToolStatus,
				result,
				payload.SessionID,
			); err != nil {
				s.logger.Warn("failed to update tool call message",
					zap.String("task_id", payload.TaskID),
					zap.String("tool_call_id", payload.Data.ToolCallID),
					zap.Error(err))
			}

			s.updateTaskSessionState(ctx, payload.TaskID, payload.SessionID, models.TaskSessionStateRunning, "", false)
		}
	}
}

func (s *Service) updateTaskSessionState(ctx context.Context, taskID, sessionID string, nextState models.TaskSessionState, errorMessage string, allowWakeFromWaiting bool) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return
	}
	if session.State == models.TaskSessionStateWaitingForInput && nextState == models.TaskSessionStateRunning && !allowWakeFromWaiting {
		return
	}
	oldState := session.State
	switch session.State {
	case models.TaskSessionStateCompleted, models.TaskSessionStateFailed, models.TaskSessionStateCancelled:
		return
	}
	if session.State == nextState {
		return
	}
	if err := s.repo.UpdateTaskSessionState(ctx, sessionID, nextState, errorMessage); err != nil {
		s.logger.Error("failed to update task session state",
			zap.String("session_id", sessionID),
			zap.String("state", string(nextState)),
			zap.Error(err))
	}
	s.logger.Info("task session state updated",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("old_state", string(oldState)),
		zap.String("new_state", string(nextState)))
	if s.eventBus != nil {
		_ = s.eventBus.Publish(ctx, events.TaskSessionStateChanged, bus.NewEvent(events.TaskSessionStateChanged, "task-session", map[string]interface{}{
			"task_id":    taskID,
			"session_id": sessionID,
			"old_state":  string(oldState),
			"new_state":  string(nextState),
		}))
	}
}

func (s *Service) finalizeAgentReady(taskID, sessionID string) {
	ctx := context.Background()
	if sessionID == "" {
		s.logger.Warn("missing session_id for finalizeAgentReady",
			zap.String("task_id", taskID))
		return
	}

	s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateWaitingForInput, "", false)

	task, err := s.taskRepo.GetTask(ctx, taskID)
	if err != nil {
		s.logger.Error("failed to get task for state check",
			zap.String("task_id", taskID),
			zap.Error(err))
		return
	}

	if task.State == v1.TaskStateInProgress {
		if err := s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateReview); err != nil {
			s.logger.Error("failed to update task state to REVIEW",
				zap.String("task_id", taskID),
				zap.Error(err))
		} else {
			s.logger.Info("task moved to REVIEW state",
				zap.String("task_id", taskID))
		}
	} else {
		s.logger.Debug("task not in IN_PROGRESS state, skipping REVIEW transition",
			zap.String("task_id", taskID),
			zap.String("current_state", string(task.State)))
	}
}

// handleGitStatusUpdated handles git status updates and persists them to agent session metadata
func (s *Service) handleGitStatusUpdated(ctx context.Context, data watcher.GitStatusData) {
	s.logger.Debug("handling git status update",
		zap.String("task_id", data.TaskID),
		zap.String("branch", data.Branch))

	if data.TaskSessionID == "" {
		s.logger.Debug("missing session_id for git status update",
			zap.String("task_id", data.TaskID))
		return
	}

	session, err := s.repo.GetTaskSession(ctx, data.TaskSessionID)
	if err != nil {
		s.logger.Debug("no task session for git status update",
			zap.String("session_id", data.TaskSessionID),
			zap.Error(err))
		return
	}

	// Update session metadata with git status
	if session.Metadata == nil {
		session.Metadata = make(map[string]interface{})
	}
	session.Metadata["git_status"] = map[string]interface{}{
		"branch":        data.Branch,
		"remote_branch": data.RemoteBranch,
		"modified":      data.Modified,
		"added":         data.Added,
		"deleted":       data.Deleted,
		"untracked":     data.Untracked,
		"renamed":       data.Renamed,
		"ahead":         data.Ahead,
		"behind":        data.Behind,
		"files":         data.Files,
		"timestamp":     data.Timestamp,
	}

	// Persist to database asynchronously
	go func() {
		if err := s.repo.UpdateTaskSession(context.Background(), session); err != nil {
			s.logger.Error("failed to update agent session with git status",
				zap.String("task_id", data.TaskID),
				zap.String("session_id", session.ID),
				zap.Error(err))
		} else {
			s.logger.Debug("persisted git status to agent session",
				zap.String("task_id", data.TaskID),
				zap.String("session_id", session.ID))
		}
	}()
}

// handlePermissionRequest handles permission request events and saves as message
func (s *Service) handlePermissionRequest(ctx context.Context, data watcher.PermissionRequestData) {
	s.logger.Info("handling permission request",
		zap.String("task_id", data.TaskID),
		zap.String("pending_id", data.PendingID),
		zap.String("title", data.Title))

	if data.TaskSessionID == "" {
		s.logger.Warn("missing session_id for permission_request",
			zap.String("task_id", data.TaskID),
			zap.String("pending_id", data.PendingID))
		return
	}

	if s.messageCreator != nil {
		_, err := s.messageCreator.CreatePermissionRequestMessage(
			ctx,
			data.TaskID,
			data.TaskSessionID,
			data.PendingID,
			data.ToolCallID,
			data.Title,
			data.Options,
			data.ActionType,
			data.ActionDetails,
		)
		if err != nil {
			s.logger.Error("failed to create permission request message",
				zap.String("task_id", data.TaskID),
				zap.String("pending_id", data.PendingID),
				zap.Error(err))
		} else {
			s.logger.Info("created permission request message",
				zap.String("task_id", data.TaskID),
				zap.String("pending_id", data.PendingID))
		}
	}
}
