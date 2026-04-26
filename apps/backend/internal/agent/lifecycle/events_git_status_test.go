package lifecycle

import (
	"context"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// Verifies the fix for "Changes panel shows no header / no existing changes".
// agentctl tags every per-repo GitStatusUpdate with RepositoryName; that field
// must survive the lifecycle PublishGitStatus translation so the orchestrator
// (and thus the frontend) sees it.
func TestPublishGitStatus_PropagatesRepositoryName(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	eventBus := bus.NewMemoryEventBus(log)
	pub := NewEventPublisher(eventBus, log)

	received := make(chan *bus.Event, 1)
	subj := events.BuildGitEventSubject("sess-multi")
	sub, err := eventBus.Subscribe(subj, func(_ context.Context, ev *bus.Event) error {
		received <- ev
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	exec := &AgentExecution{
		ID:        "exec-1",
		TaskID:    "task-1",
		SessionID: "sess-multi",
	}
	pub.PublishGitStatus(exec, &agentctl.GitStatusUpdate{
		Timestamp:      time.Now(),
		RepositoryName: "frontend",
		Branch:         "feature/x",
		Modified:       []string{"src/app.tsx"},
		Files:          map[string]agentctl.FileInfo{"src/app.tsx": {Path: "src/app.tsx"}},
	})

	select {
	case ev := <-received:
		payload, ok := ev.Data.(*GitEventPayload)
		if !ok || payload == nil {
			t.Fatalf("expected *GitEventPayload, got %T", ev.Data)
		}
		if payload.Status == nil {
			t.Fatal("expected non-nil Status on payload")
		}
		if payload.Status.RepositoryName != "frontend" {
			t.Errorf("repository_name was dropped: got %q", payload.Status.RepositoryName)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for git status event")
	}
}
