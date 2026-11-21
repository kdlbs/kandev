package handlers

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

func (r *httpTaskRepo) GetWorkflowStepWIPOccupancy(_ context.Context, stepID string) (*models.WIPStepOccupancy, error) {
	r.countedStepID = stepID
	return &models.WIPStepOccupancy{
		StepID: stepID, WorkspaceID: "ws-b", WIPLimit: 1, Counted: 1,
		Occupants: []models.WIPStepOccupant{{
			TaskID: "task-stuck", Title: "Stuck", State: "FAILED",
			HoldsSlot: true, Reason: models.WIPOccupantReasonNoLiveSession,
		}},
	}, nil
}

func TestHTTPGetStepWIPOccupants(t *testing.T) {
	repo := &httpTaskRepo{}
	h := newHTTPTaskHandlers(t, repo)
	c, rec := taskRequestAs(t, "", http.MethodGet, "/api/v1/workflow/steps/step-1/wip-occupants", "step-1")

	h.httpGetStepWIPOccupants(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"reason":"no_live_session"`)
	require.Contains(t, rec.Body.String(), `"holds_slot":true`)
	require.Equal(t, "step-1", repo.countedStepID)
}

func TestHTTPGetStepWIPOccupantsDeniesUnresolvedStep(t *testing.T) {
	repo := &httpTaskRepo{}
	h := newHTTPTaskHandlers(t, repo)
	c, rec := taskRequestAs(t, "user-a", http.MethodGet, "/api/v1/workflow/steps/step-b/wip-occupants", "step-b")

	h.httpGetStepWIPOccupants(c)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Empty(t, repo.countedStepID, "a denied read must not reach the repository")
}
