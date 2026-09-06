package github

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type fixedInstallationMinter struct {
	token        InstallationToken
	repositories *[]string
	permissions  *InstallationPermissions
}

func (m fixedInstallationMinter) MintInstallationToken(
	_ context.Context,
	_ int64,
	permissions InstallationPermissions,
	repositories []string,
) (InstallationToken, error) {
	if m.repositories != nil {
		*m.repositories = append([]string(nil), repositories...)
	}
	if m.permissions != nil {
		*m.permissions = clonePermissions(permissions)
	}
	return m.token, nil
}

func TestCachedInstallationCredentialProviderNarrowsScopedActionsPermissions(t *testing.T) {
	installationID := int64(42)
	var permissions InstallationPermissions
	provider := NewCachedInstallationCredentialProvider(NewInstallationTokenCache(fixedInstallationMinter{
		permissions: &permissions,
		token: InstallationToken{
			Token: "installation-token", ExpiresAt: time.Now().Add(time.Hour),
			Permissions: InstallationPermissions{
				"actions": PermissionWrite, "contents": PermissionRead,
				"metadata": PermissionRead, "pull_requests": PermissionRead,
			},
		},
	}))

	_, err := provider.ResolveInstallation(context.Background(), &WorkspaceConnection{
		WorkspaceID: "workspace-1", Source: ConnectionSourceGitHubAppInstallation,
		InstallationID: &installationID,
	}, ResolveCredentialRequest{
		WorkspaceID: "workspace-1", Purpose: CredentialPurposeScopedActionsWrite,
		RepoOwner: "kdlbs", RepoName: "kandev",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := InstallationPermissions{
		"actions": PermissionWrite, "contents": PermissionRead,
		"metadata": PermissionRead, "pull_requests": PermissionRead,
	}
	if !reflect.DeepEqual(permissions, want) {
		t.Fatalf("mint permissions = %#v, want %#v", permissions, want)
	}
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

func TestCachedInstallationCredentialProviderRejectsUnscopedActionsRequests(t *testing.T) {
	installationID := int64(42)
	for _, purpose := range []CredentialPurpose{
		CredentialPurposeScopedActionsWrite, CredentialPurposeScopedActionsRerun,
		CredentialPurposeScopedActionsDispatch,
	} {
		for _, request := range []ResolveCredentialRequest{
			{Purpose: purpose, RepoOwner: "", RepoName: "kandev"},
			{Purpose: purpose, RepoOwner: "kdlbs", RepoName: ""},
		} {
			provider := NewCachedInstallationCredentialProvider(NewInstallationTokenCache(fixedInstallationMinter{}))
			_, err := provider.ResolveInstallation(context.Background(), &WorkspaceConnection{
				WorkspaceID: "workspace-1", InstallationID: &installationID,
			}, ResolveCredentialRequest{WorkspaceID: "workspace-1", Purpose: request.Purpose,
				RepoOwner: request.RepoOwner, RepoName: request.RepoName})
			if err == nil {
				t.Fatalf("purpose %s request %+v was accepted", purpose, request)
			}
		}
	}
}

func TestCachedInstallationCredentialProviderRejectsDifferentRegistration(t *testing.T) {
	installationID := int64(42)
	cache := NewAppInstallationTokenCache("registration-a", 5, fixedInstallationMinter{})
	provider := NewCachedInstallationCredentialProvider(cache)
	_, err := provider.ResolveInstallation(context.Background(), &WorkspaceConnection{
		Source: ConnectionSourceGitHubAppInstallation, InstallationID: &installationID,
		AppRegistrationID: "registration-b",
	}, ResolveCredentialRequest{})
	if !errors.Is(err, ErrGitHubNotConfigured) {
		t.Fatalf("error = %v, want ErrGitHubNotConfigured", err)
	}
}
