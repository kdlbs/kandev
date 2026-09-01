package sqlite

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.4
func TestWorkspaceInventoryRecoveryReceiptSchemaExists(t *testing.T) {
	repo := newRepoForEntityTests(t)

	var count int
	err := repo.db.QueryRowContext(context.Background(),
		`SELECT COUNT(1) FROM workspace_inventory_recovery_receipts`,
	).Scan(&count)
	if err != nil {
		t.Fatalf("query workspace inventory recovery receipts: %v", err)
	}
}

func seedWorkspaceInventoryRecovery(t *testing.T, repo *Repository) (*models.TaskEnvironment, *models.TaskRepository) {
	t.Helper()
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-recovery")
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repository-recovery", WorkspaceID: "workspace-recovery", Name: "repo",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-recovery", WorkspaceID: "workspace-recovery", Title: "Recovery",
	}); err != nil {
		t.Fatal(err)
	}
	taskRepo := &models.TaskRepository{
		ID: "task-repository-recovery", TaskID: "task-recovery",
		RepositoryID: "repository-recovery", BaseBranch: "main", Position: 0,
	}
	if err := repo.CreateTaskRepository(ctx, taskRepo); err != nil {
		t.Fatal(err)
	}
	env := &models.TaskEnvironment{
		ID: "environment-recovery", TaskID: "task-recovery",
		ExecutorType: string(models.ExecutorTypeWorktree), Status: models.TaskEnvironmentStatusReady,
		WorkspacePath: "/synthetic/worktree",
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "environment-repository-recovery", RepositoryID: "repository-recovery",
			BranchSlug: "main", WorktreeID: "worktree-recovery",
			WorktreePath: "/synthetic/worktree", WorktreeBranch: "feature/recovery",
		}},
	}
	if err := repo.CreateTaskEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-recovery", TaskID: "task-recovery", TaskEnvironmentID: env.ID,
		State: models.TaskSessionStateFailed,
	}); err != nil {
		t.Fatal(err)
	}
	return env, taskRepo
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.2
func TestRepairWorkspaceInventoryInsertsMissingRowAndReceipt(t *testing.T) {
	repo := newRepoForEntityTests(t)
	env, taskRepo := seedWorkspaceInventoryRecovery(t, repo)
	ctx := context.Background()
	if _, err := repo.db.ExecContext(ctx,
		`DELETE FROM task_environment_repos WHERE id = ?`, "environment-repository-recovery",
	); err != nil {
		t.Fatal(err)
	}

	receipt, err := repo.RepairWorkspaceInventory(ctx, &models.WorkspaceInventoryRepair{
		TaskID: "task-recovery", WorkspaceID: "workspace-recovery", SessionID: "session-recovery",
		TaskEnvironmentID: env.ID, TaskRepositoryID: taskRepo.ID, RepositoryID: taskRepo.RepositoryID,
		EnvironmentRepoID: "environment-repository-repaired", BranchSlug: "main",
		WorktreeID: "worktree-recovery", WorktreePath: "/synthetic/worktree",
		WorktreeBranch: "feature/recovery", Position: 0,
		IdempotencyKey: "repair-once", RequestHash: "request-hash",
		ExpectedEnvironmentUpdatedAt: env.UpdatedAt,
		ExpectedTaskRepositoryUpdate: taskRepo.UpdatedAt,
		Preservation: models.WorkspaceInventoryPreservation{
			HeadOID: "0123456789abcdef", StatusHash: "status", ContentHash: "content",
		},
	})
	if err != nil {
		t.Fatalf("RepairWorkspaceInventory: %v", err)
	}
	if receipt.ResultCode != models.WorkspaceInventoryRecoveryRepaired {
		t.Fatalf("result code = %q", receipt.ResultCode)
	}
	rows, err := repo.ListTaskEnvironmentRepos(ctx, env.ID)
	if err != nil || len(rows) != 1 || rows[0].ID != "environment-repository-repaired" {
		t.Fatalf("inventory after repair = %+v, %v", rows, err)
	}

	deduplicated, err := repo.RepairWorkspaceInventory(ctx, &models.WorkspaceInventoryRepair{
		TaskID: "task-recovery", IdempotencyKey: "repair-once", RequestHash: "request-hash",
	})
	if err != nil || deduplicated.ID != receipt.ID || deduplicated.ResultCode != models.WorkspaceInventoryRecoveryDeduplicated {
		t.Fatalf("deduplicated receipt = %+v, %v", deduplicated, err)
	}
	_, err = repo.RepairWorkspaceInventory(ctx, &models.WorkspaceInventoryRepair{
		TaskID: "task-recovery", IdempotencyKey: "repair-once", RequestHash: "different",
	})
	if !errors.Is(err, models.ErrWorkspaceInventoryRecoveryIdempotencyConflict) {
		t.Fatalf("different payload error = %v", err)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.3
func TestRepairWorkspaceInventoryCorrectsOnlyTheProvenStaleSlot(t *testing.T) {
	repo := newRepoForEntityTests(t)
	env, taskRepo := seedWorkspaceInventoryRecovery(t, repo)
	ctx := context.Background()
	if _, err := repo.db.ExecContext(ctx,
		`UPDATE task_environment_repos SET branch_slug = ? WHERE id = ?`,
		"stale", "environment-repository-recovery",
	); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.ListTaskEnvironmentRepos(ctx, env.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("load stale inventory: %+v, %v", rows, err)
	}
	stale := rows[0]

	receipt, err := repo.RepairWorkspaceInventory(ctx, &models.WorkspaceInventoryRepair{
		TaskID: "task-recovery", WorkspaceID: "workspace-recovery", SessionID: "session-recovery",
		TaskEnvironmentID: env.ID, TaskRepositoryID: taskRepo.ID, RepositoryID: taskRepo.RepositoryID,
		EnvironmentRepoID: stale.ID, BranchSlug: "main",
		WorktreeID: stale.WorktreeID, WorktreePath: stale.WorktreePath,
		WorktreeBranch: stale.WorktreeBranch, Position: stale.Position,
		IdempotencyKey: "repair-stale", RequestHash: "request-hash-stale",
		ExpectedEnvironmentUpdatedAt:  env.UpdatedAt,
		ExpectedTaskRepositoryUpdate:  taskRepo.UpdatedAt,
		ExpectedEnvironmentRepoUpdate: stale.UpdatedAt,
		Preservation: models.WorkspaceInventoryPreservation{
			HeadOID: "0123456789abcdef", StatusHash: "status", ContentHash: "content",
		},
	})
	if err != nil {
		t.Fatalf("RepairWorkspaceInventory(stale): %v", err)
	}
	if receipt.ResultCode != models.WorkspaceInventoryRecoveryRepaired {
		t.Fatalf("result code = %q", receipt.ResultCode)
	}
	after, err := repo.ListTaskEnvironmentRepos(ctx, env.ID)
	if err != nil || len(after) != 1 {
		t.Fatalf("inventory after repair = %+v, %v", after, err)
	}
	if after[0].BranchSlug != "main" || after[0].WorktreeID != stale.WorktreeID ||
		after[0].WorktreePath != stale.WorktreePath || after[0].WorktreeBranch != stale.WorktreeBranch {
		t.Fatalf("physical identity changed: before=%+v after=%+v", stale, after[0])
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.4
func TestRepairWorkspaceInventoryRollsBackReceiptWhenMetadataWriteFails(t *testing.T) {
	repo := newRepoForEntityTests(t)
	env, taskRepo := seedWorkspaceInventoryRecovery(t, repo)
	ctx := context.Background()

	_, err := repo.RepairWorkspaceInventory(ctx, &models.WorkspaceInventoryRepair{
		TaskID: "task-recovery", WorkspaceID: "workspace-recovery", SessionID: "session-recovery",
		TaskEnvironmentID: env.ID, TaskRepositoryID: taskRepo.ID, RepositoryID: taskRepo.RepositoryID,
		// Deliberately present the stale row ID as a missing-row insert. The
		// primary-key failure happens after the receipt insert inside the same transaction.
		EnvironmentRepoID: "environment-repository-recovery", BranchSlug: "other",
		WorktreeID: "worktree-recovery", WorktreePath: "/synthetic/worktree",
		WorktreeBranch: "feature/recovery", Position: 0,
		IdempotencyKey: "rollback-receipt", RequestHash: "rollback-request",
		ExpectedEnvironmentUpdatedAt: env.UpdatedAt,
		ExpectedTaskRepositoryUpdate: taskRepo.UpdatedAt,
	})
	if err == nil {
		t.Fatal("expected metadata insert failure")
	}
	var receiptCount int
	if err := repo.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM workspace_inventory_recovery_receipts WHERE idempotency_key = ?`,
		"rollback-receipt",
	).Scan(&receiptCount); err != nil {
		t.Fatal(err)
	}
	if receiptCount != 0 {
		t.Fatalf("receipt count after rollback = %d", receiptCount)
	}
}

func TestWorkspaceInventoryRecoveryRepositoryExposesAtomicRepair(t *testing.T) {
	repo := newRepoForEntityTests(t)

	if _, ok := reflect.TypeOf(repo).MethodByName("RepairWorkspaceInventory"); !ok {
		t.Fatal("Repository does not expose RepairWorkspaceInventory")
	}
}
