package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	runsmodels "github.com/kandev/kandev/internal/runs/models"
	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// A task-scoped run queued without a workflow step records the task's step at
// queue time under queued_workflow_step_id, so every Office queue path lets
// exclude_author_provider recognize work-stage runs without opting the run
// into step-change staleness cancellation (REQ-OFFICE-REVIEW-AUTHOR-EXCL-002).
func TestQueueRun_StampsTaskWorkflowStepWhenMissing(t *testing.T) {
	cases := []struct {
		name        string
		req         runsservice.QueueRunRequest
		resolverErr error
		wantQueued  string
		wantStep    string
		wantCalls   int
	}{
		{
			name:       "payload task without step is stamped",
			req:        runsservice.QueueRunRequest{Payload: map[string]any{"task_id": "task-1"}},
			wantQueued: "review", wantCalls: 1,
		},
		{
			name:       "typed task without step is stamped",
			req:        runsservice.QueueRunRequest{TaskID: "task-1"},
			wantQueued: "review", wantCalls: 1,
		},
		{
			name:     "explicit step is kept",
			req:      runsservice.QueueRunRequest{TaskID: "task-1", WorkflowStepID: "work"},
			wantStep: "work",
		},
		{
			name:     "payload step is kept",
			req:      runsservice.QueueRunRequest{Payload: map[string]any{"task_id": "task-1", "workflow_step_id": "work"}},
			wantStep: "work",
		},
		{
			name: "taskless run is not stamped",
			req:  runsservice.QueueRunRequest{},
		},
		{
			name:        "resolver failure still queues without a step",
			req:         runsservice.QueueRunRequest{TaskID: "task-1"},
			resolverErr: errors.New("db down"),
			wantCalls:   1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, repo := newTestServiceWithRepo(t)
			calls := 0
			svc.SetTaskStepResolver(func(_ context.Context, taskID string) (string, error) {
				calls++
				if taskID != "task-1" {
					t.Fatalf("resolver task = %q", taskID)
				}
				return "review", tc.resolverErr
			})
			req := tc.req
			req.AgentProfileID = "agent-primary"
			req.Reason = "task_assigned"
			_, run, err := svc.QueueRunAndReturn(context.Background(), req)
			if err != nil || run == nil {
				t.Fatalf("queue: run=%v err=%v", run, err)
			}
			stored, err := repo.GetRunByID(context.Background(), run.ID)
			if err != nil {
				t.Fatalf("get run: %v", err)
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(stored.Payload), &payload); err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			step, _ := payload["workflow_step_id"].(string)
			queued, _ := payload[runsmodels.QueuedWorkflowStepPayloadKey].(string)
			if step != tc.wantStep || queued != tc.wantQueued || calls != tc.wantCalls {
				t.Fatalf("workflow_step_id=%q queued=%q calls=%d, want %q %q %d",
					step, queued, calls, tc.wantStep, tc.wantQueued, tc.wantCalls)
			}
		})
	}
}
