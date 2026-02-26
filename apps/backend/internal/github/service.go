package github

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// Service coordinates GitHub integration operations.
type Service struct {
	client     Client
	authMethod string
	store      *Store
	eventBus   bus.EventBus
	logger     *logger.Logger
}

// NewService creates a new GitHub service.
func NewService(client Client, authMethod string, store *Store, eventBus bus.EventBus, log *logger.Logger) *Service {
	return &Service{
		client:     client,
		authMethod: authMethod,
		store:      store,
		eventBus:   eventBus,
		logger:     log,
	}
}

// Client returns the underlying GitHub client (may be nil if not authenticated).
func (s *Service) Client() Client {
	return s.client
}

// GetStatus returns the current GitHub connection status.
func (s *Service) GetStatus(ctx context.Context) (*GitHubStatus, error) {
	status := &GitHubStatus{
		AuthMethod: s.authMethod,
	}
	if s.client == nil {
		return status, nil
	}
	ok, err := s.client.IsAuthenticated(ctx)
	if err != nil {
		return status, nil
	}
	status.Authenticated = ok
	if ok {
		user, err := s.client.GetAuthenticatedUser(ctx)
		if err == nil {
			status.Username = user
		}
	}
	return status, nil
}

// SubmitReview submits a review on a pull request.
func (s *Service) SubmitReview(ctx context.Context, owner, repo string, number int, event, body string) error {
	if s.client == nil {
		return fmt.Errorf("github client not configured")
	}
	return s.client.SubmitReview(ctx, owner, repo, number, event, body)
}

// --- PR Watch operations ---

// CreatePRWatch creates a new PR watch for a session.
func (s *Service) CreatePRWatch(ctx context.Context, sessionID, taskID, owner, repo string, prNumber int, branch string) (*PRWatch, error) {
	existing, err := s.store.GetPRWatchBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil // already watching
	}
	w := &PRWatch{
		SessionID: sessionID,
		TaskID:    taskID,
		Owner:     owner,
		Repo:      repo,
		PRNumber:  prNumber,
		Branch:    branch,
	}
	if err := s.store.CreatePRWatch(ctx, w); err != nil {
		return nil, fmt.Errorf("create PR watch: %w", err)
	}
	s.logger.Info("created PR watch",
		zap.String("session_id", sessionID),
		zap.Int("pr_number", prNumber))
	return w, nil
}

// GetPRWatchBySession returns the PR watch for a session.
func (s *Service) GetPRWatchBySession(ctx context.Context, sessionID string) (*PRWatch, error) {
	return s.store.GetPRWatchBySession(ctx, sessionID)
}

// ListActivePRWatches returns all active PR watches.
func (s *Service) ListActivePRWatches(ctx context.Context) ([]*PRWatch, error) {
	return s.store.ListActivePRWatches(ctx)
}

// DeletePRWatch deletes a PR watch by ID.
func (s *Service) DeletePRWatch(ctx context.Context, id string) error {
	return s.store.DeletePRWatch(ctx, id)
}

// CheckPRWatch fetches latest feedback for a PR watch and determines if there are changes.
func (s *Service) CheckPRWatch(ctx context.Context, watch *PRWatch) (*PRFeedback, bool, error) {
	if s.client == nil {
		return nil, false, fmt.Errorf("github client not available")
	}
	feedback, err := s.client.GetPRFeedback(ctx, watch.Owner, watch.Repo, watch.PRNumber)
	if err != nil {
		return nil, false, err
	}

	hasNew := false

	// Check for new comments
	latestCommentAt := findLatestCommentTime(feedback.Comments)
	if latestCommentAt != nil && (watch.LastCommentAt == nil || latestCommentAt.After(*watch.LastCommentAt)) {
		hasNew = true
	}

	// Check for check status changes
	checkStatus := computeOverallCheckStatus(feedback.Checks)
	if checkStatus != watch.LastCheckStatus {
		hasNew = true
	}

	// Update watch timestamps
	now := time.Now().UTC()
	if err := s.store.UpdatePRWatchTimestamps(ctx, watch.ID, now, latestCommentAt, checkStatus); err != nil {
		s.logger.Error("failed to update PR watch timestamps", zap.String("id", watch.ID), zap.Error(err))
	}

	return feedback, hasNew, nil
}

// EnsurePRWatch creates a PRWatch with pr_number=0 for a session if one doesn't already exist.
// The poller will detect the PR by searching for the branch on GitHub.
func (s *Service) EnsurePRWatch(ctx context.Context, sessionID, taskID, owner, repo, branch string) (*PRWatch, error) {
	existing, err := s.store.GetPRWatchBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	w := &PRWatch{
		SessionID: sessionID,
		TaskID:    taskID,
		Owner:     owner,
		Repo:      repo,
		PRNumber:  0,
		Branch:    branch,
	}
	if err := s.store.CreatePRWatch(ctx, w); err != nil {
		return nil, fmt.Errorf("ensure PR watch: %w", err)
	}
	s.logger.Info("created PR watch for session (will search for PR)",
		zap.String("session_id", sessionID),
		zap.String("branch", branch))
	return w, nil
}

// --- Task-PR association ---

// AssociatePRWithTask creates a task-PR association.
func (s *Service) AssociatePRWithTask(ctx context.Context, taskID string, pr *PR) (*TaskPR, error) {
	existing, err := s.store.GetTaskPR(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.PRNumber == pr.Number {
		return existing, nil
	}
	tp := &TaskPR{
		TaskID:      taskID,
		Owner:       pr.RepoOwner,
		Repo:        pr.RepoName,
		PRNumber:    pr.Number,
		PRURL:       pr.HTMLURL,
		PRTitle:     pr.Title,
		HeadBranch:  pr.HeadBranch,
		BaseBranch:  pr.BaseBranch,
		AuthorLogin: pr.AuthorLogin,
		State:       pr.State,
		Additions:   pr.Additions,
		Deletions:   pr.Deletions,
		CreatedAt:   pr.CreatedAt,
		MergedAt:    pr.MergedAt,
		ClosedAt:    pr.ClosedAt,
	}
	if err := s.store.CreateTaskPR(ctx, tp); err != nil {
		return nil, fmt.Errorf("create task PR: %w", err)
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
		zap.Int("pr_number", pr.Number))
	return tp, nil
}

// AssociatePRByURL parses a GitHub PR URL, fetches the PR data, creates a PR watch,
// and associates it with the given task. Called after user creates a PR from the UI.
func (s *Service) AssociatePRByURL(ctx context.Context, sessionID, taskID, prURL, branch string) {
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
	if _, watchErr := s.CreatePRWatch(ctx, sessionID, taskID, owner, repo, prNumber, branch); watchErr != nil {
		s.logger.Error("failed to create PR watch after PR creation",
			zap.String("session_id", sessionID), zap.Error(watchErr))
	}

	// Associate PR with task (persists + publishes WS event)
	if _, assocErr := s.AssociatePRWithTask(ctx, taskID, pr); assocErr != nil {
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

// ListTaskPRs returns PR associations for multiple tasks.
func (s *Service) ListTaskPRs(ctx context.Context, taskIDs []string) (map[string]*TaskPR, error) {
	return s.store.ListTaskPRsByTaskIDs(ctx, taskIDs)
}

// SyncTaskPR updates a TaskPR record with the latest PR data from feedback.
func (s *Service) SyncTaskPR(ctx context.Context, taskID string, feedback *PRFeedback) error {
	tp, err := s.store.GetTaskPR(ctx, taskID)
	if err != nil || tp == nil {
		return err
	}

	tp.State = feedback.PR.State
	tp.PRTitle = feedback.PR.Title
	tp.Additions = feedback.PR.Additions
	tp.Deletions = feedback.PR.Deletions
	tp.MergedAt = feedback.PR.MergedAt
	tp.ClosedAt = feedback.PR.ClosedAt
	tp.CommentCount = len(feedback.Comments)
	tp.ReviewCount = len(feedback.Reviews)
	reviewState, pendingReviewCount := deriveReviewSyncState(feedback.PR, feedback.Reviews)
	tp.ReviewState = reviewState
	tp.ChecksState = computeOverallCheckStatus(feedback.Checks)
	tp.PendingReviewCount = pendingReviewCount
	now := time.Now().UTC()
	tp.LastSyncedAt = &now

	if err := s.store.UpdateTaskPR(ctx, tp); err != nil {
		return fmt.Errorf("update task PR: %w", err)
	}

	if s.eventBus != nil {
		event := bus.NewEvent(events.GitHubTaskPRUpdated, "github", tp)
		if err := s.eventBus.Publish(ctx, events.GitHubTaskPRUpdated, event); err != nil {
			s.logger.Debug("failed to publish task PR updated event", zap.Error(err))
		}
	}
	return nil
}

// --- PR feedback (live) ---

// GetPRFeedback fetches live PR feedback from GitHub.
func (s *Service) GetPRFeedback(ctx context.Context, owner, repo string, number int) (*PRFeedback, error) {
	if s.client == nil {
		return nil, fmt.Errorf("github client not available")
	}
	return s.client.GetPRFeedback(ctx, owner, repo, number)
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

	// Filter out PRs we already created tasks for
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
			s.logger.Error("failed to list review PRs",
				zap.String("filter", qualifier), zap.Error(err))
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

// RecordReviewPRTask records that a task was created for a review PR.
func (s *Service) RecordReviewPRTask(ctx context.Context, watchID, repoOwner, repoName string, prNumber int, prURL, taskID string) error {
	rpt := &ReviewPRTask{
		ReviewWatchID: watchID,
		RepoOwner:     repoOwner,
		RepoName:      repoName,
		PRNumber:      prNumber,
		PRURL:         prURL,
		TaskID:        taskID,
	}
	return s.store.CreateReviewPRTask(ctx, rpt)
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

func computeOverallCheckStatus(checks []CheckRun) string {
	if len(checks) == 0 {
		return ""
	}
	hasPending := false
	for _, c := range checks {
		if c.Status == checkStatusCompleted && c.Conclusion == checkConclusionFail {
			return checkConclusionFail
		}
		if c.Status != checkStatusCompleted {
			hasPending = true
		}
	}
	if hasPending {
		return checkStatusPending
	}
	return checkStatusSuccess
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
