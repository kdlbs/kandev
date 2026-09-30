package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-PLUGINS-WORKFLOW-HISTORY-001.1
func TestTaskTransitionReadReturnsGenesis(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	createStepTransitionsTestTask(t, repo, "history-task", "workflow-1", "draft")
	rows, err := repo.ListTaskStepTransitions(context.Background(), "history-task", 0, 10)
	if err != nil {
		t.Fatalf("list transition history: %v", err)
	}
	if len(rows) != 1 || rows[0].FromWorkflowID != nil || rows[0].ToWorkflowStepID == nil || *rows[0].ToWorkflowStepID != "draft" {
		t.Fatalf("rows = %+v, want genesis row with a null source", rows)
	}
}

// @covers AC-PLUGINS-WORKFLOW-HISTORY-001.1 AC-PLUGINS-WORKFLOW-HISTORY-001.2
func TestTaskTransitionReadPagesByIDAndPreservesRemovedEndpoints(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	createStepTransitionsTestTask(t, repo, "history-pages", "workflow-1", "draft")
	now := time.Now().UTC()
	for _, step := range []string{"review", "removed-step", "done"} {
		if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO task_step_transitions
			(task_id, from_workflow_id, from_workflow_step_id, to_workflow_id, to_workflow_step_id, trigger, actor_kind, contract_version, occurred_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`), "history-pages", "workflow-1", "draft", "workflow-1", step, "test", "system", 1, now); err != nil {
			t.Fatal(err)
		}
	}
	first, err := repo.ListTaskStepTransitions(context.Background(), "history-pages", 0, 2)
	if err != nil || len(first) != 2 || first[0].ID <= first[1].ID {
		t.Fatalf("first page = %+v, err %v", first, err)
	}
	second, err := repo.ListTaskStepTransitions(context.Background(), "history-pages", first[1].ID, 2)
	if err != nil || len(second) != 2 || second[0].ID >= first[1].ID {
		t.Fatalf("second page = %+v, err %v", second, err)
	}
	if second[0].ToWorkflowStepID == nil || *second[0].ToWorkflowStepID != "review" || second[1].FromWorkflowID != nil {
		t.Fatalf("removed/nullable endpoints changed: %+v", second)
	}
}

// @covers AC-PLUGINS-WORKFLOW-HISTORY-002.1 AC-PLUGINS-WORKFLOW-HISTORY-002.2
func TestWorkflowTransitionGroupsCountRetainedRoutes(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	createStepTransitionsTestTask(t, repo, "route-task", "workflow-1", "draft")
	if err := repo.ArchiveTask(ctx, "route-task"); err != nil {
		t.Fatal(err)
	}
	for _, move := range [][4]any{
		{"workflow-1", "draft", "workflow-1", "review"},
		{"workflow-1", "review", "workflow-2", "other"},
		{"workflow-2", "other", "workflow-1", "removed-step"},
	} {
		if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO task_step_transitions
			(task_id, from_workflow_id, from_workflow_step_id, to_workflow_id, to_workflow_step_id, trigger, actor_kind, contract_version, occurred_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`), "route-task", move[0], move[1], move[2], move[3], "test", "system", 1, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "foreign-route", WorkspaceID: "ws-2", WorkflowID: "workflow-1", WorkflowStepID: "draft", Title: "Foreign", Priority: "medium"}); err != nil {
		t.Fatal(err)
	}
	groups, err := repo.ListWorkflowTransitionGroups(ctx, "ws-1", "workflow-1", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 4 || groups[0].Kind != "entry" || groups[1].Kind != "entry" || groups[2].Kind != "exit" || groups[3].Kind != "within" {
		t.Fatalf("groups = %+v, want two entries, exit, within", groups)
	}
	if groups[1].ToStepID == nil || *groups[1].ToStepID != "removed-step" {
		t.Fatalf("removed step lost: %+v", groups[1])
	}
	page, err := repo.ListWorkflowTransitionGroups(ctx, "ws-1", "workflow-1", "entry||draft", 2)
	if err != nil || len(page) != 2 || page[0].Kind != "entry" || page[1].Kind != "exit" {
		t.Fatalf("next route page = %+v, err %v", page, err)
	}
}

// @covers AC-PLUGINS-WORKFLOW-HISTORY-002.1
func TestWorkflowTransitionReadIndexesExistAfterSchemaReplay(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	if err := repo.initStepTransitionsSchema(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"idx_task_step_transitions_task_id", "idx_task_step_transitions_from_workflow", "idx_task_step_transitions_to_workflow"} {
		var count int
		if err := repo.db.Get(&count, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, name); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("index %s missing after schema replay", name)
		}
	}
}
