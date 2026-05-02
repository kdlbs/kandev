package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// Auth method constants.
const (
	AuthMethodNone = "none"
	AuthMethodPAT  = "pat"
)

// defaultBranchMain and defaultBranchMaster are the conventional default branch
// names sorted to the top of branch pickers.
const (
	defaultBranchMain   = "main"
	defaultBranchMaster = "master"
)

// TaskDeleter deletes tasks by ID. Used for cleaning up merged PR tasks.
type TaskDeleter interface {
	DeleteTask(ctx context.Context, taskID string) error
}

// TaskSessionChecker checks if a task has any sessions (user interacted with it).
type TaskSessionChecker interface {
	HasTaskSessions(ctx context.Context, taskID string) (bool, error)
}

// SecretManager handles secret creation, update, and deletion.
type SecretManager interface {
	Create(ctx context.Context, name, value string) (id string, err error)
	Update(ctx context.Context, id, value string) error
	Delete(ctx context.Context, id string) error
}

// prSyncFreshnessWindow is how long PR data is considered fresh (skip GitHub API).
const prSyncFreshnessWindow = 30 * time.Second

// Service coordinates GitHub integration operations.
type Service struct {
	mu                 sync.Mutex
	client             Client
	authMethod         string
	secrets            SecretProvider
	secretManager      SecretManager
	store              *Store
	eventBus           bus.EventBus
	logger             *logger.Logger
	taskDeleter        TaskDeleter
	taskSessionChecker TaskSessionChecker
	syncGroup          singleflight.Group
	taskEventSubs      []bus.Subscription
	searchCache        *ttlCache
	prStatusCache      *ttlCache
}

// NewService creates a new GitHub service.
func NewService(client Client, authMethod string, secrets SecretProvider, store *Store, eventBus bus.EventBus, log *logger.Logger) *Service {
	return &Service{
		client:        client,
		authMethod:    authMethod,
		secrets:       secrets,
		store:         store,
		eventBus:      eventBus,
		logger:        log,
		searchCache:   newTTLCache(),
		prStatusCache: newTTLCache(),
	}
}

// SetTaskDeleter sets the task deletion dependency for cleanup operations.
func (s *Service) SetTaskDeleter(d TaskDeleter) { s.taskDeleter = d }

// SetTaskSessionChecker sets the session checker for cleanup operations.
func (s *Service) SetTaskSessionChecker(c TaskSessionChecker) { s.taskSessionChecker = c }

// SetSecretManager sets the secret manager for token configuration operations.
func (s *Service) SetSecretManager(m SecretManager) { s.secretManager = m }

// Client returns the underlying GitHub client (may be nil if not authenticated).
func (s *Service) Client() Client {
	return s.client
}

// TestStore returns the store for test/mock use only.
func (s *Service) TestStore() *Store {
	return s.store
}

// TestEventBus returns the event bus for test/mock use only.
func (s *Service) TestEventBus() bus.EventBus {
	return s.eventBus
}

// IsAuthenticated returns whether the service has a working GitHub client.
// Returns false when using the NoopClient fallback (authMethod == "none").
func (s *Service) IsAuthenticated() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.client != nil && s.authMethod != AuthMethodNone
}

// AuthMethod returns the authentication method ("gh_cli", "pat", or "none").
func (s *Service) AuthMethod() string {
	return s.authMethod
}

// RequiredGitHubScopes lists the GitHub token scopes needed for full functionality.
var RequiredGitHubScopes = []string{"repo", "read:org"}

// GetStatus returns the current GitHub connection status.
// If not authenticated, it retries client creation to pick up auth changes
// (e.g. GITHUB_TOKEN secret added after startup).
func (s *Service) GetStatus(ctx context.Context) (*GitHubStatus, error) {
	if !s.IsAuthenticated() {
		s.retryClientCreation(ctx)
	}

	s.mu.Lock()
	client := s.client
	authMethod := s.authMethod
	s.mu.Unlock()

	status := &GitHubStatus{
		AuthMethod:     authMethod,
		RequiredScopes: RequiredGitHubScopes,
	}

	// Check if a GITHUB_TOKEN secret is configured
	tokenSecretID, hasToken := s.findGitHubTokenSecret(ctx)
	status.TokenConfigured = hasToken
	status.TokenSecretID = tokenSecretID

	if client == nil {
		status.Diagnostics = runGHDiagnostics(ctx)
		return status, nil
	}
	ok, err := client.IsAuthenticated(ctx)
	if err != nil {
		return status, nil
	}
	status.Authenticated = ok
	if ok {
		user, err := client.GetAuthenticatedUser(ctx)
		if err == nil {
			status.Username = user
		}
	} else {
		status.Diagnostics = runGHDiagnostics(ctx)
	}
	return status, nil
}

// findGitHubTokenSecret checks if a GITHUB_TOKEN secret exists in the secrets store.
// Returns the secret ID and whether a token is configured.
func (s *Service) findGitHubTokenSecret(ctx context.Context) (string, bool) {
	if s.secrets == nil {
		return "", false
	}
	items, err := s.secrets.List(ctx)
	if err != nil {
		return "", false
	}
	for _, item := range items {
		if !item.HasValue {
			continue
		}
		if item.Name == "GITHUB_TOKEN" || item.Name == "github_token" {
			return item.ID, true
		}
	}
	return "", false
}

// retryClientCreation attempts to create a GitHub client when not authenticated.
// This picks up auth changes made after startup (secrets added, env vars set).
func (s *Service) retryClientCreation(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.authMethod != AuthMethodNone {
		return // already authenticated
	}
	client, authMethod, err := NewClient(ctx, s.secrets, s.logger)
	if err != nil {
		s.logger.Debug("GitHub client retry failed", zap.Error(err))
		return
	}
	s.client = client
	s.authMethod = authMethod
	s.logger.Info("GitHub client recovered after retry",
		zap.String("auth_method", authMethod))
}

// runGHDiagnostics runs gh auth status if the gh CLI is available.
func runGHDiagnostics(ctx context.Context) *AuthDiagnostics {
	if !GHAvailable() {
		return &AuthDiagnostics{
			Command:  "gh auth status",
			Output:   "gh CLI is not installed. Install it from https://cli.github.com",
			ExitCode: -1,
		}
	}
	return NewGHClient().RunAuthDiagnostics(ctx)
}

// ConfigureToken saves or updates a GitHub Personal Access Token in the secrets store.
// It validates the token by making a test API call before saving.
func (s *Service) ConfigureToken(ctx context.Context, token string) error {
	if s.secretManager == nil {
		return fmt.Errorf("secret manager not configured")
	}

	// Validate the token by testing authentication
	testClient := NewPATClient(token)
	user, err := testClient.GetAuthenticatedUser(ctx)
	if err != nil {
		return fmt.Errorf("invalid token: %w", err)
	}
	s.logger.Info("validated GitHub token", zap.String("user", user))

	// Check if a GITHUB_TOKEN secret already exists
	existingID, exists := s.findGitHubTokenSecret(ctx)
	if exists {
		// Update existing secret
		if err := s.secretManager.Update(ctx, existingID, token); err != nil {
			return fmt.Errorf("update token: %w", err)
		}
		s.logger.Info("updated GitHub token secret", zap.String("id", existingID))
	} else {
		// Create new secret
		newID, err := s.secretManager.Create(ctx, "GITHUB_TOKEN", token)
		if err != nil {
			return fmt.Errorf("create token: %w", err)
		}
		s.logger.Info("created GitHub token secret", zap.String("id", newID))
	}

	// Refresh the client to use the new token
	s.mu.Lock()
	s.client = testClient
	s.authMethod = AuthMethodPAT
	s.mu.Unlock()

	return nil
}

// ClearToken removes the configured GitHub token from the secrets store.
func (s *Service) ClearToken(ctx context.Context) error {
	if s.secretManager == nil {
		return fmt.Errorf("secret manager not configured")
	}

	existingID, exists := s.findGitHubTokenSecret(ctx)
	if !exists {
		return nil // No token to clear
	}

	if err := s.secretManager.Delete(ctx, existingID); err != nil {
		return fmt.Errorf("delete token: %w", err)
	}
	s.logger.Info("cleared GitHub token secret", zap.String("id", existingID))

	// Reset to try gh CLI or other methods
	s.mu.Lock()
	s.client = nil
	s.authMethod = AuthMethodNone
	s.mu.Unlock()

	// Try to re-establish connection via other methods
	s.retryClientCreation(ctx)

	return nil
}

// SubmitReview submits a review on a pull request.
func (s *Service) SubmitReview(ctx context.Context, owner, repo string, number int, event, body string) error {
	if s.client == nil {
		return fmt.Errorf("github client not configured")
	}
	return s.client.SubmitReview(ctx, owner, repo, number, event, body)
}

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

	hasNew := false

	// Check for check status or review state changes
	if status.ChecksState != watch.LastCheckStatus {
		hasNew = true
	}
	if status.ReviewState != watch.LastReviewState {
		hasNew = true
	}

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
func (s *Service) SyncTaskPR(ctx context.Context, taskID string, status *PRStatus) error {
	if status == nil || status.PR == nil {
		return fmt.Errorf("sync task PR: missing PR data for task %s", taskID)
	}
	tp, err := s.findTaskPRForStatus(ctx, taskID, status.PR)
	if err != nil || tp == nil {
		return err
	}

	changed := tp.State != status.PR.State ||
		tp.PRTitle != status.PR.Title ||
		tp.Additions != status.PR.Additions ||
		tp.Deletions != status.PR.Deletions ||
		tp.ReviewState != status.ReviewState ||
		tp.ChecksState != status.ChecksState ||
		tp.MergeableState != status.MergeableState ||
		tp.ReviewCount != status.ReviewCount ||
		tp.PendingReviewCount != status.PendingReviewCount ||
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
	tp.ReviewCount = status.ReviewCount
	tp.PendingReviewCount = status.PendingReviewCount
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

// timeEqual compares two nullable time pointers for equality.
func timeEqual(a, b *time.Time) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(*b)
}

// --- PR info and feedback (live) ---

// GetPR fetches basic PR details from GitHub.
func (s *Service) GetPR(ctx context.Context, owner, repo string, number int) (*PR, error) {
	if s.client == nil {
		return nil, fmt.Errorf("github client not available")
	}
	return s.client.GetPR(ctx, owner, repo, number)
}

// GetPRFeedback fetches live PR feedback from GitHub.
func (s *Service) GetPRFeedback(ctx context.Context, owner, repo string, number int) (*PRFeedback, error) {
	if s.client == nil {
		return nil, fmt.Errorf("github client not available")
	}
	return s.client.GetPRFeedback(ctx, owner, repo, number)
}

// GetPRStatus fetches lightweight PR status (review + checks + mergeable).
// Cached briefly so repeat loads of the same list (pagination, re-render,
// back-navigation) don't refetch. The returned pointer is shared — callers
// must not mutate it.
func (s *Service) GetPRStatus(ctx context.Context, owner, repo string, number int) (*PRStatus, error) {
	if s.client == nil {
		return nil, fmt.Errorf("github client not available")
	}
	key := prStatusCacheKey(owner, repo, number)
	v, err := s.prStatusCache.doOrFetch(key, func() (any, error) {
		return s.client.GetPRStatus(ctx, owner, repo, number)
	})
	if err != nil {
		return nil, err
	}
	return v.(*PRStatus), nil
}

// PRRef identifies a pull request by owner/repo/number.
type PRRef struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Number int    `json:"number"`
}

// prStatusBatchConcurrency bounds how many upstream GetPRStatus calls run in
// parallel. GitHub has per-hour quotas and per-endpoint concurrency limits;
// this is well under both while still collapsing a 25-PR page into a single
// short wait on the client.
const prStatusBatchConcurrency = 8

// GetPRStatusesBatch fetches statuses for multiple PRs concurrently, honoring
// the per-PR cache. The returned map is keyed by prStatusCacheKey; PRs that
// fail to fetch are logged and omitted from the result so one bad repo
// doesn't poison the page.
func (s *Service) GetPRStatusesBatch(ctx context.Context, refs []PRRef) (map[string]*PRStatus, error) {
	if s.client == nil {
		return nil, fmt.Errorf("github client not available")
	}
	result := make(map[string]*PRStatus, len(refs))
	var mu sync.Mutex
	sem := make(chan struct{}, prStatusBatchConcurrency)
	var wg sync.WaitGroup
	for _, ref := range refs {
		if ref.Owner == "" || ref.Repo == "" || ref.Number <= 0 {
			continue
		}
		wg.Add(1)
		go func(r PRRef) {
			defer wg.Done()
			// Release queued goroutines early when the caller disconnects —
			// otherwise up to 200 refs queue up serially behind the semaphore
			// and each still runs its full upstream fetch.
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			status, err := s.GetPRStatus(ctx, r.Owner, r.Repo, r.Number)
			if err != nil {
				s.logger.Debug("batch PR status fetch failed",
					zap.String("owner", r.Owner),
					zap.String("repo", r.Repo),
					zap.Int("number", r.Number),
					zap.Error(err))
				return
			}
			mu.Lock()
			result[prStatusCacheKey(r.Owner, r.Repo, r.Number)] = status
			mu.Unlock()
		}(ref)
	}
	wg.Wait()
	return result, nil
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

// --- PR files and commits (live) ---

// GetPRFiles fetches files changed in a PR from GitHub.
func (s *Service) GetPRFiles(ctx context.Context, owner, repo string, number int) ([]PRFile, error) {
	if s.client == nil {
		return nil, fmt.Errorf("github client not available")
	}
	return s.client.ListPRFiles(ctx, owner, repo, number)
}

// GetPRCommits fetches commits in a PR from GitHub.
func (s *Service) GetPRCommits(ctx context.Context, owner, repo string, number int) ([]PRCommitInfo, error) {
	if s.client == nil {
		return nil, fmt.Errorf("github client not available")
	}
	return s.client.ListPRCommits(ctx, owner, repo, number)
}

// --- Review Watch operations ---

// CreateReviewWatch creates a new review watch and triggers an initial poll.
func (s *Service) CreateReviewWatch(ctx context.Context, req *CreateReviewWatchRequest) (*ReviewWatch, error) {
	if req.PollIntervalSeconds <= 0 {
		req.PollIntervalSeconds = defaultWatchPollIntervalSec
	}
	if req.PollIntervalSeconds < minWatchPollIntervalSec {
		req.PollIntervalSeconds = minWatchPollIntervalSec
	}
	repos := req.Repos
	if repos == nil {
		repos = []RepoFilter{}
	}
	reviewScope := req.ReviewScope
	if reviewScope == "" {
		reviewScope = ReviewScopeUserAndTeams
	}
	rw := &ReviewWatch{
		WorkspaceID:         req.WorkspaceID,
		WorkflowID:          req.WorkflowID,
		WorkflowStepID:      req.WorkflowStepID,
		Repos:               repos,
		AgentProfileID:      req.AgentProfileID,
		ExecutorProfileID:   req.ExecutorProfileID,
		Prompt:              req.Prompt,
		ReviewScope:         reviewScope,
		CustomQuery:         req.CustomQuery,
		Enabled:             true,
		PollIntervalSeconds: req.PollIntervalSeconds,
	}
	if err := s.store.CreateReviewWatch(ctx, rw); err != nil {
		return nil, fmt.Errorf("create review watch: %w", err)
	}

	// Trigger initial poll in background so the watch starts working immediately
	go s.initialReviewCheck(context.Background(), rw)

	return rw, nil
}

// initialReviewCheck runs a single poll for a newly created review watch.
func (s *Service) initialReviewCheck(ctx context.Context, watch *ReviewWatch) {
	newPRs, err := s.CheckReviewWatch(ctx, watch)
	if err != nil {
		s.logger.Debug("initial review check failed",
			zap.String("watch_id", watch.ID), zap.Error(err))
		return
	}
	for _, pr := range newPRs {
		s.publishNewReviewPREvent(ctx, watch, pr)
	}
	if len(newPRs) > 0 {
		s.logger.Info("initial review check found PRs",
			zap.String("watch_id", watch.ID),
			zap.Int("new_prs", len(newPRs)))
	}
}

// GetReviewWatch returns a review watch by ID.
func (s *Service) GetReviewWatch(ctx context.Context, id string) (*ReviewWatch, error) {
	return s.store.GetReviewWatch(ctx, id)
}

// ListReviewWatches returns all review watches for a workspace.
func (s *Service) ListReviewWatches(ctx context.Context, workspaceID string) ([]*ReviewWatch, error) {
	return s.store.ListReviewWatches(ctx, workspaceID)
}

// UpdateReviewWatch updates a review watch.
func (s *Service) UpdateReviewWatch(ctx context.Context, id string, req *UpdateReviewWatchRequest) error {
	rw, err := s.store.GetReviewWatch(ctx, id)
	if err != nil {
		return err
	}
	if rw == nil {
		return fmt.Errorf("review watch not found: %s", id)
	}
	if req.WorkflowID != nil {
		rw.WorkflowID = *req.WorkflowID
	}
	if req.WorkflowStepID != nil {
		rw.WorkflowStepID = *req.WorkflowStepID
	}
	if req.Repos != nil {
		rw.Repos = *req.Repos
	}
	if req.AgentProfileID != nil {
		rw.AgentProfileID = *req.AgentProfileID
	}
	if req.ExecutorProfileID != nil {
		rw.ExecutorProfileID = *req.ExecutorProfileID
	}
	if req.Prompt != nil {
		rw.Prompt = *req.Prompt
	}
	if req.ReviewScope != nil {
		rw.ReviewScope = *req.ReviewScope
	}
	if req.CustomQuery != nil {
		rw.CustomQuery = *req.CustomQuery
	}
	if req.Enabled != nil {
		rw.Enabled = *req.Enabled
	}
	if req.PollIntervalSeconds != nil {
		rw.PollIntervalSeconds = *req.PollIntervalSeconds
	}
	return s.store.UpdateReviewWatch(ctx, rw)
}

// DeleteReviewWatch deletes a review watch.
func (s *Service) DeleteReviewWatch(ctx context.Context, id string) error {
	return s.store.DeleteReviewWatch(ctx, id)
}

// CheckReviewWatch checks for new PRs needing review and returns ones not yet tracked.
// If watch.Repos is empty, all repos are queried. Otherwise, each repo is queried individually.
func (s *Service) CheckReviewWatch(ctx context.Context, watch *ReviewWatch) ([]*PR, error) {
	if s.client == nil {
		return nil, fmt.Errorf("github client not available")
	}

	s.logger.Debug("checking review watch for pending PRs",
		zap.String("watch_id", watch.ID),
		zap.Int("repo_filters", len(watch.Repos)),
		zap.String("custom_query", watch.CustomQuery),
		zap.String("review_scope", watch.ReviewScope),
		zap.Bool("enabled", watch.Enabled))

	prs, err := s.fetchReviewPRs(ctx, watch)
	if err != nil {
		return nil, err
	}

	s.logger.Debug("fetched review-requested PRs",
		zap.String("watch_id", watch.ID),
		zap.Int("total_prs", len(prs)))

	// Pre-filter PRs that are already tracked. This is a best-effort check
	// that avoids publishing events for PRs that clearly have tasks; it does
	// NOT need to be race-free, because the orchestrator's createReviewTask
	// atomically reserves the dedup slot before doing any task-creation work
	// (see ReserveReviewPRTask). So a race here at most causes an extra event
	// that the reservation step will drop.
	var newPRs []*PR
	for _, pr := range prs {
		exists, err := s.store.HasReviewPRTask(ctx, watch.ID, pr.RepoOwner, pr.RepoName, pr.Number)
		if err != nil {
			s.logger.Error("failed to check review PR task", zap.Error(err))
			continue
		}
		if exists {
			s.logger.Debug("skipping already-tracked PR",
				zap.String("watch_id", watch.ID),
				zap.String("repo", pr.RepoOwner+"/"+pr.RepoName),
				zap.Int("pr_number", pr.Number))
		} else {
			newPRs = append(newPRs, pr)
		}
	}

	// Enrich new PRs with full details (branch info) from the PR API,
	// since the search API does not return head/base branch.
	s.enrichPRDetails(ctx, newPRs)

	s.logger.Debug("review watch check complete",
		zap.String("watch_id", watch.ID),
		zap.Int("total_fetched", len(prs)),
		zap.Int("new_prs", len(newPRs)),
		zap.Int("already_tracked", len(prs)-len(newPRs)))

	// Update last polled
	now := time.Now().UTC()
	watch.LastPolledAt = &now
	_ = s.store.UpdateReviewWatch(ctx, watch)

	return newPRs, nil
}

// fetchReviewPRs fetches PRs needing review based on the watch configuration.
// When repo filters are set, they are always applied — even when a custom query is present
// (the filter qualifier is appended to the query for each repo).
func (s *Service) fetchReviewPRs(ctx context.Context, watch *ReviewWatch) ([]*PR, error) {
	hasRepos := len(watch.Repos) > 0

	s.logger.Debug("fetchReviewPRs: starting",
		zap.String("watch_id", watch.ID),
		zap.String("custom_query", watch.CustomQuery),
		zap.String("scope", watch.ReviewScope),
		zap.Int("repo_count", len(watch.Repos)),
		zap.Bool("has_repos", hasRepos))

	// No repo filters: use query verbatim (custom or scope-based)
	if !hasRepos {
		if watch.CustomQuery != "" {
			s.logger.Debug("fetchReviewPRs: using custom query (all repos)",
				zap.String("query", watch.CustomQuery))
			return s.client.ListReviewRequestedPRs(ctx, "", "", watch.CustomQuery)
		}
		s.logger.Debug("fetchReviewPRs: using scope (all repos)",
			zap.String("scope", watch.ReviewScope))
		return s.client.ListReviewRequestedPRs(ctx, watch.ReviewScope, "", "")
	}

	// Has repo filters: iterate repos, appending filter to customQuery or scope
	prs := s.fetchReviewPRsWithFilter(ctx, watch)
	return prs, nil
}

// fetchReviewPRsWithFilter queries each repo filter individually and deduplicates results.
// When customQuery is set, the repo qualifier is appended to it; otherwise scope+filter is used.
func (s *Service) fetchReviewPRsWithFilter(ctx context.Context, watch *ReviewWatch) []*PR {
	var allPRs []*PR
	seen := make(map[string]bool)

	for _, repo := range watch.Repos {
		qualifier := repoFilterToQualifier(repo)

		var prs []*PR
		var err error
		if watch.CustomQuery != "" {
			query := watch.CustomQuery + " " + qualifier
			s.logger.Debug("fetchReviewPRs: querying with custom query + filter",
				zap.String("watch_id", watch.ID),
				zap.String("query", query))
			prs, err = s.client.ListReviewRequestedPRs(ctx, "", "", query)
		} else {
			s.logger.Debug("fetchReviewPRs: querying with scope + filter",
				zap.String("watch_id", watch.ID),
				zap.String("scope", watch.ReviewScope),
				zap.String("filter", qualifier))
			prs, err = s.client.ListReviewRequestedPRs(ctx, watch.ReviewScope, qualifier, "")
		}
		if err != nil {
			if isConnectivityError(err) {
				s.logger.Warn("failed to list review PRs (connectivity)",
					zap.String("filter", qualifier), zap.Error(err))
			} else {
				s.logger.Error("failed to list review PRs",
					zap.String("filter", qualifier), zap.Error(err))
			}
			continue
		}

		s.logger.Debug("fetchReviewPRs: got results for filter",
			zap.String("filter", qualifier),
			zap.Int("count", len(prs)))

		for _, pr := range prs {
			key := fmt.Sprintf("%s/%s#%d", pr.RepoOwner, pr.RepoName, pr.Number)
			if !seen[key] {
				seen[key] = true
				allPRs = append(allPRs, pr)
			}
		}
	}
	return allPRs
}

// repoFilterToQualifier converts a RepoFilter to a GitHub search qualifier string.
func repoFilterToQualifier(repo RepoFilter) string {
	if repo.Name == "" {
		return "org:" + repo.Owner
	}
	return fmt.Sprintf("repo:%s/%s", repo.Owner, repo.Name)
}

// enrichPRDetails fetches full PR details for PRs missing branch info (from the search API).
func (s *Service) enrichPRDetails(ctx context.Context, prs []*PR) {
	for _, pr := range prs {
		if pr.HeadBranch != "" && pr.BaseBranch != "" {
			continue
		}
		s.logger.Debug("enriching PR with full details (missing branch info)",
			zap.String("repo", pr.RepoOwner+"/"+pr.RepoName),
			zap.Int("pr_number", pr.Number))

		full, err := s.client.GetPR(ctx, pr.RepoOwner, pr.RepoName, pr.Number)
		if err != nil {
			s.logger.Warn("failed to fetch full PR details, branch info will be empty",
				zap.String("repo", pr.RepoOwner+"/"+pr.RepoName),
				zap.Int("pr_number", pr.Number),
				zap.Error(err))
			continue
		}
		pr.HeadBranch = full.HeadBranch
		pr.HeadSHA = full.HeadSHA
		pr.BaseBranch = full.BaseBranch
		pr.Additions = full.Additions
		pr.Deletions = full.Deletions
		pr.Mergeable = full.Mergeable
	}
}

// ListUserOrgs returns the authenticated user's orgs, prepending their own username.
func (s *Service) ListUserOrgs(ctx context.Context) ([]GitHubOrg, error) {
	if s.client == nil {
		return nil, fmt.Errorf("github client not available")
	}
	orgs, err := s.client.ListUserOrgs(ctx)
	if err != nil {
		return nil, err
	}
	// Prepend the authenticated user as a pseudo-org (for personal repos).
	user, userErr := s.client.GetAuthenticatedUser(ctx)
	if userErr == nil && user != "" {
		orgs = append([]GitHubOrg{{Login: user}}, orgs...)
	}
	return orgs, nil
}

// SearchOrgRepos searches repos in an org for autocomplete.
func (s *Service) SearchOrgRepos(ctx context.Context, org, query string, limit int) ([]GitHubRepo, error) {
	if s.client == nil {
		return nil, fmt.Errorf("github client not available")
	}
	return s.client.SearchOrgRepos(ctx, org, query, limit)
}

// ListRepoBranches lists branches for a repository.
// When no authenticated client is configured, it falls back to the unauthenticated
// GitHub API so that public repositories remain accessible without a token.
// The result is sorted so that "main" appears first, "master" second, then all
// remaining branches alphabetically — matching the default branch convention and
// making the most common choices immediately visible in pickers.
func (s *Service) ListRepoBranches(ctx context.Context, owner, repo string) ([]RepoBranch, error) {
	var (
		branches []RepoBranch
		err      error
	)
	if s.client != nil {
		branches, err = s.client.ListRepoBranches(ctx, owner, repo)
		if err != nil && !errors.Is(err, ErrNoClient) {
			return nil, err
		}
	}
	if branches == nil {
		branches, err = listRepoBranchesAnonymous(ctx, owner, repo)
		if err != nil {
			return nil, err
		}
	}
	sortBranchesMainFirst(branches)
	return branches, nil
}

// sortBranchesMainFirst sorts branches in-place: "main" first, "master" second,
// then all remaining branches alphabetically.
func sortBranchesMainFirst(branches []RepoBranch) {
	priority := func(name string) int {
		switch name {
		case defaultBranchMain:
			return 0
		case defaultBranchMaster:
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(branches, func(i, j int) bool {
		pi, pj := priority(branches[i].Name), priority(branches[j].Name)
		if pi != pj {
			return pi < pj
		}
		return branches[i].Name < branches[j].Name
	})
}

// anonymousAPIBase is the GitHub API base URL used by listRepoBranchesAnonymous.
// Overridden in tests to point at a local httptest server.
var anonymousAPIBase = githubAPIBase

// anonymousHTTPClient is used by listRepoBranchesAnonymous. The 30 s timeout
// prevents a slow or unresponsive GitHub API from tying up server goroutines
// indefinitely across multi-page pagination.
var anonymousHTTPClient = &http.Client{Timeout: 30 * time.Second}

// listRepoBranchesAnonymous calls the GitHub REST API without authentication to
// list branches for public repositories, following pagination via Link headers.
// Returns ErrNoClient on network errors and a GitHubAPIError for non-2xx
// responses (404, 403, etc.) so the controller maps them to correct HTTP codes.
func listRepoBranchesAnonymous(ctx context.Context, owner, repo string) ([]RepoBranch, error) {
	next := fmt.Sprintf("%s/repos/%s/%s/branches?per_page=100",
		anonymousAPIBase, url.PathEscape(owner), url.PathEscape(repo))
	var branches []RepoBranch
	for next != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, next, nil)
		if err != nil {
			return nil, ErrNoClient
		}
		req.Header.Set("Accept", githubAccept)
		req.Header.Set("X-GitHub-Api-Version", githubAPIVersion)

		resp, err := anonymousHTTPClient.Do(req)
		if err != nil {
			return nil, ErrNoClient
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			return nil, &GitHubAPIError{
				StatusCode: resp.StatusCode,
				Endpoint:   next,
				Body:       string(body),
			}
		}

		var page []struct {
			Name string `json:"name"`
		}
		err = json.NewDecoder(resp.Body).Decode(&page)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("decode branches response: %w", err)
		}
		for _, b := range page {
			branches = append(branches, RepoBranch{Name: b.Name})
		}
		next = parseLinkNext(resp.Header.Get("Link"))
	}
	return branches, nil
}

// parseLinkNext extracts the URL for rel="next" from a GitHub Link header.
// Returns "" if no next page is present.
func parseLinkNext(link string) string {
	// Format: <url>; rel="next", <url>; rel="last"
	for part := range strings.SplitSeq(link, ",") {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, `rel="next"`) {
			continue
		}
		if start := strings.Index(part, "<"); start != -1 {
			if end := strings.Index(part, ">"); end != -1 && end > start {
				return part[start+1 : end]
			}
		}
	}
	return ""
}

// SearchUserPRs searches for PRs using a filter or custom query. Unless the
// caller already pins a type qualifier, `type:pr` is injected into the
// composed query.
func (s *Service) SearchUserPRs(ctx context.Context, filter, customQuery string) ([]*PR, error) {
	if s.client == nil {
		return nil, fmt.Errorf("github client not available")
	}
	return s.client.SearchPRs(ctx, filter, customQuery)
}

// SearchUserIssues searches for issues using a filter or custom query. Unless
// the caller already pins a type qualifier, `type:issue` is injected into the
// composed query.
func (s *Service) SearchUserIssues(ctx context.Context, filter, customQuery string) ([]*Issue, error) {
	if s.client == nil {
		return nil, fmt.Errorf("github client not available")
	}
	return s.client.ListIssues(ctx, filter, customQuery)
}

// SearchUserPRsPaged is the paginated variant of SearchUserPRs. Results are
// cached for a short window (see searchCacheTTL) — callers must not mutate
// the returned page, since it is shared across concurrent requests.
func (s *Service) SearchUserPRsPaged(ctx context.Context, filter, customQuery string, page, perPage int) (*PRSearchPage, error) {
	if s.client == nil {
		return nil, fmt.Errorf("github client not available")
	}
	// Clamp before composing the cache key so perPage=150 and perPage=100
	// share the same cached page instead of creating two entries for
	// identical results.
	page, perPage = clampSearchPage(page, perPage)
	key := searchCacheKey("pr", filter, customQuery, page, perPage)
	v, err := s.searchCache.doOrFetch(key, func() (any, error) {
		result, err := s.client.SearchPRsPaged(ctx, filter, customQuery, page, perPage)
		if err != nil {
			return nil, err
		}
		if result != nil && result.PRs == nil {
			result.PRs = []*PR{}
		}
		return result, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*PRSearchPage), nil
}

// SearchUserIssuesPaged is the paginated variant of SearchUserIssues. See
// SearchUserPRsPaged for caching semantics.
func (s *Service) SearchUserIssuesPaged(ctx context.Context, filter, customQuery string, page, perPage int) (*IssueSearchPage, error) {
	if s.client == nil {
		return nil, fmt.Errorf("github client not available")
	}
	page, perPage = clampSearchPage(page, perPage)
	key := searchCacheKey("issue", filter, customQuery, page, perPage)
	v, err := s.searchCache.doOrFetch(key, func() (any, error) {
		result, err := s.client.ListIssuesPaged(ctx, filter, customQuery, page, perPage)
		if err != nil {
			return nil, err
		}
		if result != nil && result.Issues == nil {
			result.Issues = []*Issue{}
		}
		return result, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*IssueSearchPage), nil
}

// ReserveReviewPRTask atomically claims the dedup slot for a (watch, repo, PR)
// tuple before task creation begins. Returns true if this caller won and
// should proceed to create the task, false if another caller already holds
// the slot (duplicate, skip). This closes the race window that existed when
// the dedup row was only written AFTER the slow clone + task-creation work,
// which could produce duplicate tasks when two pollers or events raced.
func (s *Service) ReserveReviewPRTask(ctx context.Context, watchID, repoOwner, repoName string, prNumber int, prURL string) (bool, error) {
	return s.store.ReserveReviewPRTask(ctx, watchID, repoOwner, repoName, prNumber, prURL)
}

// AssignReviewPRTaskID attaches a task ID to a previously reserved slot so
// downstream cleanup (CleanupMergedReviewTasks) can locate and delete the
// task when its PR is merged or closed.
func (s *Service) AssignReviewPRTaskID(ctx context.Context, watchID, repoOwner, repoName string, prNumber int, taskID string) error {
	return s.store.AssignReviewPRTaskID(ctx, watchID, repoOwner, repoName, prNumber, taskID)
}

// ReleaseReviewPRTask removes a reservation when task creation fails, so a
// later poll can retry this PR instead of it being blocked by an orphan row.
func (s *Service) ReleaseReviewPRTask(ctx context.Context, watchID, repoOwner, repoName string, prNumber int) error {
	return s.store.ReleaseReviewPRTask(ctx, watchID, repoOwner, repoName, prNumber)
}

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
	deleted := 0
	for _, rpt := range prTasks {
		// Orphan reservation: process was killed after ReserveReviewPRTask
		// succeeded but before AssignReviewPRTaskID ran, so task_id is empty
		// and there is no task to delete. Clean up the dedup row once the PR
		// reaches a terminal state, same gating as the normal path.
		if rpt.TaskID == "" {
			if should, _ := s.shouldDeleteReviewTask(ctx, rpt); should {
				_ = s.store.DeleteReviewPRTask(ctx, rpt.ID)
				deleted++
			}
			continue
		}
		shouldDelete, reason := s.shouldDeleteReviewTask(ctx, rpt)
		if !shouldDelete {
			continue
		}
		if err := s.taskDeleter.DeleteTask(ctx, rpt.TaskID); err != nil {
			if strings.Contains(err.Error(), "not found") {
				// Task already deleted; clean up the orphaned dedup record.
				_ = s.store.DeleteReviewPRTask(ctx, rpt.ID)
				deleted++
				continue
			}
			s.logger.Warn("failed to delete review PR task",
				zap.String("task_id", rpt.TaskID), zap.Error(err))
			continue
		}
		_ = s.store.DeleteReviewPRTask(ctx, rpt.ID)
		s.logger.Info("deleted review task",
			zap.String("task_id", rpt.TaskID),
			zap.String("reason", reason),
			zap.Int("pr_number", rpt.PRNumber),
			zap.String("repo", rpt.RepoOwner+"/"+rpt.RepoName))
		deleted++
	}
	return deleted, nil
}

// shouldDeleteReviewTask checks if a review PR task should be cleaned up.
// Returns true + reason if the PR is done (merged/closed/approved) AND the user
// hasn't interacted with the task yet (no sessions). Tasks with sessions are
// preserved so the user can see the PR merged banner and their work history.
func (s *Service) shouldDeleteReviewTask(ctx context.Context, rpt *ReviewPRTask) (bool, string) {
	feedback, err := s.client.GetPRFeedback(ctx, rpt.RepoOwner, rpt.RepoName, rpt.PRNumber)
	if err != nil {
		s.logger.Debug("failed to fetch PR feedback for cleanup",
			zap.Int("pr_number", rpt.PRNumber), zap.Error(err))
		return false, ""
	}
	if feedback.PR == nil {
		return false, ""
	}
	var reason string
	if feedback.PR.State == prStateMerged || feedback.PR.State == prStateClosed {
		reason = "pr_merged_or_closed"
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
	// Don't delete tasks the user has interacted with (has sessions).
	// Those show the PR merged banner instead.
	if s.taskSessionChecker != nil {
		hasSessions, err := s.taskSessionChecker.HasTaskSessions(ctx, rpt.TaskID)
		if err != nil {
			s.logger.Debug("failed to check task sessions",
				zap.String("task_id", rpt.TaskID), zap.Error(err))
			return false, ""
		}
		if hasSessions {
			return false, ""
		}
	}
	return true, reason
}

// TriggerAllReviewChecks triggers all review watches for a workspace.
func (s *Service) TriggerAllReviewChecks(ctx context.Context, workspaceID string) (int, error) {
	watches, err := s.store.ListReviewWatches(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	enabled := 0
	for _, w := range watches {
		if w.Enabled {
			enabled++
		}
	}
	s.logger.Info("triggering review checks",
		zap.String("workspace_id", workspaceID),
		zap.Int("total_watches", len(watches)),
		zap.Int("enabled_watches", enabled))

	totalNew := 0
	for _, watch := range watches {
		if !watch.Enabled {
			continue
		}
		newPRs, err := s.CheckReviewWatch(ctx, watch)
		if err != nil {
			s.logger.Error("failed to check review watch",
				zap.String("id", watch.ID), zap.Error(err))
			continue
		}
		for _, pr := range newPRs {
			s.publishNewReviewPREvent(ctx, watch, pr)
		}
		totalNew += len(newPRs)
	}
	s.logger.Info("review checks completed",
		zap.String("workspace_id", workspaceID),
		zap.Int("new_prs_found", totalNew))
	return totalNew, nil
}

// GetPRStats returns PR statistics.
func (s *Service) GetPRStats(ctx context.Context, req *PRStatsRequest) (*PRStats, error) {
	return s.store.GetPRStats(ctx, req)
}

func (s *Service) publishNewReviewPREvent(ctx context.Context, watch *ReviewWatch, pr *PR) {
	if s.eventBus == nil {
		return
	}
	event := bus.NewEvent(events.GitHubNewReviewPR, "github", &NewReviewPREvent{
		ReviewWatchID:     watch.ID,
		WorkspaceID:       watch.WorkspaceID,
		WorkflowID:        watch.WorkflowID,
		WorkflowStepID:    watch.WorkflowStepID,
		AgentProfileID:    watch.AgentProfileID,
		ExecutorProfileID: watch.ExecutorProfileID,
		Prompt:            watch.Prompt,
		PR:                pr,
	})
	if err := s.eventBus.Publish(ctx, events.GitHubNewReviewPR, event); err != nil {
		s.logger.Debug("failed to publish new review PR event", zap.Error(err))
	}
}

// --- Issue Watch service methods ---

// CreateIssueWatch creates a new issue watch and triggers an initial poll.
func (s *Service) CreateIssueWatch(ctx context.Context, req *CreateIssueWatchRequest) (*IssueWatch, error) {
	if req.PollIntervalSeconds <= 0 {
		req.PollIntervalSeconds = defaultWatchPollIntervalSec
	}
	if req.PollIntervalSeconds < minWatchPollIntervalSec {
		req.PollIntervalSeconds = minWatchPollIntervalSec
	}
	repos := req.Repos
	if repos == nil {
		repos = []RepoFilter{}
	}
	labels := req.Labels
	if labels == nil {
		labels = []string{}
	}
	iw := &IssueWatch{
		WorkspaceID:         req.WorkspaceID,
		WorkflowID:          req.WorkflowID,
		WorkflowStepID:      req.WorkflowStepID,
		Repos:               repos,
		AgentProfileID:      req.AgentProfileID,
		ExecutorProfileID:   req.ExecutorProfileID,
		Prompt:              req.Prompt,
		Labels:              labels,
		CustomQuery:         req.CustomQuery,
		Enabled:             true,
		PollIntervalSeconds: req.PollIntervalSeconds,
	}
	if err := s.store.CreateIssueWatch(ctx, iw); err != nil {
		return nil, fmt.Errorf("create issue watch: %w", err)
	}
	go s.initialIssueCheck(context.Background(), iw)
	return iw, nil
}

// initialIssueCheck runs a single poll for a newly created issue watch.
func (s *Service) initialIssueCheck(ctx context.Context, watch *IssueWatch) {
	newIssues, err := s.CheckIssueWatch(ctx, watch)
	if err != nil {
		s.logger.Debug("initial issue check failed",
			zap.String("watch_id", watch.ID), zap.Error(err))
		return
	}
	for _, issue := range newIssues {
		s.publishNewIssueEvent(ctx, watch, issue)
	}
	if len(newIssues) > 0 {
		s.logger.Info("initial issue check found issues",
			zap.String("watch_id", watch.ID),
			zap.Int("new_issues", len(newIssues)))
	}
}

// GetIssueWatch returns a single issue watch by ID.
func (s *Service) GetIssueWatch(ctx context.Context, id string) (*IssueWatch, error) {
	return s.store.GetIssueWatch(ctx, id)
}

// ListIssueWatches returns all issue watches for a workspace.
func (s *Service) ListIssueWatches(ctx context.Context, workspaceID string) ([]*IssueWatch, error) {
	return s.store.ListIssueWatches(ctx, workspaceID)
}

// UpdateIssueWatch updates an issue watch.
//
//nolint:dupl // mirrors UpdateReviewWatch — different types, same structure
func (s *Service) UpdateIssueWatch(ctx context.Context, id string, req *UpdateIssueWatchRequest) error {
	iw, err := s.store.GetIssueWatch(ctx, id)
	if err != nil {
		return err
	}
	if iw == nil {
		return fmt.Errorf("issue watch not found: %s", id)
	}
	if req.WorkflowID != nil {
		iw.WorkflowID = *req.WorkflowID
	}
	if req.WorkflowStepID != nil {
		iw.WorkflowStepID = *req.WorkflowStepID
	}
	if req.Repos != nil {
		iw.Repos = *req.Repos
	}
	if req.AgentProfileID != nil {
		iw.AgentProfileID = *req.AgentProfileID
	}
	if req.ExecutorProfileID != nil {
		iw.ExecutorProfileID = *req.ExecutorProfileID
	}
	if req.Prompt != nil {
		iw.Prompt = *req.Prompt
	}
	if req.Labels != nil {
		iw.Labels = *req.Labels
	}
	if req.CustomQuery != nil {
		iw.CustomQuery = *req.CustomQuery
	}
	if req.Enabled != nil {
		iw.Enabled = *req.Enabled
	}
	if req.PollIntervalSeconds != nil {
		v := *req.PollIntervalSeconds
		if v <= 0 {
			v = defaultWatchPollIntervalSec
		}
		if v < minWatchPollIntervalSec {
			v = minWatchPollIntervalSec
		}
		iw.PollIntervalSeconds = v
	}
	return s.store.UpdateIssueWatch(ctx, iw)
}

// DeleteIssueWatch deletes an issue watch.
func (s *Service) DeleteIssueWatch(ctx context.Context, id string) error {
	return s.store.DeleteIssueWatch(ctx, id)
}

// CheckIssueWatch checks for new issues matching the watch and returns ones not yet tracked.
func (s *Service) CheckIssueWatch(ctx context.Context, watch *IssueWatch) ([]*Issue, error) {
	if s.client == nil {
		return nil, fmt.Errorf("github client not available")
	}

	s.logger.Debug("checking issue watch for new issues",
		zap.String("watch_id", watch.ID),
		zap.Int("repo_filters", len(watch.Repos)),
		zap.String("custom_query", watch.CustomQuery),
		zap.Bool("enabled", watch.Enabled))

	issues, err := s.fetchIssues(ctx, watch)
	if err != nil {
		return nil, err
	}

	var newIssues []*Issue
	for _, issue := range issues {
		exists, checkErr := s.store.HasIssueWatchTask(ctx, watch.ID, issue.RepoOwner, issue.RepoName, issue.Number)
		if checkErr != nil {
			s.logger.Error("failed to check issue watch task", zap.Error(checkErr))
			continue
		}
		if !exists {
			newIssues = append(newIssues, issue)
		}
	}

	now := time.Now().UTC()
	watch.LastPolledAt = &now
	_ = s.store.UpdateIssueWatch(ctx, watch)

	return newIssues, nil
}

// fetchIssues fetches issues based on the watch configuration.
func (s *Service) fetchIssues(ctx context.Context, watch *IssueWatch) ([]*Issue, error) {
	hasRepos := len(watch.Repos) > 0

	if !hasRepos {
		filter := s.buildIssueFilter(watch)
		return s.client.ListIssues(ctx, filter, watch.CustomQuery)
	}

	return s.fetchIssuesWithRepoFilter(ctx, watch), nil
}

// buildIssueFilter builds the filter qualifier from watch labels. `state:open`
// is included because the watcher is only interested in active issues —
// buildIssueSearchQuery no longer injects it (the /github page presets
// supply their own state qualifiers), so we add it here instead.
func (s *Service) buildIssueFilter(watch *IssueWatch) string {
	parts := []string{"state:open"}
	for _, label := range watch.Labels {
		if strings.ContainsRune(label, ' ') {
			parts = append(parts, `label:"`+label+`"`)
		} else {
			parts = append(parts, "label:"+label)
		}
	}
	return strings.Join(parts, " ")
}

// fetchIssuesWithRepoFilter queries each repo individually and deduplicates.
func (s *Service) fetchIssuesWithRepoFilter(ctx context.Context, watch *IssueWatch) []*Issue {
	var allIssues []*Issue
	seen := make(map[string]bool)

	labelFilter := s.buildIssueFilter(watch)

	for _, repo := range watch.Repos {
		qualifier := repoFilterToQualifier(repo)
		filter := qualifier
		if labelFilter != "" {
			filter += " " + labelFilter
		}

		var issues []*Issue
		var err error
		if watch.CustomQuery != "" {
			query := watch.CustomQuery + " " + qualifier
			issues, err = s.client.ListIssues(ctx, "", query)
		} else {
			issues, err = s.client.ListIssues(ctx, filter, "")
		}
		if err != nil {
			if isConnectivityError(err) {
				s.logger.Warn("failed to list issues (connectivity)",
					zap.String("filter", qualifier), zap.Error(err))
			} else {
				s.logger.Error("failed to list issues",
					zap.String("filter", qualifier), zap.Error(err))
			}
			continue
		}

		for _, issue := range issues {
			key := fmt.Sprintf("%s/%s#%d", issue.RepoOwner, issue.RepoName, issue.Number)
			if !seen[key] {
				seen[key] = true
				allIssues = append(allIssues, issue)
			}
		}
	}
	return allIssues
}

// ReserveIssueWatchTask atomically claims a dedup slot.
func (s *Service) ReserveIssueWatchTask(ctx context.Context, watchID, repoOwner, repoName string, issueNumber int, issueURL string) (bool, error) {
	return s.store.ReserveIssueWatchTask(ctx, watchID, repoOwner, repoName, issueNumber, issueURL)
}

// AssignIssueWatchTaskID attaches a task ID to a previously reserved slot.
func (s *Service) AssignIssueWatchTaskID(ctx context.Context, watchID, repoOwner, repoName string, issueNumber int, taskID string) error {
	return s.store.AssignIssueWatchTaskID(ctx, watchID, repoOwner, repoName, issueNumber, taskID)
}

// ReleaseIssueWatchTask removes a reservation when task creation fails.
func (s *Service) ReleaseIssueWatchTask(ctx context.Context, watchID, repoOwner, repoName string, issueNumber int) error {
	return s.store.ReleaseIssueWatchTask(ctx, watchID, repoOwner, repoName, issueNumber)
}

// TriggerAllIssueChecks triggers all issue watches for a workspace.
//
//nolint:dupl // mirrors TriggerAllReviewChecks — different types, same structure
func (s *Service) TriggerAllIssueChecks(ctx context.Context, workspaceID string) (int, error) {
	watches, err := s.store.ListIssueWatches(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	enabled := 0
	for _, w := range watches {
		if w.Enabled {
			enabled++
		}
	}
	s.logger.Info("triggering issue checks",
		zap.String("workspace_id", workspaceID),
		zap.Int("total_watches", len(watches)),
		zap.Int("enabled_watches", enabled))

	totalNew := 0
	for _, watch := range watches {
		if !watch.Enabled {
			continue
		}
		newIssues, checkErr := s.CheckIssueWatch(ctx, watch)
		if checkErr != nil {
			s.logger.Error("failed to check issue watch",
				zap.String("watch_id", watch.ID), zap.Error(checkErr))
			continue
		}
		for _, issue := range newIssues {
			s.publishNewIssueEvent(ctx, watch, issue)
		}
		totalNew += len(newIssues)
		if _, cleanErr := s.CleanupClosedIssueTasks(ctx, watch); cleanErr != nil {
			s.logger.Warn("cleanup closed issue tasks failed",
				zap.String("watch_id", watch.ID), zap.Error(cleanErr))
		}
	}
	s.logger.Info("issue checks completed",
		zap.String("workspace_id", workspaceID),
		zap.Int("new_issues_found", totalNew))
	return totalNew, nil
}

// CleanupClosedIssueTasks checks issues tracked by a watch and deletes
// tasks whose issues are closed and the user hasn't interacted with.
//
//nolint:dupl // mirrors CleanupMergedReviewTasks — different types, same structure
func (s *Service) CleanupClosedIssueTasks(ctx context.Context, watch *IssueWatch) (int, error) {
	if s.client == nil || s.taskDeleter == nil || s.taskSessionChecker == nil {
		return 0, nil
	}
	issueTasks, err := s.store.ListIssueWatchTasksByWatch(ctx, watch.ID)
	if err != nil {
		return 0, fmt.Errorf("list issue watch tasks: %w", err)
	}
	deleted := 0
	for _, it := range issueTasks {
		// Orphan reservation: task_id is empty because the process was
		// killed between Reserve and Assign. Clean up once the issue is closed.
		if it.TaskID == "" {
			if should, _ := s.shouldDeleteIssueTask(ctx, it); should {
				_ = s.store.DeleteIssueWatchTask(ctx, it.ID)
				deleted++
			}
			continue
		}
		shouldDelete, reason := s.shouldDeleteIssueTask(ctx, it)
		if !shouldDelete {
			continue
		}
		if err := s.taskDeleter.DeleteTask(ctx, it.TaskID); err != nil {
			if strings.Contains(err.Error(), "not found") {
				_ = s.store.DeleteIssueWatchTask(ctx, it.ID)
				deleted++
				continue
			}
			s.logger.Warn("failed to delete issue task",
				zap.String("task_id", it.TaskID), zap.Error(err))
			continue
		}
		_ = s.store.DeleteIssueWatchTask(ctx, it.ID)
		s.logger.Info("deleted issue task",
			zap.String("task_id", it.TaskID),
			zap.String("reason", reason),
			zap.Int("issue_number", it.IssueNumber),
			zap.String("repo", it.RepoOwner+"/"+it.RepoName))
		deleted++
	}
	return deleted, nil
}

// shouldDeleteIssueTask checks if an issue task should be cleaned up.
// Returns true + reason if the issue is closed AND the user hasn't interacted
// with the task yet (no sessions).
func (s *Service) shouldDeleteIssueTask(ctx context.Context, it *IssueWatchTask) (bool, string) {
	state, err := s.client.GetIssueState(ctx, it.RepoOwner, it.RepoName, it.IssueNumber)
	if err != nil {
		s.logger.Debug("failed to fetch issue state for cleanup",
			zap.Int("issue_number", it.IssueNumber), zap.Error(err))
		return false, ""
	}
	if state != "closed" {
		return false, ""
	}
	reason := "issue_closed"
	// Don't delete tasks the user has interacted with (has sessions).
	if it.TaskID != "" {
		hasSessions, err := s.taskSessionChecker.HasTaskSessions(ctx, it.TaskID)
		if err != nil {
			s.logger.Debug("failed to check task sessions",
				zap.String("task_id", it.TaskID), zap.Error(err))
			return false, ""
		}
		if hasSessions {
			return false, ""
		}
	}
	return true, reason
}

func (s *Service) publishNewIssueEvent(ctx context.Context, watch *IssueWatch, issue *Issue) {
	if s.eventBus == nil {
		return
	}
	event := bus.NewEvent(events.GitHubNewIssue, "github", &NewIssueEvent{
		IssueWatchID:      watch.ID,
		WorkspaceID:       watch.WorkspaceID,
		WorkflowID:        watch.WorkflowID,
		WorkflowStepID:    watch.WorkflowStepID,
		AgentProfileID:    watch.AgentProfileID,
		ExecutorProfileID: watch.ExecutorProfileID,
		Prompt:            watch.Prompt,
		Issue:             issue,
	})
	if err := s.eventBus.Publish(ctx, events.GitHubNewIssue, event); err != nil {
		s.logger.Debug("failed to publish new issue event", zap.Error(err))
	}
}

func findLatestCommentTime(comments []PRComment) *time.Time {
	var latest *time.Time
	for _, c := range comments {
		t := c.UpdatedAt
		if latest == nil || t.After(*latest) {
			latest = &t
		}
	}
	return latest
}

// computeOverallCheckStatus reduces per-check runs to a single PR-level status.
// Mirrors GitHub's own UI: skipped/neutral conclusions are ignored; any failing
// terminal state (failure, timed_out, cancelled, action_required) makes the PR
// failed; non-completed checks keep the PR pending.
func computeOverallCheckStatus(checks []CheckRun) string {
	if len(checks) == 0 {
		return ""
	}
	hasPending := false
	hasPassing := false
	for _, c := range checks {
		if c.Status != checkStatusCompleted {
			hasPending = true
			continue
		}
		switch c.Conclusion {
		case checkConclusionFail, checkConclusionTimedOut,
			checkConclusionCancelled, checkConclusionActionRequired:
			return checkConclusionFail
		case checkConclusionSkipped, checkConclusionNeutral:
			// ignore — GitHub's UI does
		default:
			// Treat success and any future unknown terminal conclusion as passing.
			// Being permissive preserves the success signal if GitHub introduces
			// a new conclusion we haven't mapped yet.
			hasPassing = true
		}
	}
	if hasPending {
		return checkStatusPending
	}
	if hasPassing {
		return checkStatusSuccess
	}
	return ""
}

func computeOverallReviewState(reviews []PRReview) string {
	if len(reviews) == 0 {
		return ""
	}
	latest := latestReviewByAuthor(reviews)
	changesReq := false
	allApproved := true
	for _, r := range latest {
		if r.State == reviewStateChangesRequested {
			changesReq = true
		}
		if r.State != reviewStateApproved {
			allApproved = false
		}
	}
	if changesReq {
		return computedReviewStateChangesRequested
	}
	if allApproved {
		return computedReviewStateApproved
	}
	return computedReviewStatePending
}

func countPendingReviews(reviews []PRReview) int {
	latest := latestReviewByAuthor(reviews)
	count := 0
	for _, r := range latest {
		if r.State == reviewStatePending || r.State == reviewStateCommented {
			count++
		}
	}
	return count
}

func countPendingRequestedReviewers(pr *PR) int {
	if pr == nil {
		return 0
	}
	return len(pr.RequestedReviewers)
}

func deriveReviewSyncState(pr *PR, reviews []PRReview) (string, int) {
	pendingReviewCount := countPendingRequestedReviewers(pr)
	if pendingReviewCount == 0 {
		pendingReviewCount = countPendingReviews(reviews)
	}
	reviewState := computeOverallReviewState(reviews)
	if reviewState == "" && pendingReviewCount > 0 {
		reviewState = computedReviewStatePending
	}
	return reviewState, pendingReviewCount
}
