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
	if event == reviewEventApprove {
		user, userErr := s.client.GetAuthenticatedUser(ctx)
		if userErr == nil && user != "" {
			pr, prErr := s.client.GetPR(ctx, owner, repo, number)
			if prErr == nil && pr != nil &&
				strings.EqualFold(strings.TrimSpace(pr.AuthorLogin), strings.TrimSpace(user)) {
				return ErrSelfApprove
			}
		}
	}
	return s.client.SubmitReview(ctx, owner, repo, number, event, body)
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
	if mergeMethod == "" {
		// Resolve to an allowed method up-front so we don't rely on GitHub's
		// "default to merge" behavior, which 405s on repos that disallow
		// merge commits (squash-only / rebase-only). Best-effort: if the
		// lookup fails (or — degenerate config — reports no method allowed),
		// fall back to GitHub's default and surface its error rather than
		// blocking the merge attempt.
		if methods, err := s.GetRepoMergeMethods(ctx, owner, repo); err == nil {
			if pick := pickDefaultMergeMethod(methods); pick != "" {
				mergeMethod = pick
			}
		}
	}
	return s.client.MergePR(ctx, owner, repo, number, mergeMethod)
}

// GetRepoMergeMethods returns the merge methods a repo allows, cached for
// a few minutes since repo settings rarely change.
func (s *Service) GetRepoMergeMethods(ctx context.Context, owner, repo string) (RepoMergeMethods, error) {
	if s.client == nil {
		return RepoMergeMethods{}, ErrNoClient
	}
	key := owner + "/" + repo
	v, err := s.mergeMethodsCache.doOrFetch(key, func() (any, error) {
		return s.client.GetRepoMergeMethods(ctx, owner, repo)
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
