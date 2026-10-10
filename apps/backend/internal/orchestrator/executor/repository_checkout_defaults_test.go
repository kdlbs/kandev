package executor

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type checkoutProjectionGate struct {
	*tasksqlite.Repository
	after func()
}

func (g *checkoutProjectionGate) GetRepository(ctx context.Context, id string) (*models.Repository, error) {
	row, err := g.Repository.GetRepository(ctx, id)
	if err == nil && g.after != nil {
		after := g.after
		g.after = nil
		after()
	}
	return row, err
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.20, AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.22
func TestRepositoryCheckoutDefaultsConsumerProjection(t *testing.T) {
	for _, pull := range []bool{false, true} {
		t.Run(map[bool]string{false: "offline", true: "refresh"}[pull], func(t *testing.T) {
			database, _ := remoteSelectionSQLiteTemplate.Open(t)
			store := tasksqlite.NewWithInitializedDB(database, database, nil)
			ctx := t.Context()
			require.NoError(t, store.CreateWorkspace(ctx, &models.Workspace{ID: "projection-ws", Name: "Workspace"}))
			path := filepath.Join(t.TempDir(), "checkout")
			runGitInTest(t, "", "clone", initBareOriginWithMain(t), path)
			runGitInTest(t, path, "branch", "develop")
			require.NoError(t, store.CreateRepository(ctx, &models.Repository{ID: "projection-repo", WorkspaceID: "projection-ws", Name: "Before", LocalPath: path, DefaultBranch: "main", PullBeforeWorktree: !pull}))
			gate := &checkoutProjectionGate{Repository: store}
			first := taskservice.NewService(taskservice.Repos{Workspaces: store, RepoEntities: gate}, nil, logger.Default(), taskservice.RepositoryDiscoveryConfig{})
			second := taskservice.NewService(taskservice.Repos{Workspaces: store, RepoEntities: store}, nil, logger.Default(), taskservice.RepositoryDiscoveryConfig{})
			branch, name := "develop", "Saved"
			gate.after = func() {
				_, err := second.UpdateRepository(ctx, "projection-repo", &taskservice.UpdateRepositoryRequest{DefaultBranch: &branch, PullBeforeWorktree: &pull})
				require.NoError(t, err)
			}
			_, err := first.UpdateRepository(ctx, "projection-repo", &taskservice.UpdateRepositoryRequest{Name: &name})
			require.NoError(t, err)
			executor := newTestExecutor(t, &mockAgentManager{}, store)
			for _, explicit := range []string{"", "main"} {
				info, err := executor.resolveTaskRepoInfo(ctx, &models.TaskRepository{ID: "projection-tr", TaskID: "projection-task", RepositoryID: "projection-repo", BaseBranch: explicit})
				require.NoError(t, err)
				expected := "develop"
				if explicit != "" {
					expected = explicit
				}
				request := &LaunchAgentRequest{}
				_, err = executor.applyRepositoryConfig(request, &v1.Task{ID: "projection-task", Title: "Projection"}, info, executorConfig{ExecutorType: string(models.ExecutorTypeWorktree)}, nil)
				require.NoError(t, err)
				require.Equal(t, expected, request.BaseBranch)
				require.Equal(t, "develop", request.DefaultBranch)
				require.Equal(t, pull, request.PullBeforeWorktree)
				require.Equal(t, path, request.RepositoryPath)
				require.True(t, request.UseWorktree)
				specs := buildRepoSpecs([]*repoInfo{info})
				require.Len(t, specs, 1)
				require.Equal(t, expected, specs[0].BaseBranch)
				require.Equal(t, "develop", specs[0].DefaultBranch)
				require.Equal(t, pull, specs[0].PullBeforeWorktree)
			}
		})
	}
}

func (m *mockRepository) UpdateRepositoryWithCheckoutIntent(ctx context.Context, row *models.Repository, _ repository.RepositoryCheckoutIntent) error {
	return m.UpdateRepository(ctx, row)
}
