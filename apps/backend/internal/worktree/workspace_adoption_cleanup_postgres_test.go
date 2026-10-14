package worktree

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceAdoptionCleanupProvenancePostgres(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := tasksqlite.NewWithDB(db, db, nil)
	require.NoError(t, err)
	for _, q := range []string{
		`INSERT INTO workspaces(id,name,created_at,updated_at) VALUES('ws','Workspace',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO tasks(id,workspace_id,title,created_at,updated_at) VALUES('task','ws','Task',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO repositories(id,workspace_id,name,local_path,created_at,updated_at) VALUES('repo','ws','Repo','/project',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO task_environments(id,task_id,executor_type,status,workspace_path,ownership_generation,created_at,updated_at) VALUES('env','task','worktree','ready','/tasks/task',1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO task_environment_repos(id,task_environment_id,repository_id,worktree_id,worktree_path,worktree_branch,status,created_at,updated_at) VALUES('slot','env','repo','wt','/tasks/task/adopted','feature/adoption','active',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
	} {
		_, err := db.Exec(q)
		require.NoError(t, err)
	}
	store, err := NewSQLiteStore(db, db)
	require.NoError(t, err)
	wt, err := store.GetWorktreeByID(context.Background(), "wt")
	require.NoError(t, err)
	resetID, err := repo.ClaimTaskEnvironmentReset(context.Background(), "task", "env", 1, "reset")
	require.NoError(t, err)
	ordinary, err := store.GetWorkspaceAdoptionForArchive(recoveryclaim.WithTaskCleanupJob(context.Background(), recoveryclaim.TaskCleanupJob{ID: resetID, TaskID: "task"}), "wt")
	require.NoError(t, err)
	require.Nil(t, ordinary)
	require.NoError(t, repo.ReleaseTaskEnvironmentReset(context.Background(), resetID, models.TaskResourceCleanupStateSucceeded, ""))
	receipt := &models.WorkspaceAdoptionReceipt{OperationID: "adoption", Preview: models.WorkspaceAdoptionPreview{
		SlotID: "slot",
		Selection: models.WorkspaceAdoptionSelection{Snapshot: models.WorkspaceRecoverySelectionSnapshot{
			TaskEnvironmentID: "env", EnvironmentOwnerTaskID: "task", OwnershipGeneration: 1,
			Slots: []models.WorkspaceRecoveryInventorySlot{{EnvironmentRepoID: "slot", RepositoryID: "repo", WorktreeID: "wt", RepositoryLocalPath: "/project"}},
		}},
		Proof: models.WorkspaceAdoptionProof{CandidatePath: wt.Path, Branch: wt.Branch},
	}}
	encoded, err := json.Marshal(map[string]any{"worktrees": []*Worktree{wt}, "worktree_adoptions": map[string]*models.WorkspaceAdoptionReceipt{"wt": receipt}})
	require.NoError(t, err)
	_, err = db.Exec(db.Rebind(`INSERT INTO task_resource_cleanup_jobs(id,operation_id,task_id,trigger,state,resource_snapshot,attempts,created_at,updated_at) VALUES('job','delete','task','delete','running',?,1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`), string(encoded))
	require.NoError(t, err)
	ctx := recoveryclaim.WithTaskCleanupJob(context.Background(), recoveryclaim.TaskCleanupJob{ID: "job", TaskID: "task", Attempts: 1})
	got, err := store.GetWorkspaceAdoptionForArchive(ctx, "wt")
	require.NoError(t, err)
	require.Equal(t, receipt, got)
	_, err = db.Exec(`UPDATE task_environments SET ownership_generation=2 WHERE id='env'`)
	require.NoError(t, err)
	_, err = store.GetWorkspaceAdoptionForArchive(ctx, "wt")
	require.ErrorIs(t, err, models.ErrWorkspaceAdoptionStale)
	_, err = db.Exec(`UPDATE task_environments SET ownership_generation=1 WHERE id='env'`)
	require.NoError(t, err)
	_, err = db.Exec(`DELETE FROM tasks WHERE id='task'`)
	require.NoError(t, err)
	store, err = NewSQLiteStore(db, db)
	require.NoError(t, err)
	got, err = store.GetWorkspaceAdoptionForArchive(ctx, "wt")
	require.NoError(t, err)
	require.Equal(t, receipt, got)
	for _, claim := range []recoveryclaim.TaskCleanupJob{
		{ID: "job", TaskID: "foreign", Attempts: 1},
		{ID: "job", TaskID: "task", Attempts: 2},
	} {
		_, err := store.GetWorkspaceAdoptionForArchive(recoveryclaim.WithTaskCleanupJob(ctx, claim), "wt")
		require.Error(t, err)
	}
	_, err = db.Exec(`UPDATE task_resource_cleanup_jobs SET state='succeeded' WHERE id='job'`)
	require.NoError(t, err)
	_, err = store.GetWorkspaceAdoptionForArchive(ctx, "wt")
	require.Error(t, err)
}
