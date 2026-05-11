package github

import (
	"context"
	"sync"
	"time"
)

// BranchProtection captures the subset of a GitHub branch protection rule that
// the CI popover surfaces ("N approvals required to merge"). Stored values
// have a fixed TTL so changes to the protection rule on GitHub propagate
// without restarting the backend.
type BranchProtection struct {
	RequiredApprovingReviewCount int
	HasRule                      bool // false when GitHub returned 404 (no rule)
	FetchedAt                    time.Time
}

// branchProtectionCacheTTL is how long a cached protection lookup stays
// authoritative before a refetch is allowed. One hour balances API quota
// (4-PRs-per-cycle GraphQL polling) against rule-change latency (rare).
const branchProtectionCacheTTL = time.Hour

// branchProtectionCache is an in-memory TTL cache keyed by
// "owner/repo@base_branch". Concurrent reads share the RLock; concurrent
// cache misses for the same key may each issue their own GitHub fetch and
// race to set() the result — values are deterministic so the loser's
// write is harmless, and the extra round-trips are bounded by the 1h TTL.
// Stale entries are evicted on read so the map cannot grow unbounded over
// a long-running process: callers always observe an active TTL window.
type branchProtectionCache struct {
	mu  sync.RWMutex
	m   map[string]BranchProtection
	ttl time.Duration
}

func newBranchProtectionCache() *branchProtectionCache {
	return &branchProtectionCache{
		m:   make(map[string]BranchProtection),
		ttl: branchProtectionCacheTTL,
	}
}

func (c *branchProtectionCache) get(key string) (BranchProtection, bool) {
	c.mu.RLock()
	bp, ok := c.m[key]
	c.mu.RUnlock()
	if !ok {
		return BranchProtection{}, false
	}
	if time.Since(bp.FetchedAt) <= c.ttl {
		return bp, true
	}
	// Evict the stale entry so the map doesn't grow unbounded across
	// long-running deployments with many distinct (owner, repo, branch)
	// keys. Recheck under the write lock so a concurrent set() that just
	// landed isn't clobbered.
	c.mu.Lock()
	if cur, present := c.m[key]; present && time.Since(cur.FetchedAt) > c.ttl {
		delete(c.m, key)
	}
	c.mu.Unlock()
	return BranchProtection{}, false
}

func (c *branchProtectionCache) set(key string, bp BranchProtection) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = bp
}

func branchProtectionKey(owner, repo, branch string) string {
	return owner + "/" + repo + "@" + branch
}

// BranchProtectionFetcher is implemented by the live GitHub clients (PAT, gh
// CLI). The mock client returns a sentinel "no rule" so e2e tests get
// deterministic results unless they seed via the mock controller.
type BranchProtectionFetcher interface {
	FetchBranchProtection(ctx context.Context, owner, repo, branch string) (BranchProtection, error)
}

// fetchRequiredReviews returns the branch-protection-derived
// RequiredApprovingReviewCount for (owner, repo, branch), or nil if:
//   - the active client doesn't implement BranchProtectionFetcher,
//   - the upstream call errors (token lacks scope, network down, etc.),
//   - the repo has no protection rule on the base branch.
//
// Result is cached for branchProtectionCacheTTL. nil never gets cached so a
// transient error is retried on the next sync.
func (s *Service) fetchRequiredReviews(ctx context.Context, owner, repo, branch string) *int {
	if owner == "" || repo == "" || branch == "" {
		return nil
	}
	key := branchProtectionKey(owner, repo, branch)
	if bp, ok := s.protectionCache.get(key); ok {
		if !bp.HasRule {
			return nil
		}
		n := bp.RequiredApprovingReviewCount
		return &n
	}
	fetcher, ok := s.client.(BranchProtectionFetcher)
	if !ok {
		return nil
	}
	bp, err := fetcher.FetchBranchProtection(ctx, owner, repo, branch)
	if err != nil {
		// Don't cache errors — next sync retries.
		return nil
	}
	bp.FetchedAt = time.Now().UTC()
	s.protectionCache.set(key, bp)
	if !bp.HasRule {
		return nil
	}
	n := bp.RequiredApprovingReviewCount
	return &n
}
