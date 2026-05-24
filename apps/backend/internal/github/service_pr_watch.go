package github

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// --- PR Watch operations ---

// CreatePRWatch creates a new PR watch for a (session, repository) pair.
// `repositoryID` may be empty for legacy single-repo callers; multi-repo
// callers must pass the per-task repository_id so each repo gets its own
// watch row.
func (s *Service) CreatePRWatch(ctx context.Context, sessionID, taskID, repositoryID, owner, repo string, prNumber int, branch string) (*PRWatch, error) {
	existing, err := s.store.GetPRWatchBySessionAndRepo(ctx, sessionID, repositoryID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil // already watching this (session, repo)
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

	// Update watch timestamps
	now := time.Now().UTC()
	if err := s.store.UpdatePRWatchTimestamps(ctx, watch.ID, now, nil, status.ChecksState, status.ReviewState); err != nil {
		s.logger.Error("failed to update PR watch timestamps", zap.String("id", watch.ID), zap.Error(err))
	}

	return status, hasNew, nil
}

// EnsurePRWatch creates a PRWatch with pr_number=0 for a (session, repo) pair
// if one doesn't already exist. The poller will detect the PR by searching
// for the branch on GitHub. `repositoryID` is empty for legacy single-repo
// callers; multi-repo callers MUST pass the per-task repository_id so each
// repo gets its own watch (the table's UNIQUE(session_id, repository_id) used
// to be UNIQUE(session_id), which silently dropped second-repo watches).
func (s *Service) EnsurePRWatch(ctx context.Context, sessionID, taskID, repositoryID, owner, repo, branch string) (*PRWatch, error) {
	existing, err := s.store.GetPRWatchBySessionAndRepo(ctx, sessionID, repositoryID)
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
	// Check for an existing PR for this exact (task, repo). Multi-repo callers
	// must scope by repository_id so the same PR number in two repos doesn't
	// short-circuit the second association.
	existing, err := s.store.GetTaskPRByRepository(ctx, taskID, repositoryID)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.PRNumber == pr.Number {
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
	// ReplaceTaskPR atomically deletes any existing association for the
	// (task, repository) pair and inserts the new row inside one transaction.
	// Scoping by repository_id keeps multi-repo tasks intact; legacy callers
	// (repositoryID == "") still get the "delete all" semantics.
	if err := s.store.ReplaceTaskPR(ctx, tp); err != nil {
		return nil, fmt.Errorf("replace task PR: %w", err)
	}
	if existing != nil {
		s.logger.Info("replaced stale task PR association",
			zap.String("task_id", taskID),
			zap.String("repository_id", repositoryID),
			zap.Int("old_pr_number", existing.PRNumber),
			zap.Int("new_pr_number", pr.Number))
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

// ListTaskPRs returns PR associations for multiple tasks, grouped by task_id.
// Multi-repo tasks may have more than one PR per task.
func (s *Service) ListTaskPRs(ctx context.Context, taskIDs []string) (map[string][]*TaskPR, error) {
	return s.store.ListTaskPRsByTaskIDs(ctx, taskIDs)
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
	// if any of its PRs are stale; the sync is per-task so we only need to
	// queue each task once.
	staleTasks := make(map[string]struct{})
	for taskID, prs := range result {
		for _, tp := range prs {
			if tp.LastSyncedAt == nil || time.Since(*tp.LastSyncedAt) >= prSyncFreshnessWindow {
				staleTasks[taskID] = struct{}{}
				break
			}
		}
	}
	staleTaskIDs := make([]string, 0, len(staleTasks))
	for id := range staleTasks {
		staleTaskIDs = append(staleTaskIDs, id)
	}

	// Background refresh with bounded concurrency
	if len(staleTaskIDs) > 0 {
		go func() {
			sem := make(chan struct{}, 5)
			for _, taskID := range staleTaskIDs {
				sem <- struct{}{}
				go func(id string) {
					defer func() { <-sem }()
					syncCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					if _, syncErr := s.TriggerPRSyncAll(syncCtx, id); syncErr != nil {
						s.logger.Debug("background PR sync failed", zap.String("task_id", id), zap.Error(syncErr))
					}
				}(taskID)
			}
		}()
	}

	return result, nil
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

	changed := tp.State != status.PR.State ||
		tp.PRTitle != status.PR.Title ||
		tp.Additions != status.PR.Additions ||
		tp.Deletions != status.PR.Deletions ||
		tp.ReviewState != status.ReviewState ||
		tp.ChecksState != status.ChecksState ||
		tp.MergeableState != status.MergeableState ||
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
	tp.MergeableState = status.MergeableState
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
func (s *Service) TriggerPRSyncAll(ctx context.Context, taskID string) ([]*TaskPR, error) {
	watches, err := s.store.ListPRWatchesByTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("list PR watches: %w", err)
	}
	if len(watches) == 0 {
		// No watches — fall back to whatever TaskPRs already exist (e.g.
		// PRs imported via task-create-from-PR-URL where the watch is
		// optional). Empty slice if none.
		existing, listErr := s.store.ListTaskPRsByTask(ctx, taskID)
		if listErr != nil {
			return nil, fmt.Errorf("list task PRs: %w", listErr)
		}
		return existing, nil
	}
	results := make([]*TaskPR, 0, len(watches))
	for _, w := range watches {
		var tp *TaskPR
		var syncErr error
		if w.PRNumber == 0 {
			tp, syncErr = s.triggerPRDetection(ctx, w, taskID)
		} else {
			tp, syncErr = s.triggerPRStatusSync(ctx, w, taskID)
		}
		if syncErr != nil {
			s.logger.Warn("per-repo PR sync failed",
				zap.String("task_id", taskID),
				zap.String("repository_id", w.RepositoryID),
				zap.Int("pr_number", w.PRNumber),
				zap.Error(syncErr))
			continue
		}
		if tp != nil {
			results = append(results, tp)
		}
	}
	return results, nil
}

func (s *Service) triggerPRDetection(ctx context.Context, watch *PRWatch, taskID string) (*TaskPR, error) {
	if s.client == nil {
		return nil, nil
	}
	pr, err := s.client.FindPRByBranch(ctx, watch.Owner, watch.Repo, watch.Branch)
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
	// Freshness check: skip GitHub API if recently synced. Look up by the
	// watch's own (task, repo) — the legacy GetTaskPR(ctx, taskID) returned
	// "first match" which for multi-repo tasks would mistakenly hit the
	// other repo's row and skip the sync that this watch actually needs.
	loadTaskPR := func(c context.Context) (*TaskPR, error) {
		tp, err := s.store.GetTaskPRByRepository(c, taskID, watch.RepositoryID)
		if err != nil {
			return nil, err
		}
		if tp != nil {
			return tp, nil
		}
		// Fall back to the legacy untagged row for single-repo tasks that
		// haven't been re-associated under the multi-repo schema yet.
		return s.store.GetTaskPR(c, taskID)
	}
	if tp, _ := loadTaskPR(ctx); tp != nil && tp.LastSyncedAt != nil {
		if time.Since(*tp.LastSyncedAt) < prSyncFreshnessWindow {
			return tp, nil
		}
	}

	// Coalesce concurrent syncs for the same PR
	key := fmt.Sprintf("%s/%s/%d", watch.Owner, watch.Repo, watch.PRNumber)
	v, err, _ := s.syncGroup.Do(key, func() (interface{}, error) {
		bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		status, _, checkErr := s.CheckPRWatch(bgCtx, watch)
		if checkErr != nil {
			return nil, checkErr
		}
		if status == nil {
			return loadTaskPR(bgCtx)
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
