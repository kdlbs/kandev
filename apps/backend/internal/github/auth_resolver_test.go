package github

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeConnectionReader struct {
	workspaces map[string]*WorkspaceConnection
	users      map[string]*UserConnection
}

func (f *fakeConnectionReader) GetWorkspaceConnection(_ context.Context, workspaceID string) (*WorkspaceConnection, error) {
	return f.workspaces[workspaceID], nil
}

func (f *fakeConnectionReader) GetUserConnection(_ context.Context, workspaceID, userID string) (*UserConnection, error) {
	return f.users[workspaceID+":"+userID], nil
}

type fakeAuthSecrets map[string]string

type repoScopedInstallationProvider struct {
	clients map[string]Client
	calls   int
}

type registrationInstallationProvider struct {
	calls map[string]int
}

func (p *registrationInstallationProvider) ResolveInstallation(
	_ context.Context,
	connection *WorkspaceConnection,
	_ ResolveCredentialRequest,
) (*ResolvedCredential, error) {
	p.calls[connection.AppRegistrationID]++
	return &ResolvedCredential{
		Client: NewMockClient(),
		Principal: AuthPrincipal{
			Kind: AuthPrincipalApp, Source: ConnectionSourceGitHubAppInstallation,
			AppRegistrationID: connection.AppRegistrationID,
		},
		AppRegistrationID:       connection.AppRegistrationID,
		AppCredentialGeneration: 7,
	}, nil
}

type expiringInstallationProvider struct {
	now      func() time.Time
	lifetime time.Duration
	calls    int
}

type synchronizedConnectionReader struct {
	mu         sync.Mutex
	connection WorkspaceConnection
}

func (r *synchronizedConnectionReader) GetWorkspaceConnection(
	_ context.Context,
	_ string,
) (*WorkspaceConnection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	connection := r.connection
	if r.connection.InstallationID != nil {
		installationID := *r.connection.InstallationID
		connection.InstallationID = &installationID
	}
	return &connection, nil
}

func (r *synchronizedConnectionReader) GetUserConnection(
	context.Context,
	string,
	string,
) (*UserConnection, error) {
	return nil, nil
}

func (r *synchronizedConnectionReader) replace(connection WorkspaceConnection) {
	r.mu.Lock()
	r.connection = connection
	r.mu.Unlock()
}

type generationBarrierProvider struct {
	oldStarted chan struct{}
	releaseOld chan struct{}
	oldClient  Client
	newClient  Client
	startOnce  sync.Once
	mu         sync.Mutex
	calls      map[int64]int
}

func (p *generationBarrierProvider) ResolveInstallation(
	ctx context.Context,
	connection *WorkspaceConnection,
	_ ResolveCredentialRequest,
) (*ResolvedCredential, error) {
	p.mu.Lock()
	p.calls[connection.CredentialGeneration]++
	p.mu.Unlock()
	client := p.newClient
	if connection.CredentialGeneration == 1 {
		client = p.oldClient
		p.startOnce.Do(func() { close(p.oldStarted) })
		select {
		case <-p.releaseOld:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return &ResolvedCredential{
		Client:    client,
		Principal: AuthPrincipal{Kind: AuthPrincipalApp, Source: ConnectionSourceGitHubAppInstallation},
	}, nil
}

func (p *expiringInstallationProvider) ResolveInstallation(
	_ context.Context,
	_ *WorkspaceConnection,
	_ ResolveCredentialRequest,
) (*ResolvedCredential, error) {
	p.calls++
	return &ResolvedCredential{
		Client:    NewMockClient(),
		Principal: AuthPrincipal{Kind: AuthPrincipalApp},
		ExpiresAt: p.now().Add(p.lifetime),
	}, nil
}

func (p *repoScopedInstallationProvider) ResolveInstallation(
	_ context.Context,
	_ *WorkspaceConnection,
	req ResolveCredentialRequest,
) (*ResolvedCredential, error) {
	p.calls++
	client := NewMockClient()
	p.clients[req.RepoOwner+"/"+req.RepoName] = client
	return &ResolvedCredential{
		Client:    client,
		Principal: AuthPrincipal{Kind: AuthPrincipalApp, Source: ConnectionSourceGitHubAppInstallation},
	}, nil
}

func (f fakeAuthSecrets) Reveal(_ context.Context, id string) (string, error) {
	value, ok := f[id]
	if !ok {
		return "", errors.New("missing secret")
	}
	return value, nil
}

func TestCredentialResolverIsolatesWorkspacePATs(t *testing.T) {
	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		"personal": {WorkspaceID: "personal", Source: ConnectionSourcePAT, Login: "alice", Status: ConnectionStatusActive, CredentialGeneration: 1},
		"work":     {WorkspaceID: "work", Source: ConnectionSourcePAT, Login: "alice-work", Status: ConnectionStatusActive, CredentialGeneration: 1},
	}}
	resolver := NewCredentialResolver(connections, fakeAuthSecrets{
		WorkspacePATSecretKey("personal"): "pat-personal",
		WorkspacePATSecretKey("work"):     "pat-work",
	})

	personal, err := resolver.Resolve(context.Background(), ResolveCredentialRequest{WorkspaceID: "personal", Purpose: CredentialPurposeAutomation})
	if err != nil {
		t.Fatalf("resolve personal: %v", err)
	}
	work, err := resolver.Resolve(context.Background(), ResolveCredentialRequest{WorkspaceID: "work", Purpose: CredentialPurposeAutomation})
	if err != nil {
		t.Fatalf("resolve work: %v", err)
	}
	if personal.Client == work.Client || personal.RateTracker == work.RateTracker {
		t.Fatal("workspace credentials share client or rate tracker")
	}
	if personal.Principal.Login != "alice" || work.Principal.Login != "alice-work" {
		t.Fatalf("principals = %+v / %+v", personal.Principal, work.Principal)
	}
}

func TestCredentialResolverNeverFallsBackForDisconnectedWorkspace(t *testing.T) {
	resolver := NewCredentialResolver(&fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{}}, nil)
	resolver.SetLegacyFactory(func(context.Context) (Client, string, error) {
		t.Fatal("legacy factory called for disconnected workspace")
		return nil, "", nil
	})
	_, err := resolver.Resolve(context.Background(), ResolveCredentialRequest{WorkspaceID: "new", Purpose: CredentialPurposeAutomation})
	if !errors.Is(err, ErrGitHubNotConfigured) {
		t.Fatalf("error = %v, want ErrGitHubNotConfigured", err)
	}
}

func TestAuthResolverKeysAppCredentialsByRegistration(t *testing.T) {
	installationID := int64(42)
	connection := &WorkspaceConnection{
		WorkspaceID: "work", Source: ConnectionSourceGitHubAppInstallation,
		InstallationID: &installationID, AppRegistrationID: "registration-a",
		Status: ConnectionStatusActive, CredentialGeneration: 1,
	}
	reader := &synchronizedConnectionReader{connection: *connection}
	provider := &registrationInstallationProvider{calls: make(map[string]int)}
	resolver := NewCredentialResolver(reader, nil)
	resolver.SetInstallationProvider(provider)

	first, err := resolver.Resolve(context.Background(), ResolveCredentialRequest{
		WorkspaceID: "work", Purpose: CredentialPurposeAutomation,
	})
	if err != nil {
		t.Fatal(err)
	}
	connection.AppRegistrationID = "registration-b"
	reader.replace(*connection)
	second, err := resolver.Resolve(context.Background(), ResolveCredentialRequest{
		WorkspaceID: "work", Purpose: CredentialPurposeAutomation,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Principal.AppRegistrationID != "registration-a" ||
		second.Principal.AppRegistrationID != "registration-b" {
		t.Fatalf("principals = %+v / %+v", first.Principal, second.Principal)
	}
	if provider.calls["registration-a"] != 1 || provider.calls["registration-b"] != 1 {
		t.Fatalf("provider calls = %#v", provider.calls)
	}
}

func TestAuthResolverAppConnectionRequiresRegistrationID(t *testing.T) {
	installationID := int64(42)
	resolver := NewCredentialResolver(&fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		"work": {
			WorkspaceID: "work", Source: ConnectionSourceGitHubAppInstallation,
			InstallationID: &installationID, Status: ConnectionStatusActive,
			CredentialGeneration: 1,
		},
	}}, nil)
	resolver.SetInstallationProvider(&registrationInstallationProvider{calls: make(map[string]int)})
	_, err := resolver.Resolve(context.Background(), ResolveCredentialRequest{
		WorkspaceID: "work", Purpose: CredentialPurposeAutomation,
	})
	if !errors.Is(err, ErrGitHubNotConfigured) {
		t.Fatalf("error = %v, want ErrGitHubNotConfigured", err)
	}
}

func TestCredentialResolverUsesLegacyOnlyForMigratedSource(t *testing.T) {
	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		"old": {WorkspaceID: "old", Source: ConnectionSourceLegacyShared, Status: ConnectionStatusActive, CredentialGeneration: 1},
	}}
	resolver := NewCredentialResolver(connections, nil)
	legacy := NewMockClient()
	legacy.SetUser("legacy-user")
	resolver.SetLegacyFactory(func(context.Context) (Client, string, error) { return legacy, AuthMethodPAT, nil })

	resolved, err := resolver.Resolve(context.Background(), ResolveCredentialRequest{WorkspaceID: "old", Purpose: CredentialPurposeAutomation})
	if err != nil {
		t.Fatalf("resolve legacy: %v", err)
	}
	if resolved.Principal.Source != ConnectionSourceLegacyShared || resolved.Principal.Login != "legacy-user" {
		t.Fatalf("principal = %+v", resolved.Principal)
	}
}

func TestCredentialResolverNamedCLIUsesSelectedLogin(t *testing.T) {
	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		"work": {WorkspaceID: "work", Source: ConnectionSourceGHCLI, GitHubHost: "github.com", Login: "work-user", Status: ConnectionStatusActive, CredentialGeneration: 2},
	}}
	resolver := NewCredentialResolver(connections, nil)
	var gotHost, gotLogin string
	resolver.ghToken = func(_ context.Context, host, login string) (string, error) {
		gotHost, gotLogin = host, login
		return "cli-token", nil
	}

	resolved, err := resolver.Resolve(context.Background(), ResolveCredentialRequest{WorkspaceID: "work", Purpose: CredentialPurposeAutomation})
	if err != nil {
		t.Fatalf("resolve CLI: %v", err)
	}
	if gotHost != "github.com" || gotLogin != "work-user" {
		t.Fatalf("selected account = %s@%s", gotLogin, gotHost)
	}
	if resolved.Principal.Source != ConnectionSourceGHCLI {
		t.Fatalf("principal = %+v", resolved.Principal)
	}
}

func TestCredentialResolverAppRequiresPersonalIdentityForMyGitHub(t *testing.T) {
	installationID := int64(42)
	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		"company": {WorkspaceID: "company", Source: ConnectionSourceGitHubAppInstallation, InstallationID: &installationID, AppRegistrationID: "registration-test", Status: ConnectionStatusActive, CredentialGeneration: 1},
	}}
	resolver := NewCredentialResolver(connections, nil)
	_, err := resolver.Resolve(context.Background(), ResolveCredentialRequest{
		WorkspaceID: "company", UserID: "default-user", Purpose: CredentialPurposePersonalRead,
	})
	if !errors.Is(err, ErrGitHubPersonalRequired) {
		t.Fatalf("error = %v, want ErrGitHubPersonalRequired", err)
	}
}

func TestCredentialResolverInvalidPersonalFallsBackToAppOnlyForManualWrites(t *testing.T) {
	installationID := int64(42)
	connections := &fakeConnectionReader{
		workspaces: map[string]*WorkspaceConnection{
			"company": {
				WorkspaceID: "company", Source: ConnectionSourceGitHubAppInstallation,
				InstallationID: &installationID, AppRegistrationID: "registration-test",
				Status: ConnectionStatusActive, CredentialGeneration: 1,
			},
		},
		users: map[string]*UserConnection{
			"company:default-user": {
				WorkspaceID: "company", UserID: "default-user", Login: "octocat",
				Status: ConnectionStatusRevoked, CredentialGeneration: 2,
			},
		},
	}
	provider := &repoScopedInstallationProvider{clients: make(map[string]Client)}
	resolver := NewCredentialResolver(connections, nil)
	resolver.SetInstallationProvider(provider)
	resolver.SetUserProvider(fixedUserCredentialProvider{client: NewMockClient()})

	resolved, err := resolver.Resolve(context.Background(), ResolveCredentialRequest{
		WorkspaceID: "company", UserID: "default-user", Purpose: CredentialPurposePersonalWrite,
	})
	if err != nil {
		t.Fatalf("manual write resolution: %v", err)
	}
	if resolved.Principal.Kind != AuthPrincipalApp || resolved.Principal.Source != ConnectionSourceGitHubAppInstallation {
		t.Fatalf("manual write principal = %+v, want App installation", resolved.Principal)
	}

	_, err = resolver.Resolve(context.Background(), ResolveCredentialRequest{
		WorkspaceID: "company", UserID: "default-user", Purpose: CredentialPurposePersonalRead,
	})
	if !errors.Is(err, ErrGitHubConnectionInvalid) {
		t.Fatalf("personal read error = %v, want reconnect-required invalid connection", err)
	}
}

func TestCredentialResolverGenerationSeparatesCachedClients(t *testing.T) {
	connection := &WorkspaceConnection{WorkspaceID: "work", Source: ConnectionSourcePAT, Login: "first", Status: ConnectionStatusActive, CredentialGeneration: 1}
	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{"work": connection}}
	resolver := NewCredentialResolver(connections, fakeAuthSecrets{WorkspacePATSecretKey("work"): "pat"})
	first, err := resolver.Resolve(context.Background(), ResolveCredentialRequest{WorkspaceID: "work", Purpose: CredentialPurposeAutomation})
	if err != nil {
		t.Fatal(err)
	}
	connection.CredentialGeneration = 2
	connection.Login = "second"
	second, err := resolver.Resolve(context.Background(), ResolveCredentialRequest{WorkspaceID: "work", Purpose: CredentialPurposeAutomation})
	if err != nil {
		t.Fatal(err)
	}
	if first.Client == second.Client || second.Principal.Login != "second" {
		t.Fatalf("generation did not replace client: first=%p second=%p principal=%+v", first.Client, second.Client, second.Principal)
	}
}

func TestCredentialResolverSeparatesRepositoryScopedAppTokens(t *testing.T) {
	installationID := int64(42)
	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		"company": {
			WorkspaceID: "company", Source: ConnectionSourceGitHubAppInstallation,
			InstallationID: &installationID, AppRegistrationID: "registration-test",
			Status: ConnectionStatusActive, CredentialGeneration: 1,
		},
	}}
	provider := &repoScopedInstallationProvider{clients: make(map[string]Client)}
	resolver := NewCredentialResolver(connections, nil)
	resolver.SetInstallationProvider(provider)
	first, err := resolver.Resolve(context.Background(), ResolveCredentialRequest{
		WorkspaceID: "company", Purpose: CredentialPurposeGitTransport,
		RepoOwner: "acme", RepoName: "frontend",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := resolver.Resolve(context.Background(), ResolveCredentialRequest{
		WorkspaceID: "company", Purpose: CredentialPurposeGitTransport,
		RepoOwner: "acme", RepoName: "backend",
	})
	if err != nil {
		t.Fatal(err)
	}
	again, err := resolver.Resolve(context.Background(), ResolveCredentialRequest{
		WorkspaceID: "company", Purpose: CredentialPurposeGitTransport,
		RepoOwner: "acme", RepoName: "frontend",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Client == second.Client || again.Client != first.Client || provider.calls != 2 {
		t.Fatalf("clients = %p/%p/%p, provider calls = %d", first.Client, second.Client, again.Client, provider.calls)
	}
}

func TestCredentialResolverRefreshesCachedCredentialBeforeExpiry(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	installationID := int64(42)
	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		"company": {
			WorkspaceID: "company", Source: ConnectionSourceGitHubAppInstallation,
			InstallationID: &installationID, AppRegistrationID: "registration-test",
			Status: ConnectionStatusActive, CredentialGeneration: 1,
		},
	}}
	provider := &expiringInstallationProvider{now: func() time.Time { return now }, lifetime: 10 * time.Minute}
	resolver := NewCredentialResolver(connections, nil)
	resolver.now = func() time.Time { return now }
	resolver.SetInstallationProvider(provider)
	request := ResolveCredentialRequest{
		WorkspaceID: "company", Purpose: CredentialPurposeAutomation,
		RepoOwner: "acme", RepoName: "backend",
	}

	first, err := resolver.Resolve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(6 * time.Minute)
	second, err := resolver.Resolve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Client == second.Client || provider.calls != 2 {
		t.Fatalf("clients = %p/%p, provider calls = %d", first.Client, second.Client, provider.calls)
	}
}

func TestCredentialResolverRejectsNewlyExpiredCredential(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	installationID := int64(42)
	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		"company": {
			WorkspaceID: "company", Source: ConnectionSourceGitHubAppInstallation,
			InstallationID: &installationID, AppRegistrationID: "registration-test",
			Status: ConnectionStatusActive, CredentialGeneration: 1,
		},
	}}
	provider := &expiringInstallationProvider{now: func() time.Time { return now }, lifetime: -time.Minute}
	resolver := NewCredentialResolver(connections, nil)
	resolver.now = func() time.Time { return now }
	resolver.SetInstallationProvider(provider)

	resolved, err := resolver.Resolve(context.Background(), ResolveCredentialRequest{
		WorkspaceID: "company", Purpose: CredentialPurposeAutomation,
	})
	if resolved != nil || !errors.Is(err, ErrInstallationTokenExpired) {
		t.Fatalf("Resolve() = %+v, %v; want nil, ErrInstallationTokenExpired", resolved, err)
	}
}

func TestCredentialResolverInvalidationRejectsInflightOldGeneration(t *testing.T) {
	for _, test := range []struct {
		name            string
		resolveNewFirst bool
	}{
		{name: "replacement resolution finishes before old resolution", resolveNewFirst: true},
		{name: "old resolution finishes before replacement caller", resolveNewFirst: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			installationID := int64(42)
			oldConnection := WorkspaceConnection{
				WorkspaceID: "company", Source: ConnectionSourceGitHubAppInstallation,
				InstallationID: &installationID, AppRegistrationID: "registration-test",
				Status: ConnectionStatusActive, CredentialGeneration: 1,
			}
			newConnection := oldConnection
			newConnection.CredentialGeneration = 2
			connections := &synchronizedConnectionReader{connection: oldConnection}
			provider := &generationBarrierProvider{
				oldStarted: make(chan struct{}), releaseOld: make(chan struct{}),
				oldClient: NewMockClient(), newClient: NewMockClient(), calls: make(map[int64]int),
			}
			resolver := NewCredentialResolver(connections, nil)
			resolver.SetInstallationProvider(provider)
			request := ResolveCredentialRequest{WorkspaceID: "company", Purpose: CredentialPurposeAutomation}
			oldResult := make(chan *ResolvedCredential, 1)
			oldErr := make(chan error, 1)
			go func() {
				resolved, err := resolver.Resolve(context.Background(), request)
				oldResult <- resolved
				oldErr <- err
			}()
			<-provider.oldStarted

			connections.replace(newConnection)
			resolver.InvalidateWorkspace("company")
			var replacement *ResolvedCredential
			var err error
			if test.resolveNewFirst {
				replacement, err = resolver.Resolve(context.Background(), request)
				if err != nil {
					t.Fatal(err)
				}
				close(provider.releaseOld)
			} else {
				close(provider.releaseOld)
				replacement = <-oldResult
				if err = <-oldErr; err != nil {
					t.Fatal(err)
				}
			}

			var old *ResolvedCredential
			if test.resolveNewFirst {
				old = <-oldResult
				if err = <-oldErr; err != nil {
					t.Fatal(err)
				}
			} else {
				old = replacement
				replacement, err = resolver.Resolve(context.Background(), request)
				if err != nil {
					t.Fatal(err)
				}
			}
			if old == nil || replacement == nil || old.Client != provider.newClient ||
				replacement.Client != provider.newClient {
				t.Fatalf("old result = %+v, replacement = %+v", old, replacement)
			}
			provider.mu.Lock()
			generationOneCalls := provider.calls[1]
			provider.mu.Unlock()
			if generationOneCalls != 1 {
				t.Fatalf("generation 1 provider calls = %d, want 1", generationOneCalls)
			}
		})
	}
}
