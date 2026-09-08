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
	mock.SeedMR("group/subscribed", &MR{
		IID: 1, State: "opened", HeadBranch: "feat", BaseBranch: "main",
		WebURL:    "https://gitlab.example.com/group/subscribed/-/merge_requests/1",
		Reviewers: []MRReviewer{{Username: "alice"}},
	})
	// Seeded (not just omitted) so a filter regression that wrongly included
	// the unsubscribed row would still succeed the sync and publish a second
	// event, rather than failing for an unrelated reason (no seeded MR) and
	// masking the real assertion below.
	mock.SeedMR("group/unsubscribed", &MR{IID: 2, State: "opened", HeadBranch: "feat", BaseBranch: "main", WebURL: "https://gitlab.example.com/group/unsubscribed/-/merge_requests/2"})
	svc.workspaceClientFn = func(_ context.Context, _ *GitLabConfig, _ string) (Client, error) {
		return mock, nil
	}

	subscribed := newTestMR("task-1", "", "group/subscribed", 1)
	subscribed.Host = "https://gitlab.example.com"
	if err := store.UpsertTaskMR(ctx, subscribed); err != nil {
		t.Fatalf("seed subscribed MR: %v", err)
	}
	unsubscribed := newTestMR("task-2", "", "group/unsubscribed", 2)
	unsubscribed.Host = "https://gitlab.example.com"
	if err := store.UpsertTaskMR(ctx, unsubscribed); err != nil {
		t.Fatalf("seed unsubscribed MR: %v", err)
	}
	setMRSwitches(t, store, "task-1", mrIdentity("group/subscribed", 1), TaskMRAutomationSwitchPatch{
		PromptOnMerged: boolPtr(true),
	})

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
	if !received[0].ReviewersValid || len(received[0].Reviewers) != 1 || received[0].Reviewers[0].Username != "alice" {
		t.Fatalf("expected reviewer observation on lifecycle event, got valid=%v reviewers=%+v", received[0].ReviewersValid, received[0].Reviewers)
	}
	// The doc comment's "left untouched" claim only holds if nothing evaluated
	// the unsubscribed row either — verify no checkpoint exists for it, not
	// just that it didn't publish (a regression could mutate the row silently
	// without emitting its event).
	unsubscribedState, err := store.GetTaskMRLifecycleState(ctx, "task-2", "", "group/unsubscribed", 2)
	if err != nil {
		t.Fatalf("get unsubscribed lifecycle state: %v", err)
	}
	if unsubscribedState != nil {
		t.Fatalf("expected no checkpoint for the unsubscribed row, got %+v", unsubscribedState)
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

	broken := newTestMR("task-1", "", "group/broken", 1)
	broken.Host = "https://gitlab.example.com"
	if err := store.UpsertTaskMR(ctx, broken); err != nil {
		t.Fatalf("seed broken MR: %v", err)
	}
	ok := newTestMR("task-2", "", "group/ok", 2)
	ok.Host = "https://gitlab.example.com"
	if err := store.UpsertTaskMR(ctx, ok); err != nil {
		t.Fatalf("seed ok MR: %v", err)
	}
	setMRSwitches(t, store, "task-1", mrIdentity("group/broken", 1), TaskMRAutomationSwitchPatch{
		PromptOnMerged: boolPtr(true),
	})
	setMRSwitches(t, store, "task-2", mrIdentity("group/ok", 2), TaskMRAutomationSwitchPatch{
		PromptOnMerged: boolPtr(true),
	})

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
	if state == nil || state.LastSyncError == nil || *state.LastSyncError == "" {
		t.Fatalf("expected last_sync_error recorded for the broken row, got %+v", state)
	}
}

// TestPoller_RunMRLifecycleSync_UsesStrictClient is AC32: the lifecycle sync
// pass must resolve its GitLab client through clientForTaskStrict, never the
// ambient/legacy Service.Client() singleton. The ambient client here is
// seeded with the MR and would succeed if used; the workspace has no secret
// store configured, so only the strict path (which never falls back) fails
// closed and records the checkpoint error instead of silently syncing
// against the wrong account.
func TestPoller_RunMRLifecycleSync_UsesStrictClient(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-1", "ws-1")
	if err := store.SaveConfigForWorkspace(ctx, "ws-1", &GitLabConfig{
		Host: "https://gitlab.example.com", AuthMethod: AuthMethodPAT,
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	ambientClient := NewMockClient("https://gitlab.example.com")
	ambientClient.SeedMR("group/subscribed", &MR{
		IID: 1, State: "opened", HeadBranch: "feat", BaseBranch: "main",
		WebURL: "https://gitlab.example.com/group/subscribed/-/merge_requests/1",
	})
	svc := NewService("https://gitlab.example.com", ambientClient, AuthMethodPAT, nil, newTestLogger(t))
	svc.SetStore(store)
	// No SetWorkspaceSecretStore call: workspaceSecrets stays nil, so
	// clientForTaskStrict must fail rather than fall back to ambientClient.

	if err := store.UpsertTaskMR(ctx, newTestMR("task-1", "", "group/subscribed", 1)); err != nil {
		t.Fatalf("seed subscribed MR: %v", err)
	}
	setMRSwitches(t, store, "task-1", mrIdentity("group/subscribed", 1), TaskMRAutomationSwitchPatch{
		PromptOnMerged: boolPtr(true),
	})

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

	if len(received) != 0 {
		t.Fatalf("expected no successful sync via the ambient client, got %+v", received)
	}
	state, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/subscribed", 1)
	if err != nil {
		t.Fatalf("get lifecycle state: %v", err)
	}
	if state == nil || state.LastSyncError == nil || *state.LastSyncError == "" {
		t.Fatalf("expected last_sync_error recorded (strict client fails closed), got %+v", state)
	}
}

// TestPoller_RunMRLifecycleSync_RejectsHostChangeSinceLink guards against
// syncing a same-path MR on a newly configured GitLab host: if the
// workspace's connection host changed since this MR was linked, the
// project_path+IID pair could coincidentally resolve to an unrelated MR on
// the new host. The sync must fail (recording the checkpoint error) rather
// than silently overwrite the row and emit lifecycle automation for it.
func TestPoller_RunMRLifecycleSync_RejectsHostChangeSinceLink(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-1", "ws-1")
	if err := store.SaveConfigForWorkspace(ctx, "ws-1", &GitLabConfig{
		Host: "https://gitlab.new.example.com", AuthMethod: AuthMethodPAT,
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	secrets := &configTestSecrets{values: map[string]string{SecretKeyForWorkspace("ws-1"): "token"}}
	svc := newWorkspaceConfigService(t, store, secrets)
	mock := NewMockClient("https://gitlab.new.example.com")
	mock.SeedMR("group/subscribed", &MR{
		IID: 1, State: "opened", HeadBranch: "feat", BaseBranch: "main",
		WebURL: "https://gitlab.new.example.com/group/subscribed/-/merge_requests/1",
	})
	svc.workspaceClientFn = func(_ context.Context, _ *GitLabConfig, _ string) (Client, error) {
		return mock, nil
	}

	// Linked while the workspace pointed at the OLD host.
	linked := newTestMR("task-1", "", "group/subscribed", 1)
	linked.Host = "https://gitlab.old.example.com"
	if err := store.UpsertTaskMR(ctx, linked); err != nil {
		t.Fatalf("seed linked MR: %v", err)
	}
	setMRSwitches(t, store, "task-1", mrIdentity("group/subscribed", 1), TaskMRAutomationSwitchPatch{
		PromptOnMerged: boolPtr(true),
	})

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

	if len(received) != 0 {
		t.Fatalf("expected no sync across a host change, got %+v", received)
	}
	state, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/subscribed", 1)
	if err != nil {
		t.Fatalf("get lifecycle state: %v", err)
	}
	if state == nil || state.LastSyncError == nil || *state.LastSyncError == "" {
		t.Fatalf("expected last_sync_error recorded for the host mismatch, got %+v", state)
	}
}

// TestPoller_RunMRLifecycleSync_ClearsRecoveredError is AC25's converse: a
// previously recorded sync failure must not linger in last_error once the
// MR syncs successfully again, or the checkpoint would misreport an active
// failure indefinitely.
func TestPoller_RunMRLifecycleSync_ClearsRecoveredError(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-1", "ws-1")
	if err := store.SaveConfigForWorkspace(ctx, "ws-1", &GitLabConfig{
		Host: "https://gitlab.example.com", AuthMethod: AuthMethodPAT,
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	secrets := &configTestSecrets{values: map[string]string{SecretKeyForWorkspace("ws-1"): "token"}}
	svc := newWorkspaceConfigService(t, store, secrets)
	mock := NewMockClient("https://gitlab.example.com")
	mock.SeedMR("group/subscribed", &MR{
		IID: 1, State: "opened", HeadBranch: "feat", BaseBranch: "main",
		WebURL: "https://gitlab.example.com/group/subscribed/-/merge_requests/1",
	})
	svc.workspaceClientFn = func(_ context.Context, _ *GitLabConfig, _ string) (Client, error) {
		return mock, nil
	}

	subscribed := newTestMR("task-1", "", "group/subscribed", 1)
	subscribed.Host = "https://gitlab.example.com"
	if err := store.UpsertTaskMR(ctx, subscribed); err != nil {
		t.Fatalf("seed MR: %v", err)
	}
	setMRSwitches(t, store, "task-1", mrIdentity("group/subscribed", 1), TaskMRAutomationSwitchPatch{
		PromptOnMerged: boolPtr(true),
	})
	if err := store.RecordTaskMRSyncError(ctx, "task-1", "", "group/subscribed", 1, "prior failure"); err != nil {
		t.Fatalf("seed prior error: %v", err)
	}

	memBus := bus.NewMemoryEventBus(newTestLogger(t))
	svc.SetEventBus(memBus)

	poller := NewPoller(svc, memBus, newTestLogger(t))
	poller.runMRLifecycleSync(ctx)

	state, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/subscribed", 1)
	if err != nil {
		t.Fatalf("get lifecycle state: %v", err)
	}
	if state == nil || state.LastSyncError != nil {
		t.Fatalf("expected last_sync_error cleared after a successful sync, got %+v", state)
	}
}
