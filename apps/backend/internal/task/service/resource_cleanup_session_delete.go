package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

// sessionWorktreeInventoryProvider captures a session's worktree inventory
// before session.delete's cascade removes its task_session_worktrees rows.
// Satisfied by *worktree.Manager.
type sessionWorktreeInventoryProvider interface {
	GetAllBySessionID(ctx context.Context, sessionID string) ([]*worktree.Worktree, error)
}

// sessionWorktreeReclaimer reclaims one worktree if the deleted session was
// its last live holder, leaving it untouched otherwise. Satisfied by
// *worktree.Manager.
type sessionWorktreeReclaimer interface {
	ReclaimSessionWorktree(ctx context.Context, wt *worktree.Worktree) error
}

// sessionDeleteCleanupSnapshot is the resource_snapshot shape for the
// session_delete trigger — session-scoped, unlike taskResourceCleanupSnapshot.
type sessionDeleteCleanupSnapshot struct {
	SessionID string               `json:"session_id"`
	Worktrees []*worktree.Worktree `json:"worktrees,omitempty"`
}

// PrepareSessionDeleteResourceCleanup captures a session's worktree inventory
// and durably persists a cleanup intent before the session row is deleted.
// task_session_worktrees.session_id cascades ON DELETE, so the session row is
// the only pointer to its worktrees; once it is gone an un-recorded worktree
// is unrecoverable (docs/specs/session-delete-resource-cleanup).
//
// Returns an empty operationID and a nil error when there is nothing to
// reclaim (no worktree manager wired, or the session holds no worktrees) —
// the caller then skips activation entirely. A non-nil error means the
// caller must fail the delete closed and leave the session row in place.
func (s *Service) PrepareSessionDeleteResourceCleanup(
	ctx context.Context, sessionID, taskID string,
) (string, error) {
	if s.worktreeCleanup == nil {
		return "", nil
	}
	provider, ok := s.worktreeCleanup.(sessionWorktreeInventoryProvider)
	if !ok {
		return "", nil
	}
	worktrees, err := provider.GetAllBySessionID(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("list worktrees for session delete cleanup snapshot: %w", err)
	}
	if len(worktrees) == 0 {
		return "", nil
	}
	if s.resourceCleanups == nil {
		return "", errors.New("resource cleanup store is unavailable")
	}
	operationID := newTaskResourceCleanupOperationID(models.TaskResourceCleanupTriggerSessionDelete, taskID)
	encoded, err := json.Marshal(sessionDeleteCleanupSnapshot{SessionID: sessionID, Worktrees: worktrees})
	if err != nil {
		return "", fmt.Errorf("encode session delete cleanup snapshot: %w", err)
	}
	job := &models.TaskResourceCleanupJob{
		OperationID:      operationID,
		TaskID:           taskID,
		Trigger:          models.TaskResourceCleanupTriggerSessionDelete,
		State:            models.TaskResourceCleanupStatePrepared,
		ResourceSnapshot: string(encoded),
	}
	if err := s.resourceCleanups.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		return "", fmt.Errorf("persist session delete cleanup intent: %w", err)
	}
	return operationID, nil
}

func decodeSessionDeleteCleanupSnapshot(raw string) (sessionDeleteCleanupSnapshot, error) {
	var snapshot sessionDeleteCleanupSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return sessionDeleteCleanupSnapshot{}, fmt.Errorf("decode session delete cleanup snapshot: %w", err)
	}
	return snapshot, nil
}

// sessionDeleteCleanupMutationCommitted reports whether the session row named
// by a prepared job's snapshot has actually been removed. Used to reconcile a
// job stuck in `prepared` state after a crash: if the session row still
// exists the delete never committed, and the job must be cancelled rather
// than reclaiming worktrees a live session still owns.
func (s *Service) sessionDeleteCleanupMutationCommitted(
	ctx context.Context, job *models.TaskResourceCleanupJob,
) (bool, error) {
	snapshot, err := decodeSessionDeleteCleanupSnapshot(job.ResourceSnapshot)
	if err != nil {
		return false, err
	}
	_, sessErr := s.sessions.GetTaskSession(ctx, snapshot.SessionID)
	if sessErr == nil {
		return false, nil
	}
	if errors.Is(sessErr, models.ErrTaskSessionNotFound) {
		return true, nil
	}
	return false, sessErr
}

// executeSessionDeleteResourceCleanup reclaims every worktree the deleted
// session's snapshot named. By the time this runs — always after the session
// row (and its task_session_worktrees rows) has been removed, whether via the
// synchronous activation path or restart reconciliation — a worktree's
// reference count already excludes the deleted session, so no explicit
// exclusion is needed. A worktree still referenced by another session is left
// untouched; a worktree that fails to reclaim is retried by the owning job's
// normal backoff, and its path survives in the join error so an eventually
// exhausted job records it rather than forgetting it silently.
func (s *Service) executeSessionDeleteResourceCleanup(ctx context.Context, job *models.TaskResourceCleanupJob) error {
	snapshot, err := decodeSessionDeleteCleanupSnapshot(job.ResourceSnapshot)
	if err != nil {
		return err
	}
	if len(snapshot.Worktrees) == 0 {
		return nil
	}
	reclaimer, ok := s.worktreeCleanup.(sessionWorktreeReclaimer)
	if !ok {
		return errors.New("worktree cleanup does not support session-scoped reclamation")
	}
	var errs []error
	for _, wt := range snapshot.Worktrees {
		if wt == nil {
			continue
		}
		if err := reclaimer.ReclaimSessionWorktree(ctx, wt); err != nil {
			errs = append(errs, fmt.Errorf("worktree %s: %w", wt.Path, err))
		}
	}
	return errors.Join(errs...)
}
