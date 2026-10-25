package dashboard_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

func TestActivityLabelsWorkspaceScoped(t *testing.T) {
	deps := newTestDeps(t)
	ctx := context.Background()

	deps.agents.instances = []*models.AgentInstance{
		{ID: "agent-ws-1", WorkspaceID: "ws-1", Name: "Workspace One Agent"},
		{ID: "agent-ws-2", WorkspaceID: "ws-2", Name: "Workspace Two Agent"},
	}
	insertTestTask(t, deps.db, "task-ws-1", "ws-1", "Workspace One Task", "todo", 2)
	insertTestTask(t, deps.db, "task-ws-2", "ws-2", "Workspace Two Task", "todo", 2)

	createdAt := time.Now().UTC()
	if err := deps.repo.CreateActivityEntry(ctx, &models.ActivityEntry{
		WorkspaceID: "ws-1",
		ActorType:   models.ActivityActorType("agent"),
		ActorID:     "agent-ws-1",
		Action:      models.ActivityAction("task.updated"),
		TargetType:  models.ActivityTargetType("task"),
		TargetID:    "task-ws-1",
		CreatedAt:   createdAt,
	}); err != nil {
		t.Fatalf("create workspace-one activity: %v", err)
	}
	if err := deps.repo.CreateActivityEntry(ctx, &models.ActivityEntry{
		WorkspaceID: "ws-1",
		ActorType:   models.ActivityActorType("agent"),
		ActorID:     "agent-ws-2",
		Action:      models.ActivityAction("task.updated"),
		TargetType:  models.ActivityTargetType("task"),
		TargetID:    "task-ws-2",
		CreatedAt:   createdAt.Add(-time.Second),
	}); err != nil {
		t.Fatalf("create cross-workspace activity: %v", err)
	}

	entries, err := deps.svc.ListActivityFiltered(ctx, "ws-1", "all", 10)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("activity entry count = %d, want 2", len(entries))
	}
	if entries[0].ActorName != "Workspace One Agent" || entries[0].TargetName != "Workspace One Task" {
		t.Fatalf("workspace-one labels = actor %q target %q, want scoped names", entries[0].ActorName, entries[0].TargetName)
	}
	if entries[1].ActorName != "" || entries[1].TargetName != "" {
		t.Fatalf("cross-workspace labels = actor %q target %q, want empty", entries[1].ActorName, entries[1].TargetName)
	}
}
