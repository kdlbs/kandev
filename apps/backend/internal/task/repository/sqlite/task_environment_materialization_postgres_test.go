package sqlite_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.6
// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.7
func TestPostgresMaterializationSerializesAndRechecksOwner(t *testing.T) {
	for _, operation := range []string{"release", "launch_reelection", "binding_abandoned", "binding_failed", "binding_observes_failed", "shared_abandoned", "shared_failed", "shared_observes_failed", "finalize"} {
		t.Run(operation, func(t *testing.T) {
			db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
			db.SetMaxOpenConns(4)
			db.SetMaxIdleConns(4)
			repo, err := tasksqlite.NewWithDB(db, db, nil)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			t.Cleanup(cancel)
			owner := seedPostgresMaterializationOperation(t, repo, operation)
			const replacement = "pg-materialization-replacement"
			if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: replacement, TaskID: owner.TaskID}); err != nil {
				t.Fatal(err)
			}
			if operation == "finalize" {
				if err := repo.CreateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{ID: "pg-retained-inventory",
					TaskEnvironmentID: owner.TaskEnvironmentID, RepositoryID: "pg-repository", WorktreePath: "/retained"}); err != nil {
					t.Fatal(err)
				}
			}
			holder, err := db.BeginTxx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = holder.Rollback() })
			if _, err := holder.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
				"task-environment-materialization:"+owner.TaskEnvironmentID); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				done <- runPostgresMaterializationOperation(ctx, repo, operation, owner)
			}()
			t.Cleanup(func() {
				cancel()
				_ = holder.Rollback()
				select {
				case <-finished:
				case <-time.After(5 * time.Second):
					t.Error("materialization worker did not stop")
				}
			})
			waitForMaterializationAdvisoryLock(t, ctx, db, owner.TaskEnvironmentID, finished)
			// The independent connection changes the owner while the operation
			// waits. Its earlier snapshot must never authorize a later write.
			status, materializer := models.TaskEnvironmentStatusCreating, replacement
			if operation == "binding_observes_failed" || operation == "shared_observes_failed" {
				status, materializer = models.TaskEnvironmentStatusFailed, ""
			}
			if _, err := holder.ExecContext(ctx, db.Rebind(`UPDATE task_environments SET status = ?, materialization_session_id = ?, updated_at = ? WHERE id = ?`),
				string(status), materializer, time.Now().UTC(), owner.TaskEnvironmentID); err != nil {
				t.Fatal(err)
			}
			if err := holder.Commit(); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("materialization operation did not finish after lock release")
			}
			current, err := repo.GetTaskEnvironment(ctx, owner.TaskEnvironmentID)
			if err != nil {
				t.Fatal(err)
			}
			if status == models.TaskEnvironmentStatusFailed {
				materializer = "pg-retry"
			}
			if current.Status != models.TaskEnvironmentStatusCreating || current.MaterializationSessionID != materializer {
				t.Fatalf("replacement claim changed: status=%q owner=%q", current.Status, current.MaterializationSessionID)
			}
			if operation == "finalize" {
				inventory, err := repo.ListTaskEnvironmentRepos(ctx, owner.TaskEnvironmentID)
				if err != nil {
					t.Fatal(err)
				}
				if len(inventory) != 1 || inventory[0].ID != "pg-retained-inventory" || inventory[0].WorktreePath != "/retained" {
					t.Fatalf("former owner's inventory write escaped rollback: %+v", inventory)
				}
			}
		})
	}
}

func seedPostgresMaterializationOperation(t *testing.T, repo *tasksqlite.Repository, operation string) *models.TaskSession {
	t.Helper()
	ctx := context.Background()
	const taskID = "task-pg-materialization"
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-pg-materialization", Name: "PG materialization"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: "ws-pg-materialization", Title: taskID}); err != nil {
		t.Fatal(err)
	}
	owner := &models.TaskSession{ID: "session-pg-materialization", TaskID: taskID}
	if operation == "shared_abandoned" || operation == "shared_failed" || operation == "shared_observes_failed" {
		seedPostgresSharedMaterializer(t, repo, owner)
	} else {
		if err := repo.CreateTaskSessionWithWorkspaceBinding(ctx, owner, &models.TaskEnvironment{TaskID: taskID, ExecutorType: string(models.ExecutorTypeLocal)}); err != nil {
			t.Fatal(err)
		}
	}
	if operation == "launch_reelection" || operation == "binding_failed" || operation == "shared_failed" {
		if changed, err := repo.FailTaskEnvironmentMaterialization(ctx, owner.ID); err != nil || !changed {
			t.Fatalf("seed failed claim: changed=%v err=%v", changed, err)
		}
	}
	if operation == "binding_abandoned" || operation == "shared_abandoned" || operation == "binding_observes_failed" || operation == "shared_observes_failed" {
		if err := repo.UpdateTaskSessionState(ctx, owner.ID, models.TaskSessionStateFailed, ""); err != nil {
			t.Fatal(err)
		}
	}
	return owner
}

func runPostgresMaterializationOperation(ctx context.Context, repo *tasksqlite.Repository, operation string, owner *models.TaskSession) error {
	switch operation {
	case "release":
		changed, err := repo.FailTaskEnvironmentMaterialization(ctx, owner.ID)
		if changed {
			return errors.New("former owner released replacement claim")
		}
		return err
	case "launch_reelection":
		changed, err := repo.ReelectFailedTaskEnvironmentMaterialization(ctx, owner.TaskEnvironmentID, "pg-retry")
		if changed {
			return errors.New("stale failed snapshot re-elected replacement claim")
		}
		return err
	case "binding_abandoned", "binding_failed", "binding_observes_failed":
		err := repo.CreateTaskSessionWithWorkspaceBinding(ctx, &models.TaskSession{ID: "pg-retry", TaskID: owner.TaskID},
			&models.TaskEnvironment{TaskID: owner.TaskID, ExecutorType: string(models.ExecutorTypeLocal)})
		if operation == "binding_observes_failed" {
			return err
		}
		if !errors.Is(err, models.ErrWorkspacePreparing) {
			return fmt.Errorf("binding error = %v, want preparing", err)
		}
		return nil
	case "shared_abandoned", "shared_failed", "shared_observes_failed":
		err := repo.CreateTaskSessionWithSharedGroupWorkspaceBinding(ctx, &models.TaskSession{ID: "pg-retry", TaskID: "task-pg-member"},
			&models.TaskEnvironment{TaskID: "task-pg-member", ExecutorType: string(models.ExecutorTypeLocal)}, "pg-materialization-group")
		if operation == "shared_observes_failed" {
			return err
		}
		if !errors.Is(err, models.ErrWorkspacePreparing) {
			return fmt.Errorf("shared binding error = %v, want preparing", err)
		}
		return nil
	case "finalize":
		err := repo.FinalizeTaskEnvironmentMaterialization(ctx, &models.TaskEnvironment{ID: owner.TaskEnvironmentID, TaskID: owner.TaskID,
			ExecutorType: string(models.ExecutorTypeLocal), Status: models.TaskEnvironmentStatusReady},
			[]*models.TaskEnvironmentRepo{{ID: "pg-stale-inventory", RepositoryID: "pg-repository", WorktreePath: "/stale-owner"}}, owner.ID)
		if err == nil {
			return errors.New("former owner finalized replacement claim")
		}
		return nil
	}
	return fmt.Errorf("unknown materialization operation %q", operation)
}

func waitForMaterializationAdvisoryLock(t *testing.T, ctx context.Context, db *sqlx.DB, environmentID string, finished <-chan struct{}) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_locks
 WHERE locktype = 'advisory' AND NOT granted AND objsubid = 1
 AND classid = ((hashtextextended($1, 0) >> 32) & 4294967295)::oid
 AND objid = (hashtextextended($1, 0) & 4294967295)::oid
)`, "task-environment-materialization:"+environmentID).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		select {
		case <-finished:
			t.Fatal("operation returned without waiting for the environment advisory lock")
		case <-ctx.Done():
			t.Fatal("environment advisory lock wait was not observed")
		case <-ticker.C:
		}
	}
}

func seedPostgresSharedMaterializer(t *testing.T, repo *tasksqlite.Repository, owner *models.TaskSession) {
	t.Helper()
	ctx := context.Background()
	taskID := owner.TaskID
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-pg-member", WorkspaceID: "ws-pg-materialization", Title: "Member"}); err != nil {
		t.Fatal(err)
	}
	office, err := officesqlite.NewWithDB(sqlx.NewDb(repo.DB(), "pgx"), sqlx.NewDb(repo.DB(), "pgx"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := office.CreateWorkspaceGroup(ctx, &officemodels.WorkspaceGroup{ID: "pg-materialization-group", WorkspaceID: "ws-pg-materialization", OwnerTaskID: taskID, MaterializedKind: "local"}); err != nil {
		t.Fatal(err)
	}
	for _, member := range []string{taskID, "task-pg-member"} {
		if err := office.AddWorkspaceGroupMember(ctx, "pg-materialization-group", member, ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.CreateTaskSessionWithSharedGroupWorkspaceBinding(ctx, owner,
		&models.TaskEnvironment{TaskID: taskID, ExecutorType: string(models.ExecutorTypeLocal)}, "pg-materialization-group"); err != nil {
		t.Fatal(err)
	}
}
