package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

// foreignSessionRepo owns everything as "user-b", so a request made as
// "user-a" exercises the per-user scoping denial.
type foreignSessionRepo struct {
	mockRepository
}

func (r *foreignSessionRepo) GetTaskSession(context.Context, string) (*models.TaskSession, error) {
	return &models.TaskSession{ID: "sess-b", TaskID: "task-b"}, nil
}

func (r *foreignSessionRepo) GetTask(context.Context, string) (*models.Task, error) {
	return &models.Task{ID: "task-b", WorkspaceID: "ws-b"}, nil
}

func (r *foreignSessionRepo) GetWorkspace(context.Context, string) (*models.Workspace, error) {
	return &models.Workspace{ID: "ws-b", OwnerID: "user-b"}, nil
}

func newForeignSessionHandlers(t *testing.T) *TaskHandlers {
	t.Helper()
	log := newTestLogger(t)
	repo := &foreignSessionRepo{}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	return &TaskHandlers{service: svc, logger: log}
}

// requestAs builds a gin context for sessionID carrying userID's identity.
func requestAs(t *testing.T, userID, sessionID string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+sessionID+"/approve", nil)
	c.Request = c.Request.WithContext(
		authn.WithIdentity(c.Request.Context(), authn.Identity{UserID: userID, Role: authn.RoleMember}),
	)
	c.Params = gin.Params{{Key: "id", Value: sessionID}}
	return c, rec
}

// TestHTTPApproveSessionDeniesForeignSessionWith404 pins the status mapping for
// the approve route. Approving advances a workflow step, so a foreign session
// must be refused — and it must answer 404 like every other session route
// rather than a 500, which would tell the caller the session exists.
func TestHTTPApproveSessionDeniesForeignSessionWith404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newForeignSessionHandlers(t)
	c, rec := requestAs(t, "user-a", "sess-b")

	h.httpApproveSession(c)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}

	// Prove the 404 is the scoping denial and not some unrelated error path:
	// the owner reaches past the check and fails later, on the workflow-step
	// fixtures this test deliberately does not build.
	ownerCtx, ownerRec := requestAs(t, "user-b", "sess-b")
	h.httpApproveSession(ownerCtx)
	if ownerRec.Code == http.StatusNotFound {
		t.Fatal("owner got 404 too — the 404 above is not proving authorization")
	}
}

// TestProcessRoutesDenyForeignSession covers the session-keyed routes in
// process_handlers.go. They resolve their execution with a bare in-memory
// lookup, which skips the lifecycle manager's internal per-user check, so each
// must deny explicitly. lifecycleMgr is nil on purpose: the check has to
// short-circuit before anything touches it, and a nil-pointer panic here would
// mean the guard is in the wrong place.
func TestProcessRoutesDenyForeignSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)
	repo := &foreignSessionRepo{}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	h := &ProcessHandlers{service: svc, logger: log}

	routes := map[string]func(*gin.Context){
		"set-mode":          h.httpSetSessionMode,
		"set-model":         h.httpSetSessionModel,
		"set-config-option": h.httpSetSessionConfigOption,
		"authenticate":      h.httpAuthenticate,
		"stop-process":      h.httpStopProcessByID,
	}
	for name, handler := range routes {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{}`))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Request = c.Request.WithContext(authn.WithIdentity(
				c.Request.Context(),
				authn.Identity{UserID: "user-a", Role: authn.RoleMember},
			))
			c.Params = gin.Params{{Key: "id", Value: "sess-b"}, {Key: "processId", Value: "proc-1"}}

			handler(c)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
			}
		})
	}
}

// TestHTTPGetTaskSessionDeniesForeignSessionWith404 covers the plain read,
// whose DTO carries session metadata, the host worktree path, and branch names.
func TestHTTPGetTaskSessionDeniesForeignSessionWith404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newForeignSessionHandlers(t)
	c, rec := requestAs(t, "user-a", "sess-b")

	h.httpGetTaskSession(c)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); len(body) > 0 && rec.Code == http.StatusOK {
		t.Fatalf("foreign session body leaked: %s", body)
	}
}

// TestHTTPGetShellOutputDeniesForeignMessageWith404 pins the status mapping for
// the shell-output route. GetMessage became per-user scoped, so a denial now
// arrives as ErrTaskNotFound rather than sql.ErrNoRows; matching only the latter
// answered 500 for a foreign message and 404 for a missing one, which is an
// existence signal.
func TestHTTPGetShellOutputDeniesForeignMessageWith404(t *testing.T) {
	gin.SetMode(gin.TestMode)

	foreign := shellOutputRequestAs(t, "user-a")
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body: %s)", foreign.Code, foreign.Body.String())
	}

	// In-test witness that the 404 above comes from authorization and not from
	// the route's post-service `!ok` gate. This test was previously vacuous for
	// exactly that reason: with an empty metadata fixture it passed even with
	// the guard removed. The owner must get past the check, so if this ever
	// starts returning 404 too, the assertion above has stopped proving
	// anything — which is cheaper to notice here than in a mutation run.
	owner := shellOutputRequestAs(t, "user-b")
	if owner.Code == http.StatusNotFound {
		t.Fatal("owner got 404 too — the 404 above is not proving authorization")
	}
}

// shellOutputRequestAs issues the shell-output request for the foreign fixture's
// message as userID, and returns the recorded response.
func shellOutputRequestAs(t *testing.T, userID string) *httptest.ResponseRecorder {
	t.Helper()
	log := newTestLogger(t)
	repo := &foreignMessageRepo{}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	h := &MessageHandlers{service: svc, logger: log}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet,
		"/api/v1/task-sessions/sess-b/messages/msg-b/shell-output", nil)
	c.Request = c.Request.WithContext(authn.WithIdentity(
		c.Request.Context(), authn.Identity{UserID: userID, Role: authn.RoleMember}))
	c.Params = gin.Params{{Key: "id", Value: "sess-b"}, {Key: "message_id", Value: "msg-b"}}

	h.httpGetShellOutput(c)
	return rec
}

// foreignMessageRepo owns the message's session as "user-b".
//
// The metadata must be a VALID shell-output payload. httpGetShellOutput has a
// second 404 gate after the service call —
// `if !ok || message.TaskSessionID != c.Param("id")` — and
// ExtractShellExecOutput returns ok=false for nil metadata. With an empty
// fixture the route answers 404 through that gate whether or not scoping
// denied it, so the test would pass even with the authorization removed.
// Populating it means a scope regression reaches the 200 path and fails.
type foreignMessageRepo struct {
	foreignSessionRepo
}

func (r *foreignMessageRepo) GetMessage(context.Context, string) (*models.Message, error) {
	return &models.Message{
		ID:            "msg-b",
		TaskSessionID: "sess-b",
		Metadata: map[string]any{
			"status": "completed",
			"normalized": map[string]any{
				"shell_exec": map[string]any{
					"output": map[string]any{"stdout": "victim shell output"},
				},
			},
		},
	}, nil
}
