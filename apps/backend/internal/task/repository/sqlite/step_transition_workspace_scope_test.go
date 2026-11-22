package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-STEP-TRANSITION-READ-001.1
func TestTaskStepTransitionRecordsHideRenamedStepsOutsideWorkspace(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	for _, workspace := range []*models.Workspace{
		{ID: "source-ws", Name: "Source"},
		{ID: "destination-ws", Name: "Destination"},
	} {
		if err := repo.CreateWorkspace(ctx, workspace); err != nil {
			t.Fatalf("create workspace %s: %v", workspace.ID, err)
		}
	}
	for _, workflow := range []*models.Workflow{
		{ID: "source-wf", WorkspaceID: "source-ws", Name: "Source"},
		{ID: "destination-wf", WorkspaceID: "destination-ws", Name: "Destination"},
	} {
		if err := repo.CreateWorkflow(ctx, workflow); err != nil {
			t.Fatalf("create workflow %s: %v", workflow.ID, err)
		}
	}
	for _, step := range [][3]string{
		{"source-step", "source-wf", "Source step"},
		{"destination-step", "destination-wf", "Destination step"},
	} {
		if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO workflow_steps (id, workflow_id, name, position) VALUES (?, ?, ?, 0)`), step[0], step[1], step[2]); err != nil {
			t.Fatalf("insert step %s: %v", step[0], err)
		}
	}
	task := &models.Task{
		ID: "transferred-records", WorkspaceID: "destination-ws", WorkflowID: "destination-wf",
		WorkflowStepID: "destination-step", Title: "Transferred", Priority: "medium",
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO task_step_transitions
		(task_id, from_workflow_id, from_workflow_step_id, to_workflow_id, to_workflow_step_id, trigger, actor_kind, contract_version, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`), task.ID, "source-wf", "source-step", "destination-wf", "destination-step", "manual_move", "human", 1, time.Now().UTC()); err != nil {
		t.Fatalf("insert transferred move: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO task_step_transitions
		(task_id, from_workflow_id, from_workflow_step_id, to_workflow_id, to_workflow_step_id, trigger, actor_kind, contract_version, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`), task.ID, "destination-wf", "source-step", "destination-wf", "destination-step", "manual_move", "human", 1, time.Now().UTC()); err != nil {
		t.Fatalf("insert mismatched workflow move: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE workflow_steps SET name = ? WHERE id = ?`), "Renamed source step", "source-step"); err != nil {
		t.Fatalf("rename source step: %v", err)
	}

	rows, err := repo.ListTaskStepTransitionRecords(ctx, "destination-ws", task.ID, 0, 10)
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d records, want genesis and two moves", len(rows))
	}
	move := rows[1]
	if move.FromWorkflowStepID == nil || *move.FromWorkflowStepID != "source-step" || move.FromWorkflowStepName != nil {
		t.Fatalf("foreign source endpoint = (%v, %v), want recorded ID and null name", move.FromWorkflowStepID, move.FromWorkflowStepName)
	}
	if move.ToWorkflowStepName == nil || *move.ToWorkflowStepName != "Destination step" {
		t.Fatalf("destination step name = %v, want Destination step", move.ToWorkflowStepName)
	}
	mismatched := rows[2]
	if mismatched.FromWorkflowStepID == nil || *mismatched.FromWorkflowStepID != "source-step" || mismatched.FromWorkflowStepName != nil {
		t.Fatalf("mismatched source endpoint = (%v, %v), want recorded ID and null name", mismatched.FromWorkflowStepID, mismatched.FromWorkflowStepName)
	}
}
