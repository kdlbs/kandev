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

	// One linked MR with a lifecycle switch on is enough — the gate reads the
	// per-MR table, so a task whose MRs are configured differently still
	// counts as subscribed.
	setMRSwitches(t, store, "task-1", mrIdentity("group/a", 1), TaskMRAutomationSwitchPatch{
		PromptOnClosed: boolPtr(true),
	})
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
	if err := store.UpsertTaskMR(ctx, newTestMR("task-1", "", "group/a", 1)); err != nil {
		t.Fatalf("upsert MR: %v", err)
	}

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

// newMRAutomationServiceFixture builds a workspace-configured service whose
// strict client resolves to `username`, the shape every targeting test below
// needs.
func newMRAutomationServiceFixture(t *testing.T, username string) (*Service, *Store) {
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
	mock.SetUser(username)
	svc.workspaceClientFn = func(_ context.Context, _ *GitLabConfig, _ string) (Client, error) {
		return mock, nil
	}
	return svc, store
}

// TestUpdateTaskMRAutomationOptions_TargetsOnlyTheNamedMR is the regression
// this change exists for at the service layer: a patch carrying one MR's
// identity must not touch the task's other linked MRs.
func TestUpdateTaskMRAutomationOptions_TargetsOnlyTheNamedMR(t *testing.T) {
	svc, store := newMRAutomationServiceFixture(t, "alice")
	ctx := context.Background()
	for _, mr := range []*TaskMR{
		newTestMR("task-1", "", "group/a", 1),
		newTestMR("task-1", "", "group/b", 2),
	} {
		if err := store.UpsertTaskMR(ctx, mr); err != nil {
			t.Fatalf("upsert MR %s: %v", mr.ProjectPath, err)
		}
	}

	resp, err := svc.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		RepositoryID: stringPtr(""), ProjectPath: stringPtr("group/a"), MRIID: intPtr(1),
		AutoMergeEnabled: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("UpdateTaskMRAutomationOptions: %v", err)
	}
	if len(resp.MROptions) != 2 {
		t.Fatalf("expected one entry per linked MR, got %+v", resp.MROptions)
	}
	byProject := map[string]*TaskMRAutomationOptionsForMR{}
	for _, opt := range resp.MROptions {
		byProject[opt.ProjectPath] = opt
	}
	if !byProject["group/a"].AutoMergeEnabled {
		t.Errorf("targeted MR did not get the switch: %+v", byProject["group/a"])
	}
	if byProject["group/b"].AutoMergeEnabled {
		t.Errorf("untargeted MR inherited the switch: %+v", byProject["group/b"])
	}
	// The aggregate is "every linked MR", so a partially configured task
	// must not report the switch as on.
	if resp.AutoMergeEnabled {
		t.Errorf("aggregate reported auto-merge on while only one of two MRs has it")
	}
}

// TestUpdateTaskMRAutomationOptions_FansOutWhenNoMRIsNamed preserves the
// behavior of MCP callers that have no MR identity to send.
func TestUpdateTaskMRAutomationOptions_FansOutWhenNoMRIsNamed(t *testing.T) {
	svc, store := newMRAutomationServiceFixture(t, "alice")
	ctx := context.Background()
	for _, mr := range []*TaskMR{
		newTestMR("task-1", "", "group/a", 1),
		newTestMR("task-1", "", "group/b", 2),
	} {
		if err := store.UpsertTaskMR(ctx, mr); err != nil {
			t.Fatalf("upsert MR %s: %v", mr.ProjectPath, err)
		}
	}

	resp, err := svc.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		PromptOnMerged: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("UpdateTaskMRAutomationOptions: %v", err)
	}
	for _, opt := range resp.MROptions {
		if !opt.PromptOnMerged {
			t.Errorf("MR %s missed the fan-out: %+v", opt.ProjectPath, opt)
		}
	}
	if !resp.PromptOnMerged {
		t.Errorf("aggregate should report on once every linked MR has it: %+v", resp)
	}
}

// TestUpdateTaskMRAutomationOptions_RollsBackMixedPatch keeps the task-level
// prompt override and per-MR switch write in one transaction. A failed switch
// write must not leave the prompt override committed while returning an error.
func TestUpdateTaskMRAutomationOptions_RollsBackMixedPatch(t *testing.T) {
	svc, store := newMRAutomationServiceFixture(t, "alice")
	ctx := context.Background()
	if err := store.UpsertTaskMR(ctx, newTestMR("task-1", "", "group/a", 1)); err != nil {
		t.Fatalf("upsert MR: %v", err)
	}
	if _, err := store.db.Exec(`
		CREATE TRIGGER fail_mr_switch BEFORE UPDATE ON gitlab_task_mr_automation_options
		BEGIN SELECT RAISE(ABORT, 'injected failure'); END`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	_, err := svc.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		AutoFixPromptOverride: stringPtr("must rollback"),
		AutoMergeEnabled:      boolPtr(true),
	})
	if err == nil {
		t.Fatal("expected mixed patch to fail")
	}
	options, err := store.GetTaskMRAutomationOptions(ctx, "task-1")
	if err != nil {
		t.Fatalf("get task options: %v", err)
	}
	if options.AutoFixPromptOverride != nil {
		t.Fatalf("failed mixed patch persisted prompt override: %+v", options)
	}
}

// TestUpdateTaskMRAutomationOptions_RejectsUnlinkedOrPartialIdentity keeps a
// caller mistake from silently creating an orphan automation row, or from
// being reinterpreted as a fan-out over every MR.
func TestUpdateTaskMRAutomationOptions_RejectsUnlinkedOrPartialIdentity(t *testing.T) {
	svc, store := newMRAutomationServiceFixture(t, "alice")
	ctx := context.Background()
	if err := store.UpsertTaskMR(ctx, newTestMR("task-1", "", "group/a", 1)); err != nil {
		t.Fatalf("upsert MR: %v", err)
	}

	_, err := svc.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		RepositoryID: stringPtr(""), ProjectPath: stringPtr("group/nope"), MRIID: intPtr(42),
		AutoFixEnabled: boolPtr(true),
	})
	if !errors.Is(err, ErrTaskMRNotLinked) {
		t.Fatalf("expected ErrTaskMRNotLinked for an unlinked MR, got %v", err)
	}

	_, err = svc.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		ProjectPath: stringPtr("group/a"), AutoFixEnabled: boolPtr(true),
	})
	if !errors.Is(err, ErrTaskMRIdentityIncomplete) {
		t.Fatalf("expected ErrTaskMRIdentityIncomplete for a partial identity, got %v", err)
	}

	stored, err := store.ListTaskMRAutomationOptions(ctx, "task-1")
	if err != nil {
		t.Fatalf("ListTaskMRAutomationOptions: %v", err)
	}
	if len(stored) != 0 {
		t.Fatalf("a rejected patch still wrote rows: %+v", stored)
	}
}

// TestUpdateTaskMRAutomationOptions_KeepsReviewerWhileAnotherMRNeedsIt covers
// the one field the switches share: disabling review-request on one MR must
// not blank the task-level reviewer another MR's automation still depends on.
func TestUpdateTaskMRAutomationOptions_KeepsReviewerWhileAnotherMRNeedsIt(t *testing.T) {
	svc, store := newMRAutomationServiceFixture(t, "alice")
	ctx := context.Background()
	for _, mr := range []*TaskMR{
		newTestMR("task-1", "", "group/a", 1),
		newTestMR("task-1", "", "group/b", 2),
	} {
		if err := store.UpsertTaskMR(ctx, mr); err != nil {
			t.Fatalf("upsert MR %s: %v", mr.ProjectPath, err)
		}
	}
	if _, err := svc.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		PromptOnReviewRequested: boolPtr(true),
	}); err != nil {
		t.Fatalf("enable on both MRs: %v", err)
	}

	resp, err := svc.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		RepositoryID: stringPtr(""), ProjectPath: stringPtr("group/a"), MRIID: intPtr(1),
		PromptOnReviewRequested: boolPtr(false),
	})
	if err != nil {
		t.Fatalf("disable on one MR: %v", err)
	}
	if resp.ReviewReviewerUsername != "alice" {
		t.Fatalf("reviewer cleared while another MR still needs it: %+v", resp)
	}

	resp, err = svc.UpdateTaskMRAutomationOptions(ctx, "task-1", TaskMRAutomationPatch{
		RepositoryID: stringPtr(""), ProjectPath: stringPtr("group/b"), MRIID: intPtr(2),
		PromptOnReviewRequested: boolPtr(false),
	})
	if err != nil {
		t.Fatalf("disable on the last MR: %v", err)
	}
	if resp.ReviewReviewerUsername != "" {
		t.Fatalf("expected reviewer cleared once no MR needs it, got %+v", resp)
	}
}

func TestGetTaskMRAutomationEvaluation_UsesConfigUsernameAndExactCheckpoint(t *testing.T) {
	store := newTestStore(t)
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-1", "ws-1")
	if err := store.SaveConfigForWorkspace(context.Background(), "ws-1", &GitLabConfig{
		Host: "https://gitlab.example.com", AuthMethod: AuthMethodPAT, Username: "config-user",
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	staleUsername := "stale-task-option"
	if _, err := store.UpdateTaskMRAutomationOptions(context.Background(), "task-1", TaskMRAutomationPatch{
		PromptOnReviewRequested: boolPtr(true),
	}, &staleUsername); err != nil {
		t.Fatalf("enable review switch: %v", err)
	}
	if err := store.SetTaskMRObservedState(context.Background(), "task-1", "repo-1", "group/one", 1, "open"); err != nil {
		t.Fatalf("seed first checkpoint: %v", err)
	}
	if err := store.SetTaskMRObservedState(context.Background(), "task-1", "repo-2", "group/two", 2, "closed"); err != nil {
		t.Fatalf("seed target checkpoint: %v", err)
	}
	if err := store.SetTaskMRReviewRequestState(context.Background(), "task-1", "repo-2", "group/two", 2, true); err != nil {
		t.Fatalf("seed target review baseline: %v", err)
	}

	svc := newWorkspaceConfigService(t, store, &configTestSecrets{values: make(map[string]string)})
	evaluation, err := svc.GetTaskMRAutomationEvaluation(
		context.Background(), "task-1", "repo-2", "group/two", 2,
	)
	if err != nil {
		t.Fatalf("get evaluation snapshot: %v", err)
	}
	if evaluation == nil || evaluation.Options == nil || evaluation.Checkpoint == nil {
		t.Fatalf("incomplete evaluation snapshot: %+v", evaluation)
	}
	if got := evaluation.Options.ReviewReviewerUsername; got != "config-user" {
		t.Fatalf("reviewer username = %q, want persisted workspace config username", got)
	}
	if got := evaluation.Checkpoint.ProjectPath; got != "group/two" {
		t.Fatalf("checkpoint project = %q, want target checkpoint", got)
	}
	if got := evaluation.Checkpoint.LastObservedState; got != "closed" {
		t.Fatalf("checkpoint state = %q, want target checkpoint state", got)
	}
	persistedOptions, err := store.GetTaskMRAutomationOptions(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("read persisted automation options: %v", err)
	}
	if persistedOptions.ReviewReviewerUsername != "config-user" {
		t.Fatalf("persisted reviewer username = %q, want config-user", persistedOptions.ReviewReviewerUsername)
	}
	persistedCheckpoint, err := store.GetTaskMRLifecycleState(context.Background(), "task-1", "repo-2", "group/two", 2)
	if err != nil {
		t.Fatalf("read persisted target checkpoint: %v", err)
	}
	if persistedCheckpoint == nil || persistedCheckpoint.ReviewRequestInitialized || persistedCheckpoint.LastReviewRequested {
		t.Fatalf("changed-account evaluation must reset the persisted review baseline: %+v", persistedCheckpoint)
	}
	if evaluation.Options.MRStates != nil {
		t.Fatalf("evaluation loaded task-wide MR states: %+v", evaluation.Options.MRStates)
	}
}

// TestGetTaskMRAutomationEvaluation_UsesOnlyTheTargetMRsSwitches is the
// orchestrator-facing half of the per-MR scoping: an evaluation for one MR
// must not see a sibling MR's enabled automation.
func TestGetTaskMRAutomationEvaluation_UsesOnlyTheTargetMRsSwitches(t *testing.T) {
	svc, store := newMRAutomationServiceFixture(t, "alice")
	ctx := context.Background()
	for _, mr := range []*TaskMR{
		newTestMR("task-1", "", "group/a", 1),
		newTestMR("task-1", "", "group/b", 2),
	} {
		if err := store.UpsertTaskMR(ctx, mr); err != nil {
			t.Fatalf("upsert MR %s: %v", mr.ProjectPath, err)
		}
	}
	setMRSwitches(t, store, "task-1", mrIdentity("group/a", 1), TaskMRAutomationSwitchPatch{
		AutoFixEnabled: boolPtr(true), PromptOnMerged: boolPtr(true),
	})

	configured, err := svc.GetTaskMRAutomationEvaluation(ctx, "task-1", "", "group/a", 1)
	if err != nil {
		t.Fatalf("evaluate configured MR: %v", err)
	}
	if !configured.Options.AutoFixEnabled || !configured.Options.PromptOnMerged {
		t.Fatalf("configured MR lost its own switches: %+v", configured.Options)
	}

	sibling, err := svc.GetTaskMRAutomationEvaluation(ctx, "task-1", "", "group/b", 2)
	if err != nil {
		t.Fatalf("evaluate sibling MR: %v", err)
	}
	if sibling.Options.AutoFixEnabled || sibling.Options.PromptOnMerged ||
		sibling.Options.AutoMergeEnabled || sibling.Options.PromptOnReviewRequested ||
		sibling.Options.PromptOnClosed {
		t.Fatalf("sibling MR inherited another MR's automation: %+v", sibling.Options)
	}
}

// TestUpdateTaskMRAutomationOptions_RejectsSwitchesWithNoLinkedMRs covers the
// zero-target case. The switches live per-MR, so with nothing linked there is
// no row to write them to; returning success would report a write that stored
// nothing, and the caller would only find out when the automation it believed
// it had enabled never fired.
func TestUpdateTaskMRAutomationOptions_RejectsSwitchesWithNoLinkedMRs(t *testing.T) {
	svc, _ := newMRAutomationServiceFixture(t, "alice")

	_, err := svc.UpdateTaskMRAutomationOptions(context.Background(), "task-1", TaskMRAutomationPatch{
		PromptOnMerged: boolPtr(true),
	})
	if !errors.Is(err, ErrTaskMRNotLinked) {
		t.Fatalf("expected ErrTaskMRNotLinked for a task with no linked MRs, got %v", err)
	}
}

// TestUpdateTaskMRAutomationOptions_AllowsPromptOverrideWithNoLinkedMRs is the
// other half of the same rule: the auto-fix prompt override is task-level, so
// it stays settable before any MR is linked.
func TestUpdateTaskMRAutomationOptions_AllowsPromptOverrideWithNoLinkedMRs(t *testing.T) {
	svc, _ := newMRAutomationServiceFixture(t, "alice")

	resp, err := svc.UpdateTaskMRAutomationOptions(context.Background(), "task-1", TaskMRAutomationPatch{
		AutoFixPromptOverride: stringPtr("fix it please"),
	})
	if err != nil {
		t.Fatalf("prompt override with no linked MRs should succeed, got %v", err)
	}
	if resp.AutoFixPromptOverride == nil || *resp.AutoFixPromptOverride != "fix it please" {
		t.Errorf("override not persisted: %+v", resp.AutoFixPromptOverride)
	}
}
