package github

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	installationTokenRefreshMargin = 5 * time.Minute
	installationTokenMintTimeout   = 30 * time.Second
)

var (
	ErrInstallationTokenExpired     = errors.New("GitHub App installation token is expired")
	ErrInstallationTokenInvalidated = errors.New("GitHub App installation token was invalidated")
)

// InstallationTokenMinter mints one permission-scoped token.
type InstallationTokenMinter interface {
	MintInstallationToken(context.Context, int64, InstallationPermissions, []string) (InstallationToken, error)
}

type installationTokenCacheKey struct {
	installationID int64
	permissions    string
	repositories   string
}

type installationTokenCacheEntry struct {
	token InstallationToken
}

// InstallationTokenCache keeps GitHub's short-lived installation tokens in
// memory and coalesces concurrent refreshes for the same installation/scope.
type InstallationTokenCache struct {
	minter InstallationTokenMinter

	mu          sync.RWMutex
	entries     map[installationTokenCacheKey]installationTokenCacheEntry
	generations map[int64]uint64
	group       singleflight.Group
	now         func() time.Time
}

func NewInstallationTokenCache(minter InstallationTokenMinter) *InstallationTokenCache {
	return &InstallationTokenCache{
		minter:      minter,
		entries:     make(map[installationTokenCacheKey]installationTokenCacheEntry),
		generations: make(map[int64]uint64),
		now:         time.Now,
	}
}

// Get returns a valid token and begins refreshing before expiry. Concurrent
// callers for the same installation and permission scope share one mint.
func (c *InstallationTokenCache) Get(
	ctx context.Context,
	installationID int64,
	permissions InstallationPermissions,
	repositories []string,
) (InstallationToken, error) {
	if c == nil || c.minter == nil {
		return InstallationToken{}, errors.New("GitHub App installation token minter is not configured")
	}
	permissions = clonePermissions(permissions)
	repositories = canonicalRepositories(repositories)
	key := installationTokenCacheKey{
		installationID: installationID,
		permissions:    canonicalPermissions(permissions), repositories: strings.Join(repositories, ","),
	}
	if token, ok := c.cached(key, installationTokenRefreshMargin); ok {
		return token, nil
	}

	generation := c.generation(installationID)
	flightKey := fmt.Sprintf("%d:%d:%s:%s", installationID, generation, key.permissions, key.repositories)
	result := c.refresh(flightKey, ctx, key, installationID, generation, permissions, repositories)
	select {
	case <-ctx.Done():
		return InstallationToken{}, ctx.Err()
	case resolved := <-result:
		return c.resolveRefreshResult(key, resolved)
	}
}

func (c *InstallationTokenCache) refresh(
	flightKey string,
	ctx context.Context,
	key installationTokenCacheKey,
	installationID int64,
	generation uint64,
	permissions InstallationPermissions,
	repositories []string,
) <-chan singleflight.Result {
	return c.group.DoChan(flightKey, func() (any, error) {
		if token, ok := c.cached(key, installationTokenRefreshMargin); ok {
			return token, nil
		}
		mintCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), installationTokenMintTimeout)
		defer cancel()
		token, err := c.minter.MintInstallationToken(
			mintCtx, installationID, clonePermissions(permissions), append([]string(nil), repositories...),
		)
		if err != nil {
			return InstallationToken{}, err
		}
		if token.Token == "" || !token.ExpiresAt.After(c.now()) {
			return InstallationToken{}, ErrInstallationTokenExpired
		}

		c.mu.Lock()
		defer c.mu.Unlock()
		if c.generations[installationID] != generation {
			return InstallationToken{}, ErrInstallationTokenInvalidated
		}
		c.entries[key] = installationTokenCacheEntry{token: cloneInstallationToken(token)}
		return token, nil
	})
}

func (c *InstallationTokenCache) resolveRefreshResult(
	key installationTokenCacheKey,
	resolved singleflight.Result,
) (InstallationToken, error) {
	if resolved.Err != nil {
		if !errors.Is(resolved.Err, ErrInstallationTokenInvalidated) {
			if token, ok := c.cached(key, 0); ok {
				return token, nil
			}
		}
		return InstallationToken{}, resolved.Err
	}
	token, ok := resolved.Val.(InstallationToken)
	if !ok || token.Token == "" || !token.ExpiresAt.After(c.now()) {
		return InstallationToken{}, ErrInstallationTokenExpired
	}
	return cloneInstallationToken(token), nil
}

// Invalidate prevents cached or in-flight results for an installation from
// being reused after disconnect, suspension, revocation, or replacement.
func (c *InstallationTokenCache) Invalidate(installationID int64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generations[installationID]++
	for key := range c.entries {
		if key.installationID == installationID {
			delete(c.entries, key)
		}
	}
}

func (c *InstallationTokenCache) generation(installationID int64) uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.generations[installationID]
}

func (c *InstallationTokenCache) cached(
	key installationTokenCacheKey,
	refreshMargin time.Duration,
) (InstallationToken, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || entry.token.Token == "" || !entry.token.ExpiresAt.After(c.now().Add(refreshMargin)) {
		return InstallationToken{}, false
	}
	return cloneInstallationToken(entry.token), true
}

func canonicalPermissions(permissions InstallationPermissions) string {
	keys := make([]string, 0, len(permissions))
	for name := range permissions {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, name := range keys {
		parts = append(parts, name+"="+string(permissions[name]))
	}
	return strings.Join(parts, ",")
}

func canonicalRepositories(repositories []string) []string {
	unique := make(map[string]struct{}, len(repositories))
	for _, repository := range repositories {
		repository = strings.ToLower(strings.TrimSpace(repository))
		if repository != "" {
			unique[repository] = struct{}{}
		}
	}
	result := make([]string, 0, len(unique))
	for repository := range unique {
		result = append(result, repository)
	}
	sort.Strings(result)
	return result
}

func clonePermissions(permissions InstallationPermissions) InstallationPermissions {
	if permissions == nil {
		return nil
	}
	cloned := make(InstallationPermissions, len(permissions))
	for name, level := range permissions {
		cloned[name] = level
	}
	return cloned
}

func cloneInstallationToken(token InstallationToken) InstallationToken {
	token.Permissions = clonePermissions(token.Permissions)
	return token
}
