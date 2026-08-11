package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestTaskCountsPullCandidatesQueueAndWorkflowPlacement(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-selection")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-selection", WorkspaceID: "workspace-selection", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.April, 5, 6, 7, 8, 0, time.UTC)
	for _, stepID := range []string{"step-feeder", "step-destination"} {
		if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO workflow_steps
			(id, workflow_id, name, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`), stepID, "workflow-selection", stepID, 0, now, now); err != nil {
			t.Fatalf("insert step %s: %v", stepID, err)
		}
	}
	for _, task := range []*models.Task{
		{ID: "task-low", WorkspaceID: "workspace-selection", WorkflowID: "workflow-selection", WorkflowStepID: "step-feeder", Title: "Low", Priority: "low", Position: 1},
		{ID: "task-high", WorkspaceID: "workspace-selection", WorkflowID: "workflow-selection", WorkflowStepID: "step-feeder", Title: "High", Priority: "high", Position: 1},
		{ID: "task-first-position", WorkspaceID: "workspace-selection", WorkflowID: "workflow-selection", WorkflowStepID: "step-feeder", Title: "First", Priority: "none", Position: 0},
	} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask(%s): %v", task.ID, err)
		}
	}
	if count, err := repo.CountTasksByWorkflow(ctx, "workflow-selection"); err != nil || count != 3 {
		t.Fatalf("CountTasksByWorkflow = %d, %v", count, err)
	}
	if count, err := repo.CountTasksByWorkflowStep(ctx, "step-feeder"); err != nil || count != 3 {
		t.Fatalf("CountTasksByWorkflowStep = %d, %v", count, err)
	}
	if count, err := repo.CountTasksByWorkflowStepExcludingTask(ctx, "step-feeder", "task-low"); err != nil || count != 2 {
		t.Fatalf("CountTasksByWorkflowStepExcludingTask = %d, %v", count, err)
	}
	if count, err := repo.CountAdmittedTasksByWorkflowStep(ctx, "step-feeder"); err != nil || count != 3 {
		t.Fatalf("CountAdmittedTasksByWorkflowStep = %d, %v", count, err)
	}
	candidate, err := repo.NextPullCandidate(ctx, "step-feeder", "")
	if err != nil || candidate.ID != "task-first-position" {
		t.Fatalf("NextPullCandidate = %+v, %v", candidate, err)
	}
	candidate, err = repo.NextPullCandidateExcluding(ctx, "step-feeder", []string{"", "task-first-position", "task-high"})
	if err != nil || candidate.ID != "task-low" {
		t.Fatalf("NextPullCandidateExcluding = %+v, %v", candidate, err)
	}

	queuedAt := now.Add(time.Minute)
	if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE tasks SET queued_for_step_id = ?, queued_at = ?, wip_admitted = 0 WHERE id = ?`), "step-destination", queuedAt, "task-first-position"); err != nil {
		t.Fatal(err)
	}
	queued, err := repo.ListQueuedTasks(ctx)
	if err != nil || strings.Join(selectionTaskIDs(queued), ",") != "task-first-position" {
		t.Fatalf("ListQueuedTasks = %v, %v", selectionTaskIDs(queued), err)
	}
	if count, err := repo.CountAdmittedTasksByWorkflowStep(ctx, "step-feeder"); err != nil || count != 2 {
		t.Fatalf("admitted count after queue = %d, %v", count, err)
	}
	candidate, err = repo.NextQueuedTaskForStepExcluding(ctx, "step-feeder", "step-destination", nil)
	if err != nil || candidate.ID != "task-first-position" {
		t.Fatalf("NextQueuedTaskForStepExcluding = %+v, %v", candidate, err)
	}

	if err := repo.AddTaskToWorkflow(ctx, "task-first-position", "workflow-selection", "step-destination", 4); err != nil {
		t.Fatalf("AddTaskToWorkflow: %v", err)
	}
	placed, err := repo.GetTask(ctx, "task-first-position")
	if err != nil || placed.WorkflowStepID != "step-destination" || placed.Position != 4 {
		t.Fatalf("placed task = %+v, %v", placed, err)
	}
	if err := repo.RemoveTaskFromWorkflow(ctx, "task-first-position", "wrong-workflow"); err != nil {
		t.Fatalf("RemoveTaskFromWorkflow(wrong): %v", err)
	}
	stillPlaced, err := repo.GetTask(ctx, "task-first-position")
	if err != nil || stillPlaced.WorkflowID != "workflow-selection" {
		t.Fatalf("wrong-workflow removal mutated task: %+v, %v", stillPlaced, err)
	}
	if err := repo.RemoveTaskFromWorkflow(ctx, "task-first-position", "workflow-selection"); err != nil {
		t.Fatalf("RemoveTaskFromWorkflow: %v", err)
	}
	detached, err := repo.GetTask(ctx, "task-first-position")
	if err != nil || detached.WorkflowID != "" || detached.WorkflowStepID != "" || detached.Position != 0 {
		t.Fatalf("detached task = %+v, %v", detached, err)
	}
}

func selectionTaskIDs(tasks []*models.Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}
