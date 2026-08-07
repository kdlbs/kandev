package github

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
)

func TestInitializeWorkspaceDefaultsPersistsExecutorAndBindsActiveCLI(t *testing.T) {
	service, secrets := newWorkspaceConnectionService(t, "bob-cli")
	service.ghAccountLister = func(context.Context) ([]GHAccount, error) {
		return []GHAccount{
			{Host: "github.com", Login: "bob-cli", Active: true, State: "active"},
		}, nil
	}
	service.resolver.ghToken = func(context.Context, string, string) (string, error) {
		return "mock-gh-token", nil
	}

	if err := service.InitializeWorkspaceDefaults(context.Background(), "ws-1"); err != nil {
		t.Fatalf("InitializeWorkspaceDefaults: %v", err)
	}
	settings, err := service.store.GetWorkspaceSettings(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspaceSettings: %v", err)
	}
	if settings.TaskGitCredentialsMode != TaskGitCredentialsModeExecutor {
		t.Fatalf("task Git mode = %q, want executor", settings.TaskGitCredentialsMode)
	}
	connection, err := service.store.GetWorkspaceConnection(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspaceConnection: %v", err)
	}
	if connection == nil || connection.Source != ConnectionSourceGHCLI || connection.GitHubHost != "github.com" || connection.Login != "bob-cli" {
		t.Fatalf("connection = %+v, want exact active CLI account", connection)
	}
	if len(secrets.values) != 0 {
		t.Fatalf("workspace CLI token was persisted in secrets: %#v", secrets.values)
	}
}

func TestInitializeWorkspaceDefaultsBoundsCLIValidationContext(t *testing.T) {
	service, _ := newWorkspaceConnectionService(t, "operator")
	service.ghAccountLister = func(context.Context) ([]GHAccount, error) {
		return []GHAccount{{Host: "github.com", Login: "operator", Active: true, State: "active"}}, nil
	}
	var hasDeadline bool
	service.resolver.ghToken = func(ctx context.Context, _, _ string) (string, error) {
		_, hasDeadline = ctx.Deadline()
		return "mock-gh-token", nil
	}

	if err := service.InitializeWorkspaceDefaults(context.Background(), "ws-1"); err != nil {
		t.Fatalf("InitializeWorkspaceDefaults: %v", err)
	}
	if !hasDeadline {
		t.Fatal("CLI validation context has no deadline")
	}
}

func TestInitializeWorkspaceDefaultsDoesNotBindOperatorCLIForMember(t *testing.T) {
	service, _ := newWorkspaceConnectionService(t, "operator")
	listCalls := 0
	service.ghAccountLister = func(context.Context) ([]GHAccount, error) {
		listCalls++
		return []GHAccount{{Host: "github.com", Login: "operator", Active: true, State: "active"}}, nil
	}

	memberCtx := authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "member-1",
		Role:   authn.RoleMember,
	})
	if err := service.InitializeWorkspaceDefaults(memberCtx, "ws-1"); err != nil {
		t.Fatalf("InitializeWorkspaceDefaults(member): %v", err)
	}
	settings, err := service.store.GetWorkspaceSettings(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspaceSettings: %v", err)
	}
	if settings.TaskGitCredentialsMode != TaskGitCredentialsModeExecutor {
		t.Fatalf("task Git mode = %q, want executor", settings.TaskGitCredentialsMode)
	}
	connection, err := service.store.GetWorkspaceConnection(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspaceConnection: %v", err)
	}
	if connection != nil {
		t.Fatalf("member-created workspace inherited operator connection: %+v", connection)
	}
	if listCalls != 0 {
		t.Fatalf("CLI accounts listed %d times for member-created workspace, want 0", listCalls)
	}
}

func TestInitializeWorkspaceDefaultsSoftFailsWhenCLIUnavailable(t *testing.T) {
	service, _ := newWorkspaceConnectionService(t, "operator")
	service.ghAccountLister = func(context.Context) ([]GHAccount, error) {
		return nil, errors.New("gh is not authenticated")
	}

	if err := service.InitializeWorkspaceDefaults(context.Background(), "ws-1"); err != nil {
		t.Fatalf("InitializeWorkspaceDefaults: %v", err)
	}
	settings, err := service.store.GetWorkspaceSettings(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspaceSettings: %v", err)
	}
	if settings.TaskGitCredentialsMode != TaskGitCredentialsModeExecutor {
		t.Fatalf("task Git mode = %q, want executor", settings.TaskGitCredentialsMode)
	}
	connection, err := service.store.GetWorkspaceConnection(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspaceConnection: %v", err)
	}
	if connection != nil {
		t.Fatalf("unavailable CLI created a connection: %+v", connection)
	}
}

func TestInitializeWorkspaceDefaultsIgnoresInvalidActiveCLI(t *testing.T) {
	service, _ := newWorkspaceConnectionService(t, "operator")
	service.ghAccountLister = func(context.Context) ([]GHAccount, error) {
		return []GHAccount{{Host: "github.com", Login: "operator", Active: true, State: "unexpected"}}, nil
	}
	service.resolver.ghToken = func(context.Context, string, string) (string, error) {
		return "mock-gh-token", nil
	}

	if err := service.InitializeWorkspaceDefaults(context.Background(), "ws-1"); err != nil {
		t.Fatalf("InitializeWorkspaceDefaults: %v", err)
	}
	connection, err := service.store.GetWorkspaceConnection(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspaceConnection: %v", err)
	}
	if connection != nil {
		t.Fatalf("invalid active CLI account created a connection: %+v", connection)
	}
}

func TestInitializeWorkspaceDefaultsPreservesExistingWorkspaceState(t *testing.T) {
	service, _ := newWorkspaceConnectionService(t, "operator")
	ctx := context.Background()
	if err := service.store.UpsertWorkspaceSettings(ctx, &WorkspaceSettings{
		WorkspaceID:            "ws-1",
		TaskGitCredentialsMode: TaskGitCredentialsModeManaged,
		RepoScopeMode:          RepoScopeModeRepos,
	}); err != nil {
		t.Fatalf("UpsertWorkspaceSettings: %v", err)
	}
	if err := service.store.UpsertWorkspaceConnection(ctx, &WorkspaceConnection{
		WorkspaceID:          "ws-1",
		Source:               ConnectionSourcePAT,
		GitHubHost:           defaultGitHubHost,
		Login:                "existing-user",
		Status:               ConnectionStatusActive,
		CredentialGeneration: 1,
	}); err != nil {
		t.Fatalf("UpsertWorkspaceConnection: %v", err)
	}
	service.ghAccountLister = func(context.Context) ([]GHAccount, error) {
		return []GHAccount{{Host: "github.com", Login: "operator", Active: true, State: "active"}}, nil
	}
	service.resolver.ghToken = func(context.Context, string, string) (string, error) {
		return "mock-gh-token", nil
	}

	if err := service.InitializeWorkspaceDefaults(ctx, "ws-1"); err != nil {
		t.Fatalf("InitializeWorkspaceDefaults: %v", err)
	}
	settings, err := service.store.GetWorkspaceSettings(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspaceSettings: %v", err)
	}
	if settings.TaskGitCredentialsMode != TaskGitCredentialsModeManaged || settings.RepoScopeMode != RepoScopeModeRepos {
		t.Fatalf("existing settings changed: %+v", settings)
	}
	connection, err := service.store.GetWorkspaceConnection(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspaceConnection: %v", err)
	}
	if connection == nil || connection.Source != ConnectionSourcePAT || connection.Login != "existing-user" {
		t.Fatalf("existing connection changed: %+v", connection)
	}
}

func TestInitializeFreshWorkspaceDefaultsSeedsExistingInitialWorkspace(t *testing.T) {
	service, secrets := newWorkspaceConnectionService(t, "operator")
	service.ghAccountLister = func(context.Context) ([]GHAccount, error) {
		return []GHAccount{{Host: "github.com", Login: "operator", Active: true, State: "active"}}, nil
	}
	service.resolver.ghToken = func(context.Context, string, string) (string, error) {
		return "mock-gh-token", nil
	}
	if err := service.InitializeFreshWorkspaceDefaults(context.Background()); err != nil {
		t.Fatalf("InitializeFreshWorkspaceDefaults: %v", err)
	}
	settings, err := service.store.GetWorkspaceSettings(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspaceSettings: %v", err)
	}
	if settings.TaskGitCredentialsMode != TaskGitCredentialsModeExecutor {
		t.Fatalf("task Git mode = %q, want executor", settings.TaskGitCredentialsMode)
	}
	connection, err := service.store.GetWorkspaceConnection(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspaceConnection: %v", err)
	}
	if connection == nil || connection.Source != ConnectionSourceGHCLI || connection.Login != "operator" {
		t.Fatalf("connection = %+v, want initial active CLI account", connection)
	}
	if len(secrets.values) != 0 {
		t.Fatalf("initial workspace CLI token was persisted: %#v", secrets.values)
	}
}

func TestInitializeFreshWorkspaceDefaultsBoundsStartupContext(t *testing.T) {
	service, _ := newWorkspaceConnectionService(t, "operator")
	service.ghAccountLister = func(ctx context.Context) ([]GHAccount, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("fresh workspace defaults context has no deadline")
		}
		return nil, errors.New("gh is not authenticated")
	}

	if err := service.InitializeFreshWorkspaceDefaults(context.Background()); err != nil {
		t.Fatalf("InitializeFreshWorkspaceDefaults: %v", err)
	}
}

func TestInitializeFreshWorkspaceDefaultsSkipsExistingGitHubStore(t *testing.T) {
	service, _ := newWorkspaceConnectionService(t, "operator")
	service.store.freshInstall = false
	service.ghAccountLister = func(context.Context) ([]GHAccount, error) {
		return []GHAccount{{Host: "github.com", Login: "operator", Active: true, State: "active"}}, nil
	}

	if err := service.InitializeFreshWorkspaceDefaults(context.Background()); err != nil {
		t.Fatalf("InitializeFreshWorkspaceDefaults: %v", err)
	}
	settings, err := service.store.GetWorkspaceSettings(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspaceSettings: %v", err)
	}
	if settings.TaskGitCredentialsMode != TaskGitCredentialsModeManaged {
		t.Fatalf("existing store task Git mode = %q, want managed fallback", settings.TaskGitCredentialsMode)
	}
	connection, err := service.store.GetWorkspaceConnection(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspaceConnection: %v", err)
	}
	if connection != nil {
		t.Fatalf("existing store was rebound to host CLI: %+v", connection)
	}
}
