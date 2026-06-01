package github

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// PRWatchSyncResult is the per-watch outcome from SyncWatchesBatched.
// Callers (poller, on-demand sync) can post-process — e.g. publish
// PRFeedback events — without re-fetching from GitHub.
type PRWatchSyncResult struct {
	Watch   *PRWatch
	Status  *PRStatus // nil when no PR was found for a searching watch, or alias missing
	Found   bool      // true when status applied (PR data exists)
	Changed bool      // numbered watches only: true if checks/review state moved
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
// fails so the caller can fall back to per-watch checks.
func (s *Service) fetchBatchedWatchStatuses(
	ctx context.Context, exec GraphQLExecutor, numbered, searching []*PRWatch,
) (map[string]*PRStatus, error) {
	combined := make(map[string]*PRStatus, len(numbered)+len(searching))
	if len(numbered) > 0 {
		refs := make([]graphQLPRRef, 0, len(numbered))
		for _, w := range numbered {
			refs = append(refs, graphQLPRRef{Owner: w.Owner, Repo: w.Repo, Number: w.PRNumber})
		}
		out, err := runBatchedPRQuery(ctx, exec, refs)
		if err != nil {
			return nil, fmt.Errorf("batched PR query: %w", err)
		}
		for k, v := range out {
			combined[k] = v
		}
	}
	if len(searching) > 0 {
		refs := make([]graphQLBranchRef, 0, len(searching))
		for _, w := range searching {
			refs = append(refs, graphQLBranchRef{Owner: w.Owner, Repo: w.Repo, Branch: w.Branch})
		}
		out, err := runBatchedBranchQuery(ctx, exec, refs)
		if err != nil {
			return nil, fmt.Errorf("batched branch query: %w", err)
		}
		for k, v := range out {
			combined[k] = v
		}
	}
	return combined, nil
}

// applyBatchedNumberedWatch mirrors Poller.applyPRStatus on the service
// side so on-demand callers reuse the same DB-write sequence.
func (s *Service) applyBatchedNumberedWatch(
	ctx context.Context, w *PRWatch, statusByKey map[string]*PRStatus, now time.Time,
) PRWatchSyncResult {
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
	if syncErr := s.SyncTaskPR(ctx, w.TaskID, status); syncErr != nil {
		s.logger.Error("failed to sync task PR", zap.String("task_id", w.TaskID), zap.Error(syncErr))
		return PRWatchSyncResult{Watch: w, Status: status, Found: true, Changed: changed}
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
	status, ok := statusByKey[graphqlBranchKey(w.Owner, w.Repo, w.Branch)]
	if !ok || status == nil || status.PR == nil {
		_ = s.store.UpdatePRWatchTimestamps(ctx, w.ID, now, nil, "", "")
		return PRWatchSyncResult{Watch: w}
	}
	_ = s.store.UpdatePRWatchTimestamps(ctx, w.ID, now, nil, "", "")
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
