package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// @covers AC-OFFICE-HUMAN-DONE-002.1
func TestHandleSelectedMoveError_ApprovalsPending409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	err := fmt.Errorf("move: %w", &service.ApprovalsPendingMoveError{
		Pending: []service.PendingApprover{{AgentProfileID: "agent-ceo", Name: "CEO"}},
	})

	handleSelectedMoveError(c, newTestLogger(t), err)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	var body struct {
		Code             string `json:"code"`
		PendingApprovers []struct {
			AgentProfileID string `json:"agent_profile_id"`
			Name           string `json:"name"`
		} `json:"pending_approvers"`
	}
	if decodeErr := json.Unmarshal(rec.Body.Bytes(), &body); decodeErr != nil {
		t.Fatalf("decode: %v", decodeErr)
	}
	if body.Code != "task_approvals_pending" {
		t.Fatalf("code = %q, want task_approvals_pending", body.Code)
	}
	if len(body.PendingApprovers) != 1 || body.PendingApprovers[0].AgentProfileID != "agent-ceo" ||
		body.PendingApprovers[0].Name != "CEO" {
		t.Fatalf("pending_approvers = %+v, want CEO", body.PendingApprovers)
	}
}

type pendingMoveApprover struct{}

func (pendingMoveApprover) PendingApproversForCompletion(context.Context, string, string) ([]service.PendingApprover, error) {
	return []service.PendingApprover{{AgentProfileID: "agent-ceo", Name: "CEO"}}, nil
}

func TestWSMoveTask_PendingApproversRefuseWithConflictDetails(t *testing.T) {
	log := newTestLogger(t)
	task := &models.Task{ID: "ws-office-done", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "work", State: v1.TaskStateTODO, IsFromOffice: true}
	repo := &moveTaskConflictRepo{task: task}
	svc := service.NewService(service.Repos{Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo, Messages: repo, Turns: repo, Sessions: repo, GitSnapshots: repo, RepoEntities: repo, Executors: repo, Environments: repo, TaskEnvironments: repo, Reviews: repo}, nil, log, service.RepositoryDiscoveryConfig{})
	svc.SetWorkflowStepGetter(&reorderHandlerStepGetter{steps: map[string]*wfmodels.WorkflowStep{"done": {ID: "done", WorkflowID: "wf-1", Name: "Done", CompleteTaskOnEnter: true}}})
	svc.SetOfficeApprovalGuard(pendingMoveApprover{})
	h := &TaskHandlers{service: svc, logger: log}
	request, err := ws.NewRequest("move-office-done", ws.ActionTaskMove, map[string]any{"id": task.ID, "workflow_id": "wf-1", "workflow_step_id": "done"})
	if err != nil {
		t.Fatal(err)
	}
	response, err := h.wsMoveTask(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	payload := wsWorkflowError(t, response)
	if payload.Code != ws.ErrorCodeConflict {
		t.Fatalf("code=%s, want conflict: %+v", payload.Code, payload)
	}
	if payload.Details["error_code"] != "task_approvals_pending" {
		t.Fatalf("details=%+v, want approvals code", payload.Details)
	}
	pending, err := json.Marshal(payload.Details["pending_approvers"])
	if err != nil || string(pending) != `[{"agent_profile_id":"agent-ceo","name":"CEO"}]` {
		t.Fatalf("pending=%s error=%v", pending, err)
	}
	if repo.task.WorkflowStepID != "work" || repo.task.State != v1.TaskStateTODO {
		t.Fatalf("refused move wrote task: %+v", repo.task)
	}
}
