package service

import (
	"context"

	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/events"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// HandleWorkspaceWorktreesObserved queues a linked-worktree snapshot for the
// task-service worker. Event-bus delivery must not run Git or agentctl work on
// the producer's goroutine.
func (s *Service) HandleWorkspaceWorktreesObserved(ctx context.Context, payload *events.WorktreeInventoryData) {
	if payload == nil {
		return
	}
	worker := s.worktreeObservationWorker()
	worker.mu.Lock()
	started := worker.started
	worker.mu.Unlock()
	if !started {
		s.processWorkspaceWorktreesObserved(ctx, payload)
		return
	}
	worker.enqueue(payload)
}

// processWorkspaceWorktreesObserved adopts new linked worktrees reported by
// agentctl and retires adopted roots whose directory is gone. It never starts,
// resumes, or prompts an agent.
func (s *Service) processWorkspaceWorktreesObserved(ctx context.Context, payload *events.WorktreeInventoryData) {
	if payload == nil || payload.TaskID == "" || payload.SessionID == "" {
		return
	}
	if !supportsWorktreeAdoption(payload.ExecutorType) {
		return
	}
	environment, err := s.observedAdoptionEnvironment(ctx, payload)
	if err != nil || environment == nil {
		return
	}
	if !s.observedSessionIsCurrent(ctx, payload, environment) {
		return
	}
	rows, err := s.taskEnvironments.ListTaskEnvironmentRepos(ctx, environment.ID)
	if err != nil {
		s.logger.Warn("worktree observer: list environment repositories",
			zap.String("task_id", payload.TaskID), zap.Error(err))
		return
	}
	entries := observedEntrySet(payload)
	s.adoptObservedEntries(ctx, payload, environment, rows, entries)
	s.retireMissingAdoptedRoots(ctx, payload, environment, rows, entries)
}

func (s *Service) observedAdoptionEnvironment(ctx context.Context, payload *events.WorktreeInventoryData) (*models.TaskEnvironment, error) {
	if payload.TaskEnvironmentID != "" {
		environment, err := s.taskEnvironments.GetTaskEnvironment(ctx, payload.TaskEnvironmentID)
		if err != nil || environment == nil {
			return nil, err
		}
		if environment.TaskID != payload.TaskID {
			return nil, fmt.Errorf("worktree observer: environment owner mismatch")
		}
		return environment, nil
	}
	environment, err := s.taskEnvironments.GetTaskEnvironmentByTaskID(ctx, payload.TaskID)
	if err != nil || environment == nil {
		return nil, err
	}
	return environment, nil
}

func (s *Service) observedSessionIsCurrent(ctx context.Context, payload *events.WorktreeInventoryData, environment *models.TaskEnvironment) bool {
	if s.sessions == nil {
		return true
	}
	sessions, err := s.sessions.ListTaskSessions(ctx, payload.TaskID)
	if err != nil {
		return false
	}
	var observed bool
	for _, session := range sessions {
		if session != nil && session.ID == payload.SessionID {
			observed = session.TaskEnvironmentID == environment.ID
			break
		}
	}
	if !observed {
		return false
	}
	latest := SelectBranchMaterializationSession(sessions)
	return latest == nil || latest.ID == payload.SessionID
}

func observedEntrySet(payload *events.WorktreeInventoryData) map[string]bool {
	entries := make(map[string]bool, len(payload.Entries))
	for _, entry := range payload.Entries {
		resolved, err := filepath.EvalSymlinks(filepath.Clean(entry.Path))
		if err != nil {
			continue
		}
		entries[resolved] = true
	}
	return entries
}

func (s *Service) adoptObservedEntries(ctx context.Context, payload *events.WorktreeInventoryData, environment *models.TaskEnvironment, rows []*models.TaskEnvironmentRepo, entries map[string]bool) {
	for path := range entries {
		if observedRootAlreadyAttached(rows, environment, path) {
			continue
		}
		detached, err := s.isWorktreeDetached(ctx, payload.TaskID, path)
		if err != nil {
			continue
		}
		if detached {
			recordWorktreeAdoption(WorktreeAdoptionSourceDetected, worktreeAdoptionResultSuppressed)
			continue
		}
		if _, err := s.AdoptWorktree(ctx, AdoptWorktreeRequest{
			TaskID: payload.TaskID, Path: path, Source: WorktreeAdoptionSourceDetected,
		}); err != nil && !errors.Is(err, errAdoptionSuppressed) {
			s.logger.Warn("worktree observer: adoption rejected",
				zap.String("task_id", payload.TaskID),
				zap.String("path", path),
				zap.Error(err))
		}
	}
}

func observedRootAlreadyAttached(rows []*models.TaskEnvironmentRepo, environment *models.TaskEnvironment, path string) bool {
	if environment != nil && environment.WorkspacePath != "" && sameResolvedPath(environment.WorkspacePath, path) {
		return true
	}
	for _, row := range rows {
		if !taskEnvironmentRepoActive(row) || row.WorktreePath == "" {
			continue
		}
		if sameResolvedPath(row.WorktreePath, path) {
			return true
		}
	}
	return false
}

func (s *Service) retireMissingAdoptedRoots(ctx context.Context, payload *events.WorktreeInventoryData, environment *models.TaskEnvironment, rows []*models.TaskEnvironmentRepo, entries map[string]bool) {
	if payload.CommonDir == "" {
		return
	}
	for _, row := range rows {
		if !taskEnvironmentRepoActive(row) || row.WorktreeOrigin != models.WorktreeOriginAdopted || row.WorktreePath == "" {
			continue
		}
		if row.WorktreeSourceCommonDir != "" && !sameResolvedPath(row.WorktreeSourceCommonDir, payload.CommonDir) {
			continue
		}
		resolved, err := filepath.EvalSymlinks(filepath.Clean(row.WorktreePath))
		if err == nil && entries[resolved] {
			continue
		}
		if _, statErr := os.Stat(row.WorktreePath); statErr == nil {
			// A temporarily prunable worktree that is still on disk is kept.
			continue
		}
		taskRepo := s.taskRepositoryForEnvironmentRow(ctx, payload.TaskID, row)
		if taskRepo == nil {
			continue
		}
		if err := s.DetachTaskRepositoryFromDisk(ctx, payload.TaskID, taskRepo.ID); err != nil {
			s.logger.Warn("worktree observer: retire missing root",
				zap.String("task_id", payload.TaskID),
				zap.String("path", row.WorktreePath),
				zap.Error(err))
		}
	}
}
