package handlers

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// An executor eligibility refusal on an existing step is forbidden, not a
// missing step, and the response names no executor or profile.
func TestStepMutationExecutorDenialIsForbidden(t *testing.T) {
	h := setupStepRouter(t)
	step := createStepViaHTTP(t, h.router, map[string]interface{}{
		"workflow_id": "workflow-1", "name": "Backlog", "position": 0,
	})
	h.service.SetExecutorMutationChecker(func(context.Context, string) error {
		return fmt.Errorf("%w: %w", taskmodels.ErrExecutorMutationDenied,
			fmt.Errorf("%w: private-profile", repoerrors.ErrExecutorProfileNotFound))
	})
	rec := recordStepEvents(t, h.eventBus)

	resp := doJSON(t, h.router, http.MethodPut, "/api/v1/workflow/steps/"+step.ID, map[string]interface{}{
		"name": "Renamed",
	})
	requireJSONError(t, resp, http.StatusForbidden, taskmodels.ErrExecutorMutationDenied.Error())
	if len(rec.events) != 0 {
		t.Fatalf("denied update published %v", rec.subjects())
	}

	resp = doJSON(t, h.router, http.MethodDelete, "/api/v1/workflow/steps/"+step.ID, nil)
	requireJSONError(t, resp, http.StatusForbidden, taskmodels.ErrExecutorMutationDenied.Error())
	if len(rec.events) != 0 {
		t.Fatalf("denied delete published %v", rec.subjects())
	}
}

// A guard that permits the edit leaves the ordinary step update path intact.
func TestStepMutationExecutorGuardAllowsEdit(t *testing.T) {
	h := setupStepRouter(t)
	step := createStepViaHTTP(t, h.router, map[string]interface{}{
		"workflow_id": "workflow-1", "name": "Backlog", "position": 0,
	})
	h.service.SetExecutorMutationChecker(func(context.Context, string) error { return nil })

	resp := doJSON(t, h.router, http.MethodPut, "/api/v1/workflow/steps/"+step.ID, map[string]interface{}{
		"name": "Renamed",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", resp.Code, resp.Body.String())
	}
}

func TestStepMutationMissingWorkspaceDefaultHasConflictCode(t *testing.T) {
	h := setupStepRouter(t)
	step := createStepViaHTTP(t, h.router, map[string]interface{}{
		"workflow_id": "workflow-1", "name": "Backlog", "position": 0,
	})
	h.service.SetExecutorMutationChecker(func(context.Context, string) error {
		return taskmodels.ErrWorkspaceDefaultExecutorUnavailable
	})
	rec := recordStepEvents(t, h.eventBus)

	resp := doJSON(t, h.router, http.MethodPut, "/api/v1/workflow/steps/"+step.ID, map[string]interface{}{
		"name": "Renamed",
	})
	require.Equal(t, http.StatusConflict, resp.Code)
	require.JSONEq(t,
		`{"error":"workspace default executor is unavailable","error_code":"workspace_default_executor_unavailable"}`,
		resp.Body.String(),
	)
	require.Empty(t, rec.events)

	resp = doJSON(t, h.router, http.MethodDelete, "/api/v1/workflow/steps/"+step.ID, nil)
	require.Equal(t, http.StatusConflict, resp.Code)
	require.JSONEq(t,
		`{"error":"workspace default executor is unavailable","error_code":"workspace_default_executor_unavailable"}`,
		resp.Body.String(),
	)
	require.Empty(t, rec.events)
}
