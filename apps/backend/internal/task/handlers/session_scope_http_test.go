package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
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
