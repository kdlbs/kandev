package storeconformance

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/persistence/requiredstores"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
)

// @covers AC-EXECUTORS-PROFILE-EDITOR-001.8 AC-EXECUTORS-PROFILE-EDITOR-001.9 AC-EXECUTORS-PROFILE-EDITOR-001.12
func TestExecutorProfileScriptsStoreConformance(t *testing.T) {
	engines := []testconformance.EngineName{testconformance.EngineSQLite}
	if os.Getenv("KANDEV_TEST_POSTGRES_DSN") != "" {
		engines = append(engines, testconformance.EnginePostgres)
	}
	for _, name := range engines {
		t.Run(string(name), func(t *testing.T) {
			engine := testconformance.OpenEngine(t, name, "")
			var descriptor requiredstores.Descriptor
			for _, candidate := range requiredstores.Catalog() {
				if candidate.ID == "task" {
					descriptor = candidate
					break
				}
			}
			require.Equal(t, "task", descriptor.ID)
			adapter := adapterFor(descriptor)
			s := testconformance.ScenarioContext{Context: t.Context(), Engine: name, StoreID: "task", DB: engine.DB}
			require.NoError(t, adapter.Engines[name].Fresh(s))
			repo := tasksqlite.NewWithInitializedDB(engine.DB, engine.DB, nil)
			require.NoError(t, repo.CreateExecutor(t.Context(), &models.Executor{ID: "conformance-executor", Name: "Local", Type: models.ExecutorTypeLocal}))
			profile := &models.ExecutorProfile{ID: "conformance-profile", ExecutorID: "conformance-executor", Name: "Before", PrepareScript: "prepare", CleanupScript: "cleanup"}
			require.NoError(t, repo.CreateExecutorProfile(t.Context(), profile))
			stale := *profile
			profile.CleanupScript = "new-cleanup"
			require.NoError(t, repo.UpdateExecutorProfile(t.Context(), profile))
			stale.Name = "Renamed"
			empty := ""
			require.NoError(t, repo.UpdateExecutorProfileWithScriptIntent(t.Context(), &stale, models.ExecutorProfileScriptIntent{PrepareScript: &empty}))
			require.Empty(t, stale.PrepareScript)
			require.Equal(t, "new-cleanup", stale.CleanupScript)
			stored, err := repo.GetExecutorProfile(t.Context(), stale.ID)
			require.NoError(t, err)
			require.Equal(t, stale.PrepareScript, stored.PrepareScript)
			require.Equal(t, stale.CleanupScript, stored.CleanupScript)
			require.True(t, stale.UpdatedAt.Equal(stored.UpdatedAt))
		})
	}
}
