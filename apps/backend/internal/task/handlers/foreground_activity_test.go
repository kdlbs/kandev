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

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// fakeForegroundActivityProvider stands in for the orchestrator's in-memory
// tracker in handler tests.
type fakeForegroundActivityProvider struct {
	value v1.ForegroundActivity
}

func (f fakeForegroundActivityProvider) ForegroundActivity(string) v1.ForegroundActivity {
	return f.value
}

// orchestratorWithActivity satisfies BOTH OrchestratorStarter and
// dto.ForegroundActivityProvider so NewTaskHandlers derives the provider from it.
type orchestratorWithActivity struct {
	captureOrchestrator
	value v1.ForegroundActivity
}

func (o *orchestratorWithActivity) ForegroundActivity(string) v1.ForegroundActivity {
	return o.value
}

func newSessionHandlerService(t *testing.T, session *models.TaskSession) (*service.Service, *models.TaskSession) {
	t.Helper()
	repo := &mockRepository{sessions: map[string]*models.TaskSession{session.ID: session}}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, newTestLogger(t), service.RepositoryDiscoveryConfig{})
	return svc, session
}

func TestNewTaskHandlers_DerivesForegroundActivityProvider(t *testing.T) {
	// An orchestrator that implements the provider is picked up…
	withActivity := &orchestratorWithActivity{value: v1.ForegroundActivityBackground}
	h := NewTaskHandlers(nil, withActivity, nil, nil, newTestLogger(t))
	require.NotNil(t, h.foregroundActivity, "provider should be derived from the orchestrator")
	assert.Equal(t, v1.ForegroundActivityBackground, h.foregroundActivity.ForegroundActivity("s"))

	// …and one that does not implement it leaves the field nil (field simply omitted).
	plain := &captureOrchestrator{}
	h2 := NewTaskHandlers(nil, plain, nil, nil, newTestLogger(t))
	assert.Nil(t, h2.foregroundActivity)
}

func TestHTTPGetTaskSession_StampsForegroundActivityOnRunning(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _ := newSessionHandlerService(t, &models.TaskSession{
		ID:    "sess-run",
		State: models.TaskSessionStateRunning,
	})
	h := &TaskHandlers{
		service:            svc,
		foregroundActivity: fakeForegroundActivityProvider{value: v1.ForegroundActivityBackground},
		logger:             newTestLogger(t),
	}

	resp := doGetTaskSession(t, h, "sess-run")
	assert.Equal(t, v1.ForegroundActivityBackground, resp.Session.ForegroundActivity,
		"a RUNNING session must carry the fine-grained busy substate on a fresh fetch")
}

func TestHTTPGetTaskSession_OmitsForegroundActivityWhenNotRunning(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _ := newSessionHandlerService(t, &models.TaskSession{
		ID:    "sess-wait",
		State: models.TaskSessionStateWaitingForInput,
	})
	// Even a provider that would report "background" must not leak onto a
	// non-RUNNING session — the coarse state already tells the whole story.
	h := &TaskHandlers{
		service:            svc,
		foregroundActivity: fakeForegroundActivityProvider{value: v1.ForegroundActivityBackground},
		logger:             newTestLogger(t),
	}

	resp := doGetTaskSession(t, h, "sess-wait")
	assert.Empty(t, resp.Session.ForegroundActivity,
		"a non-RUNNING session must not carry a fine-grained busy substate")

	// And confirm the JSON wire actually omits the key (omitempty).
	rec := getTaskSessionRecorder(t, h, "sess-wait")
	assert.NotContains(t, rec.Body.String(), "foreground_activity")
}

func doGetTaskSession(t *testing.T, h *TaskHandlers, id string) dto.GetTaskSessionResponse {
	t.Helper()
	rec := getTaskSessionRecorder(t, h, id)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	var resp dto.GetTaskSessionResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

func getTaskSessionRecorder(t *testing.T, h *TaskHandlers, id string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/task-sessions/"+id, nil).
		WithContext(context.Background())
	c.Params = gin.Params{{Key: "id", Value: id}}
	h.httpGetTaskSession(c)
	return rec
}

var _ OrchestratorStarter = (*orchestratorWithActivity)(nil)
var _ dto.ForegroundActivityProvider = (*orchestratorWithActivity)(nil)
