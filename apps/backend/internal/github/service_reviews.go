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
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

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
	if !IsValidCleanupPolicy(req.CleanupPolicy) {
		return nil, fmt.Errorf("invalid cleanup_policy: %q", req.CleanupPolicy)
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
		CleanupPolicy:       NormalizeCleanupPolicy(req.CleanupPolicy),
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
	newPRs, err := s.TriggerReviewWatch(ctx, watch)
	if err != nil {
		s.logger.Debug("initial review check failed",
			zap.String("watch_id", watch.ID), zap.Error(err))
		return
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

// ListAllReviewWatches returns every review watch across all workspaces.
func (s *Service) ListAllReviewWatches(ctx context.Context) ([]*ReviewWatch, error) {
	return s.store.ListAllReviewWatches(ctx)
}

// ListEnabledReviewWatches returns the live (enabled = 1) subset. Used by
// the profile-delete dependency check so self-healed (already-disabled)
// watchers do not inflate the count and trigger spurious 409 confirmations.
func (s *Service) ListEnabledReviewWatches(ctx context.Context) ([]*ReviewWatch, error) {
	return s.store.ListEnabledReviewWatches(ctx)
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
	if req.CleanupPolicy != nil {
		if !IsValidCleanupPolicy(*req.CleanupPolicy) {
			return fmt.Errorf("invalid cleanup_policy: %q", *req.CleanupPolicy)
		}
		rw.CleanupPolicy = NormalizeCleanupPolicy(*req.CleanupPolicy)
	}
	return s.store.UpdateReviewWatch(ctx, rw)
}

// DeleteReviewWatch deletes a review watch and best-effort reaps any tasks
// it owned. The store layer drops the dedup rows transactionally with the
// watch row, but tasks live in a separate domain and would leak forever
// without this pre-pass (the global sweep can no longer find them after the
// dedup rows are gone). True best-effort: a list error logs Warn and lets
// the watch delete proceed so the user's primary action isn't blocked by
// transient task-domain failures.
//
//nolint:nestif // straight-line list → for-loop → conditional delete; readable as-is
func (s *Service) DeleteReviewWatch(ctx context.Context, id string) error {
	if s.taskDeleter != nil {
		prTasks, err := s.store.ListReviewPRTasksByWatch(ctx, id)
		if err != nil {
			s.logger.Warn("failed to list review PR tasks for pre-delete sweep",
				zap.String("watch_id", id), zap.Error(err))
		} else {
			for _, rpt := range prTasks {
				if rpt.TaskID == "" {
					continue
				}
				if err := s.taskDeleter.DeleteTask(ctx, rpt.TaskID); err != nil &&
					!isTaskNotFound(err) {
					s.logger.Warn("failed to delete review task during watch cleanup",
						zap.String("watch_id", id),
						zap.String("task_id", rpt.TaskID),
						zap.Error(err))
				}
			}
		}
	}
	return s.store.DeleteReviewWatch(ctx, id)
}

// DeleteReviewWatchesByWorkspace deletes every review watch in a workspace —
// its dedup rows (transactionally, in the store) and any tasks it owned. The
// E2E reset endpoint calls this to keep worker-scoped specs isolated: review
// watches are not otherwise cleared between tests, so a stale enabled watch
// would let the global review poller create duplicate tasks for PRs a later
// test adds that the stale watch also matches. Best-effort per watch — a
// single failure logs Warn and the sweep continues. Returns the count deleted.
func (s *Service) DeleteReviewWatchesByWorkspace(ctx context.Context, workspaceID string) (int, error) {
	watches, err := s.store.ListReviewWatches(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("list review watches: %w", err)
	}
	deleted := 0
	for _, w := range watches {
		if err := s.DeleteReviewWatch(ctx, w.ID); err != nil {
			s.logger.Warn("failed to delete review watch during workspace reset",
				zap.String("watch_id", w.ID), zap.Error(err))
			continue
		}
		deleted++
	}
	return deleted, nil
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

// TriggerReviewWatch checks one watch and publishes events for every newly
// observed PR so the orchestrator can create the corresponding tasks.
func (s *Service) TriggerReviewWatch(ctx context.Context, watch *ReviewWatch) ([]*PR, error) {
	newPRs, err := s.CheckReviewWatch(ctx, watch)
	if err != nil {
		return nil, err
	}
	for _, pr := range newPRs {
		s.publishNewReviewPREvent(ctx, watch, pr)
	}
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

// DisableReviewWatchWithError mirrors DisableIssueWatchWithError for the
// PR review watcher.
func (s *Service) DisableReviewWatchWithError(ctx context.Context, watchID, cause string) error {
	return s.store.DisableReviewWatchWithError(ctx, watchID, cause)
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
		newPRs, err := s.TriggerReviewWatch(ctx, watch)
		if err != nil {
			s.logger.Error("failed to check review watch",
				zap.String("id", watch.ID), zap.Error(err))
			continue
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
