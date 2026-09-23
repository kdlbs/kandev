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
// without bound as a task accumulates PRs. Passive reads also exclude rows
// synced inside PRSyncFreshnessWindow, mirroring triggerPRStatusSync's own
// freshness short-circuit. An explicit refresh bypasses only that freshness
// check; it keeps the terminal, detached, and malformed-row guards.
func unwatchedTaskPRs(
	rows []*TaskPR, watches []*PRWatch, now time.Time, explicitRefresh bool,
) []*TaskPR {
	watched := make(map[string]struct{}, len(watches))
	for _, w := range watches {
		if w == nil || w.PRNumber == 0 {
			continue
		}
		watched[prStatusCacheKey(w.Owner, w.Repo, w.PRNumber)] = struct{}{}
	}
	pending := make([]*TaskPR, 0, len(rows))
	for _, tp := range rows {
		if !taskPREligibleForUnwatchedSync(tp) {
			continue
		}
		if !explicitRefresh && !taskPRNeedsUnwatchedSync(tp, now) {
			continue
		}
		if _, ok := watched[prStatusCacheKey(tp.Owner, tp.Repo, tp.PRNumber)]; ok {
			continue
		}
		pending = append(pending, tp)
	}
	return pending
}

func taskPREligibleForUnwatchedSync(tp *TaskPR) bool {
	return tp != nil && tp.TaskID != "" && tp.PRNumber != 0 && tp.DetachedAt == nil &&
		tp.State != prStateMerged && tp.State != prStateClosed
}

func taskPRNeedsUnwatchedSync(tp *TaskPR, now time.Time) bool {
	if !taskPREligibleForUnwatchedSync(tp) {
		return false
	}
	return tp.LastSyncedAt == nil || now.Sub(*tp.LastSyncedAt) >= PRSyncFreshnessWindow
}

// syncUnwatchedTaskPRs reconciles the supplied unwatched rows, one batched
// query per workspace. Best-effort: every failure is logged at Debug and
// leaves the row for the next attempt, matching the surrounding
// reconciliation paths — a dead repo must not fail the caller's sync.
//
// Only lifecycle and head-scoped workflow-attention fields are written (see
// reconcileTaskPRLifecycle). Passive reads use the cheap PR query. An explicit
// refresh may also collect fresh Actions evidence for a row nobody watches.
// Check and review aggregates deliberately stay untouched: they belong to the
// active, watch-covered PR.
//
// fallbackWorkspaceID covers legacy rows written before task_prs carried
// workspace ownership; rows with neither are skipped because there is no
// credential to resolve.
func (s *Service) syncUnwatchedTaskPRs(
	ctx context.Context, pending []*TaskPR, fallbackWorkspaceID string, explicitRefresh bool,
) {
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
		s.syncUnwatchedTaskPRGroup(ctx, workspaceID, group, explicitRefresh)
	}
}

func (s *Service) syncUnwatchedTaskPRGroup(
	ctx context.Context, workspaceID string, group []*TaskPR, explicitRefresh bool,
) {
	resolved, err := s.resolveAutomationClient(ctx, workspaceID, "", "")
	if err != nil {
		s.logger.Debug("resolve client for unwatched task PR sync failed",
			zap.String("workspace_id", workspaceID), zap.Error(err))
		return
	}
	if explicitRefresh {
		for _, tp := range group {
			if tp == nil {
				continue
			}
			s.invalidateWorkflowAttentionForResolvedPR(
				resolved, tp.Owner, tp.Repo, tp.PRNumber, tp.HeadSHA,
			)
		}
	}
	live := s.fetchUnwatchedTaskPRs(ctx, resolved, group, explicitRefresh)
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
	nextHeadSHA := tp.HeadSHA
	if pr.HeadSHA != "" {
		nextHeadSHA = pr.HeadSHA
	}

	changed := tp.HeadSHA != nextHeadSHA ||
		tp.State != pr.State ||
		!timeEqual(tp.MergedAt, pr.MergedAt) ||
		!timeEqual(tp.ClosedAt, pr.ClosedAt) ||
		!boolPtrEqual(tp.IsDraft, isDraft) ||
		!intPtrEqual(tp.ChangedFiles, changedFiles) ||
		!stringPtrEqual(tp.MergedByLogin, mergedByLogin) ||
		!stringPtrEqual(tp.ClosedByLogin, closedByLogin) ||
		!timeEqual(tp.AutoMergeObservedAt, autoMergeObservedAt)
	nextWorkflowAttention := resolveTaskPRWorkflowAttention(tp, status, nextHeadSHA)
	changed = changed || !workflowAttentionSemanticEqual(tp.WorkflowAttention, nextWorkflowAttention)

	tp.State = pr.State
	tp.HeadSHA = nextHeadSHA
	tp.MergedAt = pr.MergedAt
	tp.ClosedAt = pr.ClosedAt
	tp.IsDraft = isDraft
	tp.ChangedFiles = changedFiles
	tp.MergedByLogin = mergedByLogin
	tp.ClosedByLogin = closedByLogin
	tp.AutoMergeObservedAt = autoMergeObservedAt
	tp.WorkflowAttention = nextWorkflowAttention
	tp.WorkflowAttentionJSON = marshalWorkflowAttention(nextWorkflowAttention)
	now := s.now()
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
	ctx context.Context, resolved *resolvedServiceClient, group []*TaskPR, explicitRefresh bool,
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
		out, err := s.batchedUnwatchedFetch(ctx, exec, resolved.Client, resolved.CacheScope, refs, explicitRefresh)
		if err == nil {
			return out
		}
		s.logger.Debug("batched unwatched task PR query failed; falling back per PR", zap.Error(err))
	}
	return s.fetchUnwatchedTaskPRsPerPR(ctx, resolved, refs, explicitRefresh)
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
	ctx context.Context, exec GraphQLExecutor, client Client, cacheScope string,
	refs []graphQLPRRef, explicitRefresh bool,
) (map[string]*PRStatus, error) {
	key := scopedCacheKey(cacheScope, "unwatched:"+batchedRefsKey(refs))
	if explicitRefresh {
		key += prSyncExplicitRefreshKeySuffix
	}
	fetchCtx, cancelFetch := derivedFetchContext(ctx)
	defer cancelFetch()
	v, err, _ := s.syncGroup.Do(key, func() (interface{}, error) {
		// Snapshot the negative-cache generation BEFORE the fetch so a
		// concurrent eviction wins; see Service.markRepoAsMissing.
		repoErrGen := s.repoErrorGenSnapshot()
		out, queryErr := runBatchedPRQuery(fetchCtx, exec, refs)
		out, queryErr = s.absorbMissingReposErr(out, queryErr, cacheScope, repoErrGen)
		if queryErr == nil && explicitRefresh {
			s.enrichBatchedWorkflowAttention(fetchCtx, client, cacheScope, out)
		}
		return out, queryErr
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

// fetchUnwatchedTaskPRsPerPR reads one PR at a time. Passive reads use GetPR:
// the full status helper also lists reviews and check runs (three calls per PR)
// to compute aggregates this path does not write. Explicit reads use the full
// status helper so they can refresh Actions attention as part of a user
// refresh. GetPR is still a full single-pull-request fetch, so
// is_draft/changed_files/merged_by_login are real observations here too
// (AC-10); ClosureAttributionPopulated stays false since neither REST nor the
// gh CLI can see the closing actor (AC-15).
func (s *Service) fetchUnwatchedTaskPRsPerPR(
	ctx context.Context, resolved *resolvedServiceClient, refs []graphQLPRRef, explicitRefresh bool,
) map[string]*PRStatus {
	out := make(map[string]*PRStatus, len(refs))
	statusCtx := ctx
	if explicitRefresh {
		statusCtx = withWorkflowAttentionCollector(ctx, func(
			collectorCtx context.Context, collectorClient Client, owner, repo string, pr *PR,
		) (*WorkflowAttention, error) {
			return s.collectWorkflowAttention(
				collectorCtx, collectorClient, resolved.CacheScope, owner, repo, pr,
			)
		})
	}
	for _, ref := range refs {
		var (
			status *PRStatus
			err    error
		)
		if explicitRefresh {
			status, err = resolved.Client.GetPRStatus(statusCtx, ref.Owner, ref.Repo, ref.Number)
		} else {
			var pr *PR
			pr, err = resolved.Client.GetPR(ctx, ref.Owner, ref.Repo, ref.Number)
			if pr != nil {
				status = &PRStatus{PR: pr, OutcomeFieldsPopulated: true}
			}
		}
		if err != nil {
			s.logger.Debug("per-PR unwatched task PR read failed",
				zap.String("owner", ref.Owner), zap.String("repo", ref.Repo),
				zap.Int("pr_number", ref.Number), zap.Error(err))
			continue
		}
		if status != nil && status.PR != nil {
			out[prStatusCacheKey(ref.Owner, ref.Repo, ref.Number)] = status
		}
	}
	return out
}

// reconcileTaskUnwatchedPRs refreshes the task's unwatched rows and returns
// the reloaded row set, so the WS caller hands the frontend the merged state
// rather than the pre-sync snapshot. Returns the rows unchanged when nothing
// needs reconciling.
func (s *Service) reconcileTaskUnwatchedPRs(
	ctx context.Context, taskID string, watches []*PRWatch, explicitRefresh bool,
) ([]*TaskPR, error) {
	rows, err := s.store.ListTaskPRsByTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("list task PRs: %w", err)
	}
	pending := unwatchedTaskPRs(rows, watches, time.Now().UTC(), explicitRefresh)
	if len(pending) == 0 {
		return rows, nil
	}
	fallbackWorkspaceID := ""
	if len(watches) > 0 && watches[0] != nil {
		fallbackWorkspaceID = watches[0].WorkspaceID
	}
	s.syncUnwatchedTaskPRs(ctx, pending, fallbackWorkspaceID, explicitRefresh)
	return s.store.ListTaskPRsByTask(ctx, taskID)
}

// collectStaleWorkspaceSyncTargets reads the watches and the unwatched
// non-terminal PR rows for every stale task in one pass, so the background
// workspace refresh can fan both into a single batched call each.
func (s *Service) collectStaleWorkspaceSyncTargets(
	ctx context.Context, workspaceID string, staleTasks map[string]struct{},
) ([]*PRWatch, []*TaskPR) {
	var allWatches []*PRWatch
	var pending []*TaskPR
	now := s.now()
	seenWatches := make(map[string]struct{})
	appendWatch := func(watch *PRWatch) {
		if watch == nil {
			return
		}
		key := watch.ID
		if key == "" {
			key = watch.WorkspaceID + "\x00" + watch.TaskID + "\x00" + watch.RepositoryID + "\x00" + watch.Owner + "\x00" + watch.Repo + "\x00" + watch.Branch + "\x00" + fmt.Sprint(watch.PRNumber)
		}
		if _, ok := seenWatches[key]; ok {
			return
		}
		seenWatches[key] = struct{}{}
		allWatches = append(allWatches, watch)
	}
	for taskID := range staleTasks {
		watches, err := s.store.ListPRWatchesByTask(ctx, taskID)
		if err != nil {
			s.logger.Debug("list PR watches for refresh failed",
				zap.String("task_id", taskID), zap.Error(err))
			continue
		}
		for _, watch := range watches {
			appendWatch(watch)
		}
		rows, err := s.store.ListTaskPRsByTask(ctx, taskID)
		if err != nil {
			s.logger.Debug("list task PRs for refresh failed",
				zap.String("task_id", taskID), zap.Error(err))
			continue
		}
		pending = append(pending, unwatchedTaskPRs(rows, watches, now, false)...)
	}
	// Searching watches can be absent from the task-PR projection or can have
	// a fresh cached row while still needing their adaptive discovery check.
	// Load them from the active workspace inventory so passive page refreshes
	// cannot bypass the schedule through the PR freshness window.
	activeWatches, err := s.store.ListActivePRWatchesForWorkspace(ctx, workspaceID)
	if err != nil {
		s.logger.Debug("list active PR watches for adaptive refresh failed", zap.Error(err))
	} else {
		for _, watch := range activeWatches {
			if watch != nil && watch.PRNumber == 0 {
				appendWatch(watch)
			}
		}
	}
	return allWatches, pending
}
