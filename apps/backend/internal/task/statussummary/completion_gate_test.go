package statussummary

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/events"
)

func TestTaskStatusSummaryCompletionGateRoundTripsAndValidates(t *testing.T) {
	summary := TaskStatusSummary{CompletionGate: &CompletionGateSummary{
		Revision: 3, CriteriaCount: 2, VerifiedCount: 1, BlockerCount: 1, Blocked: true,
	}}
	payload, err := summary.SemanticJSON()
	if err != nil {
		t.Fatalf("semantic JSON: %v", err)
	}
	var decoded TaskStatusSummary
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode semantic JSON: %v", err)
	}
	if !summary.SemanticEqual(decoded) {
		t.Fatalf("completion gate changed after semantic round trip: %#v", decoded.CompletionGate)
	}
	invalid := summary
	invalid.CompletionGate = &CompletionGateSummary{Revision: 1, CriteriaCount: 1, VerifiedCount: 2, Blocked: true}
	if err := invalid.Validate(); err == nil {
		t.Fatal("summary accepted more verified criteria than total criteria")
	}
}

func TestProjectorRefreshesCompletionGateOnTaskAndPREvents(t *testing.T) {
	projector, store, eventBus, updates, _ := newProjectorTest(t)
	current := &CompletionGateSummary{Revision: 1, CriteriaCount: 2, VerifiedCount: 1, BlockerCount: 1, Blocked: true}
	loads := 0
	projector.loadCompletionGate = func(context.Context, string) (*CompletionGateSummary, error) {
		loads++
		copy := *current
		return &copy, nil
	}
	const taskID = "task-completion-projection"
	publishProjectorEvent(t, eventBus, events.TaskUpdated, events.TaskUpdated, map[string]interface{}{
		"task_id": taskID, "workspace_id": "workspace-1",
	})
	if got := store.summary(taskID); got == nil || got.CompletionGate == nil || *got.CompletionGate != *current {
		t.Fatalf("completion projection not refreshed: summary=%+v loads=%d updates=%d", got, loads, updates.Load())
	}
	firstCount := updates.Load()

	current = &CompletionGateSummary{Revision: 2, CriteriaCount: 2, VerifiedCount: 2}
	publishProjectorEvent(t, eventBus, events.GitHubTaskPRUpdated, events.GitHubTaskPRUpdated, map[string]interface{}{
		"task_id": taskID, "workspace_id": "workspace-1",
	})
	if loads < 3 {
		t.Fatalf("completion gate loader called %d times, want startup and both refresh events", loads)
	}
	assertCompletionGateProjection(t, store.summary(taskID), current)
	if updates.Load() <= firstCount {
		t.Fatal("PR-head change did not publish a refreshed completion projection")
	}
}

func assertCompletionGateProjection(t *testing.T, summary *TaskStatusSummary, expected *CompletionGateSummary) {
	t.Helper()
	if summary == nil || summary.CompletionGate == nil {
		t.Fatalf("summary completion gate = %#v, want %+v", summary, expected)
	}
	if *summary.CompletionGate != *expected {
		t.Fatalf("completion gate summary = %+v, want %+v", summary.CompletionGate, expected)
	}
}
