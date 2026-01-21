// Package orchestrator provides the main orchestrator service that ties all components together.
package orchestrator

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
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
	CreateToolCallMessage(ctx context.Context, taskID, toolCallID, title, status, agentSessionID, turnID string, args map[string]interface{}) error
	UpdateToolCallMessage(ctx context.Context, taskID, toolCallID, status, result, agentSessionID string) error
	CreateSessionMessage(ctx context.Context, taskID, content, agentSessionID, messageType, turnID string, metadata map[string]interface{}, requestsInput bool) error
	CreatePermissionRequestMessage(ctx context.Context, taskID, sessionID, pendingID, toolCallID, title, turnID string, options []map[string]interface{}, actionType string, actionDetails map[string]interface{}) (string, error)
	UpdatePermissionMessage(ctx context.Context, sessionID, pendingID, status string) error
	// CreateAgentMessageStreaming creates a new agent message with a pre-generated ID for streaming updates
	CreateAgentMessageStreaming(ctx context.Context, messageID, taskID, content, agentSessionID, turnID string) error
	// AppendAgentMessage appends additional content to an existing streaming message
	AppendAgentMessage(ctx context.Context, messageID, additionalContent string) error
}

// TurnService is an interface for managing session turns
type TurnService interface {
	StartTurn(ctx context.Context, sessionID string) (*models.Turn, error)
	CompleteTurn(ctx context.Context, turnID string) error
	GetActiveTurn(ctx context.Context, sessionID string) (*models.Turn, error)
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
		OnAgentCompleted:       s.handleAgentCompleted,
		OnAgentFailed:          s.handleAgentFailed,
		OnAgentStreamEvent:     s.handleAgentStreamEvent,
		OnACPSessionCreated:    s.handleACPSessionCreated,
		OnPermissionRequest:    s.handlePermissionRequest,
		OnGitStatusUpdated:     s.handleGitStatusUpdated,
		OnContextWindowUpdated: s.handleContextWindowUpdated,
	}
	s.watcher = watcher.NewWatcher(eventBus, handlers, log)

	return s
}

// SetMessageCreator sets the message creator for saving agent responses
func (s *Service) SetMessageCreator(mc MessageCreator) {
	s.messageCreator = mc
}

// SetTurnService sets the turn service for the orchestrator.
func (s *Service) SetTurnService(turnService TurnService) {
	s.turnService = turnService
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

		startAgent := running.ResumeToken != "" && running.Resumable
		if !startAgent {
			s.logger.Warn("resuming workspace without agent session",
				zap.String("session_id", sessionID),
				zap.String("previous_state", string(previousState)),
				zap.Bool("has_resume_token", running.ResumeToken != ""),
				zap.Bool("resumable", running.Resumable))
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
		}

		s.logger.Info("session resumed successfully",
			zap.String("session_id", sessionID),
			zap.String("task_id", running.TaskID),
			zap.Bool("start_agent", startAgent),
			zap.String("state", string(models.TaskSessionStateWaitingForInput)))
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
