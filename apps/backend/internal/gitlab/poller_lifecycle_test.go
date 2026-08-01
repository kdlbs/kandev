package gitlab

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// TestPoller_RunMRLifecycleSync_SyncsSubscribedRowsAndPublishes is AC22: the
// poller's lifecycle pass syncs every gitlab_task_mrs row whose task has at
// least one switch enabled, and publishes gitlab.task_mr.updated so the
// orchestrator's lifecycle evaluation runs on fresh state. An unsubscribed
// row is left untouched.
func TestPoller_RunMRLifecycleSync_SyncsSubscribedRowsAndPublishes(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-1", "ws-1")
	seedTask(t, store, "task-2", "ws-1")
	if err := store.SaveConfigForWorkspace(ctx, "ws-1", &GitLabConfig{
		Host: "https://gitlab.example.com", AuthMethod: AuthMethodPAT,
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	secrets := &configTestSecrets{values: map[string]string{SecretKeyForWorkspace("ws-1"): "token"}}
	svc := newWorkspaceConfigService(t, store, secrets)
	mock := NewMockClient("https://gitlab.example.com")
	mock.SeedMR("group/subscribed", &MR{IID: 1, State: "opened", HeadBranch: "feat", BaseBranch: "main", WebURL: "https://gitlab.example.com/group/subscribed/-/merge_requests/1"})
	svc.workspaceClientFn = func(_ context.Context, _ *GitLabConfig, _ string) (Client, error) {
		return mock, nil
	}

	if err := store.UpsertTaskMR(ctx, newTestMR("task-1", "", "group/subscribed", 1)); err != nil {
		t.Fatalf("seed subscribed MR: %v", err)
	}
	if err := store.UpsertTaskMR(ctx, newTestMR("task-2", "", "group/unsubscribed", 2)); err != nil {
		t.Fatalf("seed unsubscribed MR: %v", err)
	}
	if _, err := store.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		PromptOnMerged: boolPtr(true),
	}, nil); err != nil {
		t.Fatalf("enable switch: %v", err)
	}

	memBus := bus.NewMemoryEventBus(newTestLogger(t))
	svc.SetEventBus(memBus)
	var received []*TaskMRUpdatedEvent
	if _, err := memBus.Subscribe(events.GitLabTaskMRUpdated, func(_ context.Context, e *bus.Event) error {
		if payload, ok := e.Data.(*TaskMRUpdatedEvent); ok {
			received = append(received, payload)
		}
		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	poller := NewPoller(svc, memBus, newTestLogger(t))
	poller.runMRLifecycleSync(ctx)

	if len(received) != 1 {
		t.Fatalf("published events = %d, want 1 (only the subscribed MR)", len(received))
	}
	if received[0].TaskID != "task-1" || received[0].ProjectPath != "group/subscribed" {
		t.Fatalf("unexpected published MR: %+v", received[0])
	}
}

// TestPoller_RunMRLifecycleSync_ErrorOnOneRowDoesNotAbortOthers is AC25: a
// sync failure for one row is recorded on its checkpoint and does not
// prevent the pass from evaluating the remaining rows.
func TestPoller_RunMRLifecycleSync_ErrorOnOneRowDoesNotAbortOthers(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-1", "ws-1")
	seedTask(t, store, "task-2", "ws-1")
	if err := store.SaveConfigForWorkspace(ctx, "ws-1", &GitLabConfig{
		Host: "https://gitlab.example.com", AuthMethod: AuthMethodPAT,
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	secrets := &configTestSecrets{values: map[string]string{SecretKeyForWorkspace("ws-1"): "token"}}
	svc := newWorkspaceConfigService(t, store, secrets)
	mock := NewMockClient("https://gitlab.example.com")
	// task-1's MR (iid 1) is deliberately NOT seeded in the mock client, so
	// GetMRStatus fails for it; task-2's MR (iid 2) is seeded and succeeds.
	mock.SeedMR("group/ok", &MR{IID: 2, State: "opened", HeadBranch: "feat", BaseBranch: "main", WebURL: "https://gitlab.example.com/group/ok/-/merge_requests/2"})
	svc.workspaceClientFn = func(_ context.Context, _ *GitLabConfig, _ string) (Client, error) {
		return mock, nil
	}

	if err := store.UpsertTaskMR(ctx, newTestMR("task-1", "", "group/broken", 1)); err != nil {
		t.Fatalf("seed broken MR: %v", err)
	}
	if err := store.UpsertTaskMR(ctx, newTestMR("task-2", "", "group/ok", 2)); err != nil {
		t.Fatalf("seed ok MR: %v", err)
	}
	for _, taskID := range []string{"task-1", "task-2"} {
		if _, err := store.UpdateTaskMRAutomationOptions(ctx, taskID, TaskMRAutomationPatch{
			PromptOnMerged: boolPtr(true),
		}, nil); err != nil {
			t.Fatalf("enable switch for %s: %v", taskID, err)
		}
	}

	memBus := bus.NewMemoryEventBus(newTestLogger(t))
	svc.SetEventBus(memBus)
	var received []*TaskMRUpdatedEvent
	if _, err := memBus.Subscribe(events.GitLabTaskMRUpdated, func(_ context.Context, e *bus.Event) error {
		if payload, ok := e.Data.(*TaskMRUpdatedEvent); ok {
			received = append(received, payload)
		}
		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	poller := NewPoller(svc, memBus, newTestLogger(t))
	poller.runMRLifecycleSync(ctx)

	if len(received) != 1 || received[0].TaskID != "task-2" {
		t.Fatalf("expected only task-2 to sync successfully, got %+v", received)
	}
	state, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/broken", 1)
	if err != nil {
		t.Fatalf("get lifecycle state: %v", err)
	}
	if state == nil || state.LastError == nil || *state.LastError == "" {
		t.Fatalf("expected last_error recorded for the broken row, got %+v", state)
	}
}
