package backendapp

import (
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestWorkspaceClonePostgresAtomicCreation(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	taskStore, err := taskrepo.NewWithDB(database, database, nil)
	require.NoError(t, err)
	workflowStore, err := workflowrepo.NewWithDB(database, database, nil)
	require.NoError(t, err)
	gh, err := github.NewStore(database, database)
	require.NoError(t, err)
	crypto, err := secrets.NewMasterKeyProvider(t.TempDir())
	require.NoError(t, err)
	secretStore, _, err := secrets.Provide(database, database, crypto)
	require.NoError(t, err)
	cloner := &workspaceClonePersistence{writer: database, tasks: taskStore, workflows: workflowStore, github: gh, secrets: secretStore}
	source := &models.Workspace{Name: "Source"}
	_, err = taskStore.CreateWorkspaceWithKanban(t.Context(), source)
	require.NoError(t, err)
	// Clone admission uses the persisted workspace version.
	source, err = taskStore.GetWorkspace(t.Context(), source.ID)
	require.NoError(t, err)
	require.NoError(t, gh.UpsertWorkspaceSettings(t.Context(), &github.WorkspaceSettings{WorkspaceID: source.ID, RepoScopeMode: github.RepoScopeModeAll, SavedPresets: []byte(`[{"id":"mine"}]`)}))
	target := &models.Workspace{Name: "Copy"}
	copied, err := cloner.CloneWorkspace(t.Context(), source, target)
	require.NoError(t, err)
	require.Len(t, copied, 1)
	steps, err := workflowStore.ListStepsByWorkflow(t.Context(), copied[0].ID)
	require.NoError(t, err)
	require.NotEmpty(t, steps)
	settings, err := gh.GetWorkspaceSettings(t.Context(), target.ID)
	require.NoError(t, err)
	require.JSONEq(t, `[{"id":"mine"}]`, string(settings.SavedPresets))
}
