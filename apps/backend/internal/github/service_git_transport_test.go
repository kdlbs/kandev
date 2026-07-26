package github

import (
	"context"
	"errors"
	"testing"
)

func TestResolveGitCredentialUsesWorkspacePAT(t *testing.T) {
	t.Parallel()

	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		"workspace-a": {
			WorkspaceID: "workspace-a", Source: ConnectionSourcePAT,
			Status: ConnectionStatusActive, CredentialGeneration: 1,
		},
	}}
	resolver := NewCredentialResolver(connections, fakeAuthSecrets{
		WorkspacePATSecretKey("workspace-a"): "workspace-a-token",
	})
	service := &Service{resolver: resolver}

	username, password, err := service.ResolveGitCredential(
		context.Background(), "workspace-a", "github", "acme", "private",
	)
	if err != nil {
		t.Fatalf("ResolveGitCredential(): %v", err)
	}
	if username != "x-access-token" || password != "workspace-a-token" {
		t.Fatalf("credential = %q/%q, want x-access-token/workspace-a-token", username, password)
	}
}

func TestResolveGitCredentialUsesSelectedGHCLIAccount(t *testing.T) {
	t.Parallel()

	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		"workspace-a": {
			WorkspaceID: "workspace-a", Source: ConnectionSourceGHCLI,
			GitHubHost: "github.com", Login: "automation-user",
			Status: ConnectionStatusActive, CredentialGeneration: 1,
		},
	}}
	resolver := NewCredentialResolver(connections, nil)
	resolver.ghToken = func(_ context.Context, host, login string) (string, error) {
		if host != "github.com" || login != "automation-user" {
			t.Fatalf("selected account = %s@%s", login, host)
		}
		return "selected-cli-token", nil
	}
	service := &Service{resolver: resolver}

	_, password, err := service.ResolveGitCredential(
		context.Background(), "workspace-a", "github", "acme", "private",
	)
	if err != nil {
		t.Fatalf("ResolveGitCredential(): %v", err)
	}
	if password != "selected-cli-token" {
		t.Fatalf("password = %q, want selected CLI credential", password)
	}
}

func TestResolveGitCredentialUsesMigratedLegacyCredential(t *testing.T) {
	t.Parallel()

	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		"workspace-a": {
			WorkspaceID: "workspace-a", Source: ConnectionSourceLegacyShared,
			Status: ConnectionStatusActive, CredentialGeneration: 7,
		},
	}}
	resolver := NewCredentialResolver(connections, nil)
	legacy := NewMockClient()
	legacy.SetUser("legacy-user")
	resolver.SetLegacyTransportFactory(func(context.Context) (Client, string, string, error) {
		return legacy, AuthMethodPAT, "legacy-transport-token", nil
	})
	service := &Service{resolver: resolver}

	username, password, err := service.ResolveGitCredential(
		context.Background(), "workspace-a", "github", "acme", "private",
	)
	if err != nil {
		t.Fatalf("ResolveGitCredential(): %v", err)
	}
	if username != "x-access-token" || password != "legacy-transport-token" {
		t.Fatalf("credential = %q/%q, want x-access-token/legacy-transport-token", username, password)
	}
}

func TestResolveGitCredentialRejectsMissingMigratedLegacyCredential(t *testing.T) {
	t.Parallel()

	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		"workspace-a": {
			WorkspaceID: "workspace-a", Source: ConnectionSourceLegacyShared,
			Status: ConnectionStatusActive, CredentialGeneration: 7,
		},
	}}
	resolver := NewCredentialResolver(connections, nil)
	resolver.SetLegacyTransportFactory(func(context.Context) (Client, string, string, error) {
		return &NoopClient{}, AuthMethodNone, "", nil
	})
	service := &Service{resolver: resolver}

	_, _, err := service.ResolveGitCredential(
		context.Background(), "workspace-a", "github", "acme", "private",
	)
	if !errors.Is(err, ErrGitHubNotConfigured) {
		t.Fatalf("ResolveGitCredential() error = %v, want ErrGitHubNotConfigured", err)
	}
}

func TestResolveGitCredentialNeverUsesPersonalConnection(t *testing.T) {
	t.Parallel()

	connections := &fakeConnectionReader{
		workspaces: map[string]*WorkspaceConnection{
			"workspace-a": {
				WorkspaceID: "workspace-a", Source: ConnectionSourceLegacyShared,
				Status: ConnectionStatusActive, CredentialGeneration: 7,
			},
		},
		users: map[string]*UserConnection{
			"workspace-a:user-a": {
				WorkspaceID: "workspace-a", UserID: "user-a",
				Status: ConnectionStatusActive,
			},
		},
	}
	resolver := NewCredentialResolver(connections, nil)
	resolver.SetUserProvider(failingUserCredentialProvider{t: t})
	legacy := NewMockClient()
	legacy.SetUser("legacy-user")
	resolver.SetLegacyTransportFactory(func(context.Context) (Client, string, string, error) {
		return legacy, AuthMethodPAT, "automation-token", nil
	})
	service := &Service{resolver: resolver}

	_, password, err := service.ResolveGitCredential(
		context.Background(), "workspace-a", "github", "acme", "private",
	)
	if err != nil {
		t.Fatalf("ResolveGitCredential(): %v", err)
	}
	if password != "automation-token" {
		t.Fatalf("password = %q, want automation credential", password)
	}
}

func TestResolveGitCredentialRejectsAppWithoutGitRead(t *testing.T) {
	t.Parallel()

	installationID := int64(42)
	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		"workspace-a": {
			WorkspaceID: "workspace-a", Source: ConnectionSourceGitHubAppInstallation,
			InstallationID: &installationID, AppRegistrationID: "registration-test",
			Status: ConnectionStatusActive, CredentialGeneration: 1,
		},
	}}
	resolver := NewCredentialResolver(connections, nil)
	resolver.SetAutomationProvider(staticTransportCredentialProvider{resolved: &ResolvedCredential{
		Client:     NewMockClient(),
		credential: "app-token",
		Principal: AuthPrincipal{
			Kind: AuthPrincipalApp, Source: ConnectionSourceGitHubAppInstallation,
			AppRegistrationID: "registration-test",
		},
		AppRegistrationID: "registration-test",
		Capabilities:      map[GitHubAppCapability]bool{},
	}})
	service := &Service{resolver: resolver}

	_, _, err := service.ResolveGitCredential(
		context.Background(), "workspace-a", "github", "acme", "private",
	)
	if !errors.Is(err, ErrGitHubCapabilityDenied) {
		t.Fatalf("ResolveGitCredential() error = %v, want capability denied", err)
	}
}

func TestResolveGitCredentialUsesAppWithGitRead(t *testing.T) {
	t.Parallel()

	installationID := int64(42)
	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		"workspace-a": {
			WorkspaceID: "workspace-a", Source: ConnectionSourceGitHubAppInstallation,
			InstallationID: &installationID, AppRegistrationID: "registration-test",
			Status: ConnectionStatusActive, CredentialGeneration: 1,
		},
	}}
	resolver := NewCredentialResolver(connections, nil)
	resolver.SetAutomationProvider(staticTransportCredentialProvider{resolved: &ResolvedCredential{
		Client:     NewMockClient(),
		credential: "app-token",
		Principal: AuthPrincipal{
			Kind: AuthPrincipalApp, Source: ConnectionSourceGitHubAppInstallation,
			AppRegistrationID: "registration-test",
		},
		AppRegistrationID: "registration-test",
		Capabilities: map[GitHubAppCapability]bool{
			CapabilityGitRead: true,
		},
	}})
	service := &Service{resolver: resolver}

	_, password, err := service.ResolveGitCredential(
		context.Background(), "workspace-a", "github", "acme", "private",
	)
	if err != nil {
		t.Fatalf("ResolveGitCredential(): %v", err)
	}
	if password != "app-token" {
		t.Fatalf("password = %q, want app token", password)
	}
}

type failingUserCredentialProvider struct {
	t *testing.T
}

func (p failingUserCredentialProvider) ResolveUser(
	context.Context,
	*UserConnection,
	ResolveCredentialRequest,
) (*ResolvedCredential, error) {
	p.t.Fatal("personal credential provider called for Git transport")
	return nil, nil
}

type staticTransportCredentialProvider struct {
	resolved *ResolvedCredential
}

func (p staticTransportCredentialProvider) ResolveAutomation(
	context.Context,
	*WorkspaceConnection,
	ResolveCredentialRequest,
) (*ResolvedCredential, error) {
	return p.resolved, nil
}
