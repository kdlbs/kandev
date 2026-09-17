package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

func assistantRouter(s *Service, users ...string) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		user := c.GetHeader("X-Test-User")
		if user == "" {
			user = "owner"
		}
		if len(users) > 0 {
			user = users[0]
		}
		authn.SetOnGin(c, authn.Identity{UserID: user, Role: authn.RoleMember})
	})
	RegisterRoutes(router.Group("/api/v1/orchestration"), &Handler{Service: s})
	return router
}

func TestAssistantBindingPrivateConversationRejectsOtherUsers(t *testing.T) {
	s, _, task := newRuntime(t)
	owner, foreign := assistantRouter(s), assistantRouter(s, "other-user")
	path := "/api/v1/orchestration/assistant"
	require.Equal(t, 200, runtimeRequest(t, owner, "PUT", path, "", "", map[string]any{"orchestrator_id": "chief"}).Code)
	require.Equal(t, 404, runtimeRequest(t, foreign, "GET", path, "", "", nil).Code)
	comments := "/api/v1/orchestration/tasks/" + task + "/comments"
	require.Equal(t, 404, runtimeRequest(t, foreign, "GET", comments, "", "", nil).Code)
	require.Equal(t, 404, runtimeRequest(t, foreign, "POST", comments, "", "", map[string]string{"body": "Inject instructions"}).Code)
}

func TestAssistantIntakeRepairsAcceptedMessageAtRestart(t *testing.T) {
	s, db, task := newRuntime(t)
	queue := s.Queue
	s.Queue = nil
	path := "/api/v1/orchestration/tasks/" + task + "/comments"
	require.Equal(t, 201, runtimeRequest(t, assistantRouter(s), "POST", path, "", "", map[string]string{"body": "Inspect only", "client_message_id": "offline"}).Code)
	s.Queue = queue
	require.NoError(t, s.RecoverInterrupted(context.Background()))
	var count int
	require.NoError(t, db.Get(&count, "SELECT count(*) FROM runs"))
	require.Equal(t, 1, count)
	require.NoError(t, s.RecoverInterrupted(context.Background()))
	require.NoError(t, db.Get(&count, "SELECT count(*) FROM runs"))
	require.Equal(t, 1, count)
}

func TestAssistantIntentNewMessageRevokesOldRunWrites(t *testing.T) {
	s, _, task := newRuntime(t)
	ctx := context.Background()
	human := assistantRouter(s)
	path := "/api/v1/orchestration/tasks/" + task + "/comments"
	require.Equal(t, 201, runtimeRequest(t, human, "POST", path, "", "", map[string]string{"body": "Work on the report"}).Code)
	run, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	require.NoError(t, s.Runs.UpdateRunRuntimeSnapshot(ctx, run.ID, "workspace_coordinator", run.Payload, "session"))
	token, err := s.Auth.MintRuntimeJWT("chief", task, "ws", run.ID, "session", "workspace_coordinator")
	require.NoError(t, err)
	require.Equal(t, 201, runtimeRequest(t, human, "POST", path, "", "", map[string]string{"body": "Stop changing things. Read only."}).Code)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1/orchestration", runtimeauth.Middleware(s.Auth, s.Personas)), &Handler{Service: s})
	result := runtimeRequest(t, router, "PUT", "/api/v1/orchestration/agents/chief/memory", token, run.ID, map[string]any{
		"entries": []map[string]string{{"layer": "user", "key": "policy", "content": "Old authority"}},
	})
	require.Equal(t, 409, result.Code, result.Body.String())
	rows, err := s.Repo.ListAgentMemory(ctx, "chief")
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestAssistantIntakeDuplicateIsOneCommentAndRun(t *testing.T) {
	s, db, task := newRuntime(t)
	router := assistantRouter(s)
	path := "/api/v1/orchestration/tasks/" + task + "/comments"
	body := map[string]string{"body": "Inspect the workspace only.", "client_message_id": "client-1"}
	first := runtimeRequest(t, router, http.MethodPost, path, "", "", body)
	require.Equal(t, 201, first.Code, first.Body.String())
	second := runtimeRequest(t, router, http.MethodPost, path, "", "", body)
	require.Equal(t, 200, second.Code, second.Body.String())
	var a, b map[string]any
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &a))
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &b))
	require.Equal(t, a["id"], b["id"])
	require.NotEmpty(t, a["run_id"])
	require.EqualValues(t, 1, b["sequence"])
	var count int
	require.NoError(t, db.Get(&count, "SELECT count(*) FROM runs"))
	require.Equal(t, 1, count)
	body["body"] = "Different instructions"
	conflict := runtimeRequest(t, router, http.MethodPost, path, "", "", body)
	require.Equal(t, 409, conflict.Code)
	rows, err := s.Repo.ListComments(context.Background(), task, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

func TestAssistantIntakeStartupBeforeQueueIsReadyKeepsOutbox(t *testing.T) {
	s, _, task := newRuntime(t)
	s.Queue = nil
	path := "/api/v1/orchestration/tasks/" + task + "/comments"
	require.Equal(t, 201, runtimeRequest(t, assistantRouter(s), "POST", path, "", "", map[string]string{"body": "Inspect only"}).Code)
	require.NoError(t, s.RecoverInterrupted(context.Background()))
	rows, err := s.Repo.PendingIntake(context.Background())
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

func TestAssistantIntakePersistsWhenQueueUnavailable(t *testing.T) {
	s, _, task := newRuntime(t)
	s.Queue = nil
	router := assistantRouter(s)
	path := "/api/v1/orchestration/tasks/" + task + "/comments"
	result := runtimeRequest(t, router, http.MethodPost, path, "", "", map[string]string{
		"body": "Read the report.", "client_message_id": "offline",
	})
	require.Equal(t, 201, result.Code, result.Body.String())
	var receipt map[string]any
	require.NoError(t, json.Unmarshal(result.Body.Bytes(), &receipt))
	require.Equal(t, "accepted", receipt["receipt_status"])
	rows, err := s.Repo.ListComments(context.Background(), task, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

func TestAssistantIntentExactSourceSurvivesNewerComments(t *testing.T) {
	s, _, task := newRuntime(t)
	ctx := context.Background()
	original := &models.TaskComment{
		ID: "original", TaskID: task, AuthorType: "user", AuthorID: "owner",
		Body:   "ORIGINAL_BOUNDARY: inspect only, do not change files.",
		Source: "user", CreatedAt: time.Now().UTC().Add(-time.Hour),
	}
	require.NoError(t, s.Repo.PutComment(ctx, original))
	for i := 0; i < 105; i++ {
		require.NoError(t, s.Repo.PutComment(ctx, &models.TaskComment{
			ID: fmt.Sprintf("newer-%03d", i), TaskID: task, AuthorType: "user", AuthorID: "owner",
			Body: "A later message", Source: "user", CreatedAt: original.CreatedAt.Add(time.Duration(i+1) * time.Second),
		}))
	}
	a, err := s.Personas.GetAgentInstance(ctx, "chief")
	require.NoError(t, err)
	prompt, err := s.prompt(ctx, a, task, map[string]any{"comment_id": original.ID})
	require.NoError(t, err)
	require.Contains(t, prompt, original.Body)
}

func TestAssistantBindingSelectsExistingConversation(t *testing.T) {
	s, _, task := newRuntime(t)
	router := assistantRouter(s)
	path := "/api/v1/orchestration/assistant"
	result := runtimeRequest(t, router, http.MethodPut, path, "", "", map[string]any{
		"orchestrator_id": "chief", "expected_version": 0,
	})
	require.Equal(t, 200, result.Code, result.Body.String())
	var binding map[string]any
	require.NoError(t, json.Unmarshal(result.Body.Bytes(), &binding))
	require.Equal(t, task, binding["conversation_id"])
	require.Equal(t, "owner", binding["owner_user_id"])
	require.EqualValues(t, 1, binding["version"])
	read := runtimeRequest(t, router, http.MethodGet, path, "", "", nil)
	require.Equal(t, 200, read.Code)
	require.JSONEq(t, result.Body.String(), read.Body.String())
	stale := runtimeRequest(t, router, http.MethodPut, path, "", "", map[string]any{
		"orchestrator_id": "chief", "expected_version": 0,
	})
	require.Equal(t, 409, stale.Code)
}
