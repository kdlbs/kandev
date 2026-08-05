package orchestrator

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/task/models"
)

// TestDecodeTaskMRUpdatedEvent_TypedPointer is the in-memory event bus shape:
// event.Data is the original *gitlab.TaskMRUpdatedEvent pointer.
func TestDecodeTaskMRUpdatedEvent_TypedPointer(t *testing.T) {
	original := &gitlab.TaskMRUpdatedEvent{
		WorkspaceID: "ws-1",
		TaskMR:      &gitlab.TaskMR{TaskID: "task-1", ProjectPath: "group/a", MRIID: 1},
	}
	got, ok := decodeTaskMRUpdatedEvent(original)
	if !ok || got != original {
		t.Fatalf("expected the original pointer to pass through unchanged, got %+v ok=%v", got, ok)
	}
}

// TestDecodeTaskMRUpdatedEvent_MapShape is the NATS event bus shape: Publish
// JSON-marshals the event and Subscribe unmarshals into the untyped Data
// field, which decodes any JSON object into a plain map — losing the
// concrete *gitlab.TaskMRUpdatedEvent type. Without decodeTaskMRUpdatedEvent
// normalizing this back, handleGitLabTaskMRUpdated's type assertion would
// always fail and every GitLab lifecycle notification would be silently
// dropped whenever the deployment uses NATS.
func TestDecodeTaskMRUpdatedEvent_MapShape(t *testing.T) {
	original := &gitlab.TaskMRUpdatedEvent{
		WorkspaceID: "ws-1",
		TaskMR:      &gitlab.TaskMR{TaskID: "task-1", RepositoryID: "repo-1", ProjectPath: "group/a", MRIID: 1, State: gitlabMRStateMerged},
	}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var asMap map[string]interface{}
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}

	got, ok := decodeTaskMRUpdatedEvent(asMap)
	if !ok || got == nil || got.TaskMR == nil {
		t.Fatalf("expected a decoded event, got %+v ok=%v", got, ok)
	}
	if got.WorkspaceID != "ws-1" || got.TaskID != "task-1" || got.RepositoryID != "repo-1" ||
		got.ProjectPath != "group/a" || got.MRIID != 1 || got.State != gitlabMRStateMerged {
		t.Fatalf("decoded event does not match original: %+v", got)
	}
}

// TestDecodeTaskMRUpdatedEvent_ReviewerObservationRoundTrip keeps the
// ephemeral reviewer observation intact across the map shape used by NATS.
// The explicit validity marker is required because an observed empty list is
// different from an old event that carried no reviewer observation at all.
func TestDecodeTaskMRUpdatedEvent_ReviewerObservationRoundTrip(t *testing.T) {
	raw := map[string]interface{}{
		"workspace_id":    "ws-1",
		"task_id":         "task-1",
		"project_path":    "group/a",
		"mr_iid":          float64(1),
		"reviewers":       []interface{}{},
		"reviewers_valid": true,
	}

	got, ok := decodeTaskMRUpdatedEvent(raw)
	if !ok || got == nil {
		t.Fatalf("expected a decoded event, got %+v ok=%v", got, ok)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal decoded event: %v", err)
	}
	var roundTripped map[string]interface{}
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal decoded event: %v", err)
	}
	if valid, ok := roundTripped["reviewers_valid"].(bool); !ok || !valid {
		t.Fatalf("reviewer validity was not preserved: %s", encoded)
	}
	if reviewers, ok := roundTripped["reviewers"].([]interface{}); !ok || len(reviewers) != 0 {
		t.Fatalf("observed empty reviewer list was not preserved: %s", encoded)
	}
}

// TestDecodeTaskMRUpdatedEvent_UnknownShape covers the same-defensive-default
// as the original direct type assertion: an unrelated payload is ignored,
// not misinterpreted.
func TestDecodeTaskMRUpdatedEvent_UnknownShape(t *testing.T) {
	got, ok := decodeTaskMRUpdatedEvent("not an event")
	if ok || got != nil {
		t.Fatalf("expected no decode for an unrelated payload, got %+v ok=%v", got, ok)
	}
}

// TestMRAutomationInFlightKey_DistinguishesProjectPathWhenRepositoryIDEmpty
// covers a self-managed GitLab host (no numeric project ID, so
// RepositoryID is empty) with two different projects that happen to share
// an MR IID. Without ProjectPath in the key, the two MRs' single-flight keys
// would collide and the second MR's lifecycle evaluation would be
// suppressed indefinitely by the first's still-in-flight entry.
func TestMRAutomationInFlightKey_DistinguishesProjectPathWhenRepositoryIDEmpty(t *testing.T) {
	a := &gitlab.TaskMR{TaskID: "task-1", RepositoryID: "", ProjectPath: "group/a", MRIID: 1}
	b := &gitlab.TaskMR{TaskID: "task-1", RepositoryID: "", ProjectPath: "group/b", MRIID: 1}

	keyA := mrAutomationInFlightKey(a)
	keyB := mrAutomationInFlightKey(b)
	if keyA == keyB {
		t.Fatalf("expected distinct in-flight keys for different project paths, both got %q", keyA)
	}
}

// TestMRAutomationInFlightKey_SameIdentityProducesSameKey guards against a
// regression that makes the key overly specific (e.g. including a
// non-identity field), which would break the single-flight dedup this key
// exists for.
func TestMRAutomationInFlightKey_SameIdentityProducesSameKey(t *testing.T) {
	a := &gitlab.TaskMR{TaskID: "task-1", RepositoryID: "repo-1", ProjectPath: "group/a", MRIID: 1, State: gitlabMRStateMerged}
	b := &gitlab.TaskMR{TaskID: "task-1", RepositoryID: "repo-1", ProjectPath: "group/a", MRIID: 1, State: "opened"}

	if mrAutomationInFlightKey(a) != mrAutomationInFlightKey(b) {
		t.Fatalf("expected the same in-flight key for the same (task, repository, project, iid) identity")
	}
}

// TestDecodeTaskMROptionsUpdatedEvent_TypedPointer is the in-memory event bus
// shape: event.Data is the original *gitlab.TaskMRAutomationResponse pointer.
func TestDecodeTaskMROptionsUpdatedEvent_TypedPointer(t *testing.T) {
	original := &gitlab.TaskMRAutomationResponse{TaskID: "task-1", PromptOnMerged: true}
	got, ok := decodeTaskMROptionsUpdatedEvent(original)
	if !ok || got != original {
		t.Fatalf("expected the original pointer to pass through unchanged, got %+v ok=%v", got, ok)
	}
}

// TestDecodeTaskMROptionsUpdatedEvent_MapShape is the NATS event bus shape.
func TestDecodeTaskMROptionsUpdatedEvent_MapShape(t *testing.T) {
	original := &gitlab.TaskMRAutomationResponse{TaskID: "task-1", PromptOnMerged: true}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var asMap map[string]interface{}
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}

	got, ok := decodeTaskMROptionsUpdatedEvent(asMap)
	if !ok || got == nil || got.TaskID != "task-1" || !got.PromptOnMerged {
		t.Fatalf("decoded event does not match original: %+v ok=%v", got, ok)
	}
}

// TestDecodeTaskMROptionsUpdatedEvent_UnknownShape covers the defensive
// default for an unrelated payload.
func TestDecodeTaskMROptionsUpdatedEvent_UnknownShape(t *testing.T) {
	got, ok := decodeTaskMROptionsUpdatedEvent(42)
	if ok || got != nil {
		t.Fatalf("expected no decode for an unrelated payload, got %+v ok=%v", got, ok)
	}
}

// TestHandleGitLabTaskMROptionsUpdated_DispatchesEveryLinkedMR proves the fix
// for a review finding: enabling a lifecycle switch (or any other options
// change, from HTTP PATCH, MCP, or the orchestrator's own recovery publish)
// must evaluate the task's linked MRs immediately rather than waiting for
// the next poller sweep. Each dispatched MR runs its evaluation in a
// detached goroutine of unpredictable speed, so this joins on
// checkpointCalls — an observable side effect inside the eval path — rather
// than racing the goroutine by peeking at the in-flight map right after the
// handler returns.
func TestHandleGitLabTaskMROptionsUpdated_DispatchesEveryLinkedMR(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	automation := &mockGitLabMRAutomationService{
		options: &gitlab.TaskMRAutomationResponse{PromptOnMerged: true},
		taskMRs: []*gitlab.TaskMR{
			{TaskID: "task-1", RepositoryID: "repo-1", ProjectPath: "group/a", MRIID: 1},
			{TaskID: "task-1", RepositoryID: "repo-1", ProjectPath: "group/b", MRIID: 2},
		},
		checkpointCalls: make(chan struct{}, 2),
	}
	svc.gitlabMRAutomation = automation

	event := &bus.Event{Data: &gitlab.TaskMRAutomationResponse{TaskID: "task-1", PromptOnMerged: true}}
	if err := svc.handleGitLabTaskMROptionsUpdated(ctx, event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	timeout := time.After(2 * time.Second)
	for range automation.taskMRs {
		select {
		case <-automation.checkpointCalls:
		case <-timeout:
			t.Fatal("timed out waiting for lifecycle evaluation to reach the checkpoint read for every linked MR")
		}
	}
}

// TestHandleGitLabTaskMROptionsUpdated_NoTaskIDIsNoop guards against a
// malformed or empty-payload event silently listing every task's MRs.
func TestHandleGitLabTaskMROptionsUpdated_NoTaskIDIsNoop(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	automation := &mockGitLabMRAutomationService{
		taskMRs: []*gitlab.TaskMR{{TaskID: "task-1", RepositoryID: "repo-1", ProjectPath: "group/a", MRIID: 1}},
	}
	svc.gitlabMRAutomation = automation

	event := &bus.Event{Data: &gitlab.TaskMRAutomationResponse{}}
	if err := svc.handleGitLabTaskMROptionsUpdated(ctx, event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := svc.mrAutomationInFlight.Load(mrAutomationInFlightKey(automation.taskMRs[0])); ok {
		t.Fatalf("expected no lifecycle automation dispatched for an empty task ID")
	}
}
