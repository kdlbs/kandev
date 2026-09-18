package storeconformance

import (
	"context"
	"github.com/kandev/kandev/internal/orchestration/models"
	orchstore "github.com/kandev/kandev/internal/orchestration/repository/sqlite"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
)

// @covers AC-ORCHESTRATION-ASSISTANT-009.1, AC-ORCHESTRATION-ASSISTANT-009.4
func TestAssistantWorkspaceGrantDefaultsAndCAS(t *testing.T) {
	for _, engine := range []testconformance.EngineName{testconformance.EngineSQLite, testconformance.EnginePostgres} {
		t.Run(string(engine), func(t *testing.T) {
			repo, b := authorityStoreFixture(t, engine, "linked")
			ctx := context.Background()
			rows, err := repo.WorkspaceGrants(ctx, b.ID, "", 10)
			require.NoError(t, err)
			require.Empty(t, rows, "no automatic workspace adoption")
			grant := &models.WorkspaceGrant{WorkspaceID: "linked", BindingVersion: b.Version, ReceiverProfileID: "selected-profile", ReceiverProfileRevision: "account-revision", AuthorityRevision: "native-authority", Scope: models.WorkspaceGrantScope{Operations: []string{"observe"}, ContextExports: []string{"task_summary"}}}
			require.NoError(t, repo.SaveWorkspaceGrant(ctx, b, grant, 0))
			stored, err := repo.WorkspaceGrant(ctx, b.ID, "linked")
			require.NoError(t, err)
			require.EqualValues(t, 1, stored.Revision)
			require.Equal(t, b.OwnerUserID, stored.OwnerUserID)
			require.Equal(t, grant.Scope, stored.Scope)
			require.ErrorIs(t, repo.SaveWorkspaceGrant(ctx, b, grant, 0), models.ErrConflict)
			_, err = repo.WorkspaceGrant(ctx, "foreign", "linked")
			require.Error(t, err)
			foreign := *b
			foreign.OwnerUserID = "foreign"
			require.ErrorIs(t, repo.RevokeWorkspaceGrant(ctx, &foreign, "linked", 1), models.ErrConflict)
			require.NoError(t, repo.Migrate())
			require.NoError(t, repo.RevokeWorkspaceGrant(ctx, b, "linked", 1))
			stored, err = repo.WorkspaceGrant(ctx, b.ID, "linked")
			require.NoError(t, err)
			require.NotNil(t, stored.RevokedAt)
			require.EqualValues(t, 2, stored.Revision)
			events, err := repo.WorkspaceGrantEvents(ctx, b.ID, "linked", "", 10)
			require.NoError(t, err)
			require.Len(t, events, 2)
			require.NoError(t, repo.SaveWorkspaceGrant(ctx, b, grant, 2))
			require.Equal(t, stored.ID, grant.ID)
			require.EqualValues(t, 3, grant.Revision)
			require.Nil(t, grant.RevokedAt)
			old := *b
			require.NoError(t, repo.SelectAssistant(ctx, b, b.Version))
			require.ErrorIs(t, repo.SaveWorkspaceGrant(ctx, &old, grant, 3), models.ErrConflict)
		})
	}
}

// @covers AC-ORCHESTRATION-ASSISTANT-009.2
func TestAssistantWorkspaceGrantConcurrentRevision(t *testing.T) {
	for _, engine := range []testconformance.EngineName{testconformance.EngineSQLite, testconformance.EnginePostgres} {
		t.Run(string(engine), func(t *testing.T) {
			repo, b := authorityStoreFixture(t, engine, "linked")
			grant := &models.WorkspaceGrant{WorkspaceID: "linked", BindingVersion: b.Version, ReceiverProfileID: "selected-profile", ReceiverProfileRevision: "account-revision", AuthorityRevision: "native-authority", Scope: models.WorkspaceGrantScope{Operations: []string{"observe"}, ContextExports: []string{"task_summary"}}}
			require.NoError(t, repo.SaveWorkspaceGrant(context.Background(), b, grant, 0))
			assertWorkspaceGrantRace(t, repo, b, grant)
		})
	}
}
func assertWorkspaceGrantRace(t *testing.T, repo *orchstore.Repository, b *models.AssistantBinding, grant *models.WorkspaceGrant) {
	t.Helper()
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if i == 0 {
				copy := *grant
				results <- repo.SaveWorkspaceGrant(context.Background(), b, &copy, 1)
			} else {
				results <- repo.RevokeWorkspaceGrant(context.Background(), b, "linked", 1)
			}
		}()
	}
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else {
			require.ErrorIs(t, err, models.ErrConflict)
		}
	}
	require.Equal(t, 1, winners)
	stored, err := repo.WorkspaceGrant(context.Background(), b.ID, "linked")
	require.NoError(t, err)
	require.EqualValues(t, 2, stored.Revision)
}

func TestAssistantWorkspaceGrantRevocationReceipt(t *testing.T) {
	for _, engine := range []testconformance.EngineName{testconformance.EngineSQLite, testconformance.EnginePostgres} {
		t.Run(string(engine), func(t *testing.T) {
			repo, b := authorityStoreFixture(t, engine, "linked")
			ctx := context.Background()
			grant := &models.WorkspaceGrant{WorkspaceID: "linked", BindingVersion: b.Version, ReceiverProfileID: "selected-profile", ReceiverProfileRevision: "account-revision", AuthorityRevision: "native-authority", Scope: models.WorkspaceGrantScope{Operations: []string{"observe"}, ContextExports: []string{"task_summary"}}}
			require.NoError(t, repo.SaveWorkspaceGrant(ctx, b, grant, 0))
			require.NoError(t, repo.RecordWorkspaceExport(ctx, b, grant, "task_summary"))
			require.NoError(t, repo.RecordWorkspaceExport(ctx, b, grant, "task_summary"))
			rows, err := repo.WorkspaceExports(ctx, b.ID, b.ConversationID, "", 10)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			require.Equal(t, grant.ReceiverProfileID, rows[0].ReceiverProfileID)
			require.Error(t, repo.RecordWorkspaceExport(ctx, b, grant, "task_result"), "ungranted fields cannot acquire a delivery receipt")
			require.NoError(t, repo.RevokeWorkspaceGrant(ctx, b, "linked", 1))
			require.ErrorIs(t, repo.RecordWorkspaceExport(ctx, b, grant, "task_summary"), models.ErrConflict)
			require.NoError(t, repo.ForgetWorkspaceContext(ctx, b, "linked", 2))
			forgotten, err := repo.WorkspaceGrant(ctx, b.ID, "linked")
			require.NoError(t, err)
			require.NotNil(t, forgotten.RevokedAt, "forgetting cannot restore revoked authority")
			require.EqualValues(t, 3, forgotten.Revision)
			require.NoError(t, repo.Migrate())
			rows, err = repo.WorkspaceExports(ctx, b.ID, b.ConversationID, "", 10)
			require.NoError(t, err)
			require.Len(t, rows, 1, "revocation cannot claim to undo a prior export")
			foreign, err := repo.WorkspaceExports(ctx, "foreign", b.ConversationID, "", 10)
			require.NoError(t, err)
			require.Empty(t, foreign)
		})
	}
}
