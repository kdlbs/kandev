package github

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type stubInstallationTokenMinter struct {
	calls atomic.Int64
	mint  func(context.Context, int64, InstallationPermissions, []string) (InstallationToken, error)
}

func (s *stubInstallationTokenMinter) MintInstallationToken(
	ctx context.Context,
	installationID int64,
	permissions InstallationPermissions,
	repositories []string,
) (InstallationToken, error) {
	s.calls.Add(1)
	return s.mint(ctx, installationID, permissions, repositories)
}

func TestInstallationTokenCache_CoalescesAndRefreshes(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	release := make(chan struct{})
	minter := &stubInstallationTokenMinter{}
	minter.mint = func(_ context.Context, installationID int64, permissions InstallationPermissions, _ []string) (InstallationToken, error) {
		<-release
		return InstallationToken{Token: "first", ExpiresAt: now.Add(time.Hour)}, nil
	}
	cache := NewInstallationTokenCache(minter)
	cache.now = func() time.Time { return now }
	permissions := InstallationPermissions{"contents": PermissionWrite}

	const callers = 20
	var wg sync.WaitGroup
	var ready sync.WaitGroup
	wg.Add(callers)
	ready.Add(callers)
	start := make(chan struct{})
	errs := make(chan error, callers)
	for range callers {
		go func() {
			defer wg.Done()
			ready.Done()
			<-start
			token, err := cache.Get(context.Background(), 42, permissions, nil)
			if err == nil && token.Token != "first" {
				err = errors.New("unexpected token")
			}
			errs <- err
		}()
	}
	ready.Wait()
	close(start)
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
	}
	if got := minter.calls.Load(); got != 1 {
		t.Fatalf("mint calls = %d, want 1", got)
	}

	minter.mint = func(_ context.Context, _ int64, _ InstallationPermissions, _ []string) (InstallationToken, error) {
		return InstallationToken{Token: "second", ExpiresAt: now.Add(2 * time.Hour)}, nil
	}
	now = now.Add(56 * time.Minute)
	token, err := cache.Get(context.Background(), 42, permissions, nil)
	if err != nil {
		t.Fatalf("refresh Get: %v", err)
	}
	if token.Token != "second" || minter.calls.Load() != 2 {
		t.Fatalf("refresh token = %q, calls = %d", token.Token, minter.calls.Load())
	}
}

func TestInstallationTokenCache_RefreshFailureUsesOnlyUnexpiredToken(t *testing.T) {
	now := time.Now()
	mintErr := errors.New("mint failed")
	minter := &stubInstallationTokenMinter{}
	minter.mint = func(_ context.Context, _ int64, _ InstallationPermissions, _ []string) (InstallationToken, error) {
		if minter.calls.Load() == 1 {
			return InstallationToken{Token: "first", ExpiresAt: now.Add(time.Hour)}, nil
		}
		return InstallationToken{}, mintErr
	}
	cache := NewInstallationTokenCache(minter)
	cache.now = func() time.Time { return now }
	first, err := cache.Get(context.Background(), 1, nil, nil)
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}

	now = now.Add(56 * time.Minute)
	fallback, err := cache.Get(context.Background(), 1, nil, nil)
	if err != nil || fallback.Token != first.Token {
		t.Fatalf("valid fallback = %+v, %v", fallback, err)
	}

	now = now.Add(5 * time.Minute)
	if _, err := cache.Get(context.Background(), 1, nil, nil); !errors.Is(err, mintErr) {
		t.Fatalf("expired fallback error = %v, want mint error", err)
	}
}

func TestInstallationTokenCache_InvalidateRejectsInflightMint(t *testing.T) {
	now := time.Now()
	started := make(chan struct{})
	release := make(chan struct{})
	minter := &stubInstallationTokenMinter{}
	minter.mint = func(_ context.Context, _ int64, _ InstallationPermissions, _ []string) (InstallationToken, error) {
		close(started)
		<-release
		return InstallationToken{Token: "stale", ExpiresAt: now.Add(time.Hour)}, nil
	}
	cache := NewInstallationTokenCache(minter)
	cache.now = func() time.Time { return now }
	result := make(chan error, 1)
	go func() {
		_, err := cache.Get(context.Background(), 1, nil, nil)
		result <- err
	}()
	<-started
	cache.Invalidate(1)
	close(release)
	if err := <-result; !errors.Is(err, ErrInstallationTokenInvalidated) {
		t.Fatalf("in-flight Get error = %v, want invalidated", err)
	}
}

func TestInstallationTokenCache_PermissionScopeIsPartOfKey(t *testing.T) {
	now := time.Now()
	minter := &stubInstallationTokenMinter{}
	minter.mint = func(_ context.Context, _ int64, _ InstallationPermissions, _ []string) (InstallationToken, error) {
		return InstallationToken{Token: "token", ExpiresAt: now.Add(time.Hour)}, nil
	}
	cache := NewInstallationTokenCache(minter)
	cache.now = func() time.Time { return now }
	_, _ = cache.Get(context.Background(), 1, InstallationPermissions{"contents": PermissionRead}, nil)
	_, _ = cache.Get(context.Background(), 1, InstallationPermissions{"contents": PermissionWrite}, nil)
	_, _ = cache.Get(context.Background(), 2, InstallationPermissions{"contents": PermissionRead}, nil)
	if got := minter.calls.Load(); got != 3 {
		t.Fatalf("mint calls = %d, want 3 distinct cache keys", got)
	}
}

func TestInstallationTokenCache_RepositoryScopeIsPartOfKey(t *testing.T) {
	now := time.Now()
	minter := &stubInstallationTokenMinter{}
	minter.mint = func(_ context.Context, _ int64, _ InstallationPermissions, repositories []string) (InstallationToken, error) {
		return InstallationToken{Token: strings.Join(repositories, ","), ExpiresAt: now.Add(time.Hour)}, nil
	}
	cache := NewInstallationTokenCache(minter)
	cache.now = func() time.Time { return now }
	first, err := cache.Get(context.Background(), 1, nil, []string{"frontend"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := cache.Get(context.Background(), 1, nil, []string{"backend"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := cache.Get(context.Background(), 1, nil, []string{"frontend"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Token != "frontend" || second.Token != "backend" || again.Token != first.Token || minter.calls.Load() != 2 {
		t.Fatalf("tokens = %q/%q/%q, calls = %d", first.Token, second.Token, again.Token, minter.calls.Load())
	}
}

func TestInstallationTokenCache_NeverReturnsExpiredToken(t *testing.T) {
	now := time.Now()
	minter := &stubInstallationTokenMinter{}
	minter.mint = func(_ context.Context, _ int64, _ InstallationPermissions, _ []string) (InstallationToken, error) {
		return InstallationToken{Token: "expired", ExpiresAt: now}, nil
	}
	cache := NewInstallationTokenCache(minter)
	cache.now = func() time.Time { return now }
	if _, err := cache.Get(context.Background(), 1, nil, nil); !errors.Is(err, ErrInstallationTokenExpired) {
		t.Fatalf("Get error = %v, want ErrInstallationTokenExpired", err)
	}
}

func TestInstallationTokenCache_InvalidateDropsCachedToken(t *testing.T) {
	now := time.Now()
	minter := &stubInstallationTokenMinter{}
	minter.mint = func(_ context.Context, _ int64, _ InstallationPermissions, _ []string) (InstallationToken, error) {
		call := minter.calls.Load()
		return InstallationToken{Token: string(rune('a' + call)), ExpiresAt: now.Add(time.Hour)}, nil
	}
	cache := NewInstallationTokenCache(minter)
	cache.now = func() time.Time { return now }
	first, err := cache.Get(context.Background(), 1, nil, nil)
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	cache.Invalidate(1)
	second, err := cache.Get(context.Background(), 1, nil, nil)
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if first.Token == second.Token || minter.calls.Load() != 2 {
		t.Fatalf("tokens = %q/%q, calls = %d", first.Token, second.Token, minter.calls.Load())
	}
}

func TestInstallationTokenCache_ReturnedPermissionsCannotMutateCache(t *testing.T) {
	now := time.Now()
	minter := &stubInstallationTokenMinter{}
	minter.mint = func(_ context.Context, _ int64, _ InstallationPermissions, _ []string) (InstallationToken, error) {
		return InstallationToken{
			Token:       "token",
			ExpiresAt:   now.Add(time.Hour),
			Permissions: InstallationPermissions{"contents": PermissionRead},
		}, nil
	}
	cache := NewInstallationTokenCache(minter)
	cache.now = func() time.Time { return now }
	first, err := cache.Get(context.Background(), 1, nil, nil)
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	first.Permissions["contents"] = PermissionWrite
	second, err := cache.Get(context.Background(), 1, nil, nil)
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if second.Permissions["contents"] != PermissionRead {
		t.Fatalf("cached permissions mutated to %q", second.Permissions["contents"])
	}
}
