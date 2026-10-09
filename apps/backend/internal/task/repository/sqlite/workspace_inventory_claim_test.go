package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	"github.com/kandev/kandev/internal/testutil"
)

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.6
func TestWorkspaceInventoryRepairHonorsEnvironmentClaim(t *testing.T) {
	testWorkspaceInventoryRepairHonorsEnvironmentClaim(t, newRepoForEntityTests(t))
}

func testWorkspaceInventoryRepairHonorsEnvironmentClaim(t *testing.T, repo *Repository) {
	t.Helper()
	env, taskRepo := seedWorkspaceInventoryRecovery(t, repo)
	ctx := context.Background()
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE task_environment_repos SET branch_slug = ? WHERE id = ?`), "stale", "environment-repository-recovery"); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.ListTaskEnvironmentRepos(ctx, env.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("load row: %v", err)
	}
	row := rows[0]
	repair := &models.WorkspaceInventoryRepair{
		TaskID: env.TaskID, WorkspaceID: "workspace-recovery", SessionID: "session-recovery", TaskEnvironmentID: env.ID,
		TaskRepositoryID: taskRepo.ID, RepositoryID: taskRepo.RepositoryID, EnvironmentRepoID: row.ID,
		BranchSlug: "main", WorktreeID: row.WorktreeID, WorktreePath: row.WorktreePath, WorktreeBranch: row.WorktreeBranch,
		IdempotencyKey: "claimed", RequestHash: "claimed-hash", ExpectedEnvironmentUpdatedAt: env.UpdatedAt,
		ExpectedTaskRepositoryUpdate: taskRepo.UpdatedAt, ExpectedEnvironmentRepoUpdate: row.UpdatedAt,
	}
	claim, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, recoveryClaimRequest(env.ID, env.TaskID, "session-recovery", "inventory-proof", env.OwnershipGeneration))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim) })
	if _, err := repo.RepairWorkspaceInventory(ctx, repair); !errors.Is(err, recoveryclaim.ErrBusy) {
		t.Fatalf("unclaimed repair accepted: %v", err)
	}
	receipt, err := repo.RepairWorkspaceInventory(recoveryclaim.WithClaim(ctx, claim), repair)
	if err != nil {
		t.Fatalf("claim owner rejected: %v", err)
	}
	if err := repo.RecordWorkspaceInventoryPostRepairAttestation(ctx, env.TaskID, receipt.IdempotencyKey, &receipt.Preservation, true, time.Now()); !errors.Is(err, recoveryclaim.ErrBusy) {
		t.Fatalf("unclaimed attestation accepted: %v", err)
	}
	if err := repo.RecordWorkspaceInventoryPostRepairAttestation(recoveryclaim.WithClaim(ctx, claim), env.TaskID, receipt.IdempotencyKey, &receipt.Preservation, true, time.Now()); err != nil {
		t.Fatalf("claim owner attestation rejected: %v", err)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.6
func TestPostgresWorkspaceInventoryRepairHonorsEnvironmentClaim(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	testWorkspaceInventoryRepairHonorsEnvironmentClaim(t, repo)
}
