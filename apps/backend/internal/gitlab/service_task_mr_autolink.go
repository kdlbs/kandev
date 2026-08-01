package gitlab

import (
	"context"
	"fmt"
	"strings"

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
	// A blank branch must never reach FindMRByBranch: PATClient interpolates
	// it into `?source_branch=&state=opened&per_page=1`, a query with no
	// effective source-branch filter, so GitLab would answer with an
	// arbitrary open MR of the project and the wrong MR would get linked.
	// Git refs cannot contain whitespace, so a whitespace-only value is
	// equally invalid. Guarded here — not just at orchestrator call sites —
	// so every caller of this service method gets the same safety guarantee.
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return nil, nil
	}
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
		// No MR is open on this branch yet — the normal "pushed but nothing
		// opened" result, not an error. Still leave a placeholder (iid=0)
		// watch behind so the poller's own iid<=0 resolution (CheckMRWatch)
		// can discover an MR opened later — e.g. from the GitLab web UI —
		// well after push detection's own retry window ([0, 30s, 60s])
		// closes. Best-effort: a watch failure here must not turn "no MR
		// yet" into a hard auto-link error.
		if _, err := s.EnsureMRWatch(ctx, sessionID, taskID, repositoryID, projectPath, 0, branch); err != nil {
			s.logger.Warn("failed to ensure placeholder MR watch while no MR is open on branch",
				zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
		}
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
	// Detach from ctx's cancellation, bounded by the same timeout as the
	// URL-link flow's equivalent post-commit call: the association above
	// already committed, so a caller's context canceling right after must
	// not skip watch creation and silently leave this MR unpolled.
	watchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), watchDeleteTimeout)
	defer cancel()
	if _, err := s.EnsureMRWatch(watchCtx, sessionID, taskID, repositoryID, projectPath, mr.IID, branch); err != nil {
		s.logger.Warn("failed to ensure MR watch after auto-link",
			zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
	}
	s.publishTaskMRUpdated(ctx, workspaceID, association)
	return association, nil
}
