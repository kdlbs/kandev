package share

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/task/models"
)

func init() { gin.SetMode(gin.TestMode) }

func newHandlersForTest(t *testing.T, reader TaskReader, backend Backend, authed bool) (*HTTPHandlers, *Service) {
	t.Helper()
	repo := newTestRepo(t)
	svc := New(repo, reader, backend, nil, "v-test")
	if mock, ok := backend.(*mockBackend); ok && !authed {
		mock.accessErr = errors.New("connect a GitHub account to share tasks publicly")
	}
	return NewHTTPHandlers(svc, nil), svc
}

func newGinRouter(h *HTTPHandlers) *gin.Engine {
	r := gin.New()
	h.RegisterRoutes(r)
	return r
}

func TestHTTP_Create_HappyPath(t *testing.T) {
	t.Parallel()
	reader := completedSession()
	// Real GistBackend returns the gist.githack.com rendered URL — match
	// that shape so the test mirrors production.
	backend := &mockBackend{nextID: "gist-x", nextURL: "https://gist.githack.com/jane/gist-x/raw/share.html"}
	handlers, _ := newHandlersForTest(t, reader, backend, true)
	router := newGinRouter(handlers)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/t-1/sessions/s-1/shares", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body shareResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.URL != "https://gist.githack.com/jane/gist-x/raw/share.html" {
		t.Fatalf("unexpected url: %s", body.URL)
	}
	if len(backend.accessWorkspaces) != 1 || backend.accessWorkspaces[0] != "workspace-1" {
		t.Fatalf("access workspaces = %v, want workspace-1", backend.accessWorkspaces)
	}
}

func TestHTTP_Create_RejectsCreatedSessionWith409(t *testing.T) {
	t.Parallel()
	reader := completedSession()
	reader.session.State = models.TaskSessionStateCreated
	backend := &mockBackend{}
	handlers, _ := newHandlersForTest(t, reader, backend, true)
	router := newGinRouter(handlers)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/t-1/sessions/s-1/shares", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d: %s", rec.Code, rec.Body.String())
	}
	var body errorBody
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Code != "session_not_shareable" {
		t.Fatalf("expected code session_not_shareable, got %q", body.Code)
	}
}

func TestHTTP_Create_BlocksWhenGitHubUnauthenticated(t *testing.T) {
	t.Parallel()
	reader := completedSession()
	backend := &mockBackend{}
	handlers, _ := newHandlersForTest(t, reader, backend, false)
	router := newGinRouter(handlers)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/t-1/sessions/s-1/shares", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("want 412, got %d", rec.Code)
	}
	var body errorBody
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Code != "github_credential_missing" {
		t.Fatalf("expected code github_credential_missing, got %q", body.Code)
	}
}

func TestHTTP_Create_DryRunReturnsSnapshotWithoutUpload(t *testing.T) {
	t.Parallel()
	reader := completedSession()
	backend := &mockBackend{}
	// Auth check is skipped for dry-run; flip authed to false to prove that.
	handlers, _ := newHandlersForTest(t, reader, backend, false)
	router := newGinRouter(handlers)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/t-1/sessions/s-1/shares?dry_run=true", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var snap Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if snap.Task.Title == "" {
		t.Fatalf("expected non-empty snapshot, got %+v", snap)
	}
	if backend.uploads != 0 {
		t.Fatalf("dry-run must not call backend Upload, got %d uploads", backend.uploads)
	}
}

func TestHTTP_List_ReturnsSharesNewestFirst(t *testing.T) {
	t.Parallel()
	reader := completedSession()
	backend := &mockBackend{nextID: "gist-a"}
	handlers, svc := newHandlersForTest(t, reader, backend, true)
	router := newGinRouter(handlers)

	if _, err := svc.CreateShare(context.Background(), "s-1"); err != nil {
		t.Fatalf("create: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/t-1/sessions/s-1/shares", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var body listSharesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Shares) != 1 {
		t.Fatalf("expected 1 share, got %d", len(body.Shares))
	}
}

func TestHTTP_Revoke_Success(t *testing.T) {
	t.Parallel()
	reader := completedSession()
	backend := &mockBackend{nextID: "gist-z"}
	handlers, svc := newHandlersForTest(t, reader, backend, true)
	router := newGinRouter(handlers)

	share, err := svc.CreateShare(context.Background(), "s-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/shares/"+share.ID, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(backend.deletes) != 1 {
		t.Fatalf("expected 1 backend delete, got %d", len(backend.deletes))
	}
}

func TestDisplayURL_NormalizesAllFormats(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			"bare_gist_url_converted_to_githack",
			"https://gist.github.com/jane/abc123",
			"https://gist.githack.com/jane/abc123/raw/share.html",
		},
		{
			"githack_url_passthrough",
			"https://gist.githack.com/jane/abc123/raw/share.html",
			"https://gist.githack.com/jane/abc123/raw/share.html",
		},
		{
			"githack_without_filename_repinned",
			"https://gist.githack.com/jane/abc123",
			"https://gist.githack.com/jane/abc123/raw/share.html",
		},
		{
			// Legacy gistpreview rows (written before the githack switch)
			// lack the owner segment we need to build a githack URL, so
			// they pass through. Users on those rows still get a working
			// link for small tasks; revoking + re-sharing surfaces the
			// githack URL going forward.
			"legacy_gistpreview_with_share_html_passthrough",
			"https://gistpreview.github.io/?abc123/share.html",
			"https://gistpreview.github.io/?abc123/share.html",
		},
		{
			// Older gistpreview rows stored without /share.html still get
			// re-pinned so gistpreview doesn't silently fall back to
			// rendering README.md.
			"legacy_gistpreview_without_filename_repinned",
			"https://gistpreview.github.io/?abc123",
			"https://gistpreview.github.io/?abc123/share.html",
		},
		{
			"empty_string",
			"",
			"",
		},
		{
			"unknown_url_passthrough",
			"https://example.com/foo",
			"https://example.com/foo",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := displayURL(tc.input); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHTTP_List_NormalizesLegacyURLsOnDisplay(t *testing.T) {
	t.Parallel()
	reader := completedSession()
	backend := &mockBackend{}
	handlers, svc := newHandlersForTest(t, reader, backend, true)
	router := newGinRouter(handlers)

	// Seed two legacy rows from earlier URL formats — bare gist and a
	// githack URL with no filename pinned. Both should surface as the
	// canonical /raw/share.html githack form.
	for _, row := range []*Share{
		{ID: "old-1", TaskSessionID: "s-1", Backend: BackendGitHubGist, ExternalID: "abc123",
			ExternalURL: "https://gist.github.com/jane/abc123"},
		{ID: "old-2", TaskSessionID: "s-1", Backend: BackendGitHubGist, ExternalID: "def456",
			ExternalURL: "https://gist.githack.com/jane/def456"},
	} {
		if err := svc.repo.Create(context.Background(), row); err != nil {
			t.Fatalf("seed %s: %v", row.ID, err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/t-1/sessions/s-1/shares", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var body listSharesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Shares) != 2 {
		t.Fatalf("expected 2 shares, got %d", len(body.Shares))
	}
	for _, s := range body.Shares {
		if !strings.HasSuffix(s.URL, "/raw/share.html") ||
			!strings.HasPrefix(s.URL, "https://gist.githack.com/") {
			t.Fatalf("expected gist.githack.com /raw/share.html URL, got %q", s.URL)
		}
	}
}

func TestHTTP_Revoke_NotFound(t *testing.T) {
	t.Parallel()
	reader := completedSession()
	backend := &mockBackend{}
	handlers, _ := newHandlersForTest(t, reader, backend, true)
	router := newGinRouter(handlers)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/shares/does-not-exist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}
