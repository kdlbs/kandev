package github

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"go.uber.org/zap"
)

// Repo-list defaults for the list-accessible-repos endpoint.
//
// The default of 50 mirrors the autocomplete pickers in the web UI; the cap
// of 100 keeps a single misbehaving caller from fanning a huge merge across
// every org the user belongs to (GitHub search itself caps per_page at 100).
const (
	defaultAccessibleReposLimit = 50
	maxAccessibleReposLimit     = 100
)

// userOrgsCacheKey is the singleton key for the cached ListUserOrgs result.
//
// The authenticated user can change at runtime (ConfigureToken / ClearToken
// swap s.client without rebuilding the Service), so a constant key would
// surface the previous identity's orgs to the next caller. Both ConfigureToken
// and ClearToken invoke ClearAccessibleReposCaches to invalidate this cache
// (and the merged accessibleReposCache) on every auth change, which keeps the
// constant key safe.
const userOrgsCacheKey = "user-orgs"

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

// ListAccessibleRepos returns the union of repos the authenticated user can
// access — their own repos plus repos under each org they belong to — merged,
// deduped by full_name, and sorted by most-recently-pushed first. Returns a
// non-nil (possibly empty) slice when err == nil.
//
// The full merged result is cached per (query, limit) for 60s so picker
// re-renders and typeahead bursts don't fan out to GitHub on every keystroke.
// Returns ErrNoClient (untouched) when GitHub is not configured / not
// authenticated; the HTTP handler maps that to 503 with the
// `github_not_configured` code.
//
// Per-source failures (a single org or the user-repos call) are logged and
// skipped — the picker UI gets a partial result rather than nothing when one
// of N orgs is temporarily failing. Total failure (every source errored or
// ctx canceled) still surfaces as an error.
//
// Cache write policy: only fully successful fan-outs are cached. If any
// source errored (rate limit, transient 5xx, etc.), the partial result is
// returned to the caller but NOT written to the cache — the next call
// re-fans out so a recovering source can contribute. Caching partial
// failures was a footgun in practice: rate-limited org searches would
// poison the cache for 60s with a result missing every org repo, and the
// picker would render "No repositories found" even for repos the user
// definitely has access to.
func (s *Service) ListAccessibleRepos(ctx context.Context, query string, limit int) ([]GitHubRepo, error) {
	if s.client == nil {
		return nil, ErrNoClient
	}
	limit = clampAccessibleReposLimit(limit)
	key := accessibleReposCacheKey(query, limit)
	// Cache lookup is best-effort: a previous fully-successful fetch may have
	// populated the entry. On miss we fan out directly (no singleflight here
	// — the per-source caches above absorb concurrent traffic, and we need
	// the post-fetch `hadErrors` signal to decide whether to write back).
	if v, ok := s.accessibleReposCache.get(key); ok {
		return v.([]GitHubRepo), nil
	}
	repos, hadErrors, err := s.fetchAccessibleRepos(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	if !hadErrors {
		s.accessibleReposCache.set(key, repos)
	}
	return repos, nil
}

// accessibleReposCacheKey composes a cache key with length-prefixed string
// fields so a user-supplied query containing the separator can't collide with
// another key.
func accessibleReposCacheKey(query string, limit int) string {
	return fmt.Sprintf("%d:%s|%d", len(query), query, limit)
}

// cachedListUserOrgs returns the authenticated user's org list cached for 60s.
// Concurrent misses for the same key coalesce via the cache's singleflight, so
// a burst of repo-list requests won't each issue a separate /user/orgs round
// trip.
//
// Note: this calls the raw client.ListUserOrgs rather than Service.ListUserOrgs
// because Service.ListUserOrgs prepends the authenticated user as a pseudo-org,
// which would double-count against the separate ListUserRepos fan-out in
// fetchAccessibleRepos.
func (s *Service) cachedListUserOrgs(ctx context.Context) ([]GitHubOrg, error) {
	v, err := s.userOrgsCache.doOrFetch(userOrgsCacheKey, func() (any, error) {
		return s.client.ListUserOrgs(ctx)
	})
	if err != nil {
		return nil, err
	}
	orgs, _ := v.([]GitHubOrg)
	return orgs, nil
}

// ClearAccessibleReposCaches drops every cached entry from the accessible-repos
// and user-orgs caches. Used by the e2e mock controller so flipping the mock's
// "repos unavailable" toggle takes effect immediately instead of waiting for the
// 60s TTL on a prior cached success to expire, and by ConfigureToken / ClearToken
// so an auth change invalidates the user-scoped caches synchronously.
//
// Nil-guards each cache because tests construct Service literals without going
// through NewService (so the caches stay nil).
func (s *Service) ClearAccessibleReposCaches() {
	if s.accessibleReposCache != nil {
		s.accessibleReposCache.clear()
	}
	if s.userOrgsCache != nil {
		s.userOrgsCache.clear()
	}
}

// fetchAccessibleRepos fans out a SearchOrgRepos call per org plus a
// ListUserRepos call for the authenticated user's own repos, then merges,
// dedupes by full_name, sorts by pushed_at desc, and truncates to limit.
// Per-source errors are logged and the source contributes zero repos; the
// call returns an error only when ErrNoClient surfaces, the context is
// canceled, or every source failed.
//
// The second return value (`hadErrors`) is true when at least one non-fatal
// source errored (the partial result was still returned). ListAccessibleRepos
// uses it to skip the cache write so a recovering source can contribute on
// the next call instead of being shadowed by a 60s stale-cache.
func (s *Service) fetchAccessibleRepos(ctx context.Context, query string, limit int) ([]GitHubRepo, bool, error) {
	orgs, err := s.cachedListUserOrgs(ctx)
	if err != nil {
		return nil, false, err
	}
	// Slot 0 is reserved for the authenticated user's own repos; slots 1..n
	// hold the per-org search results, indexed by the org's position in
	// `orgs`. Pre-sized so workers can write without coordinating on a
	// shared append.
	results := make([][]GitHubRepo, len(orgs)+1)
	sourceCount := len(orgs) + 1
	var (
		wg          sync.WaitGroup
		mu          sync.Mutex
		fatalErr    error
		failedCount int
	)
	recordResult := func(source string, repos []GitHubRepo, slot int, err error) {
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			if errors.Is(err, ErrNoClient) && fatalErr == nil {
				fatalErr = err
			} else if (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) && fatalErr == nil {
				fatalErr = err
			}
			failedCount++
			if s.logger != nil {
				s.logger.Warn("list-accessible-repos: source failed",
					zap.String("source", source),
					zap.Error(err))
			}
			return
		}
		results[slot] = repos
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		userRepos, err := s.client.ListUserRepos(ctx, query, limit)
		recordResult("user-repos", userRepos, 0, err)
	}()
	for i, org := range orgs {
		i, org := i, org
		wg.Add(1)
		go func() {
			defer wg.Done()
			orgRepos, err := s.client.SearchOrgRepos(ctx, org.Login, query, limit)
			recordResult("org:"+org.Login, orgRepos, i+1, err)
		}()
	}
	wg.Wait()
	if fatalErr != nil {
		return nil, false, fatalErr
	}
	if sourceCount > 0 && failedCount == sourceCount {
		return nil, true, fmt.Errorf("all repo sources failed")
	}
	return mergeDedupeSortRepos(results, limit), failedCount > 0, nil
}

// mergeDedupeSortRepos collapses the parallel fan-out results into a single
// list: first-seen wins on duplicate full_name, then sort by pushed_at desc
// (tiebroken alphabetically on full_name for determinism), then truncate to
// limit. Order across the input slices is irrelevant after sorting — the
// dedupe pass only decides which copy's metadata to keep.
func mergeDedupeSortRepos(results [][]GitHubRepo, limit int) []GitHubRepo {
	seen := make(map[string]struct{})
	merged := make([]GitHubRepo, 0)
	for _, list := range results {
		for _, r := range list {
			if _, ok := seen[r.FullName]; ok {
				continue
			}
			seen[r.FullName] = struct{}{}
			merged = append(merged, r)
		}
	}
	sort.SliceStable(merged, func(i, j int) bool {
		// nil PushedAt sorts last (unknown timestamp is treated as "oldest").
		switch {
		case merged[i].PushedAt == nil && merged[j].PushedAt == nil:
			return merged[i].FullName < merged[j].FullName
		case merged[i].PushedAt == nil:
			return false
		case merged[j].PushedAt == nil:
			return true
		}
		if !merged[i].PushedAt.Equal(*merged[j].PushedAt) {
			return merged[i].PushedAt.After(*merged[j].PushedAt)
		}
		return merged[i].FullName < merged[j].FullName
	})
	if len(merged) > limit {
		merged = merged[:limit]
	}
	return merged
}
