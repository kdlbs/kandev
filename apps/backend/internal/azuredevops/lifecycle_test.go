package azuredevops

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestLifecycleCleanupRemovesTaskAndWorkspaceData(t *testing.T) {
	service, store, secrets := newTestService(t, nil)
	ctx := context.Background()
	if _, err := service.SetConfigForWorkspace(ctx, "ws-a", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme",
		PAT:             "pat",
	}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	if err := store.UpsertTaskPR(ctx, testTaskPR("task-a", "repo-a", 42)); err != nil {
		t.Fatalf("upsert task PR: %v", err)
	}

	eventBus := bus.NewMemoryEventBus(logger.Default())
	cleanup, err := RegisterLifecycleCleanup(eventBus, service)
	if err != nil {
		t.Fatalf("register lifecycle cleanup: %v", err)
	}
	t.Cleanup(func() { _ = cleanup.Close() })

	taskEvent := bus.NewEvent(events.TaskDeleted, "test", map[string]interface{}{"task_id": "task-a"})
	if err := eventBus.Publish(ctx, events.TaskDeleted, taskEvent); err != nil {
		t.Fatalf("publish task deletion: %v", err)
	}
	rows, err := store.ListTaskPRsByTask(ctx, "task-a")
	if err != nil || len(rows) != 0 {
		t.Fatalf("task PR rows after deletion = %+v, err = %v", rows, err)
	}

	workspaceEvent := bus.NewEvent(events.WorkspaceDeleted, "test", map[string]interface{}{"id": "ws-a"})
	if err := eventBus.Publish(ctx, events.WorkspaceDeleted, workspaceEvent); err != nil {
		t.Fatalf("publish workspace deletion: %v", err)
	}
	config, err := store.GetConfig(ctx, "ws-a")
	if err != nil || config != nil {
		t.Fatalf("config after workspace deletion = %+v, err = %v", config, err)
	}
	if exists, err := secrets.Exists(ctx, SecretKeyForWorkspace("ws-a")); err != nil || exists {
		t.Fatalf("secret exists after workspace deletion = %v, err = %v", exists, err)
	}
}
