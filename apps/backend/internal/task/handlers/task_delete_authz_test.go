package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// DELETE /api/v1/tasks/:id built its context with context.WithTimeout(
// context.Background(), …). context.Background() carries no identity, so the
// service's authorizeTaskID saw an identity-less internal caller and permitted
// deleting anyone's task. The sibling archive route uses c.Request.Context() and
// answers 404, which is the shape of the fix — but the request context is
// cancelled when the client disconnects, and this route detaches from that on
// purpose, so the fix is context.WithoutCancel: keep the values, drop the cancel.
//
// Symbols here are authz-prefixed so they cannot collide with the fixtures PR
// #2500 adds to this package while it is unmerged.

// authzDeleteRepo serves one task owned by user-b and records every delete the
// handler managed to issue.
type authzDeleteRepo struct {
	mockRepository

	mu sync.Mutex
	// deleted records each delete alongside the liveness of the context it
	// arrived with. A cancelled context means the caller handed us one that a
	// client disconnect had already killed.
	deleted   []string
	deleteErr []error
	archived  []string
}

func (r *authzDeleteRepo) GetTask(_ context.Context, id string) (*models.Task, error) {
	return &models.Task{ID: id, WorkspaceID: "ws-b", Title: "Victim"}, nil
}

func (r *authzDeleteRepo) GetWorkspace(_ context.Context, id string) (*models.Workspace, error) {
	return &models.Workspace{ID: id, Name: "B's", OwnerID: "user-b"}, nil
}

func (r *authzDeleteRepo) ArchiveTaskIfActive(_ context.Context, id, cascadeID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.archived = append(r.archived, id+"/"+cascadeID)
	return true, nil
}

func (r *authzDeleteRepo) archives() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.archived...)
}

func (r *authzDeleteRepo) DeleteTask(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deleted = append(r.deleted, id)
	r.deleteErr = append(r.deleteErr, ctx.Err())
	return nil
}

func (r *authzDeleteRepo) ListChildren(context.Context, string) ([]*models.Task, error) {
	return nil, nil
}

func (r *authzDeleteRepo) ListChildrenIncludingArchived(context.Context, string) ([]*models.Task, error) {
	return nil, nil
}

func (r *authzDeleteRepo) CountToolCallMessagesBySession(context.Context, []string) (map[string]int, error) {
	return nil, nil
}

func (r *authzDeleteRepo) deletes() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.deleted...)
}

// deliveredLive reports whether every recorded delete arrived on a context that
// was still alive. A real repository would abort on a cancelled one; the fake
// cannot, so the liveness is asserted directly.
func (r *authzDeleteRepo) deliveredLive() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, err := range r.deleteErr {
		if err != nil {
			return false
		}
	}
	return len(r.deleteErr) > 0
}

func newAuthzTaskHandlers(t *testing.T, repo *authzDeleteRepo) *TaskHandlers {
	t.Helper()
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	return &TaskHandlers{service: svc, repo: repo, logger: log}
}

// authzDeleteRequest builds a DELETE for taskID carrying userID's identity.
func authzDeleteRequest(t *testing.T, userID, taskID string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/"+taskID, nil)
	req = req.WithContext(authn.WithIdentity(
		req.Context(), authn.Identity{UserID: userID, Role: authn.RoleMember}))
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: taskID}}
	return c, rec
}

func TestHTTPDeleteTaskDeniesForeignTask(t *testing.T) {
	repo := &authzDeleteRepo{}
	h := newAuthzTaskHandlers(t, repo)

	c, rec := authzDeleteRequest(t, "user-a", "task-b")
	h.httpDeleteTask(c)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign delete: status = %d body = %s, want 404", rec.Code, rec.Body.String())
	}
	if got := repo.deletes(); len(got) != 0 {
		t.Fatalf("a denied delete reached the repository: %v", got)
	}

	// The owner must still be able to delete, otherwise a 404 for everyone
	// would pass this test just as well.
	ownerCtx, ownerRec := authzDeleteRequest(t, "user-b", "task-b")
	h.httpDeleteTask(ownerCtx)

	if ownerRec.Code != http.StatusOK {
		t.Fatalf("owner delete: status = %d body = %s, want 200", ownerRec.Code, ownerRec.Body.String())
	}
	if got := repo.deletes(); len(got) != 1 || got[0] != "task-b" {
		t.Fatalf("owner delete did not reach the repository: %v", got)
	}
}

func TestHTTPResetTaskEnvironmentDeniesForeignTaskAsNotFound(t *testing.T) {
	repo := &authzDeleteRepo{}
	h := newAuthzTaskHandlers(t, repo)

	c, rec := authzDeleteRequest(t, "user-a", "task-b")
	c.Request.Method = http.MethodPost
	h.httpResetTaskEnvironment(c)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign environment reset: status = %d body = %s, want 404", rec.Code, rec.Body.String())
	}
}

// TestHTTPDeleteTaskDeniesForeignTaskThroughCascade covers the shape backendapp
// actually ships: with a HandoffService wired the handler never calls
// Service.DeleteTask, so the guard that carries this route lives on the cascade.
//
// SetHandoffService is called exactly as production calls it — nothing here
// installs the access checker by hand — so removing that wiring fails this test.
func TestHTTPDeleteTaskDeniesForeignTaskThroughCascade(t *testing.T) {
	repo := &authzDeleteRepo{}
	h := newAuthzTaskHandlers(t, repo)
	h.SetHandoffService(service.NewHandoffService(repo, nil, nil, nil, nil, nil))

	c, rec := authzDeleteRequest(t, "user-a", "task-b")
	h.httpDeleteTask(c)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign cascade delete: status = %d body = %s, want 404", rec.Code, rec.Body.String())
	}
	if got := repo.deletes(); len(got) != 0 {
		t.Fatalf("a denied cascade delete reached the repository: %v", got)
	}

	ownerCtx, ownerRec := authzDeleteRequest(t, "user-b", "task-b")
	h.httpDeleteTask(ownerCtx)

	if ownerRec.Code != http.StatusOK {
		t.Fatalf("owner cascade delete: status = %d body = %s, want 200", ownerRec.Code, ownerRec.Body.String())
	}
	if got := repo.deletes(); len(got) != 1 || got[0] != "task-b" {
		t.Fatalf("owner cascade delete did not reach the repository: %v", got)
	}
}

// TestWSArchiveTaskDeniesForeignTaskThroughCascade covers the third route the
// cascade substitution unscoped: wsArchiveTask also prefers HandoffService when
// one is wired, so it never reaches Service.ArchiveTask's guard either.
func TestWSArchiveTaskDeniesForeignTaskThroughCascade(t *testing.T) {
	repo := &authzDeleteRepo{}
	h := newAuthzTaskHandlers(t, repo)
	h.SetHandoffService(service.NewHandoffService(repo, nil, nil, nil, nil, nil))

	msg := &ws.Message{
		ID:      "msg-1",
		Action:  ws.ActionTaskArchive,
		Payload: json.RawMessage(`{"id":"task-b"}`),
	}

	denied, err := h.wsArchiveTask(authzWSContext("user-a"), msg)
	if err != nil {
		t.Fatalf("wsArchiveTask: %v", err)
	}
	if denied.Type != ws.MessageTypeError {
		t.Fatalf("foreign archive: type = %s payload = %s, want error", denied.Type, denied.Payload)
	}
	if got := repo.archives(); len(got) != 0 {
		t.Fatalf("a denied archive reached the repository: %v", got)
	}

	owner, err := h.wsArchiveTask(authzWSContext("user-b"), msg)
	if err != nil {
		t.Fatalf("owner archive: %v", err)
	}
	if owner.Type == ws.MessageTypeError {
		t.Fatalf("owner archive was refused: %s", owner.Payload)
	}
	if got := repo.archives(); len(got) != 1 {
		t.Fatalf("owner archive did not reach the repository: %v", got)
	}
}

// TestHTTPDeleteTaskSurvivesClientDisconnect is why the fix is
// context.WithoutCancel rather than a plain c.Request.Context() swap: a delete
// tears down worktrees, containers and subtree rows, and a browser that
// navigates away mid-request must not abort it half-done.
func TestHTTPDeleteTaskSurvivesClientDisconnect(t *testing.T) {
	repo := &authzDeleteRepo{}
	h := newAuthzTaskHandlers(t, repo)

	c, rec := authzDeleteRequest(t, "user-b", "task-b")
	gone, disconnect := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(gone)
	disconnect()

	h.httpDeleteTask(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("disconnected delete: status = %d body = %s, want 200", rec.Code, rec.Body.String())
	}
	if got := repo.deletes(); len(got) != 1 || got[0] != "task-b" {
		t.Fatalf("a client disconnect aborted the delete: %v", got)
	}
	// The status code alone proves nothing here — this fake repository ignores
	// its context, so a cancelled one would still "succeed". Assert the context
	// the delete was actually handed is live, which is what a real repository,
	// worktree teardown or container removal depends on.
	if !repo.deliveredLive() {
		t.Fatal("the delete was handed a cancelled context; a client disconnect can abort it half-done")
	}
}
