package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func runtimeRequest(t *testing.T, router *gin.Engine, method, path, token, runID string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	request := httptest.NewRequest(method, path, bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if runID != "" {
		request.Header.Set("X-Kandev-Run-Id", runID)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
func TestRuntimeAPIScopesWritesAndRevokesFinishedRuns(t *testing.T) {
	s, _, task := newRuntime(t)
	ctx := context.Background()
	for id, ws := range map[string]string{task: "ws", "delivery": "ws", "foreign": "other"} {
		s.Tasks.(*testTasks).tasks[id] = &taskmodels.Task{ID: id, WorkspaceID: ws}
	}
	require.NoError(t, s.QueueTurn(ctx, "chief", task, "task_comment", "request", nil))
	run, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	require.NoError(t, s.Runs.UpdateRunRuntimeSnapshot(ctx, run.ID, "workspace_coordinator", run.Payload, "session"))
	token, err := s.Auth.MintRuntimeJWT("chief", task, "ws", run.ID, "session", "workspace_coordinator")
	require.NoError(t, err)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1/orchestration", runtimeauth.Middleware(s.Auth, s.Personas)), &Handler{Service: s})
	url := "/api/v1/orchestration/tasks/"
	require.Equal(t, 404, runtimeRequest(t, router, "GET", url+"foreign", token, "", nil).Code)
	require.Equal(t, 403, runtimeRequest(t, router, "POST", url+"delivery/comments", token, "", map[string]string{"body": "A note"}).Code)
	require.Equal(t, 201, runtimeRequest(t, router, "POST", url+"delivery/comments", token, run.ID, map[string]string{"body": "A note", "author_id": "spoof"}).Code)
	rows, err := s.Repo.ListComments(ctx, "delivery", 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "chief", rows[0].AuthorID)
	require.NoError(t, s.Runs.FinishRun(ctx, run.ID, "finished", nil))
	require.Equal(t, 403, runtimeRequest(t, router, "POST", url+"delivery/comments", token, run.ID, map[string]string{"body": "Another note"}).Code)
	rows, err = s.Repo.ListComments(ctx, "delivery", 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
}
func TestRetryQueuesOriginalIntentWithoutReusingCredentials(t *testing.T) {
	s, _, task := newRuntime(t)
	ctx := context.Background()
	require.NoError(t, s.Repo.PutComment(ctx, &models.TaskComment{ID: "original", TaskID: task, AuthorID: "user", AuthorType: "user", Source: "user", Body: "Original request"}))
	require.NoError(t, s.QueueTurn(ctx, "chief", task, "task_comment", "failed", map[string]any{"comment_id": "original"}))
	first, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	require.NoError(t, s.Runs.UpdateRunRuntimeSnapshot(ctx, first.ID, "workspace_coordinator", first.Payload, "session"))
	require.NoError(t, s.Runs.FinishRun(ctx, first.ID, "failed", nil))
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1/orchestration"), &Handler{Service: s})
	path := "/api/v1/orchestration/tasks/" + task + "/retry"
	for i := 0; i < 2; i++ {
		require.Equal(t, 202, runtimeRequest(t, router, "POST", path, "", "", map[string]string{"session_id": "session", "action": "resume"}).Code)
	}
	next, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	require.NotEqual(t, first.ID, next.ID)
	require.Equal(t, first.Payload, next.Payload)
	s.Start = func(ctx context.Context, l Launch) error {
		require.NoError(t, l.OnSessionPrepared(ctx, "session"))
		claims, err := s.Auth.ValidateAgentJWT(l.Env["KANDEV_RUN_TOKEN"])
		require.NoError(t, err)
		require.Equal(t, next.ID, claims.RunID)
		require.Equal(t, "session", claims.SessionID)
		return nil
	}
	handled, err := s.Process(ctx, next)
	require.NoError(t, err)
	require.True(t, handled)
}
