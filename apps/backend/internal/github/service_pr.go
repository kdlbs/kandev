package github

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// SubmitReview submits a review on a pull request. For APPROVE events it
// first checks that the authenticated user is not the PR author, returning
// ErrSelfApprove instead of letting the request fail at GitHub with an opaque
// 422. Lookup failures here are non-fatal — we fall through to GitHub so a
// transient API hiccup doesn't block legitimate approvals.
func (s *Service) SubmitReview(ctx context.Context, owner, repo string, number int, event, body string) error {
	if s.client == nil {
		return fmt.Errorf("github client not configured")
	}
	return submitReview(ctx, s.client, owner, repo, number, event, body)
}

// SubmitReviewForWorkspace submits a user-triggered review with an explicitly
// resolved workspace principal. GitHub App fallback is allowed by the resolver
// and remains attributable to the App rather than impersonating the user.
func (s *Service) SubmitReviewForWorkspace(
	ctx context.Context,
	workspaceID string,
	userID string,
	owner string,
	repo string,
	number int,
	event string,
	body string,
) (AuthPrincipal, error) {
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return AuthPrincipal{}, err
	}
	resolved, err := s.resolvePersonalWriteClient(ctx, workspaceID, userID, owner, repo)
	if err != nil {
		return AuthPrincipal{}, err
	}
	if err := requireGitHubCapability(resolved, CapabilityPullRequestWrite); err != nil {
		return AuthPrincipal{}, err
	}
	return resolved.Principal, submitReview(ctx, resolved.Client, owner, repo, number, event, body)
}

func submitReview(ctx context.Context, client Client, owner, repo string, number int, event, body string) error {
	if event == reviewEventApprove {
		user, userErr := client.GetAuthenticatedUser(ctx)
		if userErr == nil && user != "" {
			pr, prErr := client.GetPR(ctx, owner, repo, number)
			if prErr == nil && pr != nil &&
				strings.EqualFold(strings.TrimSpace(pr.AuthorLogin), strings.TrimSpace(user)) {
				return ErrSelfApprove
			}
		}
	}
	return client.SubmitReview(ctx, owner, repo, number, event, body)
}

// MergePR merges a pull request. mergeMethod is one of "merge", "squash",
// "rebase"; an empty string asks the service to pick the first method the
// repo allows. The caller is expected to refresh PR feedback after a
// successful merge — the background poller will catch the merged state on
// its next pass.
func (s *Service) MergePR(ctx context.Context, owner, repo string, number int, mergeMethod string) error {
	if s.client == nil {
		return ErrNoClient
	}
	return s.mergePRWithClient(ctx, s.client, "legacy", owner, repo, number, mergeMethod)
}

// MergePRForWorkspace performs a user-triggered merge using the workspace's
// personal-write routing rules and returns the actual attributed principal.
func (s *Service) MergePRForWorkspace(
	ctx context.Context,
	workspaceID string,
	userID string,
	owner string,
	repo string,
	number int,
	mergeMethod string,
) (AuthPrincipal, error) {
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return AuthPrincipal{}, err
	}
	resolved, err := s.resolvePersonalWriteClient(ctx, workspaceID, userID, owner, repo)
	if err != nil {
		return AuthPrincipal{}, err
	}
	if err := requireGitHubCapability(resolved, CapabilityPullRequestWrite); err != nil {
		return AuthPrincipal{}, err
	}
	return resolved.Principal, s.mergePRWithClient(
		ctx, resolved.Client, resolved.CacheScope, owner, repo, number, mergeMethod,
	)
}

func (s *Service) mergePRWithClient(
	ctx context.Context,
	client Client,
	cacheScope string,
	owner string,
	repo string,
	number int,
	mergeMethod string,
) error {
	if mergeMethod == "" {
		// Resolve to an allowed method up-front so we don't rely on GitHub's
		// "default to merge" behavior, which 405s on repos that disallow
		// merge commits (squash-only / rebase-only). Best-effort: if the
		// lookup fails (or — degenerate config — reports no method allowed),
		// fall back to GitHub's default and surface its error rather than
		// blocking the merge attempt.
		if methods, err := s.getRepoMergeMethods(ctx, client, cacheScope, owner, repo); err == nil {
			if pick := pickDefaultMergeMethod(methods); pick != "" {
				mergeMethod = pick
			}
		}
	}
	return client.MergePR(ctx, owner, repo, number, mergeMethod)
}

// GetRepoMergeMethods returns the merge methods a repo allows, cached for
// a few minutes since repo settings rarely change.
func (s *Service) GetRepoMergeMethods(ctx context.Context, owner, repo string) (RepoMergeMethods, error) {
	if s.client == nil {
		return RepoMergeMethods{}, ErrNoClient
	}
	return s.getRepoMergeMethods(ctx, s.client, "legacy", owner, repo)
}

func (s *Service) GetRepoMergeMethodsForWorkspace(
	ctx context.Context, workspaceID, userID, owner, repo string,
) (RepoMergeMethods, error) {
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return RepoMergeMethods{}, err
	}
	resolved, err := s.resolvePersonalReadClient(ctx, workspaceID, userID, owner, repo)
	if err != nil {
		return RepoMergeMethods{}, err
	}
	return s.getRepoMergeMethods(ctx, resolved.Client, resolved.CacheScope, owner, repo)
}

func (s *Service) getRepoMergeMethods(
	ctx context.Context,
	client Client,
	cacheScope string,
	owner string,
	repo string,
) (RepoMergeMethods, error) {
	key := scopedCacheKey(cacheScope, owner+"/"+repo)
	v, err := s.mergeMethodsCache.doOrFetch(key, func() (any, error) {
		return client.GetRepoMergeMethods(ctx, owner, repo)
	})
	if err != nil {
		return RepoMergeMethods{}, err
	}
	return v.(RepoMergeMethods), nil
}

// Merge method identifiers accepted by GitHub's pulls/{number}/merge endpoint
// and used throughout the merge resolution paths.
const (
	mergeMethodMerge  = "merge"
	mergeMethodSquash = "squash"
	mergeMethodRebase = "rebase"
)

// pickDefaultMergeMethod picks the merge method to use when the caller
// didn't pin one. Prefers squash (matches the convention most repos in
// this codebase follow) and falls back to merge, then rebase.
func pickDefaultMergeMethod(m RepoMergeMethods) string {
	switch {
	case m.Squash:
		return mergeMethodSquash
	case m.Merge:
		return mergeMethodMerge
	case m.Rebase:
		return mergeMethodRebase
	default:
		return ""
	}
}

// --- PR info and feedback (live) ---

// GetPR fetches basic PR details from GitHub.
func (s *Service) GetPR(ctx context.Context, owner, repo string, number int) (*PR, error) {
	if s.client == nil {
		return nil, ErrNoClient
	}
	return s.client.GetPR(ctx, owner, repo, number)
}

func (s *Service) GetPRForWorkspace(
	ctx context.Context, workspaceID, userID, owner, repo string, number int,
) (*PR, error) {
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return nil, err
	}
	resolved, err := s.resolvePersonalReadClient(ctx, workspaceID, userID, owner, repo)
	if err != nil {
		return nil, err
	}
	return resolved.Client.GetPR(ctx, owner, repo, number)
}

// GetIssue fetches basic issue details from GitHub. The create-task dialog is
// currently the only caller and dedupes requests per URL on the frontend.
func (s *Service) GetIssue(ctx context.Context, owner, repo string, number int) (*Issue, error) {
	if s.client == nil {
		return nil, ErrNoClient
	}
	return s.client.GetIssue(ctx, owner, repo, number)
}

func (s *Service) GetIssueForWorkspace(
	ctx context.Context, workspaceID, userID, owner, repo string, number int,
) (*Issue, error) {
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return nil, err
	}
	resolved, err := s.resolvePersonalReadClient(ctx, workspaceID, userID, owner, repo)
	if err != nil {
		return nil, err
	}
	return resolved.Client.GetIssue(ctx, owner, repo, number)
}

// GetPRFeedback fetches live PR feedback from GitHub. Cached briefly with
// singleflight coalescing: the task page fires this for the same PR many times
// in quick succession (chat-bar CI chip, detail panel, hover popover — each on
// its own render/mount triggers), and each miss is 4 sequential GitHub REST
// calls. Without this, a burst fanned out into dozens of slow, rate-limited
// duplicate requests. The returned pointer is shared — callers must not mutate it.
func (s *Service) GetPRFeedback(ctx context.Context, owner, repo string, number int) (*PRFeedback, error) {
	if s.client == nil {
		return nil, fmt.Errorf("github client not available")
	}
	return s.getPRFeedback(ctx, s.client, "legacy", owner, repo, number)
}

func (s *Service) GetPRFeedbackForWorkspace(
	ctx context.Context, workspaceID, userID, owner, repo string, number int,
) (*PRFeedback, error) {
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return nil, err
	}
	resolved, err := s.resolvePersonalReadClient(ctx, workspaceID, userID, owner, repo)
	if err != nil {
		return nil, err
	}
	return s.getPRFeedback(ctx, resolved.Client, resolved.CacheScope, owner, repo, number)
}

func (s *Service) getPRFeedback(
	ctx context.Context, client Client, cacheScope, owner, repo string, number int,
) (*PRFeedback, error) {
	// Detach the upstream call's context from the singleflight leader: a
	// cancelled leader (user closes the tab) would otherwise return its
	// context error to all co-waiters, cascading a single client disconnect
	// into a burst of transient failures across concurrent callers.
	// context.WithoutCancel strips the deadline too, so re-attach the
	// parent deadline explicitly — without this, a still-active caller's
	// timeout wouldn't bound the shared fetch and quota could keep being
	// consumed past the deadline.
	fetchCtx, cancelFetch := derivedFetchContext(ctx)
	defer cancelFetch()
	key := scopedCacheKey(cacheScope, prStatusCacheKey(owner, repo, number))
	v, err := s.prFeedbackCache.doOrFetch(key, func() (any, error) {
		return client.GetPRFeedback(fetchCtx, owner, repo, number)
	})
	if err != nil {
		return nil, err
	}
	return v.(*PRFeedback), nil
}

// GetPRStatus fetches lightweight PR status (review + checks + mergeable).
// Cached briefly so repeat loads of the same list (pagination, re-render,
// back-navigation) don't refetch. The returned pointer is shared — callers
// must not mutate it.
func (s *Service) GetPRStatus(ctx context.Context, owner, repo string, number int) (*PRStatus, error) {
	if s.client == nil {
		return nil, fmt.Errorf("github client not available")
	}
	return s.getPRStatus(ctx, s.client, "legacy", owner, repo, number)
}

func (s *Service) GetPRStatusForWorkspace(
	ctx context.Context, workspaceID, userID, owner, repo string, number int,
) (*PRStatus, error) {
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return nil, err
	}
	resolved, err := s.resolvePersonalReadClient(ctx, workspaceID, userID, owner, repo)
	if err != nil {
		return nil, err
	}
	return s.getPRStatus(ctx, resolved.Client, resolved.CacheScope, owner, repo, number)
}

func (s *Service) getPRStatus(
	ctx context.Context, client Client, cacheScope, owner, repo string, number int,
) (*PRStatus, error) {
	// Detach the upstream call from the singleflight leader's context; see
	// GetPRFeedback for the cascading-cancel + deadline-preserve rationale.
	fetchCtx, cancelFetch := derivedFetchContext(ctx)
	defer cancelFetch()
	key := scopedCacheKey(cacheScope, prStatusCacheKey(owner, repo, number))
	v, err := s.prStatusCache.doOrFetch(key, func() (any, error) {
		return client.GetPRStatus(fetchCtx, owner, repo, number)
	})
	if err != nil {
		return nil, err
	}
	return v.(*PRStatus), nil
}

// derivedFetchContext builds the upstream-fetch context used by the
// singleflight-cached GetPRFeedback / GetPRStatus paths. It strips the
// caller's cancellation via context.WithoutCancel so a single client
// disconnect doesn't cascade the same error to all co-waiters, but
// preserves the caller's deadline (when present) so the fetch can't
// outlive the request budget and keep consuming GitHub quota past it.
// Returns the derived context and a cancel func the caller must defer
// to release resources promptly.
func derivedFetchContext(ctx context.Context) (context.Context, context.CancelFunc) {
	fetchCtx := context.WithoutCancel(ctx)
	if deadline, ok := ctx.Deadline(); ok {
		return context.WithDeadline(fetchCtx, deadline)
	}
	return fetchCtx, func() {}
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
	return s.getPRStatusesBatchWithClient(ctx, s.client, "legacy", refs)
}

func (s *Service) GetPRStatusesBatchForWorkspace(
	ctx context.Context, workspaceID, userID string, refs []PRRef,
) (map[string]*PRStatus, error) {
	resolved, err := s.resolvePersonalReadClient(ctx, workspaceID, userID, "", "")
	if err != nil {
		return nil, err
	}
	settings, err := s.GetWorkspaceSettings(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	filtered := make([]PRRef, 0, len(refs))
	for _, ref := range refs {
		if repositoryInWorkspaceScope(settings, ref.Owner, ref.Repo) {
			filtered = append(filtered, ref)
		}
	}
	visibilityInput := make([]RepoFilter, 0, len(filtered))
	for _, ref := range filtered {
		visibilityInput = append(visibilityInput, RepoFilter{Owner: ref.Owner, Name: ref.Repo})
	}
	visible, err := s.automationRepositoryVisibility(ctx, workspaceID, visibilityInput)
	if err != nil {
		return nil, err
	}
	allowed := filtered[:0]
	for _, ref := range filtered {
		if visible[repositoryIdentityKey(ref.Owner, ref.Repo)] {
			allowed = append(allowed, ref)
		}
	}
	return s.getPRStatusesBatchWithClient(ctx, resolved.Client, resolved.CacheScope, allowed)
}

func (s *Service) getPRStatusesBatchWithClient(
	ctx context.Context, client Client, cacheScope string, refs []PRRef,
) (map[string]*PRStatus, error) {
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
			status, err := s.getPRStatus(ctx, client, cacheScope, r.Owner, r.Repo, r.Number)
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

// --- PR files and commits (live) ---

// GetPRFiles fetches files changed in a PR from GitHub.
func (s *Service) GetPRFiles(ctx context.Context, owner, repo string, number int) ([]PRFile, error) {
	if s.client == nil {
		return nil, fmt.Errorf("github client not available")
	}
	return s.client.ListPRFiles(ctx, owner, repo, number)
}

func (s *Service) GetPRFilesForWorkspace(
	ctx context.Context, workspaceID, userID, owner, repo string, number int,
) ([]PRFile, error) {
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return nil, err
	}
	resolved, err := s.resolvePersonalReadClient(ctx, workspaceID, userID, owner, repo)
	if err != nil {
		return nil, err
	}
	return resolved.Client.ListPRFiles(ctx, owner, repo, number)
}

// GetPRCommits fetches commits in a PR from GitHub.
func (s *Service) GetPRCommits(ctx context.Context, owner, repo string, number int) ([]PRCommitInfo, error) {
	if s.client == nil {
		return nil, fmt.Errorf("github client not available")
	}
	return s.client.ListPRCommits(ctx, owner, repo, number)
}

func (s *Service) GetPRCommitsForWorkspace(
	ctx context.Context, workspaceID, userID, owner, repo string, number int,
) ([]PRCommitInfo, error) {
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return nil, err
	}
	resolved, err := s.resolvePersonalReadClient(ctx, workspaceID, userID, owner, repo)
	if err != nil {
		return nil, err
	}
	return resolved.Client.ListPRCommits(ctx, owner, repo, number)
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

// intPtrEqual compares two nullable int pointers for equality.
func intPtrEqual(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
