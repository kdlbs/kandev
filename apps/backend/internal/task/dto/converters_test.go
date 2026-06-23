package dto

import (
	"testing"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestFromWorkflowStep_PreservesGenericEvents(t *testing.T) {
	step := &wfmodels.WorkflowStep{
		ID:         "step-1",
		WorkflowID: "wf-1",
		Name:       "Work",
		Events: wfmodels.StepEvents{
			OnChildrenCompleted: []wfmodels.GenericAction{{
				Type: wfmodels.GenericActionQueueRun,
				Config: map[string]interface{}{
					"target": "primary",
					"reason": "children_completed",
				},
			}},
		},
	}

	got := FromWorkflowStep(step)
	if got.Events == nil {
		t.Fatal("Events nil, want on_children_completed")
	}
	if len(got.Events.OnChildrenCompleted) != 1 {
		t.Fatalf("OnChildrenCompleted len = %d, want 1", len(got.Events.OnChildrenCompleted))
	}
	action := got.Events.OnChildrenCompleted[0]
	if action.Type != "queue_run" {
		t.Fatalf("action type = %q, want queue_run", action.Type)
	}
	if action.Config["reason"] != "children_completed" {
		t.Fatalf("action reason = %v, want children_completed", action.Config["reason"])
	}
}
