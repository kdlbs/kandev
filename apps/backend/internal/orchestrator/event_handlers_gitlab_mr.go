package orchestrator

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/gitlab"
)

// gitlabMRStateOpen mirrors the gitlab package's own normalized "open" state
// value (GitLab's API returns "opened"; the client normalizes it to "open"
// before it ever reaches a TaskMR row). Kept as a local constant rather than
// exported from the gitlab package, matching how event_handlers_github.go's
// githubPRStateOpen mirrors the github package's vocabulary.
const gitlabMRStateOpen = "open"

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
	if s.alreadyLinkedGitLabMR(ctx, sessionID, taskID, repositoryID, branch) {
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
			if s.alreadyLinkedGitLabMR(ctx, sessionID, taskID, repositoryID, branch) {
				return
			}
		}
		if s.tryAutoLinkMRForPush(ctx, workspaceID, sessionID, taskID, repositoryID, repositoryName, projectPath, branch, delay) {
			return
		}
	}
	s.logger.Warn("exhausted all retries, no gitlab MR found after push",
		zap.String("session_id", sessionID),
		zap.String("task_id", taskID),
		zap.String("repository_name", repositoryName),
		zap.String("branch", branch))
}

// alreadyLinkedGitLabMR checks for an existing (repositoryID, branch)
// association and, if found, ensures its refresh watch before reporting
// true. Checked both before the retry loop starts and again after each
// delay, in case a concurrent Create-MR action or manual URL link races
// ahead of this push detection.
func (s *Service) alreadyLinkedGitLabMR(ctx context.Context, sessionID, taskID, repositoryID, branch string) bool {
	existing := s.gitlabTaskMRFor(ctx, taskID, repositoryID, branch)
	if existing == nil {
		return false
	}
	s.ensureWatchForLinkedMR(ctx, sessionID, taskID, repositoryID, existing)
	return true
}

// tryAutoLinkMRForPush runs one AutoLinkMRForBranch attempt for the push
// retry loop, logging the outcome. Returns true when a link was made (the
// caller's retry loop should stop).
func (s *Service) tryAutoLinkMRForPush(
	ctx context.Context, workspaceID, sessionID, taskID, repositoryID, repositoryName, projectPath, branch string, delay time.Duration,
) bool {
	assoc, err := s.gitlabMRLinkService.AutoLinkMRForBranch(
		ctx, workspaceID, sessionID, taskID, repositoryID, projectPath, branch,
	)
	if err != nil {
		// A real service/store error (e.g. an unconfigured GitLab connection)
		// is a backend problem, not "no MR is open yet" — worth a louder log
		// than the normal not-found retry case, even though the retry loop
		// itself still just tries again (a transient error may clear before
		// the next delay).
		s.logger.Warn("gitlab auto-link attempt failed (will retry)",
			zap.String("branch", branch),
			zap.String("session_id", sessionID),
			zap.String("repository_name", repositoryName),
			zap.Duration("delay", delay),
			zap.Error(err))
		return false
	}
	if assoc == nil {
		s.logger.Debug("no gitlab MR found for branch (will retry)",
			zap.String("branch", branch),
			zap.String("session_id", sessionID),
			zap.String("repository_name", repositoryName),
			zap.Duration("delay", delay))
		return false
	}
	s.logger.Info("gitlab MR found after push, associated with task",
		zap.String("session_id", sessionID),
		zap.String("task_id", taskID),
		zap.String("repository_name", repositoryName),
		zap.Int("mr_iid", assoc.MRIID),
		zap.String("branch", branch))
	// AutoLinkMRForBranch already tries to ensure the watch itself, but only
	// logs a failure rather than surfacing it — this may be the push path's
	// last attempt (retries are exhausted once the caller returns), so give
	// the idempotent get-or-create one more chance here rather than
	// declaring the link complete with the association persisted but no
	// watch to keep its status polled.
	s.ensureWatchForLinkedMR(ctx, sessionID, taskID, repositoryID, assoc)
	return true
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

// gitlabTaskMRFor returns taskID's existing *open* association for
// (repositoryID, branch), or nil when there is none, so retries and
// duplicate push events don't refetch or re-link an MR that's already
// linked. Scoped to state=open so a merged/closed MR's leftover row doesn't
// shadow a replacement MR opened later from the same branch (the prior MR
// closed, a new one opened): EnsureMRWatch's iid-replacement support only
// runs when the search this guards against actually re-executes instead of
// always short-circuiting on any historical row for the branch.
func (s *Service) gitlabTaskMRFor(ctx context.Context, taskID, repositoryID, branch string) *gitlab.TaskMR {
	mrs, err := s.gitlabMRLinkService.ListTaskMRsByTask(ctx, taskID)
	if err != nil {
		return nil
	}
	for _, mr := range mrs {
		if mr.RepositoryID == repositoryID && mr.HeadBranch == branch && mr.State == gitlabMRStateOpen {
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

	// Push detection never reaches this service's GitLab client for a
	// GitHub-provider repository — dispatchPushDetection routes by provider
	// before either detect function runs. This on-demand entry point has no
	// such router in front of it (a caller invokes it directly by WS action
	// name), so it must apply the same guard itself: without it, calling
	// gitlab.check_session_mr for a GitHub-backed session would install a
	// bogus GitLab watch keyed off the GitHub repository's owner/name before
	// AutoLinkMRForBranch's own identity check ever runs.
	if s.resolvePushRepositoryProvider(ctx, sessionID, taskID, "") != gitlabProviderName {
		return false, nil
	}

	owner, repoName, repositoryID := s.resolvePushRepo(ctx, sessionID, taskID, "")
	if owner == "" || repoName == "" || repositoryID == "" {
		return false, nil
	}
	branch := strings.TrimSpace(s.resolvePRWatchBranch(ctx, taskID, sessionID, ""))
	if branch == "" {
		return false, nil
	}
	projectPath := owner + "/" + repoName

	// Already associated for this exact (repository, branch) — ensure its
	// watch exists (Create-MR action / manual URL linking writes
	// gitlab_task_mrs but no watch) and return without re-searching. Scoped
	// to (repositoryID, branch) rather than "any MR linked to the task", so a
	// multi-repo/multi-branch task's on-demand check for one branch isn't
	// short-circuited by a sibling branch's already-linked MR.
	if existing := s.gitlabTaskMRFor(ctx, taskID, repositoryID, branch); existing != nil {
		s.ensureWatchForLinkedMR(ctx, sessionID, taskID, repositoryID, existing)
		return true, nil
	}

	workspaceID := s.taskWorkspaceID(ctx, taskID)
	if workspaceID == "" {
		return false, nil
	}

	// Ensure a watch exists (mr_iid=0, backfilled by AutoLinkMRForBranch on a
	// match) before searching, so the background poller keeps checking this
	// branch even when no MR is open yet — mirrors CheckSessionPR's
	// EnsurePRWatchForWorkspace-before-lookup ordering.
	if _, err := s.gitlabMRLinkService.EnsureMRWatch(ctx, sessionID, taskID, repositoryID, projectPath, 0, branch); err != nil {
		s.logger.Warn("failed to ensure MR watch during check",
			zap.String("session_id", sessionID), zap.Error(err))
	}

	assoc, err := s.gitlabMRLinkService.AutoLinkMRForBranch(
		ctx, workspaceID, sessionID, taskID, repositoryID, projectPath, branch,
	)
	// A real service/store error (e.g. an unconfigured GitLab connection) is
	// a backend problem, not "no MR is open on this branch" — propagate it
	// to the WS caller rather than reporting a misleadingly clean
	// found=false. Only assoc == nil with a nil error means "searched and
	// found nothing."
	if err != nil {
		return false, err
	}
	if assoc == nil {
		return false, nil
	}
	return true, nil
}
