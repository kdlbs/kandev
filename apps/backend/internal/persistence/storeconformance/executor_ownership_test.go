package storeconformance

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/persistence/requiredstores"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
)

func TestExecutorOwnershipStoreConformance(t *testing.T) {
	for _, name := range []testconformance.EngineName{testconformance.EngineSQLite, testconformance.EnginePostgres} {
		t.Run(string(name), func(t *testing.T) {
			engine := testconformance.OpenEngine(t, name, "")
			var descriptor requiredstores.Descriptor
			for _, candidate := range requiredstores.Catalog() {
				if candidate.ID == "task" {
					descriptor = candidate
					break
				}
			}
			adapter := adapterFor(descriptor)
			scenario := testconformance.ScenarioContext{Context: t.Context(), Engine: name, StoreID: "task", DB: engine.DB}
			require.NoError(t, adapter.Engines[name].Fresh(scenario))
			repo := tasksqlite.NewWithInitializedDB(engine.DB, engine.DB, nil)
			executor := &models.Executor{ID: "ownership-parent", Name: "Shared", Type: models.ExecutorTypeLocal}
			require.NoError(t, repo.CreateExecutor(t.Context(), executor))
			profile := &models.ExecutorProfile{ID: "ownership-profile", ExecutorID: executor.ID, Name: "Personal", OwnerUserID: "alice"}
			require.NoError(t, repo.CreateExecutorProfile(t.Context(), profile))
			stale := *profile
			bob := "bob"
			profile.OwnershipIntent = &models.ExecutorOwnershipIntent{ExpectedOwnerUserID: "alice", OwnerUserID: &bob}
			require.NoError(t, repo.UpdateExecutorProfile(t.Context(), profile))
			stale.Name = "Stale"
			stale.OwnershipIntent = &models.ExecutorOwnershipIntent{ExpectedOwnerUserID: "alice"}
			require.ErrorIs(t, repo.UpdateExecutorProfileWithScriptIntent(t.Context(), &stale, models.ExecutorProfileScriptIntent{}), models.ErrExecutorOwnershipChanged)
			loaded, err := repo.GetExecutorProfile(t.Context(), profile.ID)
			require.NoError(t, err)
			require.Equal(t, "bob", loaded.OwnerUserID)
			require.Equal(t, "Personal", loaded.Name)
			require.NoError(t, adapter.Engines[name].Fresh(scenario), "same-database schema replay")
			loaded, err = repo.GetExecutorProfile(t.Context(), profile.ID)
			require.NoError(t, err)
			require.Equal(t, "bob", loaded.OwnerUserID)
		})
	}
}
