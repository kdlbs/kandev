package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestProcessOnChildrenCompleted_TransitionsParentWhenAllActiveChildrenTerminal(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "parent", "parent-session", "step_wait")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step_wait"] = &wfmodels.WorkflowStep{
		ID:         "step_wait",
		WorkflowID: "wf1",
		Name:       "Wait for Subtasks",
		Position:   0,
		Events: wfmodels.StepEvents{
			OnChildrenCompleted: []wfmodels.GenericAction{
				{Type: wfmodels.GenericActionMoveToNext},
			},
		},
	}
	stepGetter.steps["step_done"] = &wfmodels.WorkflowStep{
		ID:         "step_done",
		WorkflowID: "wf1",
		Name:       "Done",
		Position:   1,
	}

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createEngineService(t, repo, stepGetter, agentMgr)

	now := time.Now().UTC()
	for _, child := range []*models.Task{
		{
			ID:         "child-complete",
			WorkflowID: "wf1",
			Title:      "Complete child",
			State:      v1.TaskStateCompleted,
			ParentID:   "parent",
			CreatedAt:  now,
			UpdatedAt:  now,
		},
		{
			ID:         "child-open",
			WorkflowID: "wf1",
			Title:      "Open child",
			State:      v1.TaskStateInProgress,
			ParentID:   "parent",
			CreatedAt:  now,
			UpdatedAt:  now,
		},
	} {
		if err := repo.CreateTask(ctx, child); err != nil {
			t.Fatalf("create child %s: %v", child.ID, err)
		}
	}

	if transitioned := svc.processOnChildrenCompleted(ctx, "parent"); transitioned {
		t.Fatalf("expected mixed terminal/non-terminal children not to transition")
	}
	parent, err := repo.GetTask(ctx, "parent")
	if err != nil {
		t.Fatalf("load parent: %v", err)
	}
	if parent.WorkflowStepID != "step_wait" {
		t.Fatalf("expected parent to stay on step_wait, got %q", parent.WorkflowStepID)
	}

	if err := repo.UpdateTaskState(ctx, "child-open", v1.TaskStateCompleted); err != nil {
		t.Fatalf("complete child-open: %v", err)
	}
	if transitioned := svc.processOnChildrenCompleted(ctx, "parent"); !transitioned {
		t.Fatalf("expected all-terminal active children to transition parent")
	}

	parent, err = repo.GetTask(ctx, "parent")
	if err != nil {
		t.Fatalf("load parent after transition: %v", err)
	}
	if parent.WorkflowStepID != "step_done" {
		t.Fatalf("expected parent to move to step_done, got %q", parent.WorkflowStepID)
	}
}
