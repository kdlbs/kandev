package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	terminalrepo "github.com/kandev/kandev/internal/terminal/repository"
	terminalservice "github.com/kandev/kandev/internal/terminal/service"
)

// The two SSR terminal-listing routes read terminal state straight from the
// terminal service and the interactive runner, bypassing the lifecycle
// manager's checked accessors. With auth enabled that let any authenticated
// user list another user's terminals: display names and the initial command
// lines of their work. The WS siblings are covered by the dispatch backstop in
// internal/gateway/websocket/dispatch_scope.go; these HTTP routes had no
// equivalent, so the guard is mounted on the routes themselves.

const (
	ownTaskID     = "task-a"
	ownEnvID      = "env-a"
	foreignTaskID = "task-b"
	foreignEnvID  = "env-b"
	// unknown* stand in for IDs that exist for nobody. A foreign ID and an
	// unknown one must be indistinguishable (no existence leak).
	unknownTaskID = "task-zzz"
	unknownEnvID  = "env-zzz"
	// unrelatedEnvID is an environment the caller DOES own, but which is not
	// bound to ownTaskID. Authorizing the two IDs independently admits this
	// pair; only a pairing check refuses it.
	unrelatedEnvID = "env-a-other"

	// leakMarker is the initial command stored on the foreign task's
	// terminal. It must never appear in a response to another user.
	leakMarker = "psql -h prod-db -U admin"
)

// scopedShellHandlers builds handlers whose lifecycle manager admits only
// ownTaskID/ownEnvID, backed by a real terminal service holding one terminal
// for the caller's task and one for a foreign task.
func scopedShellHandlers(t *testing.T) *ShellHandlers {
	t.Helper()
	mgr := &lifecycle.Manager{}
	mgr.SetTaskAccessChecker(func(_ context.Context, taskID string) error {
		if taskID == ownTaskID {
			return nil
		}
		return errors.New("task not found")
	})
	// The caller owns two environments; only ownEnvID is bound to ownTaskID.
	mgr.SetEnvironmentAccessChecker(func(_ context.Context, envID string) error {
		if envID == ownEnvID || envID == unrelatedEnvID {
			return nil
		}
		return errors.New("task environment not found")
	})
	mgr.SetTaskEnvironmentAccessChecker(func(_ context.Context, taskID, envID string) error {
		if taskID == ownTaskID && envID == ownEnvID {
			return nil
		}
		return errors.New("task not found")
	})
	h := NewShellHandlers(mgr, nil, newTestLogger())
	h.SetTerminalService(seededTerminalService(t))
	return h
}

// seededTerminalService returns a real terminal service over an in-memory
// SQLite DB, holding one terminal per task. Using the real service (rather
// than a stub returning nothing) means a leak would actually show up in the
// response body.
func seededTerminalService(t *testing.T) *terminalservice.Service {
	t.Helper()
	// Unique DSN per test: `cache=shared` in-memory DBs are keyed by name
	// process-wide, so a fixed name would let parallel tests share rows.
	rawDB, err := sql.Open("sqlite3", "file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	rawDB.SetMaxOpenConns(1)
	db := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := terminalrepo.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("terminal repo: %v", err)
	}
	svc := terminalservice.New(repo, noopPTYBackend{}, nil)
	if _, err := svc.Create(context.Background(), ownTaskID, ownEnvID, "echo mine"); err != nil {
		t.Fatalf("seed own terminal: %v", err)
	}
	if _, err := svc.Create(context.Background(), foreignTaskID, foreignEnvID, leakMarker); err != nil {
		t.Fatalf("seed foreign terminal: %v", err)
	}
	return svc
}

// noopPTYBackend satisfies terminalservice.PTYBackend without a real agentctl.
type noopPTYBackend struct{}

func (noopPTYBackend) Register(string, string)                        {}
func (noopPTYBackend) Stop(context.Context, string, string) error     { return nil }
func (noopPTYBackend) StopScope(context.Context, string) (int, error) { return 0, nil }
func (noopPTYBackend) IsAlive(string, string) bool                    { return false }

func scopedShellRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterShellRoutesOn(router, scopedShellHandlers(t))
	return router
}

func getPath(t *testing.T, router *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}

// TestSSRTerminalRoutesDenyForeignScope is the core regression: both routes
// must refuse a scope the caller cannot see, with 404 (not 403) and without
// echoing any terminal state.
func TestSSRTerminalRoutesDenyForeignScope(t *testing.T) {
	cases := map[string]struct {
		path string
		want string
	}{
		"environment route": {
			path: "/api/v1/environments/" + foreignEnvID + "/terminals",
			want: `{"error":"task environment not found"}`,
		},
		"task route": {
			path: "/api/v1/tasks/" + foreignTaskID + "/terminals",
			want: `{"error":"task not found"}`,
		},
		"task route with environment query": {
			path: "/api/v1/tasks/" + foreignTaskID + "/terminals?task_environment_id=" + foreignEnvID,
			want: `{"error":"task not found"}`,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			router := scopedShellRouter(t)

			recorder := getPath(t, router, tc.path)

			if recorder.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404 (body %s)", recorder.Code, recorder.Body.String())
			}
			if got := strings.TrimSpace(recorder.Body.String()); got != tc.want {
				t.Errorf("body = %s, want %s", got, tc.want)
			}
			if strings.Contains(recorder.Body.String(), leakMarker) {
				t.Errorf("response leaked the foreign initial command: %s", recorder.Body.String())
			}
		})
	}
}

// TestSSRTaskTerminalsGuardsEnvironmentQuery covers the second way in:
// task_environment_id is passed to appendUnmanagedShells, so naming your own
// task while pointing the query param at someone else's environment must fail
// too.
func TestSSRTaskTerminalsGuardsEnvironmentQuery(t *testing.T) {
	router := scopedShellRouter(t)

	recorder := getPath(t, router, "/api/v1/tasks/"+ownTaskID+"/terminals?task_environment_id="+foreignEnvID)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body %s)", recorder.Code, recorder.Body.String())
	}
	if got := strings.TrimSpace(recorder.Body.String()); got != `{"error":"task environment not found"}` {
		t.Errorf("body = %s", got)
	}
}

// TestSSRTerminalRoutesHideExistence keeps the no-existence-leak convention
// documented at the top of internal/task/service/service_access.go: a foreign
// ID and a nonexistent one must be byte-identical.
func TestSSRTerminalRoutesHideExistence(t *testing.T) {
	cases := map[string][2]string{
		"environment route": {
			"/api/v1/environments/" + foreignEnvID + "/terminals",
			"/api/v1/environments/" + unknownEnvID + "/terminals",
		},
		"task route": {
			"/api/v1/tasks/" + foreignTaskID + "/terminals",
			"/api/v1/tasks/" + unknownTaskID + "/terminals",
		},
	}
	for name, paths := range cases {
		t.Run(name, func(t *testing.T) {
			router := scopedShellRouter(t)

			foreign := getPath(t, router, paths[0])
			unknown := getPath(t, router, paths[1])

			if foreign.Code != unknown.Code {
				t.Errorf("status foreign = %d, unknown = %d", foreign.Code, unknown.Code)
			}
			if foreign.Body.String() != unknown.Body.String() {
				t.Errorf("body foreign = %s, unknown = %s", foreign.Body.String(), unknown.Body.String())
			}
		})
	}
}

// TestSSRTerminalGuardsAbortBeforeReadingState proves the guard is a route
// filter, not a post-hoc response filter: the listing handler is the only code
// that touches the terminal service and the interactive runner, so a sentinel
// that never runs in its place means neither was queried. A "list then drop
// the rows" implementation cannot pass this.
func TestSSRTerminalGuardsAbortBeforeReadingState(t *testing.T) {
	h := scopedShellHandlers(t)
	cases := map[string]struct {
		guard gin.HandlerFunc
		route string
		path  string
	}{
		"environment route": {
			guard: h.authorizeEnvironmentRoute,
			route: "/api/v1/environments/:id/terminals",
			path:  "/api/v1/environments/" + foreignEnvID + "/terminals",
		},
		"task route": {
			guard: h.authorizeTaskRoute,
			route: "/api/v1/tasks/:id/terminals",
			path:  "/api/v1/tasks/" + foreignTaskID + "/terminals",
		},
		"task route via environment query": {
			guard: h.authorizeTaskRoute,
			route: "/api/v1/tasks/:id/terminals",
			path:  "/api/v1/tasks/" + ownTaskID + "/terminals?task_environment_id=" + foreignEnvID,
		},
		"task route via unrelated owned environment": {
			guard: h.authorizeTaskRoute,
			route: "/api/v1/tasks/:id/terminals",
			path:  "/api/v1/tasks/" + ownTaskID + "/terminals?task_environment_id=" + unrelatedEnvID,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			reached := false
			router.GET(tc.route, tc.guard, func(c *gin.Context) {
				reached = true
				c.JSON(http.StatusOK, gin.H{"terminals": []any{}})
			})

			if code := getPath(t, router, tc.path).Code; code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", code)
			}
			if reached {
				t.Error("listing handler ran for a denied scope; terminal state was read before the guard")
			}
		})
	}
}

// TestSSRTerminalRoutesServeOwner is the other half of the guard: the person
// who owns the task still gets their terminals.
//
// The manager here holds no executions and no interactive runner, which is the
// state after the agent behind the environment has been torn down. That is the
// case worth pinning: these routes back a server-rendered panel, so an
// over-eager guard does not surface as an error, it silently empties the
// owner's terminal list. Authorization must key off task and environment
// ownership only, never off a live execution.
func TestSSRTerminalRoutesServeOwner(t *testing.T) {
	router := scopedShellRouter(t)

	recorder := getPath(t, router, "/api/v1/tasks/"+ownTaskID+"/terminals?task_environment_id="+ownEnvID)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "echo mine") {
		t.Errorf("owner did not get their own terminal: %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), leakMarker) {
		t.Errorf("owner response included another task's terminal: %s", recorder.Body.String())
	}

	if code := getPath(t, router, "/api/v1/environments/"+ownEnvID+"/terminals").Code; code != http.StatusOK {
		t.Errorf("environment route status = %d, want 200", code)
	}
}

// TestSSRTerminalGuardsAreNoOpWithAuthDisabled pins single-user behavior.
// With auth disabled no checker denies anything (callerScope reports unscoped
// for the synthetic identity, and nothing is wired at all in the legacy boot
// path), so both routes must answer exactly as they did before this guard.
func TestSSRTerminalGuardsAreNoOpWithAuthDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := NewShellHandlers(newTestManager(), nil, newTestLogger())
	h.SetTerminalService(seededTerminalService(t))
	RegisterShellRoutesOn(router, h)

	// Any ID at all — including one belonging to "another user" in the
	// scoped tests — is served, because there is no scoping to apply.
	for _, path := range []string{
		"/api/v1/environments/" + foreignEnvID + "/terminals",
		"/api/v1/tasks/" + foreignTaskID + "/terminals?task_environment_id=" + foreignEnvID,
		"/api/v1/tasks/" + unknownTaskID + "/terminals",
	} {
		recorder := getPath(t, router, path)
		if recorder.Code != http.StatusOK {
			t.Errorf("%s status = %d, want 200 (body %s)", path, recorder.Code, recorder.Body.String())
		}
	}

	// The foreign task's rows are still returned verbatim in single-user mode.
	if body := getPath(t, router, "/api/v1/tasks/"+foreignTaskID+"/terminals").Body.String(); !strings.Contains(body, leakMarker) {
		t.Errorf("single-user mode lost a terminal: %s", body)
	}

	// And the manager's own predicate is permissive when unwired.
	bare := &lifecycle.Manager{}
	if err := bare.CheckTaskAccess(context.Background(), ownTaskID); err != nil {
		t.Errorf("unwired task checker denied the call (%v); pre-auth behavior broken", err)
	}
}

// TestSSRTaskTerminalsRefusesUnpairedEnvironment is the pairing regression.
// Authorizing the task ID and the environment ID independently both succeed
// for a caller who owns each of them separately; the handler would then merge
// the path task's ordinary terminals with the unrelated environment's
// unmanaged shells, labels and initial commands included. The guard must
// refuse the pair, and must do so before appendUnmanagedShells can run.
func TestSSRTaskTerminalsRefusesUnpairedEnvironment(t *testing.T) {
	router := scopedShellRouter(t)

	recorder := getPath(t, router, "/api/v1/tasks/"+ownTaskID+"/terminals?task_environment_id="+unrelatedEnvID)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body %s)", recorder.Code, recorder.Body.String())
	}
	if got := strings.TrimSpace(recorder.Body.String()); got != `{"error":"task environment not found"}` {
		t.Errorf("body = %s", got)
	}

	// The environment is legitimately the caller's own, so the env-keyed route
	// still serves it. Only the *pair* is refused, not the environment.
	if code := getPath(t, router, "/api/v1/environments/"+unrelatedEnvID+"/terminals").Code; code != http.StatusOK {
		t.Errorf("env-keyed route status = %d, want 200; the pairing check must not gate the environment itself", code)
	}

	// A refused pair is indistinguishable from a foreign one.
	unrelated := getPath(t, router, "/api/v1/tasks/"+ownTaskID+"/terminals?task_environment_id="+unrelatedEnvID)
	foreign := getPath(t, router, "/api/v1/tasks/"+ownTaskID+"/terminals?task_environment_id="+foreignEnvID)
	if unrelated.Code != foreign.Code || unrelated.Body.String() != foreign.Body.String() {
		t.Errorf("unrelated = %d %s, foreign = %d %s", unrelated.Code, unrelated.Body.String(),
			foreign.Code, foreign.Body.String())
	}
}
