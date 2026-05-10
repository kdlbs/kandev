package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/prompts/controller"
	"github.com/kandev/kandev/internal/prompts/service"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
)

func newTestRouter(t *testing.T) (*gin.Engine, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, repoCleanup, err := promptstore.Provide(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("provide repo: %v", err)
	}
	svc := service.NewService(repo)
	ctrl := controller.NewController(svc)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})

	router := gin.New()
	RegisterRoutes(router, ctrl, log)

	cleanup := func() {
		if err := sqlxDB.Close(); err != nil {
			t.Errorf("close sqlite: %v", err)
		}
		if err := repoCleanup(); err != nil {
			t.Errorf("close repo: %v", err)
		}
	}
	return router, cleanup
}

func postJSON(t *testing.T, router *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// Duplicate prompt name on POST /api/v1/prompts must surface as 409 with a
// human-readable error message — not a generic 500 — so the frontend can show
// a useful warning rather than crashing the user out of the create form.
func TestHTTPCreatePrompt_DuplicateName_Returns409(t *testing.T) {
	router, cleanup := newTestRouter(t)
	defer cleanup()

	first := postJSON(t, router, "/api/v1/prompts", map[string]string{
		"name": "dupe", "content": "first",
	})
	if first.Code != http.StatusOK {
		t.Fatalf("seed prompt: status %d body %s", first.Code, first.Body.String())
	}

	second := postJSON(t, router, "/api/v1/prompts", map[string]string{
		"name": "dupe", "content": "second",
	})
	if second.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body %s", second.Code, second.Body.String())
	}

	var resp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error == "" {
		t.Fatal("expected non-empty error message")
	}
	if !bytes.Contains([]byte(resp.Error), []byte("already exists")) {
		t.Fatalf("expected 'already exists' in message, got %q", resp.Error)
	}
}

// Built-in prompts (seeded with names like "code-review") share the unique
// name space with user-created prompts, so creating a custom prompt that
// collides with a built-in must also return 409, not 500.
func TestHTTPCreatePrompt_BuiltinName_Returns409(t *testing.T) {
	router, cleanup := newTestRouter(t)
	defer cleanup()

	rec := postJSON(t, router, "/api/v1/prompts", map[string]string{
		"name": "code-review", "content": "shadow",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body %s", rec.Code, rec.Body.String())
	}
}
