package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type workspaceDeleteProjectionCase struct {
	name       string
	err        error
	httpStatus int
	httpBody   string
	wsCode     string
	wsMessage  string
}

func workspaceDeleteProjectionCases() []workspaceDeleteProjectionCase {
	return []workspaceDeleteProjectionCase{
		{
			name:       "wrapped confirmation mismatch",
			err:        fmt.Errorf("compare workspace name: %w", service.ErrWorkspaceConfirmNameMismatch),
			httpStatus: http.StatusBadRequest,
			httpBody:   "compare workspace name: " + service.ErrWorkspaceConfirmNameMismatch.Error(),
			wsCode:     ws.ErrorCodeValidation,
			wsMessage:  "compare workspace name: " + service.ErrWorkspaceConfirmNameMismatch.Error(),
		},
		{
			name:       "wrapped workspace absence",
			err:        fmt.Errorf("load workspace: %w", repoerrors.ErrWorkspaceNotFound),
			httpStatus: http.StatusNotFound,
			httpBody:   "workspace not found",
			wsCode:     ws.ErrorCodeNotFound,
			wsMessage:  "Workspace not found",
		},
		{
			name:       "wrapped permission denial",
			err:        fmt.Errorf("resolve manage scope: %w", service.ErrForbidden),
			httpStatus: http.StatusForbidden,
			httpBody:   service.ErrForbidden.Error(),
			wsCode:     ws.ErrorCodeForbidden,
			wsMessage:  service.ErrForbidden.Error(),
		},
		{
			name:       "unrelated storage error containing not found",
			err:        errors.New("storage failure: workspace metadata not found"),
			httpStatus: http.StatusInternalServerError,
			httpBody:   "workspace not deleted",
			wsCode:     ws.ErrorCodeInternalError,
			wsMessage:  "failed to delete workspace",
		},
		{
			name:       "unrelated error containing permission words",
			err:        errors.New("permission cache unavailable"),
			httpStatus: http.StatusInternalServerError,
			httpBody:   "workspace not deleted",
			wsCode:     ws.ErrorCodeInternalError,
			wsMessage:  "failed to delete workspace",
		},
	}
}

// @covers AC-WORKSPACES-DELETION-001.9, AC-WORKSPACES-DELETION-001.10, AC-WORKSPACES-DELETION-001.11
func TestWorkspaceDeleteErrorProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &WorkspaceHandlers{logger: newTestLogger(t)}

	for _, tc := range workspaceDeleteProjectionCases() {
		t.Run("HTTP/"+tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/workspaces/ws-1", nil)

			h.handleHTTPDeleteWorkspaceError(context, tc.err)

			require.Equal(t, tc.httpStatus, recorder.Code)
			require.JSONEq(t, fmt.Sprintf(`{"error":%q}`, tc.httpBody), recorder.Body.String())
		})
		t.Run("WS/"+tc.name, func(t *testing.T) {
			response, err := h.wsDeleteWorkspaceError(&ws.Message{
				ID:     "delete-request",
				Action: ws.ActionWorkspaceDelete,
			}, tc.err)

			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeError, response.Type)

			var payload ws.ErrorPayload
			require.NoError(t, json.Unmarshal(response.Payload, &payload))
			require.Equal(t, tc.wsCode, payload.Code)
			require.Equal(t, tc.wsMessage, payload.Message)
		})
	}
}
