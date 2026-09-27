package provideraccess

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/github"
)

type fakeLeaseAuthority struct {
	mu       sync.Mutex
	verified VerifiedLease
	drift    bool
	calls    int
}

func (a *fakeLeaseAuthority) VerifyLease(_ context.Context, _ string) (VerifiedLease, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls++
	result := a.verified
	if a.drift && a.calls > 1 {
		result.Expected.SessionID = "other-session"
	}
	return result, nil
}

type fakeRerunTokens struct {
	mu          sync.Mutex
	mintCount   int
	revocations int
	revokeErr   error
	onMint      func()
	overbroad   bool
}

func (f *fakeRerunTokens) Mint(_ context.Context, _ int64, _ string) (github.InstallationToken, error) {
	f.mu.Lock()
	f.mintCount++
	onMint := f.onMint
	f.mu.Unlock()
	if onMint != nil {
		onMint()
	}
	token := github.InstallationToken{Token: "fake-secret",
		ExpiresAt: time.Now().Add(45 * time.Minute),
		Principal: github.TokenPrincipal{PrincipalID: "installation:42", InstallationID: 42},
		Permissions: github.InstallationPermissions{
			"actions": github.PermissionWrite, "pull_requests": github.PermissionRead,
			"metadata": github.PermissionRead,
		},
		Repositories: []github.InstallationTokenRepository{{FullName: "repo-1"}}}
	if f.overbroad {
		token.Permissions["contents"] = github.PermissionWrite
	}
	return token, nil
}

func (f *fakeRerunTokens) Revoke(_ context.Context, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revocations++
	return f.revokeErr
}

func newRuntimeTestFixture(t *testing.T) (*Runtime, *Store, *fakeLeaseAuthority, *fakeRerunTokens, Grant, *Lease) {
	t.Helper()
	store := newGrantTestStore(t)
	grant := testGrant("grant-1")
	if err := store.ReplaceGrant(context.Background(), &grant); err != nil {
		t.Fatal(err)
	}
	lease, err := store.IssueLease(context.Background(), testLeaseClaim(grant))
	if err != nil {
		t.Fatal(err)
	}
	authority := &fakeLeaseAuthority{verified: VerifiedLease{
		LeaseID: lease.ID, GrantID: grant.ID,
		Expected:       testMintClaim(grant, lease).Expected,
		InstallationID: 42, CanonicalRepository: grant.RepositoryID}}
	tokens := &fakeRerunTokens{}
	runtime, err := NewRuntime(store, authority, tokens)
	if err != nil {
		t.Fatal(err)
	}
	return runtime, store, authority, tokens, grant, lease
}

func TestRuntimeRedeemIsOneShotAndReleasesExactToken(t *testing.T) {
	runtime, store, _, tokens, _, lease := newRuntimeTestFixture(t)
	ctx := context.Background()
	issued, err := runtime.Redeem(ctx, lease.ID)
	if err != nil || issued.Token != "fake-secret" {
		t.Fatalf("redemption = %+v, err = %v", issued, err)
	}
	if _, err := runtime.Redeem(ctx, lease.ID); !errors.Is(err, ErrLeaseAlreadyRedeemed) {
		t.Fatalf("second redemption = %v", err)
	}
	if tokens.mintCount != 1 {
		t.Fatalf("mint count = %d", tokens.mintCount)
	}
	if err := runtime.Release(ctx, lease.ID); err != nil {
		t.Fatal(err)
	}
	if tokens.revocations != 1 {
		t.Fatalf("revocations = %d", tokens.revocations)
	}
	if state, err := store.ExposureStateAt(ctx, lease.ID, time.Now()); err != nil ||
		state != ExposureRevokedAtProvider {
		t.Fatalf("released exposure = %s, err = %v", state, err)
	}
}

func TestRuntimeDeniesDriftAndRevokesUnexportedToken(t *testing.T) {
	runtime, store, authority, tokens, _, lease := newRuntimeTestFixture(t)
	authority.drift = true
	if token, err := runtime.Redeem(context.Background(), lease.ID); !errors.Is(err, ErrGrantUnavailable) ||
		token.Token != "" || tokens.revocations != 1 {
		t.Fatalf("drift redemption = %+v, err = %v, revocations = %d", token, err, tokens.revocations)
	}
	if state, err := store.ExposureStateAt(context.Background(), lease.ID, time.Now()); err != nil ||
		state != ExposureUnknown {
		t.Fatalf("drift exposure = %s, err = %v", state, err)
	}
}

func TestRuntimeRejectsOverbroadTokenFromInjectedSource(t *testing.T) {
	runtime, store, _, tokens, _, lease := newRuntimeTestFixture(t)
	tokens.overbroad = true
	if token, err := runtime.Redeem(context.Background(), lease.ID); !errors.Is(err, ErrProviderTokenScope) ||
		token.Token != "" || tokens.revocations != 1 {
		t.Fatalf("overbroad runtime token = %+v, err = %v, revocations = %d",
			token, err, tokens.revocations)
	}
	if state, err := store.ExposureStateAt(context.Background(), lease.ID, time.Now()); err != nil ||
		state != ExposureUnknown {
		t.Fatalf("overbroad exposure = %s, err = %v", state, err)
	}
}

func TestRuntimeWorkspaceFenceRecordsFailedRevocationResidual(t *testing.T) {
	runtime, store, _, tokens, grant, lease := newRuntimeTestFixture(t)
	ctx := context.Background()
	if _, err := runtime.Redeem(ctx, lease.ID); err != nil {
		t.Fatal(err)
	}
	tokens.revokeErr = errors.New("provider echoed fake-secret")
	result, err := runtime.FenceWorkspace(ctx, grant.WorkspaceID)
	if !errors.Is(err, ErrRevocationUnconfirmed) ||
		len(result.ExposedLeaseIDs) != 1 || result.ExposedLeaseIDs[0] != lease.ID {
		t.Fatalf("failed fence revoke = %+v, err = %v", result, err)
	}
	if state, err := store.ExposureStateAt(ctx, lease.ID, time.Now()); err != nil ||
		state != ExposureResidual {
		t.Fatalf("residual exposure = %s, err = %v", state, err)
	}
	if _, err := runtime.Redeem(ctx, lease.ID); !errors.Is(err, ErrGrantUnavailable) {
		t.Fatalf("fenced redemption = %v", err)
	}
}
