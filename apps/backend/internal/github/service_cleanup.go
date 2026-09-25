package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/kandev/kandev/internal/common/authcircuit"
	"go.uber.org/zap"
)

type scheduledReviewCleanupFailure struct {
	Task         *ReviewPRTask
	WorkspaceID  string
	RecordScoped bool
	Err          error
}

type scheduledReviewCleanupSuccess struct {
	Task        *ReviewPRTask
	WorkspaceID string
}

type scheduledReviewCleanupResult struct {
	Deleted   int
	Failures  []scheduledReviewCleanupFailure
	Successes []scheduledReviewCleanupSuccess
}

type scheduledReviewCleanupAdmission struct {
	workspace func(string) (allow, stopWorkspace bool)
	record    func(*ReviewPRTask, *RateTracker) (allow, stopWorkspace bool)
}

func (a scheduledReviewCleanupAdmission) allowWorkspace(workspaceID string) bool {
	if a.workspace == nil {
		return true
	}
	allowed, _ := a.workspace(workspaceID)
	return allowed
}

func cleanupBatchAdmission(ctx context.Context, client Client, tracker *RateTracker, policy string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if policy == CleanupPolicyNever || tracker == nil {
		return nil
	}
	resource := cleanupRateResource(client)
	if tracker.WaitDuration(resource) <= 0 {
		return nil
	}
	return &GitHubAPIError{
		StatusCode: http.StatusTooManyRequests,
		Endpoint:   "cleanup",
		Body:       fmt.Sprintf("GitHub %s rate limit exhausted", resource),
	}
}

type cleanupRateResourceReporter interface {
	RateResource() Resource
}

func cleanupRateResource(client Client) Resource {
	if reporter, ok := client.(cleanupRateResourceReporter); ok {
		if resource := reporter.RateResource(); resource != "" {
			return resource
		}
	}
	return ResourceCore
}

func cleanupBatchShouldStop(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var apiErr *GitHubAPIError
	if errors.As(err, &apiErr) && isGitHubRateLimitAPIError(apiErr) {
		return true
	}
	// Legacy GHClient calls return the CLI's stderr wrapped in an exec error.
	// Keep the established marker classifier at this boundary so cleanup stops
	// after the first affected row even when no typed API response exists.
	return ghStderrIndicatesRateLimit(err.Error())
}

// --- Review-task cleanup ---

// CleanupMergedReviewTasks checks PRs tracked by a review watch and deletes
// tasks whose PRs are merged/closed. Returns the number of tasks deleted.
func (s *Service) CleanupMergedReviewTasks(ctx context.Context, watch *ReviewWatch) (int, error) {
	if s.taskDeleter == nil {
		return 0, nil
	}
	if watch == nil || watch.WorkspaceID == "" {
		return 0, ErrGitHubWorkspaceRequired
	}
	resolved, err := s.resolveAutomationClient(ctx, watch.WorkspaceID, "", "")
	if err != nil {
		return 0, err
	}
	prTasks, err := s.store.ListReviewPRTasksByWatch(ctx, watch.ID)
	if err != nil {
		return 0, fmt.Errorf("list review PR tasks: %w", err)
	}
	policy := NormalizeCleanupPolicy(watch.CleanupPolicy)
	return s.cleanupReviewPRTaskBatch(ctx, resolved.Client, resolved.RateTracker, prTasks,
		func(_ *ReviewPRTask) string { return policy })
}

func (s *Service) cleanupScheduledMergedReviewTasks(
	ctx context.Context, watch *ReviewWatch, admission scheduledReviewCleanupAdmission,
) (scheduledReviewCleanupResult, error) {
	if s.taskDeleter == nil {
		return scheduledReviewCleanupResult{}, nil
	}
	if watch == nil || watch.WorkspaceID == "" {
		return scheduledReviewCleanupResult{}, ErrGitHubWorkspaceRequired
	}
	prTasks, err := s.store.ListScheduledReviewPRTasksByWatch(ctx, watch.ID)
	if err != nil {
		return scheduledReviewCleanupResult{}, fmt.Errorf("list scheduled review PR tasks: %w", err)
	}
	if len(prTasks) == 0 {
		return scheduledReviewCleanupResult{}, nil
	}
	if !admission.allowWorkspace(watch.WorkspaceID) {
		return scheduledReviewCleanupResult{}, nil
	}
	resolved, err := s.resolveAutomationClient(ctx, watch.WorkspaceID, "", "")
	if err != nil {
		return scheduledReviewCleanupResult{Failures: []scheduledReviewCleanupFailure{{
			Task: prTasks[0], WorkspaceID: watch.WorkspaceID, Err: err,
		}}}, nil
	}
	policy := NormalizeCleanupPolicy(watch.CleanupPolicy)
	result := s.cleanupScheduledReviewPRTaskBatch(ctx, resolved.Client, resolved.RateTracker,
		prTasks, watch.WorkspaceID, func(_ *ReviewPRTask) string { return policy }, admission)
	return result, nil
}

// CleanupAllOrphanedReviewTasks sweeps dedup rows whose watch is deleted or
// disabled. The per-watch poller loop already processes enabled-watch rows
// in the same cycle, so re-walking them here would double GitHub API
// consumption (the GetPRFeedback path bypasses prStatusCache). Returns the
// number of tasks deleted.
func (s *Service) CleanupAllOrphanedReviewTasks(ctx context.Context) (int, error) {
	result, err := s.cleanupAllReviewTasks(ctx, true, false, scheduledReviewCleanupAdmission{})
	return result.Deleted, err
}

func (s *Service) cleanupScheduledOrphanedReviewTasks(
	ctx context.Context, admission scheduledReviewCleanupAdmission,
) (scheduledReviewCleanupResult, error) {
	return s.cleanupAllReviewTasks(ctx, true, true, admission)
}

// CleanupAllReviewTasks sweeps every dedup row across all review watches,
// including rows owned by currently-enabled watches. Used by the manual
// settings-page cleanup button so the user can drain everything on demand
// without waiting for the next 5-minute poll cycle.
func (s *Service) CleanupAllReviewTasks(ctx context.Context) (int, error) {
	result, err := s.cleanupAllReviewTasks(ctx, false, false, scheduledReviewCleanupAdmission{})
	return result.Deleted, err
}

// CleanupReviewTasksForWorkspace sweeps review dedup rows owned by watches in
// one workspace. Rows for deleted watches are intentionally skipped because
// their workspace can no longer be proven.
//
//nolint:dupl // mirrors CleanupIssueTasksForWorkspace — different task/watch types
func (s *Service) CleanupReviewTasksForWorkspace(ctx context.Context, workspaceID string) (int, error) {
	if s.taskDeleter == nil {
		return 0, nil
	}
	resolved, err := s.resolveAutomationClient(ctx, workspaceID, "", "")
	if err != nil {
		return 0, err
	}
	watches, err := s.store.ListReviewWatches(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("list review watches: %w", err)
	}
	watchPolicies := make(map[string]string, len(watches))
	for _, watch := range watches {
		watchPolicies[watch.ID] = NormalizeCleanupPolicy(watch.CleanupPolicy)
	}
	if len(watchPolicies) == 0 {
		return 0, nil
	}
	prTasks, err := s.store.ListAllReviewPRTasks(ctx)
	if err != nil {
		return 0, fmt.Errorf("list all review PR tasks: %w", err)
	}
	candidates := make([]*ReviewPRTask, 0, len(prTasks))
	for _, rpt := range prTasks {
		if _, ok := watchPolicies[rpt.ReviewWatchID]; ok {
			candidates = append(candidates, rpt)
		}
	}
	return s.cleanupReviewPRTaskBatch(ctx, resolved.Client, resolved.RateTracker, candidates, func(rpt *ReviewPRTask) string {
		return watchPolicies[rpt.ReviewWatchID]
	})
}

// cleanupAllReviewTasks is the shared body. When orphansOnly is true, rows
// whose watch is currently enabled are skipped (the per-watch poller will
// handle them in the same cycle).
//
//nolint:dupl // mirrors cleanupAllIssueTasks — different types, same orchestration
func (s *Service) cleanupAllReviewTasks(
	ctx context.Context,
	orphansOnly bool,
	scheduled bool,
	admission scheduledReviewCleanupAdmission,
) (scheduledReviewCleanupResult, error) {
	if s.taskDeleter == nil {
		return scheduledReviewCleanupResult{}, nil
	}
	var (
		prTasks []*ReviewPRTask
		err     error
	)
	if scheduled {
		prTasks, err = s.store.ListScheduledReviewPRTasks(ctx)
	} else {
		prTasks, err = s.store.ListAllReviewPRTasks(ctx)
	}
	if err != nil {
		return scheduledReviewCleanupResult{}, fmt.Errorf("list all review PR tasks: %w", err)
	}
	if len(prTasks) == 0 {
		return scheduledReviewCleanupResult{}, nil
	}
	policyCache, enabledCache, unknownCache, workspaceCache := s.buildReviewWatchCaches(ctx, prTasks)
	// Allocate a fresh slice rather than reusing prTasks' backing array
	// via prTasks[:0]; the in-place version is safe today (write index
	// always trails read index) but silently breaks if a future caller
	// reads prTasks after the filter.
	candidatesByWorkspace := make(map[string][]*ReviewPRTask)
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
		workspaceID := workspaceCache[rpt.ReviewWatchID]
		if workspaceID == "" {
			continue
		}
		candidatesByWorkspace[workspaceID] = append(candidatesByWorkspace[workspaceID], rpt)
	}
	result := scheduledReviewCleanupResult{}
	for workspaceID, candidates := range candidatesByWorkspace {
		workspaceResult, workspaceErr := s.cleanupReviewTasksForWorkspace(
			ctx, workspaceID, candidates, scheduled, admission,
			func(rpt *ReviewPRTask) string { return policyCache[rpt.ReviewWatchID] },
		)
		result.Deleted += workspaceResult.Deleted
		result.Failures = append(result.Failures, workspaceResult.Failures...)
		result.Successes = append(result.Successes, workspaceResult.Successes...)
		if workspaceErr != nil {
			return result, workspaceErr
		}
	}
	return result, nil
}

func (s *Service) cleanupReviewTasksForWorkspace(
	ctx context.Context,
	workspaceID string,
	candidates []*ReviewPRTask,
	scheduled bool,
	admission scheduledReviewCleanupAdmission,
	resolvePolicy func(*ReviewPRTask) string,
) (scheduledReviewCleanupResult, error) {
	if scheduled && !admission.allowWorkspace(workspaceID) {
		return scheduledReviewCleanupResult{}, nil
	}
	resolved, err := s.resolveAutomationClient(ctx, workspaceID, "", "")
	if err != nil {
		s.logger.Warn("skip review cleanup for unavailable workspace connection",
			zap.String("workspace_id", workspaceID), zap.Error(err))
		if scheduled {
			return scheduledReviewCleanupResult{Failures: []scheduledReviewCleanupFailure{{
				Task: candidates[0], WorkspaceID: workspaceID, Err: err,
			}}}, nil
		}
		return scheduledReviewCleanupResult{}, nil
	}
	if scheduled {
		return s.cleanupScheduledReviewPRTaskBatch(
			ctx, resolved.Client, resolved.RateTracker, candidates, workspaceID, resolvePolicy, admission,
		), nil
	}
	deleted, err := s.cleanupReviewPRTaskBatch(
		ctx, resolved.Client, resolved.RateTracker, candidates, resolvePolicy,
	)
	return scheduledReviewCleanupResult{Deleted: deleted}, err
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
//
//nolint:dupl // mirrors buildIssueWatchCaches — different task/watch types
func (s *Service) buildReviewWatchCaches(
	ctx context.Context, prTasks []*ReviewPRTask,
) (policy map[string]string, enabled map[string]bool, unknown map[string]bool, workspace map[string]string) {
	seen := make(map[string]struct{})
	for _, rpt := range prTasks {
		seen[rpt.ReviewWatchID] = struct{}{}
	}
	policy = make(map[string]string, len(seen))
	enabled = make(map[string]bool, len(seen))
	unknown = make(map[string]bool)
	workspace = make(map[string]string, len(seen))
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
		workspace[watchID] = watch.WorkspaceID
	}
	return policy, enabled, unknown, workspace
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
func (s *Service) cleanupReviewPRTaskBatch(
	ctx context.Context, client Client, tracker *RateTracker, prTasks []*ReviewPRTask,
	resolvePolicy func(*ReviewPRTask) string,
) (int, error) {
	deleted := 0
	for _, rpt := range prTasks {
		policy := resolvePolicy(rpt)
		if err := cleanupBatchAdmission(ctx, client, tracker, policy); err != nil {
			return deleted, err
		}
		shouldDelete, reason, err := s.shouldDeleteReviewTaskWithClient(ctx, client, rpt, policy)
		if err != nil {
			if cleanupBatchShouldStop(err) {
				return deleted, err
			}
			continue
		}
		if !shouldDelete {
			continue
		}
		deleted += s.applyReviewPRTaskDeletion(ctx, rpt, policy, reason)
	}
	return deleted, nil
}

func (s *Service) cleanupScheduledReviewPRTaskBatch(
	ctx context.Context,
	client Client,
	tracker *RateTracker,
	prTasks []*ReviewPRTask,
	workspaceID string,
	resolvePolicy func(*ReviewPRTask) string,
	admission scheduledReviewCleanupAdmission,
) scheduledReviewCleanupResult {
	result := scheduledReviewCleanupResult{}
	for _, rpt := range prTasks {
		policy := resolvePolicy(rpt)
		if policy == CleanupPolicyNever || s.retainAutoReviewTaskBeforeFeedback(ctx, rpt, policy) {
			continue
		}
		if admission.record != nil {
			allowed, stopWorkspace := admission.record(rpt, tracker)
			if !allowed {
				if stopWorkspace {
					break
				}
				continue
			}
		}
		if err := cleanupBatchAdmission(ctx, client, tracker, policy); err != nil {
			result.Failures = append(result.Failures, scheduledReviewCleanupFailure{
				Task: rpt, WorkspaceID: workspaceID, Err: err,
			})
			break
		}
		shouldDelete, reason, err := s.shouldDeleteReviewTaskWithClient(ctx, client, rpt, policy)
		if err != nil {
			class := classifyPollErr(err)
			result.Failures = append(result.Failures, scheduledReviewCleanupFailure{
				Task: rpt, WorkspaceID: workspaceID,
				RecordScoped: class == authcircuit.FailureClassConfig, Err: err,
			})
			if class != authcircuit.FailureClassConfig {
				break
			}
			continue
		}
		result.Successes = append(result.Successes, scheduledReviewCleanupSuccess{
			Task: rpt, WorkspaceID: workspaceID,
		})
		if shouldDelete {
			result.Deleted += s.applyReviewPRTaskDeletion(ctx, rpt, policy, reason)
		}
	}
	return result
}

func (s *Service) applyReviewPRTaskDeletion(ctx context.Context, rpt *ReviewPRTask, policy, reason string) int {
	if rpt.TaskID == "" {
		if err := s.store.DeleteReviewPRTask(ctx, rpt.ID); err != nil {
			s.logger.Warn("failed to delete orphan reservation row",
				zap.String("dedup_id", rpt.ID), zap.Error(err))
		}
		return 0
	}
	if err := s.deleteTaskWithReason(ctx, rpt.TaskID, reason); err != nil {
		if isTaskNotFound(err) {
			if err := s.store.DeleteReviewPRTask(ctx, rpt.ID); err != nil {
				s.logger.Warn("failed to delete orphan dedup row after task-not-found",
					zap.String("dedup_id", rpt.ID), zap.Error(err))
				return 0
			}
			return 1
		}
		s.logger.Warn("failed to delete review PR task",
			zap.String("task_id", rpt.TaskID), zap.Error(err))
		return 0
	}
	if err := s.store.DeleteReviewPRTask(ctx, rpt.ID); err != nil {
		s.logger.Warn("deleted task but failed to remove dedup row",
			zap.String("task_id", rpt.TaskID), zap.String("dedup_id", rpt.ID), zap.Error(err))
		return 0
	}
	s.logger.Info("deleted review task",
		zap.String("task_id", rpt.TaskID), zap.String("reason", reason),
		zap.String("policy", policy), zap.Int("pr_number", rpt.PRNumber),
		zap.String("repo", rpt.RepoOwner+"/"+rpt.RepoName))
	return 1
}

func (s *Service) retainAutoReviewTaskBeforeFeedback(ctx context.Context, rpt *ReviewPRTask, policy string) bool {
	if policy != CleanupPolicyAuto || rpt == nil || rpt.TaskID == "" {
		return false
	}
	if s.store == nil {
		return true
	}
	if s.taskSessionChecker != nil {
		hasUserMsg, err := s.taskSessionChecker.HasUserAuthoredMessage(ctx, rpt.TaskID)
		if err != nil {
			if errors.Is(err, ErrTaskNotFound) {
				return false
			}
			s.logger.Debug("failed to check task user messages before scheduled cleanup",
				zap.String("task_id", rpt.TaskID), zap.Error(err))
			return true
		}
		if hasUserMsg {
			return true
		}
	}
	enabled, err := s.HasEnabledTaskPRAgentPrompts(ctx, rpt.TaskID)
	if err != nil {
		s.logger.Debug("failed to check task PR agent prompt options before scheduled cleanup",
			zap.String("task_id", rpt.TaskID), zap.Error(err))
		return true
	}
	return enabled || s.taskSessionChecker == nil
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
	should, reason, _ := s.shouldDeleteReviewTaskWithClient(ctx, s.client, rpt, policy)
	return should, reason
}

func (s *Service) shouldDeleteReviewTaskWithClient(
	ctx context.Context, client Client, rpt *ReviewPRTask, policy string,
) (bool, string, error) {
	if policy == CleanupPolicyNever {
		return false, "", nil
	}
	failureKey := reviewFailureKey(rpt)
	pr, err := client.GetPR(ctx, rpt.RepoOwner, rpt.RepoName, rpt.PRNumber)
	if err != nil {
		s.trackCleanupFailure(failureKey, "review", rpt.RepoOwner+"/"+rpt.RepoName, rpt.PRNumber, err)
		return false, "", err
	}
	s.resetCleanupFailure(failureKey)
	if pr == nil {
		return false, "", nil
	}
	var reason string
	if pr.State == prStateMerged || pr.State == prStateClosed {
		reason = "pr_merged_or_closed" //nolint:goconst // also referenced by string in service_cleanup_policy_test.go; introducing a constant would require coupling the test
	} else {
		reason, err = s.reviewApprovalCleanupReason(ctx, client, rpt)
		if err != nil {
			s.trackCleanupFailure(failureKey, "review", rpt.RepoOwner+"/"+rpt.RepoName, rpt.PRNumber, err)
			return false, "", err
		}
	}
	if reason == "" {
		return false, "", nil
	}
	if policy == CleanupPolicyAlways || rpt.TaskID == "" {
		return true, reason, nil
	}
	if s.taskSessionChecker != nil {
		hasUserMsg, err := s.taskSessionChecker.HasUserAuthoredMessage(ctx, rpt.TaskID)
		if err != nil {
			if errors.Is(err, ErrTaskNotFound) {
				return true, reason, nil
			}
			s.logger.Debug("failed to check task user messages",
				zap.String("task_id", rpt.TaskID), zap.Error(err))
			return false, "", nil
		}
		if hasUserMsg {
			return false, "", nil
		}
	}
	if s.store != nil {
		enabled, err := s.HasEnabledTaskPRAgentPrompts(ctx, rpt.TaskID)
		if err != nil {
			s.logger.Debug("failed to check task PR agent prompt options",
				zap.String("task_id", rpt.TaskID), zap.Error(err))
			return false, "", nil
		}
		if enabled {
			return false, "", nil
		}
	}
	return true, reason, nil
}

func (s *Service) reviewApprovalCleanupReason(
	ctx context.Context, client Client, rpt *ReviewPRTask,
) (string, error) {
	reviews, err := client.ListPRReviews(ctx, rpt.RepoOwner, rpt.RepoName, rpt.PRNumber)
	if err != nil {
		return "", err
	}
	approved := false
	for _, review := range reviews {
		if review.State == reviewStateApproved {
			approved = true
			break
		}
	}
	if !approved {
		return "", nil
	}
	user, userErr := client.GetAuthenticatedUser(ctx)
	if userErr != nil {
		return "", userErr
	}
	for _, review := range reviews {
		if review.State == reviewStateApproved && review.Author == user {
			return "pr_approved_by_user", nil
		}
	}
	return "", nil
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
	if s.taskDeleter == nil {
		return 0, nil
	}
	if watch == nil || watch.WorkspaceID == "" {
		return 0, ErrGitHubWorkspaceRequired
	}
	resolved, err := s.resolveAutomationClient(ctx, watch.WorkspaceID, "", "")
	if err != nil {
		return 0, err
	}
	issueTasks, err := s.store.ListIssueWatchTasksByWatch(ctx, watch.ID)
	if err != nil {
		return 0, fmt.Errorf("list issue watch tasks: %w", err)
	}
	policy := NormalizeCleanupPolicy(watch.CleanupPolicy)
	return s.cleanupIssueTaskBatch(ctx, resolved.Client, resolved.RateTracker, issueTasks,
		func(_ *IssueWatchTask) string { return policy })
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

// CleanupIssueTasksForWorkspace mirrors CleanupReviewTasksForWorkspace for
// issue-watch dedup rows.
//
//nolint:dupl // mirrors CleanupReviewTasksForWorkspace — different task/watch types
func (s *Service) CleanupIssueTasksForWorkspace(ctx context.Context, workspaceID string) (int, error) {
	if s.taskDeleter == nil {
		return 0, nil
	}
	resolved, err := s.resolveAutomationClient(ctx, workspaceID, "", "")
	if err != nil {
		return 0, err
	}
	watches, err := s.store.ListIssueWatches(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("list issue watches: %w", err)
	}
	watchPolicies := make(map[string]string, len(watches))
	for _, watch := range watches {
		watchPolicies[watch.ID] = NormalizeCleanupPolicy(watch.CleanupPolicy)
	}
	if len(watchPolicies) == 0 {
		return 0, nil
	}
	issueTasks, err := s.store.ListAllIssueWatchTasks(ctx)
	if err != nil {
		return 0, fmt.Errorf("list all issue watch tasks: %w", err)
	}
	candidates := make([]*IssueWatchTask, 0, len(issueTasks))
	for _, it := range issueTasks {
		if _, ok := watchPolicies[it.IssueWatchID]; ok {
			candidates = append(candidates, it)
		}
	}
	return s.cleanupIssueTaskBatch(ctx, resolved.Client, resolved.RateTracker, candidates, func(it *IssueWatchTask) string {
		return watchPolicies[it.IssueWatchID]
	})
}

//nolint:dupl // mirrors cleanupAllReviewTasks — different types, same orchestration
func (s *Service) cleanupAllIssueTasks(ctx context.Context, orphansOnly bool) (int, error) {
	if s.taskDeleter == nil {
		return 0, nil
	}
	issueTasks, err := s.store.ListAllIssueWatchTasks(ctx)
	if err != nil {
		return 0, fmt.Errorf("list all issue watch tasks: %w", err)
	}
	if len(issueTasks) == 0 {
		return 0, nil
	}
	policyCache, enabledCache, unknownCache, workspaceCache := s.buildIssueWatchCaches(ctx, issueTasks)
	candidatesByWorkspace := make(map[string][]*IssueWatchTask)
	for _, it := range issueTasks {
		if unknownCache[it.IssueWatchID] {
			continue
		}
		if orphansOnly && enabledCache[it.IssueWatchID] {
			continue
		}
		workspaceID := workspaceCache[it.IssueWatchID]
		if workspaceID == "" {
			continue
		}
		candidatesByWorkspace[workspaceID] = append(candidatesByWorkspace[workspaceID], it)
	}
	deleted := 0
	for workspaceID, candidates := range candidatesByWorkspace {
		resolved, resolveErr := s.resolveAutomationClient(ctx, workspaceID, "", "")
		if resolveErr != nil {
			s.logger.Warn("skip issue cleanup for unavailable workspace connection",
				zap.String("workspace_id", workspaceID), zap.Error(resolveErr))
			continue
		}
		batchDeleted, batchErr := s.cleanupIssueTaskBatch(ctx, resolved.Client, resolved.RateTracker, candidates, func(it *IssueWatchTask) string {
			return policyCache[it.IssueWatchID]
		})
		deleted += batchDeleted
		if batchErr != nil {
			return deleted, batchErr
		}
	}
	return deleted, nil
}

//nolint:dupl // mirrors buildReviewWatchCaches — different task/watch types
func (s *Service) buildIssueWatchCaches(
	ctx context.Context, issueTasks []*IssueWatchTask,
) (policy map[string]string, enabled map[string]bool, unknown map[string]bool, workspace map[string]string) {
	seen := make(map[string]struct{})
	for _, it := range issueTasks {
		seen[it.IssueWatchID] = struct{}{}
	}
	policy = make(map[string]string, len(seen))
	enabled = make(map[string]bool, len(seen))
	unknown = make(map[string]bool)
	workspace = make(map[string]string, len(seen))
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
		workspace[watchID] = watch.WorkspaceID
	}
	return policy, enabled, unknown, workspace
}

//nolint:dupl // mirrors cleanupReviewPRTaskBatch — different types, same structure
func (s *Service) cleanupIssueTaskBatch(
	ctx context.Context, client Client, tracker *RateTracker, issueTasks []*IssueWatchTask,
	resolvePolicy func(*IssueWatchTask) string,
) (int, error) {
	deleted := 0
	for _, it := range issueTasks {
		policy := resolvePolicy(it)
		if err := cleanupBatchAdmission(ctx, client, tracker, policy); err != nil {
			return deleted, err
		}
		if it.TaskID == "" {
			// Orphan reservation row — no task was created. Clean it up
			// but don't count it as a deleted task.
			should, _, err := s.shouldDeleteIssueTaskWithClient(ctx, client, it, policy)
			if err != nil {
				if cleanupBatchShouldStop(err) {
					return deleted, err
				}
				continue
			}
			if should {
				if err := s.store.DeleteIssueWatchTask(ctx, it.ID); err != nil {
					s.logger.Warn("failed to delete orphan reservation row",
						zap.String("dedup_id", it.ID), zap.Error(err))
				}
			}
			continue
		}
		shouldDelete, reason, err := s.shouldDeleteIssueTaskWithClient(ctx, client, it, policy)
		if err != nil {
			if cleanupBatchShouldStop(err) {
				return deleted, err
			}
			continue
		}
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
	return deleted, nil
}

// shouldDeleteIssueTask checks whether an issue task is eligible for cleanup.
// Policy gating mirrors shouldDeleteReviewTask.
func (s *Service) shouldDeleteIssueTask(ctx context.Context, it *IssueWatchTask, policy string) (bool, string) {
	should, reason, _ := s.shouldDeleteIssueTaskWithClient(ctx, s.client, it, policy)
	return should, reason
}

func (s *Service) shouldDeleteIssueTaskWithClient(
	ctx context.Context, client Client, it *IssueWatchTask, policy string,
) (bool, string, error) {
	if policy == CleanupPolicyNever {
		return false, "", nil
	}
	failureKey := issueFailureKey(it)
	state, err := client.GetIssueState(ctx, it.RepoOwner, it.RepoName, it.IssueNumber)
	if err != nil {
		s.trackCleanupFailure(failureKey, "issue", it.RepoOwner+"/"+it.RepoName, it.IssueNumber, err)
		return false, "", err
	}
	s.resetCleanupFailure(failureKey)
	if state != "closed" {
		return false, "", nil
	}
	reason := "issue_closed"
	if policy == CleanupPolicyAlways || it.TaskID == "" || s.taskSessionChecker == nil {
		return true, reason, nil
	}
	hasUserMsg, err := s.taskSessionChecker.HasUserAuthoredMessage(ctx, it.TaskID)
	if err != nil {
		s.logger.Debug("failed to check task user messages",
			zap.String("task_id", it.TaskID), zap.Error(err))
		return false, "", nil
	}
	if hasUserMsg {
		return false, "", nil
	}
	return true, reason, nil
}

func issueFailureKey(it *IssueWatchTask) string {
	return fmt.Sprintf("issue:%s:%s/%s#%d", it.IssueWatchID, it.RepoOwner, it.RepoName, it.IssueNumber)
}
