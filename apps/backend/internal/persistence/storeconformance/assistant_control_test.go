package storeconformance

import (
	"context"
	"github.com/kandev/kandev/internal/orchestration/models"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAssistantControlPersistenceEngines(t *testing.T) {
	for _, name := range []testconformance.EngineName{testconformance.EngineSQLite, testconformance.EnginePostgres} {
		t.Run(string(name), func(t *testing.T) {
			repo, binding := authorityStoreFixture(t, name)
			ctx := context.Background()
			require.NoError(t, repo.SetAssistantPaused(ctx, binding, true))
			require.NoError(t, repo.SetAssistantPaused(ctx, binding, false))
			stale := *binding
			stale.Version++
			require.ErrorIs(t, repo.SetAssistantPaused(ctx, &stale, true), models.ErrConflict)
			require.ErrorIs(t, repo.AdvanceAssistantControlIntent(ctx, &stale, 0), models.ErrConflict)
			require.NoError(t, repo.AdvanceAssistantControlIntent(ctx, binding, 0))
			require.ErrorIs(t, repo.AdvanceAssistantControlIntent(ctx, binding, 0), models.ErrConflict)
			require.NoError(t, repo.Migrate())
			revision, err := repo.IntentRevision(ctx, binding.ConversationID)
			require.NoError(t, err)
			require.EqualValues(t, 1, revision)
			require.NoError(t, repo.AdvanceAssistantControlIntent(ctx, binding, revision))
			tasks, err := repo.AssistantManagedTasks(ctx, binding.ID, "", 26)
			require.NoError(t, err)
			require.Empty(t, tasks, "the private conversation is never a managed worker")
		})
	}
}
