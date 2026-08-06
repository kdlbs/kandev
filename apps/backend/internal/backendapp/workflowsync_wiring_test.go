package backendapp

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// TestInitWorkflowSyncService_WiresRealWorkspaceAuthorization calls the real
// initWorkflowSyncService boot function — not a hand-copied re-wiring — with a
// real taskservice.Service backed by SQLite, so a regression that removes or
// breaks the SetWorkspaceAuthorizer(taskSvc.AuthorizeWorkspaceAccess) line
// inside it fails this test. The workflowsync package's own tests only ever
// install a hand-written fake authorizer and cannot detect that class of
// regression.
func TestInitWorkflowSyncService_WiresRealWorkspaceAuthorization(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()

	ownerCtx := authn.WithIdentity(ctx, authn.Identity{UserID: "owner-1", Role: authn.RoleMember})
	workspace, err := harness.taskSvc.CreateWorkspace(ownerCtx, &taskservice.CreateWorkspaceRequest{
		Name: "victim workspace",
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if workspace.OwnerID != "owner-1" {
		t.Fatalf("workspace owner = %q, want owner-1 (authenticated caller owns what it creates)", workspace.OwnerID)
	}

	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "workflowsync.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() {
		if err := sqlxDB.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}

	// A non-nil GitHub service is required for initWorkflowSyncService to
	// wire anything at all (see its early "no GitHub or GitLab service
	// available" return); the client itself is never exercised by this
	// test. GitLab is left nil — this test only covers the shared
	// authorization wiring, which applies regardless of provider.
	githubSvc := github.NewService(nil, github.AuthMethodNone, nil, nil, nil, log)

	svc := initWorkflowSyncService(db.NewPool(sqlxDB, sqlxDB), githubSvc, nil, harness.workflowSvc, harness.taskSvc, log)
	if svc == nil {
		t.Fatal("initWorkflowSyncService returned nil")
	}

	if _, err := svc.GetConfigForWorkspace(ownerCtx, workspace.ID); err != nil {
		t.Fatalf("owner GetConfigForWorkspace: %v", err)
	}

	foreignCtx := authn.WithIdentity(ctx, authn.Identity{UserID: "attacker-1", Role: authn.RoleMember})
	_, err = svc.GetConfigForWorkspace(foreignCtx, workspace.ID)
	if !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("foreign identity GetConfigForWorkspace err = %v, want ErrWorkspaceNotFound", err)
	}

	// Internal caller (no identity in context — matches the periodic poller)
	// and the pre-auth synthetic identity are both unscoped.
	if _, err := svc.GetConfigForWorkspace(ctx, workspace.ID); err != nil {
		t.Fatalf("identity-free GetConfigForWorkspace: %v", err)
	}
	syntheticCtx := authn.WithIdentity(ctx, authn.Identity{UserID: "single-user", Role: authn.RoleAdmin, Synthetic: true})
	if _, err := svc.GetConfigForWorkspace(syntheticCtx, workspace.ID); err != nil {
		t.Fatalf("synthetic identity GetConfigForWorkspace: %v", err)
	}
}
