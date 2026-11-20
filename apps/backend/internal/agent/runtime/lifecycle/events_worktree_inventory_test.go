package lifecycle

import (
	"context"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestPublishWorktreeInventoryCarriesExecutionIdentity(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	eventBus := bus.NewMemoryEventBus(log)
	pub := NewEventPublisher(eventBus, log)

	received := make(chan *bus.Event, 1)
	subject := events.BuildWorkspaceWorktreesSubject("sess-inventory")
	sub, err := eventBus.Subscribe(subject, func(_ context.Context, ev *bus.Event) error {
		received <- ev
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	pub.PublishWorktreeInventory(&AgentExecution{
		ID: "exec-1", TaskID: "task-1", SessionID: "sess-inventory",
		TaskEnvironmentID: "env-inventory", ExecutorType: "worktree",
	}, &agentctl.WorktreeInventory{
		RepositoryName: "frontend",
		CommonDir:      "/task/.git",
		Entries: []agentctl.WorktreeInventoryEntry{{
			Path: "/task/.claude/worktrees/fix", Branch: "fix", HeadSHA: "abc123",
		}},
		ObservedAt: time.Now().UTC(),
	})

	select {
	case ev := <-received:
		payload, ok := ev.Data.(*WorktreeInventoryData)
		if !ok || payload == nil {
			t.Fatalf("expected *WorktreeInventoryData, got %T", ev.Data)
		}
		if payload.TaskID != "task-1" || payload.SessionID != "sess-inventory" || payload.TaskEnvironmentID != "env-inventory" || payload.ExecutorType != "worktree" {
			t.Fatalf("identity was dropped: %+v", payload)
		}
		if payload.RepositoryName != "frontend" || len(payload.Entries) != 1 || payload.Entries[0].Branch != "fix" {
			t.Fatalf("inventory payload was dropped: %+v", payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for worktree inventory event")
	}
}
