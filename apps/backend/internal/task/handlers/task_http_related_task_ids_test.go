package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/task/models"
)

func TestHTTPCreateTaskPersistsRelatedTaskIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, repo, _, _ := newCreationAttachmentHandler(t)
	db := sqlx.NewDb(repo.DB(), "sqlite3")
	officeRepo, err := sqlite.NewWithDB(db, db, nil)
	require.NoError(t, err)
	h.service.SetRelationRepository(officeRepo)
	h.service.SetBlockerRepository(officeRepo)

	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "owner-http"})
	peer := &models.Task{
		ID: "related-http-peer", WorkspaceID: "ws-http-creation",
		WorkflowID: "wf-http-creation", Title: "Related peer",
	}
	require.NoError(t, repo.CreateTask(ctx, peer))
	payload := `{"workspace_id":"ws-http-creation","workflow_id":"wf-http-creation","title":"HTTP task","related_task_ids":["related-http-peer"]}`
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request = c.Request.WithContext(ctx)
	h.httpCreateTask(c)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var response struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.NotEmpty(t, response.ID)

	createdRelations, err := officeRepo.ListRelationsForTask(ctx, response.ID)
	require.NoError(t, err)
	require.Equal(t, []string{peer.ID}, createdRelations)
	peerRelations, err := officeRepo.ListRelationsForTask(ctx, peer.ID)
	require.NoError(t, err)
	require.Equal(t, []string{response.ID}, peerRelations)
}
