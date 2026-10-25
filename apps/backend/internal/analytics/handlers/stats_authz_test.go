package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/auth/authn"
	commonlogger "github.com/kandev/kandev/internal/common/logger"
)

// Per-user workspace scoping for the analytics stats routes.
//
// The handlers hold the analytics repository directly, so they never pass
// through the task service's authorize* helpers. The authorizer is injected at
// the wiring boundary and consulted in parseRequest, which every stats handler
// funnels through. These tests assert on the fake repository's call log, so an
// implementation that queries first and filters the response afterwards cannot
// pass them.

const (
	wsA          = "ws-a"
	wsB          = "ws-b"
	wsLegacy     = "ws-legacy"
	wsMissing    = "ws-missing"
	notFoundBody = `{"error":"workspace not found"}`
)

// errFakeWorkspaceNotFound stands in for repoerrors.ErrWorkspaceNotFound, which
// the real task service returns for both a foreign and a nonexistent workspace.
var errFakeWorkspaceNotFound = errors.New("workspace not found")

// recordingRepo records which analytics query each request reached, so a test
// can assert that a denied request never touched the database.
type recordingRepo struct {
	calls []string
}

func (r *recordingRepo) GetTaskStats(_ context.Context, _ string, _ *time.Time, _ int) ([]*models.TaskStats, error) {
	r.calls = append(r.calls, "GetTaskStats")
	return []*models.TaskStats{{TaskID: "task-1", TaskTitle: "secret title", CreatedAt: time.Unix(0, 0)}}, nil
}

func (r *recordingRepo) GetGlobalStats(_ context.Context, _ string, _ *time.Time) (*models.GlobalStats, error) {
	r.calls = append(r.calls, "GetGlobalStats")
	return &models.GlobalStats{TotalTasks: 3}, nil
}

func (r *recordingRepo) GetDailyActivity(_ context.Context, _ string, _ int) ([]*models.DailyActivity, error) {
	r.calls = append(r.calls, "GetDailyActivity")
	return []*models.DailyActivity{{Date: "2026-01-01", TaskCount: 1}}, nil
}

func (r *recordingRepo) GetCompletedTaskActivity(
	_ context.Context, _ string, _ int,
) ([]*models.CompletedTaskActivity, error) {
	r.calls = append(r.calls, "GetCompletedTaskActivity")
	return []*models.CompletedTaskActivity{{Date: "2026-01-01", CompletedTasks: 1}}, nil
}

func (r *recordingRepo) GetModelUsage(
	_ context.Context, _ string, _ int, _ *time.Time,
) ([]*models.ModelUsage, error) {
	r.calls = append(r.calls, "GetModelUsage")
	return []*models.ModelUsage{{Model: "opus", SessionCount: 1}}, nil
}

func (r *recordingRepo) GetRepositoryStats(
	_ context.Context, _ string, _ *time.Time,
) ([]*models.RepositoryStats, error) {
	r.calls = append(r.calls, "GetRepositoryStats")
	return []*models.RepositoryStats{{RepositoryID: "repo-1", RepositoryName: "secret/repo"}}, nil
}

func (r *recordingRepo) GetGitStats(_ context.Context, _ string, _ *time.Time) (*models.GitStats, error) {
	r.calls = append(r.calls, "GetGitStats")
	return &models.GitStats{TotalCommits: 7}, nil
}

func (r *recordingRepo) ListSessionCodeStats(
	_ context.Context, _ models.SessionCodeStatsFilter,
) ([]*models.SessionCodeStats, error) {
	r.calls = append(r.calls, "ListSessionCodeStats")
	return nil, nil
}

// ownerAuthorizer mirrors task/service.authorizeWorkspaceID: unscoped for
// internal (no identity) and synthetic (auth disabled) callers, and otherwise
// visible only to the owner, with unowned pre-auth rows visible to everyone.
// Both a foreign and an unknown workspace yield the same not-found sentinel.
type ownerAuthorizer struct {
	owners map[string]string
}

func newOwnerAuthorizer() ownerAuthorizer {
	return ownerAuthorizer{owners: map[string]string{wsA: "user-a", wsB: "user-b", wsLegacy: ""}}
}

func (a ownerAuthorizer) AuthorizeWorkspaceAccess(ctx context.Context, workspaceID string) error {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.Synthetic {
		return nil
	}
	owner, exists := a.owners[workspaceID]
	if !exists {
		return errFakeWorkspaceNotFound
	}
	if owner == "" || owner == identity.UserID {
		return nil
	}
	return errFakeWorkspaceNotFound
}

func authzTestLogger(t *testing.T) *commonlogger.Logger {
	t.Helper()
	log, err := commonlogger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log
}

// newStatsRouter builds the real route set with the injected authorizer, and an
// identity middleware standing in for auth/httpmw.
func newStatsRouter(t *testing.T, identity *authn.Identity) (*gin.Engine, *recordingRepo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if identity != nil {
		router.Use(func(c *gin.Context) {
			authn.SetOnGin(c, *identity)
			c.Next()
		})
	}
	repo := &recordingRepo{}
	RegisterStatsRoutes(router, repo, newOwnerAuthorizer(), authzTestLogger(t))
	return router, repo
}

func memberIdentity(userID string) *authn.Identity {
	return &authn.Identity{UserID: userID, Role: authn.RoleMember}
}

func syntheticIdentity() *authn.Identity {
	return &authn.Identity{UserID: "default-user", Role: authn.RoleAdmin, Synthetic: true}
}

// registeredStatsPaths returns every mounted workspace stats route, so a route
// added later is covered by these tests without editing them.
func registeredStatsPaths(t *testing.T, router *gin.Engine) []string {
	t.Helper()
	var paths []string
	for _, route := range router.Routes() {
		if strings.HasPrefix(route.Path, "/api/v1/workspaces/:id/stats/") {
			paths = append(paths, route.Path)
		}
	}
	if len(paths) == 0 {
		t.Fatal("no /api/v1/workspaces/:id/stats/* routes registered")
	}
	return paths
}

func requestPath(routePath, workspaceID string) string {
	return strings.Replace(routePath, ":id", workspaceID, 1)
}

func doGet(t *testing.T, router *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	router.ServeHTTP(rec, req)
	return rec
}

// TestStatsRoutesDenyForeignWorkspace covers every registered stats route: a
// scoped caller gets 404 for someone else's workspace and the analytics
// repository is never queried. A route added without going through the
// authorizing helper fails here.
func TestStatsRoutesDenyForeignWorkspace(t *testing.T) {
	paths := registeredStatsPaths(t, mustRouter(t))
	if len(paths) < 7 {
		t.Fatalf("expected at least 7 stats routes, found %d: %v", len(paths), paths)
	}
	for _, routePath := range paths {
		t.Run(routePath, func(t *testing.T) {
			router, repo := newStatsRouter(t, memberIdentity("user-a"))
			rec := doGet(t, router, requestPath(routePath, wsB))
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404 (body %s)", rec.Code, rec.Body.String())
			}
			if body := strings.TrimSpace(rec.Body.String()); body != notFoundBody {
				t.Fatalf("body = %s, want %s", body, notFoundBody)
			}
			if len(repo.calls) != 0 {
				t.Fatalf("denied request reached the analytics repository: %v", repo.calls)
			}
		})
	}
}

// TestStatsForeignAndMissingWorkspaceAreIndistinguishable pins the no-existence-leak
// convention: "not yours" must look exactly like "does not exist".
func TestStatsForeignAndMissingWorkspaceAreIndistinguishable(t *testing.T) {
	for _, routePath := range registeredStatsPaths(t, mustRouter(t)) {
		t.Run(routePath, func(t *testing.T) {
			router, _ := newStatsRouter(t, memberIdentity("user-a"))
			foreign := doGet(t, router, requestPath(routePath, wsB))
			missing := doGet(t, router, requestPath(routePath, wsMissing))
			if foreign.Code != missing.Code {
				t.Fatalf("status foreign=%d missing=%d", foreign.Code, missing.Code)
			}
			if foreign.Body.String() != missing.Body.String() {
				t.Fatalf("body foreign=%s missing=%s", foreign.Body.String(), missing.Body.String())
			}
		})
	}
}

// TestStatsOwnerStillReadsOwnWorkspace guards against an over-broad denial.
func TestStatsOwnerStillReadsOwnWorkspace(t *testing.T) {
	for _, routePath := range registeredStatsPaths(t, mustRouter(t)) {
		t.Run(routePath, func(t *testing.T) {
			router, repo := newStatsRouter(t, memberIdentity("user-a"))
			rec := doGet(t, router, requestPath(routePath, wsA))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
			}
			if len(repo.calls) != 1 {
				t.Fatalf("repository calls = %v, want exactly one", repo.calls)
			}
		})
	}
}

// TestStatsUnscopedCallersUnchanged pins single-user behavior: with auth
// disabled the middleware injects a synthetic identity (and internal callers
// carry none), both of which must reach every workspace exactly as before.
func TestStatsUnscopedCallersUnchanged(t *testing.T) {
	callers := map[string]*authn.Identity{
		"auth disabled (synthetic)": syntheticIdentity(),
		"internal (no identity)":    nil,
	}
	for name, identity := range callers {
		for _, routePath := range registeredStatsPaths(t, mustRouter(t)) {
			t.Run(name+" "+routePath, func(t *testing.T) {
				router, repo := newStatsRouter(t, identity)
				// wsB is owned by another user; wsMissing has no row at all.
				for _, workspaceID := range []string{wsA, wsB, wsLegacy, wsMissing} {
					rec := doGet(t, router, requestPath(routePath, workspaceID))
					if rec.Code != http.StatusOK {
						t.Fatalf("%s: status = %d, want 200 (body %s)", workspaceID, rec.Code, rec.Body.String())
					}
				}
				if len(repo.calls) != 4 {
					t.Fatalf("repository calls = %v, want one per request", repo.calls)
				}
			})
		}
	}
}

// TestStatsTaskTitlesNotLeaked is the concrete disclosure this fix closes:
// stats/tasks carries task titles.
func TestStatsTaskTitlesNotLeaked(t *testing.T) {
	router, _ := newStatsRouter(t, memberIdentity("user-a"))
	rec := doGet(t, router, "/api/v1/workspaces/"+wsB+"/stats/tasks")
	if strings.Contains(rec.Body.String(), "secret title") {
		t.Fatalf("foreign task title disclosed: %s", rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
}

// mustRouter builds a throwaway router only to enumerate the mounted routes.
func mustRouter(t *testing.T) *gin.Engine {
	t.Helper()
	router, _ := newStatsRouter(t, nil)
	return router
}

// errAuthorizerLookup stands in for a workspace lookup that fails for a reason
// other than ownership (a database error, say).
var errAuthorizerLookup = errors.New("workspace lookup failed")

// failingAuthorizer denies every request with a non-ownership error.
type failingAuthorizer struct{}

func (failingAuthorizer) AuthorizeWorkspaceAccess(context.Context, string) error {
	return errAuthorizerLookup
}

// newDenyingRouter mounts the stats routes with an authorizer that never
// permits: either absent entirely, or failing its lookup.
func newDenyingRouter(t *testing.T, authorizer WorkspaceAuthorizer, log *commonlogger.Logger) (*gin.Engine, *recordingRepo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, *memberIdentity("user-a"))
		c.Next()
	})
	repo := &recordingRepo{}
	RegisterStatsRoutes(router, repo, authorizer, log)
	return router, repo
}

// assertDeniedEverywhere pins the shared denial contract across every mounted
// stats route: 404, the one not-found body, and no repository query.
func assertDeniedEverywhere(t *testing.T, router *gin.Engine, repo *recordingRepo) {
	t.Helper()
	for _, routePath := range registeredStatsPaths(t, router) {
		rec := doGet(t, router, requestPath(routePath, wsA))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d, want 404 (body %s)", routePath, rec.Code, rec.Body.String())
		}
		if body := strings.TrimSpace(rec.Body.String()); body != notFoundBody {
			t.Fatalf("%s: body = %s, want %s", routePath, body, notFoundBody)
		}
	}
	if len(repo.calls) != 0 {
		t.Fatalf("denied requests reached the analytics repository: %v", repo.calls)
	}
}

// TestStatsNilAuthorizerFailsClosed covers the unwired-authorizer branch:
// scoping that was never injected must deny, not permit. Reviewer-requested
// coverage of behavior already present on this branch.
func TestStatsNilAuthorizerFailsClosed(t *testing.T) {
	router, repo := newDenyingRouter(t, nil, authzTestLogger(t))
	assertDeniedEverywhere(t, router, repo)
}

// TestStatsAuthorizerLookupErrorDenies covers a non-ownership authorizer
// failure: it collapses into the same 404 as a foreign workspace rather than
// surfacing as a 500 that distinguishes an existing workspace from an absent
// one. Reviewer-requested coverage of behavior already present on this branch.
func TestStatsAuthorizerLookupErrorDenies(t *testing.T) {
	router, repo := newDenyingRouter(t, failingAuthorizer{}, authzTestLogger(t))
	assertDeniedEverywhere(t, router, repo)
}

// TestStatsDenialIsAuditable pins the denial log above Debug: a caller probing
// for workspaces they do not own must leave a trace in the default log stream,
// not only under a debug level nobody runs in production.
func TestStatsDenialIsAuditable(t *testing.T) {
	core, recorded := observer.New(zapcore.DebugLevel)
	log, err := commonlogger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, *memberIdentity("user-a"))
		c.Next()
	})
	RegisterStatsRoutes(router, &recordingRepo{}, newOwnerAuthorizer(), log)

	if rec := doGet(t, router, "/api/v1/workspaces/"+wsB+"/stats/tasks"); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	entries := recorded.FilterLevelExact(zapcore.WarnLevel).All()
	if len(entries) != 1 {
		t.Fatalf("want exactly one Warn-level denial log, got %d of %d entries", len(entries), recorded.Len())
	}
	fields := entries[0].ContextMap()
	if got := fields["workspace_id"]; got != wsB {
		t.Fatalf("denial log workspace_id = %v, want %s", got, wsB)
	}
	// Without the caller, the line says a workspace was probed but not by whom,
	// which is the half that makes enumeration detectable.
	if got := fields["user_id"]; got != "user-a" {
		t.Fatalf("denial log user_id = %v, want user-a", got)
	}
}
