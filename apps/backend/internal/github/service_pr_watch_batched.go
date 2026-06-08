package github

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
)

// PRWatchSyncResult is the per-watch outcome from SyncWatchesBatched.
// Callers (poller, on-demand sync) can post-process — e.g. publish
// PRFeedback events — without re-fetching from GitHub.
type PRWatchSyncResult struct {
	Watch      *PRWatch
	Status     *PRStatus // nil when no PR was found for a searching watch, or alias missing
	Found      bool      // true when status applied (PR data exists)
	Changed    bool      // numbered watches only: true if checks/review state moved
	SyncFailed bool      // true when SyncTaskPR returned an error — callers must NOT publish events
}

// SyncWatchesBatched runs the batched GraphQL queries for the supplied
// watches and applies the resulting DB updates: timestamps, task PR sync,
// watch PR-number promotion on detection, watch reset on merge/close.
// Returns per-watch results so callers can post-process (event publishing).
//
// Returns an error when the batched fetch itself fails — the caller should
// fall back to per-watch checks rather than silently dropping a poll cycle.
// Errors from the per-watch DB applies are logged but do not abort the
// loop, matching the per-watch path's best-effort semantics.
//
// This is the single seam both the 1-minute poller and the on-demand
// TriggerPRSyncAll / ListWorkspaceTaskPRs background refresh share, so a
// 40-watch workspace fans out to ~2 gh subprocess calls instead of 40.
func (s *Service) SyncWatchesBatched(ctx context.Context, watches []*PRWatch) ([]PRWatchSyncResult, error) {
	if len(watches) == 0 {
		return nil, nil
	}
	exec, err := graphQLExecutorFor(s.client)
	if err != nil {
		return nil, err
	}
	numbered, searching := splitPRWatches(watches)
	statusByKey, err := s.fetchBatchedWatchStatuses(ctx, exec, numbered, searching)
	if err != nil {
		return nil, err
	}

	results := make([]PRWatchSyncResult, 0, len(watches))
	now := time.Now().UTC()
	for _, w := range numbered {
		results = append(results, s.applyBatchedNumberedWatch(ctx, w, statusByKey, now))
	}
	for _, w := range searching {
		results = append(results, s.applyBatchedSearchingWatch(ctx, w, statusByKey, now))
	}
	return results, nil
}

// fetchBatchedWatchStatuses runs the numbered- and branch-keyed GraphQL
// queries and merges their results. Returns an error when either query
// fails so the caller can fall back to per-watch checks. Watches whose
// (owner, repo) is already cached as missing are filtered out before
// hitting GraphQL, and any newly-discovered missing repos in the
// response are fed into the negative cache so subsequent polls
// short-circuit before acquiring the gh throttle.
//
// The full fetch is gated by a service-level singleflight keyed by the
// sorted ref set. Without it, a burst of concurrent SyncWatchesBatched
// calls (poller racing the WS sync racing the workspace background
// refresh) all hit GraphQL during the window before the first caller
// seeds the negative cache — exactly the storm this change is trying to
// calm. The shared-result map is read-only at the call sites (only
// map lookups in applyBatched*), so it's safe to share.
//
// The upstream fetch detaches from the leader's cancellation via
// derivedFetchContext so one caller (WS) disconnecting mid-flight
// doesn't cascade context.Canceled to all co-waiters — same pattern as
// GetPRFeedback / GetPRStatus. The leader's deadline is preserved so
// the fetch can't outlive the request budget.
func (s *Service) fetchBatchedWatchStatuses(
	ctx context.Context, exec GraphQLExecutor, numbered, searching []*PRWatch,
) (map[string]*PRStatus, error) {
	key := batchedFetchSingleflightKey(numbered, searching)
	fetchCtx, cancelFetch := derivedFetchContext(ctx)
	defer cancelFetch()
	v, err, _ := s.syncGroup.Do(key, func() (interface{}, error) {
		// Snapshot the negative-cache generation BEFORE the batched
		// fetch so an eviction (relink / clear) firing while the
		// GraphQL call is in flight wins over the post-fetch
		// markRepoAsMissing writes — see Service.markRepoAsMissing.
		repoErrGen := s.repoErrorGenSnapshot()
		combined := make(map[string]*PRStatus, len(numbered)+len(searching))
		if err := s.fetchBatchedPRStatuses(fetchCtx, exec, numbered, combined, repoErrGen); err != nil {
			return nil, err
		}
		if err := s.fetchBatchedBranchStatuses(fetchCtx, exec, searching, combined, repoErrGen); err != nil {
			return nil, err
		}
		return combined, nil
	})
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	return v.(map[string]*PRStatus), nil
}

// batchedFetchSingleflightKey builds a deterministic key from the sorted
// ref tuples in (numbered, searching). Sorting ensures ordering doesn't
// split equivalent calls into separate slots, and lowercasing matches
// repoErrorCacheKey so the in-flight key is stable regardless of caller
// casing (watches from the DB always have consistent casing in practice,
// but it costs nothing to make the key insensitive). Distinct "n:"/"b:"
// prefixes prevent a numbered ref colliding with a branch ref that
// happens to render the same string.
func batchedFetchSingleflightKey(numbered, searching []*PRWatch) string {
	parts := make([]string, 0, len(numbered)+len(searching))
	for _, w := range numbered {
		parts = append(parts, fmt.Sprintf("n:%s/%s#%d",
			strings.ToLower(w.Owner), strings.ToLower(w.Repo), w.PRNumber))
	}
	for _, w := range searching {
		parts = append(parts, fmt.Sprintf("b:%s/%s@%s",
			strings.ToLower(w.Owner), strings.ToLower(w.Repo), w.Branch))
	}
	sort.Strings(parts)
	return "batched-fetch:" + strings.Join(parts, "|")
}

// fetchBatchedPRStatuses runs the numbered-watch (PR-number-keyed) batch.
// Watches whose repo is in the negative cache are dropped before building
// the GraphQL refs so the dead-repo storm doesn't burn gh throttle slots.
// Missing repos surfaced by the response are negative-cached for the next
// 10 minutes; partial results for the repos that did resolve are merged
// into `combined`.
func (s *Service) fetchBatchedPRStatuses(
	ctx context.Context, exec GraphQLExecutor, numbered []*PRWatch, combined map[string]*PRStatus,
	repoErrGen uint64,
) error {
	refs := make([]graphQLPRRef, 0, len(numbered))
	for _, w := range numbered {
		if s.isRepoCachedAsMissing(w.Owner, w.Repo) {
			continue
		}
		refs = append(refs, graphQLPRRef{Owner: w.Owner, Repo: w.Repo, Number: w.PRNumber})
	}
	if len(refs) == 0 {
		return nil
	}
	out, err := runBatchedPRQuery(ctx, exec, refs)
	out, err = s.absorbMissingReposErr(out, err, repoErrGen)
	if err != nil {
		return fmt.Errorf("batched PR query: %w", err)
	}
	for k, v := range out {
		combined[k] = v
	}
	return nil
}

// fetchBatchedBranchStatuses runs the branch-keyed batch with the same
// negative-cache filter and missing-repo absorption as the numbered path.
func (s *Service) fetchBatchedBranchStatuses(
	ctx context.Context, exec GraphQLExecutor, searching []*PRWatch, combined map[string]*PRStatus,
	repoErrGen uint64,
) error {
	refs := make([]graphQLBranchRef, 0, len(searching))
	for _, w := range searching {
		if s.isRepoCachedAsMissing(w.Owner, w.Repo) {
			continue
		}
		refs = append(refs, graphQLBranchRef{Owner: w.Owner, Repo: w.Repo, Branch: w.Branch})
	}
	if len(refs) == 0 {
		return nil
	}
	out, err := runBatchedBranchQuery(ctx, exec, refs)
	out, err = s.absorbMissingReposErr(out, err, repoErrGen)
	if err != nil {
		return fmt.Errorf("batched branch query: %w", err)
	}
	for k, v := range out {
		combined[k] = v
	}
	return nil
}

// absorbMissingReposErr extracts the negative-cacheable repos from a
// runBatched* result and seeds the cache, then returns (partial,
// remaining-err). When the error was PURELY missing-repo errors, the
// partial decode is preserved and a nil error is returned so the caller
// can use the resolved repos' data. When other errors were also
// present, the inner error is returned unchanged so the caller falls
// back to per-watch checks. `repoErrGen` is the negative-cache generation
// snapshot taken BEFORE the fetch so a concurrent eviction wins.
func (s *Service) absorbMissingReposErr(out map[string]*PRStatus, err error, repoErrGen uint64) (map[string]*PRStatus, error) {
	if err == nil {
		return out, nil
	}
	var missingErr *batchedMissingReposErr
	if !errors.As(err, &missingErr) {
		return nil, err
	}
	for _, r := range missingErr.Repos {
		s.markRepoAsMissing(r.Owner, r.Repo, repoErrGen)
		s.logger.Debug("repo not resolvable; negative-cached",
			zap.String("owner", r.Owner), zap.String("repo", r.Repo))
	}
	if missingErr.Inner != nil {
		return nil, missingErr.Inner
	}
	return out, nil
}

// applyBatchedNumberedWatch mirrors Poller.applyPRStatus on the service
// side so on-demand callers reuse the same DB-write sequence.
func (s *Service) applyBatchedNumberedWatch(
	ctx context.Context, w *PRWatch, statusByKey map[string]*PRStatus, now time.Time,
) PRWatchSyncResult {
	// Repo is in the 10-min negative cache — the fetch path didn't include
	// it in the GraphQL batch, so the apply path can't probe upstream either.
	// Bump last_checked_at to suppress retry storms and surface SyncFailed
	// so the WS handler can flag the result as permanent for the frontend.
	if s.isRepoCachedAsMissing(w.Owner, w.Repo) {
		_ = s.store.UpdatePRWatchTimestamps(ctx, w.ID, now, nil, "", "")
		return PRWatchSyncResult{Watch: w, SyncFailed: true}
	}
	status, ok := statusByKey[prStatusCacheKey(w.Owner, w.Repo, w.PRNumber)]
	if !ok || status == nil {
		// Alias missing — best-effort liveness bump so we don't immediately re-probe.
		_ = s.store.UpdatePRWatchTimestamps(ctx, w.ID, now, nil, "", "")
		return PRWatchSyncResult{Watch: w}
	}
	changed := status.ChecksState != w.LastCheckStatus || status.ReviewState != w.LastReviewState
	if err := s.store.UpdatePRWatchTimestamps(ctx, w.ID, now, nil, status.ChecksState, status.ReviewState); err != nil {
		s.logger.Error("failed to update PR watch timestamps", zap.String("id", w.ID), zap.Error(err))
	}
	// Gap-fill: a numbered watch can exist even when its exact task_pr row was
	// never created. This targeted read is unconditional because the common
	// existing-row path is cheap, and the missing-row path must repair before
	// SyncTaskPR. If AssociatePRWithTask creates the row, it publishes the
	// creation event; the following SyncTaskPR may publish a second event when
	// status fields changed. That double event is harmless because clients
	// re-fetch the task PR state.
	if existing, err := s.store.GetTaskPRByRepoAndNumber(ctx, w.TaskID, w.RepositoryID, w.PRNumber); err != nil {
		s.logger.Error("failed to load exact task PR",
			zap.String("task_id", w.TaskID), zap.String("repository_id", w.RepositoryID),
			zap.Int("pr_number", w.PRNumber), zap.Error(err))
		return PRWatchSyncResult{Watch: w, Status: status, Found: true, SyncFailed: true}
	} else if existing == nil && status.PR != nil {
		if _, assocErr := s.AssociatePRWithTask(ctx, w.TaskID, w.RepositoryID, status.PR); assocErr != nil {
			s.logger.Error("failed to associate numbered PR with task",
				zap.String("task_id", w.TaskID), zap.Int("pr_number", w.PRNumber), zap.Error(assocErr))
			return PRWatchSyncResult{Watch: w, Status: status, Found: true, SyncFailed: true}
		}
	}
	if syncErr := s.SyncTaskPR(ctx, w.TaskID, status); syncErr != nil {
		s.logger.Error("failed to sync task PR", zap.String("task_id", w.TaskID), zap.Error(syncErr))
		// SyncFailed=true so poller skips publishing PR feedback while the
		// task_pr row is still stale — old applyPRStatus path early-returned
		// on this error for the same reason.
		return PRWatchSyncResult{Watch: w, Status: status, Found: true, SyncFailed: true}
	}
	// Reset to "searching" when the PR is merged/closed so a follow-up PR on the
	// same branch can be detected without manual intervention.
	if status.PR != nil && (status.PR.State == prStateMerged || status.PR.State == prStateClosed) {
		if resetErr := s.store.UpdatePRWatchPRNumber(ctx, w.ID, 0); resetErr != nil {
			s.logger.Error("failed to reset completed PR watch", zap.String("id", w.ID), zap.Error(resetErr))
		}
	}
	return PRWatchSyncResult{Watch: w, Status: status, Found: true, Changed: changed}
}

// applyBatchedSearchingWatch mirrors Poller.applyDetectedPR on the service
// side. A searching watch (pr_number=0) is promoted to a known PR when the
// branch lookup returns one; otherwise we just bump last_checked_at.
func (s *Service) applyBatchedSearchingWatch(
	ctx context.Context, w *PRWatch, statusByKey map[string]*PRStatus, now time.Time,
) PRWatchSyncResult {
	// Timestamps get bumped on both the "no PR found" and "PR detected"
	// paths, so hoist the single call above the branch to make that
	// invariant obvious.
	_ = s.store.UpdatePRWatchTimestamps(ctx, w.ID, now, nil, "", "")
	if s.isRepoCachedAsMissing(w.Owner, w.Repo) {
		return PRWatchSyncResult{Watch: w, SyncFailed: true}
	}
	status, ok := statusByKey[graphqlBranchKey(w.Owner, w.Repo, w.Branch)]
	if !ok || status == nil || status.PR == nil {
		return PRWatchSyncResult{Watch: w}
	}
	if err := s.store.UpdatePRWatchPRNumber(ctx, w.ID, status.PR.Number); err != nil {
		s.logger.Error("failed to update PR watch with detected PR",
			zap.String("watch_id", w.ID), zap.Int("pr_number", status.PR.Number), zap.Error(err))
		return PRWatchSyncResult{Watch: w, Status: status, Found: true}
	}
	if _, err := s.AssociatePRWithTask(ctx, w.TaskID, w.RepositoryID, status.PR); err != nil {
		s.logger.Error("failed to associate detected PR with task",
			zap.String("task_id", w.TaskID), zap.Int("pr_number", status.PR.Number), zap.Error(err))
		return PRWatchSyncResult{Watch: w, Status: status, Found: true}
	}
	s.logger.Info("detected PR for session branch (batched)",
		zap.String("watch_id", w.ID), zap.String("branch", w.Branch), zap.Int("pr_number", status.PR.Number))
	return PRWatchSyncResult{Watch: w, Status: status, Found: true}
}
