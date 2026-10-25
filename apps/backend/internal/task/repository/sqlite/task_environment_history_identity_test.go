package sqlite

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestTaskEnvironmentAliasIdentityDeletedHistory(t *testing.T) {
	exerciseEnvironmentDeletedHistory(t, newRepoForEntityTests)
}

func TestPostgresAliasIdentityDeletedHistory(t *testing.T) {
	exerciseEnvironmentDeletedHistory(t, func(t *testing.T) *Repository {
		db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
		repo, err := NewWithDB(db, db, nil)
		require.NoError(t, err)
		return repo
	})
}

func exerciseEnvironmentDeletedHistory(t *testing.T, open func(*testing.T) *Repository) {
	t.Helper()
	for _, operation := range []string{"transition", "update history", "delete history", "delete slots", "delete environment", "delete task environments", "revive history", "ambiguous history"} {
		t.Run(operation, func(t *testing.T) {
			r := open(t)
			ctx := t.Context()
			require.NoError(t, r.CreateWorkspace(ctx, &models.Workspace{ID: "workspace", Name: "Workspace"}))
			require.NoError(t, r.CreateTask(ctx, &models.Task{ID: "task", WorkspaceID: "workspace", Title: "Task"}))
			env := &models.TaskEnvironment{ID: "env", TaskID: "task", ExecutorType: "worktree", WorkspacePath: "/tasks/task", Status: models.TaskEnvironmentStatusReady}
			require.NoError(t, r.CreateTaskEnvironment(ctx, env))
			live := &models.TaskEnvironmentRepo{ID: "live", TaskEnvironmentID: "env", RepositoryID: "repo", BranchSlug: "main", WorktreeID: "physical", WorktreePath: "/tasks/task/repo", WorktreeBranch: "main"}
			require.NoError(t, r.CreateTaskEnvironmentRepo(ctx, live))
			_, err := r.db.Exec(`INSERT INTO task_environment_repos(id,task_environment_id,repository_id,branch_slug,worktree_id,status,deleted_at,created_at,updated_at) VALUES('history','env','repo','old','physical','deleted',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
			require.NoError(t, err)
			deleted := time.Now().UTC()
			history := &models.TaskEnvironmentRepo{ID: "history", TaskEnvironmentID: "env", RepositoryID: "repo", BranchSlug: "old", WorktreeID: "physical", Status: "deleted", DeletedAt: &deleted}
			switch operation {
			case "transition":
				err = r.PersistTaskEnvironmentTransition(ctx, env, []*models.TaskEnvironmentRepo{live}, false)
			case "update history":
				history.ErrorMessage = "retained history"
				err = r.UpdateTaskEnvironmentRepo(ctx, history)
			case "delete history":
				err = r.DeleteTaskEnvironmentRepo(ctx, "history")
			case "delete slots":
				err = r.DeleteTaskEnvironmentReposByEnv(ctx, "env")
			case "delete environment":
				err = r.DeleteTaskEnvironment(ctx, "env")
			case "delete task environments":
				err = r.DeleteTaskEnvironmentsByTask(ctx, "task")
			case "revive history":
				history.Status = "active"
				history.DeletedAt = nil
				err = r.UpdateTaskEnvironmentRepo(ctx, history)
			case "ambiguous history":
				_, err = r.db.Exec(`UPDATE task_environment_repos SET status='deleted',deleted_at=CURRENT_TIMESTAMP WHERE id='live'`)
				require.NoError(t, err)
				err = r.DeleteTaskEnvironmentRepo(ctx, "history")
			}
			if operation == "revive history" || operation == "ambiguous history" {
				var ambiguous *models.WorktreeIdentityAmbiguousError
				require.ErrorAs(t, err, &ambiguous)
				var status string
				require.NoError(t, r.db.Get(&status, `SELECT status FROM task_environment_repos WHERE id='history'`))
				require.Equal(t, "deleted", status)
				return
			}
			require.NoError(t, err)
			if operation == "transition" || operation == "update history" || operation == "delete history" {
				var physical string
				require.NoError(t, r.db.Get(&physical, `SELECT worktree_id FROM task_environment_repos WHERE id='live'`))
				require.Equal(t, "physical", physical)
			}
		})
	}
}
