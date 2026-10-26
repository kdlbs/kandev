package github

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type prWatchLifecycleLock struct {
	mu   sync.Mutex
	refs int
}

func (s *Service) lockPRWatchLifecycle(watchID string) func() {
	s.prWatchLifecycleLocksMu.Lock()
	if s.prWatchLifecycleLocks == nil {
		s.prWatchLifecycleLocks = make(map[string]*prWatchLifecycleLock)
	}
	lock := s.prWatchLifecycleLocks[watchID]
	if lock == nil {
		lock = &prWatchLifecycleLock{}
		s.prWatchLifecycleLocks[watchID] = lock
	}
	lock.refs++
	s.prWatchLifecycleLocksMu.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		s.prWatchLifecycleLocksMu.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(s.prWatchLifecycleLocks, watchID)
		}
		s.prWatchLifecycleLocksMu.Unlock()
	}
}

// ListPRWatchesForWorkspace returns an authorized watch inventory. Retired
// lifetimes are included only when explicitly requested.
func (s *Service) ListPRWatchesForWorkspace(
	ctx context.Context, workspaceID string, includeRetired bool,
) ([]*PRWatch, error) {
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return nil, err
	}
	return s.store.ListPRWatchesForWorkspace(ctx, workspaceID, includeRetired)
}

// DeletePRWatch retires a PR watch by ID while preserving its task history.
func (s *Service) DeletePRWatch(ctx context.Context, id string) error {
	unlock := s.lockPRWatchLifecycle(id)
	defer unlock()
	watch, err := s.store.GetPRWatch(ctx, id)
	if err != nil {
		return err
	}
	if watch == nil {
		return ErrPRWatchNotFound
	}
	if err := s.authorizeWorkspaceAccess(ctx, watch.WorkspaceID); err != nil {
		return err
	}
	retired, err := s.store.RetirePRWatch(ctx, id, time.Now().UTC())
	if err != nil {
		return err
	}
	if retired == nil {
		return ErrPRWatchNotFound
	}
	s.removePRDiscoveryWatchConsumer(retired)
	return nil
}

// ResumePRWatch starts a new active lifetime for an authorized retired target.
// Target identity always comes from the stored row.
func (s *Service) ResumePRWatch(ctx context.Context, id, workspaceID string) (*PRWatch, error) {
	unlock := s.lockPRWatchLifecycle(id)
	defer unlock()
	watch, err := s.store.GetPRWatch(ctx, id)
	if err != nil {
		return nil, err
	}
	if watch == nil || watch.WorkspaceID != workspaceID {
		return nil, ErrPRWatchNotFound
	}
	if err := s.authorizeWorkspaceAccess(ctx, watch.WorkspaceID); err != nil {
		return nil, err
	}
	if err := s.validatePRWatchResumeTarget(ctx, watch); err != nil {
		return nil, err
	}
	resumed, err := s.store.ResumePRWatch(ctx, id, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	if resumed == nil {
		return nil, ErrPRWatchNotFound
	}
	if resumed.ID != id {
		s.evictRepoNegative(watch.Owner, watch.Repo)
		s.resetPassiveWorkspaceRefreshAdmission(resumed.WorkspaceID)
	}
	return resumed, nil
}

func (s *Service) validatePRWatchResumeTarget(ctx context.Context, watch *PRWatch) error {
	if watch == nil || watch.TaskID == "" || watch.Owner == "" || watch.Repo == "" {
		return ErrPRWatchTargetIneligible
	}
	taskStore := s.getTaskIssueStore()
	if taskStore == nil {
		return ErrPRWatchTargetIneligible
	}
	task, err := taskStore.GetTask(ctx, watch.TaskID)
	if errors.Is(err, ErrTaskNotFound) || (err == nil && task == nil) {
		return ErrPRWatchTargetIneligible
	}
	if err != nil {
		return fmt.Errorf("load PR watch task: %w", err)
	}
	if task.WorkspaceID != watch.WorkspaceID {
		return ErrPRWatchTargetIneligible
	}
	if task.ArchivedAt != nil {
		return ErrPRWatchTargetIneligible
	}
	if err := s.validateTaskRepositoryID(ctx, watch.TaskID, watch.RepositoryID); err != nil {
		if errors.Is(err, ErrTaskPRRepositoryMismatch) {
			return ErrPRWatchTargetIneligible
		}
		return err
	}
	return nil
}

// CheckPRWatchTargetAdmissionForWorkspace checks the canonical target before
// automatic discovery spends a provider request.
func (s *Service) CheckPRWatchTargetAdmissionForWorkspace(
	ctx context.Context, workspaceID, sessionID, taskID, repositoryID, branch string,
) error {
	if strings.TrimSpace(workspaceID) == "" {
		return ErrGitHubWorkspaceRequired
	}
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return err
	}
	taskID = s.resolveEffectiveAssociationTaskIDForSession(ctx, sessionID, taskID, repositoryID)
	retired, err := s.store.HasRetiredPRWatchByTaskRepoBranch(ctx, taskID, repositoryID, branch)
	if err != nil {
		return err
	}
	if retired {
		return ErrPRWatchRetired
	}
	return nil
}
