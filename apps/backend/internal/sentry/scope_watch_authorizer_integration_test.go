package sentry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

func newTaskServiceWithWorkspaceRepository(t *testing.T) (*taskservice.Service, repository.WorkspaceRepository) {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "task-service.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	database := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })

	repos, cleanup, err := repository.Provide(database, database, logger.Default())
	if err != nil {
		t.Fatalf("provide task repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	log := logger.Default()
	return taskservice.NewService(
		taskservice.Repos{Workspaces: repos},
		bus.NewMemoryEventBus(log),
		log,
		taskservice.RepositoryDiscoveryConfig{},
	), repos
}

func seedAuthorizerWorkspace(t *testing.T, workspaces repository.WorkspaceRepository, id, ownerID string) {
	t.Helper()
	if err := workspaces.CreateWorkspace(context.Background(), &models.Workspace{
		ID: id, Name: id, OwnerID: ownerID,
	}); err != nil {
		t.Fatalf("create workspace %q: %v", id, err)
	}
}

// TestHTTP_ListAllIssueWatches_UsesTaskServiceAuthorizer covers the complete
// unscoped HTTP path with the production workspace authorizer. It also keeps
// identity-less and synthetic callers unscoped when the authorizer is wired.
func TestHTTP_ListAllIssueWatches_UsesTaskServiceAuthorizer(t *testing.T) {
	taskSvc, workspaces := newTaskServiceWithWorkspaceRepository(t)
	seedAuthorizerWorkspace(t, workspaces, "ws-owned", "user-a")
	seedAuthorizerWorkspace(t, workspaces, "ws-foreign", "user-b")

	ctrl, router, _ := newTestController(t)
	ctrl.service.SetWorkspaceAuthorizer(taskSvc.AuthorizeWorkspaceAccess)
	for _, workspaceID := range []string{"ws-owned", "ws-foreign"} {
		if err := ctrl.service.store.CreateIssueWatch(context.Background(), newTestIssueWatch(workspaceID)); err != nil {
			t.Fatalf("seed watch for %q: %v", workspaceID, err)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/sentry/watches/issue", nil)
	request = request.WithContext(authn.WithIdentity(request.Context(), authn.Identity{
		UserID: "user-a",
		Role:   authn.RoleMember,
	}))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Watches []IssueWatch `json:"watches"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Watches) != 1 || body.Watches[0].WorkspaceID != "ws-owned" {
		t.Fatalf("HTTP watches = %+v, want only user-a's workspace", body.Watches)
	}

	for name, ctx := range map[string]context.Context{
		"identity-less": context.Background(),
		"synthetic": authn.WithIdentity(context.Background(), authn.Identity{
			UserID:    "synthetic",
			Role:      authn.RoleAdmin,
			Synthetic: true,
		}),
	} {
		watches, err := ctrl.service.ListAllIssueWatches(ctx)
		if err != nil {
			t.Fatalf("%s ListAllIssueWatches: %v", name, err)
		}
		if len(watches) != 2 {
			t.Errorf("%s watches = %+v, want both workspaces", name, watches)
		}
	}
}
