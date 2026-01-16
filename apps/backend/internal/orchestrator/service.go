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

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/pkg/acp/protocol"
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

// InputRequestHandler is called when an agent requests user input
type InputRequestHandler func(ctx context.Context, taskID, agentID, message string) error

// MessageCreator is an interface for creating messages on tasks
type MessageCreator interface {
	CreateAgentMessage(ctx context.Context, taskID, content, agentSessionID string) error
	CreateUserMessage(ctx context.Context, taskID, content, agentSessionID string) error
	CreateToolCallMessage(ctx context.Context, taskID, toolCallID, title, status, agentSessionID string, args map[string]interface{}) error
	UpdateToolCallMessage(ctx context.Context, taskID, toolCallID, status, result, agentSessionID string) error
	CreateSessionMessage(ctx context.Context, taskID, content, agentSessionID, messageType string, metadata map[string]interface{}, requestsInput bool) error
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

	// ACP message handlers (for WebSocket streaming)
	acpHandlers []func(taskID string, msg *protocol.Message)
	acpMu       sync.RWMutex

	// Input request handler (for agent-user conversation)
	inputRequestHandler InputRequestHandler

	// Message creator for saving agent responses
	messageCreator MessageCreator

	// Service state
	mu        sync.RWMutex
	running   bool
	startedAt time.Time

	readyMu     sync.Mutex
	readyStates map[string]*readyState
}

type readyState struct {
	readySeen          bool
	promptCompleteSeen bool
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
	log *logger.Logger,
) *Service {
	svcLogger := log.WithFields(zap.String("component", "orchestrator"))

	// Create the task queue with configured size
	taskQueue := queue.NewTaskQueue(cfg.QueueSize)

	// Create the executor with the agent manager client and repository for persistent sessions
	execCfg := executor.ExecutorConfig{
		MaxConcurrent:   cfg.Scheduler.MaxConcurrent,
		WorktreeEnabled: cfg.WorktreeEnabled,
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
		acpHandlers:  make([]func(taskID string, msg *protocol.Message), 0),
		readyStates:  make(map[string]*readyState),
	}

	// Create the watcher with event handlers that wire everything together
	handlers := watcher.EventHandlers{
		OnTaskStateChanged:  s.handleTaskStateChanged,
		OnAgentReady:        s.handleAgentReady,
		OnAgentCompleted:    s.handleAgentCompleted,
		OnAgentFailed:       s.handleAgentFailed,
		OnACPMessage:        s.handleACPMessage,
		OnACPSessionCreated: s.handleACPSessionCreated,
		OnPromptComplete:    s.handlePromptComplete,
		OnToolCallStarted:   s.handleToolCallStarted,
		OnToolCallComplete:  s.handleToolCallComplete,
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

	// Load any active sessions from the database to restore state
	if err := s.executor.LoadActiveSessionsFromDB(ctx); err != nil {
		s.logger.Warn("failed to load active sessions from database (non-fatal)", zap.Error(err))
	}

	// Sync with instances recovered from Docker by the lifecycle manager
	// This ensures we track containers that are running but weren't in the DB as active
	recovered := s.agentManager.GetRecoveredInstances()
	if len(recovered) > 0 {
		s.logger.Info("syncing with recovered Docker instances", zap.Int("count", len(recovered)))
		s.executor.SyncWithRecoveredInstances(ctx, recovered)
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
func (s *Service) StartTask(ctx context.Context, taskID string, agentProfileID string, priority int) (*executor.TaskExecution, error) {
	s.logger.Info("manually starting task",
		zap.String("task_id", taskID),
		zap.String("agent_profile_id", agentProfileID),
		zap.Int("priority", priority))

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

	execution, err := s.executor.ExecuteWithProfile(ctx, task, agentProfileID)
	if err != nil {
		return nil, err
	}

	if execution.SessionID != "" {
		s.updateTaskSessionState(ctx, taskID, execution.SessionID, models.TaskSessionStateRunning, "", true)
		if s.messageCreator != nil && task.Description != "" {
			if err := s.messageCreator.CreateUserMessage(ctx, taskID, task.Description, execution.SessionID); err != nil {
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
		zap.String("task_session_id", sessionID))

	if exec, ok := s.executor.GetExecutionWithContext(ctx, taskID); ok && exec != nil {
		return nil, executor.ErrExecutionAlreadyRunning
	}

	if err := s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateScheduling); err != nil {
		s.logger.Warn("failed to update task state to SCHEDULING",
			zap.String("task_id", taskID),
			zap.Error(err))
	}

	task, err := s.scheduler.GetTask(ctx, taskID)
	if err != nil {
		s.logger.Error("failed to fetch task for resume",
			zap.String("task_id", taskID),
			zap.Error(err))
		return nil, err
	}

	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.TaskID != taskID {
		return nil, fmt.Errorf("task session does not belong to task")
	}
	if len(session.Worktrees) == 0 || session.Worktrees[0].WorktreePath == "" {
		return nil, fmt.Errorf("task session has no worktree path")
	}
	if _, err := os.Stat(session.Worktrees[0].WorktreePath); err != nil {
		return nil, fmt.Errorf("worktree path not found: %w", err)
	}

	s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateStarting, "", true)

	execution, err := s.executor.ResumeSession(ctx, task, session)
	if err != nil {
		return nil, err
	}

	s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateRunning, "", true)

	if err := s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateInProgress); err != nil {
		s.logger.Warn("failed to update task state to IN_PROGRESS",
			zap.String("task_id", taskID),
			zap.Error(err))
	}

	return execution, nil
}

// StopTask stops agent execution for a task
func (s *Service) StopTask(ctx context.Context, taskID string, reason string, force bool) error {
	s.logger.Info("stopping task execution",
		zap.String("task_id", taskID),
		zap.String("reason", reason),
		zap.Bool("force", force))
	return s.executor.Stop(ctx, taskID, reason, force)
}

// PromptResult contains the result of a prompt operation
type PromptResult struct {
	StopReason   string // The reason the agent stopped (e.g., "end_turn")
	AgentMessage string // The agent's accumulated response message
}

// PromptTask sends a follow-up prompt to a running agent for a task
func (s *Service) PromptTask(ctx context.Context, taskID string, prompt string) (*PromptResult, error) {
	s.logger.Info("sending prompt to task agent",
		zap.String("task_id", taskID),
		zap.Int("prompt_length", len(prompt)))
	if sessionID, err := s.executor.GetActiveSessionID(ctx, taskID); err == nil && sessionID != "" {
		s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateRunning, "", true)
	}
	result, err := s.executor.Prompt(ctx, taskID, prompt)
	if err != nil {
		return nil, err
	}
	return &PromptResult{
		StopReason:   result.StopReason,
		AgentMessage: result.AgentMessage,
	}, nil
}

// RespondToPermission sends a response to a permission request for a task
func (s *Service) RespondToPermission(ctx context.Context, taskID, pendingID, optionID string, cancelled bool) error {
	s.logger.Info("responding to permission request",
		zap.String("task_id", taskID),
		zap.String("pending_id", pendingID),
		zap.String("option_id", optionID),
		zap.Bool("cancelled", cancelled))
	return s.executor.RespondToPermission(ctx, taskID, pendingID, optionID, cancelled)
}

// CompleteTask explicitly completes a task and stops its agent
func (s *Service) CompleteTask(ctx context.Context, taskID string) error {
	s.logger.Info("completing task",
		zap.String("task_id", taskID))

	// Stop the agent (which will trigger AgentCompleted event and update task state)
	if err := s.executor.Stop(ctx, taskID, "task completed by user", false); err != nil {
		// If agent is already stopped, just update the task state directly
		s.logger.Warn("failed to stop agent, updating task state directly",
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

// RegisterACPHandler registers a handler for ACP messages (used by WebSocket streaming)
func (s *Service) RegisterACPHandler(handler func(taskID string, msg *protocol.Message)) {
	s.acpMu.Lock()
	defer s.acpMu.Unlock()
	s.acpHandlers = append(s.acpHandlers, handler)
	s.logger.Debug("registered ACP handler", zap.Int("total_handlers", len(s.acpHandlers)))
}

// UnregisterACPHandler removes an ACP handler
func (s *Service) UnregisterACPHandler(handler func(taskID string, msg *protocol.Message)) {
	s.acpMu.Lock()
	defer s.acpMu.Unlock()

	// Find and remove the handler by comparing function pointers
	// Note: This uses a simple approach - in production you might want to use handler IDs
	for i := range s.acpHandlers {
		// We can't directly compare functions, so we'll keep the handler in the list
		// In a real implementation, you'd use unique handler IDs
		_ = i
	}
	s.logger.Debug("unregister ACP handler called")
}

// broadcastACP broadcasts an ACP message to all registered handlers
func (s *Service) broadcastACP(taskID string, msg *protocol.Message) {
	s.acpMu.RLock()
	handlers := make([]func(taskID string, msg *protocol.Message), len(s.acpHandlers))
	copy(handlers, s.acpHandlers)
	s.acpMu.RUnlock()

	for _, handler := range handlers {
		handler(taskID, msg)
	}
}

// SetInputRequestHandler sets the handler for agent input requests
func (s *Service) SetInputRequestHandler(handler InputRequestHandler) {
	s.inputRequestHandler = handler
	s.logger.Debug("input request handler set")
}

// Event handlers

// handleTaskStateChanged handles task state change events
func (s *Service) handleTaskStateChanged(ctx context.Context, data watcher.TaskEventData) {
	s.logger.Debug("handling task state changed",
		zap.String("task_id", data.TaskID))

	// No auto-enqueue on TODO; sessions are started explicitly with agent profile.
}

// handleAgentReady handles agent ready events (prompt completed, ready for follow-up)
// This is the definitive signal that the agent has finished processing a prompt.
// AgentReady fires ONCE after each prompt completes (unlike PromptComplete which
// fires for intermediate messages before tool calls).
func (s *Service) handleAgentReady(ctx context.Context, data watcher.AgentEventData) {
	s.logger.Info("handling agent ready",
		zap.String("task_id", data.TaskID),
		zap.String("agent_instance_id", data.AgentInstanceID))

	if s.markAgentReady(data.TaskID) {
		s.finalizeAgentReady(data.TaskID, "")
	}
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

	if session.Metadata == nil {
		session.Metadata = make(map[string]interface{})
	}
	if existing, ok := session.Metadata["acp_session_id"].(string); ok && existing == data.ACPSessionID {
		return
	}
	session.Metadata["acp_session_id"] = data.ACPSessionID

	if err := s.repo.UpdateTaskSession(ctx, session); err != nil {
		s.logger.Warn("failed to persist ACP session id",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}

	s.logger.Info("stored ACP session id for task session",
		zap.String("task_id", data.TaskID),
		zap.String("task_session_id", sessionID),
		zap.String("acp_session_id", data.ACPSessionID))
}

// handleAgentCompleted handles agent completion events
func (s *Service) handleAgentCompleted(ctx context.Context, data watcher.AgentEventData) {
	s.logger.Info("handling agent completed",
		zap.String("task_id", data.TaskID),
		zap.String("agent_instance_id", data.AgentInstanceID))

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
		zap.String("agent_instance_id", data.AgentInstanceID),
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

// handleACPMessage handles ACP messages from agents
func (s *Service) handleACPMessage(ctx context.Context, taskID string, msg *protocol.Message) {
	s.logger.Debug("handling ACP message",
		zap.String("task_id", taskID),
		zap.String("message_type", string(msg.Type)))

	// Handle input_required messages specially
	if msg.Type == protocol.MessageTypeInputRequired {
		s.handleInputRequired(ctx, taskID, msg)
	}

	// Normalize ACP messages into session messages for persistence.
	normalized := s.normalizeACPMessage(msg)
	if normalized != nil && s.messageCreator != nil {
		sessionID, err := s.executor.GetActiveSessionID(ctx, taskID)
		if err == nil && sessionID != "" {
			if err := s.messageCreator.CreateSessionMessage(
				ctx,
				taskID,
				normalized.content,
				sessionID,
				normalized.messageType,
				normalized.metadata,
				normalized.requestsInput,
			); err != nil {
				s.logger.Error("failed to create normalized session message",
					zap.String("task_id", taskID),
					zap.String("message_type", normalized.messageType),
					zap.Error(err))
			}

			if normalized.bumpToRunning {
				s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateRunning, "", false)
			}
		}
	}

	// Broadcast to all registered handlers (WebSocket streaming)
	s.broadcastACP(taskID, msg)
}

type normalizedACPMessage struct {
	messageType   string
	content       string
	metadata      map[string]interface{}
	requestsInput bool
	bumpToRunning bool
}

func (s *Service) normalizeACPMessage(msg *protocol.Message) *normalizedACPMessage {
	if msg == nil {
		return nil
	}

	switch msg.Type {
	case protocol.MessageTypeProgress:
		return &normalizedACPMessage{
			messageType:   string(v1.MessageTypeProgress),
			content:       extractACPContent(msg),
			metadata:      buildACPMetadata(msg),
			bumpToRunning: true,
		}
	case protocol.MessageTypeStatus:
		return &normalizedACPMessage{
			messageType:   string(v1.MessageTypeStatus),
			content:       extractACPContent(msg),
			metadata:      buildACPMetadata(msg),
			bumpToRunning: true,
		}
	case protocol.MessageTypeLog:
		return &normalizedACPMessage{
			messageType:   string(v1.MessageTypeStatus),
			content:       extractACPContent(msg),
			metadata:      buildACPMetadata(msg),
			bumpToRunning: true,
		}
	case protocol.MessageTypeError:
		return &normalizedACPMessage{
			messageType:   string(v1.MessageTypeError),
			content:       extractACPContent(msg),
			metadata:      buildACPMetadata(msg),
			bumpToRunning: true,
		}
	case protocol.MessageTypeInputRequired:
		return nil
	default:
		return nil
	}
}

func extractACPContent(msg *protocol.Message) string {
	if msg == nil || msg.Data == nil {
		return string(msg.Type)
	}
	for _, key := range []string{"message", "text", "content", "detail"} {
		if value, ok := msg.Data[key].(string); ok && value != "" {
			return value
		}
	}
	return string(msg.Type)
}

func buildACPMetadata(msg *protocol.Message) map[string]interface{} {
	if msg == nil {
		return nil
	}
	metadata := map[string]interface{}{
		"provider":       "acp",
		"provider_type":  string(msg.Type),
		"provider_agent": msg.AgentID,
	}
	if len(msg.Data) > 0 {
		metadata["provider_data"] = msg.Data
	}
	return metadata
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
			"task_id":         taskID,
			"task_session_id": sessionID,
			"old_state":       string(oldState),
			"new_state":       string(nextState),
		}))
	}
	if taskID != "" {
		s.executor.UpdateExecutionState(taskID, v1.TaskSessionState(nextState))
	}
}

func (s *Service) markAgentReady(taskID string) bool {
	s.readyMu.Lock()
	defer s.readyMu.Unlock()

	state, ok := s.readyStates[taskID]
	if !ok {
		state = &readyState{}
		s.readyStates[taskID] = state
	}

	state.readySeen = true
	readyToFinalize := state.promptCompleteSeen
	if readyToFinalize {
		delete(s.readyStates, taskID)
	}

	return readyToFinalize
}

func (s *Service) markPromptComplete(taskID string) bool {
	s.readyMu.Lock()
	defer s.readyMu.Unlock()

	state, ok := s.readyStates[taskID]
	if !ok {
		state = &readyState{}
		s.readyStates[taskID] = state
	}

	state.promptCompleteSeen = true
	readyToFinalize := state.readySeen
	if readyToFinalize {
		delete(s.readyStates, taskID)
	}

	return readyToFinalize
}

func (s *Service) finalizeAgentReady(taskID, sessionID string) {
	ctx := context.Background()
	if sessionID == "" {
		sessionID, _ = s.executor.GetActiveSessionID(ctx, taskID)
	}

	if sessionID != "" {
		s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateWaitingForInput, "", false)
	}

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

// handleInputRequired handles agent input request messages
func (s *Service) handleInputRequired(ctx context.Context, taskID string, msg *protocol.Message) {
	s.logger.Info("agent requesting user input",
		zap.String("task_id", taskID),
		zap.String("agent_id", msg.AgentID))

	// Extract message from data
	message := ""
	if msg.Data != nil {
		if m, ok := msg.Data["message"].(string); ok {
			message = m
		}
	}

	// Update task state to WAITING_FOR_INPUT
	if err := s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateWaitingForInput); err != nil {
		s.logger.Error("failed to update task state to WAITING_FOR_INPUT",
			zap.String("task_id", taskID),
			zap.Error(err))
	}

	// Update session state to WAITING_FOR_INPUT
	if sessionID, err := s.executor.GetActiveSessionID(ctx, taskID); err == nil && sessionID != "" {
		s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateWaitingForInput, "", false)
	}

	// Call the input request handler if set
	if s.inputRequestHandler != nil {
		if err := s.inputRequestHandler(ctx, taskID, msg.AgentID, message); err != nil {
			s.logger.Error("input request handler failed",
				zap.String("task_id", taskID),
				zap.Error(err))
		}
	}
}

// handleGitStatusUpdated handles git status updates and persists them to agent session metadata
func (s *Service) handleGitStatusUpdated(ctx context.Context, data watcher.GitStatusData) {
	s.logger.Debug("handling git status update",
		zap.String("task_id", data.TaskID),
		zap.String("branch", data.Branch))

	// Get the active agent session for this task
	session, err := s.repo.GetActiveTaskSessionByTaskID(ctx, data.TaskID)
	if err != nil {
		s.logger.Debug("no active agent session for git status update",
			zap.String("task_id", data.TaskID),
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

// handlePromptComplete handles prompt complete events and saves agent response as message
// Note: This is called for EVERY agent message (including intermediate ones before tool calls),
// so we should NOT transition task state here. State transitions happen in the comment handler
// after the synchronous PromptTask call returns.
func (s *Service) handlePromptComplete(ctx context.Context, data watcher.PromptCompleteData) {
	s.logger.Info("handling prompt complete",
		zap.String("task_id", data.TaskID),
		zap.Int("message_length", len(data.AgentMessage)))

	// Save agent response as a message if we have a message creator
	if s.messageCreator != nil && data.AgentMessage != "" {
		// Get the active session ID for this task
		sessionID, _ := s.executor.GetActiveSessionID(ctx, data.TaskID)

		if err := s.messageCreator.CreateAgentMessage(ctx, data.TaskID, data.AgentMessage, sessionID); err != nil {
			s.logger.Error("failed to create agent message",
				zap.String("task_id", data.TaskID),
				zap.Error(err))
		} else {
			s.logger.Info("created agent message",
				zap.String("task_id", data.TaskID),
				zap.Int("message_length", len(data.AgentMessage)))
		}

		if s.markPromptComplete(data.TaskID) {
			s.finalizeAgentReady(data.TaskID, sessionID)
		}
	}
}

// handleToolCallStarted handles tool call started events and saves as message
func (s *Service) handleToolCallStarted(ctx context.Context, data watcher.ToolCallData) {
	s.logger.Info("handling tool call started",
		zap.String("task_id", data.TaskID),
		zap.String("tool_call_id", data.ToolCallID),
		zap.String("title", data.Title))

	// Save tool call as a message if we have a message creator
	if s.messageCreator != nil {
		// Get the active session ID for this task
		sessionID, _ := s.executor.GetActiveSessionID(ctx, data.TaskID)

		if err := s.messageCreator.CreateToolCallMessage(ctx, data.TaskID, data.ToolCallID, data.Title, data.Status, sessionID, data.Args); err != nil {
			s.logger.Error("failed to create tool call message",
				zap.String("task_id", data.TaskID),
				zap.String("tool_call_id", data.ToolCallID),
				zap.Error(err))
		} else {
			s.logger.Info("created tool call message",
				zap.String("task_id", data.TaskID),
				zap.String("tool_call_id", data.ToolCallID))
		}

		if sessionID != "" {
			s.updateTaskSessionState(ctx, data.TaskID, sessionID, models.TaskSessionStateRunning, "", false)
		}
	}
}

// handleToolCallComplete handles tool call complete events and updates the message
func (s *Service) handleToolCallComplete(ctx context.Context, data watcher.ToolCallCompleteData) {
	// Update tool call message status if we have a message creator
	if s.messageCreator != nil {
		sessionID, _ := s.executor.GetActiveSessionID(ctx, data.TaskID)
		if err := s.messageCreator.UpdateToolCallMessage(ctx, data.TaskID, data.ToolCallID, data.Status, data.Result, sessionID); err != nil {
			s.logger.Error("failed to update tool call message",
				zap.String("task_id", data.TaskID),
				zap.String("tool_call_id", data.ToolCallID),
				zap.Error(err))
		}

		if sessionID != "" {
			s.updateTaskSessionState(ctx, data.TaskID, sessionID, models.TaskSessionStateRunning, "", false)
		}
	}
}
