package github

import (
	"context"
	"testing"
	"time"
)

type fixedInstallationMinter struct {
	token        InstallationToken
	repositories *[]string
}

func (m fixedInstallationMinter) MintInstallationToken(
	_ context.Context,
	_ int64,
	_ InstallationPermissions,
	repositories []string,
) (InstallationToken, error) {
	if m.repositories != nil {
		*m.repositories = append([]string(nil), repositories...)
	}
	return m.token, nil
}

func TestCachedInstallationCredentialProviderPreservesActorCapabilitiesAndExpiry(t *testing.T) {
	installationID := int64(42)
	expiresAt := time.Now().Add(time.Hour)
	var repositories []string
	provider := NewCachedInstallationCredentialProvider(NewInstallationTokenCache(fixedInstallationMinter{
		repositories: &repositories,
		token: InstallationToken{
			Token:     "installation-token",
			ExpiresAt: expiresAt,
			Permissions: InstallationPermissions{
				"contents":      PermissionWrite,
				"pull_requests": PermissionRead,
			},
			Principal: TokenPrincipal{
				Kind:           TokenCredentialInstallation,
				PrincipalID:    "installation:42",
				InstallationID: installationID,
			},
		},
	}))

	resolved, err := provider.ResolveInstallation(context.Background(), &WorkspaceConnection{
		Source:                   ConnectionSourceGitHubAppInstallation,
		InstallationID:           &installationID,
		InstallationAccountLogin: "acme",
	}, ResolveCredentialRequest{
		WorkspaceID: "workspace-1", Purpose: CredentialPurposeAutomation, RepoName: "widgets",
	})
	if err != nil {
		t.Fatalf("ResolveInstallation: %v", err)
	}
	if resolved.Principal.Kind != AuthPrincipalApp || resolved.Principal.Login != "acme" {
		t.Fatalf("principal = %+v", resolved.Principal)
	}
	if !resolved.Capabilities[CapabilityGitWrite] || resolved.Capabilities[CapabilityPullRequestWrite] {
		t.Fatalf("capabilities = %#v", resolved.Capabilities)
	}
	if !resolved.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expiry = %v, want %v", resolved.ExpiresAt, expiresAt)
	}
	if len(repositories) != 1 || repositories[0] != "widgets" {
		t.Fatalf("mint repositories = %v", repositories)
	}
	client, ok := resolved.Client.(*TokenClient)
	if !ok || client.Principal().InstallationID != installationID {
		t.Fatalf("client principal = %+v", client)
	}
}

func TestCachedInstallationCredentialProviderRequiresInstallationID(t *testing.T) {
	provider := NewCachedInstallationCredentialProvider(NewInstallationTokenCache(fixedInstallationMinter{}))
	if _, err := provider.ResolveInstallation(
		context.Background(),
		&WorkspaceConnection{},
		ResolveCredentialRequest{},
	); err == nil {
		t.Fatal("expected missing installation ID error")
	}
}
