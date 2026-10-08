package executor

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/testutil"
)

var projectionScriptTemplate = testutil.NewSQLiteTemplate(func(database *sqlx.DB) error {
	_, err := tasksqlite.NewWithDB(database, database, nil)
	return err
})

type profileProjectionStore struct {
	executorStore
	repo *tasksqlite.Repository
}

func (s *profileProjectionStore) GetExecutorProfile(ctx context.Context, id string) (*models.ExecutorProfile, error) {
	return s.repo.GetExecutorProfile(ctx, id)
}

func (s *profileProjectionStore) GetExecutor(ctx context.Context, id string) (*models.Executor, error) {
	return s.repo.GetExecutor(ctx, id)
}

// @covers AC-EXECUTORS-PROFILE-EDITOR-001.10
func TestExecutorProfileScriptsApplyProfile(t *testing.T) {
	database, _ := projectionScriptTemplate.Open(t)
	repo := tasksqlite.NewWithInitializedDB(database, database, nil)
	require.NoError(t, repo.CreateExecutor(t.Context(), &models.Executor{ID: "projection-executor", Name: "SSH", Type: models.ExecutorTypeSSH}))
	require.NoError(t, repo.CreateExecutorProfile(t.Context(), &models.ExecutorProfile{ID: "projection-profile", ExecutorID: "projection-executor", PrepareScript: "before", CleanupScript: "before"}))
	e := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	e.repo = &profileProjectionStore{executorStore: e.repo, repo: repo}
	svc := service.NewService(service.Repos{Executors: repo}, nil, e.logger, service.RepositoryDiscoveryConfig{})
	prepare, cleanup := "saved-setup", "saved-cleanup"
	_, err := svc.UpdateExecutorProfile(t.Context(), "projection-profile", &service.UpdateExecutorProfileRequest{PrepareScript: &prepare, CleanupScript: &cleanup})
	require.NoError(t, err)
	name := "Renamed"
	_, err = svc.UpdateExecutorProfile(t.Context(), "projection-profile", &service.UpdateExecutorProfileRequest{Name: &name})
	require.NoError(t, err)
	cfg := executorConfig{ExecutorID: "projection-executor"}
	metadata := map[string]interface{}{}
	e.applyProfile(t.Context(), "projection-profile", &cfg, metadata)
	require.Equal(t, prepare, cfg.SetupScript)
	require.Equal(t, cleanup, cfg.CleanupScript)
	require.Equal(t, cleanup, metadata[lifecycle.MetadataKeyCleanupScript])
}
