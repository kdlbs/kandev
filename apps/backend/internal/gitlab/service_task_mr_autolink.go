package gitlab

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// AutoLinkMRForBranch is the push-detection counterpart to
// AssociateExistingMRByURL: instead of a user-supplied MR URL, it searches
// the branch for an open MR and links whatever it finds. Returns (nil, nil)
// when no open MR exists on the branch yet — that is the normal "pushed but
// no MR opened" case, not an error.
//
// Order matters: ResolveTaskMRRepository and ValidateTaskMRRepositoryIdentity
// run before any network call, so a task whose repository doesn't match the
// requested project fails closed without leaking an API call to a merge
// request the caller was never entitled to link.
//
// Idempotent: UpsertTaskMR and EnsureMRWatch are both get-or-create/update
// keyed on their respective unique constraints, so re-running this for an
// already-linked (task, repository, project, iid) updates the existing rows
// in place rather than duplicating them.
func (s *Service) AutoLinkMRForBranch(
	ctx context.Context,
	workspaceID, sessionID, taskID, repositoryID, projectPath, branch string,
) (*TaskMR, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	repositoryID, err := store.ResolveTaskMRRepository(ctx, workspaceID, taskID, repositoryID)
	if err != nil {
		return nil, err
	}
	client, err := s.ClientForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if err := store.ValidateTaskMRRepositoryIdentity(
		ctx, workspaceID, taskID, repositoryID, client.Host(), projectPath,
	); err != nil {
		return nil, err
	}
	mr, err := client.FindMRByBranch(ctx, projectPath, branch)
	if err != nil {
		return nil, fmt.Errorf("find merge request by branch: %w", err)
	}
	if mr == nil {
		return nil, nil
	}
	status, err := client.GetMRStatus(ctx, projectPath, mr.IID)
	if err != nil {
		return nil, fmt.Errorf("fetch merge request: %w", err)
	}
	if err := validateReturnedMRIdentity(status, client.Host(), projectPath, mr.IID); err != nil {
		return nil, ErrTaskMRNotFound
	}
	association := taskMRFromStatus(taskID, repositoryID, client.Host(), projectPath, status)
	if err := store.UpsertTaskMR(ctx, association); err != nil {
		return nil, fmt.Errorf("upsert task MR: %w", err)
	}
	if _, err := s.EnsureMRWatch(ctx, sessionID, taskID, repositoryID, projectPath, mr.IID, branch); err != nil {
		s.logger.Warn("failed to ensure MR watch after auto-link",
			zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
	}
	s.publishTaskMRUpdated(ctx, workspaceID, association)
	return association, nil
}
