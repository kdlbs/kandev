package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestTaskBulkRemovalFenceMergesWithoutLosingMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	f := fieldProtocolPair(t, "owner")[0]
	for _, suppressed := range []bool{true, false} {
		body := `{"suppressed":true}`
		if !suppressed {
			body = `{"suppressed":false}`
		}
		request := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/protocol-fields/session-auto-provisioning", strings.NewReader(body))
		response := httptest.NewRecorder()
		f.router.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		task, err := f.gate.Repository.GetTask(context.Background(), "protocol-fields")
		require.NoError(t, err)
		require.Equal(t, suppressed, task.Metadata[models.MetaKeyBulkSessionRemovalSuppressEnsure])
		require.Equal(t, "current", task.Metadata["keep"])
	}
}

func TestTaskBulkRemovalFenceValidatesAndAuthorizes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	f := fieldProtocolPair(t, "owner")[0]
	for _, body := range []string{`{}`, `{"suppressed":null}`, `{"suppressed":"true"}`, `{"suppressed":true,"x":1}`, `{"suppressed":`, `{"suppressed":true} {}`} {
		request := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/protocol-fields/session-auto-provisioning", strings.NewReader(body))
		response := httptest.NewRecorder()
		f.router.ServeHTTP(response, request)
		require.Equal(t, http.StatusBadRequest, response.Code, body)
	}
	ctx := context.Background()
	require.NoError(t, f.gate.UpsertWorkspaceMember(ctx, &models.WorkspaceMember{
		WorkspaceID: "protocol-ws", UserID: "viewer", Role: "viewer",
	}))
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/protocol-fields/session-auto-provisioning", strings.NewReader(`{"suppressed":true}`))
	request = request.WithContext(authn.WithIdentity(ctx, authn.Identity{UserID: "viewer", Role: authn.RoleMember}))
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, request)
	require.Equal(t, http.StatusForbidden, response.Code)
	task, err := f.gate.Repository.GetTask(ctx, "protocol-fields")
	require.NoError(t, err)
	require.Nil(t, task.Metadata[models.MetaKeyBulkSessionRemovalSuppressEnsure])
	require.Equal(t, "current", task.Metadata["keep"])
}
