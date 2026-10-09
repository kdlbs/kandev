package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPluginExecutorPostgresInventoryCAS(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	seedPostgresPluginExecutorInventory(t, repo)
	first := pluginExecutorRunningFixture("execution-pg-one", 7, "allocating")
	first.TaskID = "task-pg-plugin-executor-inventory"
	if err := repo.CheckpointPluginExecutorInventory(ctx, first); err != nil {
		t.Fatalf("initial checkpoint: %v", err)
	}
	if err := repo.UpdateResumeToken(ctx, first.SessionID, first.AgentExecutionID, "resume-pg", "message-pg"); err != nil {
		t.Fatalf("UpdateResumeToken: %v", err)
	}

	secondDB := openSecondPostgresConnection(t, dsn, db)
	secondRepo, err := NewWithDB(secondDB, secondDB, nil)
	if err != nil {
		t.Fatalf("init second postgres repo: %v", err)
	}
	var group sync.WaitGroup
	errs := make(chan error, 2)
	group.Add(2)
	go func() {
		defer group.Done()
		checkpoint := pluginExecutorRunningFixture(first.AgentExecutionID, 7, "provisioned")
		checkpoint.TaskID = first.TaskID
		checkpoint.ExpectedPluginExecutorRevision = first.ExpectedPluginExecutorRevision
		errs <- secondRepo.CheckpointPluginExecutorInventory(ctx, checkpoint)
	}()
	go func() {
		defer group.Done()
		errs <- repo.UpdateResumeToken(ctx, first.SessionID, first.AgentExecutionID, "resume-pg-race", "message-pg-race")
	}()
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent inventory/resume update: %v", err)
		}
	}
	stored, err := repo.GetExecutorRunningBySessionID(ctx, first.SessionID)
	if err != nil {
		t.Fatalf("GetExecutorRunningBySessionID: %v", err)
	}
	if stored.AgentExecutionID != first.AgentExecutionID || stored.ResumeToken != "resume-pg-race" || stored.LastMessageUUID != "message-pg-race" {
		t.Fatalf("plugin checkpoint changed lifecycle-owned resume fields: %+v", stored)
	}

	staleExecution := pluginExecutorRunningFixture("execution-pg-stale", 7, "stale")
	staleExecution.TaskID = first.TaskID
	if err := repo.CheckpointPluginExecutorInventory(ctx, staleExecution); !errors.Is(err, models.ErrExecutionRotated) {
		t.Fatalf("stale execution checkpoint error = %v, want ErrExecutionRotated", err)
	}
	staleOwner := pluginExecutorRunningFixture(first.AgentExecutionID, 6, "stale-owner")
	staleOwner.TaskID = first.TaskID
	if err := repo.CheckpointPluginExecutorInventory(ctx, staleOwner); !errors.Is(err, models.ErrExecutionRotated) {
		t.Fatalf("stale ownership checkpoint error = %v, want ErrExecutionRotated", err)
	}
}

func TestPluginExecutorPostgresCleanupClaim(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	seedPostgresPluginExecutorInventory(t, repo)
	ctx := context.Background()
	running := pluginExecutorRunningFixture("execution-pg-cleanup", 7, "allocating")
	running.TaskID = "task-pg-plugin-executor-inventory"
	if err := repo.CheckpointPluginExecutorInventory(ctx, running); err != nil {
		t.Fatalf("checkpoint allocation intent: %v", err)
	}
	envelope, ok := running.Metadata["plugin_executor"].(map[string]interface{})
	if !ok {
		t.Fatalf("plugin inventory after allocation intent has type %T", running.Metadata["plugin_executor"])
	}
	envelope["phase"] = "ready"
	if err := repo.CheckpointPluginExecutorInventory(ctx, running); err != nil {
		t.Fatalf("checkpoint ready plugin inventory: %v", err)
	}
	claim, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, models.TaskEnvironmentRecoveryClaimRequest{
		TaskEnvironmentID: "environment-plugin-executor-inventory", OwnerTaskID: running.TaskID,
		OwnershipGeneration: 7, SessionID: running.SessionID, OperationID: "operation-pg-cleanup",
		ExecutorType: string(models.ExecutorTypePluginRemote), AllowCurrentSessionRuntime: true,
	})
	if err != nil {
		t.Fatalf("acquire plugin cleanup claim: %v", err)
	}
	claimedCtx := recoveryclaim.WithClaim(ctx, claim)
	if err := repo.DeletePluginExecutorInventoryIfCurrent(claimedCtx, running.SessionID, running.AgentExecutionID, 7); err != nil {
		t.Fatalf("delete claimed plugin inventory: %v", err)
	}
	if _, err := repo.GetExecutorRunningBySessionID(ctx, running.SessionID); !errors.Is(err, models.ErrExecutorRunningNotFound) {
		t.Fatalf("inventory after claimed delete = %v, want ErrExecutorRunningNotFound", err)
	}
	if err := repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim); err != nil {
		t.Fatalf("release cleanup claim: %v", err)
	}
}

func seedPostgresPluginExecutorInventory(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	const taskID = "task-pg-plugin-executor-inventory"
	workspaceID := "workspace-pg-plugin-executor-inventory"
	workflowID := "workflow-pg-plugin-executor-inventory"
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Plugin executor inventory"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: "Plugin executor inventory"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, WorkflowID: workflowID, WorkflowStepID: "step-plugin-executor-inventory", Title: taskID, Priority: "medium"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "environment-plugin-executor-inventory", TaskID: taskID,
		ExecutorType: string(models.ExecutorTypePluginRemote), OwnershipGeneration: 7,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-plugin-executor-inventory", TaskID: taskID, TaskEnvironmentID: "environment-plugin-executor-inventory"}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
}
