package handlers

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestListWorkflowStepsEmptyArray(t *testing.T) {
	h := setupStepRouter(t)
	response := doJSON(t, h.router, http.MethodGet, "/api/v1/workflows/workflow-1/workflow/steps", nil)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	var body struct {
		Steps json.RawMessage `json:"steps"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response %s: %v", response.Body.String(), err)
	}
	if string(body.Steps) != "[]" {
		t.Fatalf("steps = %s, want []", body.Steps)
	}
}

func TestListWorkflowStepsStorageErrorReturns500(t *testing.T) {
	h := setupStepRouter(t)
	if _, err := h.db.Exec(`DROP TABLE workflow_steps`); err != nil {
		t.Fatalf("drop workflow steps table: %v", err)
	}

	response := doJSON(t, h.router, http.MethodGet, "/api/v1/workflows/workflow-1/workflow/steps", nil)
	requireJSONError(t, response, http.StatusInternalServerError, "Failed to list steps")
}
