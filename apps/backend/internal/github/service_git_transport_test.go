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

func TestResolveGitCredentialRejectsAppWithoutGitRead(t *testing.T) {
	t.Parallel()

	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		"workspace-a": {
			WorkspaceID: "workspace-a", Source: ConnectionSourceGitHubAppInstallation,
			Status: ConnectionStatusActive, CredentialGeneration: 1,
		},
	}}
	resolver := NewCredentialResolver(connections, nil)
	resolver.SetAutomationProvider(staticTransportCredentialProvider{resolved: &ResolvedCredential{
		Client:       NewMockClient(),
		credential:   "app-token",
		Principal:    AuthPrincipal{Kind: AuthPrincipalApp, Source: ConnectionSourceGitHubAppInstallation},
		Capabilities: map[GitHubAppCapability]bool{},
	}})
	service := &Service{resolver: resolver}

	_, _, err := service.ResolveGitCredential(
		context.Background(), "workspace-a", "github", "acme", "private",
	)
	if !errors.Is(err, ErrGitHubCapabilityDenied) {
		t.Fatalf("ResolveGitCredential() error = %v, want capability denied", err)
	}
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
