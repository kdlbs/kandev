package dto

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
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

func TestFromTaskSerializesWorkspaceFolders(t *testing.T) {
	now := time.Now().UTC()
	got := FromTask(&models.Task{
		ID: "task-folders",
		WorkspaceFolders: []*models.TaskWorkspaceFolder{{
			ID: "folder-1", TaskID: "task-folders", LocalPath: "/canonical/docs", DisplayName: "docs", Position: 2, CreatedAt: now, UpdatedAt: now,
		}},
	})
	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal task DTO: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode task DTO: %v", err)
	}
	if _, ok := fields["workspace_folders"]; !ok {
		t.Fatalf("workspace_folders missing from task DTO: %s", payload)
	}
}

func TestFromWorkflowStep_PreservesWIPFields(t *testing.T) {
	step := &wfmodels.WorkflowStep{
		ID:             "step-1",
		WorkflowID:     "wf-1",
		Name:           "Work",
		WIPLimit:       3,
		PullFromStepID: "queue-step",
	}

	got := FromWorkflowStep(step)

	if got.WIPLimit != 3 {
		t.Fatalf("WIPLimit = %d, want 3", got.WIPLimit)
	}
	if got.PullFromStepID != "queue-step" {
		t.Fatalf("PullFromStepID = %q, want queue-step", got.PullFromStepID)
	}
}

func TestFromWorkflowStep_PreservesCancelTriggersTurnComplete(t *testing.T) {
	got := FromWorkflowStep(&wfmodels.WorkflowStep{
		ID:                         "step-cancel",
		WorkflowID:                 "wf-1",
		CancelTriggersTurnComplete: true,
	})
	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal workflow step DTO: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode workflow step DTO: %v", err)
	}
	if gotValue, ok := fields["cancel_triggers_turn_complete"].(bool); !ok || !gotValue {
		t.Fatalf("cancel trigger DTO field = %#v, want true", fields["cancel_triggers_turn_complete"])
	}
}

func TestFromTaskSession_IncludesAllWorktrees(t *testing.T) {
	session := &models.TaskSession{
		ID:            "session-1",
		TaskID:        "task-1",
		WorkspacePath: "/task-root",
		Worktrees: []*models.TaskSessionWorktree{
			{ID: "assoc-1", WorktreeID: "wt-1", RepositoryID: "repo-a", WorktreePath: "/x/a"},
			{ID: "assoc-2", WorktreeID: "wt-2", RepositoryID: "repo-b", WorktreePath: "/x/b"},
		},
	}

	full := FromTaskSession(session)
	if len(full.Worktrees) != 2 {
		t.Fatalf("FromTaskSession Worktrees len = %d, want 2", len(full.Worktrees))
	}
	if full.WorktreePath != "/x/a" {
		t.Fatalf("WorktreePath = %q, want /x/a (first worktree)", full.WorktreePath)
	}
	if full.WorkspacePath != "/task-root" {
		t.Fatalf("WorkspacePath = %q, want /task-root", full.WorkspacePath)
	}

	summary := FromTaskSessionSummary(session)
	if len(summary.Worktrees) != 2 {
		t.Fatalf("FromTaskSessionSummary Worktrees len = %d, want 2", len(summary.Worktrees))
	}
	if summary.Worktrees[1].WorktreeID != "wt-2" {
		t.Fatalf("second worktree id = %q, want wt-2", summary.Worktrees[1].WorktreeID)
	}
	if summary.WorkspacePath != "/task-root" {
		t.Fatalf("summary WorkspacePath = %q, want /task-root", summary.WorkspacePath)
	}
}

func TestFromTask_DerivesInterruptedFromMetadata(t *testing.T) {
	now := time.Now().UTC()

	t.Run("marked", func(t *testing.T) {
		task := &models.Task{
			ID:        "t1",
			CreatedAt: now,
			UpdatedAt: now,
			Metadata: map[string]interface{}{
				models.MetaKeyInterruptedAt: "2026-08-02T10:00:00Z",
			},
		}
		got := FromTask(task)
		if !got.Interrupted {
			t.Fatal("Interrupted = false, want true when interrupted_at metadata is present")
		}
	})

	t.Run("unmarked", func(t *testing.T) {
		task := &models.Task{ID: "t1", CreatedAt: now, UpdatedAt: now}
		got := FromTask(task)
		if got.Interrupted {
			t.Fatal("Interrupted = true, want false when interrupted_at metadata is absent")
		}
	})
}

func TestTaskToAPI_DerivesInterruptedFromMetadata(t *testing.T) {
	now := time.Now().UTC()

	marked := (&models.Task{
		ID:        "t1",
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  map[string]interface{}{models.MetaKeyInterruptedAt: "2026-08-02T10:00:00Z"},
	}).ToAPI()
	if !marked.Interrupted {
		t.Fatal("ToAPI Interrupted = false, want true when interrupted_at metadata is present")
	}

	unmarked := (&models.Task{ID: "t1", CreatedAt: now, UpdatedAt: now}).ToAPI()
	if unmarked.Interrupted {
		t.Fatal("ToAPI Interrupted = true, want false when interrupted_at metadata is absent")
	}
}
