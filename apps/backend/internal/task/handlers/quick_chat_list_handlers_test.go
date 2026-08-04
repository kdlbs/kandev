package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

// quickChatListRepo serves one restorable quick chat plus rows the tab strip
// must ignore, so the endpoint's filtering and payload shape are pinned.
type quickChatListRepo struct {
	mockRepository
	tasks    []*models.Task
	sessions map[string]*models.TaskSession
}

func (r *quickChatListRepo) ListTasksByWorkspace(
	_ context.Context, _, _, _, _ string, _, _ int, _ string, _, _, _, _ bool,
) ([]*models.Task, int, error) {
	return r.tasks, len(r.tasks), nil
}

func (r *quickChatListRepo) GetPrimarySessionInfoByTaskIDs(
	_ context.Context, _ []string,
) (map[string]*models.TaskSession, error) {
	return r.sessions, nil
}

// TestHTTPListQuickChatSessions pins the resync endpoint: it returns the
// workspace's restorable quick chats with their sessions attached, so a client
// that missed WS events converges on the server's tab list instead of drifting.
func TestHTTPListQuickChatSessions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)
	repo := &quickChatListRepo{
		tasks: []*models.Task{
			{
				ID: "task-chat", WorkspaceID: "ws-1", Title: "Renamed Chat", IsEphemeral: true,
				Metadata: map[string]interface{}{models.MetaKeyAgentProfileID: "agent-1"},
			},
			// Workflow-bound ephemeral task: never a quick-chat tab.
			{ID: "task-workflow", WorkspaceID: "ws-1", WorkflowID: "wf-1", IsEphemeral: true},
		},
		sessions: map[string]*models.TaskSession{
			"task-chat":     {ID: "session-chat", TaskID: "task-chat"},
			"task-workflow": {ID: "session-workflow", TaskID: "task-workflow"},
		},
	}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	h := &TaskHandlers{
		service:             svc,
		logger:              log,
		cancellationPending: fakeCancellationPendingProvider{pending: true},
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/workspaces/ws-1/quick-chats", nil)
	c.Params = gin.Params{{Key: "id", Value: "ws-1"}}

	h.httpListQuickChatSessions(c)

	assert.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	var body httpListQuickChatSessionsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Sessions, 1)
	assert.Equal(t, httpQuickChatSession{
		SessionID:      "session-chat",
		TaskID:         "task-chat",
		WorkspaceID:    "ws-1",
		Kind:           service.QuickChatKindChat,
		Name:           "Renamed Chat",
		AgentProfileID: "agent-1",
	}, body.Sessions[0])
	require.Len(t, body.TaskSessions, 1)
	assert.Equal(t, "task-chat", body.TaskSessions[0].TaskID)
	assert.True(t, body.TaskSessions[0].CancellationPending)
}

// TestHTTPListQuickChatSessionsEmptyIsArray keeps the empty response an array
// rather than null: the client replaces its tab list with this payload, and a
// null would be read as "no data" and leave stale tabs in place.
func TestHTTPListQuickChatSessionsEmptyIsArray(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)
	repo := &quickChatListRepo{}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	h := &TaskHandlers{service: svc, logger: log}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/workspaces/ws-1/quick-chats", nil)
	c.Params = gin.Params{{Key: "id", Value: "ws-1"}}

	h.httpListQuickChatSessions(c)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"sessions":[],"task_sessions":[]}`, rec.Body.String())
}

// TestQuickChatRoutesAreRegistered proves the resync endpoint is reachable
// through the real router. Gin resolves routes by path segment, so the GET
// ".../quick-chats" collection must coexist with the POST ".../quick-chat"
// action rather than panicking or shadowing it at registration time.
func TestQuickChatRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &TaskHandlers{logger: newTestLogger(t)}
	handlers.registerHTTP(router)

	registered := make(map[string]bool)
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}

	assert.True(t, registered["GET /api/v1/workspaces/:id/quick-chats"], "resync route missing")
	assert.True(t, registered["POST /api/v1/workspaces/:id/quick-chat"], "start route missing")
}
