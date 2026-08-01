package orchestrator

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// detectPushAndAssociateMR is the GitLab twin of detectPushAndAssociatePR: on
// push to a session branch, it looks up the open merge request whose source
// branch matches and links it to the task, scoped by repository_id. Mirrors
// the GitHub retry shape ([0, 30s, 60s]) so an MR opened moments after the
// push (a common `glab mr create` race) still gets picked up.
func (s *Service) detectPushAndAssociateMR(
	ctx context.Context, sessionID, taskID, repositoryName, branch string,
) {
	if s.gitlabMRLinkService == nil {
		return
	}
	workspaceID := s.taskWorkspaceID(ctx, taskID)
	if workspaceID == "" {
		return
	}
	owner, repoName, repositoryID := s.resolvePushRepo(ctx, sessionID, taskID, repositoryName)
	if owner == "" || repoName == "" || repositoryID == "" {
		return
	}
	projectPath := owner + "/" + repoName

	// Already linked for this (task, repository, branch) — nothing to do.
	// Mirrors GitHub's existing-watch short-circuit in detectPushAndAssociatePR.
	if s.gitlabTaskMRExists(ctx, taskID, repositoryID, branch) {
		return
	}

	delays := []time.Duration{0, 30 * time.Second, 60 * time.Second}
	for _, delay := range delays {
		if delay > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			if s.gitlabTaskMRExists(ctx, taskID, repositoryID, branch) {
				return
			}
		}
		assoc, err := s.gitlabMRLinkService.AutoLinkMRForBranch(
			ctx, workspaceID, sessionID, taskID, repositoryID, projectPath, branch,
		)
		if err != nil || assoc == nil {
			s.logger.Debug("no gitlab MR found for branch (will retry)",
				zap.String("branch", branch),
				zap.String("session_id", sessionID),
				zap.String("repository_name", repositoryName),
				zap.Duration("delay", delay))
			continue
		}
		s.logger.Info("gitlab MR found after push, associated with task",
			zap.String("session_id", sessionID),
			zap.String("task_id", taskID),
			zap.String("repository_name", repositoryName),
			zap.Int("mr_iid", assoc.MRIID),
			zap.String("branch", branch))
		return
	}
	s.logger.Warn("exhausted all retries, no gitlab MR found after push",
		zap.String("session_id", sessionID),
		zap.String("task_id", taskID),
		zap.String("repository_name", repositoryName),
		zap.String("branch", branch))
}

// gitlabTaskMRExists reports whether taskID already has an association for
// (repositoryID, branch), so retries and duplicate push events don't refetch
// or re-link an MR that's already linked.
func (s *Service) gitlabTaskMRExists(ctx context.Context, taskID, repositoryID, branch string) bool {
	mrs, err := s.gitlabMRLinkService.ListTaskMRsByTask(ctx, taskID)
	if err != nil {
		return false
	}
	for _, mr := range mrs {
		if mr.RepositoryID == repositoryID && mr.HeadBranch == branch {
			return true
		}
	}
	return false
}

// CheckSessionMR checks whether an open GitLab merge request exists for a
// session's branch and associates it if found. On-demand counterpart to the
// push-detection path, mirroring GitHub's CheckSessionPR: a caller can
// trigger immediate MR detection without waiting for the next push or the
// background poller.
func (s *Service) CheckSessionMR(ctx context.Context, taskID, sessionID string) (bool, error) {
	// Per-user scoping first, before any early return — see CheckSessionPR's
	// doc comment for why both IDs (not just the session) must be authorized:
	// everything below is keyed off taskID, so authorizing only the session
	// would let a caller pair one of their own sessions with another user's
	// task.
	if err := s.authorizeTaskSessionPair(ctx, taskID, sessionID); err != nil {
		return false, nil
	}

	if s.gitlabMRLinkService == nil {
		return false, nil
	}

	// Already associated (any repository/branch on this task) — nothing to do.
	if existing, err := s.gitlabMRLinkService.ListTaskMRsByTask(ctx, taskID); err == nil && len(existing) > 0 {
		return true, nil
	}

	owner, repoName, repositoryID := s.resolvePushRepo(ctx, sessionID, taskID, "")
	if owner == "" || repoName == "" || repositoryID == "" {
		return false, nil
	}
	branch := s.resolvePRWatchBranch(ctx, taskID, sessionID, "")
	if branch == "" {
		return false, nil
	}
	workspaceID := s.taskWorkspaceID(ctx, taskID)
	if workspaceID == "" {
		return false, nil
	}

	assoc, err := s.gitlabMRLinkService.AutoLinkMRForBranch(
		ctx, workspaceID, sessionID, taskID, repositoryID, owner+"/"+repoName, branch,
	)
	if err != nil || assoc == nil {
		return false, nil
	}
	return true, nil
}
