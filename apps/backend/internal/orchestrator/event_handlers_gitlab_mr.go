package orchestrator

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/gitlab"
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
	// An empty branch must never reach FindMRByBranch: it builds
	// `?source_branch=&state=opened&per_page=1`, which carries no effective
	// source-branch filter, so GitLab answers with an arbitrary open MR of the
	// project and we would link the wrong merge request to the task. Git refs
	// cannot contain spaces, so a whitespace-only value is equally invalid.
	// CheckSessionMR already refuses an empty branch; this is the same guard on
	// the push path.
	branch = strings.TrimSpace(branch)
	if branch == "" {
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

	// Already linked for this (task, repository, branch) — don't re-link, but
	// still make sure the refresh watch exists. AssociateExistingMRByURL (the
	// Create-MR action and manual URL linking) writes gitlab_task_mrs without
	// a watch, so returning outright here would leave the association with
	// nothing for Poller.runMRMonitor to poll and its review/pipeline status
	// would never update.
	if existing := s.gitlabTaskMRFor(ctx, taskID, repositoryID, branch); existing != nil {
		s.ensureWatchForLinkedMR(ctx, sessionID, taskID, repositoryID, existing)
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
			if existing := s.gitlabTaskMRFor(ctx, taskID, repositoryID, branch); existing != nil {
				s.ensureWatchForLinkedMR(ctx, sessionID, taskID, repositoryID, existing)
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

// ensureWatchForLinkedMR creates the refresh watch for an association that
// already exists, covering the MRs linked by AssociateExistingMRByURL (the
// Create-MR action and manual URL linking), which persists gitlab_task_mrs
// but no watch. Best-effort: the association is already correct, so a watch
// failure must not turn push detection into an error path.
func (s *Service) ensureWatchForLinkedMR(
	ctx context.Context, sessionID, taskID, repositoryID string, mr *gitlab.TaskMR,
) {
	if _, err := s.gitlabMRLinkService.EnsureMRWatch(
		ctx, sessionID, taskID, repositoryID, mr.ProjectPath, mr.MRIID, mr.HeadBranch,
	); err != nil {
		s.logger.Warn("failed to ensure MR watch for already-linked merge request",
			zap.String("session_id", sessionID),
			zap.String("task_id", taskID),
			zap.Int("mr_iid", mr.MRIID),
			zap.Error(err))
	}
}

// gitlabTaskMRFor returns taskID's existing association for (repositoryID,
// branch), or nil when there is none, so retries and duplicate push events
// don't refetch or re-link an MR that's already linked.
func (s *Service) gitlabTaskMRFor(ctx context.Context, taskID, repositoryID, branch string) *gitlab.TaskMR {
	mrs, err := s.gitlabMRLinkService.ListTaskMRsByTask(ctx, taskID)
	if err != nil {
		return nil
	}
	for _, mr := range mrs {
		if mr.RepositoryID == repositoryID && mr.HeadBranch == branch {
			return mr
		}
	}
	return nil
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
