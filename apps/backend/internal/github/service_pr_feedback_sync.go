package github

import (
	"context"

	"go.uber.org/zap"
)

// persistPRFeedbackState reconciles the workspace's github_task_prs rows for a
// PR from the live data a feedback fetch just retrieved.
//
// The feedback read is the same server-side GitHub fetch the status sync makes
// — GetPR + ListPRReviews + ListCheckRuns, plus comments — so the authoritative
// PR state is already in hand here. It used to be shipped to the browser and
// re-derived there (useSyncLivePRState in pr-detail-panel.tsx), which patched
// only the Zustand copy: opening a PR's detail panel turned that one tab purple
// while the kanban card, the boot payload and every other surface kept reading
// state=open from the stale row, and a reload snapped it back. Writing through
// SyncTaskPR here makes the backend the single writer and lets the resulting
// github.task_pr.updated event converge every surface at once.
//
// Best-effort by contract: this hangs off a read endpoint, so every failure is
// logged at Debug and swallowed. A dead store must never turn a working PR
// panel into an error.
//
// Scoping: rows are looked up inside workspaceID only, matching the credential
// scope the caller already passed ensureRepositoryInWorkspaceScope with. The
// legacy workspace-less GetPRFeedback entry point passes "" and is skipped
// rather than being allowed to write across workspaces.
func (s *Service) persistPRFeedbackState(ctx context.Context, workspaceID string, feedback *PRFeedback) {
	if s == nil || s.store == nil || workspaceID == "" {
		return
	}
	if feedback == nil || feedback.PR == nil || feedback.PR.Number == 0 {
		return
	}
	pr := feedback.PR
	rows, err := s.store.ListTaskPRsByPRNumber(ctx, workspaceID, pr.RepoOwner, pr.RepoName, pr.Number)
	if err != nil {
		s.logger.Debug("list task PRs for feedback persist failed",
			zap.String("workspace_id", workspaceID), zap.String("owner", pr.RepoOwner),
			zap.String("repo", pr.RepoName), zap.Int("pr_number", pr.Number), zap.Error(err))
		return
	}
	if len(rows) == 0 {
		return
	}
	status := newPRStatus(pr, feedback.Reviews, feedback.Checks)
	// Several tasks in a workspace can link the same PR; each owns its own row.
	// SyncTaskPR re-narrows by (task, owner, repo, number) internally.
	seen := make(map[string]struct{}, len(rows))
	for _, tp := range rows {
		if tp == nil || tp.TaskID == "" {
			continue
		}
		if _, dup := seen[tp.TaskID]; dup {
			continue
		}
		seen[tp.TaskID] = struct{}{}
		if syncErr := s.SyncTaskPR(ctx, tp.TaskID, status); syncErr != nil {
			s.logger.Debug("task PR sync from PR feedback failed",
				zap.String("task_id", tp.TaskID), zap.Int("pr_number", pr.Number), zap.Error(syncErr))
		}
	}
}
