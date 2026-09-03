package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// ErrTaskReferenceNotFound marks a create rejected because a caller-supplied
// reference (a repository, a blocker) does not resolve.
//
// It exists so transports can classify the failure as VALIDATION rather than
// INTERNAL_ERROR. A bad id in the request is the caller's to fix, and telling
// them only "Failed to create task" — as this path used to — leaves the actual
// cause visible nowhere but the backend log, which most callers cannot read.
var ErrTaskReferenceNotFound = errors.New("task reference not found")

// unknownRepositoryReferenceError describes a repositories[i].repository_id
// that does not resolve, naming both the field and the offending value.
//
// The hint is not decoration: a task's repositories[] entry exposes BOTH an
// `id` (the task_repositories join row) and a `repository_id` (the repository
// itself), and passing the former is the mistake this error most often
// reports.
func unknownRepositoryReferenceError(index int, repositoryID string) error {
	return fmt.Errorf(
		"%w: unknown repositories[%d].repository_id %q; no repository with that id exists in this workspace. "+
			"If you copied it from a task's repositories[] entry, pass repository_id (the repository) rather than id (the task-repository link)",
		ErrTaskReferenceNotFound, index, repositoryID,
	)
}

// unknownBlockerReferenceError describes a blocked_by entry that does not
// resolve to a task in this workspace.
func unknownBlockerReferenceError(index int, blockerID string) error {
	return fmt.Errorf(
		"%w: unknown blocked_by[%d] %q; no task with that id exists in this workspace",
		ErrTaskReferenceNotFound, index, blockerID,
	)
}

// validateBlockerReferences checks every blocked_by id BEFORE the task row is
// inserted.
//
// AddBlocker remains the authoritative validator for dependency edges (see
// AddDependency's single-validator contract); this is not a second one. It
// resolves only what can be known without the new task existing — existence,
// authorization, and workspace agreement — so the request is rejected while
// there is still nothing to roll back. Cycle detection is deliberately absent:
// a task that does not exist yet has no outgoing edges and cannot close a
// cycle, and self-reference is impossible against an id not yet assigned.
func (s *Service) validateBlockerReferences(ctx context.Context, req *CreateTaskRequest) error {
	if len(req.BlockedBy) == 0 {
		return nil
	}
	if s.blockers == nil {
		return ErrDependencyRepositoryUnavailable
	}
	for index, blockerID := range req.BlockedBy {
		if blockerID == "" {
			return fmt.Errorf("%w: blocked_by[%d] is empty", ErrTaskReferenceNotFound, index)
		}
		if err := s.authorizeTaskID(ctx, blockerID); err != nil {
			// Denials surface as not-found so a caller cannot probe for the
			// existence of another user's task, matching authorizeDependencyPair.
			if errors.Is(err, taskrepo.ErrTaskNotFound) {
				return unknownBlockerReferenceError(index, blockerID)
			}
			return err
		}
		blocker, err := s.tasks.GetTask(ctx, blockerID)
		if err != nil {
			if errors.Is(err, taskrepo.ErrTaskNotFound) {
				return unknownBlockerReferenceError(index, blockerID)
			}
			return fmt.Errorf("resolve blocked_by[%d]: %w", index, err)
		}
		if blocker == nil {
			return unknownBlockerReferenceError(index, blockerID)
		}
		if req.WorkspaceID != "" && blocker.WorkspaceID != "" && blocker.WorkspaceID != req.WorkspaceID {
			return fmt.Errorf(
				"%w: blocked_by[%d] %q belongs to a different workspace",
				ErrTaskReferenceNotFound, index, blockerID,
			)
		}
	}
	return nil
}

// rollbackPartialTask deletes a task whose post-insert writes failed, and
// returns the original error to surface.
//
// Pre-insert validation closes the ordinary case; this closes the TOCTOU
// remainder, where a reference was valid when checked and gone by the time it
// was written. It mirrors the rollback the MCP create handler already performs
// for remote-contribution association and workspace-policy attach. A failed
// rollback is logged, never substituted for the original error: the caller
// needs to know why the create failed, not why the cleanup did.
// Cleanup is detached from caller cancellation but remains bounded, because a
// canceled request can be the reason finalization failed in the first place.
//
// Dependency edges are removed first. blocked_by is written one edge at a
// time, so a failure on the second entry leaves the first already persisted,
// and task_blockers predates the tasks foreign key — nothing cascades. Rolling
// back only the task row would trade an orphan task for an orphan edge
// pointing at a task that no longer exists.
func (s *Service) rollbackPartialTask(ctx context.Context, taskID string, cause error) error {
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	s.deleteDependencyEdgesForTask(rollbackCtx, taskID)
	if err := s.tasks.DeleteTask(rollbackCtx, taskID); err != nil {
		s.logger.Error("rollback delete failed; task left in inconsistent state",
			zap.String("task_id", taskID), zap.Error(err))
	}
	return cause
}

// classifyRepositoryResolutionError converts a repository lookup failure into
// the caller-facing reference error, leaving anything else untouched.
func classifyRepositoryResolutionError(index int, repositoryID string, err error) error {
	if errors.Is(err, repoerrors.ErrRepositoryNotFound) {
		return unknownRepositoryReferenceError(index, repositoryID)
	}
	return err
}
