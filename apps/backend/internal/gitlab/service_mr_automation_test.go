package gitlab

import (
	"context"
	"errors"
	"testing"
)

// TestClientForTaskStrict_NilWorkspaceSecrets_NeverUsesAmbientClient is the
// AC32 regression test: a service without a workspace secret store must
// fail rather than silently binding to the legacy ambient Service.Client().
func TestClientForTaskStrict_NilWorkspaceSecrets_NeverUsesAmbientClient(t *testing.T) {
	store := newTestStore(t)
	ambient := NewMockClient("https://ambient.example.com")
	svc := NewService("https://ambient.example.com", ambient, AuthMethodPAT, nil, newTestLogger(t))
	svc.SetStore(store)
	// workspaceSecrets deliberately left nil.

	_, err := svc.clientForTaskStrict(context.Background(), "task-1")
	if !errors.Is(err, ErrWorkspaceClientRequired) {
		t.Fatalf("expected ErrWorkspaceClientRequired, got %v", err)
	}
}

func TestClientForTaskStrict_NilStore(t *testing.T) {
	svc := NewService(DefaultHost, NewNoopClient(DefaultHost), AuthMethodNone, nil, newTestLogger(t))
	_, err := svc.clientForTaskStrict(context.Background(), "task-1")
	if !errors.Is(err, ErrWorkspaceClientRequired) {
		t.Fatalf("expected ErrWorkspaceClientRequired, got %v", err)
	}
}

func TestClientForTaskStrict_EmptyWorkspaceIDOnTaskRow(t *testing.T) {
	store := newTestStore(t)
	seedWorkspace(t, store, "")
	seedTask(t, store, "task-1", "")
	svc := newWorkspaceConfigService(t, store, &configTestSecrets{values: make(map[string]string)})
	_, err := svc.clientForTaskStrict(context.Background(), "task-1")
	if !errors.Is(err, ErrWorkspaceClientRequired) {
		t.Fatalf("expected ErrWorkspaceClientRequired for empty workspace_id, got %v", err)
	}
}

// TestClientForTaskStrict_WorkspaceLookupErrorWrapsSentinel pins the
// documented contract that ErrWorkspaceClientRequired marks every failure
// path of clientForTaskStrict — including a WorkspaceIDForTask lookup error
// (an unseeded task row), not just the empty-workspace-id case.
func TestClientForTaskStrict_WorkspaceLookupErrorWrapsSentinel(t *testing.T) {
	store := newTestStore(t)
	svc := newWorkspaceConfigService(t, store, &configTestSecrets{values: make(map[string]string)})
	_, err := svc.clientForTaskStrict(context.Background(), "does-not-exist")
	if !errors.Is(err, ErrWorkspaceClientRequired) {
		t.Fatalf("expected ErrWorkspaceClientRequired for an unresolvable task, got %v", err)
	}
}

func TestClientForTaskStrict_ResolvesWorkspaceScopedClient(t *testing.T) {
	store := newTestStore(t)
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-1", "ws-1")
	if err := store.SaveConfigForWorkspace(context.Background(), "ws-1", &GitLabConfig{
		Host: "https://gitlab.example.com", AuthMethod: AuthMethodPAT,
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	secrets := &configTestSecrets{values: map[string]string{SecretKeyForWorkspace("ws-1"): "token"}}
	svc := newWorkspaceConfigService(t, store, secrets)

	client, err := svc.clientForTaskStrict(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("clientForTaskStrict: %v", err)
	}
	if client == nil || client.Host() != "https://gitlab.example.com" {
		t.Fatalf("unexpected client: %+v", client)
	}
}

func newSyncTaskMRStrictFixture(t *testing.T) *Service {
	t.Helper()
	store := newTestStore(t)
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-1", "ws-1")
	if err := store.SaveConfigForWorkspace(context.Background(), "ws-1", &GitLabConfig{
		Host: "https://gitlab.example.com", AuthMethod: AuthMethodPAT,
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	secrets := &configTestSecrets{values: map[string]string{SecretKeyForWorkspace("ws-1"): "token"}}
	svc := newWorkspaceConfigService(t, store, secrets)
	mock := NewMockClient("https://gitlab.example.com")
	mock.SeedMR("group/a", &MR{
		IID: 1, State: "opened", HeadBranch: "feat", BaseBranch: "main",
		WebURL: "https://gitlab.example.com/group/a/-/merge_requests/1",
	})
	svc.workspaceClientFn = func(_ context.Context, _ *GitLabConfig, _ string) (Client, error) {
		return mock, nil
	}
	return svc
}

// TestSyncTaskMRStrict_RejectsEmptyExistingHost pins the P1 finding: an
// unknown-identity (empty host) row must fail closed, not be silently
// treated as "no check needed." validateHost's config-fallback default
// (empty -> DefaultHost) makes this the same as "no check" unless handled
// explicitly before origin comparison.
func TestSyncTaskMRStrict_RejectsEmptyExistingHost(t *testing.T) {
	svc := newSyncTaskMRStrictFixture(t)
	_, err := svc.SyncTaskMRStrict(context.Background(), "task-1", "", "group/a", 1, "")
	if !errors.Is(err, ErrTaskMRHostMismatch) {
		t.Fatalf("expected ErrTaskMRHostMismatch for an empty existing host, got %v", err)
	}
}

// TestSyncTaskMRStrict_AcceptsEquivalentOrigin pins that the host guard
// compares configured origins (scheme + host, default port stripped), not
// raw strings — an explicit ":443" on the stored side must still match.
func TestSyncTaskMRStrict_AcceptsEquivalentOrigin(t *testing.T) {
	svc := newSyncTaskMRStrictFixture(t)
	_, err := svc.SyncTaskMRStrict(context.Background(), "task-1", "", "group/a", 1, "https://gitlab.example.com:443")
	if err != nil {
		t.Fatalf("expected the explicit-default-port origin to match, got %v", err)
	}
}

// TestSyncTaskMRStrict_RejectsDifferentOrigin is the converse: a genuinely
// different host must still fail closed.
func TestSyncTaskMRStrict_RejectsDifferentOrigin(t *testing.T) {
	svc := newSyncTaskMRStrictFixture(t)
	_, err := svc.SyncTaskMRStrict(context.Background(), "task-1", "", "group/a", 1, "https://gitlab.other.example.com")
	if !errors.Is(err, ErrTaskMRHostMismatch) {
		t.Fatalf("expected ErrTaskMRHostMismatch for a different host, got %v", err)
	}
}

func TestHasEnabledTaskMRAgentPrompts(t *testing.T) {
	store := newTestStore(t)
	svc := newWorkspaceConfigService(t, store, &configTestSecrets{values: make(map[string]string)})
	ctx := context.Background()
	seedTask(t, store, "task-1", "")

	enabled, err := svc.HasEnabledTaskMRAgentPrompts(ctx, "task-1")
	if err != nil || enabled {
		t.Fatalf("expected disabled by default: enabled=%v err=%v", enabled, err)
	}

	if _, err := store.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		PromptOnClosed: boolPtr(true),
	}, nil); err != nil {
		t.Fatalf("enable switch: %v", err)
	}
	enabled, err = svc.HasEnabledTaskMRAgentPrompts(ctx, "task-1")
	if err != nil || !enabled {
		t.Fatalf("expected enabled after patch: enabled=%v err=%v", enabled, err)
	}
}

func TestUpdateTaskMRAutomationOptions_ResolvesAndClearsReviewer(t *testing.T) {
	store := newTestStore(t)
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-1", "ws-1")
	if err := store.SaveConfigForWorkspace(context.Background(), "ws-1", &GitLabConfig{
		Host: "https://gitlab.example.com", AuthMethod: AuthMethodPAT,
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	secrets := &configTestSecrets{values: map[string]string{SecretKeyForWorkspace("ws-1"): "token"}}
	svc := newWorkspaceConfigService(t, store, secrets)
	mock := NewMockClient("https://gitlab.example.com")
	mock.SetUser("alice")
	svc.workspaceClientFn = func(_ context.Context, _ *GitLabConfig, _ string) (Client, error) {
		return mock, nil
	}
	ctx := context.Background()

	resp, err := svc.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		PromptOnReviewRequested: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("UpdateTaskMRAutomationOptions (enable): %v", err)
	}
	if !resp.PromptOnReviewRequested || resp.ReviewReviewerUsername != "alice" {
		t.Fatalf("expected resolved reviewer, got %+v", resp)
	}

	resp, err = svc.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		PromptOnReviewRequested: boolPtr(false),
	})
	if err != nil {
		t.Fatalf("UpdateTaskMRAutomationOptions (disable): %v", err)
	}
	if resp.PromptOnReviewRequested || resp.ReviewReviewerUsername != "" {
		t.Fatalf("expected cleared reviewer, got %+v", resp)
	}
}
