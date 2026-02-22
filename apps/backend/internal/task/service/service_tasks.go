package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

type taskStopTarget struct {
	sessionID   string
	executionID string
}

// Task operations

// CreateTask creates a new task and publishes a task.created event
func (s *Service) CreateTask(ctx context.Context, req *CreateTaskRequest) (*models.Task, error) {
	// Auto-resolve start step if not provided
	workflowStepID := req.WorkflowStepID
	if workflowStepID == "" && req.WorkflowID != "" && s.startStepResolver != nil {
		resolvedID, err := s.startStepResolver.ResolveStartStep(ctx, req.WorkflowID)
		if err != nil {
			s.logger.Warn("failed to resolve start step, using empty",
				zap.String("workflow_id", req.WorkflowID),
				zap.Error(err))
		} else {
			workflowStepID = resolvedID
		}
	}

	state := v1.TaskStateCreated
	if req.State != nil {
		state = *req.State
	}
	task := &models.Task{
		ID:             uuid.New().String(),
		WorkspaceID:    req.WorkspaceID,
		WorkflowID:     req.WorkflowID,
		WorkflowStepID: workflowStepID,
		Title:          req.Title,
		Description:    req.Description,
		State:          state,
		Priority:       req.Priority,
		Position:       req.Position,
		Metadata:       req.Metadata,
	}

	if err := s.tasks.CreateTask(ctx, task); err != nil {
		s.logger.Error("failed to create task", zap.Error(err))
		return nil, err
	}

	if err := s.createTaskRepositories(ctx, task.ID, req.WorkspaceID, req.Repositories); err != nil {
		return nil, err
	}

	// Load repositories into task for response
	repos, err := s.taskRepos.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		s.logger.Error("failed to list task repositories", zap.Error(err))
	} else {
		task.Repositories = repos
	}

	s.publishTaskEvent(ctx, events.TaskCreated, task, nil)
	s.logger.Info("task created", zap.String("task_id", task.ID), zap.String("title", task.Title))

	return task, nil
}

// createTaskRepositories creates task-repository associations, resolving local paths to repository IDs.
func (s *Service) createTaskRepositories(ctx context.Context, taskID, workspaceID string, repositories []TaskRepositoryInput) error {
	var repoByPath map[string]*models.Repository
	for _, repoInput := range repositories {
		if repoInput.RepositoryID == "" && repoInput.LocalPath != "" {
			repos, err := s.repoEntities.ListRepositories(ctx, workspaceID)
			if err != nil {
				s.logger.Error("failed to list repositories", zap.Error(err))
				return err
			}
			repoByPath = make(map[string]*models.Repository, len(repos))
			for _, repo := range repos {
				if repo.LocalPath == "" {
					continue
				}
				repoByPath[repo.LocalPath] = repo
			}
			break
		}
	}

	for i, repoInput := range repositories {
		repositoryID, baseBranch, err := s.resolveRepoInput(ctx, workspaceID, repoInput, repoByPath)
		if err != nil {
			return err
		}
		if repositoryID == "" {
			return fmt.Errorf("repository_id is required")
		}
		taskRepo := &models.TaskRepository{
			TaskID:       taskID,
			RepositoryID: repositoryID,
			BaseBranch:   baseBranch,
			Position:     i,
			Metadata:     make(map[string]interface{}),
		}
		if err := s.taskRepos.CreateTaskRepository(ctx, taskRepo); err != nil {
			s.logger.Error("failed to create task repository", zap.Error(err))
			return err
		}
	}
	return nil
}

// resolveRepoInput resolves a RepositoryInput to a repositoryID and baseBranch,
// creating the repository if it doesn't exist yet.
func (s *Service) resolveRepoInput(ctx context.Context, workspaceID string, repoInput TaskRepositoryInput, repoByPath map[string]*models.Repository) (repositoryID, baseBranch string, err error) {
	repositoryID = repoInput.RepositoryID
	baseBranch = repoInput.BaseBranch
	if repositoryID != "" || repoInput.LocalPath == "" {
		return repositoryID, baseBranch, nil
	}
	repo := repoByPath[repoInput.LocalPath]
	if repo == nil {
		name := strings.TrimSpace(repoInput.Name)
		if name == "" {
			name = filepath.Base(repoInput.LocalPath)
		}
		defaultBranch := repoInput.DefaultBranch
		if defaultBranch == "" {
			defaultBranch = repoInput.BaseBranch
		}
		created, createErr := s.CreateRepository(ctx, &CreateRepositoryRequest{
			WorkspaceID:   workspaceID,
			Name:          name,
			SourceType:    "local",
			LocalPath:     repoInput.LocalPath,
			DefaultBranch: defaultBranch,
		})
		if createErr != nil {
			return "", "", createErr
		}
		repo = created
		if repoByPath != nil {
			repoByPath[repoInput.LocalPath] = repo
		}
	}
	repositoryID = repo.ID
	if baseBranch == "" {
		baseBranch = repo.DefaultBranch
	}
	return repositoryID, baseBranch, nil
}

// replaceTaskRepositories deletes all existing task-repository associations and recreates them.
func (s *Service) replaceTaskRepositories(ctx context.Context, taskID, workspaceID string, repositories []TaskRepositoryInput) error {
	if err := s.taskRepos.DeleteTaskRepositoriesByTask(ctx, taskID); err != nil {
		s.logger.Error("failed to delete task repositories", zap.Error(err))
		return err
	}
	return s.createTaskRepositories(ctx, taskID, workspaceID, repositories)
}

// GetTask retrieves a task by ID and populates repositories
func (s *Service) GetTask(ctx context.Context, id string) (*models.Task, error) {
	task, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}

	// Load task repositories
	repos, err := s.taskRepos.ListTaskRepositories(ctx, id)
	if err != nil {
		s.logger.Error("failed to list task repositories", zap.Error(err))
	} else {
		task.Repositories = repos
	}

	return task, nil
}

// UpdateTask updates an existing task and publishes a task.updated event
func (s *Service) UpdateTask(ctx context.Context, id string, req *UpdateTaskRequest) (*models.Task, error) {
	task, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}
	var oldState *v1.TaskState
	stateChanged := false

	if req.Title != nil {
		task.Title = *req.Title
	}
	if req.Description != nil {
		task.Description = *req.Description
	}
	if req.Priority != nil {
		task.Priority = *req.Priority
	}
	if req.State != nil && task.State != *req.State {
		current := task.State
		oldState = &current
		task.State = *req.State
		stateChanged = true
	}
	if req.Position != nil {
		task.Position = *req.Position
	}
	if req.Metadata != nil {
		task.Metadata = req.Metadata
	}
	task.UpdatedAt = time.Now().UTC()

	if err := s.tasks.UpdateTask(ctx, task); err != nil {
		s.logger.Error("failed to update task", zap.String("task_id", id), zap.Error(err))
		return nil, err
	}

	// Update task repositories if provided
	if req.Repositories != nil {
		if err := s.replaceTaskRepositories(ctx, task.ID, task.WorkspaceID, req.Repositories); err != nil {
			return nil, err
		}
	}

	// Load repositories into task for response
	repos, err := s.taskRepos.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		s.logger.Error("failed to list task repositories", zap.Error(err))
	} else {
		task.Repositories = repos
	}

	if stateChanged && oldState != nil {
		s.publishTaskEvent(ctx, events.TaskStateChanged, task, oldState)
	}
	s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
	s.logger.Info("task updated", zap.String("task_id", task.ID))

	return task, nil
}

// ArchiveTask archives a task by setting its archived_at timestamp.
// The task remains in the DB but is excluded from active board views.
// Active agent sessions are stopped and worktrees cleaned up in background.
func (s *Service) ArchiveTask(ctx context.Context, id string) error {
	start := time.Now()

	// 1. Get task and verify it exists
	task, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		return err
	}

	if task.ArchivedAt != nil {
		return fmt.Errorf("task is already archived: %s", id)
	}

	// 2. Gather data needed for cleanup BEFORE archive
	var stopTargets []taskStopTarget
	if s.executionStopper != nil {
		activeSessions, err := s.sessions.ListActiveTaskSessionsByTaskID(ctx, id)
		if err != nil {
			s.logger.Warn("failed to list active sessions for archive",
				zap.String("task_id", id),
				zap.Error(err))
		}
		stopTargets = s.buildStopTargets(ctx, id, activeSessions)
	}

	sessions, err := s.sessions.ListTaskSessions(ctx, id)
	if err != nil {
		s.logger.Warn("failed to list task sessions for archive",
			zap.String("task_id", id),
			zap.Error(err))
	}

	var worktrees []*worktree.Worktree
	if s.worktreeCleanup != nil {
		if provider, ok := s.worktreeCleanup.(WorktreeProvider); ok {
			worktrees, err = provider.GetAllByTaskID(ctx, id)
			if err != nil {
				s.logger.Warn("failed to list worktrees for archive",
					zap.String("task_id", id),
					zap.Error(err))
			}
		}
	}

	// 3. Set archived_at in DB
	if err := s.tasks.ArchiveTask(ctx, id); err != nil {
		return err
	}

	// 4. Re-read task for updated archived_at field
	task, err = s.tasks.GetTask(ctx, id)
	if err != nil {
		return err
	}

	// 5. Publish task.updated event so frontend removes from board
	s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
	s.logger.Info("task archived",
		zap.String("task_id", id),
		zap.Duration("duration", time.Since(start)))

	// 6. Background: Stop agents and cleanup worktrees
	if len(stopTargets) > 0 || s.worktreeCleanup != nil || len(sessions) > 0 {
		s.runAsyncTaskCleanup(id, sessions, worktrees, stopTargets,
			"task archived", "failed to stop session on task archive", "task archive cleanup completed")
	}

	return nil
}

// DeleteTask deletes a task and publishes a task.deleted event.
// For fast UI response, the DB delete and event publish happen synchronously,
// while agent stopping and worktree cleanup happen asynchronously.
func (s *Service) DeleteTask(ctx context.Context, id string) error {
	start := time.Now()

	// 1. Get task (sync, fast)
	task, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		return err
	}

	// 2. Gather data needed for cleanup BEFORE delete (sync, fast)
	sessions, err := s.sessions.ListTaskSessions(ctx, id)
	if err != nil {
		s.logger.Warn("failed to list task sessions for delete",
			zap.String("task_id", id),
			zap.Error(err))
	}

	worktrees := s.gatherWorktreesForDelete(ctx, id)

	// 3. Get active sessions for stopping agents (sync, fast)
	// Must query before delete since DB records will be gone
	var stopTargets []taskStopTarget
	if s.executionStopper != nil {
		activeSessions, err := s.sessions.ListActiveTaskSessionsByTaskID(ctx, id)
		if err != nil {
			s.logger.Warn("failed to list active sessions for delete",
				zap.String("task_id", id),
				zap.Error(err))
		}
		stopTargets = s.buildStopTargets(ctx, id, activeSessions)
	}

	// 4. Delete from DB (sync, fast)
	if err := s.tasks.DeleteTask(ctx, id); err != nil {
		s.logger.Error("failed to delete task", zap.String("task_id", id), zap.Error(err))
		return err
	}

	// 5. Publish event (sync, fast) - frontend removes task immediately
	s.publishTaskEvent(ctx, events.TaskDeleted, task, nil)
	s.logger.Info("task deleted",
		zap.String("task_id", id),
		zap.Duration("duration", time.Since(start)))

	// 6. Return immediately - all remaining cleanup is async

	// 7. Background: Stop agents and cleanup worktrees
	if len(stopTargets) > 0 || s.worktreeCleanup != nil || len(sessions) > 0 {
		s.runAsyncTaskCleanup(id, sessions, worktrees, stopTargets,
			"task deleted", "failed to stop session on task delete", "task cleanup completed")
	}

	return nil
}

// gatherWorktreesForDelete collects worktrees for a task before it is deleted.
// For legacy WorktreeCleanup implementations that do not implement WorktreeProvider,
// it triggers cleanup immediately and returns nil.
func (s *Service) gatherWorktreesForDelete(ctx context.Context, taskID string) []*worktree.Worktree {
	if s.worktreeCleanup == nil {
		return nil
	}
	provider, ok := s.worktreeCleanup.(WorktreeProvider)
	if !ok {
		// Fallback for legacy implementations: cleanup before delete.
		if err := s.worktreeCleanup.OnTaskDeleted(ctx, taskID); err != nil {
			s.logger.Warn("failed to cleanup worktree on task deletion",
				zap.String("task_id", taskID),
				zap.Error(err))
		}
		return nil
	}
	worktrees, err := provider.GetAllByTaskID(ctx, taskID)
	if err != nil {
		s.logger.Warn("failed to list worktrees for delete",
			zap.String("task_id", taskID),
			zap.Error(err))
	}
	return worktrees
}

func (s *Service) runAsyncTaskCleanup(
	id string,
	sessions []*models.TaskSession,
	worktrees []*worktree.Worktree,
	stopTargets []taskStopTarget,
	stopReason, stopFailMsg, cleanupMsg string,
) {
	go func() {
		cleanupStart := time.Now()
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		if s.executionStopper != nil && len(stopTargets) > 0 {
			for _, target := range stopTargets {
				if target.executionID != "" {
					if err := s.executionStopper.StopExecution(cleanupCtx, target.executionID, stopReason, true); err != nil {
						s.logger.Warn(stopFailMsg,
							zap.String("task_id", id),
							zap.String("session_id", target.sessionID),
							zap.String("execution_id", target.executionID),
							zap.Error(err))
					}
					continue
				}
				if err := s.executionStopper.StopSession(cleanupCtx, target.sessionID, stopReason, true); err != nil {
					s.logger.Warn(stopFailMsg,
						zap.String("task_id", id),
						zap.String("session_id", target.sessionID),
						zap.Error(err))
				}
			}
		}

		cleanupErrors := s.performTaskCleanup(cleanupCtx, id, sessions, worktrees)

		if len(cleanupErrors) > 0 {
			s.logger.Warn(cleanupMsg+" with errors",
				zap.String("task_id", id),
				zap.Int("error_count", len(cleanupErrors)),
				zap.Duration("duration", time.Since(cleanupStart)))
		} else {
			s.logger.Info(cleanupMsg,
				zap.String("task_id", id),
				zap.Duration("duration", time.Since(cleanupStart)))
		}
	}()
}

func (s *Service) buildStopTargets(ctx context.Context, taskID string, activeSessions []*models.TaskSession) []taskStopTarget {
	targets := make([]taskStopTarget, 0, len(activeSessions))
	for _, sess := range activeSessions {
		if sess == nil || sess.ID == "" {
			continue
		}
		target := taskStopTarget{
			sessionID:   sess.ID,
			executionID: strings.TrimSpace(sess.AgentExecutionID),
		}
		if target.executionID == "" {
			running, err := s.executors.GetExecutorRunningBySessionID(ctx, sess.ID)
			if err == nil && running != nil {
				target.executionID = strings.TrimSpace(running.AgentExecutionID)
			}
		}
		targets = append(targets, target)
	}
	s.logger.Debug("prepared task cleanup stop targets",
		zap.String("task_id", taskID),
		zap.Int("count", len(targets)))
	return targets
}

// performTaskCleanup handles post-deletion cleanup operations.
// Handles worktree cleanup and executor_running records.
// Agent stopping is handled separately in the DeleteTask background goroutine.
// Returns a slice of errors encountered (empty if all succeeded).
func (s *Service) performTaskCleanup(
	ctx context.Context,
	taskID string,
	sessions []*models.TaskSession,
	worktrees []*worktree.Worktree,
) []error {
	var errs []error

	// Cleanup worktrees
	if len(worktrees) > 0 {
		if cleaner, ok := s.worktreeCleanup.(WorktreeBatchCleaner); ok {
			if err := cleaner.CleanupWorktrees(ctx, worktrees); err != nil {
				s.logger.Warn("failed to cleanup worktrees after delete",
					zap.String("task_id", taskID),
					zap.Error(err))
				errs = append(errs, fmt.Errorf("cleanup worktrees: %w", err))
			}
		}
	}

	// Delete executor running records for sessions
	for _, session := range sessions {
		if session == nil || session.ID == "" {
			continue
		}
		if err := s.executors.DeleteExecutorRunningBySessionID(ctx, session.ID); err != nil {
			s.logger.Debug("failed to delete executor runtime for session",
				zap.String("task_id", taskID),
				zap.String("session_id", session.ID),
				zap.Error(err))
			// Don't add to errs - this is a debug-level issue
		}
	}

	return errs
}

// ListTasks returns all tasks for a workflow
func (s *Service) ListTasks(ctx context.Context, workflowID string) ([]*models.Task, error) {
	tasks, err := s.tasks.ListTasks(ctx, workflowID)
	if err != nil {
		return nil, err
	}

	if err := s.loadTaskRepositoriesBatch(ctx, tasks); err != nil {
		s.logger.Error("failed to batch-load task repositories", zap.Error(err))
	}

	return tasks, nil
}

// ListTasksByWorkspace returns paginated tasks for a workspace with task repositories loaded.
// If query is non-empty, filters by task title, description, repository name, or repository path.
func (s *Service) ListTasksByWorkspace(ctx context.Context, workspaceID string, query string, page, pageSize int, includeArchived bool) ([]*models.Task, int, error) {
	tasks, total, err := s.tasks.ListTasksByWorkspace(ctx, workspaceID, query, page, pageSize, includeArchived)
	if err != nil {
		return nil, 0, err
	}

	if err := s.loadTaskRepositoriesBatch(ctx, tasks); err != nil {
		s.logger.Error("failed to batch-load task repositories", zap.Error(err))
	}

	return tasks, total, nil
}

// loadTaskRepositoriesBatch loads repositories for multiple tasks in a single query.
func (s *Service) loadTaskRepositoriesBatch(ctx context.Context, tasks []*models.Task) error {
	if len(tasks) == 0 {
		return nil
	}
	taskIDs := make([]string, len(tasks))
	for i, t := range tasks {
		taskIDs[i] = t.ID
	}
	repoMap, err := s.taskRepos.ListTaskRepositoriesByTaskIDs(ctx, taskIDs)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		task.Repositories = repoMap[task.ID]
	}
	return nil
}
