package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/auth/authn"
	authstore "github.com/kandev/kandev/internal/auth/store"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/github"
	mcphandlers "github.com/kandev/kandev/internal/mcp/handlers"
	userstore "github.com/kandev/kandev/internal/user/store"
	"github.com/stretchr/testify/require"
)

// @covers AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.1
func TestTaskChangeCoordinatorGitHubIdentityModes(t *testing.T) {
	disabled := newDisabledAuthService(t)
	require.Equal(t, auth.ModeDisabled, disabled.Mode())
	cfg := &config.Config{}
	cfg.Features.Auth = true
	enabled := newEnabledAuthService(t, cfg)
	setup := newTaskChangeSetupAuthService(t)
	identity := &authn.Identity{UserID: "user-1", Role: authn.RoleMember, TokenID: "token-1"}
	for _, tc := range []struct {
		name       string
		svc        *auth.Service
		identity   *authn.Identity
		wantUser   string
		noResolver bool
	}{
		{name: "disabled", svc: disabled, wantUser: userstore.DefaultUserID},
		{name: "enabled rejects missing identity", svc: enabled},
		{name: "setup rejects missing identity", svc: setup},
		{name: "unavailable rejects missing identity"},
		{name: "unwired resolver rejects missing identity", noResolver: true},
		{name: "disabled preserves real identity", svc: disabled, identity: identity, wantUser: "user-1"},
		{name: "enabled preserves real identity", svc: enabled, identity: identity, wantUser: "user-1"},
		{name: "unavailable preserves real identity", identity: identity, wantUser: "user-1"},
		{name: "empty identity rejects fallback", svc: disabled, identity: &authn.Identity{}},
		{name: "blank identity rejects fallback", svc: disabled, identity: &authn.Identity{UserID: " \t"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			taskSvc, repos := newTaskChangeCoordinatorHarness(t)
			seedTaskChangeCoordinatorTask(t, repos, "ws-1", "task-1", "repo-1", "https://github.com", "acme", "api")
			provider := &fakeGitHubChangeLinks{}
			coordinator := taskChangeLinkCoordinator{
				tasks: taskSvc, github: provider, singleUserIdentity: taskChangeLinkIdentityResolver(tc.svc),
			}
			if tc.noResolver {
				coordinator.singleUserIdentity = nil
			}
			ctx := context.Background()
			if tc.identity != nil {
				ctx = authn.WithIdentity(ctx, *tc.identity)
			}
			links, err := coordinator.LinkTaskChange(ctx, mcphandlers.TaskChangeLinkRequest{
				TaskID: "task-1", Link: mcphandlers.TaskChangeLink{Provider: "github", RepositoryID: "repo-1", Number: 42},
			})
			if tc.wantUser == "" {
				require.EqualError(t, err, "authenticated user identity is required for GitHub PR links")
				require.Empty(t, links)
				require.Empty(t, provider.linkedURL)
				require.Empty(t, provider.prs)
				return
			}
			require.NoError(t, err)
			require.Len(t, links, 1)
			require.Equal(t, tc.wantUser, provider.linkedUserID)
			require.Equal(t, "ws-1", provider.linkedWorkspaceID)
			gotIdentity, ok := authn.IdentityFromContext(ctx)
			require.Equal(t, tc.identity != nil, ok)
			if tc.identity != nil {
				require.Equal(t, *tc.identity, gotIdentity)
			}
		})
	}
}

func newTaskChangeSetupAuthService(t *testing.T) *auth.Service {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	users, cleanup, err := userstore.Provide(conn, conn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cleanup()) })
	store, err := authstore.New(conn, conn)
	require.NoError(t, err)
	cfg := &config.Config{}
	cfg.Features.Auth = true
	svc, err := auth.NewService(context.Background(), auth.Deps{Cfg: cfg, Store: store, Users: users})
	require.NoError(t, err)
	require.Equal(t, auth.ModeSetup, svc.Mode())
	return svc
}

// @covers AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.3
// @covers AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.4
func TestTaskChangeCoordinatorGitHubReplacementSingleUser(t *testing.T) {
	for _, tc := range []struct {
		name    string
		same    bool
		linkErr error
	}{
		{name: "replaces old link"},
		{name: "same link is a no-op", same: true},
		{name: "provider failure preserves old link", linkErr: errors.New("provider unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			taskSvc, repos := newTaskChangeCoordinatorHarness(t)
			seedTaskChangeCoordinatorTask(t, repos, "ws-1", "task-1", "repo-1", "https://github.com", "acme", "api")
			old := mcphandlers.TaskChangeLink{Provider: "github", RepositoryID: "repo-1", Number: 7}
			provider := &fakeGitHubChangeLinks{
				prs:     []*github.TaskPR{{ID: "old-link", TaskID: "task-1", RepositoryID: "repo-1", PRNumber: 7}},
				linkErr: tc.linkErr,
			}
			coordinator := taskChangeLinkCoordinator{
				tasks: taskSvc, github: provider, singleUserIdentity: taskChangeLinkIdentityResolver(newDisabledAuthService(t)),
			}
			next := mcphandlers.TaskChangeLink{Provider: "github", RepositoryID: "repo-1", Number: 42}
			if tc.same {
				next = old
			}
			links, err := coordinator.ReplaceTaskChange(context.Background(), mcphandlers.TaskChangeLinkRequest{
				TaskID: "task-1", Old: &old, Link: next,
			})
			switch {
			case tc.linkErr != nil:
				require.ErrorIs(t, err, tc.linkErr)
				require.Empty(t, provider.unlinked)
				require.Len(t, provider.prs, 1)
				require.Equal(t, "old-link", provider.prs[0].ID)
			case tc.same:
				require.NoError(t, err)
				require.Equal(t, []mcphandlers.TaskChangeLink{old}, links)
				require.Empty(t, provider.linkedURL)
				require.Empty(t, provider.unlinked)
			default:
				require.NoError(t, err)
				require.Equal(t, []mcphandlers.TaskChangeLink{next}, links)
				require.Equal(t, []string{"old-link"}, provider.unlinked)
				require.Equal(t, userstore.DefaultUserID, provider.linkedUserID)
			}
		})
	}
}
