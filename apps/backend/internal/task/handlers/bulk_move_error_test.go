package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/completion"
	"github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

type bulkMoveHTTPRefusalPolicy struct{}

func (bulkMoveHTTPRefusalPolicy) AdmitCompletion(
	_ context.Context,
	_ completion.Querier,
	current, _ *models.Task,
	_ completion.Admission,
) error {
	return &completion.RefusedError{Code: completion.CodeApprovalsPending, Subjects: []string{"agent-approver"}}
}

func TestHTTPBulkMoveRefusalReturnsStructuredConflict(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{
			name: "selected tasks",
			body: `{"task_ids":["task-bulk-refused"],"target_workflow_id":"wf-reorder-http","target_step_id":"step-bulk-done"}`,
		},
		{
			name: "source step",
			body: `{"source_workflow_id":"wf-reorder-http","source_step_id":"step-bulk-source","target_workflow_id":"wf-reorder-http","target_step_id":"step-bulk-done"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, repo := newReorderHandlerTest(t, "step-bulk-source", "task-bulk-refused")
			ctx := context.Background()
			now := time.Now().UTC()
			_, err := repo.DB().ExecContext(ctx, `INSERT INTO workflow_steps
				(id, workflow_id, name, position, complete_task_on_enter, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				"step-bulk-done", "wf-reorder-http", "Done", 1, true, now, now)
			if err != nil {
				t.Fatalf("insert completion step: %v", err)
			}
			h.service.SetWorkflowStepGetter(&reorderHandlerStepGetter{steps: map[string]*wfmodels.WorkflowStep{
				"step-bulk-source": {ID: "step-bulk-source", WorkflowID: "wf-reorder-http", Name: "Work", Position: 0},
				"step-bulk-done":   {ID: "step-bulk-done", WorkflowID: "wf-reorder-http", Name: "Done", Position: 1, CompleteTaskOnEnter: true},
			}})
			repo.SetCompletionPolicies(bulkMoveHTTPRefusalPolicy{})
			h.service.SetApproverNameResolver(fixedApproverNameResolver{})
			before, err := repo.GetTask(ctx, "task-bulk-refused")
			if err != nil {
				t.Fatalf("read task before move: %v", err)
			}
			c, rec := newWorkflowRequest(t, http.MethodPost, "/api/v1/tasks/bulk-move", tc.body)

			h.httpBulkMoveTasks(c)

			if rec.Code != http.StatusConflict {
				t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body)
			}
			var body struct {
				Code         string   `json:"code"`
				MovedTaskIDs []string `json:"moved_task_ids"`
				Refusals     []struct {
					TaskID           string `json:"task_id"`
					TaskTitle        string `json:"task_title"`
					Code             string `json:"code"`
					PendingApprovers []struct {
						AgentProfileID string `json:"agent_profile_id"`
						Name           string `json:"name"`
					} `json:"pending_approvers"`
				} `json:"refusals"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.Code != service.BulkMoveRefusedCode || len(body.MovedTaskIDs) != 0 || len(body.Refusals) != 1 {
				t.Fatalf("response = %+v, want whole-batch refusal", body)
			}
			refusal := body.Refusals[0]
			if refusal.TaskID != "task-bulk-refused" || refusal.TaskTitle != "task-bulk-refused" || refusal.Code != "task_approvals_pending" {
				t.Fatalf("refusal = %+v", refusal)
			}
			if len(refusal.PendingApprovers) != 1 || refusal.PendingApprovers[0].AgentProfileID != "agent-approver" || refusal.PendingApprovers[0].Name != "CEO" {
				t.Fatalf("pending approvers = %+v", refusal.PendingApprovers)
			}
			stored, err := repo.GetTask(ctx, "task-bulk-refused")
			if err != nil {
				t.Fatalf("read task: %v", err)
			}
			if stored.WorkflowID != before.WorkflowID || stored.WorkflowStepID != before.WorkflowStepID || stored.State != before.State {
				t.Fatalf("refused move wrote task: %+v", stored)
			}
		})
	}
}
