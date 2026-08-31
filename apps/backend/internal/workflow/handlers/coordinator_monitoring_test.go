package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/controller"
)

// TestCoordinatorMonitoringEndpoints_EmptyByDefault pins the GET shape when
// nothing has been saved: an empty array, never null.
func TestCoordinatorMonitoringEndpoints_EmptyByDefault(t *testing.T) {
	h := setupStepRouter(t)
	createStepViaHTTP(t, h.router, map[string]interface{}{
		"workflow_id": "workflow-1", "name": "Backlog", "position": 0,
	})

	rec := doJSON(t, h.router, http.MethodGet, "/api/v1/workflows/workflow-1/coordinator-monitoring", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !bytesContainsEmptyEntries(rec.Body.Bytes()) {
		t.Fatalf("expected entries: [], got body = %s", rec.Body.String())
	}
}

func bytesContainsEmptyEntries(body []byte) bool {
	var resp controller.ListCoordinatorMonitoringResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return false
	}
	return resp.Entries != nil && len(resp.Entries) == 0
}

// TestCoordinatorMonitoringEndpoints_SaveThenLoad pins the PUT/GET round trip.
func TestCoordinatorMonitoringEndpoints_SaveThenLoad(t *testing.T) {
	h := setupStepRouter(t)
	step1 := createStepViaHTTP(t, h.router, map[string]interface{}{
		"workflow_id": "workflow-1", "name": "Backlog", "position": 0,
	})
	step2 := createStepViaHTTP(t, h.router, map[string]interface{}{
		"workflow_id": "workflow-1", "name": "In Progress", "position": 1,
	})

	putRec := doJSON(t, h.router, http.MethodPut, "/api/v1/workflows/workflow-1/coordinator-monitoring", map[string]interface{}{
		"workspace_id": "ws-1",
		"entries": []map[string]interface{}{
			{"workflow_step_id": step1.ID, "selected": true, "prompt": "watch step 1"},
			{"workflow_step_id": step2.ID, "selected": false, "prompt": ""},
		},
	})
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", putRec.Code, putRec.Body.String())
	}
	var putResp controller.ListCoordinatorMonitoringResponse
	if err := json.Unmarshal(putRec.Body.Bytes(), &putResp); err != nil {
		t.Fatalf("decode PUT response: %v", err)
	}
	// step2 is neither selected nor prompted, so it is skipped on save.
	if len(putResp.Entries) != 1 || putResp.Entries[0].WorkflowStepID != step1.ID {
		t.Fatalf("PUT entries = %#v, want only step1", putResp.Entries)
	}

	getRec := doJSON(t, h.router, http.MethodGet, "/api/v1/workflows/workflow-1/coordinator-monitoring", nil)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", getRec.Code, getRec.Body.String())
	}
	var getResp controller.ListCoordinatorMonitoringResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("decode GET response: %v", err)
	}
	if len(getResp.Entries) != 1 || getResp.Entries[0].Prompt != "watch step 1" {
		t.Fatalf("GET entries = %#v", getResp.Entries)
	}
}

// TestCoordinatorMonitoringEndpoints_RejectsForeignStepID pins the 400
// surfaced when a saved entry names a step from a different workflow.
func TestCoordinatorMonitoringEndpoints_RejectsForeignStepID(t *testing.T) {
	h := setupStepRouter(t)
	otherStep := createStepViaHTTP(t, h.router, map[string]interface{}{
		"workflow_id": "workflow-2", "name": "Elsewhere", "position": 0,
	})

	rec := doJSON(t, h.router, http.MethodPut, "/api/v1/workflows/workflow-1/coordinator-monitoring", map[string]interface{}{
		"workspace_id": "ws-1",
		"entries": []map[string]interface{}{
			{"workflow_step_id": otherStep.ID, "selected": true, "prompt": "foreign"},
		},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}

// TestCoordinatorMonitoringEndpoints_BlockedOnReadOnlyWorkflow pins the 409
// mapping shared with step CRUD when the workflow is not mutable.
func TestCoordinatorMonitoringEndpoints_BlockedOnReadOnlyWorkflow(t *testing.T) {
	h := setupStepRouter(t)
	step := createStepViaHTTP(t, h.router, map[string]interface{}{
		"workflow_id": "workflow-1", "name": "Backlog", "position": 0,
	})
	h.service.SetWorkflowProvider(&fakeWorkflowProvider{workflows: []*taskmodels.Workflow{
		{ID: "workflow-1", WorkspaceID: "ws-1", Source: taskmodels.WorkflowSourceGitHub},
	}})
	h.service.SetWorkspaceProvider(&fakeWorkspaceProvider{
		workspaces: map[string]*taskmodels.Workspace{"ws-1": {ID: "ws-1", Name: "Normal"}},
	})

	rec := doJSON(t, h.router, http.MethodPut, "/api/v1/workflows/workflow-1/coordinator-monitoring", map[string]interface{}{
		"workspace_id": "ws-1",
		"entries": []map[string]interface{}{
			{"workflow_step_id": step.ID, "selected": true, "prompt": "nope"},
		},
	})
	requireJSONError(t, rec, http.StatusConflict,
		"workflow is managed by GitHub sync and is read-only; edit its definition in the synced repository")
}
