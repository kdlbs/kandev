package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestKubernetesEnvironmentInventorySurvivesSessionRemoval(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedRecoveryClaimEnvironment(t, repo, "task-kube", "env-kube")
	// The physical resource record must not depend on a session's lifetime.
	_, err := repo.db.ExecContext(ctx, `INSERT INTO task_environment_kubernetes
  (environment_id, task_id, ownership_generation, metadata, control_secret_id, bootstrap_secret_id)
  VALUES ('env-kube', 'task-kube', 1, '{"pod_uid":"exact-uid"}', 'control-secret', 'bootstrap-secret')`)
	require.NoError(t, err)
	session := &models.TaskSession{ID: "session-kube", TaskID: "task-kube", TaskEnvironmentID: "env-kube"}
	require.NoError(t, repo.CreateTaskSession(ctx, session))
	require.NoError(t, repo.DeleteTaskSession(ctx, session))
	require.Error(t, repo.DeleteTaskEnvironment(ctx, "env-kube"), "remote inventory must be cleaned before its environment can be deleted")
	var metadata string
	require.NoError(t, repo.db.QueryRowContext(ctx, `SELECT metadata FROM task_environment_kubernetes WHERE environment_id = 'env-kube'`).Scan(&metadata))
	require.JSONEq(t, `{"pod_uid":"exact-uid"}`, metadata)
}

func TestKubernetesEnvironmentClaimExclusion(t *testing.T) {
	repo := newRepoForEntityTests(t)
	store, ok := interface{}(repo).(interface {
		ClaimKubernetesEnvironment(context.Context, string, string, int64, string) (*models.KubernetesEnvironment, error)
		SaveKubernetesEnvironment(context.Context, *models.KubernetesEnvironment, bool) error
		GetKubernetesEnvironment(context.Context, string) (*models.KubernetesEnvironment, error)
	})
	require.True(t, ok, "repository must support durable Kubernetes inventory claims")
	ctx := context.Background()
	seedRecoveryClaimEnvironment(t, repo, "task-claims", "env-claims")
	claim, err := store.ClaimKubernetesEnvironment(ctx, "env-claims", "task-claims", 1, "create")
	require.NoError(t, err)
	_, err = store.ClaimKubernetesEnvironment(ctx, "env-claims", "task-claims", 1, "delete")
	require.Error(t, err, "a competing operation must not acquire the resource")
	claim.Metadata = map[string]interface{}{"pod_uid": "original"}
	require.NoError(t, store.SaveKubernetesEnvironment(ctx, claim, true))
	replacement, err := store.ClaimKubernetesEnvironment(ctx, "env-claims", "task-claims", 1, "delete")
	require.NoError(t, err)
	require.Error(t, store.SaveKubernetesEnvironment(ctx, claim, true), "stale operation must not overwrite a replacement")
	got, err := store.GetKubernetesEnvironment(ctx, "env-claims")
	require.NoError(t, err)
	require.Equal(t, replacement.OperationID, got.OperationID)
	require.Equal(t, "original", got.Metadata["pod_uid"])
	_, err = store.ClaimKubernetesEnvironment(ctx, "env-claims", "foreign-task", 1, "foreign")
	require.Error(t, err)
	_, err = store.ClaimKubernetesEnvironment(ctx, "env-claims", "task-claims", 2, "stale-generation")
	require.Error(t, err)
}

func TestKubernetesEnvironmentRestartReplay(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedRecoveryClaimEnvironment(t, repo, "task-replay-kube", "env-replay-kube")
	claim, err := repo.ClaimKubernetesEnvironment(ctx, "env-replay-kube", "task-replay-kube", 1, "operation")
	require.NoError(t, err)
	claim.Metadata = map[string]interface{}{"pod_uid": "retained"}
	claim.ControlSecretID = "encrypted-control-reference"
	claim.BootstrapSecretID = "encrypted-bootstrap-reference"
	require.NoError(t, repo.SaveKubernetesEnvironment(ctx, claim, false))
	require.NoError(t, repo.runMigrations(ctx))
	recovered, err := repo.ClaimKubernetesEnvironment(ctx, claim.EnvironmentID, claim.TaskID, 1, "operation")
	require.NoError(t, err)
	require.Equal(t, claim, recovered)
}

func TestPostgresKubernetesEnvironmentClaimExclusion(t *testing.T) {
	first, second, _ := newTaskPostgresRepoPair(t)
	ctx := context.Background()
	seedRecoveryClaimEnvironment(t, first, "task-pg-kube", "env-pg-kube")
	claim, err := first.ClaimKubernetesEnvironment(ctx, "env-pg-kube", "task-pg-kube", 1, "create")
	require.NoError(t, err)
	_, err = second.ClaimKubernetesEnvironment(ctx, "env-pg-kube", "task-pg-kube", 1, "delete")
	require.ErrorIs(t, err, models.ErrKubernetesEnvironmentConflict)
	require.NoError(t, first.SaveKubernetesEnvironment(ctx, claim, true))
	_, err = second.ClaimKubernetesEnvironment(ctx, "env-pg-kube", "task-pg-kube", 1, "delete")
	require.NoError(t, err)
}

func TestKubernetesEnvironmentRejectsChangedOwnerCheckpoint(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedRecoveryClaimEnvironment(t, repo, "task-owner-kube", "env-owner-kube")
	claim, err := repo.ClaimKubernetesEnvironment(ctx, "env-owner-kube", "task-owner-kube", 1, "create")
	require.NoError(t, err)
	_, err = repo.db.ExecContext(ctx, `UPDATE task_environments SET ownership_generation = 2 WHERE id = 'env-owner-kube'`)
	require.NoError(t, err)
	require.ErrorIs(t, repo.SaveKubernetesEnvironment(ctx, claim, true), models.ErrKubernetesEnvironmentConflict)
}

func TestKubernetesEnvironmentDeletionRequiresCurrentClaim(t *testing.T) {
	repo := newRepoForEntityTests(t)
	deleter, ok := interface{}(repo).(interface {
		DeleteKubernetesEnvironment(context.Context, *models.KubernetesEnvironment) error
	})
	require.True(t, ok, "resource cleanup requires a conditional inventory release")
	ctx := context.Background()
	seedRecoveryClaimEnvironment(t, repo, "task-delete-kube", "env-delete-kube")
	claim, err := repo.ClaimKubernetesEnvironment(ctx, "env-delete-kube", "task-delete-kube", 1, "delete")
	require.NoError(t, err)
	stale := *claim
	stale.Revision--
	require.ErrorIs(t, deleter.DeleteKubernetesEnvironment(ctx, &stale), models.ErrKubernetesEnvironmentConflict)
	require.NoError(t, deleter.DeleteKubernetesEnvironment(ctx, claim))
	require.NoError(t, repo.DeleteTaskEnvironment(ctx, "env-delete-kube"))
}

func TestKubernetesEnvironmentInterruptedOperationRecovery(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedRecoveryClaimEnvironment(t, repo, "task-interrupted", "env-interrupted")
	record, err := repo.ClaimKubernetesEnvironment(ctx, "env-interrupted", "task-interrupted", 1, "old-process")
	require.NoError(t, err)
	record.Metadata = map[string]interface{}{"pod_uid": "retained"}
	require.NoError(t, repo.SaveKubernetesEnvironment(ctx, record, false))
	recovery, ok := interface{}(repo).(interface{ RecoverInterruptedKubernetesOperations(context.Context) error })
	require.True(t, ok, "startup under exclusive runtime ownership must fence interrupted operations")
	require.NoError(t, recovery.RecoverInterruptedKubernetesOperations(ctx))
	require.ErrorIs(t, repo.SaveKubernetesEnvironment(ctx, record, true), models.ErrKubernetesEnvironmentConflict)
	resumed, err := repo.ClaimKubernetesEnvironment(ctx, record.EnvironmentID, record.TaskID, 1, "new-process")
	require.NoError(t, err)
	require.Equal(t, "retained", resumed.Metadata["pod_uid"])
}

func TestKubernetesEnvironmentInventoryListsWithoutSessions(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedRecoveryClaimEnvironment(t, repo, "task-retained", "env-retained")
	_, err := repo.ClaimKubernetesEnvironment(ctx, "env-retained", "task-retained", 1, "create")
	require.NoError(t, err)
	lister, ok := interface{}(repo).(interface {
		ListKubernetesEnvironments(context.Context) ([]*models.KubernetesEnvironment, error)
	})
	require.True(t, ok, "retained pods must be discoverable without a session row")
	records, err := lister.ListKubernetesEnvironments(ctx)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "env-retained", records[0].EnvironmentID)
}

func TestKubernetesEnvironmentInventorySurvivesTaskDeletion(t *testing.T) {
	testKubernetesEnvironmentInventorySurvivesTaskDeletion(t, newRepoForEntityTests(t))
}

func TestPostgresKubernetesEnvironmentInventorySurvivesTaskDeletion(t *testing.T) {
	repo, _, _ := newTaskPostgresRepoPair(t)
	testKubernetesEnvironmentInventorySurvivesTaskDeletion(t, repo)
}

func testKubernetesEnvironmentInventorySurvivesTaskDeletion(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	seedRecoveryClaimEnvironment(t, repo, "task-deleted-kube", "env-deleted-kube")
	record, err := repo.ClaimKubernetesEnvironment(ctx, "env-deleted-kube", "task-deleted-kube", 1, "create")
	require.NoError(t, err)
	record.Metadata = map[string]interface{}{"pod_uid": "retained"}
	require.NoError(t, repo.SaveKubernetesEnvironment(ctx, record, true))
	require.NoError(t, repo.DeleteTask(ctx, record.TaskID))
	retained, err := repo.GetKubernetesEnvironment(ctx, record.EnvironmentID)
	require.NoError(t, err)
	require.Equal(t, record.Metadata, retained.Metadata)
	_, err = repo.ClaimKubernetesEnvironment(ctx, record.EnvironmentID, record.TaskID, 1, "launch")
	require.Error(t, err, "deleted owners cannot launch")
	_, err = repo.ClaimKubernetesEnvironmentCleanup(ctx, record.EnvironmentID, record.TaskID, 2, "wrong-generation")
	require.Error(t, err)
	cleanup, err := repo.ClaimKubernetesEnvironmentCleanup(ctx, record.EnvironmentID, record.TaskID, 1, "delete")
	require.NoError(t, err)
	require.NoError(t, repo.SaveKubernetesEnvironment(ctx, cleanup, true))
	cleanup, err = repo.ClaimKubernetesEnvironmentCleanup(ctx, record.EnvironmentID, record.TaskID, 1, "retry")
	require.NoError(t, err)
	require.NoError(t, repo.DeleteKubernetesEnvironment(ctx, cleanup))
}
