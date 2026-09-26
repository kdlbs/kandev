package dashboard_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
)

// fakeGitGuard is a test double for dashboard.ActiveSourceChecker.
type fakeGitGuard struct {
	active bool
	err    error
}

func (f *fakeGitGuard) HasActiveSource(context.Context, string) (bool, error) {
	return f.active, f.err
}

var errGitGuardUnavailable = errors.New("guard: source unavailable")

// newGitGuardTestRouter mounts dashboard routes with the given guard and a
// nil git manager: since the guard check runs before the nil-gitMgr check,
// these tests exercise the guard without needing a real git manager.
func newGitGuardTestRouter(t *testing.T, guard dashboard.ActiveSourceChecker) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	log := logger.Default()
	activity := shared.NewActivityLogger(repo, log)
	svc := dashboard.NewDashboardService(repo, log, activity, &stubAgentReader{}, &stubCostChecker{})

	router := gin.New()
	group := router.Group("/api/v1/office")
	dashboard.RegisterRoutes(group, svc, repo, nil, nil, guard, log)
	return router
}

var gitGuardedRoutes = []struct {
	name   string
	method string
	path   string
}{
	{"gitClone", http.MethodPost, "/api/v1/office/workspaces/ws-1/git/clone"},
	{"gitPull", http.MethodPost, "/api/v1/office/workspaces/ws-1/git/pull"},
}

func TestHandler_GitRoutes_RefuseWhenConfigSyncActive(t *testing.T) {
	for _, rt := range gitGuardedRoutes {
		t.Run(rt.name, func(t *testing.T) {
			router := newGitGuardTestRouter(t, &fakeGitGuard{active: true})
			req := httptest.NewRequest(rt.method, rt.path, strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusConflict {
				t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusConflict, rec.Body.String())
			}
		})
	}
}

func TestHandler_GitRoutes_RefuseWhenGuardErrors(t *testing.T) {
	for _, rt := range gitGuardedRoutes {
		t.Run(rt.name, func(t *testing.T) {
			router := newGitGuardTestRouter(t, &fakeGitGuard{err: errGitGuardUnavailable})
			req := httptest.NewRequest(rt.method, rt.path, strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusConflict {
				t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusConflict, rec.Body.String())
			}
		})
	}
}

func TestHandler_GitRoutes_ProceedWhenConfigSyncInactive(t *testing.T) {
	for _, rt := range gitGuardedRoutes {
		t.Run(rt.name, func(t *testing.T) {
			// Nil git manager means an inactive guard falls through to the
			// handler's own 503, not a 409 from the guard.
			router := newGitGuardTestRouter(t, &fakeGitGuard{active: false})
			req := httptest.NewRequest(rt.method, rt.path, strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code == http.StatusConflict {
				t.Errorf("expected the route to proceed past the guard, got 409: %s", rec.Body.String())
			}
		})
	}
}
