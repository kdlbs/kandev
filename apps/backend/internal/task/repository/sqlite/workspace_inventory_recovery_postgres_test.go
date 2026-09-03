package sqlite

import (
	"context"
	"errors"
	"sync"
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

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.7
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.9
//
// TestPostgresRepairWorkspaceInventoryConcurrentSameKeyRetryConvergesToOneRepairAndOneDeduplicated
// pins the ordering fix directly on PostgreSQL, where lockTaskRowInTx takes
// a real SELECT ... FOR UPDATE lock (a no-op on SQLite). Racing the SAME
// idempotency key and request hash against the same stale row must still
// converge to exactly one repaired result and every other caller observing
// its own committed receipt as deduplicated — never an occupied-slot
// conflict from two transactions that both observed "no receipt" before
// either committed, which is exactly what checking the receipt before
// locking the task row would allow. It skips unless
// KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresRepairWorkspaceInventoryConcurrentSameKeyRetryConvergesToOneRepairAndOneDeduplicated(t *testing.T) {
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

	buildRepair := func() *models.WorkspaceInventoryRepair {
		return &models.WorkspaceInventoryRepair{
			TaskID: "task-recovery", WorkspaceID: "workspace-recovery", SessionID: "session-recovery",
			TaskEnvironmentID: env.ID, TaskRepositoryID: taskRepo.ID, RepositoryID: taskRepo.RepositoryID,
			EnvironmentRepoID: row.ID, BranchSlug: "main",
			WorktreeID: row.WorktreeID, WorktreePath: row.WorktreePath,
			WorktreeBranch: row.WorktreeBranch, Position: row.Position,
			IdempotencyKey: "postgres-concurrent-repair-key", RequestHash: "postgres-concurrent-request-hash",
			ExpectedEnvironmentUpdatedAt:  env.UpdatedAt,
			ExpectedTaskRepositoryUpdate:  taskRepo.UpdatedAt,
			ExpectedEnvironmentRepoUpdate: row.UpdatedAt,
			Preservation: models.WorkspaceInventoryPreservation{
				HeadOID: "0123456789abcdef", StatusHash: "status", ContentHash: "content",
			},
		}
	}

	const attempts = 8
	receipts := make([]*models.WorkspaceInventoryRecoveryReceipt, attempts)
	errs := make([]error, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			receipts[i], errs[i] = repo.RepairWorkspaceInventory(ctx, buildRepair())
		}(i)
	}
	wg.Wait()

	repairedCount, deduplicatedCount := 0, 0
	var firstID string
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent RepairWorkspaceInventory[%d]: %v", i, err)
		}
		switch receipts[i].ResultCode {
		case models.WorkspaceInventoryRecoveryRepaired:
			repairedCount++
		case models.WorkspaceInventoryRecoveryDeduplicated:
			deduplicatedCount++
		default:
			t.Fatalf("unexpected result code[%d] = %q", i, receipts[i].ResultCode)
		}
		if firstID == "" {
			firstID = receipts[i].ID
		} else if receipts[i].ID != firstID {
			t.Fatalf("concurrent repairs produced divergent receipt IDs: %q vs %q", firstID, receipts[i].ID)
		}
	}
	if repairedCount != 1 {
		t.Fatalf("repaired count = %d, want exactly 1 (attempts=%d)", repairedCount, attempts)
	}
	if deduplicatedCount != attempts-1 {
		t.Fatalf("deduplicated count = %d, want %d", deduplicatedCount, attempts-1)
	}

	after, err := repo.ListTaskEnvironmentRepos(ctx, env.ID)
	if err != nil || len(after) != 1 {
		t.Fatalf("canonical inventory duplicated by concurrent repair: %+v, %v", after, err)
	}
	var receiptRowCount int
	if err := repo.db.QueryRowContext(ctx, repo.db.Rebind(
		`SELECT COUNT(1) FROM workspace_inventory_recovery_receipts WHERE idempotency_key = ?`,
	), "postgres-concurrent-repair-key").Scan(&receiptRowCount); err != nil {
		t.Fatal(err)
	}
	if receiptRowCount != 1 {
		t.Fatalf("receipt row count = %d, want exactly 1", receiptRowCount)
	}
}
