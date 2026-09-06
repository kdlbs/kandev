package sqlite

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

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
	// Reload env and taskRepo from the DB rather than trusting the
	// nanosecond-precision in-memory timestamps CreateTaskEnvironment/
	// CreateTaskRepository just assigned: PostgreSQL's TIMESTAMP column only
	// stores microsecond precision, so an "expected updated_at" optimistic
	// -concurrency check built from the raw create-time value would never
	// match what a genuine repair caller (which always reloads the entity
	// before repairing it) observes back from the same column. Real launch
	// callers already reload every entity through a repository Get/List call
	// before capturing repair identity, so reloading here mirrors production
	// and lets every dialect compare like for like.
	reloadedEnv, err := repo.GetTaskEnvironment(ctx, env.ID)
	if err != nil {
		t.Fatal(err)
	}
	reloadedTaskRepo, err := repo.GetTaskRepository(ctx, taskRepo.ID)
	if err != nil {
		t.Fatal(err)
	}
	return reloadedEnv, reloadedTaskRepo
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

func TestWorkspaceInventoryRecoveryReceiptSurvivesSessionAndEnvironmentDeletion(t *testing.T) {
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
		IdempotencyKey: "preserve-audit", RequestHash: "preserve-audit-request",
		ExpectedEnvironmentUpdatedAt: env.UpdatedAt,
		ExpectedTaskRepositoryUpdate: taskRepo.UpdatedAt,
	})
	if err != nil {
		t.Fatalf("RepairWorkspaceInventory: %v", err)
	}

	if _, err := repo.db.ExecContext(ctx, `DELETE FROM task_sessions WHERE id = ?`, "session-recovery"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, `DELETE FROM task_environments WHERE id = ?`, env.ID); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := repo.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM workspace_inventory_recovery_receipts WHERE id = ?`, receipt.ID,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("receipt count after session and environment deletion = %d, want 1", count)
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

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.9
func TestGetWorkspaceInventoryRepairReceiptReturnsCommittedReceipt(t *testing.T) {
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

	missing, err := repo.GetWorkspaceInventoryRepairReceipt(ctx, "task-recovery", "no-such-key")
	if err != nil || missing != nil {
		t.Fatalf("lookup for missing key = %+v, %v", missing, err)
	}

	receipt, err := repo.RepairWorkspaceInventory(ctx, &models.WorkspaceInventoryRepair{
		TaskID: "task-recovery", WorkspaceID: "workspace-recovery", SessionID: "session-recovery",
		TaskEnvironmentID: env.ID, TaskRepositoryID: taskRepo.ID, RepositoryID: taskRepo.RepositoryID,
		EnvironmentRepoID: stale.ID, BranchSlug: "main",
		WorktreeID: stale.WorktreeID, WorktreePath: stale.WorktreePath,
		WorktreeBranch: stale.WorktreeBranch, Position: stale.Position,
		IdempotencyKey: "lookup-key", RequestHash: "request-hash",
		ExpectedEnvironmentUpdatedAt:  env.UpdatedAt,
		ExpectedTaskRepositoryUpdate:  taskRepo.UpdatedAt,
		ExpectedEnvironmentRepoUpdate: stale.UpdatedAt,
		Preservation: models.WorkspaceInventoryPreservation{
			HeadOID: "0123456789abcdef", StatusHash: "status", ContentHash: "content",
		},
	})
	if err != nil {
		t.Fatalf("RepairWorkspaceInventory: %v", err)
	}

	found, err := repo.GetWorkspaceInventoryRepairReceipt(ctx, "task-recovery", "lookup-key")
	if err != nil {
		t.Fatalf("GetWorkspaceInventoryRepairReceipt: %v", err)
	}
	if found == nil || found.ID != receipt.ID || found.ResultCode != models.WorkspaceInventoryRecoveryDeduplicated {
		t.Fatalf("found receipt = %+v, want deduplicated copy of %+v", found, receipt)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.10
func TestRecordWorkspaceInventoryPostRepairAttestationPersistsBeforeAndAfterEvidence(t *testing.T) {
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
		IdempotencyKey: "attestation-key", RequestHash: "request-hash",
		ExpectedEnvironmentUpdatedAt:  env.UpdatedAt,
		ExpectedTaskRepositoryUpdate:  taskRepo.UpdatedAt,
		ExpectedEnvironmentRepoUpdate: stale.UpdatedAt,
		Preservation: models.WorkspaceInventoryPreservation{
			HeadOID: "0123456789abcdef", StatusHash: "status", ContentHash: "content",
		},
	})
	if err != nil {
		t.Fatalf("RepairWorkspaceInventory: %v", err)
	}
	if receipt.PostRepairMatched || receipt.PostRepairEvidence != nil || receipt.PostRepairVerifiedAt != nil {
		t.Fatalf("receipt should have no post-repair attestation before it is recorded: %+v", receipt)
	}

	after := models.WorkspaceInventoryPreservation{
		HeadOID: "0123456789abcdef", StatusHash: "status", ContentHash: "content",
	}
	verifiedAt := time.Now().UTC().Truncate(time.Second)
	if err := repo.RecordWorkspaceInventoryPostRepairAttestation(ctx, "task-recovery", "attestation-key", &after, true, verifiedAt); err != nil {
		t.Fatalf("RecordWorkspaceInventoryPostRepairAttestation: %v", err)
	}

	found, err := repo.GetWorkspaceInventoryRepairReceipt(ctx, "task-recovery", "attestation-key")
	if err != nil {
		t.Fatalf("GetWorkspaceInventoryRepairReceipt: %v", err)
	}
	if found == nil || !found.PostRepairMatched || found.PostRepairEvidence == nil ||
		found.PostRepairEvidence.HeadOID != after.HeadOID || found.PostRepairVerifiedAt == nil ||
		!found.PostRepairVerifiedAt.Equal(verifiedAt) {
		t.Fatalf("post-repair attestation not persisted: %+v", found)
	}
	// The original before-repair preservation evidence remains intact
	// alongside the newly persisted after-repair evidence.
	if found.Preservation.HeadOID != receipt.Preservation.HeadOID {
		t.Fatalf("before-repair preservation evidence overwritten: %+v", found.Preservation)
	}

	// A divergent after-repair inspection is itself durable audit evidence,
	// not silently dropped.
	mismatch := models.WorkspaceInventoryPreservation{HeadOID: "divergent", StatusHash: "status", ContentHash: "content"}
	if err := repo.RecordWorkspaceInventoryPostRepairAttestation(ctx, "task-recovery", "attestation-key", &mismatch, false, verifiedAt.Add(time.Second)); err != nil {
		t.Fatalf("RecordWorkspaceInventoryPostRepairAttestation(mismatch): %v", err)
	}
	found, err = repo.GetWorkspaceInventoryRepairReceipt(ctx, "task-recovery", "attestation-key")
	if err != nil {
		t.Fatalf("GetWorkspaceInventoryRepairReceipt: %v", err)
	}
	if found.PostRepairMatched || found.PostRepairEvidence == nil || found.PostRepairEvidence.HeadOID != "divergent" {
		t.Fatalf("mismatch attestation not persisted: %+v", found)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.6
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.10
func TestRecordWorkspaceInventoryPostRepairAttestationCannotReplaceDivergenceWithMatch(t *testing.T) {
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

	if _, err := repo.RepairWorkspaceInventory(ctx, &models.WorkspaceInventoryRepair{
		TaskID: "task-recovery", WorkspaceID: "workspace-recovery", SessionID: "session-recovery",
		TaskEnvironmentID: env.ID, TaskRepositoryID: taskRepo.ID, RepositoryID: taskRepo.RepositoryID,
		EnvironmentRepoID: stale.ID, BranchSlug: "main",
		WorktreeID: stale.WorktreeID, WorktreePath: stale.WorktreePath,
		WorktreeBranch: stale.WorktreeBranch, Position: stale.Position,
		IdempotencyKey: "negative-attestation-key", RequestHash: "request-hash",
		ExpectedEnvironmentUpdatedAt:  env.UpdatedAt,
		ExpectedTaskRepositoryUpdate:  taskRepo.UpdatedAt,
		ExpectedEnvironmentRepoUpdate: stale.UpdatedAt,
		Preservation: models.WorkspaceInventoryPreservation{
			HeadOID: "before", StatusHash: "status", ContentHash: "content",
		},
	}); err != nil {
		t.Fatalf("RepairWorkspaceInventory: %v", err)
	}

	divergent := models.WorkspaceInventoryPreservation{
		HeadOID: "divergent", StatusHash: "status", ContentHash: "content",
	}
	verifiedAt := time.Now().UTC().Truncate(time.Second)
	if err := repo.RecordWorkspaceInventoryPostRepairAttestation(
		ctx, "task-recovery", "negative-attestation-key", &divergent, false, verifiedAt,
	); err != nil {
		t.Fatalf("record divergent attestation: %v", err)
	}

	matching := models.WorkspaceInventoryPreservation{
		HeadOID: "before", StatusHash: "status", ContentHash: "content",
	}
	if err := repo.RecordWorkspaceInventoryPostRepairAttestation(
		ctx, "task-recovery", "negative-attestation-key", &matching, true, verifiedAt.Add(time.Second),
	); !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
		t.Fatalf("replace divergent attestation error = %v, want recovery conflict", err)
	}

	found, err := repo.GetWorkspaceInventoryRepairReceipt(ctx, "task-recovery", "negative-attestation-key")
	if err != nil {
		t.Fatalf("GetWorkspaceInventoryRepairReceipt: %v", err)
	}
	if found == nil || found.PostRepairMatched || found.PostRepairEvidence == nil ||
		found.PostRepairEvidence.HeadOID != divergent.HeadOID || found.PostRepairVerifiedAt == nil ||
		!found.PostRepairVerifiedAt.Equal(verifiedAt) {
		t.Fatalf("divergent attestation was overwritten: %+v", found)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.2
//
// TestRepairWorkspaceInventoryReceiptPersistsSourceRecordRevisions proves the
// durable receipt carries forward the exact source-record revisions
// (task_environments.updated_at, task_repositories.updated_at,
// task_environment_repos.updated_at) the repair transaction proved and
// guarded against a concurrent writer, so an audit can verify which
// authoritative revisions were preserved without re-deriving them from the
// transient WorkspaceInventoryRepair request.
func TestRepairWorkspaceInventoryReceiptPersistsSourceRecordRevisions(t *testing.T) {
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
		t.Fatalf("load inventory: %+v, %v", rows, err)
	}
	row := rows[0]

	receipt, err := repo.RepairWorkspaceInventory(ctx, &models.WorkspaceInventoryRepair{
		TaskID: "task-recovery", WorkspaceID: "workspace-recovery", SessionID: "session-recovery",
		TaskEnvironmentID: env.ID, TaskRepositoryID: taskRepo.ID, RepositoryID: taskRepo.RepositoryID,
		EnvironmentRepoID: row.ID, BranchSlug: "main",
		WorktreeID: row.WorktreeID, WorktreePath: row.WorktreePath,
		WorktreeBranch: row.WorktreeBranch, Position: row.Position,
		IdempotencyKey: "revision-key", RequestHash: "revision-request-hash",
		ExpectedEnvironmentUpdatedAt:  env.UpdatedAt,
		ExpectedTaskRepositoryUpdate:  taskRepo.UpdatedAt,
		ExpectedEnvironmentRepoUpdate: row.UpdatedAt,
		Preservation: models.WorkspaceInventoryPreservation{
			HeadOID: "0123456789abcdef", StatusHash: "status", ContentHash: "content",
		},
	})
	if err != nil {
		t.Fatalf("RepairWorkspaceInventory: %v", err)
	}
	if !receipt.ExpectedEnvironmentUpdatedAt.Equal(env.UpdatedAt) ||
		!receipt.ExpectedTaskRepositoryUpdate.Equal(taskRepo.UpdatedAt) ||
		!receipt.ExpectedEnvironmentRepoUpdate.Equal(row.UpdatedAt) {
		t.Fatalf("receipt did not persist source-record revisions: %+v", receipt)
	}

	found, err := repo.GetWorkspaceInventoryRepairReceipt(ctx, "task-recovery", "revision-key")
	if err != nil {
		t.Fatalf("GetWorkspaceInventoryRepairReceipt: %v", err)
	}
	if !found.ExpectedEnvironmentUpdatedAt.Equal(env.UpdatedAt) ||
		!found.ExpectedTaskRepositoryUpdate.Equal(taskRepo.UpdatedAt) ||
		!found.ExpectedEnvironmentRepoUpdate.Equal(row.UpdatedAt) {
		t.Fatalf("reloaded receipt lost source-record revisions: %+v", found)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.3
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.9
//
// TestRepairWorkspaceInventoryConcurrentRetryConvergesToOneRepairAndOneDeduplicated
// races two RepairWorkspaceInventory calls for the SAME task-scoped
// idempotency key and identical request hash against the same stale row.
// Exactly one call must observe the atomic transaction as the original
// repair; the other must observe its own committed receipt and return a
// deduplicated copy — never a second repair, a duplicated canonical row, or
// a duplicated receipt.
func TestRepairWorkspaceInventoryConcurrentRetryConvergesToOneRepairAndOneDeduplicated(t *testing.T) {
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
		t.Fatalf("load inventory: %+v, %v", rows, err)
	}
	row := rows[0]

	buildRepair := func() *models.WorkspaceInventoryRepair {
		return &models.WorkspaceInventoryRepair{
			TaskID: "task-recovery", WorkspaceID: "workspace-recovery", SessionID: "session-recovery",
			TaskEnvironmentID: env.ID, TaskRepositoryID: taskRepo.ID, RepositoryID: taskRepo.RepositoryID,
			EnvironmentRepoID: row.ID, BranchSlug: "main",
			WorktreeID: row.WorktreeID, WorktreePath: row.WorktreePath,
			WorktreeBranch: row.WorktreeBranch, Position: row.Position,
			IdempotencyKey: "concurrent-repair-key", RequestHash: "concurrent-request-hash",
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
	if err := repo.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM workspace_inventory_recovery_receipts WHERE idempotency_key = ?`,
		"concurrent-repair-key",
	).Scan(&receiptRowCount); err != nil {
		t.Fatal(err)
	}
	if receiptRowCount != 1 {
		t.Fatalf("receipt row count = %d, want exactly 1", receiptRowCount)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.5
//
// TestRepairWorkspaceInventoryRejectsCrossScopeAndTerminalStates proves each
// non-reciprocal or terminal server-owned record state is refused with the
// stable typed conflict error rather than silently repairing, leaking which
// specific check failed, or partially mutating state.
func TestRepairWorkspaceInventoryRejectsCrossScopeAndTerminalStates(t *testing.T) {
	baseline := func(t *testing.T) (*Repository, *models.TaskEnvironment, *models.TaskRepository, *models.TaskEnvironmentRepo) {
		t.Helper()
		repo := newRepoForEntityTests(t)
		env, taskRepo := seedWorkspaceInventoryRecovery(t, repo)
		ctx := context.Background()
		// Move the canonical row off its target branch_slug first so the
		// repair's occupied-slot guard does not short-circuit before the
		// scope/terminal-state check under test ever runs, mirroring
		// TestRepairWorkspaceInventoryCorrectsOnlyTheProvenStaleSlot.
		if _, err := repo.db.ExecContext(ctx,
			`UPDATE task_environment_repos SET branch_slug = ? WHERE id = ?`,
			"stale", "environment-repository-recovery",
		); err != nil {
			t.Fatal(err)
		}
		rows, err := repo.ListTaskEnvironmentRepos(ctx, env.ID)
		if err != nil || len(rows) != 1 {
			t.Fatalf("load inventory: %+v, %v", rows, err)
		}
		return repo, env, taskRepo, rows[0]
	}
	buildRepair := func(env *models.TaskEnvironment, taskRepo *models.TaskRepository, row *models.TaskEnvironmentRepo) *models.WorkspaceInventoryRepair {
		return &models.WorkspaceInventoryRepair{
			TaskID: "task-recovery", WorkspaceID: "workspace-recovery", SessionID: "session-recovery",
			TaskEnvironmentID: env.ID, TaskRepositoryID: taskRepo.ID, RepositoryID: taskRepo.RepositoryID,
			EnvironmentRepoID: row.ID, BranchSlug: "main",
			WorktreeID: row.WorktreeID, WorktreePath: row.WorktreePath,
			WorktreeBranch: row.WorktreeBranch, Position: row.Position,
			IdempotencyKey: "cross-scope-key", RequestHash: "cross-scope-request-hash",
			ExpectedEnvironmentUpdatedAt:  env.UpdatedAt,
			ExpectedTaskRepositoryUpdate:  taskRepo.UpdatedAt,
			ExpectedEnvironmentRepoUpdate: row.UpdatedAt,
			Preservation: models.WorkspaceInventoryPreservation{
				HeadOID: "0123456789abcdef", StatusHash: "status", ContentHash: "content",
			},
		}
	}

	t.Run("cross-workspace repository", func(t *testing.T) {
		repo, env, taskRepo, row := baseline(t)
		ctx := context.Background()
		if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-other", Name: "other"}); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.db.ExecContext(ctx,
			`UPDATE repositories SET workspace_id = ? WHERE id = ?`, "workspace-other", taskRepo.RepositoryID,
		); err != nil {
			t.Fatal(err)
		}
		repair := buildRepair(env, taskRepo, row)
		if _, err := repo.RepairWorkspaceInventory(ctx, repair); !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
			t.Fatalf("cross-workspace repository error = %v, want conflict", err)
		}
	})

	t.Run("deleted repository", func(t *testing.T) {
		repo, env, taskRepo, row := baseline(t)
		ctx := context.Background()
		if _, err := repo.db.ExecContext(ctx,
			`UPDATE repositories SET deleted_at = ? WHERE id = ?`, time.Now().UTC(), taskRepo.RepositoryID,
		); err != nil {
			t.Fatal(err)
		}
		repair := buildRepair(env, taskRepo, row)
		if _, err := repo.RepairWorkspaceInventory(ctx, repair); !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
			t.Fatalf("deleted repository error = %v, want conflict", err)
		}
	})

	t.Run("failed task environment", func(t *testing.T) {
		repo, env, taskRepo, row := baseline(t)
		ctx := context.Background()
		if _, err := repo.db.ExecContext(ctx,
			`UPDATE task_environments SET status = ? WHERE id = ?`, string(models.TaskEnvironmentStatusFailed), env.ID,
		); err != nil {
			t.Fatal(err)
		}
		repair := buildRepair(env, taskRepo, row)
		if _, err := repo.RepairWorkspaceInventory(ctx, repair); !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
			t.Fatalf("failed task environment error = %v, want conflict", err)
		}
	})

	t.Run("wrong environment session", func(t *testing.T) {
		repo, env, taskRepo, row := baseline(t)
		ctx := context.Background()
		if err := repo.CreateTask(ctx, &models.Task{ID: "task-recovery-wrong-env", WorkspaceID: "workspace-recovery", Title: "Wrong Env"}); err != nil {
			t.Fatal(err)
		}
		other := &models.TaskEnvironment{
			ID: "environment-recovery-other", TaskID: "task-recovery-wrong-env",
			ExecutorType: string(models.ExecutorTypeWorktree), Status: models.TaskEnvironmentStatusReady,
			WorkspacePath: "/synthetic/other",
			Repos: []*models.TaskEnvironmentRepo{{
				RepositoryID: taskRepo.RepositoryID, BranchSlug: "main",
				WorktreeID: "worktree-other", WorktreePath: "/synthetic/other",
				WorktreeBranch: "main",
			}},
		}
		if err := repo.CreateTaskEnvironment(ctx, other); err != nil {
			t.Fatal(err)
		}
		// The session still belongs to task-recovery, but now points at a
		// different task's environment: the repair targets env.ID while the
		// session's actual environment is other.ID.
		if _, err := repo.db.ExecContext(ctx,
			`UPDATE task_sessions SET task_environment_id = ? WHERE id = ?`, other.ID, "session-recovery",
		); err != nil {
			t.Fatal(err)
		}
		repair := buildRepair(env, taskRepo, row)
		if _, err := repo.RepairWorkspaceInventory(ctx, repair); !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
			t.Fatalf("wrong-environment session error = %v, want conflict", err)
		}
	})

	t.Run("cross-task session", func(t *testing.T) {
		repo, env, taskRepo, row := baseline(t)
		ctx := context.Background()
		if err := repo.CreateTask(ctx, &models.Task{ID: "task-recovery-other", WorkspaceID: "workspace-recovery", Title: "Other"}); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.db.ExecContext(ctx,
			`UPDATE task_sessions SET task_id = ? WHERE id = ?`, "task-recovery-other", "session-recovery",
		); err != nil {
			t.Fatal(err)
		}
		repair := buildRepair(env, taskRepo, row)
		if _, err := repo.RepairWorkspaceInventory(ctx, repair); !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
			t.Fatalf("cross-task session error = %v, want conflict", err)
		}
	})

	t.Run("deleted environment repo row", func(t *testing.T) {
		repo, env, taskRepo, row := baseline(t)
		ctx := context.Background()
		if _, err := repo.db.ExecContext(ctx,
			`UPDATE task_environment_repos SET deleted_at = ? WHERE id = ?`, time.Now().UTC(), row.ID,
		); err != nil {
			t.Fatal(err)
		}
		repair := buildRepair(env, taskRepo, row)
		if _, err := repo.RepairWorkspaceInventory(ctx, repair); !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
			t.Fatalf("deleted environment repo row error = %v, want conflict", err)
		}
	})

	t.Run("failed environment repo row", func(t *testing.T) {
		repo, env, taskRepo, row := baseline(t)
		ctx := context.Background()
		if _, err := repo.db.ExecContext(ctx,
			`UPDATE task_environment_repos SET status = ? WHERE id = ?`, workspaceInventoryRepoStatusFailed, row.ID,
		); err != nil {
			t.Fatal(err)
		}
		repair := buildRepair(env, taskRepo, row)
		if _, err := repo.RepairWorkspaceInventory(ctx, repair); !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
			t.Fatalf("failed environment repo row error = %v, want conflict", err)
		}
	})
}
