package github

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// unwatchedTaskPRs returns the linked TaskPR rows whose upstream state can
// still change but which no supplied watch is pointing at.
//
// A PR watch is unique per (session, repository, branch) and gets re-pointed
// whenever the session's branch moves on — a follow-up PR, a renamed branch,
// a second branch in the same repo. Every PR the task linked from an earlier
// branch then loses its only sync handle: both the 1-minute poller and the
// on-demand WS sync iterate *watches*, so an orphaned row keeps whatever
// state it last observed. A PR that merges after the handover renders as
// open, green and mergeable in the UI forever, with a live merge button.
//
// Terminal rows (merged / closed) and detached rows are excluded — their
// state can no longer change, so re-fetching them would grow every batch
// without bound as a task accumulates PRs. Rows synced inside
// PRSyncFreshnessWindow are excluded too, mirroring triggerPRStatusSync's own
// freshness short-circuit so a burst of syncs costs one upstream call.
func unwatchedTaskPRs(rows []*TaskPR, watches []*PRWatch, now time.Time) []*TaskPR {
	watched := make(map[string]struct{}, len(watches))
	for _, w := range watches {
		if w == nil || w.PRNumber == 0 {
			continue
		}
		watched[prStatusCacheKey(w.Owner, w.Repo, w.PRNumber)] = struct{}{}
	}
	pending := make([]*TaskPR, 0, len(rows))
	for _, tp := range rows {
		if !taskPRNeedsUnwatchedSync(tp, now) {
			continue
		}
		if _, ok := watched[prStatusCacheKey(tp.Owner, tp.Repo, tp.PRNumber)]; ok {
			continue
		}
		pending = append(pending, tp)
	}
	return pending
}

func taskPRNeedsUnwatchedSync(tp *TaskPR, now time.Time) bool {
	if tp == nil || tp.TaskID == "" || tp.PRNumber == 0 || tp.DetachedAt != nil {
		return false
	}
	if tp.State == prStateMerged || tp.State == prStateClosed {
		return false
	}
	return tp.LastSyncedAt == nil || now.Sub(*tp.LastSyncedAt) >= PRSyncFreshnessWindow
}

// syncUnwatchedTaskPRs refreshes the supplied unwatched rows, one batched
// query per workspace. Best-effort: every failure is logged at Debug and
// leaves the row for the next attempt, matching the surrounding
// reconciliation paths — a dead repo must not fail the caller's sync.
//
// fallbackWorkspaceID covers legacy rows written before task_prs carried
// workspace ownership; rows with neither are skipped because there is no
// credential to resolve.
func (s *Service) syncUnwatchedTaskPRs(ctx context.Context, pending []*TaskPR, fallbackWorkspaceID string) {
	byWorkspace := make(map[string][]*TaskPR, 1)
	for _, tp := range pending {
		workspaceID := tp.WorkspaceID
		if workspaceID == "" {
			workspaceID = fallbackWorkspaceID
		}
		if workspaceID == "" {
			s.logger.Debug("skipping unwatched task PR without workspace ownership",
				zap.String("task_id", tp.TaskID), zap.Int("pr_number", tp.PRNumber))
			continue
		}
		byWorkspace[workspaceID] = append(byWorkspace[workspaceID], tp)
	}
	for workspaceID, group := range byWorkspace {
		s.syncUnwatchedTaskPRGroup(ctx, workspaceID, group)
	}
}

func (s *Service) syncUnwatchedTaskPRGroup(ctx context.Context, workspaceID string, group []*TaskPR) {
	resolved, err := s.resolveAutomationClient(ctx, workspaceID, "", "")
	if err != nil {
		s.logger.Debug("resolve client for unwatched task PR sync failed",
			zap.String("workspace_id", workspaceID), zap.Error(err))
		return
	}
	statuses := s.fetchUnwatchedTaskPRStatuses(ctx, resolved, group)
	for _, tp := range group {
		status := statuses[prStatusCacheKey(tp.Owner, tp.Repo, tp.PRNumber)]
		if status == nil || status.PR == nil {
			continue
		}
		if syncErr := s.SyncTaskPR(ctx, tp.TaskID, status); syncErr != nil {
			s.logger.Debug("unwatched task PR sync failed",
				zap.String("task_id", tp.TaskID), zap.Int("pr_number", tp.PRNumber), zap.Error(syncErr))
		}
	}
}

// fetchUnwatchedTaskPRStatuses prefers the batched GraphQL query — one call
// for the whole group — and falls back to per-PR status fetches when the
// client can't speak GraphQL (NoopClient) or the batch itself fails. Same
// batched-then-per-item shape as runBatchedOrPerWatchSync.
func (s *Service) fetchUnwatchedTaskPRStatuses(
	ctx context.Context, resolved *resolvedServiceClient, group []*TaskPR,
) map[string]*PRStatus {
	refs := make([]graphQLPRRef, 0, len(group))
	for _, tp := range group {
		// The repo is in the 10-min negative cache; probing it would only
		// burn a gh throttle slot to fail again.
		if s.isRepoCachedAsMissingForScope(resolved.CacheScope, tp.Owner, tp.Repo) {
			continue
		}
		refs = append(refs, graphQLPRRef{Owner: tp.Owner, Repo: tp.Repo, Number: tp.PRNumber})
	}
	if len(refs) == 0 {
		return nil
	}
	if exec, execErr := graphQLExecutorFor(resolved.Client); execErr == nil {
		// Snapshot the negative-cache generation BEFORE the fetch so a
		// concurrent eviction wins; see Service.markRepoAsMissing.
		repoErrGen := s.repoErrorGenSnapshot()
		out, err := runBatchedPRQuery(ctx, exec, refs)
		out, err = s.absorbMissingReposErr(out, err, resolved.CacheScope, repoErrGen)
		if err == nil {
			return out
		}
		s.logger.Debug("batched unwatched task PR query failed; falling back per PR", zap.Error(err))
	}
	return s.fetchUnwatchedTaskPRStatusesPerPR(ctx, resolved, refs)
}

func (s *Service) fetchUnwatchedTaskPRStatusesPerPR(
	ctx context.Context, resolved *resolvedServiceClient, refs []graphQLPRRef,
) map[string]*PRStatus {
	out := make(map[string]*PRStatus, len(refs))
	for _, ref := range refs {
		status, err := s.getPRStatus(ctx, resolved.Client, resolved.CacheScope, ref.Owner, ref.Repo, ref.Number)
		if err != nil {
			s.logger.Debug("per-PR unwatched task PR status fetch failed",
				zap.String("owner", ref.Owner), zap.String("repo", ref.Repo),
				zap.Int("pr_number", ref.Number), zap.Error(err))
			continue
		}
		if status != nil {
			out[prStatusCacheKey(ref.Owner, ref.Repo, ref.Number)] = status
		}
	}
	return out
}

// reconcileTaskUnwatchedPRs refreshes the task's unwatched rows and returns
// the reloaded row set, so the WS caller hands the frontend the merged state
// rather than the pre-sync snapshot. Returns the rows unchanged when nothing
// needed reconciling.
func (s *Service) reconcileTaskUnwatchedPRs(
	ctx context.Context, taskID string, watches []*PRWatch,
) ([]*TaskPR, error) {
	rows, err := s.store.ListTaskPRsByTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("list task PRs: %w", err)
	}
	pending := unwatchedTaskPRs(rows, watches, time.Now().UTC())
	if len(pending) == 0 {
		return rows, nil
	}
	fallbackWorkspaceID := ""
	if len(watches) > 0 && watches[0] != nil {
		fallbackWorkspaceID = watches[0].WorkspaceID
	}
	s.syncUnwatchedTaskPRs(ctx, pending, fallbackWorkspaceID)
	return s.store.ListTaskPRsByTask(ctx, taskID)
}

// collectStaleWorkspaceSyncTargets reads the watches and the unwatched
// non-terminal PR rows for every stale task in one pass, so the background
// workspace refresh can fan both into a single batched call each.
func (s *Service) collectStaleWorkspaceSyncTargets(
	ctx context.Context, staleTasks map[string]struct{},
) ([]*PRWatch, []*TaskPR) {
	var allWatches []*PRWatch
	var pending []*TaskPR
	now := time.Now().UTC()
	for taskID := range staleTasks {
		watches, err := s.store.ListPRWatchesByTask(ctx, taskID)
		if err != nil {
			s.logger.Debug("list PR watches for refresh failed",
				zap.String("task_id", taskID), zap.Error(err))
			continue
		}
		allWatches = append(allWatches, watches...)
		rows, err := s.store.ListTaskPRsByTask(ctx, taskID)
		if err != nil {
			s.logger.Debug("list task PRs for refresh failed",
				zap.String("task_id", taskID), zap.Error(err))
			continue
		}
		pending = append(pending, unwatchedTaskPRs(rows, watches, now)...)
	}
	return allWatches, pending
}
