package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresWorkspaceInventoryNegativeAttestationIsMonotonic pins the
// boolean predicate that prevents a concurrent positive observation from
// replacing already-durable divergent evidence. It skips unless
// KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresWorkspaceInventoryNegativeAttestationIsMonotonic(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	env, taskRepo := seedWorkspaceInventoryRecovery(t, repo)
	ctx := context.Background()
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE task_environment_repos SET branch_slug = ? WHERE id = ?`,
	), "stale", "environment-repository-recovery"); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.ListTaskEnvironmentRepos(ctx, env.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("load stale inventory: %+v, %v", rows, err)
	}
	row := rows[0]

	if _, err := repo.RepairWorkspaceInventory(ctx, &models.WorkspaceInventoryRepair{
		TaskID: "task-recovery", WorkspaceID: "workspace-recovery", SessionID: "session-recovery",
		TaskEnvironmentID: env.ID, TaskRepositoryID: taskRepo.ID, RepositoryID: taskRepo.RepositoryID,
		EnvironmentRepoID: row.ID, BranchSlug: "main",
		WorktreeID: row.WorktreeID, WorktreePath: row.WorktreePath,
		WorktreeBranch: row.WorktreeBranch, Position: row.Position,
		IdempotencyKey: "postgres-negative-attestation", RequestHash: "request-hash",
		ExpectedEnvironmentUpdatedAt:  env.UpdatedAt,
		ExpectedTaskRepositoryUpdate:  taskRepo.UpdatedAt,
		ExpectedEnvironmentRepoUpdate: row.UpdatedAt,
		Preservation:                  models.WorkspaceInventoryPreservation{HeadOID: "before"},
	}); err != nil {
		t.Fatalf("RepairWorkspaceInventory: %v", err)
	}

	verifiedAt := time.Now().UTC().Truncate(time.Microsecond)
	divergent := &models.WorkspaceInventoryPreservation{HeadOID: "divergent"}
	if err := repo.RecordWorkspaceInventoryPostRepairAttestation(
		ctx, "task-recovery", "postgres-negative-attestation", divergent, false, verifiedAt,
	); err != nil {
		t.Fatalf("record divergent attestation: %v", err)
	}
	if err := repo.RecordWorkspaceInventoryPostRepairAttestation(
		ctx, "task-recovery", "postgres-negative-attestation",
		&models.WorkspaceInventoryPreservation{HeadOID: "before"}, true, verifiedAt.Add(time.Second),
	); !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
		t.Fatalf("replace divergent attestation error = %v, want recovery conflict", err)
	}

	found, err := repo.GetWorkspaceInventoryRepairReceipt(ctx, "task-recovery", "postgres-negative-attestation")
	if err != nil {
		t.Fatal(err)
	}
	if found == nil || found.PostRepairMatched || found.PostRepairEvidence == nil ||
		found.PostRepairEvidence.HeadOID != divergent.HeadOID {
		t.Fatalf("divergent postgres attestation was overwritten: %+v", found)
	}
}
