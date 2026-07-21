package github

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// --- PR Watch operations ---

// CreatePRWatch creates a new PR watch for a (session, repository, branch)
// triple. `repositoryID` may be empty for legacy single-repo callers;
// multi-repo callers must pass the per-task repository_id so each repo gets
// its own watch row. Multi-branch tasks store one watch per branch so a
// secondary branch's push isn't lost behind the primary's existing watch.
func (s *Service) CreatePRWatch(ctx context.Context, sessionID, taskID, repositoryID, owner, repo string, prNumber int, branch string) (*PRWatch, error) {
	// Evict any negative-cache entry up front. Caller intent on either
	// branch (creating a new watch or re-finding an existing one) is "I
	// want this repo watched", which means a stale "missing" verdict
	// from a prior auth failure should be re-probed immediately rather
	// than held for the rest of the 10-min TTL.
	s.evictRepoNegative(owner, repo)
	existing, err := s.store.GetPRWatchBySessionRepoAndBranch(ctx, sessionID, repositoryID, branch)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil // already watching this (session, repo, branch)
	}
	w := &PRWatch{
		SessionID:    sessionID,
		TaskID:       taskID,
		RepositoryID: repositoryID,
		Owner:        owner,
		Repo:         repo,
		PRNumber:     prNumber,
		Branch:       branch,
	}
	if err := s.store.CreatePRWatch(ctx, w); err != nil {
		return nil, fmt.Errorf("create PR watch: %w", err)
	}
	s.logger.Info("created PR watch",
		zap.String("session_id", sessionID),
		zap.String("repository_id", repositoryID),
		zap.String("branch", branch),
		zap.Int("pr_number", prNumber))
	return w, nil
}

// GetPRWatchBySession returns the first PR watch for a session. Multi-repo
// callers should prefer GetPRWatchBySessionAndRepo to avoid landing on the
// wrong repo's watch.
func (s *Service) GetPRWatchBySession(ctx context.Context, sessionID string) (*PRWatch, error) {
	return s.store.GetPRWatchBySession(ctx, sessionID)
}

// GetPRWatchBySessionAndRepo returns the PR watch for a (session, repo) pair.
func (s *Service) GetPRWatchBySessionAndRepo(ctx context.Context, sessionID, repositoryID string) (*PRWatch, error) {
	return s.store.GetPRWatchBySessionAndRepo(ctx, sessionID, repositoryID)
}

// GetPRWatchBySessionRepoAndBranch returns the PR watch for a precise
// (session, repository, branch) triple — used by push detection in
// multi-branch tasks so each branch's push lands on its own watch row.
func (s *Service) GetPRWatchBySessionRepoAndBranch(ctx context.Context, sessionID, repositoryID, branch string) (*PRWatch, error) {
	return s.store.GetPRWatchBySessionRepoAndBranch(ctx, sessionID, repositoryID, branch)
}

// ListPRWatchesBySession returns every PR watch for a session.
func (s *Service) ListPRWatchesBySession(ctx context.Context, sessionID string) ([]*PRWatch, error) {
	return s.store.ListPRWatchesBySession(ctx, sessionID)
}

// ListPRWatchesByTask returns every PR watch for a task.
func (s *Service) ListPRWatchesByTask(ctx context.Context, taskID string) ([]*PRWatch, error) {
	return s.store.ListPRWatchesByTask(ctx, taskID)
}

// ListActivePRWatches returns all active PR watches.
func (s *Service) ListActivePRWatches(ctx context.Context) ([]*PRWatch, error) {
	return s.store.ListActivePRWatches(ctx)
}

// DeletePRWatch deletes a PR watch by ID.
func (s *Service) DeletePRWatch(ctx context.Context, id string) error {
	return s.store.DeletePRWatch(ctx, id)
}

// UpdatePRWatchBranchIfSearching atomically updates branch only when pr_number = 0.
func (s *Service) UpdatePRWatchBranchIfSearching(ctx context.Context, id, branch string) error {
	return s.store.UpdatePRWatchBranchIfSearching(ctx, id, branch)
}

// UpdatePRWatchPRNumber updates a PR watch's PR number after discovery.
func (s *Service) UpdatePRWatchPRNumber(ctx context.Context, id string, prNumber int) error {
	return s.store.UpdatePRWatchPRNumber(ctx, id, prNumber)
}

// ResetPRWatch atomically resets a watch's branch and clears its pr_number so
// the poller re-searches for a PR on the new branch. See Store.ResetPRWatch.
func (s *Service) ResetPRWatch(ctx context.Context, id, branch string) error {
	return s.store.ResetPRWatch(ctx, id, branch)
}

// CheckPRWatch fetches lightweight PR status for a watch and determines if there are changes.
func (s *Service) CheckPRWatch(ctx context.Context, watch *PRWatch) (*PRStatus, bool, error) {
	if s.client == nil {
		return nil, false, fmt.Errorf("github client not available")
	}
	status, err := s.client.GetPRStatus(ctx, watch.Owner, watch.Repo, watch.PRNumber)
	if err != nil {
		return nil, false, err
	}

	// Check for check status or review state changes
	hasNew := status.ChecksState != watch.LastCheckStatus || status.ReviewState != watch.LastReviewState
	commentAt := prWatchFeedbackWatermark(watch, status)
	hasNew = hasNew || prWatchFeedbackUpdatedSinceWatch(watch, status)

	// Update watch timestamps
	now := time.Now().UTC()
	if err := s.store.UpdatePRWatchTimestamps(ctx, watch.ID, now, commentAt, status.ChecksState, status.ReviewState); err != nil {
		s.logger.Error("failed to update PR watch timestamps", zap.String("id", watch.ID), zap.Error(err))
	}

	return status, hasNew, nil
}

func prWatchFeedbackUpdatedSinceWatch(watch *PRWatch, status *PRStatus) bool {
	if watch == nil || status == nil || status.PR == nil || status.PR.UpdatedAt.IsZero() {
		return false
	}
	return watch.LastCommentAt == nil || status.PR.UpdatedAt.After(*watch.LastCommentAt)
}

func prWatchFeedbackWatermark(watch *PRWatch, status *PRStatus) *time.Time {
	if status != nil && status.PR != nil && !status.PR.UpdatedAt.IsZero() {
		updatedAt := status.PR.UpdatedAt
		if watch != nil && watch.LastCommentAt != nil && watch.LastCommentAt.After(updatedAt) {
			return watch.LastCommentAt
		}
		return &updatedAt
	}
	if watch == nil {
		return nil
	}
	return watch.LastCommentAt
}

// EnsurePRWatch creates a PRWatch with pr_number=0 for a
// (session, repo, branch) triple if one doesn't already exist. The poller
// will detect the PR by searching for the branch on GitHub. `repositoryID`
// is empty for legacy single-repo callers; multi-repo / multi-branch
// callers MUST pass the per-task repository_id and the worktree's branch
// so each branch gets its own watch — keying on (session, repo) alone
// drops secondary branches' watches behind the primary's existing row.
func (s *Service) EnsurePRWatch(ctx context.Context, sessionID, taskID, repositoryID, owner, repo, branch string) (*PRWatch, error) {
	// Same up-front eviction as CreatePRWatch — see that function for the
	// rationale. The existing-watch early-return path must not inherit a
	// stale "missing" verdict from a prior incarnation.
	s.evictRepoNegative(owner, repo)
	existing, err := s.store.GetPRWatchBySessionRepoAndBranch(ctx, sessionID, repositoryID, branch)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	w := &PRWatch{
		SessionID:    sessionID,
		TaskID:       taskID,
		RepositoryID: repositoryID,
		Owner:        owner,
		Repo:         repo,
		PRNumber:     0,
		Branch:       branch,
	}
	if err := s.store.CreatePRWatch(ctx, w); err != nil {
		return nil, fmt.Errorf("ensure PR watch: %w", err)
	}
	s.logger.Info("created PR watch for session (will search for PR)",
		zap.String("session_id", sessionID),
		zap.String("repository_id", repositoryID),
		zap.String("branch", branch))
	return w, nil
}

// --- Task-PR association ---

// AssociatePRWithTask creates a task-PR association scoped to a specific
// repository. `repositoryID` is the per-task repository_id (from
// task_repositories); empty preserves legacy single-repo behavior. Multi-repo
// callers MUST pass it — empty causes ReplaceTaskPR to wipe the entire task's
// PR rows (legacy "delete all" branch), which is what older code relied on.
func (s *Service) AssociatePRWithTask(ctx context.Context, taskID, repositoryID string, pr *PR) (*TaskPR, error) {
	// Multi-branch: scope the "already-current" short-circuit by exact
	// pr_number too. A task can hold multiple PR rows per (task, repo) on
	// different branches; the legacy by-repo lookup returns whichever row
	// was most recently updated, which would make Associate think the
	// secondary PR is already there (wrong PR number) and skip the insert
	// — or worse, fall through to ReplaceTaskPR which used to delete the
	// sibling row.
	existing, err := s.store.GetTaskPRByRepoAndNumber(ctx, taskID, repositoryID, pr.Number)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	tp := &TaskPR{
		TaskID:       taskID,
		RepositoryID: repositoryID,
		Owner:        pr.RepoOwner,
		Repo:         pr.RepoName,
		PRNumber:     pr.Number,
		PRURL:        pr.HTMLURL,
		PRTitle:      pr.Title,
		HeadBranch:   pr.HeadBranch,
		BaseBranch:   pr.BaseBranch,
		AuthorLogin:  pr.AuthorLogin,
		State:        pr.State,
		Additions:    pr.Additions,
		Deletions:    pr.Deletions,
		CreatedAt:    pr.CreatedAt,
		MergedAt:     pr.MergedAt,
		ClosedAt:     pr.ClosedAt,
	}
	// ReplaceTaskPR upserts the row matching (task, repository, pr_number).
	// Multi-branch tasks may already hold sibling rows for the SAME
	// (task, repository) on different PR numbers — ReplaceTaskPR no longer
	// touches them. The early-return above guarantees we only reach this
	// line when no row for the exact pr_number exists yet, so this is a
	// straight insert in steady state; the delete-then-insert form is
	// retained so a retry that races a partial write resolves cleanly.
	if err := s.store.ReplaceTaskPR(ctx, tp); err != nil {
		return nil, fmt.Errorf("replace task PR: %w", err)
	}

	// Publish event for UI
	if s.eventBus != nil {
		event := bus.NewEvent(events.GitHubTaskPRUpdated, "github", tp)
		if err := s.eventBus.Publish(ctx, events.GitHubTaskPRUpdated, event); err != nil {
			s.logger.Debug("failed to publish task PR updated event", zap.Error(err))
		}
	}

	s.logger.Info("associated PR with task",
		zap.String("task_id", taskID),
		zap.String("repository_id", repositoryID),
		zap.Int("pr_number", pr.Number))
	return tp, nil
}

// AssociateExistingPRByURL parses a GitHub PR URL, fetches the PR data, and
// associates it with the given task. No PR watch is created — this is used
// when the caller already knows the PR (e.g. user clicked "+ Task" on a PR
// in the GitHub page), so branch-based discovery is unnecessary. The watch
// for ongoing status sync is still created later when the agent session
// starts (see ensureSessionPRWatch).
//
// Returns the persisted TaskPR row so callers can confirm the association
// and react to errors synchronously, in contrast to AssociatePRByURL's
// fire-and-forget logging.
func (s *Service) AssociateExistingPRByURL(ctx context.Context, taskID, repositoryID, prURL string) (*TaskPR, error) {
	if s.client == nil {
		return nil, fmt.Errorf("github client not available")
	}
	owner, repo, prNumber, err := parsePRURL(prURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidPRURL, err)
	}
	pr, err := s.client.GetPR(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, fmt.Errorf("fetch PR: %w", err)
	}
	tp, err := s.AssociatePRWithTask(ctx, taskID, repositoryID, pr)
	if err != nil {
		return nil, fmt.Errorf("associate PR with task: %w", err)
	}
	return tp, nil
}

// AssociatePRByURL parses a GitHub PR URL, fetches the PR data, creates a PR
// watch, and associates it with the given task. Called after the user
// creates a PR from the UI. `repositoryID` scopes the watch + association to
// a specific per-task repository (multi-repo tasks); empty preserves the
// legacy single-repo behavior. Without this, the second repo's UI-initiated
// PR would overwrite the first's TaskPR row.
func (s *Service) AssociatePRByURL(ctx context.Context, sessionID, taskID, repositoryID, prURL, branch string) {
	if s.client == nil {
		return
	}
	owner, repo, prNumber, err := parsePRURL(prURL)
	if err != nil {
		s.logger.Error("failed to parse PR URL", zap.String("url", prURL), zap.Error(err))
		return
	}

	pr, err := s.client.GetPR(ctx, owner, repo, prNumber)
	if err != nil {
		s.logger.Error("failed to fetch PR after creation",
			zap.String("url", prURL), zap.Error(err))
		return
	}

	// Create PR watch for ongoing monitoring
	if branch == "" {
		branch = pr.HeadBranch
	}
	if _, watchErr := s.CreatePRWatch(ctx, sessionID, taskID, repositoryID, owner, repo, prNumber, branch); watchErr != nil {
		s.logger.Error("failed to create PR watch after PR creation",
			zap.String("session_id", sessionID), zap.Error(watchErr))
	}

	// Associate PR with task (persists + publishes WS event)
	if _, assocErr := s.AssociatePRWithTask(ctx, taskID, repositoryID, pr); assocErr != nil {
		s.logger.Error("failed to associate PR with task after creation",
			zap.String("task_id", taskID), zap.Error(assocErr))
	}
}

// parsePRURL extracts owner, repo, and PR number from a GitHub PR URL.
// Expected format: https://github.com/{owner}/{repo}/pull/{number}
// Handles trailing slashes, query parameters, and URL fragments.
func parsePRURL(prURL string) (owner, repo string, number int, err error) {
	// Strip trailing whitespace/newlines
	prURL = strings.TrimSpace(prURL)

	// Find the /pull/ segment
	idx := strings.Index(prURL, "/pull/")
	if idx < 0 {
		return "", "", 0, fmt.Errorf("URL does not contain /pull/: %s", prURL)
	}

	// Parse PR number after /pull/, stripping query params, fragments, and trailing slashes
	numStr := prURL[idx+len("/pull/"):]
	if i := strings.IndexAny(numStr, "?#"); i >= 0 {
		numStr = numStr[:i]
	}
	numStr = strings.TrimRight(numStr, "/")
	number, err = strconv.Atoi(numStr)
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid PR number in URL %s: %w", prURL, err)
	}
	if number <= 0 {
		return "", "", 0, fmt.Errorf("invalid PR number in URL %s: must be greater than zero", prURL)
	}

	// Parse owner/repo from path before /pull/
	pathBefore := prURL[:idx]
	// Remove scheme+host prefix (find last two path segments)
	parts := strings.Split(strings.TrimRight(pathBefore, "/"), "/")
	if len(parts) < 2 {
		return "", "", 0, fmt.Errorf("cannot extract owner/repo from URL: %s", prURL)
	}
	repo = parts[len(parts)-1]
	owner = parts[len(parts)-2]
	if owner == "" || repo == "" {
		return "", "", 0, fmt.Errorf("empty owner or repo in URL: %s", prURL)
	}
	return owner, repo, number, nil
}

// GetTaskPR returns the PR association for a task.
func (s *Service) GetTaskPR(ctx context.Context, taskID string) (*TaskPR, error) {
	return s.store.GetTaskPR(ctx, taskID)
}

// GetTaskPRByOwnerRepoNumber returns the task PR row matching a PR feedback event.
func (s *Service) GetTaskPRByOwnerRepoNumber(ctx context.Context, taskID, owner, repo string, prNumber int) (*TaskPR, error) {
	prs, err := s.store.ListTaskPRsByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	for _, pr := range prs {
		if pr.Owner == owner && pr.Repo == repo && pr.PRNumber == prNumber {
			return pr, nil
		}
	}
	return nil, nil
}

// ListTaskPRs returns PR associations for multiple tasks, grouped by task_id.
// Multi-repo tasks may have more than one PR per task.
func (s *Service) ListTaskPRs(ctx context.Context, taskIDs []string) (map[string][]*TaskPR, error) {
	return s.store.ListTaskPRsByTaskIDs(ctx, taskIDs)
}

// FindTaskIDsByPRNumber returns the IDs of tasks in a workspace associated with
// the given PR number. Used by task search to surface a task by its PR number.
func (s *Service) FindTaskIDsByPRNumber(ctx context.Context, workspaceID string, prNumber int) ([]string, error) {
	return s.store.ListTaskIDsByPRNumber(ctx, workspaceID, prNumber)
}

// ListWorkspaceTaskPRs returns all PR associations for a workspace, grouped by
// task_id. Multi-repo tasks may have more than one PR per task. It returns
// cached data immediately and triggers background refresh for stale entries.
func (s *Service) ListWorkspaceTaskPRs(ctx context.Context, workspaceID string) (map[string][]*TaskPR, error) {
	result, err := s.store.ListTaskPRsByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	// Collect stale task IDs for background refresh. A task is considered stale
	// if any of its PRs are stale.
	staleTasks := make(map[string]struct{})
	for taskID, prs := range result {
		for _, tp := range prs {
			if tp.LastSyncedAt == nil || time.Since(*tp.LastSyncedAt) >= PRSyncFreshnessWindow {
				staleTasks[taskID] = struct{}{}
				break
			}
		}
	}
	if len(staleTasks) > 0 {
		s.refreshStaleWorkspaceWatches(workspaceID, staleTasks)
	}
	return result, nil
}

// refreshStaleWorkspaceWatches fans in all stale watches across the stale
// tasks and runs ONE batched GraphQL sync, so a 40-watch workspace fires
// ~2 gh subprocess calls instead of 40 (one per_task * five concurrent).
// Best-effort: errors are logged at Debug and the cached result is still
// returned to the caller. Background goroutine so the WS handler returns
// immediately.
//
// Coalesces overlapping refreshes for the same workspace: if a refresh is
// already in flight, subsequent calls drop their stale set on the floor
// and let the running goroutine finish. Without this, the frontend's 5s
// poll (or a burst of workspace events) would stack goroutines that each
// fire a batched GraphQL request, defeating the per-process throttle
// once the cap is small enough to start queueing.
func (s *Service) refreshStaleWorkspaceWatches(workspaceID string, staleTasks map[string]struct{}) {
	if _, inflight := s.inflightWorkspaceRefreshes.LoadOrStore(workspaceID, struct{}{}); inflight {
		return
	}
	// Track the goroutine on the service WaitGroup so Stop() drains it,
	// and derive syncCtx from s.stopCtx so shutdown cancels the in-flight
	// sync instead of letting it run past process teardown. See
	// apps/backend/AGENTS.md "Goroutine ownership and leak testing".
	s.bgWG.Add(1)
	go func() {
		defer s.bgWG.Done()
		defer s.inflightWorkspaceRefreshes.Delete(workspaceID)
		syncCtx, cancel := context.WithTimeout(s.stopCtx, 60*time.Second)
		defer cancel()
		var allWatches []*PRWatch
		for taskID := range staleTasks {
			watches, err := s.store.ListPRWatchesByTask(syncCtx, taskID)
			if err != nil {
				s.logger.Debug("list PR watches for refresh failed",
					zap.String("task_id", taskID), zap.Error(err))
				continue
			}
			allWatches = append(allWatches, watches...)
		}
		if len(allWatches) == 0 {
			return
		}
		if _, err := s.SyncWatchesBatched(syncCtx, allWatches); err != nil {
			// Batched fetch failed (noop client, auth blip, GraphQL error).
			// Fall back to per-task sync with bounded concurrency so we
			// don't spawn one gh per watch in lockstep.
			s.logger.Debug("batched workspace PR sync failed; falling back per-task",
				zap.Int("watches", len(allWatches)), zap.Error(err))
			s.refreshStaleTasksPerTask(syncCtx, staleTasks)
		}
	}()
}

// refreshStaleTasksPerTask is the legacy bounded-concurrency fallback for
// when SyncWatchesBatched can't be used (e.g. NoopClient). Kept small —
// the global gh subprocess semaphore in gh_throttle.go is what actually
// caps fan-out now, the worker pool here just bounds goroutine count.
//
// Blocks until every spawned goroutine returns. The caller relies on this
// to keep `inflightWorkspaceRefreshes` from being cleared while
// TriggerPRSyncAll is still writing PR-watch state — otherwise a follow-up
// refresh for the same workspace could race the still-running fallback.
func (s *Service) refreshStaleTasksPerTask(ctx context.Context, staleTasks map[string]struct{}) {
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup
	for taskID := range staleTasks {
		// Honor ctx so shutdown / parent-timeout stops queuing new tasks
		// instead of waiting on a busy semaphore slot.
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return
		}
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			defer func() { <-sem }()
			if _, syncErr := s.TriggerPRSyncAll(ctx, id); syncErr != nil {
				s.logger.Debug("background PR sync failed",
					zap.String("task_id", id), zap.Error(syncErr))
			}
		}(taskID)
	}
	wg.Wait()
}

// findTaskPRForStatus locates the TaskPR row matching the (task, owner, repo,
// pr_number) tuple from a poll result. Multi-repo tasks can have multiple
// rows for the same task — narrowing by (owner, repo, pr_number) ensures the
// caller updates the right one. Returns nil (no error) when no row exists,
// matching the prior GetTaskPR semantics.
func (s *Service) findTaskPRForStatus(ctx context.Context, taskID string, pr *PR) (*TaskPR, error) {
	rows, err := s.store.ListTaskPRsByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	for _, tp := range rows {
		if tp.Owner == pr.RepoOwner && tp.Repo == pr.RepoName && tp.PRNumber == pr.Number {
			return tp, nil
		}
	}
	return nil, nil
}

// SyncTaskPR updates a TaskPR record with the latest PR status. Multi-repo:
// the row is found by (task_id, owner, repo, pr_number) since the same
// task can have several PRs; the legacy GetTaskPR(taskID) "first match"
// would cross repos and silently update the wrong row.
// It only publishes a github.task_pr.updated event when data actually changed,
// preventing feedback loops with frontend sync handlers.
//
//nolint:cyclop // sequential field-by-field "populated/preserve" reconciliation; splitting helpers hides intent
func (s *Service) SyncTaskPR(ctx context.Context, taskID string, status *PRStatus) error {
	if status == nil || status.PR == nil {
		return fmt.Errorf("sync task PR: missing PR data for task %s", taskID)
	}
	tp, err := s.findTaskPRForStatus(ctx, taskID, status.PR)
	if err != nil || tp == nil {
		return err
	}

	// Some sync paths (notably the batched GraphQL poller) don't populate
	// ChecksTotal / ChecksPassing — they only carry the rollup state. The
	// caller sets status.ChecksPopulated=true when it actually counted
	// checks; otherwise we preserve the persisted values so the popover
	// doesn't flap to "0/0" between a rich REST sync and a lightweight
	// GraphQL one. When the populated counter says 0/0 it really is 0/0
	// (e.g. all workflows were removed from the PR), so we honor it.
	nextChecksTotal, nextChecksPassing := tp.ChecksTotal, tp.ChecksPassing
	if status.ChecksPopulated {
		nextChecksTotal = status.ChecksTotal
		nextChecksPassing = status.ChecksPassing
	}
	// Same Populated/preserve dance for unresolved review threads — the
	// REST path doesn't fetch them, so blindly writing status.UnresolvedReviewThreads
	// would clobber the non-zero value set by the GraphQL path on every poll.
	nextUnresolved := tp.UnresolvedReviewThreads
	if status.UnresolvedReviewThreadsPopulated {
		nextUnresolved = status.UnresolvedReviewThreads
	}
	// Review counts: only overwrite when the caller actually computed them.
	// Both REST and GraphQL paths now populate these, but a partial sync
	// path that doesn't would otherwise reset the popover's "Approved (N)"
	// to zero.
	nextReviewCount, nextPendingReviewCount := tp.ReviewCount, tp.PendingReviewCount
	if status.ReviewCountsPopulated {
		nextReviewCount = status.ReviewCount
		nextPendingReviewCount = status.PendingReviewCount
	}
	// PRs can be retargeted to a different base branch; pick up the new
	// branch from status.PR before resolving branch-protection so we don't
	// indefinitely surface the wrong rule.
	nextBaseBranch := tp.BaseBranch
	if status.PR.BaseBranch != "" && status.PR.BaseBranch != tp.BaseBranch {
		nextBaseBranch = status.PR.BaseBranch
	}
	// RequiredReviews comes from branch protection, fetched separately.
	// Treat nil as "unknown — don't touch"; only write when the caller has it
	// or our cache resolves the rule for this base branch.
	nextRequiredReviews := tp.RequiredReviews
	if status.RequiredReviews != nil {
		nextRequiredReviews = status.RequiredReviews
	} else if fetched := s.fetchRequiredReviews(ctx, tp.Owner, tp.Repo, nextBaseBranch); fetched != nil {
		nextRequiredReviews = fetched
	}
	// GitHub reports draft status separately from mergeStateStatus and can
	// return CLEAN for a draft PR. Persist the effective blocker so every
	// TaskPR consumer agrees that drafts are not ready to merge.
	nextMergeableState := status.MergeableState
	if status.PR.Draft {
		nextMergeableState = "draft"
	}

	changed := tp.State != status.PR.State ||
		tp.PRTitle != status.PR.Title ||
		tp.Additions != status.PR.Additions ||
		tp.Deletions != status.PR.Deletions ||
		tp.ReviewState != status.ReviewState ||
		tp.ChecksState != status.ChecksState ||
		tp.MergeableState != nextMergeableState ||
		tp.ReviewCount != nextReviewCount ||
		tp.PendingReviewCount != nextPendingReviewCount ||
		!intPtrEqual(tp.RequiredReviews, nextRequiredReviews) ||
		tp.ChecksTotal != nextChecksTotal ||
		tp.ChecksPassing != nextChecksPassing ||
		tp.UnresolvedReviewThreads != nextUnresolved ||
		tp.BaseBranch != nextBaseBranch ||
		!timeEqual(tp.MergedAt, status.PR.MergedAt) ||
		!timeEqual(tp.ClosedAt, status.PR.ClosedAt)

	tp.State = status.PR.State
	tp.PRTitle = status.PR.Title
	tp.Additions = status.PR.Additions
	tp.Deletions = status.PR.Deletions
	tp.MergedAt = status.PR.MergedAt
	tp.ClosedAt = status.PR.ClosedAt
	tp.ReviewState = status.ReviewState
	tp.ChecksState = status.ChecksState
	tp.MergeableState = nextMergeableState
	tp.ReviewCount = nextReviewCount
	tp.PendingReviewCount = nextPendingReviewCount
	tp.RequiredReviews = nextRequiredReviews
	tp.ChecksTotal = nextChecksTotal
	tp.ChecksPassing = nextChecksPassing
	tp.UnresolvedReviewThreads = nextUnresolved
	tp.BaseBranch = nextBaseBranch
	// CommentCount is no longer updated from polling -- only refreshed on-demand
	now := time.Now().UTC()
	tp.LastSyncedAt = &now

	if err := s.store.UpdateTaskPR(ctx, tp); err != nil {
		return fmt.Errorf("update task PR: %w", err)
	}

	if changed && s.eventBus != nil {
		event := bus.NewEvent(events.GitHubTaskPRUpdated, "github", tp)
		if err := s.eventBus.Publish(ctx, events.GitHubTaskPRUpdated, event); err != nil {
			s.logger.Debug("failed to publish task PR updated event", zap.Error(err))
		}
	}
	return nil
}

// TriggerPRSync performs an immediate PR status sync for a task. Single-repo
// callers see the same single-PR contract as before. Multi-repo callers get
// the primary repo's PR back; they should use TriggerPRSyncAll to refresh
// every repo's PR in one round-trip.
func (s *Service) TriggerPRSync(ctx context.Context, taskID string) (*TaskPR, error) {
	watch, err := s.store.GetPRWatchByTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("get PR watch: %w", err)
	}
	if watch == nil {
		// No watch — just return existing TaskPR if any
		return s.store.GetTaskPR(ctx, taskID)
	}

	if watch.PRNumber == 0 {
		return s.triggerPRDetection(ctx, watch, taskID)
	}

	return s.triggerPRStatusSync(ctx, watch, taskID)
}

// TriggerPRSyncAll performs an immediate PR status sync for every PR watch
// associated with the task and returns every resulting TaskPR. For
// multi-repo tasks this is the right entry point — TriggerPRSync only
// touches the most recently updated watch and silently leaves the other
// repos' PRs stale. Returns an empty slice (not nil) when the task has no
// watches.
//
// When the client supports GraphQL (the production path) all of the task's
// watches are fetched in 1-2 batched gh subprocess calls. The per-watch
// fallback below is reached only for the NoopClient (auth disabled) or
// when the batched fetch itself fails; in those cases we fan out one
// subprocess per watch as before.
func (s *Service) TriggerPRSyncAll(ctx context.Context, taskID string) ([]*TaskPR, error) {
	prs, _, err := s.triggerPRSyncAllPermanent(ctx, taskID)
	return prs, err
}

// TriggerPRSyncAllPermanent extends TriggerPRSyncAll with a "permanent"
// flag: true iff the task has at least one PR watch and every one of
// those watches is currently in the 10-min negative cache (or its repo
// is otherwise classified as unresolvable). The WS handler uses this to
// tell the frontend to stop the 5s sync-retry interval, so a task
// pointing at a deleted repo doesn't keep hammering the gh throttle for
// the lifetime of the task.
func (s *Service) TriggerPRSyncAllPermanent(ctx context.Context, taskID string) ([]*TaskPR, bool, error) {
	return s.triggerPRSyncAllPermanent(ctx, taskID)
}

func (s *Service) triggerPRSyncAllPermanent(ctx context.Context, taskID string) ([]*TaskPR, bool, error) {
	watches, err := s.store.ListPRWatchesByTask(ctx, taskID)
	if err != nil {
		return nil, false, fmt.Errorf("list PR watches: %w", err)
	}
	if len(watches) == 0 {
		// No watches — fall back to whatever TaskPRs already exist (e.g.
		// PRs imported via task-create-from-PR-URL where the watch is
		// optional). Empty slice if none. permanent=false (the task can
		// still acquire a watch later via push detection).
		existing, listErr := s.store.ListTaskPRsByTask(ctx, taskID)
		if listErr != nil {
			return nil, false, fmt.Errorf("list task PRs: %w", listErr)
		}
		return existing, false, nil
	}
	prs, syncErr := s.runBatchedOrPerWatchSync(ctx, taskID, watches)
	return prs, s.areAllWatchesPermanentlyMissing(watches), syncErr
}

// runBatchedOrPerWatchSync is the shared "try batched, fall back to
// per-watch" body of triggerPRSyncAllPermanent. Split out so the
// permanent-flag computation can wrap the result without duplicating
// the batched / fallback branches inline.
func (s *Service) runBatchedOrPerWatchSync(ctx context.Context, taskID string, watches []*PRWatch) ([]*TaskPR, error) {
	if _, batchErr := s.SyncWatchesBatched(ctx, watches); batchErr != nil {
		s.logger.Debug("batched PR sync failed; falling back to per-watch",
			zap.String("task_id", taskID), zap.Error(batchErr))
		return s.triggerPRSyncAllPerWatch(ctx, taskID, watches)
	}
	// Batched path applied all DB updates inline; reload so the WS caller
	// sees the freshest TaskPR rows.
	return s.store.ListTaskPRsByTask(ctx, taskID)
}

// areAllWatchesPermanentlyMissing reports whether every supplied watch's
// (owner, repo) is currently in the 10-min negative cache. Used to set
// the permanent flag on wsSyncTaskPR's response so the frontend stops
// retrying when a task points only at deleted/inaccessible repositories.
// Returns false when the input is empty so an empty task doesn't get
// classified as permanently missing.
func (s *Service) areAllWatchesPermanentlyMissing(watches []*PRWatch) bool {
	if len(watches) == 0 {
		return false
	}
	for _, w := range watches {
		if !s.isRepoCachedAsMissing(w.Owner, w.Repo) {
			return false
		}
	}
	return true
}

// triggerPRSyncAllPerWatch is the legacy fan-out path: one gh subprocess
// per watch. Kept as a fallback for the NoopClient and for the rare case
// where the batched GraphQL call fails (auth glitch, network blip) so a
// single bad cycle doesn't leave the UI staring at stale data.
func (s *Service) triggerPRSyncAllPerWatch(ctx context.Context, taskID string, watches []*PRWatch) ([]*TaskPR, error) {
	results := make([]*TaskPR, 0, len(watches))
	var syncErrs []error
	for _, w := range watches {
		var tp *TaskPR
		var syncErr error
		if w.PRNumber == 0 {
			tp, syncErr = s.triggerPRDetection(ctx, w, taskID)
		} else {
			tp, syncErr = s.triggerPRStatusSync(ctx, w, taskID)
		}
		if syncErr != nil {
			// Debug, not Warn: this is best-effort background reconciliation and
			// the most common failure — a branch with no PR yet, or an
			// unresolvable repo — is an expected steady state, not an
			// operational warning. The background poller logs the same failure
			// at Debug (see poller.detectPRForWatch / checkSinglePRWatch); the
			// WS handler still returns the error to the caller.
			s.logger.Debug("per-repo PR sync failed",
				zap.String("task_id", taskID),
				zap.String("repository_id", w.RepositoryID),
				zap.Int("pr_number", w.PRNumber),
				zap.Error(syncErr))
			syncErrs = append(syncErrs, fmt.Errorf("%s/%s#%d: %w", w.Owner, w.Repo, w.PRNumber, syncErr))
			continue
		}
		if tp != nil {
			results = append(results, tp)
		}
	}
	if len(syncErrs) == 0 {
		return results, nil
	}
	err := errors.Join(syncErrs...)
	if len(results) > 0 {
		return results, &PartialPRSyncError{Err: err}
	}
	return results, err
}

// PartialPRSyncError reports that some PR watches failed while others synced.
type PartialPRSyncError struct {
	Err error
}

func (e *PartialPRSyncError) Error() string {
	if e == nil || e.Err == nil {
		return "partial PR sync failure"
	}
	return e.Err.Error()
}

func (e *PartialPRSyncError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (s *Service) triggerPRDetection(ctx context.Context, watch *PRWatch, taskID string) (*TaskPR, error) {
	if s.client == nil {
		return nil, nil
	}
	// Coalesce concurrent detection probes for the same watch. Without this the
	// freshness check below is racy: parallel callers (e.g. the 5s frontend
	// retry racing the workspace background refresh) can all read a stale
	// last_checked_at and probe GitHub simultaneously, defeating the throttle.
	// Keyed distinctly from triggerPRStatusSync's owner/repo/number key so the
	// two never collide. singleflight only blocks same-key calls, so the nested
	// triggerPRStatusSync (different key) below cannot deadlock.
	key := "pr-detect:" + watch.ID
	v, err, _ := s.syncGroup.Do(key, func() (interface{}, error) {
		return s.detectPRForWatchOnce(ctx, watch, taskID)
	})
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	return v.(*TaskPR), nil
}

// detectPRForWatchOnce performs a single branch-detection probe for a
// searching (pr_number=0) watch. It runs inside triggerPRDetection's
// singleflight so only one probe per watch is in flight at a time.
func (s *Service) detectPRForWatchOnce(ctx context.Context, watch *PRWatch, taskID string) (*TaskPR, error) {
	// Short-circuit when this repo is already in the 10-min negative
	// cache. Without this, an unresolvable repo gets a fresh per-watch
	// probe every 5s (the frontend retry cadence) until the freshness
	// timestamp below stamps in — multiplied by every watch the task has.
	if s.isRepoCachedAsMissing(watch.Owner, watch.Repo) {
		return nil, ErrRepoNotResolvable
	}
	// Throttle branch-detection probes. A watch still searching for its PR
	// (pr_number=0) has no TaskPR row yet, so triggerPRStatusSync's freshness
	// check can't gate it — we gate on the watch's own last_checked_at instead.
	// Without this, a branch whose PR never appears (e.g. an unresolvable repo)
	// re-hits `gh` on every on-demand sync, and the frontend re-syncs every 5s
	// while no PR is found — flooding the logs with identical failures.
	if watch.LastCheckedAt != nil && time.Since(*watch.LastCheckedAt) < PRSyncFreshnessWindow {
		return s.store.GetTaskPRByRepository(ctx, taskID, watch.RepositoryID)
	}

	// Snapshot the negative-cache generation BEFORE the fetch so an
	// eviction (CreatePRWatch / EnsurePRWatch / explicit clear) that
	// fires WHILE FindPRByBranch is running wins over our post-fetch
	// classification — see Service.markRepoAsMissing for the rationale.
	repoErrGen := s.repoErrorGenSnapshot()
	pr, err := s.client.FindPRByBranch(ctx, watch.Owner, watch.Repo, watch.Branch)
	if err != nil && isRepoNotResolvableErr(err) {
		s.markRepoAsMissing(watch.Owner, watch.Repo, repoErrGen)
		// Wrap so wsSyncTaskPR can errors.Is(err, ErrRepoNotResolvable)
		// to flag the WS response as permanent without re-running the
		// classifier on the raw upstream error string.
		err = fmt.Errorf("%w: %w", ErrRepoNotResolvable, err)
	}
	// Stamp last_checked_at regardless of outcome — including on error. The
	// flood this fixes IS the error path (an unresolvable repo makes `gh` exit
	// non-zero), so stamping only on success would leave the bug unfixed. The
	// tradeoff: a transient GitHub error throttles the next on-demand probe for
	// PRSyncFreshnessWindow, which is fine — it stops hammering a failing
	// endpoint, and the 60s background poller still retries. This diverges
	// intentionally from poller.detectPRForWatch, which stamps only on success.
	// The empty-string check/review args clear last_check_status /
	// last_review_state. That's harmless here — this path only runs for
	// pr_number=0 (searching) watches, which never carry those fields — and
	// matches the poller's detection stamp. Only call this on searching
	// watches; for a resolved PR use the status-sync path instead.
	now := time.Now().UTC()
	if tsErr := s.store.UpdatePRWatchTimestamps(ctx, watch.ID, now, nil, "", ""); tsErr != nil {
		s.logger.Debug("failed to stamp PR watch after detection probe",
			zap.String("watch_id", watch.ID), zap.Error(tsErr))
	}
	if err != nil || pr == nil {
		return nil, err
	}
	if err := s.store.UpdatePRWatchPRNumber(ctx, watch.ID, pr.Number); err != nil {
		s.logger.Error("failed to update PR watch number during sync",
			zap.String("watch_id", watch.ID), zap.Int("pr_number", pr.Number), zap.Error(err))
		return nil, fmt.Errorf("update PR watch: %w", err)
	}
	if _, assocErr := s.AssociatePRWithTask(ctx, taskID, watch.RepositoryID, pr); assocErr != nil {
		s.logger.Error("failed to associate PR with task during sync",
			zap.String("task_id", taskID), zap.Int("pr_number", pr.Number), zap.Error(assocErr))
		return nil, fmt.Errorf("associate PR: %w", assocErr)
	}
	// Also fetch status so the first response includes review/check state
	watch.PRNumber = pr.Number
	return s.triggerPRStatusSync(ctx, watch, taskID)
}

func (s *Service) triggerPRStatusSync(ctx context.Context, watch *PRWatch, taskID string) (*TaskPR, error) {
	// Freshness check: skip GitHub API if this exact PR row was recently
	// synced. Multi-branch tasks can have multiple PRs in the same repo, so a
	// repo-only lookup would let a fresh sibling suppress this PR's sync.
	loadTaskPR := func(c context.Context) (*TaskPR, error) {
		tp, err := s.store.GetTaskPRByRepoAndNumber(c, taskID, watch.RepositoryID, watch.PRNumber)
		if err != nil {
			return nil, err
		}
		if tp != nil {
			return tp, nil
		}
		// Fall back to the legacy untagged row for single-repo tasks that
		// haven't been re-associated under the multi-repo schema yet.
		if watch.RepositoryID != "" {
			return nil, nil
		}
		return s.store.GetTaskPR(c, taskID)
	}
	if tp, _ := loadTaskPR(ctx); tp != nil && tp.LastSyncedAt != nil {
		if time.Since(*tp.LastSyncedAt) < PRSyncFreshnessWindow {
			return tp, nil
		}
	}

	// Short-circuit on negative cache so a 5s frontend retry against a
	// dead repo doesn't burn the gh throttle. Eviction happens on watch
	// (re)create paths, so a freshly linked repo is probed immediately.
	if s.isRepoCachedAsMissing(watch.Owner, watch.Repo) {
		return nil, ErrRepoNotResolvable
	}

	// Coalesce concurrent syncs for the same PR
	key := fmt.Sprintf("%s/%s/%d", watch.Owner, watch.Repo, watch.PRNumber)
	v, err, _ := s.syncGroup.Do(key, func() (interface{}, error) {
		bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		// Snapshot cache generation BEFORE the per-watch probe so a
		// concurrent eviction wins; see Service.markRepoAsMissing.
		repoErrGen := s.repoErrorGenSnapshot()
		status, _, checkErr := s.CheckPRWatch(bgCtx, watch)
		if checkErr != nil {
			if isRepoNotResolvableErr(checkErr) {
				s.markRepoAsMissing(watch.Owner, watch.Repo, repoErrGen)
				return nil, fmt.Errorf("%w: %w", ErrRepoNotResolvable, checkErr)
			}
			return nil, checkErr
		}
		if status == nil {
			return loadTaskPR(bgCtx)
		}
		if existing, loadErr := loadTaskPR(bgCtx); loadErr != nil {
			return nil, loadErr
		} else if existing == nil && status.PR != nil {
			// Gap-fill a numbered watch whose exact task_pr row was never
			// created. AssociatePRWithTask publishes the creation event; the
			// following SyncTaskPR may publish again if status fields changed.
			// That double event is harmless because clients re-fetch state.
			if _, assocErr := s.AssociatePRWithTask(bgCtx, taskID, watch.RepositoryID, status.PR); assocErr != nil {
				return nil, assocErr
			}
		}
		if syncErr := s.SyncTaskPR(bgCtx, taskID, status); syncErr != nil {
			return nil, syncErr
		}
		return loadTaskPR(bgCtx)
	})
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	return v.(*TaskPR), nil
}
