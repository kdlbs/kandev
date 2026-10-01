package executor

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	kandevdb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type inventoryClaimRepository struct {
	*mockRepository
	claims       *tasksqlite.Repository
	beforeRepair func(context.Context)
}

func (r *inventoryClaimRepository) AcquireTaskEnvironmentRecoveryClaim(ctx context.Context, req models.TaskEnvironmentRecoveryClaimRequest) (*models.TaskEnvironmentRecoveryClaim, error) {
	return r.claims.AcquireTaskEnvironmentRecoveryClaim(ctx, req)
}
func (r *inventoryClaimRepository) ReleaseTaskEnvironmentRecoveryClaim(ctx context.Context, claim *models.TaskEnvironmentRecoveryClaim) error {
	return r.claims.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim)
}
func (r *inventoryClaimRepository) RepairWorkspaceInventory(ctx context.Context, repair *models.WorkspaceInventoryRepair) (*models.WorkspaceInventoryRecoveryReceipt, error) {
	if r.beforeRepair != nil {
		r.beforeRepair(ctx)
	}
	return r.mockRepository.RepairWorkspaceInventory(ctx, repair)
}
func inventoryClaimFixture(t *testing.T) (*inventoryClaimRepository, *Executor, *models.TaskSession) {
	t.Helper()
	mock, _, session := inventoryAdmissionFixture(t)
	conn, err := kandevdb.OpenSQLite(filepath.Join(t.TempDir(), "inventory.db"))
	if err != nil {
		t.Fatal(err)
	}
	db := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	store, err := tasksqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.CreateWorkspace(ctx, &models.Workspace{ID: "review-workspace", Name: "Inventory"}); err != nil {
		t.Fatal(err)
	}
	for _, task := range mock.tasks {
		if err := store.CreateTask(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	env := *mock.taskEnvironments[session.TaskEnvironmentID]
	env.Repos = nil
	if err := store.CreateTaskEnvironment(ctx, &env); err != nil {
		t.Fatal(err)
	}
	mock.taskEnvironments[env.ID].OwnershipGeneration = env.OwnershipGeneration
	if err := store.CreateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	repo := &inventoryClaimRepository{mockRepository: mock, claims: store}
	return repo, newTestExecutor(t, &mockAgentManager{}, repo), session
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.6
func TestInventoryRepairSerializesInheritedWriterAdmission(t *testing.T) {
	repo, executor, session := inventoryClaimFixture(t)
	ctx := context.Background()
	child := &models.Task{ID: "child", WorkspaceID: "review-workspace", Title: "Child"}
	if err := repo.claims.CreateTask(ctx, child); err != nil {
		t.Fatal(err)
	}
	borrower := &models.TaskSession{ID: "borrower", TaskID: child.ID, TaskEnvironmentID: session.TaskEnvironmentID, State: models.TaskSessionStateRunning}
	attempted := false
	repo.beforeRepair = func(ctx context.Context) {
		attempted = true
		// This transaction is the writer-admission boundary used by child launches.
		result := make(chan error, 1)
		go func() { result <- repo.claims.CreateTaskSession(context.Background(), borrower) }()
		if err := <-result; !errors.Is(err, recoveryclaim.ErrBusy) {
			t.Errorf("writer admitted during repair: %v", err)
		}
	}
	_, _, _, _, _, err := executor.buildResumeRequestAtCredentialBoundaryWithOptions(ctx, repo.tasks[session.TaskID].ToAPI(), session, false, nil,
		ResumeOptions{RepairWorkspaceInventory: true, WorkspaceInventoryIdempotencyKey: "serialized"})
	if err != nil {
		t.Fatal(err)
	}
	if !attempted {
		t.Fatal("repair transaction not reached")
	}
	if err := repo.claims.CreateTaskSession(ctx, borrower); err != nil {
		t.Fatalf("writer blocked after claim release: %v", err)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.6
func TestInventoryRepairRejectsDurableInheritedWriter(t *testing.T) {
	for _, runtimeOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "active_session", true: "orphan_runtime"}[runtimeOnly], func(t *testing.T) {
			repo, executor, session := inventoryClaimFixture(t)
			ctx := context.Background()
			if err := repo.claims.CreateTask(ctx, &models.Task{ID: "child", WorkspaceID: "review-workspace", Title: "Child"}); err != nil {
				t.Fatal(err)
			}
			state := models.TaskSessionStateRunning
			if runtimeOnly {
				state = models.TaskSessionStateFailed
			}
			if err := repo.claims.CreateTaskSession(ctx, &models.TaskSession{ID: "borrower", TaskID: "child", TaskEnvironmentID: session.TaskEnvironmentID, State: state}); err != nil {
				t.Fatal(err)
			}
			if runtimeOnly {
				if err := repo.claims.UpsertExecutorRunning(ctx, &models.ExecutorRunning{TaskID: "child", SessionID: "borrower", Status: models.ExecutorRunningStatusRunning}); err != nil {
					t.Fatal(err)
				}
			}
			_, _, _, _, _, err := executor.buildResumeRequestAtCredentialBoundaryWithOptions(ctx, repo.tasks[session.TaskID].ToAPI(), session, false, nil,
				ResumeOptions{RepairWorkspaceInventory: true, WorkspaceInventoryIdempotencyKey: "busy"})
			if !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
				t.Fatalf("expected writer conflict, got %v", err)
			}
			if len(repo.workspaceInventoryReceipts) != 0 {
				t.Fatal("repair committed beside active writer")
			}
		})
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.6
func TestInventoryRepairReleasesClaimAfterCancellation(t *testing.T) {
	repo, executor, session := inventoryClaimFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repo.beforeRepair = func(context.Context) { cancel() }
	_, _, _, _, _, err := executor.buildResumeRequestAtCredentialBoundaryWithOptions(ctx, repo.tasks[session.TaskID].ToAPI(), session, false, nil,
		ResumeOptions{RepairWorkspaceInventory: true, WorkspaceInventoryIdempotencyKey: "cancelled"})
	if !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
		t.Fatalf("cancelled repair admitted: %v", err)
	}
	if err := repo.claims.CreateTaskSession(context.Background(), &models.TaskSession{
		ID: "after-cancel", TaskID: session.TaskID, TaskEnvironmentID: session.TaskEnvironmentID, State: models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatalf("claim leaked after cancellation: %v", err)
	}
}
