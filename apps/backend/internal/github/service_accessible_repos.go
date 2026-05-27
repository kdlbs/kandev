package github

import (
	"context"
	"fmt"
	"sort"

	"golang.org/x/sync/errgroup"
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
// The cache is service-instance-scoped — the authenticated user is fixed for
// the lifetime of the Service — so a constant key is sufficient.
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
// deduped by full_name, and sorted by most-recently-pushed first.
//
// The full merged result is cached per (query, limit) for 60s so picker
// re-renders and typeahead bursts don't fan out to GitHub on every keystroke.
// Returns ErrNoClient (untouched) when GitHub is not configured / not
// authenticated; the HTTP handler maps that to 503 with the
// `github_unavailable` code.
func (s *Service) ListAccessibleRepos(ctx context.Context, query string, limit int) ([]GitHubRepo, error) {
	if s.client == nil {
		return nil, ErrNoClient
	}
	limit = clampAccessibleReposLimit(limit)
	key := accessibleReposCacheKey(query, limit)
	v, err := s.accessibleReposCache.doOrFetch(key, func() (any, error) {
		return s.fetchAccessibleRepos(ctx, query, limit)
	})
	if err != nil {
		return nil, err
	}
	return v.([]GitHubRepo), nil
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

// fetchAccessibleRepos fans out a SearchOrgRepos call per org plus a
// ListUserRepos call for the authenticated user's own repos, then merges,
// dedupes by full_name, sorts by pushed_at desc, and truncates to limit.
// Org calls run in parallel via errgroup so a slow org does not block the
// others.
func (s *Service) fetchAccessibleRepos(ctx context.Context, query string, limit int) ([]GitHubRepo, error) {
	orgs, err := s.cachedListUserOrgs(ctx)
	if err != nil {
		return nil, err
	}
	// Slot 0 is reserved for the authenticated user's own repos; slots 1..n
	// hold the per-org search results, indexed by the org's position in
	// `orgs`. Pre-sized so workers can write without coordinating on a
	// shared append.
	results := make([][]GitHubRepo, len(orgs)+1)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		userRepos, err := s.client.ListUserRepos(gctx, query, limit)
		if err != nil {
			return err
		}
		results[0] = userRepos
		return nil
	})
	for i, org := range orgs {
		i, org := i, org
		g.Go(func() error {
			orgRepos, err := s.client.SearchOrgRepos(gctx, org.Login, query, limit)
			if err != nil {
				return err
			}
			results[i+1] = orgRepos
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return mergeDedupeSortRepos(results, limit), nil
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
		if !merged[i].PushedAt.Equal(merged[j].PushedAt) {
			return merged[i].PushedAt.After(merged[j].PushedAt)
		}
		return merged[i].FullName < merged[j].FullName
	})
	if len(merged) > limit {
		merged = merged[:limit]
	}
	return merged
}
