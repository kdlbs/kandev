package github

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// --- Review-task cleanup ---

// CleanupMergedReviewTasks checks PRs tracked by a review watch and deletes
// tasks whose PRs are merged/closed. Returns the number of tasks deleted.
func (s *Service) CleanupMergedReviewTasks(ctx context.Context, watch *ReviewWatch) (int, error) {
	if s.client == nil || s.taskDeleter == nil {
		return 0, nil
	}
	prTasks, err := s.store.ListReviewPRTasksByWatch(ctx, watch.ID)
	if err != nil {
		return 0, fmt.Errorf("list review PR tasks: %w", err)
	}
	policy := NormalizeCleanupPolicy(watch.CleanupPolicy)
	return s.cleanupReviewPRTaskBatch(ctx, prTasks, func(_ *ReviewPRTask) string { return policy }), nil
}

// CleanupAllOrphanedReviewTasks sweeps dedup rows whose watch is deleted or
// disabled. The per-watch poller loop already processes enabled-watch rows
// in the same cycle, so re-walking them here would double GitHub API
// consumption (the GetPRFeedback path bypasses prStatusCache). Returns the
// number of tasks deleted.
func (s *Service) CleanupAllOrphanedReviewTasks(ctx context.Context) (int, error) {
	return s.cleanupAllReviewTasks(ctx, true)
}

// CleanupAllReviewTasks sweeps every dedup row across all review watches,
// including rows owned by currently-enabled watches. Used by the manual
// settings-page cleanup button so the user can drain everything on demand
// without waiting for the next 5-minute poll cycle.
func (s *Service) CleanupAllReviewTasks(ctx context.Context) (int, error) {
	return s.cleanupAllReviewTasks(ctx, false)
}

// cleanupAllReviewTasks is the shared body. When orphansOnly is true, rows
// whose watch is currently enabled are skipped (the per-watch poller will
// handle them in the same cycle).
//
//nolint:dupl // mirrors cleanupAllIssueTasks — different types, same orchestration
func (s *Service) cleanupAllReviewTasks(ctx context.Context, orphansOnly bool) (int, error) {
	if s.client == nil || s.taskDeleter == nil {
		return 0, nil
	}
	prTasks, err := s.store.ListAllReviewPRTasks(ctx)
	if err != nil {
		return 0, fmt.Errorf("list all review PR tasks: %w", err)
	}
	if len(prTasks) == 0 {
		return 0, nil
	}
	policyCache, enabledCache, unknownCache := s.buildReviewWatchCaches(ctx, prTasks)
	// Allocate a fresh slice rather than reusing prTasks' backing array
	// via prTasks[:0]; the in-place version is safe today (write index
	// always trails read index) but silently breaks if a future caller
	// reads prTasks after the filter.
	candidates := make([]*ReviewPRTask, 0, len(prTasks))
	for _, rpt := range prTasks {
		// Watch fetch failed — skip this cycle so we don't fail-open and
		// reap an enabled-watch row under the wrong policy.
		if unknownCache[rpt.ReviewWatchID] {
			continue
		}
		// Orphan-only path skips rows owned by an enabled watch; the
		// per-watch loop already handled them in the same cycle.
		if orphansOnly && enabledCache[rpt.ReviewWatchID] {
			continue
		}
		candidates = append(candidates, rpt)
	}
	if len(candidates) == 0 {
		return 0, nil
	}
	return s.cleanupReviewPRTaskBatch(ctx, candidates, func(rpt *ReviewPRTask) string {
		if p, ok := policyCache[rpt.ReviewWatchID]; ok {
			return p
		}
		// Watch was deleted: fall back to the documented default so the row
		// (and any task it points at) can still be reaped.
		return CleanupPolicyAuto
	}), nil
}

// buildReviewWatchCaches loads the cleanup policy + enabled flag for each
// distinct watch ID referenced by prTasks. A per-row fetch error logs Warn
// and adds the watch ID to the returned `unknown` set — the caller MUST
// skip those rows this cycle. Without that signal a transient DB hiccup
// would silently fail-open: rows for the failed watch would be treated as
// orphaned and reaped under the fallback auto policy, potentially losing
// tasks the user wanted preserved under `never`. The next sweep cycle
// retries the fetch and recovers naturally. Missing watches (deleted) are
// distinct: they're absent from both `policy` and `enabled` but NOT in
// `unknown`, so callers treat them as legitimate orphans.
func (s *Service) buildReviewWatchCaches(ctx context.Context, prTasks []*ReviewPRTask) (policy map[string]string, enabled map[string]bool, unknown map[string]bool) {
	seen := make(map[string]struct{})
	for _, rpt := range prTasks {
		seen[rpt.ReviewWatchID] = struct{}{}
	}
	policy = make(map[string]string, len(seen))
	enabled = make(map[string]bool, len(seen))
	unknown = make(map[string]bool)
	for watchID := range seen {
		watch, err := s.store.GetReviewWatch(ctx, watchID)
		if err != nil {
			s.logger.Warn("failed to fetch review watch during orphan sweep",
				zap.String("watch_id", watchID), zap.Error(err))
			unknown[watchID] = true
			continue
		}
		if watch == nil {
			continue
		}
		policy[watchID] = NormalizeCleanupPolicy(watch.CleanupPolicy)
		enabled[watchID] = watch.Enabled
	}
	return policy, enabled, unknown
}

// deleteTaskWithReason deletes a task, threading the cleanup reason through to
// the task.deleted event when the wired deleter supports it (see
// TaskDeleterWithReason). Falls back to plain DeleteTask otherwise.
func (s *Service) deleteTaskWithReason(ctx context.Context, taskID, reason string) error {
	if d, ok := s.taskDeleter.(TaskDeleterWithReason); ok {
		return d.DeleteTaskWithReason(ctx, taskID, reason)
	}
	return s.taskDeleter.DeleteTask(ctx, taskID)
}

// cleanupReviewPRTaskBatch runs the deletion gate over a slice of dedup rows.
// resolvePolicy returns the effective cleanup policy for each row; callers
// supply it so per-watch and global-sweep paths can share this body.
//
//nolint:dupl // mirrors cleanupIssueTaskBatch — different types, same structure
func (s *Service) cleanupReviewPRTaskBatch(ctx context.Context, prTasks []*ReviewPRTask, resolvePolicy func(*ReviewPRTask) string) int {
	deleted := 0
	for _, rpt := range prTasks {
		policy := resolvePolicy(rpt)
		// Orphan reservation: process was killed after ReserveReviewPRTask
		// succeeded but before AssignReviewPRTaskID ran, so task_id is empty
		// and there is no task to delete. Clean up the dedup row once the PR
		// reaches a terminal state, same gating as the normal path.
		if rpt.TaskID == "" {
			// Orphan reservation row — no task was ever created (process
			// crashed between Reserve and Assign). Clean it up but DON'T
			// increment `deleted`: the count is reported back to the
			// settings-page toast as "Deleted N tasks", and these rows
			// never had an associated task.
			if should, _ := s.shouldDeleteReviewTask(ctx, rpt, policy); should {
				if err := s.store.DeleteReviewPRTask(ctx, rpt.ID); err != nil {
					s.logger.Warn("failed to delete orphan reservation row",
						zap.String("dedup_id", rpt.ID), zap.Error(err))
				}
			}
			continue
		}
		shouldDelete, reason := s.shouldDeleteReviewTask(ctx, rpt, policy)
		if !shouldDelete {
			continue
		}
		if err := s.deleteTaskWithReason(ctx, rpt.TaskID, reason); err != nil {
			if isTaskNotFound(err) {
				// Task already deleted; clean up the orphaned dedup record.
				if err := s.store.DeleteReviewPRTask(ctx, rpt.ID); err != nil {
					s.logger.Warn("failed to delete orphan dedup row after task-not-found",
						zap.String("dedup_id", rpt.ID), zap.Error(err))
					continue
				}
				deleted++
				continue
			}
			s.logger.Warn("failed to delete review PR task",
				zap.String("task_id", rpt.TaskID), zap.Error(err))
			continue
		}
		if err := s.store.DeleteReviewPRTask(ctx, rpt.ID); err != nil {
			// Task is gone but dedup row survived: log Warn and DON'T
			// increment deleted (the next sweep cycle will retry and the
			// settings-page toast stays accurate).
			s.logger.Warn("deleted task but failed to remove dedup row",
				zap.String("task_id", rpt.TaskID),
				zap.String("dedup_id", rpt.ID), zap.Error(err))
			continue
		}
		s.logger.Info("deleted review task",
			zap.String("task_id", rpt.TaskID),
			zap.String("reason", reason),
			zap.String("policy", policy),
			zap.Int("pr_number", rpt.PRNumber),
			zap.String("repo", rpt.RepoOwner+"/"+rpt.RepoName))
		deleted++
	}
	return deleted
}

// shouldDeleteReviewTask checks whether a review PR task is eligible for
// cleanup under the supplied policy. Returns true + a short reason on hit.
//   - CleanupPolicyNever  → always false.
//   - CleanupPolicyAlways → terminal state alone is enough.
//   - CleanupPolicyAuto   → terminal state + no user-authored messages.
//
// Terminal state covers: PR merged/closed, OR the authenticated user already
// approved the PR on GitHub (so it's effectively done from their POV).
func (s *Service) shouldDeleteReviewTask(ctx context.Context, rpt *ReviewPRTask, policy string) (bool, string) {
	if policy == CleanupPolicyNever {
		return false, ""
	}
	failureKey := reviewFailureKey(rpt)
	feedback, err := s.client.GetPRFeedback(ctx, rpt.RepoOwner, rpt.RepoName, rpt.PRNumber)
	if err != nil {
		s.trackCleanupFailure(failureKey, "review", rpt.RepoOwner+"/"+rpt.RepoName, rpt.PRNumber, err)
		return false, ""
	}
	s.resetCleanupFailure(failureKey)
	if feedback.PR == nil {
		return false, ""
	}
	var reason string
	if feedback.PR.State == prStateMerged || feedback.PR.State == prStateClosed {
		reason = "pr_merged_or_closed" //nolint:goconst // also referenced by string in service_cleanup_policy_test.go; introducing a constant would require coupling the test
	} else {
		// Check if the authenticated user already approved the PR on GitHub.
		user, _ := s.client.GetAuthenticatedUser(ctx)
		for _, review := range feedback.Reviews {
			if review.State == "APPROVED" && review.Author == user {
				reason = "pr_approved_by_user"
				break
			}
		}
	}
	if reason == "" {
		return false, ""
	}
	if policy == CleanupPolicyAlways || rpt.TaskID == "" || s.taskSessionChecker == nil {
		return true, reason
	}
	hasUserMsg, err := s.taskSessionChecker.HasUserAuthoredMessage(ctx, rpt.TaskID)
	if err != nil {
		s.logger.Debug("failed to check task user messages",
			zap.String("task_id", rpt.TaskID), zap.Error(err))
		return false, ""
	}
	if hasUserMsg {
		return false, ""
	}
	return true, reason
}

// reviewFailureKey builds the stable per-row identifier used for failure
// tracking. Stable across polls so consecutive errors increment the same
// counter and a recovery resets it cleanly. Includes the watch ID so two
// watches monitoring the same (owner, repo, PR) don't collide and reset
// each other's failure counters, suppressing the threshold-crossing Warn.
func reviewFailureKey(rpt *ReviewPRTask) string {
	return fmt.Sprintf("review:%s:%s/%s#%d", rpt.ReviewWatchID, rpt.RepoOwner, rpt.RepoName, rpt.PRNumber)
}

// trackCleanupFailure increments the failure counter for key and emits a
// Warn log once the consecutive-failure threshold is crossed. Below the
// threshold the failure is recorded at Debug so the normal log isn't flooded.
func (s *Service) trackCleanupFailure(key, kind, repo string, number int, cause error) {
	s.cleanupFailureMu.Lock()
	s.cleanupFailureCounts[key]++
	n := s.cleanupFailureCounts[key]
	s.cleanupFailureMu.Unlock()
	if n < cleanupFetchFailureThreshold {
		s.logger.Debug("cleanup state fetch failed",
			zap.String("kind", kind),
			zap.String("repo", repo),
			zap.Int("number", number),
			zap.Int("consecutive_failures", n),
			zap.Error(cause))
		return
	}
	s.logger.Warn("cleanup state fetch failing repeatedly — blocked task deletion",
		zap.String("kind", kind),
		zap.String("repo", repo),
		zap.Int("number", number),
		zap.Int("consecutive_failures", n),
		zap.Error(cause))
}

// resetCleanupFailure drops the counter for key once the upstream fetch
// recovers, so a future flap doesn't cross the threshold prematurely.
func (s *Service) resetCleanupFailure(key string) {
	s.cleanupFailureMu.Lock()
	delete(s.cleanupFailureCounts, key)
	s.cleanupFailureMu.Unlock()
}

// --- Issue-task cleanup ---

// CleanupClosedIssueTasks checks issues tracked by a watch and deletes
// tasks whose issues are closed under the watch's cleanup policy.
//
//nolint:dupl // mirrors CleanupMergedReviewTasks — different types, same structure
func (s *Service) CleanupClosedIssueTasks(ctx context.Context, watch *IssueWatch) (int, error) {
	if s.client == nil || s.taskDeleter == nil {
		return 0, nil
	}
	issueTasks, err := s.store.ListIssueWatchTasksByWatch(ctx, watch.ID)
	if err != nil {
		return 0, fmt.Errorf("list issue watch tasks: %w", err)
	}
	policy := NormalizeCleanupPolicy(watch.CleanupPolicy)
	return s.cleanupIssueTaskBatch(ctx, issueTasks, func(_ *IssueWatchTask) string { return policy }), nil
}

// CleanupAllOrphanedIssueTasks sweeps dedup rows whose watch is deleted or
// disabled (mirrors CleanupAllOrphanedReviewTasks).
func (s *Service) CleanupAllOrphanedIssueTasks(ctx context.Context) (int, error) {
	return s.cleanupAllIssueTasks(ctx, true)
}

// CleanupAllIssueTasks sweeps every dedup row across all issue watches for
// the manual settings-page button.
func (s *Service) CleanupAllIssueTasks(ctx context.Context) (int, error) {
	return s.cleanupAllIssueTasks(ctx, false)
}

//nolint:dupl // mirrors cleanupAllReviewTasks — different types, same orchestration
func (s *Service) cleanupAllIssueTasks(ctx context.Context, orphansOnly bool) (int, error) {
	if s.client == nil || s.taskDeleter == nil {
		return 0, nil
	}
	issueTasks, err := s.store.ListAllIssueWatchTasks(ctx)
	if err != nil {
		return 0, fmt.Errorf("list all issue watch tasks: %w", err)
	}
	if len(issueTasks) == 0 {
		return 0, nil
	}
	policyCache, enabledCache, unknownCache := s.buildIssueWatchCaches(ctx, issueTasks)
	candidates := make([]*IssueWatchTask, 0, len(issueTasks))
	for _, it := range issueTasks {
		if unknownCache[it.IssueWatchID] {
			continue
		}
		if orphansOnly && enabledCache[it.IssueWatchID] {
			continue
		}
		candidates = append(candidates, it)
	}
	if len(candidates) == 0 {
		return 0, nil
	}
	return s.cleanupIssueTaskBatch(ctx, candidates, func(it *IssueWatchTask) string {
		if p, ok := policyCache[it.IssueWatchID]; ok {
			return p
		}
		return CleanupPolicyAuto
	}), nil
}

func (s *Service) buildIssueWatchCaches(ctx context.Context, issueTasks []*IssueWatchTask) (policy map[string]string, enabled map[string]bool, unknown map[string]bool) {
	seen := make(map[string]struct{})
	for _, it := range issueTasks {
		seen[it.IssueWatchID] = struct{}{}
	}
	policy = make(map[string]string, len(seen))
	enabled = make(map[string]bool, len(seen))
	unknown = make(map[string]bool)
	for watchID := range seen {
		watch, err := s.store.GetIssueWatch(ctx, watchID)
		if err != nil {
			s.logger.Warn("failed to fetch issue watch during orphan sweep",
				zap.String("watch_id", watchID), zap.Error(err))
			unknown[watchID] = true
			continue
		}
		if watch == nil {
			continue
		}
		policy[watchID] = NormalizeCleanupPolicy(watch.CleanupPolicy)
		enabled[watchID] = watch.Enabled
	}
	return policy, enabled, unknown
}

//nolint:dupl // mirrors cleanupReviewPRTaskBatch — different types, same structure
func (s *Service) cleanupIssueTaskBatch(ctx context.Context, issueTasks []*IssueWatchTask, resolvePolicy func(*IssueWatchTask) string) int {
	deleted := 0
	for _, it := range issueTasks {
		policy := resolvePolicy(it)
		if it.TaskID == "" {
			// Orphan reservation row — no task was created. Clean it up
			// but don't count it as a deleted task.
			if should, _ := s.shouldDeleteIssueTask(ctx, it, policy); should {
				if err := s.store.DeleteIssueWatchTask(ctx, it.ID); err != nil {
					s.logger.Warn("failed to delete orphan reservation row",
						zap.String("dedup_id", it.ID), zap.Error(err))
				}
			}
			continue
		}
		shouldDelete, reason := s.shouldDeleteIssueTask(ctx, it, policy)
		if !shouldDelete {
			continue
		}
		if err := s.deleteTaskWithReason(ctx, it.TaskID, reason); err != nil {
			if isTaskNotFound(err) {
				if err := s.store.DeleteIssueWatchTask(ctx, it.ID); err != nil {
					s.logger.Warn("failed to delete orphan dedup row after task-not-found",
						zap.String("dedup_id", it.ID), zap.Error(err))
					continue
				}
				deleted++
				continue
			}
			s.logger.Warn("failed to delete issue task",
				zap.String("task_id", it.TaskID), zap.Error(err))
			continue
		}
		if err := s.store.DeleteIssueWatchTask(ctx, it.ID); err != nil {
			s.logger.Warn("deleted task but failed to remove dedup row",
				zap.String("task_id", it.TaskID),
				zap.String("dedup_id", it.ID), zap.Error(err))
			continue
		}
		s.logger.Info("deleted issue task",
			zap.String("task_id", it.TaskID),
			zap.String("reason", reason),
			zap.String("policy", policy),
			zap.Int("issue_number", it.IssueNumber),
			zap.String("repo", it.RepoOwner+"/"+it.RepoName))
		deleted++
	}
	return deleted
}

// shouldDeleteIssueTask checks whether an issue task is eligible for cleanup.
// Policy gating mirrors shouldDeleteReviewTask.
func (s *Service) shouldDeleteIssueTask(ctx context.Context, it *IssueWatchTask, policy string) (bool, string) {
	if policy == CleanupPolicyNever {
		return false, ""
	}
	failureKey := issueFailureKey(it)
	state, err := s.client.GetIssueState(ctx, it.RepoOwner, it.RepoName, it.IssueNumber)
	if err != nil {
		s.trackCleanupFailure(failureKey, "issue", it.RepoOwner+"/"+it.RepoName, it.IssueNumber, err)
		return false, ""
	}
	s.resetCleanupFailure(failureKey)
	if state != "closed" {
		return false, ""
	}
	reason := "issue_closed"
	if policy == CleanupPolicyAlways || it.TaskID == "" || s.taskSessionChecker == nil {
		return true, reason
	}
	hasUserMsg, err := s.taskSessionChecker.HasUserAuthoredMessage(ctx, it.TaskID)
	if err != nil {
		s.logger.Debug("failed to check task user messages",
			zap.String("task_id", it.TaskID), zap.Error(err))
		return false, ""
	}
	if hasUserMsg {
		return false, ""
	}
	return true, reason
}

func issueFailureKey(it *IssueWatchTask) string {
	return fmt.Sprintf("issue:%s:%s/%s#%d", it.IssueWatchID, it.RepoOwner, it.RepoName, it.IssueNumber)
}
