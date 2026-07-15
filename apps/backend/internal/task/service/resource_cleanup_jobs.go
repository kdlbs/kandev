package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/worktree"
)

const taskResourceCleanupRetryDelay = time.Minute

type persistedTaskStopTarget struct {
	SessionID   string `json:"session_id"`
	ExecutionID string `json:"execution_id,omitempty"`
	Terminal    bool   `json:"terminal,omitempty"`
}

type taskResourceCleanupSnapshot struct {
	Sessions              []*models.TaskSession     `json:"sessions,omitempty"`
	Worktrees             []*worktree.Worktree      `json:"worktrees,omitempty"`
	StopTargets           []persistedTaskStopTarget `json:"stop_targets,omitempty"`
	TaskEnvironment       *models.TaskEnvironment   `json:"task_environment,omitempty"`
	DeleteEnvironmentRow  bool                      `json:"delete_environment_row,omitempty"`
	LegacyWorktreeCleanup bool                      `json:"legacy_worktree_cleanup,omitempty"`
}

type taskResourceCleanupRun struct {
	job    *models.TaskResourceCleanupJob
	cancel context.CancelFunc
	done   chan struct{}
}

func newTaskResourceCleanupOperationID(trigger models.TaskResourceCleanupTrigger, taskID string) string {
	return string(trigger) + ":" + taskID + ":" + uuid.NewString()
}

func (s *Service) persistTaskResourceCleanup(
	ctx context.Context,
	taskID string,
	trigger models.TaskResourceCleanupTrigger,
	operationID string,
	sessions []*models.TaskSession,
	worktrees []*worktree.Worktree,
	stopTargets []taskStopTarget,
	envCleanup taskEnvironmentCleanup,
	prepared bool,
) (*models.TaskResourceCleanupJob, error) {
	if s.resourceCleanups == nil {
		return nil, nil
	}
	if operationID == "" {
		operationID = newTaskResourceCleanupOperationID(trigger, taskID)
	}
	snapshot := taskResourceCleanupSnapshot{
		Sessions: sessions, Worktrees: worktrees,
		StopTargets:           persistStopTargets(stopTargets),
		TaskEnvironment:       envCleanup.env,
		DeleteEnvironmentRow:  envCleanup.deleteRow,
		LegacyWorktreeCleanup: s.hasLegacyWorktreeCleanup(),
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("encode task resource cleanup snapshot: %w", err)
	}
	state := models.TaskResourceCleanupStatePending
	if prepared {
		state = models.TaskResourceCleanupStatePrepared
	}
	job := &models.TaskResourceCleanupJob{
		OperationID: operationID, TaskID: taskID, Trigger: trigger,
		State: state, ResourceSnapshot: string(encoded),
	}
	if err := s.resourceCleanups.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		return nil, fmt.Errorf("persist task resource cleanup intent: %w", err)
	}
	return s.resourceCleanups.GetTaskResourceCleanupJobByOperationID(ctx, operationID)
}

func persistStopTargets(targets []taskStopTarget) []persistedTaskStopTarget {
	result := make([]persistedTaskStopTarget, 0, len(targets))
	for _, target := range targets {
		result = append(result, persistedTaskStopTarget{
			SessionID: target.sessionID, ExecutionID: target.executionID, Terminal: target.terminal,
		})
	}
	return result
}

func restoreStopTargets(targets []persistedTaskStopTarget) []taskStopTarget {
	result := make([]taskStopTarget, 0, len(targets))
	for _, target := range targets {
		result = append(result, taskStopTarget{
			sessionID: target.SessionID, executionID: target.ExecutionID, terminal: target.Terminal,
		})
	}
	return result
}

func (s *Service) startTaskResourceCleanup(job *models.TaskResourceCleanupJob) {
	if job == nil {
		return
	}
	s.cleanupWorkerMu.Lock()
	wake := s.cleanupWorkerWake
	s.cleanupWorkerMu.Unlock()
	if wake != nil {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}

// StartTaskResourceCleanupWorker owns the install-wide durable task cleanup
// loop. StopTaskResourceCleanupWorker joins it during backend shutdown.
func (s *Service) StartTaskResourceCleanupWorker(ctx context.Context) error {
	if s.resourceCleanups == nil {
		return nil
	}
	s.cleanupWorkerMu.Lock()
	if s.cleanupWorkerCancel != nil {
		s.cleanupWorkerMu.Unlock()
		return nil
	}
	workerCtx, cancel := context.WithCancel(ctx)
	wake := make(chan struct{}, 1)
	s.cleanupWorkerCancel = cancel
	s.cleanupWorkerWake = wake
	s.cleanupWorkerWG.Add(1)
	s.cleanupWorkerMu.Unlock()
	if err := s.ResumeTaskResourceCleanupJobs(workerCtx); err != nil {
		s.cleanupWorkerWG.Done()
		s.StopTaskResourceCleanupWorker()
		return err
	}
	go s.runTaskResourceCleanupWorker(workerCtx, wake)
	return nil
}

func (s *Service) runTaskResourceCleanupWorker(ctx context.Context, wake <-chan struct{}) {
	defer s.cleanupWorkerWG.Done()
	ticker := time.NewTicker(taskResourceCleanupRetryDelay)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-wake:
		}
		if err := s.processDueTaskResourceCleanupJobs(ctx); err != nil && ctx.Err() == nil {
			s.logger.Warn("process due task resource cleanup jobs", zap.Error(err))
		}
	}
}

func (s *Service) StopTaskResourceCleanupWorker() {
	s.cleanupWorkerMu.Lock()
	cancel := s.cleanupWorkerCancel
	s.cleanupWorkerCancel = nil
	s.cleanupWorkerWake = nil
	s.cleanupWorkerMu.Unlock()
	if cancel != nil {
		cancel()
		s.cleanupWorkerWG.Wait()
	}
}

// ResumeTaskResourceCleanupJobs reconstructs interrupted task cleanup after a
// backend restart. It is independent of optional scheduled storage maintenance.
func (s *Service) ResumeTaskResourceCleanupJobs(ctx context.Context) error {
	if s.resourceCleanups == nil {
		return nil
	}
	if err := s.resourceCleanups.ResetRunningTaskResourceCleanupJobs(ctx); err != nil {
		return fmt.Errorf("reset interrupted task cleanup jobs: %w", err)
	}
	return s.processDueTaskResourceCleanupJobs(ctx)
}

func (s *Service) processDueTaskResourceCleanupJobs(ctx context.Context) error {
	jobs, err := s.resourceCleanups.ListDueTaskResourceCleanupJobs(ctx, time.Now().UTC(), 100)
	if err != nil {
		return fmt.Errorf("list due task cleanup jobs: %w", err)
	}
	for _, job := range jobs {
		if err := s.processTaskResourceCleanupJob(ctx, job.ID); err != nil {
			s.logger.Warn("resumed task resource cleanup job failed",
				zap.String("job_id", job.ID), zap.String("task_id", job.TaskID), zap.Error(err))
		}
	}
	return nil
}

func (s *Service) processTaskResourceCleanupJob(ctx context.Context, id string) error {
	candidate, err := s.resourceCleanups.GetTaskResourceCleanupJob(ctx, id)
	if err != nil {
		return err
	}
	runCtx, run := s.registerTaskResourceCleanupRun(ctx, candidate)
	defer s.finishTaskResourceCleanupRun(run)

	claimed, err := s.resourceCleanups.MarkTaskResourceCleanupJobRunning(ctx, id)
	if err != nil || !claimed {
		return err
	}
	job, err := s.resourceCleanups.GetTaskResourceCleanupJob(runCtx, id)
	if err != nil {
		return err
	}
	if s.cleanupActivity != nil {
		lease, acquireErr := s.cleanupActivity.AcquireTaskResourceCleanup(runCtx)
		if acquireErr != nil {
			return s.retryTaskResourceCleanupJob(runCtx, job, acquireErr)
		}
		defer lease.Release()
	}
	if cancelled, cancelErr := s.cancelIfTaskUnarchived(runCtx, job); cancelErr != nil || cancelled {
		if cancelErr != nil {
			return s.retryTaskResourceCleanupJob(runCtx, job, cancelErr)
		}
		return nil
	}
	var snapshot taskResourceCleanupSnapshot
	if err := json.Unmarshal([]byte(job.ResourceSnapshot), &snapshot); err != nil {
		return s.retryTaskResourceCleanupJob(runCtx, job, fmt.Errorf("decode resource snapshot: %w", err))
	}
	cleanupErr := s.executeTaskResourceCleanupJob(runCtx, job, snapshot)
	if cleanupErr != nil {
		return s.retryTaskResourceCleanupJob(runCtx, job, cleanupErr)
	}
	_, err = s.resourceCleanups.CompleteClaimedTaskResourceCleanupJob(
		runCtx, job.ID, job.Attempts, models.TaskResourceCleanupStateSucceeded, "", nil,
	)
	return err
}

func (s *Service) registerTaskResourceCleanupRun(
	ctx context.Context,
	job *models.TaskResourceCleanupJob,
) (context.Context, *taskResourceCleanupRun) {
	runCtx, cancel := context.WithCancel(ctx)
	run := &taskResourceCleanupRun{job: job, cancel: cancel, done: make(chan struct{})}
	s.cleanupRunsMu.Lock()
	if s.cleanupRuns == nil {
		s.cleanupRuns = make(map[*taskResourceCleanupRun]struct{})
	}
	s.cleanupRuns[run] = struct{}{}
	s.cleanupRunsMu.Unlock()
	return runCtx, run
}

func (s *Service) finishTaskResourceCleanupRun(run *taskResourceCleanupRun) {
	run.cancel()
	s.cleanupRunsMu.Lock()
	delete(s.cleanupRuns, run)
	close(run.done)
	s.cleanupRunsMu.Unlock()
}

func (s *Service) cancelAndJoinArchiveTaskResourceCleanupRuns(ctx context.Context, taskID string) error {
	s.cleanupRunsMu.Lock()
	runs := make([]*taskResourceCleanupRun, 0)
	for run := range s.cleanupRuns {
		if run.job != nil && run.job.TaskID == taskID && run.job.IsArchive() {
			run.cancel()
			runs = append(runs, run)
		}
	}
	s.cleanupRunsMu.Unlock()
	for _, run := range runs {
		select {
		case <-run.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (s *Service) executeTaskResourceCleanupJob(
	ctx context.Context,
	job *models.TaskResourceCleanupJob,
	snapshot taskResourceCleanupSnapshot,
) error {
	targets := restoreStopTargets(snapshot.StopTargets)
	failedStops := s.stopTaskRuntimeTargets(ctx, job.TaskID, targets, taskResourceCleanupStopReason(job.Trigger), "task cleanup runtime stop failed")
	if cancelled, err := s.cancelIfTaskUnarchived(ctx, job); err != nil || cancelled {
		return err
	}
	errs := s.performTaskCleanup(ctx, job.TaskID, snapshot.Sessions, snapshot.Worktrees, targets,
		taskEnvironmentCleanup{env: snapshot.TaskEnvironment, deleteRow: snapshot.DeleteEnvironmentRow}, failedStops)
	if cause := context.Cause(ctx); cause != nil {
		return errors.Join(append(errs, cause)...)
	}
	if snapshot.LegacyWorktreeCleanup && len(failedStops) == 0 && s.worktreeCleanup != nil {
		if err := s.worktreeCleanup.OnTaskDeleted(ctx, job.TaskID); err != nil {
			errs = append(errs, fmt.Errorf("legacy worktree cleanup: %w", err))
		}
	}
	if cause := context.Cause(ctx); cause != nil {
		return errors.Join(append(errs, cause)...)
	}
	s.signalCleanupDoneForTest()
	if len(errs) == 0 && len(failedStops) == 0 {
		return nil
	}
	if len(failedStops) > 0 {
		errs = append(errs, fmt.Errorf("%d runtime stop operations failed", len(failedStops)))
	}
	return errors.Join(errs...)
}

func (s *Service) hasLegacyWorktreeCleanup() bool {
	if s.worktreeCleanup == nil {
		return false
	}
	_, isProvider := s.worktreeCleanup.(WorktreeProvider)
	return !isProvider
}

func taskResourceCleanupStopReason(trigger models.TaskResourceCleanupTrigger) string {
	switch trigger {
	case models.TaskResourceCleanupTriggerArchive:
		return "task archived"
	case models.TaskResourceCleanupTriggerCascadeArchive:
		return "cascade archive"
	case models.TaskResourceCleanupTriggerCascadeDelete:
		return "cascade delete"
	default:
		return "task deleted"
	}
}

func (s *Service) cancelIfTaskUnarchived(ctx context.Context, job *models.TaskResourceCleanupJob) (bool, error) {
	if !job.IsArchive() {
		return false, nil
	}
	current, err := s.resourceCleanups.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		return false, err
	}
	if current.State == models.TaskResourceCleanupStateCancelled {
		return true, nil
	}
	task, err := s.tasks.GetTask(ctx, job.TaskID)
	if err != nil && !errors.Is(err, taskrepo.ErrTaskNotFound) {
		return false, err
	}
	if errors.Is(err, taskrepo.ErrTaskNotFound) || task == nil || task.ArchivedAt == nil {
		_, completeErr := s.resourceCleanups.CompleteClaimedTaskResourceCleanupJob(
			ctx, job.ID, current.Attempts, models.TaskResourceCleanupStateCancelled, "", nil,
		)
		return true, completeErr
	}
	return false, nil
}

func (s *Service) cancelTaskResourceCleanupJob(ctx context.Context, job *models.TaskResourceCleanupJob) {
	if job == nil || s.resourceCleanups == nil {
		return
	}
	_ = s.resourceCleanups.CompleteTaskResourceCleanupJob(
		ctx, job.ID, models.TaskResourceCleanupStateCancelled, "", nil,
	)
}

func (s *Service) retryTaskResourceCleanupJob(ctx context.Context, job *models.TaskResourceCleanupJob, cleanupErr error) error {
	nextAttempt := time.Now().UTC().Add(taskResourceCleanupRetryDelay)
	transitionCtx, cancel := detachedCleanupTransitionContext(ctx)
	defer cancel()
	_, err := s.resourceCleanups.CompleteClaimedTaskResourceCleanupJob(
		transitionCtx, job.ID, job.Attempts, models.TaskResourceCleanupStateRetryWait, cleanupErr.Error(), &nextAttempt,
	)
	if err != nil {
		return errors.Join(cleanupErr, err)
	}
	return cleanupErr
}

func detachedCleanupTransitionContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
}

// CancelArchiveTaskResourceCleanup cancels retryable archive cleanup before an
// unarchive mutation makes the task active again.
func (s *Service) CancelArchiveTaskResourceCleanup(ctx context.Context, taskID string) error {
	if s.resourceCleanups == nil {
		return nil
	}
	cancelErr := s.resourceCleanups.CancelArchiveTaskResourceCleanupJobs(ctx, taskID)
	joinErr := s.cancelAndJoinArchiveTaskResourceCleanupRuns(ctx, taskID)
	return errors.Join(cancelErr, joinErr)
}

// PrepareTaskResourceCleanup captures cleanup handles before a cascade mutates
// task rows. StartPreparedTaskResourceCleanup is called only after the matching
// lifecycle mutation commits.
func (s *Service) PrepareTaskResourceCleanup(
	ctx context.Context,
	taskID string,
	trigger models.TaskResourceCleanupTrigger,
	operationID string,
	deleteEnvironmentRow bool,
) error {
	sessions, err := s.sessions.ListTaskSessions(ctx, taskID)
	if err != nil {
		return fmt.Errorf("list task sessions for cleanup snapshot: %w", err)
	}
	stopTargets, err := s.buildStopTargets(ctx, taskID, sessions)
	if err != nil {
		return fmt.Errorf("list runtime cleanup inventory: %w", err)
	}
	worktrees, err := s.gatherWorktreesForDelete(ctx, taskID)
	if err != nil {
		return fmt.Errorf("list worktrees for cleanup snapshot: %w", err)
	}
	taskEnv, err := s.gatherTaskEnvironmentForCleanup(ctx, taskID)
	if err != nil {
		return fmt.Errorf("lookup task environment for cleanup snapshot: %w", err)
	}
	_, err = s.persistTaskResourceCleanup(ctx, taskID, trigger, operationID,
		sessions, worktrees, stopTargets,
		taskEnvironmentCleanup{env: taskEnv, deleteRow: deleteEnvironmentRow}, true)
	return err
}

func (s *Service) StartPreparedTaskResourceCleanup(ctx context.Context, operationID string) error {
	if s.resourceCleanups == nil {
		return nil
	}
	job, err := s.resourceCleanups.GetTaskResourceCleanupJobByOperationID(ctx, operationID)
	if err != nil {
		return err
	}
	started, err := s.resourceCleanups.StartPreparedTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		return err
	}
	if !started {
		return nil
	}
	job.State = models.TaskResourceCleanupStatePending
	s.startTaskResourceCleanup(job)
	return nil
}

func (s *Service) CancelPreparedTaskResourceCleanup(ctx context.Context, operationID string) error {
	if s.resourceCleanups == nil {
		return nil
	}
	job, err := s.resourceCleanups.GetTaskResourceCleanupJobByOperationID(ctx, operationID)
	if err != nil {
		return err
	}
	return s.resourceCleanups.CompleteTaskResourceCleanupJob(
		ctx, job.ID, models.TaskResourceCleanupStateCancelled, "", nil,
	)
}
