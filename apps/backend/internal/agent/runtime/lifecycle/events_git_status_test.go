package lifecycle

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
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
		ID:                "exec-1",
		TaskID:            "task-1",
		SessionID:         "sess-multi",
		TaskEnvironmentID: "env-multi",
	}
	pub.PublishGitStatus(exec, &agentctl.GitStatusUpdate{
		Timestamp:        time.Now(),
		RepositoryName:   "frontend",
		IsSubmodule:      true,
		Branch:           "feature/x",
		HeadCommit:       "local-head",
		BaseCommit:       "base-head",
		RemoteBranch:     "contributor/feature/x",
		RemoteHeadCommit: "upstream-head",
		RemoteAhead:      2,
		RemoteBehind:     1,
		Modified:         []string{"src/app.tsx"},
		Files:            map[string]agentctl.FileInfo{"src/app.tsx": {Path: "src/app.tsx"}},
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
		if !payload.Status.IsSubmodule {
			t.Error("is_submodule was dropped")
		}
		if payload.Status.HeadCommit != "local-head" || payload.Status.BaseCommit != "base-head" {
			t.Errorf("commit comparison SHAs were dropped: head=%q base=%q", payload.Status.HeadCommit, payload.Status.BaseCommit)
		}
		if payload.Status.RemoteHeadCommit != "upstream-head" || payload.Status.RemoteAhead != 2 || payload.Status.RemoteBehind != 1 {
			t.Errorf("upstream evidence was dropped: head=%q ahead=%d behind=%d", payload.Status.RemoteHeadCommit, payload.Status.RemoteAhead, payload.Status.RemoteBehind)
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal git event payload: %v", err)
		}
		var fields map[string]interface{}
		if err := json.Unmarshal(encoded, &fields); err != nil {
			t.Fatalf("decode git event payload: %v", err)
		}
		if got, _ := fields["task_environment_id"].(string); got != "env-multi" {
			t.Errorf("task_environment_id = %q, want env-multi", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for git status event")
	}
}
