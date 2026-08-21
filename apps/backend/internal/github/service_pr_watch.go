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
	return s.createPRWatch(ctx, "", sessionID, taskID, repositoryID, owner, repo, prNumber, branch)
}

func (s *Service) CreatePRWatchForWorkspace(
	ctx context.Context, workspaceID, sessionID, taskID, repositoryID, owner, repo string, prNumber int, branch string,
) (*PRWatch, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, ErrGitHubWorkspaceRequired
	}
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return nil, err
	}
	return s.createPRWatch(ctx, workspaceID, sessionID, taskID, repositoryID, owner, repo, prNumber, branch)
}

func (s *Service) createPRWatch(
	ctx context.Context, workspaceID, sessionID, taskID, repositoryID, owner, repo string, prNumber int, branch string,
) (*PRWatch, error) {
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
		WorkspaceID:  workspaceID,
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

// GetPRWatch returns one PR watch after authorizing its stored workspace.
func (s *Service) GetPRWatch(ctx context.Context, id string) (*PRWatch, error) {
	watch, err := s.store.GetPRWatch(ctx, id)
	if err != nil || watch == nil {
		return watch, err
	}
	if err := s.authorizeWorkspaceAccess(ctx, watch.WorkspaceID); err != nil {
		return nil, err
	}
	return watch, nil
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

// ListActivePRWatchesForWorkspace returns active PR watches for one authorized
// workspace. The all-workspaces variant is reserved for identity-less pollers.
func (s *Service) ListActivePRWatchesForWorkspace(ctx context.Context, workspaceID string) ([]*PRWatch, error) {
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return nil, err
	}
	return s.store.ListActivePRWatchesForWorkspace(ctx, workspaceID)
}

// DeletePRWatch deletes a PR watch by ID.
func (s *Service) DeletePRWatch(ctx context.Context, id string) error {
	watch, err := s.store.GetPRWatch(ctx, id)
	if err != nil {
		return err
	}
	if watch != nil {
		if err := s.authorizeWorkspaceAccess(ctx, watch.WorkspaceID); err != nil {
			return err
		}
	}
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

// UpdatePRWatchRepository updates a watch's provider repository identity after
// PR discovery. This keeps future status polling on the repository that owns
// the PR rather than the repository used to discover the branch.
func (s *Service) UpdatePRWatchRepository(ctx context.Context, id, owner, repo string) error {
	return s.store.UpdatePRWatchRepository(ctx, id, owner, repo)
}

func (s *Service) rebindPRWatchRepository(ctx context.Context, watch *PRWatch, pr *PR) error {
	if watch == nil || pr == nil || strings.TrimSpace(pr.RepoOwner) == "" || strings.TrimSpace(pr.RepoName) == "" {
		return nil
	}
	if sameRepositoryIdentity(watch.Owner, watch.Repo, pr.RepoOwner, pr.RepoName) {
		return nil
	}
	if err := s.store.UpdatePRWatchRepository(ctx, watch.ID, pr.RepoOwner, pr.RepoName); err != nil {
		return fmt.Errorf("update PR watch repository: %w", err)
	}
	watch.Owner = pr.RepoOwner
	watch.Repo = pr.RepoName
	return nil
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
	return s.checkPRWatchWithClient(ctx, s.client, watch)
}

func (s *Service) CheckPRWatchForWorkspace(ctx context.Context, watch *PRWatch) (*PRStatus, bool, error) {
	if watch == nil || strings.TrimSpace(watch.WorkspaceID) == "" {
		return nil, false, ErrGitHubWorkspaceRequired
	}
	resolved, err := s.resolveAutomationClient(ctx, watch.WorkspaceID, watch.Owner, watch.Repo)
	if err != nil {
		return nil, false, err
	}
	return s.checkPRWatchWithClient(ctx, resolved.Client, watch)
}

func (s *Service) checkPRWatchWithClient(
	ctx context.Context, client Client, watch *PRWatch,
) (*PRStatus, bool, error) {
	status, err := client.GetPRStatus(ctx, watch.Owner, watch.Repo, watch.PRNumber)
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
	return s.ensurePRWatch(ctx, "", sessionID, taskID, repositoryID, owner, repo, branch)
}

func (s *Service) EnsurePRWatchForWorkspace(
	ctx context.Context, workspaceID, sessionID, taskID, repositoryID, owner, repo, branch string,
) (*PRWatch, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, ErrGitHubWorkspaceRequired
	}
	return s.ensurePRWatch(ctx, workspaceID, sessionID, taskID, repositoryID, owner, repo, branch)
}

func (s *Service) ensurePRWatch(
	ctx context.Context, workspaceID, sessionID, taskID, repositoryID, owner, repo, branch string,
) (*PRWatch, error) {
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
		WorkspaceID:  workspaceID,
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
	// pr here comes from branch-search discovery (poller.detectPRForWatch) or
	// a batched status result of unknown populated-ness — never assert
	// outcomeFieldsPopulated for this wrapper's callers (AC-11).
	return s.associatePRWithTask(ctx, "", taskID, repositoryID, pr, false, false)
}

func (s *Service) AssociatePRWithTaskForWorkspace(
	ctx context.Context, workspaceID, taskID, repositoryID string, pr *PR,
) (*TaskPR, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, ErrGitHubWorkspaceRequired
	}
	return s.associatePRWithTask(ctx, workspaceID, taskID, repositoryID, pr, false, false)
}

// associatePRWithTask creates the task-PR association. outcomeFieldsPopulated
// tells it whether pr came from a full single-PR fetch (AC-10) — only the
// direct GetPR-based callers in this file set it true; the two public
// wrappers above always pass false, preserving their existing callers'
// behavior exactly, because a branch-search or batched-status pr may not
// carry real observations for these fields (AC-11).
func (s *Service) associatePRWithTask(
	ctx context.Context, workspaceID, taskID, repositoryID string, pr *PR, restoreDetached, outcomeFieldsPopulated bool,
) (*TaskPR, error) {
	// Multi-branch: scope the "already-current" short-circuit by exact
	// pr_number too. A task can hold multiple PR rows per (task, repo) on
	// different branches; the legacy by-repo lookup returns whichever row
	// was most recently updated, which would make Associate think the
	// secondary PR is already there (wrong PR number) and skip the insert
	// — or worse, fall through to ReplaceTaskPR which used to delete the
	// sibling row.
	existing, err := s.store.GetTaskPRByRepoAndNumberIncludingDetached(ctx, taskID, repositoryID, pr.Number)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if existing.DetachedAt != nil && restoreDetached {
			// RestoreTaskPR resolves the five outcome-attribution columns
			// itself, inside its own transaction, against the row it is
			// about to overwrite (AC-43, AC-43a) — this caller must not
			// pre-resolve them via resolveTaskPROutcomeFields, since a
			// value resolved here would be stale by the time the store's
			// statement executes.
			restored, restoreErr := s.store.RestoreTaskPR(
				ctx, taskID, repositoryID, &PRStatus{PR: pr, OutcomeFieldsPopulated: outcomeFieldsPopulated},
			)
			if restoreErr != nil || restored == nil {
				return restored, restoreErr
			}
			if s.eventBus != nil {
				event := bus.NewEvent(events.GitHubTaskPRUpdated, "github", restored)
				if err := s.eventBus.Publish(ctx, events.GitHubTaskPRUpdated, event); err != nil {
					s.logger.Debug("failed to publish restored task PR event", zap.Error(err))
				}
			}
			s.reconcileComparisonTarget(ctx, taskID, pr)
			return restored, nil
		}
		s.reconcileComparisonTarget(ctx, taskID, pr)
		return existing, nil
	}
	tp := &TaskPR{
		WorkspaceID:  workspaceID,
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
	// ReplaceTaskPR upserts the row matching (task, repository, pr_number)
	// and resolves the five outcome-attribution columns itself, inside its
	// own transaction, against the row it is about to overwrite (AC-43,
	// AC-43a) — this caller must not pre-resolve them. Multi-branch tasks
	// may already hold sibling rows for the SAME (task, repository) on
	// different PR numbers — ReplaceTaskPR no longer touches them. The
	// early-return above guarantees we only reach this line when no row for
	// the exact pr_number exists yet, so this is a straight insert in
	// steady state; the delete-then-insert form is retained so a retry that
	// races a partial write resolves cleanly.
	replaced, err := s.store.ReplaceTaskPR(ctx, tp, &PRStatus{PR: pr, OutcomeFieldsPopulated: outcomeFieldsPopulated})
	if err != nil {
		return nil, fmt.Errorf("replace task PR: %w", err)
	}
	tp = replaced

	// Publish event for UI
	if s.eventBus != nil {
		event := bus.NewEvent(events.GitHubTaskPRUpdated, "github", tp)
		if err := s.eventBus.Publish(ctx, events.GitHubTaskPRUpdated, event); err != nil {
			s.logger.Debug("failed to publish task PR updated event", zap.Error(err))
		}
	}
	s.reconcileComparisonTarget(ctx, taskID, pr)

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
	tp, err := s.associatePRWithTask(ctx, "", taskID, repositoryID, pr, true, true)
	if err != nil {
		return nil, fmt.Errorf("associate PR with task: %w", err)
	}
	return tp, nil
}

func (s *Service) AssociateExistingPRByURLForWorkspace(
	ctx context.Context, workspaceID, userID, taskID, repositoryID, prURL string,
) (*TaskPR, error) {
	owner, repo, prNumber, err := parsePRURL(prURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidPRURL, err)
	}
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return nil, err
	}
	resolved, err := s.resolvePersonalReadClient(ctx, workspaceID, userID, owner, repo)
	if err != nil {
		return nil, err
	}
	pr, err := resolved.Client.GetPR(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, fmt.Errorf("fetch PR: %w", err)
	}
	tp, err := s.associatePRWithTask(ctx, workspaceID, taskID, repositoryID, pr, true, true)
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
	if _, assocErr := s.associatePRWithTask(ctx, "", taskID, repositoryID, pr, true, true); assocErr != nil {
		s.logger.Error("failed to associate PR with task after creation",
			zap.String("task_id", taskID), zap.Error(assocErr))
	}
}

func (s *Service) AssociatePRByURLForWorkspace(
	ctx context.Context, workspaceID, userID, sessionID, taskID, repositoryID, prURL, branch string,
) error {
	owner, repo, prNumber, err := parsePRURL(prURL)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidPRURL, err)
	}
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return err
	}
	resolved, err := s.resolvePersonalReadClient(ctx, workspaceID, userID, owner, repo)
	if err != nil {
		return err
	}
	pr, err := resolved.Client.GetPR(ctx, owner, repo, prNumber)
	if err != nil {
		return fmt.Errorf("fetch PR: %w", err)
	}
	if branch == "" {
		branch = pr.HeadBranch
	}
	if _, err := s.CreatePRWatchForWorkspace(
		ctx, workspaceID, sessionID, taskID, repositoryID, owner, repo, prNumber, branch,
	); err != nil {
		return fmt.Errorf("create PR watch: %w", err)
	}
	if _, err := s.associatePRWithTask(ctx, workspaceID, taskID, repositoryID, pr, true, true); err != nil {
		return fmt.Errorf("associate PR with task: %w", err)
	}
	return nil
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
		allWatches, unwatched := s.collectStaleWorkspaceSyncTargets(syncCtx, staleTasks)
		// PRs left behind by a branch handover have no watch to fan in, so
		// they would stay stale on every workspace load. Reconcile them in
		// one batched call for the whole workspace; once a row reaches a
		// terminal state it drops out of this set permanently.
		if len(unwatched) > 0 {
			s.syncUnwatchedTaskPRs(syncCtx, unwatched, workspaceID)
		}
		if len(allWatches) == 0 {
			return
		}
		if _, err := s.SyncWorkspaceWatchesBatched(syncCtx, workspaceID, allWatches); err != nil {
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

type taskPRSyncState struct {
	checksTotal, checksPassing                  int
	unresolved, reviewCount, pendingReviewCount int
	requiredReviews                             *int
	baseBranch, mergeableState, state           string
	mergedAt, closedAt                          *time.Time
	isDraft                                     *bool
	changedFiles                                *int
	mergedByLogin, closedByLogin                *string
	autoMergeObservedAt                         *time.Time
}

// resolveTaskPROutcomeFields applies the populated/preserve dance for the
// five outcome-attribution columns, split out of prepareTaskPRSyncState to
// keep that function's complexity within the repo's lint limits (see the
// cyclomatic-complexity suppression on SyncTaskPR below).
//
//   - is_draft / changed_files each follow their own presence flag
//     (status.PR.IsDraftObserved / status.PR.ChangedFilesObserved) rather
//     than the whole-group status.OutcomeFieldsPopulated: a group marked
//     populated is not a promise that every field in it arrived, so a
//     populated sync whose upstream response omitted or nulled one of these
//     two writes NULL for that column specifically, not the decoder's zero
//     value (AC-12a). merged_by_login stays keyed off the group flag alone
//     (nil merged_by_login when upstream reports no merger, never "").
//     An unpopulated group preserves all three stored values verbatim,
//     including NULL (AC-13).
//   - closed_by_login follows status.ClosureAttributionPopulated the same
//     way (AC-14, AC-15) — only the GraphQL path ever sets it.
//   - auto_merge_observed_at is a latch: set once, on the first populating
//     sync that observes auto-merge armed while the stored value is NULL,
//     and never cleared or overwritten afterwards (AC-16, AC-17).
func resolveTaskPROutcomeFields(tp *TaskPR, status *PRStatus) (
	isDraft *bool, changedFiles *int, mergedByLogin, closedByLogin *string, autoMergeObservedAt *time.Time,
) {
	isDraft, changedFiles, mergedByLogin = tp.IsDraft, tp.ChangedFiles, tp.MergedByLogin
	if status.OutcomeFieldsPopulated {
		isDraft = nil
		if status.PR.IsDraftObserved {
			draft := status.PR.Draft
			isDraft = &draft
		}
		changedFiles = nil
		if status.PR.ChangedFilesObserved {
			files := status.PR.ChangedFiles
			changedFiles = &files
		}
		mergedByLogin = nil
		if status.PR.MergedByLogin != "" {
			login := status.PR.MergedByLogin
			mergedByLogin = &login
		}
	}

	closedByLogin = tp.ClosedByLogin
	if status.ClosureAttributionPopulated {
		login := status.ClosedByLogin
		closedByLogin = &login
	}

	autoMergeObservedAt = tp.AutoMergeObservedAt
	if status.OutcomeFieldsPopulated && status.PR.AutoMergeEnabled && tp.AutoMergeObservedAt == nil {
		now := time.Now().UTC()
		autoMergeObservedAt = &now
	}
	return isDraft, changedFiles, mergedByLogin, closedByLogin, autoMergeObservedAt
}

// resolveTerminalMergeState keeps a row that already reached "merged" there.
// GitHub cannot un-merge a PR, so a live read reporting anything else for a
// merged row is stale — an eventually-consistent REST reply, or a fetch that
// started before a concurrent writer observed the merge. Without this, two
// writers racing the same PR could flip its badge back to green.
//
// Only "merged" is guarded. The PR detail panel used to apply a full
// merged > closed > open rank client-side; that is deliberately not ported,
// because closed -> open is a real transition (a PR can be reopened on GitHub)
// and ranking it as a regression would pin every reopened PR closed forever.
func resolveTerminalMergeState(tp *TaskPR, pr *PR) (string, *time.Time, *time.Time) {
	if tp.State == prStateMerged && pr.State != prStateMerged {
		return tp.State, tp.MergedAt, tp.ClosedAt
	}
	return pr.State, pr.MergedAt, pr.ClosedAt
}

func (s *Service) prepareTaskPRSyncState(ctx context.Context, tp *TaskPR, status *PRStatus) taskPRSyncState {
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
	} else if fetched := s.fetchRequiredReviewsForTaskPR(ctx, tp, nextBaseBranch); fetched != nil {
		nextRequiredReviews = fetched
	}
	// GitHub reports draft status separately from mergeStateStatus and can
	// return CLEAN for a draft PR. Persist the effective blocker so every
	// TaskPR consumer agrees that drafts are not ready to merge.
	nextMergeableState := status.MergeableState
	if status.PR.Draft {
		nextMergeableState = "draft"
	}
	nextState, nextMergedAt, nextClosedAt := resolveTerminalMergeState(tp, status.PR)
	nextIsDraft, nextChangedFiles, nextMergedByLogin, nextClosedByLogin, nextAutoMergeObservedAt :=
		resolveTaskPROutcomeFields(tp, status)
	return taskPRSyncState{
		checksTotal: nextChecksTotal, checksPassing: nextChecksPassing,
		unresolved: nextUnresolved, reviewCount: nextReviewCount, pendingReviewCount: nextPendingReviewCount,
		requiredReviews: nextRequiredReviews, baseBranch: nextBaseBranch, mergeableState: nextMergeableState,
		state: nextState, mergedAt: nextMergedAt, closedAt: nextClosedAt,
		isDraft: nextIsDraft, changedFiles: nextChangedFiles,
		mergedByLogin: nextMergedByLogin, closedByLogin: nextClosedByLogin,
		autoMergeObservedAt: nextAutoMergeObservedAt,
	}
}

func taskPRChangedFields(tp *TaskPR, status *PRStatus, next taskPRSyncState) []string {
	if tp == nil || status == nil || status.PR == nil {
		return nil
	}
	changed := make([]string, 0, 16)
	changed = appendChangedField(changed, "state", tp.State != next.state)
	changed = appendChangedField(changed, "pr_title", tp.PRTitle != status.PR.Title)
	changed = appendChangedField(changed, "additions", tp.Additions != status.PR.Additions)
	changed = appendChangedField(changed, "deletions", tp.Deletions != status.PR.Deletions)
	changed = appendChangedField(changed, "review_state", tp.ReviewState != status.ReviewState)
	changed = appendChangedField(changed, "checks_state", tp.ChecksState != status.ChecksState)
	changed = appendChangedField(changed, "mergeable_state", tp.MergeableState != next.mergeableState)
	changed = appendChangedField(changed, "review_count", tp.ReviewCount != next.reviewCount)
	changed = appendChangedField(
		changed,
		"pending_review_count",
		tp.PendingReviewCount != next.pendingReviewCount,
	)
	changed = appendChangedField(
		changed,
		"required_reviews",
		!intPtrEqual(tp.RequiredReviews, next.requiredReviews),
	)
	changed = appendChangedField(changed, "checks_total", tp.ChecksTotal != next.checksTotal)
	changed = appendChangedField(changed, "checks_passing", tp.ChecksPassing != next.checksPassing)
	changed = appendChangedField(
		changed,
		"unresolved_review_threads",
		tp.UnresolvedReviewThreads != next.unresolved,
	)
	changed = appendChangedField(changed, "base_branch", tp.BaseBranch != next.baseBranch)
	changed = appendChangedField(changed, "merged_at", !timeEqual(tp.MergedAt, next.mergedAt))
	changed = appendChangedField(changed, "closed_at", !timeEqual(tp.ClosedAt, next.closedAt))
	changed = appendChangedField(changed, "is_draft", !boolPtrEqual(tp.IsDraft, next.isDraft))
	changed = appendChangedField(changed, "changed_files", !intPtrEqual(tp.ChangedFiles, next.changedFiles))
	changed = appendChangedField(changed, "merged_by_login", !stringPtrEqual(tp.MergedByLogin, next.mergedByLogin))
	changed = appendChangedField(changed, "closed_by_login", !stringPtrEqual(tp.ClosedByLogin, next.closedByLogin))
	changed = appendChangedField(
		changed,
		"auto_merge_observed_at",
		!timeEqual(tp.AutoMergeObservedAt, next.autoMergeObservedAt),
	)
	return changed
}

func appendChangedField(changed []string, field string, isChanged bool) []string {
	if !isChanged {
		return changed
	}
	return append(changed, field)
}

// SyncTaskPR updates a TaskPR record with the latest PR status. Multi-repo:
// the row is found by (task_id, owner, repo, pr_number) since the same
// task can have several PRs; the legacy GetTaskPR(taskID) "first match"
// would cross repos and silently update the wrong row.
// It only publishes a github.task_pr.updated event when data actually changed,
// preventing feedback loops with frontend sync handlers.
//
//nolint:cyclop // sequential field-by-field reconciliation keeps update intent clear
func (s *Service) SyncTaskPR(ctx context.Context, taskID string, status *PRStatus) error {
	if status == nil || status.PR == nil {
		return fmt.Errorf("sync task PR: missing PR data for task %s", taskID)
	}
	tp, err := s.findTaskPRForStatus(ctx, taskID, status.PR)
	if err != nil || tp == nil {
		return err
	}
	next := s.prepareTaskPRSyncState(ctx, tp, status)

	changedFields := taskPRChangedFields(tp, status, next)
	changed := len(changedFields) > 0

	tp.State = next.state
	tp.PRTitle = status.PR.Title
	tp.Additions = status.PR.Additions
	tp.Deletions = status.PR.Deletions
	tp.MergedAt = next.mergedAt
	tp.ClosedAt = next.closedAt
	tp.ReviewState = status.ReviewState
	tp.ChecksState = status.ChecksState
	tp.MergeableState = next.mergeableState
	tp.ReviewCount = next.reviewCount
	tp.PendingReviewCount = next.pendingReviewCount
	tp.RequiredReviews = next.requiredReviews
	tp.ChecksTotal = next.checksTotal
	tp.ChecksPassing = next.checksPassing
	tp.UnresolvedReviewThreads = next.unresolved
	tp.BaseBranch = next.baseBranch
	tp.IsDraft = next.isDraft
	tp.ChangedFiles = next.changedFiles
	tp.MergedByLogin = next.mergedByLogin
	tp.ClosedByLogin = next.closedByLogin
	tp.AutoMergeObservedAt = next.autoMergeObservedAt
	// CommentCount is no longer updated from polling -- only refreshed on-demand
	now := time.Now().UTC()
	tp.LastSyncedAt = &now
	if len(changedFields) > 0 && s.logger != nil {
		s.logger.Debug("task PR semantic state changed",
			zap.String("task_id", taskID),
			zap.String("pr_owner", tp.Owner),
			zap.String("pr_repo", tp.Repo),
			zap.Int("pr_number", tp.PRNumber),
			zap.Strings("changed_fields", changedFields),
		)
	}

	return s.persistAndPublishTaskPRSync(ctx, tp, status.PR, changed, status.OutcomeFieldsPopulated)
}

// persistAndPublishTaskPRSync writes the reconciled sync state and, on
// change, publishes github.task_pr.updated. It re-reads the row after the
// write rather than publishing tp directly: tp.AutoMergeObservedAt is
// computed from THIS call's own (possibly stale-by-the-time-of-write) read
// at the top of SyncTaskPR, but UpdateTaskPR persists it through
// COALESCE(auto_merge_observed_at, ?), so a concurrent sync that lands its
// own write in the gap between this call's read and its write can leave the
// column holding a DIFFERENT, earlier timestamp than tp carries in memory.
// Publishing tp unmodified would then broadcast a value that never matches
// what a subsequent read of the row returns (codex [P2]). Split out of
// SyncTaskPR to keep that function within the repo's complexity limits and
// to make the re-read-before-publish behavior directly testable.
func (s *Service) persistAndPublishTaskPRSync(ctx context.Context, tp *TaskPR, pr *PR, changed, outcomeFieldsPopulated bool) error {
	// AC-38/AC-18c: the counter fires at the populated-ness decision point,
	// before the write is attempted, and survives write failure — it
	// measures what the sync observed, not whether the store call happened
	// to succeed.
	incTaskPROutcomeSync(outcomeFieldsPopulated)
	if err := s.store.UpdateTaskPR(ctx, tp); err != nil {
		return fmt.Errorf("update task PR: %w", err)
	}
	// Provider payloads carry the authoritative head/base repository identity
	// and branch. Reconcile after the TaskPR write so a malformed or
	// unmatchable payload never prevents the review association from persisting.
	s.reconcileComparisonTargetFromSync(ctx, tp.TaskID, pr)

	if changed && s.eventBus != nil {
		published, err := s.store.GetTaskPRByID(ctx, tp.ID)
		if err != nil {
			s.logger.Debug("failed to re-read task PR before publishing", zap.Error(err))
			published = tp
		}
		event := bus.NewEvent(events.GitHubTaskPRUpdated, "github", published)
		if err := s.eventBus.Publish(ctx, events.GitHubTaskPRUpdated, event); err != nil {
			s.logger.Debug("failed to publish task PR updated event", zap.Error(err))
		}
	}
	return nil
}

func (s *Service) fetchRequiredReviewsForTaskPR(ctx context.Context, tp *TaskPR, branch string) *int {
	if tp == nil {
		return nil
	}
	if tp.WorkspaceID == "" {
		return s.fetchRequiredReviews(ctx, tp.Owner, tp.Repo, branch)
	}
	return s.fetchRequiredReviewsForWorkspace(ctx, tp.WorkspaceID, tp.Owner, tp.Repo, branch)
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
		// No watches — the TaskPRs that already exist (e.g. PRs imported via
		// task-create-from-PR-URL where the watch is optional) have no other
		// sync path at all, so reconcile them here rather than returning the
		// stored snapshot verbatim. Empty slice if none. permanent=false (the
		// task can still acquire a watch later via push detection).
		existing, listErr := s.reconcileTaskUnwatchedPRs(ctx, taskID, nil)
		if listErr != nil {
			return nil, false, listErr
		}
		return existing, false, nil
	}
	prs, syncErr := s.runBatchedOrPerWatchSync(ctx, taskID, watches)
	// PRs the task linked from an earlier branch are no longer covered by any
	// watch — the loop above cannot see them, and without this they keep their
	// last-observed state (open, green, mergeable) forever.
	if reconciled, listErr := s.reconcileTaskUnwatchedPRs(ctx, taskID, watches); listErr != nil {
		s.logger.Debug("unwatched task PR reconciliation failed",
			zap.String("task_id", taskID), zap.Error(listErr))
	} else {
		prs = reconciled
	}
	return prs, s.areAllWatchesPermanentlyMissing(ctx, watches), syncErr
}

// runBatchedOrPerWatchSync is the shared "try batched, fall back to
// per-watch" body of triggerPRSyncAllPermanent. Split out so the
// permanent-flag computation can wrap the result without duplicating
// the batched / fallback branches inline.
func (s *Service) runBatchedOrPerWatchSync(ctx context.Context, taskID string, watches []*PRWatch) ([]*TaskPR, error) {
	workspaceID := watches[0].WorkspaceID
	if _, batchErr := s.SyncWorkspaceWatchesBatched(ctx, workspaceID, watches); batchErr != nil {
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
func (s *Service) areAllWatchesPermanentlyMissing(ctx context.Context, watches []*PRWatch) bool {
	if len(watches) == 0 {
		return false
	}
	for _, w := range watches {
		resolved, err := s.resolveAutomationClient(ctx, w.WorkspaceID, w.Owner, w.Repo)
		if err != nil || !s.isRepoCachedAsMissingForScope(resolved.CacheScope, w.Owner, w.Repo) {
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
	if watch == nil || strings.TrimSpace(watch.WorkspaceID) == "" {
		return nil, ErrGitHubWorkspaceRequired
	}
	resolved, err := s.resolveAutomationClient(ctx, watch.WorkspaceID, watch.Owner, watch.Repo)
	if err != nil {
		return nil, err
	}
	// Coalesce concurrent detection probes for the same watch. Without this the
	// freshness check below is racy: parallel callers (e.g. the 5s frontend
	// retry racing the workspace background refresh) can all read a stale
	// last_checked_at and probe GitHub simultaneously, defeating the throttle.
	// Keyed distinctly from triggerPRStatusSync's owner/repo/number key so the
	// two never collide. singleflight only blocks same-key calls, so the nested
	// triggerPRStatusSync (different key) below cannot deadlock.
	key := scopedCacheKey(resolved.CacheScope, "pr-detect:"+watch.ID)
	v, err, _ := s.syncGroup.Do(key, func() (interface{}, error) {
		return s.detectPRForWatchOnce(ctx, resolved, watch, taskID)
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
func (s *Service) detectPRForWatchOnce(
	ctx context.Context, resolved *resolvedServiceClient, watch *PRWatch, taskID string,
) (*TaskPR, error) {
	// Short-circuit when this repo is already in the 10-min negative
	// cache. Without this, an unresolvable repo gets a fresh per-watch
	// probe every 5s (the frontend retry cadence) until the freshness
	// timestamp below stamps in — multiplied by every watch the task has.
	if s.isRepoCachedAsMissingForScope(resolved.CacheScope, watch.Owner, watch.Repo) {
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
	pr, err := s.findPRByBranchInForkNetwork(
		ctx, resolved.Client, resolved.CacheScope, watch.Owner, watch.Repo, watch.Branch,
	)
	if err != nil && isRepoNotResolvableErr(err) {
		s.markRepoAsMissingForScope(resolved.CacheScope, watch.Owner, watch.Repo, repoErrGen)
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
	if rebindErr := s.rebindPRWatchRepository(ctx, watch, pr); rebindErr != nil {
		return nil, rebindErr
	}
	if err := s.store.UpdatePRWatchPRNumber(ctx, watch.ID, pr.Number); err != nil {
		s.logger.Error("failed to update PR watch number during sync",
			zap.String("watch_id", watch.ID), zap.Int("pr_number", pr.Number), zap.Error(err))
		return nil, fmt.Errorf("update PR watch: %w", err)
	}
	if _, assocErr := s.AssociatePRWithTaskForWorkspace(ctx, watch.WorkspaceID, taskID, watch.RepositoryID, pr); assocErr != nil {
		s.logger.Error("failed to associate PR with task during sync",
			zap.String("task_id", taskID), zap.Int("pr_number", pr.Number), zap.Error(assocErr))
		return nil, fmt.Errorf("associate PR: %w", assocErr)
	}
	// Also fetch status so the first response includes review/check state
	watch.PRNumber = pr.Number
	return s.triggerPRStatusSync(ctx, watch, taskID)
}

func (s *Service) triggerPRStatusSync(ctx context.Context, watch *PRWatch, taskID string) (*TaskPR, error) {
	if watch == nil || strings.TrimSpace(watch.WorkspaceID) == "" {
		return nil, ErrGitHubWorkspaceRequired
	}
	resolved, err := s.resolveAutomationClient(ctx, watch.WorkspaceID, watch.Owner, watch.Repo)
	if err != nil {
		return nil, err
	}
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
	if s.isRepoCachedAsMissingForScope(resolved.CacheScope, watch.Owner, watch.Repo) {
		return nil, ErrRepoNotResolvable
	}

	// Coalesce concurrent syncs for the same PR
	key := scopedCacheKey(resolved.CacheScope, fmt.Sprintf("%s/%s/%d", watch.Owner, watch.Repo, watch.PRNumber))
	v, err, _ := s.syncGroup.Do(key, func() (interface{}, error) {
		bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		// Snapshot cache generation BEFORE the per-watch probe so a
		// concurrent eviction wins; see Service.markRepoAsMissing.
		repoErrGen := s.repoErrorGenSnapshot()
		status, _, checkErr := s.checkPRWatchWithClient(bgCtx, resolved.Client, watch)
		if checkErr != nil {
			if isRepoNotResolvableErr(checkErr) {
				s.markRepoAsMissingForScope(resolved.CacheScope, watch.Owner, watch.Repo, repoErrGen)
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
