// Package scheduler processes the task queue and coordinates execution.
package scheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

// Common errors
var (
	ErrSchedulerAlreadyRunning = errors.New("scheduler is already running")
	ErrSchedulerNotRunning     = errors.New("scheduler is not running")
	ErrTaskNotFound            = errors.New("task not found")
)

// SchedulerConfig holds scheduler configuration
type SchedulerConfig struct {
	ProcessInterval time.Duration // How often to process the queue
	MaxConcurrent   int           // Max concurrent agent executions
	RetryLimit      int           // Max retries for failed tasks
	RetryDelay      time.Duration // Delay between retries
}

// DefaultSchedulerConfig returns default configuration
func DefaultSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		ProcessInterval: 5 * time.Second,
		MaxConcurrent:   5,
		RetryLimit:      3,
		RetryDelay:      30 * time.Second,
	}
}

// TaskRepository interface for loading task data
type TaskRepository interface {
	GetTask(ctx context.Context, taskID string) (*v1.Task, error)
	UpdateTaskState(ctx context.Context, taskID string, state v1.TaskState) error
}

// QueueStatus contains queue statistics
type QueueStatus struct {
	QueuedTasks      int
	ActiveExecutions int
	MaxConcurrent    int
	TotalProcessed   int64
	TotalFailed      int64
}

// Scheduler manages the task queue processing loop
type Scheduler struct {
	queue    *queue.TaskQueue
	executor *executor.Executor
	taskRepo TaskRepository
	logger   *logger.Logger
	config   SchedulerConfig

	// Retry tracking
	retryCount map[string]int
	retryMu    sync.RWMutex

	// Statistics
	totalProcessed int64
	totalFailed    int64

	mu      sync.RWMutex
	running bool
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

// NewScheduler creates a new scheduler
func NewScheduler(
	q *queue.TaskQueue,
	exec *executor.Executor,
	taskRepo TaskRepository,
	log *logger.Logger,
	config SchedulerConfig,
) *Scheduler {
	return &Scheduler{
		queue:      q,
		executor:   exec,
		taskRepo:   taskRepo,
		logger:     log.WithFields(zap.String("component", "scheduler")),
		config:     config,
		retryCount: make(map[string]int),
	}
}

// Start begins the scheduler processing loop
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return ErrSchedulerAlreadyRunning
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

	s.logger.Info("scheduler starting",
		zap.Duration("process_interval", s.config.ProcessInterval),
		zap.Int("max_concurrent", s.config.MaxConcurrent))

	s.wg.Add(1)
	go s.processLoop(ctx)

	return nil
}

// Stop stops the scheduler
func (s *Scheduler) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return ErrSchedulerNotRunning
	}
	s.running = false
	close(s.stopCh)
	s.mu.Unlock()

	s.wg.Wait()
	s.logger.Info("scheduler stopped")
	return nil
}

// IsRunning returns true if the scheduler is active
func (s *Scheduler) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// EnqueueTask adds a task to the queue (called by watcher when task state changes)
func (s *Scheduler) EnqueueTask(task *v1.Task) error {
	s.logger.Info("enqueueing task",
		zap.String("task_id", task.ID),
		zap.String("title", task.Title),
		zap.Int("priority", task.Priority))
	return s.queue.Enqueue(task)
}

// RemoveTask removes a task from the queue
func (s *Scheduler) RemoveTask(taskID string) bool {
	removed := s.queue.Remove(taskID)
	if removed {
		s.logger.Info("removed task from queue", zap.String("task_id", taskID))
	}
	return removed
}

// GetQueueStatus returns the current queue status
func (s *Scheduler) GetQueueStatus() *QueueStatus {
	return &QueueStatus{
		QueuedTasks:      s.queue.Len(),
		ActiveExecutions: s.executor.ActiveCount(),
		MaxConcurrent:    s.config.MaxConcurrent,
		TotalProcessed:   atomic.LoadInt64(&s.totalProcessed),
		TotalFailed:      atomic.LoadInt64(&s.totalFailed),
	}
}

// GetTask retrieves a task from the repository
func (s *Scheduler) GetTask(ctx context.Context, taskID string) (*v1.Task, error) {
	return s.taskRepo.GetTask(ctx, taskID)
}

// HandleTaskCompleted handles agent completion
func (s *Scheduler) HandleTaskCompleted(taskID string, success bool) {
	s.logger.Info("handling task completion",
		zap.String("task_id", taskID),
		zap.Bool("success", success))

	if success {
		atomic.AddInt64(&s.totalProcessed, 1)
		// Clear retry count on success
		s.retryMu.Lock()
		delete(s.retryCount, taskID)
		s.retryMu.Unlock()
	} else {
		atomic.AddInt64(&s.totalFailed, 1)
	}
}

// RetryTask re-queues a failed task if retry limit not exceeded
func (s *Scheduler) RetryTask(taskID string) bool {
	s.retryMu.Lock()
	count := s.retryCount[taskID]
	if count >= s.config.RetryLimit {
		s.retryMu.Unlock()
		s.logger.Warn("retry limit exceeded for task",
			zap.String("task_id", taskID),
			zap.Int("retry_count", count),
			zap.Int("retry_limit", s.config.RetryLimit))
		return false
	}
	s.retryCount[taskID] = count + 1
	s.retryMu.Unlock()

	// Fetch task and re-enqueue
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	task, err := s.taskRepo.GetTask(ctx, taskID)
	if err != nil {
		s.logger.Error("failed to fetch task for retry",
			zap.String("task_id", taskID),
			zap.Error(err))
		return false
	}

	// Schedule retry after delay
	go func() {
		time.Sleep(s.config.RetryDelay)
		if err := s.queue.Enqueue(task); err != nil {
			s.logger.Error("failed to re-enqueue task for retry",
				zap.String("task_id", taskID),
				zap.Error(err))
		} else {
			s.logger.Info("task re-enqueued for retry",
				zap.String("task_id", taskID),
				zap.Int("retry_attempt", count+1))
		}
	}()

	return true
}

// processLoop is the main processing loop
func (s *Scheduler) processLoop(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.ProcessInterval)
	defer ticker.Stop()

	s.logger.Info("scheduler processing loop started")

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("scheduler stopping due to context cancellation")
			return
		case <-s.stopCh:
			s.logger.Info("scheduler stopping due to stop signal")
			return
		case <-ticker.C:
			s.processTasks(ctx)
		}
	}
}

// processTasks processes pending tasks from the queue
func (s *Scheduler) processTasks(ctx context.Context) {
	// Process tasks while we have capacity
	for s.executor.CanExecute() {
		// Check if we should stop
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		default:
		}

		// Dequeue the next task
		queuedTask := s.queue.Dequeue()
		if queuedTask == nil {
			// Queue is empty
			return
		}

		task := queuedTask.Task
		s.logger.Info("processing task from queue",
			zap.String("task_id", task.ID),
			zap.String("title", task.Title),
			zap.Int("priority", queuedTask.Priority))

		// Update task state to IN_PROGRESS
		if err := s.taskRepo.UpdateTaskState(ctx, task.ID, v1.TaskStateInProgress); err != nil {
			s.logger.Error("failed to update task state to IN_PROGRESS",
				zap.String("task_id", task.ID),
				zap.Error(err))
			// Re-enqueue the task
			if enqErr := s.queue.Enqueue(task); enqErr != nil {
				s.logger.Error("failed to re-enqueue task after state update failure",
					zap.String("task_id", task.ID),
					zap.Error(enqErr))
			}
			continue
		}

		// Execute the task
		_, err := s.executor.Execute(ctx, task)
		if err != nil {
			s.logger.Error("failed to execute task",
				zap.String("task_id", task.ID),
				zap.Error(err))

			// Attempt retry
			if !s.RetryTask(task.ID) {
				// Retry limit exceeded, mark as failed
				if stateErr := s.taskRepo.UpdateTaskState(ctx, task.ID, v1.TaskStateFailed); stateErr != nil {
					s.logger.Error("failed to update task state to FAILED",
						zap.String("task_id", task.ID),
						zap.Error(stateErr))
				}
				atomic.AddInt64(&s.totalFailed, 1)
			}
			continue
		}

		atomic.AddInt64(&s.totalProcessed, 1)
		s.logger.Info("task execution started successfully",
			zap.String("task_id", task.ID))
	}
}

// MockTaskRepository is a placeholder implementation
