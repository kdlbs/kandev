package github

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type personalAuthMemoryRepository struct {
	mu          sync.Mutex
	workspace   *WorkspaceConnection
	connection  *UserConnection
	tokens      GitHubOAuthTokens
	replaceErr  error
	invalidated bool
	version     int64
}

func (r *personalAuthMemoryRepository) GetWorkspaceConnection(_ context.Context, _ string) (*WorkspaceConnection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.workspace, nil
}

func (r *personalAuthMemoryRepository) GetUserConnection(_ context.Context, _, _ string) (*UserConnection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.connection == nil {
		return nil, nil
	}
	copy := *r.connection
	return &copy, nil
}

func (r *personalAuthMemoryRepository) GetPersonalConnectionGeneration(
	context.Context,
	string,
	string,
) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.currentGenerationLocked(), nil
}

func (r *personalAuthMemoryRepository) currentGenerationLocked() int64 {
	if r.version > 0 {
		return r.version
	}
	return personalConnectionGeneration(r.connection)
}

func (r *personalAuthMemoryRepository) GetPersonalTokens(_ context.Context, _, _ string) (GitHubOAuthTokens, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tokens, nil
}

func (r *personalAuthMemoryRepository) ReplacePersonalConnection(
	_ context.Context, connection *UserConnection, tokens GitHubOAuthTokens, expectedGeneration int64,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.replaceErr != nil {
		return r.replaceErr
	}
	if r.currentGenerationLocked() != expectedGeneration {
		return ErrPersonalConnectionStale
	}
	copy := *connection
	r.connection = &copy
	r.tokens = tokens
	r.version = connection.CredentialGeneration
	return nil
}

func (r *personalAuthMemoryRepository) ReplacePersonalConnectionForFlow(
	_ context.Context,
	connection *UserConnection,
	tokens GitHubOAuthTokens,
	expectedGeneration int64,
	expectedWorkspace WorkspaceConnectionExpectation,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !matchesWorkspaceConnectionExpectation(r.workspace, expectedWorkspace) ||
		r.currentGenerationLocked() != expectedGeneration ||
		connection.CredentialGeneration != expectedGeneration+1 {
		return ErrOAuthFlowStale
	}
	copy := *connection
	r.connection = &copy
	r.tokens = tokens
	r.version = connection.CredentialGeneration
	return nil
}

func (r *personalAuthMemoryRepository) MarkPersonalConnectionInvalid(
	_ context.Context, _, _ string, expectedGeneration int64, _ error,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if personalConnectionGeneration(r.connection) != expectedGeneration {
		return ErrPersonalConnectionStale
	}
	r.invalidated = true
	if r.connection != nil {
		r.connection.Status = ConnectionStatusInvalid
		r.version = r.connection.CredentialGeneration
	}
	return nil
}

func (r *personalAuthMemoryRepository) RevokePersonalConnection(_ context.Context, _, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.version = r.currentGenerationLocked() + 1
	r.connection = nil
	r.tokens = GitHubOAuthTokens{}
	return nil
}

func (r *personalAuthMemoryRepository) RevokePersonalConnectionIfUnchanged(
	_ context.Context,
	expected *UserConnection,
) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !sameUserConnectionVersion(r.connection, expected) {
		return false, nil
	}
	r.version = r.currentGenerationLocked() + 1
	r.connection = nil
	r.tokens = GitHubOAuthTokens{}
	return true, nil
}

func (r *personalAuthMemoryRepository) RevokeWorkspacePersonalConnections(_ context.Context, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.version = r.currentGenerationLocked() + 1
	r.connection = nil
	r.tokens = GitHubOAuthTokens{}
	return nil
}

type personalOAuthRemote struct {
	fakeGitHubAppOAuth
	refreshed  GitHubOAuthTokens
	refreshErr error
	refreshArg string
	refreshes  atomic.Int32
	started    chan struct{}
	release    chan struct{}
}

func (r *personalOAuthRemote) RefreshUserToken(
	ctx context.Context, refreshToken string,
) (GitHubOAuthTokens, error) {
	r.refreshes.Add(1)
	r.refreshArg = refreshToken
	if r.started != nil {
		select {
		case r.started <- struct{}{}:
		default:
		}
	}
	if r.release != nil {
		select {
		case <-r.release:
		case <-ctx.Done():
			return GitHubOAuthTokens{}, ctx.Err()
		}
	}
	return r.refreshed, r.refreshErr
}

func TestPersonalAuthStartUsesPKCEAndWorkspaceAppBoundary(t *testing.T) {
	flowStore := &oauthFlowMemoryStore{}
	flows := NewOAuthFlowManager(flowStore)
	flows.random = strings.NewReader(strings.Repeat("g", oauthRandomBytes*2))
	repo := &personalAuthMemoryRepository{workspace: activeAppWorkspace("workspace-1", 42)}
	service := NewPersonalAuthService(
		PersonalAuthConfig{ClientID: "Iv1.client", CallbackURL: "https://kandev.example/personal/callback"},
		flows, repo, &personalOAuthRemote{},
	)

	started, err := service.Start(context.Background(), "workspace-1", "user-1")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	parsed, err := url.Parse(started.URL)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	query := parsed.Query()
	if parsed.Path != "/login/oauth/authorize" || query.Get("code_challenge_method") != "S256" ||
		query.Get("code_challenge") == "" || query.Get("state") == "" {
		t.Fatalf("personal authorization URL = %q", started.URL)
	}
}

func TestPersonalAuthCompleteAtomicallyStoresVerifiedIdentity(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	flowStore := &oauthFlowMemoryStore{}
	flows := NewOAuthFlowManager(flowStore)
	flows.now = func() time.Time { return now }
	flows.random = strings.NewReader(strings.Repeat("h", oauthRandomBytes*2))
	repo := &personalAuthMemoryRepository{workspace: activeAppWorkspace("workspace-1", 42)}
	refreshExpiry := now.Add(180 * 24 * time.Hour)
	remote := &personalOAuthRemote{fakeGitHubAppOAuth: fakeGitHubAppOAuth{
		tokens: GitHubOAuthTokens{
			AccessToken: "access", RefreshToken: "refresh", AccessExpiresAt: now.Add(8 * time.Hour),
			RefreshExpiresAt: &refreshExpiry,
		},
		user: GitHubOAuthUser{ID: 11, Login: "octocat"}, canAccess: true,
	}}
	service := NewPersonalAuthService(
		PersonalAuthConfig{ClientID: "Iv1.client", CallbackURL: "https://kandev.example/personal/callback"},
		flows, repo, remote,
	)
	service.now = func() time.Time { return now }
	started, err := service.Start(context.Background(), "workspace-1", "user-1")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	_, err = service.Complete(context.Background(), PersonalAuthCallback{
		State: started.State, Code: "oauth-code",
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if repo.connection == nil || repo.connection.GitHubUserID != 11 || repo.connection.Login != "octocat" ||
		repo.connection.CredentialGeneration != 1 || repo.tokens.RefreshToken != "refresh" {
		t.Fatalf("stored personal connection = %+v, tokens = %+v", repo.connection, repo.tokens)
	}
}

func TestPersonalAuthCompleteRejectsUserOutsideWorkspaceInstallation(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	flowStore := &oauthFlowMemoryStore{}
	flows := NewOAuthFlowManager(flowStore)
	flows.now = func() time.Time { return now }
	flows.random = strings.NewReader(strings.Repeat("i", oauthRandomBytes*2))
	repo := &personalAuthMemoryRepository{workspace: activeAppWorkspace("workspace-1", 42)}
	remote := &personalOAuthRemote{fakeGitHubAppOAuth: fakeGitHubAppOAuth{
		tokens: GitHubOAuthTokens{AccessToken: "access", AccessExpiresAt: now.Add(time.Hour)},
		user:   GitHubOAuthUser{ID: 11, Login: "outsider"}, canAccess: false,
	}}
	service := NewPersonalAuthService(
		PersonalAuthConfig{ClientID: "Iv1.client", CallbackURL: "https://kandev.example/personal/callback"},
		flows, repo, remote,
	)
	service.now = func() time.Time { return now }
	started, err := service.Start(context.Background(), "workspace-1", "user-1")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	_, err = service.Complete(context.Background(), PersonalAuthCallback{
		WorkspaceID: "workspace-1", UserID: "user-1", State: started.State, Code: "oauth-code",
	})
	if !errors.Is(err, ErrInstallationAssociationUnverified) || repo.connection != nil {
		t.Fatalf("Complete() error = %v, connection = %+v", err, repo.connection)
	}
}

func TestPersonalAuthCompleteCannotSurviveWorkspaceSwitchToPAT(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	flowStore := &oauthFlowMemoryStore{}
	flows := NewOAuthFlowManager(flowStore)
	flows.now = func() time.Time { return now }
	flows.random = strings.NewReader(strings.Repeat("p", oauthRandomBytes*2))
	repo := &personalAuthMemoryRepository{workspace: activeAppWorkspace("workspace-1", 42)}
	remote := &personalOAuthRemote{fakeGitHubAppOAuth: fakeGitHubAppOAuth{
		tokens: GitHubOAuthTokens{
			AccessToken: "access", RefreshToken: "refresh", AccessExpiresAt: now.Add(time.Hour),
		},
		user: GitHubOAuthUser{ID: 11, Login: "octocat"}, canAccess: true,
	}}
	service := NewPersonalAuthService(
		PersonalAuthConfig{ClientID: "Iv1.client", CallbackURL: "https://kandev.example/personal/callback"},
		flows, repo, remote,
	)
	service.now = func() time.Time { return now }
	started, err := service.Start(context.Background(), "workspace-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	repo.workspace = &WorkspaceConnection{
		WorkspaceID: "workspace-1", Source: ConnectionSourcePAT, Login: "octocat",
		Status: ConnectionStatusActive, CredentialGeneration: 2,
	}

	_, err = service.Complete(context.Background(), PersonalAuthCallback{State: started.State, Code: "oauth-code"})
	if !errors.Is(err, ErrGitHubPersonalRequired) || repo.connection != nil || repo.tokens.AccessToken != "" {
		t.Fatalf("Complete() error = %v, connection = %+v, tokens = %+v", err, repo.connection, repo.tokens)
	}
}

func TestPersonalAuthCompletionRacingPATSwitchLeavesNoPersonalConnection(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	flowStore := &oauthFlowMemoryStore{}
	flows := NewOAuthFlowManager(flowStore)
	flows.now = func() time.Time { return now }
	flows.random = strings.NewReader(strings.Repeat("r", oauthRandomBytes*2))
	repo := &personalAuthMemoryRepository{workspace: activeAppWorkspace("workspace-1", 42)}
	remote := &personalOAuthRemote{fakeGitHubAppOAuth: fakeGitHubAppOAuth{
		tokens: GitHubOAuthTokens{
			AccessToken: "access", RefreshToken: "refresh", AccessExpiresAt: now.Add(time.Hour),
		},
		user: GitHubOAuthUser{ID: 11, Login: "octocat"}, canAccess: true,
		exchangeStarted: make(chan struct{}, 1), releaseExchange: make(chan struct{}),
	}}
	service := NewPersonalAuthService(
		PersonalAuthConfig{ClientID: "Iv1.client", CallbackURL: "https://kandev.example/personal/callback"},
		flows, repo, remote,
	)
	service.now = func() time.Time { return now }
	var workspaceLock sync.Mutex
	service.SetWorkspaceMutationLock(func(string) *sync.Mutex { return &workspaceLock })
	started, err := service.Start(context.Background(), "workspace-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	completeDone := make(chan error, 1)
	go func() {
		_, completeErr := service.Complete(
			context.Background(), PersonalAuthCallback{State: started.State, Code: "oauth-code"},
		)
		completeDone <- completeErr
	}()
	<-remote.exchangeStarted
	patDone := make(chan struct{})
	go func() {
		workspaceLock.Lock()
		repo.mu.Lock()
		repo.workspace = &WorkspaceConnection{
			WorkspaceID: "workspace-1", Source: ConnectionSourcePAT, Login: "octocat",
			Status: ConnectionStatusActive, CredentialGeneration: 2,
		}
		repo.connection = nil
		repo.tokens = GitHubOAuthTokens{}
		repo.mu.Unlock()
		workspaceLock.Unlock()
		close(patDone)
	}()
	close(remote.releaseExchange)
	if err := <-completeDone; err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	<-patDone
	if repo.workspace.Source != ConnectionSourcePAT || repo.connection != nil || repo.tokens.AccessToken != "" {
		t.Fatalf("workspace=%+v connection=%+v tokens=%+v", repo.workspace, repo.connection, repo.tokens)
	}
}

func TestPersonalAuthCompleteCannotOverwriteNewerReconnect(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	flowStore := &oauthFlowMemoryStore{}
	flows := NewOAuthFlowManager(flowStore)
	flows.now = func() time.Time { return now }
	flows.random = strings.NewReader(strings.Repeat("q", oauthRandomBytes*2))
	repo := &personalAuthMemoryRepository{
		workspace: activeAppWorkspace("workspace-1", 42),
		connection: &UserConnection{
			WorkspaceID: "workspace-1", UserID: "user-1", GitHubUserID: 11, Login: "old",
			Status: ConnectionStatusActive, AccessExpiresAt: now.Add(time.Hour), CredentialGeneration: 1,
		},
	}
	remote := &personalOAuthRemote{fakeGitHubAppOAuth: fakeGitHubAppOAuth{
		tokens: GitHubOAuthTokens{
			AccessToken: "stale", RefreshToken: "stale-refresh", AccessExpiresAt: now.Add(time.Hour),
		},
		user: GitHubOAuthUser{ID: 11, Login: "old"}, canAccess: true,
	}}
	service := NewPersonalAuthService(
		PersonalAuthConfig{ClientID: "Iv1.client", CallbackURL: "https://kandev.example/personal/callback"},
		flows, repo, remote,
	)
	service.now = func() time.Time { return now }
	started, err := service.Start(context.Background(), "workspace-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.RevokePersonalConnection(context.Background(), "workspace-1", "user-1"); err != nil {
		t.Fatal(err)
	}
	reconnected := &UserConnection{
		WorkspaceID: "workspace-1", UserID: "user-1", GitHubUserID: 11, Login: "new",
		Status: ConnectionStatusActive, AccessExpiresAt: now.Add(2 * time.Hour), CredentialGeneration: 3,
	}
	if err := repo.ReplacePersonalConnection(context.Background(), reconnected, GitHubOAuthTokens{
		AccessToken: "new", RefreshToken: "new-refresh", AccessExpiresAt: now.Add(2 * time.Hour),
	}, 2); err != nil {
		t.Fatal(err)
	}

	_, err = service.Complete(context.Background(), PersonalAuthCallback{State: started.State, Code: "oauth-code"})
	if !errors.Is(err, ErrOAuthFlowStale) || repo.connection.Login != "new" || repo.tokens.AccessToken != "new" {
		t.Fatalf("Complete() error = %v, connection = %+v, tokens = %+v", err, repo.connection, repo.tokens)
	}
}

func TestPersonalAuthRefreshFailureDoesNotReplaceUsableSecrets(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	oldTokens := GitHubOAuthTokens{
		AccessToken: "old-access", RefreshToken: "old-refresh", AccessExpiresAt: now.Add(time.Minute),
	}
	repo := &personalAuthMemoryRepository{
		workspace: activeAppWorkspace("workspace-1", 42),
		connection: &UserConnection{
			WorkspaceID: "workspace-1", UserID: "user-1", GitHubUserID: 11, Login: "octocat",
			Status: ConnectionStatusActive, AccessExpiresAt: oldTokens.AccessExpiresAt, CredentialGeneration: 4,
		},
		tokens: oldTokens, replaceErr: errors.New("atomic repository commit failed"),
	}
	remote := &personalOAuthRemote{
		fakeGitHubAppOAuth: fakeGitHubAppOAuth{user: GitHubOAuthUser{ID: 11, Login: "octocat"}, canAccess: true},
		refreshed: GitHubOAuthTokens{
			AccessToken: "new-access", RefreshToken: "new-refresh", AccessExpiresAt: now.Add(8 * time.Hour),
		},
	}
	service := NewPersonalAuthService(PersonalAuthConfig{}, nil, repo, remote)
	service.now = func() time.Time { return now }

	_, _, err := service.ResolvePersonalToken(context.Background(), "workspace-1", "user-1")
	if err == nil || repo.tokens.AccessToken != "old-access" || repo.connection.CredentialGeneration != 4 {
		t.Fatalf("ResolvePersonalToken() error = %v, connection = %+v, tokens = %+v", err, repo.connection, repo.tokens)
	}
}

func TestPersonalAuthRefreshRevokesEffectiveAuthWhenInstallationAccessIsLost(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	repo := &personalAuthMemoryRepository{
		workspace: activeAppWorkspace("workspace-1", 42),
		connection: &UserConnection{
			WorkspaceID: "workspace-1", UserID: "user-1", GitHubUserID: 11, Login: "octocat",
			Status: ConnectionStatusActive, AccessExpiresAt: now.Add(time.Minute), CredentialGeneration: 2,
		},
		tokens: GitHubOAuthTokens{AccessToken: "old", RefreshToken: "refresh", AccessExpiresAt: now.Add(time.Minute)},
	}
	remote := &personalOAuthRemote{
		fakeGitHubAppOAuth: fakeGitHubAppOAuth{user: GitHubOAuthUser{ID: 11, Login: "octocat"}, canAccess: false},
		refreshed:          GitHubOAuthTokens{AccessToken: "new", RefreshToken: "new-refresh", AccessExpiresAt: now.Add(time.Hour)},
	}
	service := NewPersonalAuthService(PersonalAuthConfig{}, nil, repo, remote)
	service.now = func() time.Time { return now }
	_, _, err := service.ResolvePersonalToken(context.Background(), "workspace-1", "user-1")
	if !errors.Is(err, ErrInstallationAssociationUnverified) || !repo.invalidated || repo.tokens.AccessToken != "old" {
		t.Fatalf("ResolvePersonalToken() error = %v, invalidated = %v, tokens = %+v", err, repo.invalidated, repo.tokens)
	}
}

func TestPersonalAuthRefreshRejectsChangedGitHubUser(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	repo := &personalAuthMemoryRepository{
		workspace: activeAppWorkspace("workspace-1", 42),
		connection: &UserConnection{
			WorkspaceID: "workspace-1", UserID: "user-1", GitHubUserID: 11, Login: "octocat",
			Status: ConnectionStatusActive, AccessExpiresAt: now.Add(time.Minute), CredentialGeneration: 2,
		},
		tokens: GitHubOAuthTokens{AccessToken: "old", RefreshToken: "refresh", AccessExpiresAt: now.Add(time.Minute)},
	}
	remote := &personalOAuthRemote{
		fakeGitHubAppOAuth: fakeGitHubAppOAuth{user: GitHubOAuthUser{ID: 99, Login: "attacker"}},
		refreshed:          GitHubOAuthTokens{AccessToken: "new", RefreshToken: "new-refresh", AccessExpiresAt: now.Add(time.Hour)},
	}
	service := NewPersonalAuthService(PersonalAuthConfig{}, nil, repo, remote)
	service.now = func() time.Time { return now }
	_, _, err := service.ResolvePersonalToken(context.Background(), "workspace-1", "user-1")
	if !errors.Is(err, ErrPersonalIdentityChanged) || !repo.invalidated || repo.tokens.AccessToken != "old" {
		t.Fatalf("ResolvePersonalToken() error = %v, invalidated = %v, tokens = %+v", err, repo.invalidated, repo.tokens)
	}
}

func TestPersonalAuthRefreshCoalescesConcurrentCallers(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	repo := expiringPersonalAuthRepository(now)
	remote := &personalOAuthRemote{
		fakeGitHubAppOAuth: fakeGitHubAppOAuth{user: GitHubOAuthUser{ID: 11, Login: "octocat"}, canAccess: true},
		refreshed: GitHubOAuthTokens{
			AccessToken: "new", RefreshToken: "new-refresh", AccessExpiresAt: now.Add(time.Hour),
		},
		started: make(chan struct{}, 1), release: make(chan struct{}),
	}
	service := NewPersonalAuthService(PersonalAuthConfig{}, nil, repo, remote)
	service.now = func() time.Time { return now }

	type result struct {
		token string
		err   error
	}
	results := make(chan result, 2)
	resolve := func() {
		token, _, err := service.ResolvePersonalToken(context.Background(), "workspace-1", "user-1")
		results <- result{token: token, err: err}
	}
	go resolve()
	<-remote.started
	go resolve()
	time.Sleep(20 * time.Millisecond)
	close(remote.release)

	for range 2 {
		got := <-results
		if got.err != nil || got.token != "new" {
			t.Fatalf("ResolvePersonalToken() = %q, %v", got.token, got.err)
		}
	}
	if got := remote.refreshes.Load(); got != 1 {
		t.Fatalf("RefreshUserToken calls = %d, want 1", got)
	}
}

func TestPersonalAuthRefreshCallerCancellationDoesNotCancelSharedRotation(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	repo := expiringPersonalAuthRepository(now)
	remote := &personalOAuthRemote{
		fakeGitHubAppOAuth: fakeGitHubAppOAuth{user: GitHubOAuthUser{ID: 11, Login: "octocat"}, canAccess: true},
		refreshed: GitHubOAuthTokens{
			AccessToken: "new", RefreshToken: "new-refresh", AccessExpiresAt: now.Add(time.Hour),
		},
		started: make(chan struct{}, 1), release: make(chan struct{}),
	}
	service := NewPersonalAuthService(PersonalAuthConfig{}, nil, repo, remote)
	service.now = func() time.Time { return now }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, _, err := service.ResolvePersonalToken(ctx, "workspace-1", "user-1")
		done <- err
	}()
	<-remote.started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled caller error = %v", err)
	}
	close(remote.release)
	token, _, err := service.ResolvePersonalToken(context.Background(), "workspace-1", "user-1")
	if err != nil || token != "new" {
		t.Fatalf("ResolvePersonalToken() after canceled caller = %q, %v", token, err)
	}
}

func TestPersonalAuthTransientRefreshFailureDoesNotInvalidateConnection(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	repo := expiringPersonalAuthRepository(now)
	remote := &personalOAuthRemote{refreshErr: errors.New("temporary network failure")}
	service := NewPersonalAuthService(PersonalAuthConfig{}, nil, repo, remote)
	service.now = func() time.Time { return now }

	if _, _, err := service.ResolvePersonalToken(context.Background(), "workspace-1", "user-1"); err == nil {
		t.Fatal("expected refresh failure")
	}
	if repo.invalidated || repo.connection.Status != ConnectionStatusActive {
		t.Fatalf("transient failure invalidated connection: %+v", repo.connection)
	}
}

func TestPersonalAuthInvalidGrantRefreshFailureInvalidatesConnection(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	repo := expiringPersonalAuthRepository(now)
	remote := &personalOAuthRemote{refreshErr: &GitHubOAuthError{Code: "invalid_grant"}}
	service := NewPersonalAuthService(PersonalAuthConfig{}, nil, repo, remote)
	service.now = func() time.Time { return now }

	if _, _, err := service.ResolvePersonalToken(context.Background(), "workspace-1", "user-1"); err == nil {
		t.Fatal("expected refresh failure")
	}
	if !repo.invalidated || repo.connection.Status != ConnectionStatusInvalid {
		t.Fatalf("invalid grant did not invalidate connection: %+v", repo.connection)
	}
}

func TestPersonalAuthRevokeWinsRaceWithRefresh(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	repo := expiringPersonalAuthRepository(now)
	remote := &personalOAuthRemote{
		fakeGitHubAppOAuth: fakeGitHubAppOAuth{user: GitHubOAuthUser{ID: 11, Login: "octocat"}, canAccess: true},
		refreshed: GitHubOAuthTokens{
			AccessToken: "new", RefreshToken: "new-refresh", AccessExpiresAt: now.Add(time.Hour),
		},
		started: make(chan struct{}, 1), release: make(chan struct{}),
	}
	service := NewPersonalAuthService(PersonalAuthConfig{}, nil, repo, remote)
	service.now = func() time.Time { return now }

	done := make(chan error, 1)
	go func() {
		_, _, err := service.ResolvePersonalToken(context.Background(), "workspace-1", "user-1")
		done <- err
	}()
	<-remote.started
	if err := service.Revoke(context.Background(), "workspace-1", "user-1"); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	close(remote.release)
	if err := <-done; !errors.Is(err, ErrPersonalConnectionStale) {
		t.Fatalf("ResolvePersonalToken() error = %v, want stale connection", err)
	}
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if repo.connection != nil || repo.tokens.AccessToken != "" || repo.tokens.RefreshToken != "" {
		t.Fatalf("revoked connection was resurrected: connection=%+v tokens=%+v", repo.connection, repo.tokens)
	}
}

func TestPersonalAuthorizationLateRevokeDoesNotDeleteReconnect(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	current := &UserConnection{
		WorkspaceID: "workspace-1", UserID: "user-1", GitHubUserID: 11, Login: "octocat",
		Status: ConnectionStatusActive, AccessExpiresAt: now.Add(time.Hour),
		CredentialGeneration: 2, UpdatedAt: now.Add(time.Second),
	}
	repo := &personalAuthMemoryRepository{
		workspace: activeAppWorkspace("workspace-1", 42), connection: current,
		tokens: GitHubOAuthTokens{
			AccessToken: "reconnected", RefreshToken: "refresh", AccessExpiresAt: now.Add(time.Hour),
		},
	}
	service := NewPersonalAuthService(
		PersonalAuthConfig{}, nil, repo,
		&personalOAuthRemote{fakeGitHubAppOAuth: fakeGitHubAppOAuth{
			user: GitHubOAuthUser{ID: 11, Login: "octocat"},
		}},
	)

	revoked, err := service.ReconcileAuthorizationRevocation(context.Background(), current)
	if err != nil || revoked || repo.connection == nil || repo.tokens.AccessToken != "reconnected" {
		t.Fatalf("ReconcileAuthorizationRevocation() = %v, %v; connection=%+v tokens=%+v",
			revoked, err, repo.connection, repo.tokens)
	}
}

func expiringPersonalAuthRepository(now time.Time) *personalAuthMemoryRepository {
	return &personalAuthMemoryRepository{
		workspace: activeAppWorkspace("workspace-1", 42),
		connection: &UserConnection{
			WorkspaceID: "workspace-1", UserID: "user-1", GitHubUserID: 11, Login: "octocat",
			Status: ConnectionStatusActive, AccessExpiresAt: now.Add(time.Minute), CredentialGeneration: 2,
		},
		tokens: GitHubOAuthTokens{
			AccessToken: "old", RefreshToken: "refresh", AccessExpiresAt: now.Add(time.Minute),
		},
	}
}

func activeAppWorkspace(workspaceID string, installationID int64) *WorkspaceConnection {
	return &WorkspaceConnection{
		WorkspaceID: workspaceID, Source: ConnectionSourceGitHubAppInstallation,
		GitHubHost: defaultGitHubHost, InstallationID: &installationID,
		InstallationAccountLogin: "acme", InstallationAccountType: "Organization",
		Status: ConnectionStatusActive, CredentialGeneration: 1,
	}
}
