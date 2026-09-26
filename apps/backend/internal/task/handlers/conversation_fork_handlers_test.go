package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
)

func TestConversationForkDraftAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router, dbConn := newConversationForkTestRouter(t)
	defer func() { _ = dbConn.Close() }()
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-a", Role: authn.RoleMember})

	candidates := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/task-sessions/session-fork-http/fork-candidates?cutoff_message_id=message-fork-http", nil).WithContext(ctx)
	router.ServeHTTP(candidates, req)
	if candidates.Code != http.StatusOK {
		t.Fatalf("candidate status = %d, body = %s", candidates.Code, candidates.Body.String())
	}
	var candidateBody map[string]any
	if err := json.Unmarshal(candidates.Body.Bytes(), &candidateBody); err != nil {
		t.Fatalf("decode candidate response: %v", err)
	}
	if candidateBody["session_id"] != "session-fork-http" || candidateBody["revision"] == nil {
		t.Fatalf("candidate response = %#v", candidateBody)
	}

	create := httptest.NewRecorder()
	body := `{"cutoff_message_id":"message-fork-http","draft_request_id":"http-request-1","model_id":"model-a"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/task-sessions/session-fork-http/conversation-forks", strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(create, req)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", create.Code, create.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("created draft response = %+v", created)
	}

	get := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/conversation-forks/"+created.ID, nil).WithContext(ctx)
	router.ServeHTTP(get, req)
	if get.Code != http.StatusOK || strings.Contains(get.Body.String(), "Keep the HTTP source.") {
		t.Fatalf("get status = %d, body = %s", get.Code, get.Body.String())
	}
	content := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/conversation-forks/"+created.ID+"/content", nil).WithContext(ctx)
	router.ServeHTTP(content, req)
	if content.Code != http.StatusOK || !strings.Contains(content.Body.String(), "Keep the HTTP source.") || content.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("content status = %d, cache = %q, body = %s", content.Code, content.Header().Get("Cache-Control"), content.Body.String())
	}
	estimate := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/conversation-forks/"+created.ID+"/estimate", strings.NewReader(`{"model_id":"model-b"}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(estimate, req)
	if estimate.Code != http.StatusOK || !strings.Contains(estimate.Body.String(), `"model_id":"model-b"`) {
		t.Fatalf("estimate status = %d, body = %s", estimate.Code, estimate.Body.String())
	}

	foreign := httptest.NewRecorder()
	foreignCtx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-b", Role: authn.RoleMember})
	req = httptest.NewRequest(http.MethodGet, "/api/v1/conversation-forks/"+created.ID, nil).WithContext(foreignCtx)
	router.ServeHTTP(foreign, req)
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("foreign get status = %d, body = %s", foreign.Code, foreign.Body.String())
	}

	discard := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/conversation-forks/"+created.ID, nil).WithContext(ctx)
	router.ServeHTTP(discard, req)
	if discard.Code != http.StatusNoContent {
		t.Fatalf("discard status = %d, body = %s", discard.Code, discard.Body.String())
	}

	badCutoff := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/task-sessions/session-fork-http/fork-candidates", nil).WithContext(ctx)
	router.ServeHTTP(badCutoff, req)
	if badCutoff.Code != http.StatusBadRequest {
		t.Fatalf("missing cutoff status = %d, body = %s", badCutoff.Code, badCutoff.Body.String())
	}
}

func newConversationForkTestRouter(t *testing.T) (*gin.Engine, *sqlx.DB) {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "conversation-fork-handlers.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	store := sqlx.NewDb(dbConn, "sqlite3")
	repo, err := sqliterepo.NewWithDB(store, store, nil)
	if err != nil {
		_ = store.Close()
		t.Fatalf("new repository: %v", err)
	}
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo, Messages: repo,
		Attachments: repo, Turns: repo, Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo, Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	handlers := NewTaskHandlers(svc, nil, repo, nil, log)
	router := gin.New()
	handlers.registerHTTP(router)
	now := time.Now().UTC()
	if err := repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "workspace-fork-http", Name: "Fork HTTP", OwnerID: "user-a"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateTask(context.Background(), &models.Task{ID: "task-fork-http", WorkspaceID: "workspace-fork-http", Title: "Fork HTTP source"}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{ID: "session-fork-http", TaskID: "task-fork-http", State: models.TaskSessionStateCreated}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := repo.CreateTurn(context.Background(), &models.Turn{ID: "turn-fork-http", TaskSessionID: "session-fork-http", TaskID: "task-fork-http", StartedAt: now, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create turn: %v", err)
	}
	if err := repo.CreateMessage(context.Background(), &models.Message{
		ID: "message-fork-http", TaskSessionID: "session-fork-http", TaskID: "task-fork-http", TurnID: "turn-fork-http",
		AuthorType: models.MessageAuthorUser, Type: models.MessageTypeMessage, Content: "Keep the HTTP source.", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create message: %v", err)
	}
	return router, store
}
