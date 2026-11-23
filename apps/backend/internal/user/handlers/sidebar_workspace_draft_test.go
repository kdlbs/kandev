package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/user/controller"
	"github.com/kandev/kandev/internal/user/dto"
	"github.com/kandev/kandev/internal/user/service"
	userstore "github.com/kandev/kandev/internal/user/store"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestHTTPWorkspaceDraftDisclosureRoundTrip(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	repo, cleanup, err := userstore.Provide(conn, conn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cleanup() })
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	svc := service.NewService(repo, nil, log)
	svc.SetSidebarWorkspaceAccess(func(context.Context) ([]string, error) { return []string{"a"}, nil })
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandlers(controller.NewController(svc), log).registerHTTP(router)
	for _, body := range []string{
		`{"sidebar_view_state":{"workspace_id":"a","views_upsert":[{"id":"base","name":"Tag","group":"plugin:tags:tags","collapsed_groups":["tag"]}],"active_view_id":"base","draft":{"base_view_id":"base","group":"plugin:tags:multi-tags","filters":[],"sort":{"key":"title","direction":"asc"},"collapsed_groups":[]}}}`,
		`{"sidebar_view_state":{"workspace_id":"a","draft":{"base_view_id":"base","group":"plugin:tags:multi-tags","filters":[],"sort":{"key":"title","direction":"asc"},"collapsed_groups":["multi"]}}}`,
	} {
		request := httptest.NewRequest(http.MethodPatch, "/api/v1/user/settings", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/user/settings", nil))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var payload dto.UserSettingsResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	state := payload.Settings.SidebarViewsByWorkspace["a"]
	require.NotNil(t, state.Draft)
	require.Equal(t, "plugin:tags:multi-tags", state.Draft.Group)
	require.Equal(t, []string{"multi"}, state.Draft.CollapsedGroups)
	for _, view := range state.Views {
		if view.ID == "base" {
			require.Equal(t, []string{"tag"}, view.CollapsedGroups)
		}
	}
}
