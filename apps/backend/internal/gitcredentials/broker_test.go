package gitcredentials

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type recordingAuthorizer struct {
	mu    sync.Mutex
	calls int
	last  Scope
	err   error
}

func (a *recordingAuthorizer) AuthorizeGitCredential(_ context.Context, scope Scope) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls++
	a.last = scope
	return a.err
}

type rotatingProvider struct {
	mu       sync.Mutex
	binding  string
	username string
	secret   string
	calls    int
	err      error
	disabled bool
	started  chan struct{}
	release  chan struct{}
}

func (p *rotatingProvider) Supports(providerID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return providerID == "bitbucket" && !p.disabled
}

func (p *rotatingProvider) Binding(context.Context, Scope) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.binding, p.err
}

func (p *rotatingProvider) Resolve(ctx context.Context, _ Scope) (Credential, error) {
	if p.started != nil {
		select {
		case p.started <- struct{}{}:
		case <-ctx.Done():
			return Credential{}, ctx.Err()
		}
		select {
		case <-p.release:
		case <-ctx.Done():
			return Credential{}, ctx.Err()
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.err != nil {
		return Credential{}, p.err
	}
	return Credential{Username: p.username, Password: p.secret}, nil
}

// pathRecordingProvider captures every scope path a resolver observes, so a
// test can prove the broker never rewrites it.
type pathRecordingProvider struct {
	mu       sync.Mutex
	binding  string
	username string
	secret   string
	paths    []string
}

func (p *pathRecordingProvider) Supports(providerID string) bool { return providerID == "bitbucket" }

func (p *pathRecordingProvider) Binding(_ context.Context, scope Scope) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.paths = append(p.paths, scope.Path)
	return p.binding, nil
}

func (p *pathRecordingProvider) Resolve(_ context.Context, scope Scope) (Credential, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.paths = append(p.paths, scope.Path)
	return Credential{Username: p.username, Password: p.secret}, nil
}

func testScope() Scope {
	return Scope{
		ProviderID: "bitbucket", WorkspaceID: "workspace-1", TaskID: "task-1", SessionID: "session-1",
		RepositoryID: "repository-1", Host: "bitbucket.example", Path: "/scm/ENG/widgets.git",
	}
}

func testRedemption(lease string) Redemption {
	return Redemption{
		Lease: lease, TaskID: "task-1", SessionID: "session-1", RepositoryID: "repository-1",
		Host: "bitbucket.example", Path: "/scm/ENG/widgets.git",
	}
}

func TestBrokerIssuesExactScopedLeaseAndReresolves(t *testing.T) {
	provider := &rotatingProvider{binding: "generation-1", username: "x-token-auth", secret: "first"}
	authorizer := &recordingAuthorizer{}
	broker := NewBroker(provider, authorizer)

	lease, err := broker.Issue(t.Context(), testScope())
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	credential, err := broker.Redeem(t.Context(), testRedemption(lease.Token))
	if err != nil {
		t.Fatalf("Redeem() error = %v", err)
	}
	if credential.Username != "x-token-auth" || credential.Password != "first" {
		t.Fatalf("credential = %#v", credential)
	}
	provider.mu.Lock()
	provider.secret = "rotated"
	provider.mu.Unlock()
	credential, err = broker.Redeem(t.Context(), testRedemption(lease.Token))
	if err != nil {
		t.Fatalf("second Redeem() error = %v", err)
	}
	if credential.Password != "rotated" || provider.calls != 2 {
		t.Fatalf("second credential = %#v, provider calls = %d", credential, provider.calls)
	}
	if authorizer.calls != 3 {
		t.Fatalf("authorizer calls = %d, want issue + two redemptions", authorizer.calls)
	}
}

func TestBrokerRejectsExactScopeMismatch(t *testing.T) {
	provider := &rotatingProvider{binding: "generation-1", username: "user", secret: "secret"}
	broker := NewBroker(provider, &recordingAuthorizer{})
	lease, err := broker.Issue(t.Context(), testScope())
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	for name, mismatch := range map[string]func(*Redemption){
		"task":       func(request *Redemption) { request.TaskID = "other-task" },
		"session":    func(request *Redemption) { request.SessionID = "other-session" },
		"repository": func(request *Redemption) { request.RepositoryID = "other-repository" },
		"host":       func(request *Redemption) { request.Host = "other.example" },
		"path":       func(request *Redemption) { request.Path = "/scm/ENG/other.git" },
	} {
		t.Run(name, func(t *testing.T) {
			request := testRedemption(lease.Token)
			mismatch(&request)
			if _, err := broker.Redeem(t.Context(), request); !errors.Is(err, ErrScopeDenied) {
				t.Fatalf("%s mismatch error = %v, want ErrScopeDenied", name, err)
			}
		})
	}
}

func TestBrokerRejectsRepositoryPathCaseMismatch(t *testing.T) {
	provider := &rotatingProvider{binding: "generation-1", username: "user", secret: "secret"}
	broker := NewBroker(provider, &recordingAuthorizer{})
	scope := testScope()
	scope.Path = "/Team/Repo.git"
	lease, err := broker.Issue(t.Context(), scope)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	request := testRedemption(lease.Token)
	request.Path = "/team/repo.git"
	if _, err := broker.Redeem(t.Context(), request); !errors.Is(err, ErrScopeDenied) {
		t.Fatalf("Redeem() error = %v, want ErrScopeDenied", err)
	}
}

// A scope path comes from a clone URL and keeps its ".git" suffix; the gh CLI
// shim asks for the bare "/<owner>/<repo>". Both name the same repository, so
// the suffix must not decide whether a lease redeems.
func TestBrokerAcceptsRepositoryPathWithoutGitSuffix(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		scopePath  string
		redeemPath string
	}{
		{name: "scope keeps suffix", scopePath: "/scm/ENG/widgets.git", redeemPath: "/scm/ENG/widgets"},
		{name: "redemption keeps suffix", scopePath: "/scm/ENG/widgets", redeemPath: "/scm/ENG/widgets.git"},
		{name: "trailing slash", scopePath: "/scm/ENG/widgets.git", redeemPath: "/scm/ENG/widgets/"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			provider := &rotatingProvider{binding: "generation-1", username: "user", secret: "secret"}
			broker := NewBroker(provider, &recordingAuthorizer{})
			scope := testScope()
			scope.Path = testCase.scopePath
			lease, err := broker.Issue(t.Context(), scope)
			if err != nil {
				t.Fatalf("Issue() error = %v", err)
			}
			request := testRedemption(lease.Token)
			request.Path = testCase.redeemPath
			if _, err := broker.Redeem(t.Context(), request); err != nil {
				t.Fatalf("Redeem() error = %v, want success", err)
			}
		})
	}
}

// Suffix-insensitivity is a comparison rule only. Plugin providers compare the
// scope path verbatim in their binding and resolve RPCs, so a lease must carry
// the exact path it was issued for, not a rewritten one.
func TestBrokerKeepsExactScopePathForResolvers(t *testing.T) {
	provider := &pathRecordingProvider{binding: "generation-1", username: "user", secret: "secret"}
	authorizer := &recordingAuthorizer{}
	broker := NewBroker(provider, authorizer)
	lease, err := broker.Issue(t.Context(), testScope())
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if got := authorizer.last.Path; got != "/scm/ENG/widgets.git" {
		t.Fatalf("authorizer scope path = %q, want the exact issued path", got)
	}
	request := testRedemption(lease.Token)
	request.Path = "/scm/ENG/widgets"
	if _, err := broker.Redeem(t.Context(), request); err != nil {
		t.Fatalf("Redeem() error = %v", err)
	}
	for _, got := range provider.paths {
		if got != "/scm/ENG/widgets.git" {
			t.Fatalf("resolver scope path = %q, want the exact issued path", got)
		}
	}
	if len(provider.paths) == 0 {
		t.Fatal("resolver was never called")
	}
}

func TestBrokerRejectsRepositoryPathMismatch(t *testing.T) {
	provider := &rotatingProvider{binding: "generation-1", username: "user", secret: "secret"}
	broker := NewBroker(provider, &recordingAuthorizer{})
	lease, err := broker.Issue(t.Context(), testScope())
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	request := testRedemption(lease.Token)
	request.Path = "/scm/ENG/widgets-staging"
	if _, err := broker.Redeem(t.Context(), request); !errors.Is(err, ErrScopeDenied) {
		t.Fatalf("Redeem() error = %v, want ErrScopeDenied", err)
	}
}

func TestBrokerExpiresAndRevokesLease(t *testing.T) {
	provider := &rotatingProvider{binding: "generation-1", username: "user", secret: "secret"}
	broker := NewBroker(provider, &recordingAuthorizer{})
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	broker.now = func() time.Time { return now }
	scope := testScope()
	scope.TTL = time.Minute
	lease, err := broker.Issue(t.Context(), scope)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := broker.Redeem(t.Context(), testRedemption(lease.Token)); !errors.Is(err, ErrLeaseExpired) {
		t.Fatalf("expired redemption error = %v, want ErrLeaseExpired", err)
	}

	now = now.Add(-2 * time.Minute)
	lease, err = broker.Issue(t.Context(), scope)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	broker.RevokeSession(scope.SessionID)
	if _, err := broker.Redeem(t.Context(), testRedemption(lease.Token)); !errors.Is(err, ErrLeaseInvalid) {
		t.Fatalf("revoked redemption error = %v, want ErrLeaseInvalid", err)
	}
}

func TestBrokerClampsRequestedTTLToDefaultMaximum(t *testing.T) {
	provider := &rotatingProvider{binding: "generation-1", username: "user", secret: "secret"}
	broker := NewBroker(provider, &recordingAuthorizer{})
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	broker.now = func() time.Time { return now }
	scope := testScope()
	scope.TTL = defaultLeaseTTL + time.Hour

	lease, err := broker.Issue(t.Context(), scope)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if want := now.Add(defaultLeaseTTL); !lease.ExpiresAt.Equal(want) {
		t.Fatalf("ExpiresAt = %s, want clamped maximum %s", lease.ExpiresAt, want)
	}
}

func TestBrokerActiveLeaseCountSweepsExpiredRecords(t *testing.T) {
	provider := &rotatingProvider{binding: "generation-1", username: "user", secret: "secret"}
	broker := NewBroker(provider, &recordingAuthorizer{})
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	broker.now = func() time.Time { return now }
	scope := testScope()
	scope.TTL = time.Minute

	if _, err := broker.Issue(t.Context(), scope); err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	now = now.Add(2 * time.Minute)
	if count := broker.ActiveLeaseCount(); count != 0 {
		t.Fatalf("ActiveLeaseCount() = %d, want expired record swept", count)
	}
	broker.mu.Lock()
	retained := len(broker.leases)
	broker.mu.Unlock()
	if retained != 0 {
		t.Fatalf("retained lease records = %d, want 0 after sweep", retained)
	}
}

func TestBrokerRenewsLeaseOnlyAfterSuccessfulRedemption(t *testing.T) {
	provider := &rotatingProvider{binding: "generation-1", username: "user", secret: "secret"}
	broker := NewBroker(provider, &recordingAuthorizer{})
	issuedAt := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	now := issuedAt
	broker.now = func() time.Time { return now }
	scope := testScope()
	scope.TTL = time.Minute
	lease, err := broker.Issue(t.Context(), scope)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	now = issuedAt.Add(30 * time.Second)
	if _, err := broker.Redeem(t.Context(), testRedemption(lease.Token)); err != nil {
		t.Fatalf("Redeem() error = %v", err)
	}
	now = issuedAt.Add(75 * time.Second)
	if _, err := broker.Redeem(t.Context(), testRedemption(lease.Token)); err != nil {
		t.Fatalf("Redeem() after original expiry error = %v", err)
	}
}

func TestBrokerConcurrentRevocationNeverReturnsCredential(t *testing.T) {
	provider := &rotatingProvider{
		binding: "generation-1", username: "user", secret: "secret", started: make(chan struct{}), release: make(chan struct{}),
	}
	broker := NewBroker(provider, &recordingAuthorizer{})
	lease, err := broker.Issue(t.Context(), testScope())
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	result := make(chan struct {
		credential Credential
		err        error
	}, 1)
	go func() {
		credential, redeemErr := broker.Redeem(context.Background(), testRedemption(lease.Token))
		result <- struct {
			credential Credential
			err        error
		}{credential, redeemErr}
	}()
	select {
	case <-provider.started:
	case <-time.After(5 * time.Second):
		t.Fatal("Redeem() did not reach provider resolution")
	}
	broker.RevokeSession("session-1")
	close(provider.release)
	var got struct {
		credential Credential
		err        error
	}
	select {
	case got = <-result:
	case <-time.After(5 * time.Second):
		t.Fatal("Redeem() did not return after provider release")
	}
	if got.credential.Password != "" || !errors.Is(got.err, ErrLeaseRevoked) {
		t.Fatalf("Redeem() = %#v, %v; want no credential and ErrLeaseRevoked", got.credential, got.err)
	}
}

func TestCompositeResolverRevokesLeaseWhenProviderDisables(t *testing.T) {
	provider := &rotatingProvider{binding: "generation-1", username: "user", secret: "secret"}
	resolver := NewCompositeResolver(provider)
	broker := NewBroker(resolver, &recordingAuthorizer{})
	lease, err := broker.Issue(t.Context(), testScope())
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	provider.mu.Lock()
	provider.disabled = true
	provider.mu.Unlock()
	if _, err := broker.Redeem(t.Context(), testRedemption(lease.Token)); !errors.Is(err, ErrLeaseRevoked) {
		t.Fatalf("disabled provider redemption error = %v, want ErrLeaseRevoked", err)
	}
}

func TestBrokerRevokesProviderLeases(t *testing.T) {
	provider := &rotatingProvider{binding: "generation-1", username: "user", secret: "secret"}
	broker := NewBroker(provider, &recordingAuthorizer{})
	lease, err := broker.Issue(t.Context(), testScope())
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	broker.RevokeProvider("bitbucket")
	if _, err := broker.Redeem(t.Context(), testRedemption(lease.Token)); !errors.Is(err, ErrLeaseInvalid) {
		t.Fatalf("provider-revoked redemption error = %v, want ErrLeaseInvalid", err)
	}
}
