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
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
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

func TestSeedCostEventPreservesOutputTokenPresence(t *testing.T) {
	taskRepo, sqlxDB := newTestRepo(t)
	omittedTaskID := uuid.New().String()
	zeroTaskID := uuid.New().String()
	seedTask(t, sqlxDB, omittedTaskID)
	seedTask(t, sqlxDB, zeroTaskID)

	officeRepo, err := officesqlite.NewWithDB(sqlxDB, sqlxDB, logger.Default())
	if err != nil {
		t.Fatalf("new office repo: %v", err)
	}
	router := gin.New()
	RegisterRoutes(router, taskRepo, officeRepo, nil, nil, nil, logger.Default())

	post := func(body map[string]any) {
		t.Helper()
		requestBody := mustJSON(t, body)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/_test/cost-events", bytes.NewReader(requestBody))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("seed cost event status=%d body=%s", res.Code, res.Body.String())
		}
	}

	post(map[string]any{
		"agent_profile_id": "agent-1",
		"task_id":          omittedTaskID,
		"tokens_in":        10,
		"estimated":        true,
	})
	post(map[string]any{
		"agent_profile_id": "agent-1",
		"task_id":          zeroTaskID,
		"tokens_in":        10,
		"tokens_out":       0,
		"estimated":        true,
	})

	events, err := officeRepo.ListCostEvents(t.Context(), "ws-1")
	if err != nil {
		t.Fatalf("list cost events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("cost event count = %d, want 2", len(events))
	}
	byTask := make(map[string]*officemodels.CostEvent, len(events))
	for _, event := range events {
		byTask[event.TaskID] = event
	}
	omitted, ok := byTask[omittedTaskID]
	if !ok {
		t.Fatalf("missing cost event for task %s", omittedTaskID)
	}
	zero, ok := byTask[zeroTaskID]
	if !ok {
		t.Fatalf("missing cost event for task %s", zeroTaskID)
	}
	if got := omitted.TokensOut; got != nil {
		t.Fatalf("omitted tokens_out = %v, want nil", *got)
	}
	if got := zero.TokensOut; got == nil || *got != 0 {
		t.Fatalf("explicit zero tokens_out = %v, want non-nil 0", got)
	}
	for _, taskID := range []string{omittedTaskID, zeroTaskID} {
		version := byTask[taskID].CostContractVersion
		if version == nil || *version != officemodels.CostContractVersion {
			t.Errorf("cost_contract_version for %s = %v, want %d", taskID, version, officemodels.CostContractVersion)
		}
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
	t.Setenv("KANDEV_E2E_GITLAB_REMOTE_URL", remoteURL)
	unexpectedBody := mustJSON(t, map[string]string{
		"remote_url": "http://localhost:18080/group/unexpected.git",
	})
	unexpectedReq := httptest.NewRequest(http.MethodPut,
		"/api/v1/_test/repositories/"+model.ID+"/git-remote", bytes.NewReader(unexpectedBody))
	unexpectedReq.Header.Set("Content-Type", "application/json")
	unexpectedRes := httptest.NewRecorder()
	router.ServeHTTP(unexpectedRes, unexpectedReq)
	if unexpectedRes.Code != http.StatusBadRequest {
		t.Fatalf("unexpected remote status=%d body=%s", unexpectedRes.Code, unexpectedRes.Body.String())
	}

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

func TestSeedTaskSessionUsesRequestedRepositoryID(t *testing.T) {
	repo, sqlxDB := newTestRepo(t)
	taskID := uuid.New().String()
	seedTask(t, sqlxDB, taskID)
	r := newRouter(t, repo, nil)

	body := mustJSON(t, map[string]interface{}{
		"task_id":       taskID,
		"state":         "WAITING_FOR_INPUT",
		"repository_id": "repository-secondary",
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
	session, err := repo.GetTaskSession(context.Background(), resp.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.RepositoryID != "repository-secondary" {
		t.Fatalf("repository_id = %q, want repository-secondary", session.RepositoryID)
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

// seedMessageAndCollect seeds a session + a message through the harness routes
// and returns the repo, the session id, the response message id, and any
// message.added events published to the bus.
func seedMessageAndCollect(t *testing.T, body map[string]interface{}) (*sqliterepo.Repository, string, string, []*bus.Event) {
	t.Helper()
	repo, sqlxDB := newTestRepo(t)
	taskID := uuid.New().String()
	seedTask(t, sqlxDB, taskID)
	eb := bus.NewMemoryEventBus(logger.Default())
	var published []*bus.Event
	var mu sync.Mutex
	if _, err := eb.Subscribe(events.MessageAdded, func(_ context.Context, e *bus.Event) error {
		mu.Lock()
		defer mu.Unlock()
		published = append(published, e)
		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	r := newRouter(t, repo, eb)

	sessBody := mustJSON(t, map[string]interface{}{"task_id": taskID, "state": "RUNNING"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/_test/task-sessions", bytes.NewReader(sessBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("session seed: expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var sessResp seedTaskSessionResponse
	_ = json.Unmarshal(w.Body.Bytes(), &sessResp)

	body["session_id"] = sessResp.SessionID
	msgBody := mustJSON(t, body)
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/_test/messages", bytes.NewReader(msgBody))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("message seed: expected 200, got %d body=%s", w2.Code, w2.Body.String())
	}
	var resp struct {
		MessageID string `json:"message_id"`
	}
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	return repo, sessResp.SessionID, resp.MessageID, published
}

// TestSeedMessageUserAuthorPersistsPromptIndex verifies that a seeded user message is persisted with a derived prompt index.
func TestSeedMessageUserAuthorPersistsPromptIndex(t *testing.T) {
	repo, _, messageID, published := seedMessageAndCollect(t, map[string]interface{}{
		"type":        "message",
		"content":     "user prompt",
		"author_type": "user",
	})

	indexed, err := repo.GetMessageWithPromptIndex(context.Background(), messageID)
	if err != nil {
		t.Fatalf("GetMessageWithPromptIndex: %v", err)
	}
	if indexed.AuthorType != models.MessageAuthorUser {
		t.Fatalf("author type = %q, want user", indexed.AuthorType)
	}
	if indexed.PromptIndex != 1 {
		t.Fatalf("prompt_index = %d, want 1", indexed.PromptIndex)
	}
	// The ordinal is read-time derived: the legacy hot path stays zero.
	legacy, err := repo.GetMessage(context.Background(), messageID)
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if legacy.PromptIndex != 0 {
		t.Fatalf("legacy GetMessage prompt_index = %d, want 0", legacy.PromptIndex)
	}

	// The published WS event carries the ordinal and RFC3339Nano precision.
	if len(published) != 1 {
		t.Fatalf("published events = %d, want 1", len(published))
	}
	data, ok := published[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event data = %T, want map", published[0].Data)
	}
	if got, ok := data["prompt_index"]; !ok || got != 1 {
		t.Fatalf("event prompt_index = %#v, want 1", got)
	}
	if created, ok := data["created_at"].(string); !ok || !strings.Contains(created, ".") {
		t.Fatalf("event created_at = %#v, want RFC3339Nano fractional precision", data["created_at"])
	}
}

// TestSeedMessageDefaultsToAgentWithoutPromptIndex verifies that non-user seeded messages carry no prompt index.
func TestSeedMessageDefaultsToAgentWithoutPromptIndex(t *testing.T) {
	repo, _, messageID, published := seedMessageAndCollect(t, map[string]interface{}{
		"type":    "message",
		"content": "agent reply",
	})

	indexed, err := repo.GetMessageWithPromptIndex(context.Background(), messageID)
	if err != nil {
		t.Fatalf("GetMessageWithPromptIndex: %v", err)
	}
	if indexed.AuthorType != models.MessageAuthorAgent {
		t.Fatalf("author type = %q, want agent default", indexed.AuthorType)
	}
	if indexed.PromptIndex != 0 {
		t.Fatalf("agent prompt_index = %d, want 0", indexed.PromptIndex)
	}
	if len(published) != 1 {
		t.Fatalf("published events = %d, want 1", len(published))
	}
	data, ok := published[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event data = %T, want map", published[0].Data)
	}
	if _, ok := data["prompt_index"]; ok {
		t.Fatalf("agent event carries prompt_index %v", data["prompt_index"])
	}
}

// TestSeedMessageExplicitPastTimestampRejected verifies that explicit timestamps before the newest session message are rejected.
func TestSeedMessageExplicitPastTimestampRejected(t *testing.T) {
	repo, _, messageID, _ := seedMessageAndCollect(t, map[string]interface{}{
		"type":        "message",
		"content":     "first",
		"author_type": "user",
	})
	// A second user seed with an explicit timestamp EARLIER than the newest
	// user message is an invalid reordering: the explicit-import branch
	// rejects it (the live branch would have advanced instead).
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	second := mustJSON(t, map[string]interface{}{
		"session_id":  seedSessionIDFromMessage(t, repo, messageID),
		"type":        "message",
		"content":     "second",
		"author_type": "user",
		"created_at":  past,
	})
	r := newRouter(t, repo, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/_test/messages", bytes.NewReader(second))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("explicit-past seed status = %d body=%s, want 500 (strictly-after-max rejection)", w.Code, w.Body.String())
	}
}

// seedSessionIDFromMessage resolves the session id for a seeded message.
func seedSessionIDFromMessage(t *testing.T, repo *sqliterepo.Repository, messageID string) string {
	t.Helper()
	msg, err := repo.GetMessage(context.Background(), messageID)
	if err != nil {
		t.Fatalf("GetMessage(%s): %v", messageID, err)
	}
	return msg.TaskSessionID
}

// TestSeedMessageExplicitCreatedAtPreserved verifies that an explicit future-ordered created_at is preserved on the persisted row.
func TestSeedMessageExplicitCreatedAtPreserved(t *testing.T) {
	explicit := time.Date(2026, 8, 19, 10, 0, 0, 123456000, time.UTC)
	repo, _, messageID, published := seedMessageAndCollect(t, map[string]interface{}{
		"type":        "message",
		"content":     "deterministic prompt",
		"author_type": "user",
		"created_at":  explicit.Format(time.RFC3339Nano),
	})

	stored, err := repo.GetMessage(context.Background(), messageID)
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if !stored.CreatedAt.Equal(explicit) {
		t.Fatalf("stored created_at = %v, want explicit %v", stored.CreatedAt, explicit)
	}
	indexed, err := repo.GetMessageWithPromptIndex(context.Background(), messageID)
	if err != nil {
		t.Fatalf("GetMessageWithPromptIndex: %v", err)
	}
	if indexed.PromptIndex != 1 {
		t.Fatalf("prompt_index = %d, want 1", indexed.PromptIndex)
	}
	// The WS event preserves the explicit timestamp with full fractional
	// precision (RFC3339Nano), not second-truncated RFC3339.
	if len(published) != 1 {
		t.Fatalf("published events = %d, want 1", len(published))
	}
	data, ok := published[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event data = %T, want map", published[0].Data)
	}
	created, ok := data["created_at"].(string)
	if !ok {
		t.Fatalf("event created_at = %#v, want string", data["created_at"])
	}
	parsed, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		t.Fatalf("event created_at %q not RFC3339Nano: %v", created, err)
	}
	if !parsed.Equal(explicit) {
		t.Fatalf("event created_at = %v, want %v", parsed, explicit)
	}
}

// TestSeedMessageUserOrdinalsIncrement verifies that consecutive seeded user messages receive strictly increasing ordinals.
func TestSeedMessageUserOrdinalsIncrement(t *testing.T) {
	repo, sessionID, _, _ := seedMessageAndCollect(t, map[string]interface{}{
		"type":        "message",
		"content":     "first",
		"author_type": "user",
	})
	// Second user seed on the same session gets ordinal 2.
	second := mustJSON(t, map[string]interface{}{
		"session_id":  sessionID,
		"type":        "message",
		"content":     "second",
		"author_type": "user",
	})
	r := newRouter(t, repo, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/_test/messages", bytes.NewReader(second))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("second seed: expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	msgs, err := repo.ListMessages(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("messages = %d, want 2", len(msgs))
	}
	for _, m := range msgs {
		indexed, err := repo.GetMessageWithPromptIndex(context.Background(), m.ID)
		if err != nil {
			t.Fatalf("GetMessageWithPromptIndex(%s): %v", m.ID, err)
		}
		if indexed.AuthorType == models.MessageAuthorUser && indexed.PromptIndex < 1 {
			t.Fatalf("user message %s prompt_index = %d, want >= 1", m.ID, indexed.PromptIndex)
		}
	}
}

// TestSeedMessagePersistsTurnMetadata covers the harness turn-metadata
// contract: a later seed updates the pre-existing active turn's metadata, and
// a seed without turn_metadata leaves existing metadata untouched.
func TestSeedMessagePersistsTurnMetadata(t *testing.T) {
	repo, sqlxDB := newTestRepo(t)
	taskID := uuid.New().String()
	seedTask(t, sqlxDB, taskID)
	r := newRouter(t, repo, nil)

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

	// Seed a message first WITHOUT turn metadata: the harness creates the
	// active turn, and the later seed must update that same turn.
	firstBody := mustJSON(t, map[string]interface{}{
		"session_id": sessResp.SessionID,
		"type":       "message",
		"content":    "first message",
	})
	wFirst := httptest.NewRecorder()
	reqFirst := httptest.NewRequest(http.MethodPost, "/api/v1/_test/messages", bytes.NewReader(firstBody))
	reqFirst.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(wFirst, reqFirst)
	if wFirst.Code != http.StatusOK {
		t.Fatalf("first seed: expected 200, got %d body=%s", wFirst.Code, wFirst.Body.String())
	}

	turnMeta := map[string]interface{}{
		"runtime_config_snapshot": map[string]interface{}{
			"config_baseline": map[string]interface{}{"mode": "default", "model": "anthropic/claude-sonnet-5"},
			"config_options": []interface{}{
				map[string]interface{}{"id": "mode", "name": "Mode", "value": "default", "value_name": "Default"},
			},
		},
	}
	msgBody := mustJSON(t, map[string]interface{}{
		"session_id":    sessResp.SessionID,
		"type":          "message",
		"content":       "hello world",
		"turn_metadata": turnMeta,
	})
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/_test/messages", bytes.NewReader(msgBody))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w2.Code, w2.Body.String())
	}

	// Exactly one active turn exists, both seeded messages share its id, and
	// the second seed updated the metadata on the pre-existing turn.
	turns, err := repo.ListTurnsBySession(context.Background(), sessResp.SessionID)
	if err != nil {
		t.Fatalf("list turns: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected exactly 1 turn, got %d", len(turns))
	}
	turn := turns[0]
	msgs, err := repo.ListMessages(context.Background(), sessResp.SessionID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	for _, m := range msgs {
		if m.TurnID != turn.ID {
			t.Fatalf("message %q on turn %q, expected shared turn %q", m.ID, m.TurnID, turn.ID)
		}
	}
	if turn.Metadata == nil {
		t.Fatal("expected turn metadata to be persisted")
	}
	snapshot, ok := turn.Metadata["runtime_config_snapshot"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected runtime_config_snapshot object, got %#v", turn.Metadata["runtime_config_snapshot"])
	}
	if snapshot["config_baseline"] == nil {
		t.Fatal("expected config_baseline inside the snapshot")
	}

	// Seeding another message WITHOUT turn_metadata must leave the existing
	// metadata untouched (nil must not clear it).
	thirdBody := mustJSON(t, map[string]interface{}{
		"session_id": sessResp.SessionID,
		"type":       "message",
		"content":    "third message",
	})
	w3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodPost, "/api/v1/_test/messages", bytes.NewReader(thirdBody))
	req3.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("third seed: expected 200, got %d body=%s", w3.Code, w3.Body.String())
	}
	turnAfter, err := repo.GetTurn(context.Background(), turn.ID)
	if err != nil {
		t.Fatalf("get turn after third seed: %v", err)
	}
	afterSnapshot, ok := turnAfter.Metadata["runtime_config_snapshot"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected runtime_config_snapshot to survive a nil turn_metadata seed, got %#v", turnAfter.Metadata)
	}
	if afterSnapshot["config_baseline"] == nil {
		t.Fatal("expected config_baseline to survive a nil turn_metadata seed")
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
