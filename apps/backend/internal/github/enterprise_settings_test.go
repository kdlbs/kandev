package github

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/common/githubhost"
	"testing"
)

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.3
func TestEnterpriseStaleSettingsDraftCannotWidenScope(t *testing.T) {
	service, _ := newWorkspaceConnectionService(t, "alice")
	ctx := context.Background()
	if _, err := service.SetWorkspaceConnection(ctx, "ws-1", SetWorkspaceConnectionRequest{Source: ConnectionSourcePAT, Token: "public"}); err != nil {
		t.Fatal(err)
	}
	service.hostTokenClientFactory = func(string, githubhost.Host) Client {
		client := NewMockClient()
		client.SetUser("alice")
		return client
	}
	if _, err := service.SetWorkspaceConnection(ctx, "ws-1", SetWorkspaceConnectionRequest{Source: ConnectionSourcePAT, Host: "git.example.com", Token: "enterprise"}); err != nil {
		t.Fatal(err)
	}
	all := RepoScopeModeAll
	_, err := service.UpdateWorkspaceSettings(withExpectedGitHubHost(ctx, "github.com"), &UpdateWorkspaceSettingsRequest{WorkspaceID: "ws-1", RepoScopeMode: &all})
	if !errors.Is(err, ErrGitHubHostMismatch) {
		t.Fatalf("stale settings draft accepted: %v", err)
	}
	settings, err := service.GetWorkspaceSettings(ctx, "ws-1")
	if err != nil || settings.RepoScopeMode != RepoScopeModeRepos {
		t.Fatalf("replacement scope widened %+v, error %v", settings, err)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.3
func TestEnterpriseStaleActionPresetDraftRejected(t *testing.T) {
	service, _ := newWorkspaceConnectionService(t, "alice")
	if err := service.store.UpsertWorkspaceConnection(context.Background(), &WorkspaceConnection{WorkspaceID: "ws-1", Source: ConnectionSourcePAT, GitHubHost: "git.example.com", Login: "alice", Status: ConnectionStatusActive}); err != nil {
		t.Fatal(err)
	}
	_, err := service.UpdateActionPresets(withExpectedGitHubHost(context.Background(), "github.com"), &UpdateActionPresetsRequest{WorkspaceID: "ws-1"})
	if !errors.Is(err, ErrGitHubHostMismatch) {
		t.Fatalf("stale preset mutation accepted: %v", err)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.3
func TestEnterpriseFirstConnectionPersistsSettingsHost(t *testing.T) {
	service, _ := newWorkspaceConnectionService(t, "alice")
	service.hostTokenClientFactory = func(string, githubhost.Host) Client {
		client := NewMockClient()
		client.SetUser("alice")
		return client
	}
	_, err := service.SetWorkspaceConnection(context.Background(), "ws-1", SetWorkspaceConnectionRequest{Source: ConnectionSourcePAT, Host: "git.example.com", Token: "enterprise"})
	if err != nil {
		t.Fatal(err)
	}
	settings, err := service.GetWorkspaceSettings(context.Background(), "ws-1")
	if err != nil || settings.GitHubHost != "git.example.com" {
		t.Fatalf("new Enterprise host scope = %+v, error %v", settings, err)
	}
}
