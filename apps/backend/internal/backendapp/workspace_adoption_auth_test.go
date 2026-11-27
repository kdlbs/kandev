package backendapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/httpmw"
	"github.com/kandev/kandev/internal/common/logger"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceAdoptionAuthDisabledRejectsAnonymousIdleTask(t *testing.T) {
	svc, repo, _ := newRunSubscriptionCheckHarness(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "adoption-workspace", Name: "Adoption"}))
	require.NoError(t, repo.CreateTask(ctx, &taskmodels.Task{ID: "idle-adoption-task", WorkspaceID: "adoption-workspace", Title: "Idle task", State: "IN_PROGRESS"}))
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpmw.Middleware(newDisabledAuthService(t)))
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	require.NoError(t, err)
	h := taskhandlers.RegisterTaskRoutes(router, ws.NewDispatcher(), svc, nil, repo, nil, log)
	configureWorkspaceAdoptionCaller(h)
	for _, endpoint := range []struct{ method, suffix string }{
		{http.MethodPost, "preview"}, {http.MethodPost, "apply"}, {http.MethodGet, "operation"},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(endpoint.method, "/api/v1/tasks/idle-adoption-task/workspace-adoption/"+endpoint.suffix, strings.NewReader(`{}`))
		require.Empty(t, request.Header.Get("Authorization"))
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
	}
}
