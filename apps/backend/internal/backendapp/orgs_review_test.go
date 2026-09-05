package backendapp

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
	userstore "github.com/kandev/kandev/internal/user/store"
)

type orgReviewSecretDeleter struct {
	workspaceIDs []string
}

func (d *orgReviewSecretDeleter) DeleteWorkspaceSecrets(_ context.Context, workspaceID string) error {
	d.workspaceIDs = append(d.workspaceIDs, workspaceID)
	return nil
}

func TestOrganizationDeletionUsesWorkspaceLifecycle(t *testing.T) {
	ctx := context.Background()
	raw, err := db.OpenSQLite(filepath.Join(t.TempDir(), "org-delete.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	database := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	taskRepo, cleanupTaskRepo, err := repository.Provide(database, database, nil)
	if err != nil {
		t.Fatalf("task repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanupTaskRepo() })
	accounts, cleanupAccounts, err := userstore.Provide(database, database)
	if err != nil {
		t.Fatalf("user repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanupAccounts() })
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	taskSvc := taskservice.NewService(taskservice.Repos{
		Workspaces:        taskRepo,
		Tasks:             taskRepo,
		TaskRepos:         taskRepo,
		WorkspaceFolders:  taskRepo,
		Workflows:         taskRepo,
		Messages:          taskRepo,
		Attachments:       taskRepo,
		Turns:             taskRepo,
		Sessions:          taskRepo,
		GitSnapshots:      taskRepo,
		RepoEntities:      taskRepo,
		RepositorySets:    taskRepo,
		BranchPolicies:    taskRepo,
		RepositoryCleanup: taskRepo,
		Executors:         taskRepo,
		Environments:      taskRepo,
		TaskEnvironments:  taskRepo,
		Reviews:           taskRepo,
		ResourceCleanups:  taskRepo,
		StatusSummaries:   taskRepo,
		TaskActivity:      taskRepo,
		SubagentContexts:  taskRepo,
		Usage:             taskRepo,
	}, eventBus, log, taskservice.RepositoryDiscoveryConfig{})
	secretDeleter := &orgReviewSecretDeleter{}
	taskSvc.SetWorkspaceSecretDeleter(secretDeleter)

	cfg := &config.Config{}
	cfg.Features.MultiTenancy = true
	pool := db.NewPool(database, database)
	orgSvc, err := buildOrgService(cfg, pool, &Repositories{Task: taskRepo, UserAccounts: accounts}, taskSvc, log)
	if err != nil {
		t.Fatalf("organization service: %v", err)
	}
	defaultOrg, err := orgSvc.EnsureDefaultOrg(ctx, "Default organization")
	if err != nil {
		t.Fatalf("create default organization: %v", err)
	}
	doomedOrg, err := orgSvc.Create(ctx, "Doomed", "doomed")
	if err != nil {
		t.Fatalf("create doomed organization: %v", err)
	}
	if err := taskRepo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-doomed", Name: "Doomed workspace", OrgID: doomedOrg.ID,
	}); err != nil {
		t.Fatalf("create doomed workspace: %v", err)
	}
	if err := taskRepo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-keep", Name: "Kept workspace", OrgID: defaultOrg.ID,
	}); err != nil {
		t.Fatalf("create kept workspace: %v", err)
	}
	deletedWorkspaceID := ""
	subscription, err := eventBus.Subscribe(events.WorkspaceDeleted, func(_ context.Context, event *bus.Event) error {
		if data, ok := event.Data.(map[string]interface{}); ok {
			deletedWorkspaceID, _ = data["id"].(string)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe to workspace deletion: %v", err)
	}
	t.Cleanup(func() { _ = subscription.Unsubscribe() })

	if err := orgSvc.Delete(ctx, doomedOrg.ID, doomedOrg.Slug); err != nil {
		t.Fatalf("delete organization: %v", err)
	}
	if len(secretDeleter.workspaceIDs) != 1 || secretDeleter.workspaceIDs[0] != "ws-doomed" || deletedWorkspaceID != "ws-doomed" {
		t.Fatalf("workspace lifecycle secret deletes=%v deleted event=%q", secretDeleter.workspaceIDs, deletedWorkspaceID)
	}
	if _, err := taskRepo.GetWorkspace(ctx, "ws-keep"); err != nil {
		t.Fatalf("unrelated organization workspace was removed: %v", err)
	}
}
