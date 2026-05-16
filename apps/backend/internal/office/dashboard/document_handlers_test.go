package dashboard_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/office/dashboard"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
)

// documentTestDeps wires a real task repo + DocumentService for HTTP handler tests.
type documentTestDeps struct {
	db      *sqlx.DB
	repo    *tasksqlite.Repository
	svc     *taskservice.DocumentService
	handler *dashboard.DocumentHandler
	router  *gin.Engine
	tmpDir  string
}

func newDocumentTestDeps(t *testing.T) *documentTestDeps {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	rawDB, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })

	repo, err := tasksqlite.NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("new task repo: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	if _, err := worktree.NewSQLiteStore(sqlxDB, sqlxDB); err != nil {
		t.Fatalf("init worktree store: %v", err)
	}

	log := logger.Default()
	svc := taskservice.NewDocumentService(repo, log)
	h := dashboard.NewDocumentHandler(svc, tmpDir, log)

	router := gin.New()
	group := router.Group("/api/v1/office")
	dashboard.RegisterDocumentRoutes(group, h)

	return &documentTestDeps{
		db:      sqlxDB,
		repo:    repo,
		svc:     svc,
		handler: h,
		router:  router,
		tmpDir:  tmpDir,
	}
}

// seedTask inserts a minimal task row to satisfy the FK constraint on task_documents.
func seedTask(t *testing.T, d *documentTestDeps, taskID string) {
	t.Helper()
	_, err := d.db.Exec(`
		INSERT OR IGNORE INTO tasks (id, workspace_id, title, state, created_at, updated_at)
		VALUES (?, '', 'Test Task', 'todo', datetime('now'), datetime('now'))
	`, taskID)
	if err != nil {
		t.Fatalf("seed task %s: %v", taskID, err)
	}
}

func TestDocumentHandler_ListDocuments_Empty(t *testing.T) {
	deps := newDocumentTestDeps(t)
	seedTask(t, deps, "task1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/tasks/task1/documents", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp dashboard.DocumentListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Documents == nil {
		t.Error("expected non-nil documents slice")
	}
	if len(resp.Documents) != 0 {
		t.Errorf("expected 0 documents, got %d", len(resp.Documents))
	}
}

func TestDocumentHandler_CreateOrUpdate_AndGet(t *testing.T) {
	deps := newDocumentTestDeps(t)
	seedTask(t, deps, "task1")

	body := `{"type":"spec","title":"Feature Spec","content":"## Spec","author_kind":"agent","author_name":"Agent"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/office/tasks/task1/documents/spec",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("create: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var createResp dashboard.DocumentResponse
	if err := json.NewDecoder(w.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if createResp.Document == nil {
		t.Fatal("expected document in create response")
	}
	if createResp.Document.Key != "spec" {
		t.Errorf("expected key 'spec', got %q", createResp.Document.Key)
	}
	if createResp.Document.Content != "## Spec" {
		t.Errorf("expected content '## Spec', got %q", createResp.Document.Content)
	}
	if createResp.Document.ID == "" {
		t.Error("expected non-empty document ID")
	}

	// GET the document
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/office/tasks/task1/documents/spec", nil)
	w2 := httptest.NewRecorder()
	deps.router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var getResp dashboard.DocumentResponse
	if err := json.NewDecoder(w2.Body).Decode(&getResp); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if getResp.Document.Key != "spec" {
		t.Errorf("expected key 'spec', got %q", getResp.Document.Key)
	}
}

func TestDocumentHandler_GetDocument_NotFound(t *testing.T) {
	deps := newDocumentTestDeps(t)
	// No task seed needed — the document won't be found regardless

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/tasks/task1/documents/missing", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDocumentHandler_DeleteDocument(t *testing.T) {
	deps := newDocumentTestDeps(t)
	seedTask(t, deps, "task1")

	// Create first
	body := `{"title":"To Delete","content":"bye"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/office/tasks/task1/documents/del",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("create before delete: %d: %s", w.Code, w.Body.String())
	}

	// Delete
	req2 := httptest.NewRequest(http.MethodDelete, "/api/v1/office/tasks/task1/documents/del", nil)
	w2 := httptest.NewRecorder()
	deps.router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	// Verify gone
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/office/tasks/task1/documents/del", nil)
	w3 := httptest.NewRecorder()
	deps.router.ServeHTTP(w3, req3)

	if w3.Code != http.StatusNotFound {
		t.Fatalf("after delete: expected 404, got %d", w3.Code)
	}
}

func TestDocumentHandler_DeleteDocument_NotFound(t *testing.T) {
	deps := newDocumentTestDeps(t)
	// No task seed needed — delete of non-existent doc returns 404 regardless

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/office/tasks/task1/documents/nope", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDocumentHandler_ListRevisions(t *testing.T) {
	deps := newDocumentTestDeps(t)
	seedTask(t, deps, "task1")

	// Create a document (creates revision 1)
	body := `{"content":"v1"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/office/tasks/task1/documents/plan",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("create: %d: %s", w.Code, w.Body.String())
	}

	// List revisions
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/office/tasks/task1/documents/plan/revisions", nil)
	w2 := httptest.NewRecorder()
	deps.router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("list revisions: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var resp dashboard.DocumentRevisionListResponse
	if err := json.NewDecoder(w2.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Revisions) == 0 {
		t.Fatal("expected at least 1 revision")
	}
}

func TestDocumentHandler_RevertDocument(t *testing.T) {
	deps := newDocumentTestDeps(t)
	seedTask(t, deps, "task1")

	// Create document v1
	req1 := httptest.NewRequest(http.MethodPut, "/api/v1/office/tasks/task1/documents/notes",
		strings.NewReader(`{"content":"original","author_kind":"user","author_name":"Alice"}`))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	deps.router.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("create v1: %d: %s", w1.Code, w1.Body.String())
	}

	// List revisions to get revision ID
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/office/tasks/task1/documents/notes/revisions", nil)
	w2 := httptest.NewRecorder()
	deps.router.ServeHTTP(w2, req2)

	var revList dashboard.DocumentRevisionListResponse
	if err := json.NewDecoder(w2.Body).Decode(&revList); err != nil {
		t.Fatalf("decode revisions: %v", err)
	}
	if len(revList.Revisions) == 0 {
		t.Fatal("expected at least 1 revision")
	}
	revID := revList.Revisions[0].ID

	// Update the document so revert is meaningful
	req3 := httptest.NewRequest(http.MethodPut, "/api/v1/office/tasks/task1/documents/notes",
		strings.NewReader(`{"content":"updated","author_kind":"agent","author_name":"Bot"}`))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	deps.router.ServeHTTP(w3, req3)

	if w3.Code != http.StatusOK {
		t.Fatalf("update: %d: %s", w3.Code, w3.Body.String())
	}

	// Revert to original revision
	revertURL := "/api/v1/office/tasks/task1/documents/notes/revisions/" + revID + "/restore"
	req4 := httptest.NewRequest(http.MethodPost, revertURL, nil)
	w4 := httptest.NewRecorder()
	deps.router.ServeHTTP(w4, req4)

	if w4.Code != http.StatusOK {
		t.Fatalf("revert: expected 200, got %d: %s", w4.Code, w4.Body.String())
	}

	var revertResp dashboard.DocumentRevisionResponse
	if err := json.NewDecoder(w4.Body).Decode(&revertResp); err != nil {
		t.Fatalf("decode revert: %v", err)
	}
	if revertResp.Revision == nil {
		t.Fatal("expected revision in revert response")
	}
}

func TestDocumentHandler_UploadAndDownloadAttachment(t *testing.T) {
	deps := newDocumentTestDeps(t)
	seedTask(t, deps, "task1")

	// Build a multipart form with a file field
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "hello.txt")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := io.WriteString(fw, "hello attachment"); err != nil {
		t.Fatalf("write file content: %v", err)
	}
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/office/tasks/task1/documents/att1/upload",
		&buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("upload: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var uploadResp dashboard.DocumentResponse
	if err := json.NewDecoder(w.Body).Decode(&uploadResp); err != nil {
		t.Fatalf("decode upload: %v", err)
	}
	if uploadResp.Document == nil {
		t.Fatal("expected document in upload response")
	}
	if uploadResp.Document.Filename != "hello.txt" {
		t.Errorf("expected filename 'hello.txt', got %q", uploadResp.Document.Filename)
	}

	// Download the attachment
	req2 := httptest.NewRequest(http.MethodGet,
		"/api/v1/office/tasks/task1/documents/att1/download", nil)
	w2 := httptest.NewRecorder()
	deps.router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("download: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	content := w2.Body.String()
	if content != "hello attachment" {
		t.Errorf("expected content 'hello attachment', got %q", content)
	}
}

func TestDocumentHandler_CreateOrUpdate_InvalidJSON(t *testing.T) {
	deps := newDocumentTestDeps(t)
	// No seed needed — invalid JSON is rejected before any DB operation

	req := httptest.NewRequest(http.MethodPut, "/api/v1/office/tasks/task1/documents/bad",
		strings.NewReader(`not json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDocumentHandler_ListDocuments_Multiple(t *testing.T) {
	deps := newDocumentTestDeps(t)
	seedTask(t, deps, "task1")

	for _, key := range []string{"spec", "plan", "notes"} {
		body := `{"title":"` + key + `","content":"content"}`
		req := httptest.NewRequest(http.MethodPut,
			"/api/v1/office/tasks/task1/documents/"+key,
			strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		deps.router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("create %s: %d: %s", key, w.Code, w.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/tasks/task1/documents", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp dashboard.DocumentListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Documents) != 3 {
		t.Errorf("expected 3 documents, got %d", len(resp.Documents))
	}
}
