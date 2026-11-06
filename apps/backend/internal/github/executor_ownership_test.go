package github

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestExecutorOwnershipSourceCreatorIsAuthenticated(t *testing.T) {
	_, store := setupWatchServiceTest(t)
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "alice", Role: authn.RoleMember})
	watch := &IssueWatch{WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Prompt: "Investigate", Enabled: true}
	watch.CreatedByUserID = "forged-creator"
	watch.GitHubHost = "git.company.example"
	require.NoError(t, store.CreateIssueWatch(ctx, watch))
	var creator string
	require.NoError(t, store.db.QueryRowContext(ctx, store.db.Rebind("SELECT created_by_user_id FROM github_issue_watches WHERE id = ?"), watch.ID).Scan(&creator))
	require.Equal(t, "alice", creator)
	loaded, err := store.GetIssueWatch(ctx, watch.ID)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, "alice", loaded.CreatedByUserID)
	require.Equal(t, watch.GitHubHost, loaded.GitHubHost)
}

func TestExecutorOwnershipSourceAdmissionPropagatesDenial(t *testing.T) {
	denied := errors.New("private executor unavailable")
	s := &Service{}
	s.SetExecutorContext(func(ctx context.Context, creator, profile string) (context.Context, error) {
		require.Equal(t, "alice", creator)
		require.Equal(t, "private-profile", profile)
		return ctx, denied
	})
	_, err := s.executorActorContext(t.Context(), "alice", "private-profile")
	require.ErrorIs(t, err, denied)
}
