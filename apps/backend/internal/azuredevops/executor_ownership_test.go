package azuredevops

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestExecutorOwnershipSourceCreatorIsAuthenticated(t *testing.T) {
	store := newWatchStore(t)
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "alice", Role: authn.RoleMember})
	watch := &WorkItemWatch{WorkspaceID: "ws-1", ProjectID: "project-1", WIQL: "SELECT [System.Id] FROM WorkItems"}
	watch.CreatedByUserID = "forged-creator"
	require.NoError(t, store.CreateWorkItemWatch(ctx, watch))
	var creator string
	require.NoError(t, store.db.QueryRowContext(ctx, store.db.Rebind("SELECT created_by_user_id FROM azure_devops_work_item_watches WHERE id = ?"), watch.ID).Scan(&creator))
	require.Equal(t, "alice", creator)
	loaded, err := store.GetWorkItemWatch(ctx, watch.ID)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, "alice", loaded.CreatedByUserID)
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
