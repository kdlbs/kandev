package backendapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestWorkspaceSecretUpdateThroughHTTP(t *testing.T) {
	h := newBootStateTestHarness(t)
	ctx := context.Background()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	crypto, err := secrets.NewMasterKeyProvider(t.TempDir())
	require.NoError(t, err)
	store, closeSecrets, err := secrets.Provide(h.db, h.db, crypto)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, closeSecrets()) })
	svc := secrets.NewService(secrets.NewUserVisibleStore(store), log)
	svc.SetWorkspaceAuthorizer(h.taskSvc.AuthorizeWorkspaceAccess)

	workspace := &models.Workspace{ID: "secret-update-workspace", Name: "Secret update workspace"}
	require.NoError(t, h.taskRepo.CreateWorkspace(ctx, workspace))
	secret, err := svc.Create(ctx, &secrets.CreateSecretRequest{
		Name: "WORKSPACE_TOKEN", Value: "original-value",
		Scope: secrets.ScopeWorkspace, WorkspaceID: workspace.ID,
	})
	require.NoError(t, err)

	router := gin.New()
	secrets.RegisterRoutes(router, ws.NewDispatcher(), svc, log)

	rec := httptest.NewRecorder()
	body := `{"name":"WORKSPACE_TOKEN","value":"rotated-value"}`
	router.ServeHTTP(rec, httptest.NewRequest(
		http.MethodPut,
		"/api/v1/secrets/"+secret.ID+"?workspace_id="+workspace.ID,
		strings.NewReader(body),
	))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var updated secrets.SecretListItem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	require.Equal(t, secret.ID, updated.ID)

	value, err := svc.RevealWorkspaceSecret(ctx, secret.ID, workspace.ID)
	require.NoError(t, err)
	require.Equal(t, "rotated-value", value)
}
