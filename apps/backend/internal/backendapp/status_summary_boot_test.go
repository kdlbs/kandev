package backendapp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/task/statussummary"
	"github.com/kandev/kandev/internal/webapp"
)

func TestBootKanbanSnapshotHydratesTaskStatusSummaryInBatch(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	workspaces, err := harness.taskSvc.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	workflows, err := harness.taskSvc.ListWorkflows(ctx, workspaces[0].ID, true)
	if err != nil {
		t.Fatalf("list workflows: %v", err)
	}
	steps, err := harness.workflowSvc.ListStepsByWorkflow(ctx, workflows[0].ID)
	if err != nil {
		t.Fatalf("list workflow steps: %v", err)
	}
	taskResult, err := harness.taskSvc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID:    workspaces[0].ID,
		WorkflowID:     workflows[0].ID,
		WorkflowStepID: steps[0].ID,
		Title:          "Summary hydration task",
	})
	task := taskResult.Task
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	changed, err := harness.taskRepo.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{
		TaskID:      task.ID,
		WorkspaceID: workspaces[0].ID,
		Summary: statussummary.TaskStatusSummary{
			Revision:           7,
			ForegroundActivity: "background",
			PendingAction:      "clarification",
		},
	})
	if err != nil || !changed {
		t.Fatalf("persist task summary: changed=%v err=%v", changed, err)
	}

	state := bootInitialState(ctx, nil, routeParams{
		taskSvc:  harness.taskSvc,
		services: &Services{Workflow: harness.workflowSvc},
	}, webapp.ClassifyRoute("/"))
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal boot state: %v", err)
	}
	var decoded struct {
		KanbanMulti struct {
			Snapshots map[string]struct {
				Tasks []struct {
					ID            string                           `json:"id"`
					StatusSummary *statussummary.TaskStatusSummary `json:"statusSummary"`
				} `json:"tasks"`
			} `json:"snapshots"`
		} `json:"kanbanMulti"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode boot state: %v", err)
	}
	snapshot := decoded.KanbanMulti.Snapshots[workflows[0].ID]
	for _, item := range snapshot.Tasks {
		if item.ID != task.ID {
			continue
		}
		if item.StatusSummary == nil || item.StatusSummary.Revision != 7 || item.StatusSummary.PendingAction != "clarification" {
			t.Fatalf("boot status summary = %#v", item.StatusSummary)
		}
		return
	}
	t.Fatalf("task %q missing from boot snapshot: %s", task.ID, raw)
}

type bootFakeQueuedCounter struct {
	byTask map[string]int
}

func (f bootFakeQueuedCounter) CountPendingByTaskIDs(_ context.Context, taskIDs []string) (map[string]int, error) {
	out := make(map[string]int, len(taskIDs))
	for _, id := range taskIDs {
		out[id] = f.byTask[id]
	}
	return out, nil
}

func TestBootKanbanSnapshotStampsQueuedPromptCount(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	workspaces, err := harness.taskSvc.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	workflows, err := harness.taskSvc.ListWorkflows(ctx, workspaces[0].ID, true)
	if err != nil {
		t.Fatalf("list workflows: %v", err)
	}
	steps, err := harness.workflowSvc.ListStepsByWorkflow(ctx, workflows[0].ID)
	if err != nil {
		t.Fatalf("list workflow steps: %v", err)
	}
	taskResult, err := harness.taskSvc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID:    workspaces[0].ID,
		WorkflowID:     workflows[0].ID,
		WorkflowStepID: steps[0].ID,
		Title:          "Queued count boot task",
	})
	task := taskResult.Task
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	harness.taskSvc.SetQueuedPromptCounter(bootFakeQueuedCounter{byTask: map[string]int{task.ID: 2}})

	state := bootInitialState(ctx, nil, routeParams{
		taskSvc:  harness.taskSvc,
		services: &Services{Workflow: harness.workflowSvc},
	}, webapp.ClassifyRoute("/"))
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal boot state: %v", err)
	}
	var decoded struct {
		KanbanMulti struct {
			Snapshots map[string]struct {
				Tasks []struct {
					ID            string                           `json:"id"`
					StatusSummary *statussummary.TaskStatusSummary `json:"statusSummary"`
				} `json:"tasks"`
			} `json:"snapshots"`
		} `json:"kanbanMulti"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode boot state: %v", err)
	}
	for _, item := range decoded.KanbanMulti.Snapshots[workflows[0].ID].Tasks {
		if item.ID != task.ID {
			continue
		}
		if item.StatusSummary == nil || item.StatusSummary.QueuedPromptCount != 2 {
			t.Fatalf("boot status summary queued prompt count = %#v, want 2", item.StatusSummary)
		}
		return
	}
	t.Fatalf("task %q missing from boot snapshot", task.ID)
}
