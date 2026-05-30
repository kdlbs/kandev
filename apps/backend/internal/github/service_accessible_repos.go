package github

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// Repo-list defaults for the list-accessible-repos endpoint.
//
// The default of 50 mirrors the autocomplete pickers in the web UI; the cap
// of 100 matches GitHub's per_page maximum and the frontend picker's own cap.
const (
	defaultAccessibleReposLimit = 50
	maxAccessibleReposLimit     = 100
)

// accessibleReposFetchTimeout bounds the single GET /user/repos fetch. It
// matches the gh CLI's default 30s timeout. The fetch runs under a detached
// context (see ListAccessibleRepos) so this deadline — not a caller's request
// cancellation — is what stops the work.
const accessibleReposFetchTimeout = 30 * time.Second

// clampAccessibleReposLimit normalises caller-supplied limit values: <=0 falls
// back to the default, anything above the cap is clamped down. Mirrors the
// existing clampRepoSearchLimit shape so callers see consistent behaviour
// between this endpoint and the autocomplete /repos/search endpoint.
func clampAccessibleReposLimit(limit int) int {
	if limit <= 0 {
		return defaultAccessibleReposLimit
	}
	if limit > maxAccessibleReposLimit {
		return maxAccessibleReposLimit
	}
	return limit
}

// ListAccessibleRepos returns the repos the authenticated user can access —
// their own repos plus collaborator and org-member repos — via a single
// GET /user/repos call on GitHub's core REST quota (5000/min). Results are
// sorted by most-recently-pushed first and returned as a non-nil (possibly
// empty) slice when err == nil.
//
// The result is cached per (query, limit) for 60s so picker re-renders and
// typeahead bursts don't hit GitHub on every keystroke. Returns ErrNoClient
// (untouched) when GitHub is not configured / not authenticated; the HTTP
// handler maps that to 503 with the `github_not_configured` code. Any other
// error is returned to the caller and NOT cached, so the next call retries.
//
// The cached fetch runs under a context detached from the caller's request
// (context.WithoutCancel). This is the core correctness fix: the picker's
// frontend aborts in-flight requests via an AbortController, and a `gh`
// subprocess started under the request context would be SIGKILLed on that
// abort — surfacing as `signal: killed` to every coalesced caller. Detaching
// lets the fetch outlive any single request; only the fetch timeout stops it.
func (s *Service) ListAccessibleRepos(ctx context.Context, query string, limit int) ([]GitHubRepo, error) {
	if s.client == nil {
		return nil, ErrNoClient
	}
	limit = clampAccessibleReposLimit(limit)
	key := accessibleReposCacheKey(query, limit)
	if v, ok := s.accessibleReposCache.get(key); ok {
		return v.([]GitHubRepo), nil
	}
	// Snapshot the cache generation BEFORE the fetch. If a clear() runs while
	// this fetch is in flight (token swap via ConfigureToken/ClearToken, or the
	// e2e mock toggle), the generation bumps and the post-fetch write is
	// dropped — otherwise a fetch that started under the previous user/token
	// could write that user's stale repos back into the just-cleared cache.
	gen := s.accessibleReposCache.generation()
	// Detach from the caller's request context so a client-side abort can't
	// cancel (and SIGKILL) the shared fetch; bound it with our own timeout.
	fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), accessibleReposFetchTimeout)
	defer cancel()
	repos, err := s.client.ListAccessibleRepos(fetchCtx, query, limit)
	if err != nil {
		return nil, err
	}
	sortReposByPushedAt(repos)
	if len(repos) > limit {
		repos = repos[:limit]
	}
	s.accessibleReposCache.setIfCurrentGeneration(key, repos, gen)
	return repos, nil
}

// accessibleReposCacheKey composes a cache key with length-prefixed string
// fields so a user-supplied query containing the separator can't collide with
// another key.
func accessibleReposCacheKey(query string, limit int) string {
	return fmt.Sprintf("%d:%s|%d", len(query), query, limit)
}

// ClearAccessibleReposCaches drops every cached entry from the accessible-repos
// cache. Used by the e2e mock controller so flipping the mock's "repos
// unavailable" toggle takes effect immediately instead of waiting for the 60s
// TTL on a prior cached success to expire, and by ConfigureToken / ClearToken
// so an auth change invalidates the user-scoped cache synchronously.
//
// Nil-guards the cache because tests construct Service literals without going
// through NewService (so the cache stays nil).
func (s *Service) ClearAccessibleReposCaches() {
	if s.accessibleReposCache != nil {
		s.accessibleReposCache.clear()
	}
}

// sortReposByPushedAt sorts repos in place by pushed_at desc, tiebroken
// alphabetically on full_name for determinism. A nil PushedAt sorts last
// (unknown timestamp is treated as "oldest"). GitHub's /user/repos already
// returns sort=pushed order, but we re-sort defensively so the contract holds
// even if the upstream order changes or the query filter reorders nothing.
func sortReposByPushedAt(repos []GitHubRepo) {
	sort.SliceStable(repos, func(i, j int) bool {
		switch {
		case repos[i].PushedAt == nil && repos[j].PushedAt == nil:
			return repos[i].FullName < repos[j].FullName
		case repos[i].PushedAt == nil:
			return false
		case repos[j].PushedAt == nil:
			return true
		}
		if !repos[i].PushedAt.Equal(*repos[j].PushedAt) {
			return repos[i].PushedAt.After(*repos[j].PushedAt)
		}
		return repos[i].FullName < repos[j].FullName
	})
}
