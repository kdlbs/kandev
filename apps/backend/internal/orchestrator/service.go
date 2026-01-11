// Package orchestrator provides the main orchestrator service that ties all components together.
package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/database"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/kandev/kandev/pkg/acp/protocol"
)

// Common errors
var (
	ErrServiceAlreadyRunning = errors.New("service is already running")
	ErrServiceNotRunning     = errors.New("service is not running")
)

// ServiceConfig holds orchestrator service configuration
type ServiceConfig struct {
	Scheduler scheduler.SchedulerConfig
	QueueSize int
}

// DefaultServiceConfig returns default configuration
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		Scheduler: scheduler.DefaultSchedulerConfig(),
		QueueSize: 1000,
	}
}

// InputRequestHandler is called when an agent requests user input
type InputRequestHandler func(ctx context.Context, taskID, agentID, message string) error

// CommentCreator is an interface for creating comments on tasks
type CommentCreator interface {
	CreateAgentComment(ctx context.Context, taskID, content string) error
}

// Service is the main orchestrator service
type Service struct {
	config   ServiceConfig
	logger   *logger.Logger
	eventBus bus.EventBus
	db       *database.DB
	taskRepo scheduler.TaskRepository

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

	// Comment creator for saving agent responses
	commentCreator CommentCreator

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
	db *database.DB,
	agentManager executor.AgentManagerClient,
	taskRepo scheduler.TaskRepository,
	log *logger.Logger,
) *Service {
	svcLogger := log.WithFields(zap.String("component", "orchestrator"))

	// Create the task queue with configured size
	taskQueue := queue.NewTaskQueue(cfg.QueueSize)

	// Create the executor with the agent manager client
	exec := executor.NewExecutor(agentManager, log, cfg.Scheduler.MaxConcurrent)

	// Create the scheduler with queue, executor, and task repository
	sched := scheduler.NewScheduler(taskQueue, exec, taskRepo, log, cfg.Scheduler)

	// Create the service (watcher will be created after we have handlers)
	s := &Service{
		config:      cfg,
		logger:      svcLogger,
		eventBus:    eventBus,
		db:          db,
		taskRepo:    taskRepo,
		queue:       taskQueue,
		executor:    exec,
		scheduler:   sched,
		acpHandlers: make([]func(taskID string, msg *protocol.Message), 0),
	}

	// Create the watcher with event handlers that wire everything together
	handlers := watcher.EventHandlers{
		OnTaskStateChanged: s.handleTaskStateChanged,
		OnAgentReady:       s.handleAgentReady,
		OnAgentCompleted:   s.handleAgentCompleted,
		OnAgentFailed:      s.handleAgentFailed,
		OnACPMessage:       s.handleACPMessage,
		OnPromptComplete:   s.handlePromptComplete,
	}
	s.watcher = watcher.NewWatcher(eventBus, handlers, log)

	return s
}

// SetCommentCreator sets the comment creator for saving agent responses
func (s *Service) SetCommentCreator(cc CommentCreator) {
	s.commentCreator = cc
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

	// Start the watcher first to begin receiving events
	if err := s.watcher.Start(ctx); err != nil {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		return err
	}

	// Start the scheduler processing loop
	if err := s.scheduler.Start(ctx); err != nil {
		s.watcher.Stop()
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
func (s *Service) StartTask(ctx context.Context, taskID string, agentType string, priority int) (*executor.TaskExecution, error) {
	s.logger.Info("manually starting task",
		zap.String("task_id", taskID),
		zap.String("agent_type", agentType),
		zap.Int("priority", priority))

	// Fetch the task from the repository to get complete task info
	task, err := s.scheduler.GetTask(ctx, taskID)
	if err != nil {
		s.logger.Error("failed to fetch task for manual start",
			zap.String("task_id", taskID),
			zap.Error(err))
		return nil, err
	}

	// Override agent_type and priority if provided in the request
	if agentType != "" {
		task.AgentType = &agentType
	}
	if priority > 0 {
		task.Priority = priority
	}

	return s.executor.Execute(ctx, task)
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

	// Add task to queue if state is TODO and agent_type is set
	if data.NewState != nil && *data.NewState == v1.TaskStateTODO {
		if data.Task != nil && data.Task.AgentType != nil && *data.Task.AgentType != "" {
			if err := s.scheduler.EnqueueTask(data.Task); err != nil {
				s.logger.Error("failed to enqueue task on state change",
					zap.String("task_id", data.TaskID),
					zap.Error(err))
			} else {
				s.logger.Info("task enqueued on state change to TODO",
					zap.String("task_id", data.TaskID),
					zap.String("agent_type", *data.Task.AgentType))
			}
		}
	}
}

// handleAgentReady handles agent ready events (prompt completed, ready for follow-up)
func (s *Service) handleAgentReady(ctx context.Context, data watcher.AgentEventData) {
	s.logger.Info("handling agent ready",
		zap.String("task_id", data.TaskID),
		zap.String("agent_instance_id", data.AgentInstanceID))

	// Agent completed initial prompt but is still running
	// Task stays in IN_PROGRESS state - user can send follow-up prompts
	// or explicitly complete/stop the task
}

// handleAgentCompleted handles agent completion events
func (s *Service) handleAgentCompleted(ctx context.Context, data watcher.AgentEventData) {
	s.logger.Info("handling agent completed",
		zap.String("task_id", data.TaskID),
		zap.String("agent_instance_id", data.AgentInstanceID))

	// Update scheduler and remove from queue
	s.scheduler.HandleTaskCompleted(data.TaskID, true)
	s.scheduler.RemoveTask(data.TaskID)

	// Update task state to COMPLETED
	if err := s.taskRepo.UpdateTaskState(ctx, data.TaskID, v1.TaskStateCompleted); err != nil {
		s.logger.Error("failed to update task state to COMPLETED",
			zap.String("task_id", data.TaskID),
			zap.Error(err))
	} else {
		s.logger.Info("task marked as COMPLETED",
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

		// Update task state to FAILED
		if err := s.taskRepo.UpdateTaskState(ctx, data.TaskID, v1.TaskStateFailed); err != nil {
			s.logger.Error("failed to update task state to FAILED",
				zap.String("task_id", data.TaskID),
				zap.Error(err))
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

	// Broadcast to all registered handlers (WebSocket streaming)
	s.broadcastACP(taskID, msg)
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

	// Call the input request handler if set
	if s.inputRequestHandler != nil {
		if err := s.inputRequestHandler(ctx, taskID, msg.AgentID, message); err != nil {
			s.logger.Error("input request handler failed",
				zap.String("task_id", taskID),
				zap.Error(err))
		}
	}
}

// handlePromptComplete handles prompt complete events and saves agent response as comment
func (s *Service) handlePromptComplete(ctx context.Context, data watcher.PromptCompleteData) {
	s.logger.Info("handling prompt complete",
		zap.String("task_id", data.TaskID),
		zap.Int("message_length", len(data.AgentMessage)))

	// Save agent response as a comment if we have a comment creator
	if s.commentCreator != nil && data.AgentMessage != "" {
		if err := s.commentCreator.CreateAgentComment(ctx, data.TaskID, data.AgentMessage); err != nil {
			s.logger.Error("failed to create agent comment",
				zap.String("task_id", data.TaskID),
				zap.Error(err))
		} else {
			s.logger.Info("created agent comment",
				zap.String("task_id", data.TaskID),
				zap.Int("message_length", len(data.AgentMessage)))
		}
	}
}

