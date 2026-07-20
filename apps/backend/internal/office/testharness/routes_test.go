package testharness

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newTestRepo creates an in-memory SQLite repo for the test.
func newTestRepo(t *testing.T) (*sqliterepo.Repository, *sqlx.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	conn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(conn, "sqlite3")
	repo, err := sqliterepo.NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	t.Cleanup(func() { _ = sqlxDB.Close() })
	return repo, sqlxDB
}

// seedTask inserts a minimal tasks row directly so the validation pass succeeds.
func seedTask(t *testing.T, db *sqlx.DB, taskID string) {
	t.Helper()
	now := time.Now().UTC()
	_, err := db.Exec(`
		INSERT INTO tasks (id, workspace_id, workflow_id, workflow_step_id, title, description, state, created_at, updated_at)
		VALUES (?, 'ws-1', '', '', 'seeded test task', '', 'TODO', ?, ?)
	`, taskID, now, now)
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}
}

// newRouter sets up gin engine with the test routes mounted.
func newRouter(t *testing.T, repo *sqliterepo.Repository, eb bus.EventBus) *gin.Engine {
	t.Helper()
	r := gin.New()
	RegisterRoutes(r, repo, nil, nil, nil, eb, logger.Default())
	return r
}

func TestRoutesNotMountedByDefault(t *testing.T) {
	// When RegisterRoutes is NOT called, the routes must 404. This is the
	// production behaviour — the env-var gate in helpers.go ensures the
	// caller only invokes RegisterRoutes when KANDEV_E2E_MOCK=true.
	r := gin.New()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/_test/task-sessions", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when routes are not registered, got %d", w.Code)
	}
}

func TestEnabledReadsEnvVar(t *testing.T) {
	t.Setenv(EnvVar, "true")
	if !Enabled() {
		t.Fatalf("Enabled() should be true when %s=true", EnvVar)
	}
	t.Setenv(EnvVar, "false")
	if Enabled() {
		t.Fatalf("Enabled() should be false when %s=false", EnvVar)
	}
	t.Setenv(EnvVar, "1")
	if Enabled() {
		t.Fatalf("Enabled() should require literal 'true', not '1'")
	}
}

func TestHealthRouteReturnsOK(t *testing.T) {
	repo, _ := newTestRepo(t)
	r := newRouter(t, repo, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/_test/health", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestConfigureGitRemoteRoute(t *testing.T) {
	repo, sqlxDB := newTestRepo(t)
	now := time.Now().UTC()
	if _, err := sqlxDB.Exec(`INSERT INTO workspaces (id, name, created_at, updated_at)
		VALUES ('ws-1', 'Test workspace', ?, ?)`, now, now); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	repoDir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if output, err := exec.Command("git", "init", "-b", "main", repoDir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	model := &models.Repository{WorkspaceID: "ws-1", Name: "repo", SourceType: "local", LocalPath: repoDir}
	if err := repo.CreateRepository(t.Context(), model); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "gitlab-push")
	record := filepath.Join(t.TempDir(), "gitlab-push-record")
	t.Setenv("KANDEV_E2E_GITLAB_PUSH_FILE", marker)
	t.Setenv("KANDEV_E2E_GITLAB_PUSH_RECORD_FILE", record)
	router := newRouter(t, repo, nil)
	remoteURL := "http://localhost:18080/group/project.git"
	body := mustJSON(t, map[string]string{"remote_url": remoteURL})
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/_test/repositories/"+model.ID+"/git-remote", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	output, err := exec.Command("git", "-C", repoDir, "remote", "get-url", "origin").Output()
	if err != nil || strings.TrimSpace(string(output)) != remoteURL {
		t.Fatalf("remote=%q err=%v", output, err)
	}
	markerValue, err := os.ReadFile(marker)
	if err != nil || string(markerValue) != remoteURL {
		t.Fatalf("marker=%q err=%v", markerValue, err)
	}
	if err := os.WriteFile(record, []byte("push --set-upstream origin HEAD\n"), 0o600); err != nil {
		t.Fatalf("write push record: %v", err)
	}
	pushReq := httptest.NewRequest(http.MethodGet,
		"/api/v1/_test/repositories/"+model.ID+"/git-push-record", nil)
	pushRes := httptest.NewRecorder()
	router.ServeHTTP(pushRes, pushReq)
	if pushRes.Code != http.StatusOK || !strings.Contains(pushRes.Body.String(), "origin HEAD") {
		t.Fatalf("push record status=%d body=%s", pushRes.Code, pushRes.Body.String())
	}
	remove := httptest.NewRequest(http.MethodDelete,
		"/api/v1/_test/repositories/"+model.ID+"/git-remote", nil)
	removed := httptest.NewRecorder()
	router.ServeHTTP(removed, remove)
	if removed.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", removed.Code, removed.Body.String())
	}
	if err := exec.Command("git", "-C", repoDir, "remote", "get-url", "origin").Run(); err == nil {
		t.Fatal("origin remote still exists after cleanup")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("push marker still exists after cleanup: %v", err)
	}
	if _, err := os.Stat(record); !os.IsNotExist(err) {
		t.Fatalf("push record still exists after cleanup: %v", err)
	}
}

func TestSeedTaskSessionHappyPath(t *testing.T) {
	repo, sqlxDB := newTestRepo(t)
	taskID := uuid.New().String()
	seedTask(t, sqlxDB, taskID)
	eb := bus.NewMemoryEventBus(logger.Default())

	r := newRouter(t, repo, eb)
	body := mustJSON(t, map[string]interface{}{
		"task_id": taskID,
		"state":   "RUNNING",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/_test/task-sessions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp seedTaskSessionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.SessionID == "" {
		t.Fatal("expected session_id in response")
	}
	// Confirm the row was actually written.
	session, err := repo.GetTaskSession(context.Background(), resp.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.State != models.TaskSessionStateRunning {
		t.Fatalf("expected RUNNING, got %s", session.State)
	}
}

func TestSeedTaskSessionUsesRequestedSessionID(t *testing.T) {
	repo, sqlxDB := newTestRepo(t)
	taskID := uuid.New().String()
	sessionID := uuid.New().String()
	seedTask(t, sqlxDB, taskID)
	r := newRouter(t, repo, nil)

	body := mustJSON(t, map[string]interface{}{
		"task_id":    taskID,
		"session_id": sessionID,
		"state":      "RUNNING",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/_test/task-sessions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp seedTaskSessionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.SessionID != sessionID {
		t.Fatalf("session_id = %q, want requested %q", resp.SessionID, sessionID)
	}
	session, err := repo.GetTaskSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("get requested session: %v", err)
	}
	if session.TaskID != taskID {
		t.Fatalf("task_id = %q, want %q", session.TaskID, taskID)
	}
}

func TestSeedTaskSessionTerminalRequiresCompletedAt(t *testing.T) {
	repo, sqlxDB := newTestRepo(t)
	taskID := uuid.New().String()
	seedTask(t, sqlxDB, taskID)

	r := newRouter(t, repo, nil)
	body := mustJSON(t, map[string]interface{}{
		"task_id": taskID,
		"state":   "COMPLETED",
		// completed_at intentionally missing
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/_test/task-sessions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestSeedTaskSessionTerminalWithCompletedAtSucceeds(t *testing.T) {
	repo, sqlxDB := newTestRepo(t)
	taskID := uuid.New().String()
	seedTask(t, sqlxDB, taskID)

	r := newRouter(t, repo, nil)
	startedAt := time.Now().Add(-30 * time.Second).UTC().Format(time.RFC3339)
	completedAt := time.Now().UTC().Format(time.RFC3339)
	body := mustJSON(t, map[string]interface{}{
		"task_id":       taskID,
		"state":         "COMPLETED",
		"started_at":    startedAt,
		"completed_at":  completedAt,
		"command_count": 3,
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/_test/task-sessions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp seedTaskSessionResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	msgs, err := repo.ListMessages(context.Background(), resp.SessionID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 tool_call messages, got %d", len(msgs))
	}
	for _, m := range msgs {
		if m.Type != models.MessageTypeToolCall {
			t.Fatalf("expected tool_call, got %s", m.Type)
		}
	}
}

func TestSeedTaskSessionRejectsUnknownTask(t *testing.T) {
	repo, _ := newTestRepo(t)
	r := newRouter(t, repo, nil)
	body := mustJSON(t, map[string]interface{}{
		"task_id": "does-not-exist",
		"state":   "RUNNING",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/_test/task-sessions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestSeedTaskSessionRejectsBadState(t *testing.T) {
	repo, sqlxDB := newTestRepo(t)
	taskID := uuid.New().String()
	seedTask(t, sqlxDB, taskID)
	r := newRouter(t, repo, nil)
	body := mustJSON(t, map[string]interface{}{
		"task_id": taskID,
		"state":   "WAT",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/_test/task-sessions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestSeedMessageHappyPath(t *testing.T) {
	repo, sqlxDB := newTestRepo(t)
	taskID := uuid.New().String()
	seedTask(t, sqlxDB, taskID)
	r := newRouter(t, repo, nil)

	// Seed a session first.
	sessBody := mustJSON(t, map[string]interface{}{
		"task_id": taskID,
		"state":   "RUNNING",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/_test/task-sessions", bytes.NewReader(sessBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("session seed: expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var sessResp seedTaskSessionResponse
	_ = json.Unmarshal(w.Body.Bytes(), &sessResp)

	msgBody := mustJSON(t, map[string]interface{}{
		"session_id": sessResp.SessionID,
		"type":       "message",
		"content":    "hello world",
	})
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/_test/messages", bytes.NewReader(msgBody))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w2.Code, w2.Body.String())
	}
	msgs, err := repo.ListMessages(context.Background(), sessResp.SessionID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "hello world" {
		t.Fatalf("unexpected content: %q", msgs[0].Content)
	}
}

func TestSeedMessageRejectsUnknownSession(t *testing.T) {
	repo, _ := newTestRepo(t)
	r := newRouter(t, repo, nil)
	body := mustJSON(t, map[string]interface{}{
		"session_id": "does-not-exist",
		"type":       "message",
		"content":    "hi",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/_test/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

// The depth-1 guard lives in the task SERVICE; the harness writes through the
// repository so e2e tests can build arbitrary parent_id chains (depth >= 2),
// which is the only way to exercise the sidebar's multi-level rendering.
func TestSeedTaskAllowsArbitraryDepthChain(t *testing.T) {
	repo, _ := newTestRepo(t)
	r := newRouter(t, repo, nil)

	create := func(title, parentID string) string {
		body := mustJSON(t, map[string]interface{}{
			"workspace_id":     "ws-1",
			"workflow_id":      "wf-1",
			"workflow_step_id": "step-1",
			"title":            title,
			"parent_id":        parentID,
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/_test/tasks", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 creating %q, got %d: %s", title, w.Code, w.Body.String())
		}
		var resp struct {
			TaskID string `json:"task_id"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		return resp.TaskID
	}

	root := create("Root", "")
	child := create("Child", root)
	grandchild := create("Grandchild", child) // depth 2 — would be rejected by the service guard

	got, err := repo.GetTask(context.Background(), grandchild)
	if err != nil {
		t.Fatalf("get grandchild: %v", err)
	}
	if got.ParentID != child {
		t.Fatalf("grandchild parent_id = %q, want %q (the child, i.e. depth 2)", got.ParentID, child)
	}
}

func TestSeedTaskRejectsMissingFields(t *testing.T) {
	repo, _ := newTestRepo(t)
	r := newRouter(t, repo, nil)

	body := mustJSON(t, map[string]interface{}{"workflow_id": "wf-1"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/_test/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing workspace_id/title, got %d", w.Code)
	}
}

func TestSeedTaskRejectsUnknownParent(t *testing.T) {
	repo, _ := newTestRepo(t)
	r := newRouter(t, repo, nil)

	body := mustJSON(t, map[string]interface{}{
		"workspace_id": "ws-1",
		"title":        "Orphan",
		"parent_id":    "does-not-exist",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/_test/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown parent_id, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSeedWorkflowPersistsStyle(t *testing.T) {
	repo, _ := newTestRepo(t)
	r := newRouter(t, repo, nil)

	body := mustJSON(t, map[string]interface{}{
		"workspace_id": "ws-1",
		"name":         "Office Only Workflow",
		"style":        models.WorkflowStyleOffice,
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/_test/workflows", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	workflows, err := repo.ListWorkflows(context.Background(), "ws-1", true)
	if err != nil {
		t.Fatalf("list workflows: %v", err)
	}
	if len(workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(workflows))
	}
	if got := workflows[0].Style; got != models.WorkflowStyleOffice {
		t.Fatalf("expected style %q, got %q", models.WorkflowStyleOffice, got)
	}
}

func TestSeedWorkflowRejectsMissingFields(t *testing.T) {
	repo, _ := newTestRepo(t)
	r := newRouter(t, repo, nil)

	body := mustJSON(t, map[string]interface{}{"style": "office"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/_test/workflows", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing workspace_id/name, got %d", w.Code)
	}
}

func mustJSON(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
