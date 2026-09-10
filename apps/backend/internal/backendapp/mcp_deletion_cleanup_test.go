package backendapp

import (
	"context"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

type mcpDeletionCleanerStub struct {
	tasks      []string
	sessions   [][]string
	workspaces []string
}

func (s *mcpDeletionCleanerStub) DeleteMCPTaskData(_ context.Context, taskID string, sessionIDs []string) error {
	s.tasks = append(s.tasks, taskID)
	s.sessions = append(s.sessions, append([]string(nil), sessionIDs...))
	return nil
}

func (s *mcpDeletionCleanerStub) DeleteMCPWorkspaceData(_ context.Context, workspaceID string) error {
	s.workspaces = append(s.workspaces, workspaceID)
	return nil
}

func TestRegisterMCPDeletionCleanupReadsDeletionEventScopes(t *testing.T) {
	eventBus := bus.NewMemoryEventBus(logger.Default())
	cleaner := &mcpDeletionCleanerStub{}
	registerMCPDeletionCleanup(eventBus, cleaner, logger.Default())

	if err := eventBus.Publish(context.Background(), events.TaskDeleted, bus.NewEvent(events.TaskDeleted, "test", map[string]interface{}{
		"task_id":         " task-1 ",
		"mcp_session_ids": []string{"session-1", " session-1 ", "", "session-2"},
	})); err != nil {
		t.Fatalf("publish task.deleted: %v", err)
	}
	if err := eventBus.Publish(context.Background(), events.WorkspaceDeleted, bus.NewEvent(events.WorkspaceDeleted, "test", map[string]interface{}{
		"id": " workspace-1 ",
	})); err != nil {
		t.Fatalf("publish workspace.deleted: %v", err)
	}

	if !reflect.DeepEqual(cleaner.tasks, []string{"task-1"}) {
		t.Fatalf("task cleanup IDs = %#v", cleaner.tasks)
	}
	if !reflect.DeepEqual(cleaner.sessions, [][]string{{"session-1", "session-2"}}) {
		t.Fatalf("session cleanup IDs = %#v", cleaner.sessions)
	}
	if !reflect.DeepEqual(cleaner.workspaces, []string{"workspace-1"}) {
		t.Fatalf("workspace cleanup IDs = %#v", cleaner.workspaces)
	}
}
