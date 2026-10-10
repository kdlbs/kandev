package handlers

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-WORKFLOW-STEP-ORDERING-001.4
func TestReorderStepsInvalidOrdersAreBadRequests(t *testing.T) {
	for _, name := range []string{"duplicate", "omitted", "empty"} {
		t.Run(name, func(t *testing.T) {
			h := setupStepRouter(t)
			seedReorderWorkflow(t, h)
			first := createStepViaHTTP(t, h.router, map[string]interface{}{"workflow_id": "workflow-1", "name": "First", "position": 0})
			second := createStepViaHTTP(t, h.router, map[string]interface{}{"workflow_id": "workflow-1", "name": "Second", "position": 1})
			orders := map[string][]string{"duplicate": {second.ID, second.ID}, "omitted": {second.ID}, "empty": {}}
			before, err := h.repo.ListStepsByWorkflow(context.Background(), "workflow-1")
			require.NoError(t, err)
			response := doJSON(t, h.router, http.MethodPut, "/api/v1/workflows/workflow-1/workflow/steps/reorder", map[string]interface{}{"step_ids": orders[name]})
			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			after, err := h.repo.ListStepsByWorkflow(context.Background(), "workflow-1")
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.Equal(t, first.ID, after[0].ID)
		})
	}
}

func seedReorderWorkflow(t *testing.T, h *workflowHarness) {
	t.Helper()
	_, err := h.db.Exec(`INSERT INTO workflows (id, name, created_at, updated_at) VALUES ('workflow-1', 'Test', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
}
