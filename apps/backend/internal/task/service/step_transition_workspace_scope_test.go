package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type transferAfterAuthorizationTaskRepository struct {
	repository.TaskRepository
	sqliteRepo              *sqliterepo.Repository
	transferWorkspaceID     string
	workspaceIDUsedForNames string
}

func (r *transferAfterAuthorizationTaskRepository) ListTaskStepTransitionRecords(
	ctx context.Context, workspaceID, taskID string, afterID int64, limit int,
) ([]models.StepTransitionRecord, error) {
	r.workspaceIDUsedForNames = workspaceID
	if _, err := r.sqliteRepo.DB().ExecContext(ctx,
		`UPDATE tasks SET workspace_id = ?, workflow_id = ?, workflow_step_id = ? WHERE id = ?`,
		r.transferWorkspaceID, "after-transfer-wf", "after-transfer-step", taskID); err != nil {
		return nil, err
	}
	return r.sqliteRepo.ListTaskStepTransitionRecords(ctx, workspaceID, taskID, afterID, limit)
}

// @covers AC-TASKS-STEP-TRANSITION-READ-001.1
func TestTaskStepTransitionReadKeepsAuthorizedWorkspaceAcrossTransfer(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	for _, workspace := range []*models.Workspace{
		{ID: "source-ws", Name: "Source"},
		{ID: "authorized-ws", Name: "Authorized", OwnerID: "reader"},
		{ID: "after-transfer-ws", Name: "After transfer"},
	} {
		if err := repo.CreateWorkspace(ctx, workspace); err != nil {
			t.Fatalf("create workspace %s: %v", workspace.ID, err)
		}
	}
	for _, workflow := range []*models.Workflow{
		{ID: "source-wf", WorkspaceID: "source-ws", Name: "Source"},
		{ID: "authorized-wf", WorkspaceID: "authorized-ws", Name: "Authorized"},
		{ID: "after-transfer-wf", WorkspaceID: "after-transfer-ws", Name: "After transfer"},
	} {
		if err := repo.CreateWorkflow(ctx, workflow); err != nil {
			t.Fatalf("create workflow %s: %v", workflow.ID, err)
		}
	}
	for _, step := range [][3]string{
		{"source-step", "source-wf", "Renamed source step"},
		{"authorized-step", "authorized-wf", "Authorized step"},
		{"after-transfer-step", "after-transfer-wf", "After-transfer step"},
	} {
		if _, err := repo.DB().Exec(`INSERT INTO workflow_steps (id, workflow_id, name, position) VALUES (?, ?, ?, 0)`, step[0], step[1], step[2]); err != nil {
			t.Fatalf("insert step %s: %v", step[0], err)
		}
	}
	task := &models.Task{
		ID: "transition-transfer-race", WorkspaceID: "authorized-ws", WorkflowID: "authorized-wf",
		WorkflowStepID: "authorized-step", Title: "Transfer race", Priority: "medium",
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := repo.DB().Exec(`INSERT INTO task_step_transitions
		(task_id, from_workflow_id, from_workflow_step_id, to_workflow_id, to_workflow_step_id, trigger, actor_kind, contract_version, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, task.ID, "source-wf", "source-step", "authorized-wf", "authorized-step", "manual_move", "human", 1, time.Now().UTC()); err != nil {
		t.Fatalf("insert transferred move: %v", err)
	}
	transferRepo := &transferAfterAuthorizationTaskRepository{
		TaskRepository: repo, sqliteRepo: repo, transferWorkspaceID: "after-transfer-ws",
	}
	svc.tasks = transferRepo

	identityCtx := authn.WithIdentity(ctx, authn.Identity{UserID: "reader", Role: authn.RoleMember})
	rows, _, err := svc.ListTaskStepTransitionRecords(identityCtx, task.ID, 10, "")
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d records, want genesis and transferred move", len(rows))
	}
	if transferRepo.workspaceIDUsedForNames != "authorized-ws" {
		t.Fatalf("name lookup workspace = %q, want the workspace used for authorization", transferRepo.workspaceIDUsedForNames)
	}
	move := rows[1]
	if move.FromWorkflowStepID == nil || *move.FromWorkflowStepID != "source-step" || move.FromWorkflowStepName != nil {
		t.Fatalf("foreign source endpoint = (%v, %v), want recorded ID and null name", move.FromWorkflowStepID, move.FromWorkflowStepName)
	}
	if move.ToWorkflowStepName == nil || *move.ToWorkflowStepName != "Authorized step" {
		t.Fatalf("authorized destination name = %v, want Authorized step", move.ToWorkflowStepName)
	}
}
