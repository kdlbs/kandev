package github

import (
	"context"
	"fmt"
	"sort"
	"strings"
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

// syncUnwatchedTaskPRs reconciles the supplied unwatched rows, one batched
// query per workspace. Best-effort: every failure is logged at Debug and
// leaves the row for the next attempt, matching the surrounding
// reconciliation paths — a dead repo must not fail the caller's sync.
//
// Only lifecycle fields are written (see reconcileTaskPRLifecycle). Check and
// review aggregates deliberately stay untouched: they belong to the active,
// watch-covered PR, and a row nobody watches only needs to learn that it
// reached a terminal state.
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
	live := s.fetchUnwatchedTaskPRs(ctx, resolved, group)
	seen := make(map[string]struct{}, len(group))
	for _, tp := range group {
		if tp == nil || tp.TaskID == "" {
			continue
		}
		status := live[prStatusCacheKey(tp.Owner, tp.Repo, tp.PRNumber)]
		if status == nil || status.PR == nil {
			continue
		}
		effectiveTaskID := s.reconcileTaskPROwnership(ctx, "", tp.TaskID, tp.RepositoryID, tp.PRNumber)
		seenKey := fmt.Sprintf("%s\x00%s\x00%d", effectiveTaskID, tp.RepositoryID, tp.PRNumber)
		if _, duplicate := seen[seenKey]; duplicate {
			continue
		}
		seen[seenKey] = struct{}{}
		if effectiveTaskID != tp.TaskID {
			ownerTP, loadErr := s.store.GetTaskPRByRepoAndNumber(ctx, effectiveTaskID, tp.RepositoryID, tp.PRNumber)
			if loadErr != nil {
				s.logger.Debug("load reconciled owner task PR failed",
					zap.String("task_id", effectiveTaskID), zap.Int("pr_number", tp.PRNumber), zap.Error(loadErr))
				continue
			}
			if ownerTP == nil {
				continue
			}
			tp = ownerTP
		}
		if syncErr := s.reconcileTaskPRLifecycle(ctx, tp, status); syncErr != nil {
			s.logger.Debug("unwatched task PR reconcile failed",
				zap.String("task_id", tp.TaskID), zap.Int("pr_number", tp.PRNumber), zap.Error(syncErr))
		}
	}
}

// reconcileTaskPRLifecycle writes the fields that describe where the PR sits
// in its lifecycle, plus the five outcome-attribution fields (AC-36/AC-37
// require merged_by_login / is_draft to be non-NULL on a row this path can
// take terminal, so they cannot be left for a populating sync that may never
// come — a PR nobody watches gets no other writer). It reuses
// resolveTaskPROutcomeFields, the same populated/preserve logic SyncTaskPR
// uses, on the *PRStatus fetchUnwatchedTaskPRs already builds from data this
// path was fetching anyway. It then publishes the same update event
// SyncTaskPR does so every client converges.
//
// Aggregates (checks_state, checks_total/passing, review_state, review
// counts, unresolved threads, mergeable_state) stay untouched on purpose:
// they are owned by the watch-driven sync, which fetches the reviews and
// check runs needed to compute them. Writing them from here would mean
// either re-fetching all of that for a PR nobody is working on, or — worse —
// persisting a less-informed answer over a better one: the per-PR REST
// status sets ChecksPopulated/ReviewCountsPopulated with zeroed counts when
// it has nothing to count, which SyncTaskPR then faithfully stores.
func (s *Service) reconcileTaskPRLifecycle(ctx context.Context, tp *TaskPR, status *PRStatus) error {
	pr := status.PR
	isDraft, changedFiles, mergedByLogin, closedByLogin, autoMergeObservedAt :=
		resolveTaskPROutcomeFields(tp, status)

	changed := tp.State != pr.State ||
		!timeEqual(tp.MergedAt, pr.MergedAt) ||
		!timeEqual(tp.ClosedAt, pr.ClosedAt) ||
		!boolPtrEqual(tp.IsDraft, isDraft) ||
		!intPtrEqual(tp.ChangedFiles, changedFiles) ||
		!stringPtrEqual(tp.MergedByLogin, mergedByLogin) ||
		!stringPtrEqual(tp.ClosedByLogin, closedByLogin) ||
		!timeEqual(tp.AutoMergeObservedAt, autoMergeObservedAt)

	tp.State = pr.State
	tp.MergedAt = pr.MergedAt
	tp.ClosedAt = pr.ClosedAt
	tp.IsDraft = isDraft
	tp.ChangedFiles = changedFiles
	tp.MergedByLogin = mergedByLogin
	tp.ClosedByLogin = closedByLogin
	tp.AutoMergeObservedAt = autoMergeObservedAt
	now := time.Now().UTC()
	tp.LastSyncedAt = &now

	// AC-18: publish must reflect the row as stored, not this call's
	// in-memory tp — UpdateTaskPR latches AutoMergeObservedAt through
	// COALESCE, so a concurrent writer can persist an earlier timestamp than
	// this call's own resolved value. persistAndPublishTaskPRSync already
	// re-reads before publishing for exactly this reason (codex [P2] on the
	// SyncTaskPR path); reuse it here instead of duplicating the write.
	return s.persistAndPublishTaskPRSync(ctx, tp.TaskID, status.PR, tp, changed, status.OutcomeFieldsPopulated, false)
}

// fetchUnwatchedTaskPRs prefers the batched GraphQL query — one call for the
// whole group — and falls back to per-PR reads when the client can't speak
// GraphQL (NoopClient) or the batch itself fails. Same batched-then-per-item
// shape as runBatchedOrPerWatchSync. Returns the live statuses keyed by
// prStatusCacheKey, carrying the outcome-field population flags each path
// already computed rather than the bare PR (AC-08/AC-10/AC-14 apply here
// exactly as they do to the watch-driven batched query).
func (s *Service) fetchUnwatchedTaskPRs(
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
		out, err := s.batchedUnwatchedFetch(ctx, exec, resolved.CacheScope, refs)
		if err == nil {
			return out
		}
		s.logger.Debug("batched unwatched task PR query failed; falling back per PR", zap.Error(err))
	}
	return s.fetchUnwatchedTaskPRsPerPR(ctx, resolved, refs)
}

// batchedUnwatchedFetch runs the batched query under the same service-level
// singleflight the watch path uses (see fetchBatchedWatchStatuses). Without it
// the on-demand sync, the workspace background refresh, and a second tab all
// issue their own identical GraphQL call — the storm that singleflight exists
// to damp. Keyed on the sorted ref set so equivalent callers share one flight,
// and scoped by credential so two workspaces never share a result.
//
// The upstream fetch detaches from the leader's cancellation via
// derivedFetchContext so one caller disconnecting mid-flight doesn't cascade
// context.Canceled to its co-waiters, while keeping the leader's deadline.
func (s *Service) batchedUnwatchedFetch(
	ctx context.Context, exec GraphQLExecutor, cacheScope string, refs []graphQLPRRef,
) (map[string]*PRStatus, error) {
	key := scopedCacheKey(cacheScope, "unwatched:"+batchedRefsKey(refs))
	fetchCtx, cancelFetch := derivedFetchContext(ctx)
	defer cancelFetch()
	v, err, _ := s.syncGroup.Do(key, func() (interface{}, error) {
		// Snapshot the negative-cache generation BEFORE the fetch so a
		// concurrent eviction wins; see Service.markRepoAsMissing.
		repoErrGen := s.repoErrorGenSnapshot()
		out, queryErr := runBatchedPRQuery(fetchCtx, exec, refs)
		return s.absorbMissingReposErr(out, queryErr, cacheScope, repoErrGen)
	})
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	return v.(map[string]*PRStatus), nil
}

// batchedRefsKey renders a deterministic key for a numbered-ref set. Sorting
// keeps caller ordering from splitting equivalent flights, and lowercasing
// matches repoErrorCacheKey. Mirrors batchedFetchSingleflightKey, which builds
// the same shape from watches.
func batchedRefsKey(refs []graphQLPRRef) string {
	parts := make([]string, 0, len(refs))
	for _, r := range refs {
		parts = append(parts, fmt.Sprintf("%s/%s#%d",
			strings.ToLower(r.Owner), strings.ToLower(r.Repo), r.Number))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

// fetchUnwatchedTaskPRsPerPR reads one PR at a time. GetPR is deliberate: the
// full status helper also lists reviews and check runs (three calls per PR) to
// compute aggregates this path does not write. GetPR is still a full
// single-pull-request fetch, so is_draft/changed_files/merged_by_login are
// real observations here too (AC-10); ClosureAttributionPopulated stays
// false since neither REST nor the gh CLI can see the closing actor (AC-15).
func (s *Service) fetchUnwatchedTaskPRsPerPR(
	ctx context.Context, resolved *resolvedServiceClient, refs []graphQLPRRef,
) map[string]*PRStatus {
	out := make(map[string]*PRStatus, len(refs))
	for _, ref := range refs {
		pr, err := resolved.Client.GetPR(ctx, ref.Owner, ref.Repo, ref.Number)
		if err != nil {
			s.logger.Debug("per-PR unwatched task PR read failed",
				zap.String("owner", ref.Owner), zap.String("repo", ref.Repo),
				zap.Int("pr_number", ref.Number), zap.Error(err))
			continue
		}
		if pr != nil {
			out[prStatusCacheKey(ref.Owner, ref.Repo, ref.Number)] = &PRStatus{
				PR:                     pr,
				OutcomeFieldsPopulated: true,
			}
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
