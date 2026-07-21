package gitlab

import "testing"

func TestStoreResetWorkspaceE2EIsScopedAndClearsAutomationState(t *testing.T) {
	store := newTestStore(t)
	for _, workspaceID := range []string{"ws-a", "ws-b"} {
		seedWorkspace(t, store, workspaceID)
		seedTask(t, store, "task-"+workspaceID, workspaceID)
		if err := store.UpsertConfigForWorkspace(t.Context(), workspaceID, &GitLabConfig{
			Host: "https://" + workspaceID + ".example.com", AuthMethod: AuthMethodPAT,
		}); err != nil {
			t.Fatalf("seed config %s: %v", workspaceID, err)
		}
	}

	reviewA := &ReviewWatch{WorkspaceID: "ws-a", WorkflowID: "wf", WorkflowStepID: "step", AgentProfileID: "agent", ExecutorProfileID: "exec", Enabled: true}
	reviewB := &ReviewWatch{WorkspaceID: "ws-b", WorkflowID: "wf", WorkflowStepID: "step", AgentProfileID: "agent", ExecutorProfileID: "exec", Enabled: true}
	issueA := &IssueWatch{WorkspaceID: "ws-a", WorkflowID: "wf", WorkflowStepID: "step", AgentProfileID: "agent", ExecutorProfileID: "exec", Enabled: true}
	issueB := &IssueWatch{WorkspaceID: "ws-b", WorkflowID: "wf", WorkflowStepID: "step", AgentProfileID: "agent", ExecutorProfileID: "exec", Enabled: true}
	for _, watch := range []*ReviewWatch{reviewA, reviewB} {
		if err := store.CreateReviewWatch(t.Context(), watch); err != nil {
			t.Fatal(err)
		}
		if _, err := store.ReserveReviewMRTask(t.Context(), watch.ID, "group/project", 7, "url"); err != nil {
			t.Fatal(err)
		}
	}
	for _, watch := range []*IssueWatch{issueA, issueB} {
		if err := store.CreateIssueWatch(t.Context(), watch); err != nil {
			t.Fatal(err)
		}
		if _, err := store.ReserveIssueWatchTask(t.Context(), watch.ID, "group/project", 8, "url"); err != nil {
			t.Fatal(err)
		}
	}
	for _, workspaceID := range []string{"ws-a", "ws-b"} {
		if err := store.UpsertActionPresets(t.Context(), &ActionPresets{
			WorkspaceID: workspaceID,
			MR:          []ActionPreset{{ID: "preset", Label: "Preset"}},
		}); err != nil {
			t.Fatal(err)
		}
		if err := store.UpsertTaskMR(t.Context(), &TaskMR{
			TaskID: "task-" + workspaceID, ProjectPath: "group/project", MRIID: 7, MRURL: "url",
		}); err != nil {
			t.Fatal(err)
		}
		if err := store.CreateMRWatch(t.Context(), &MRWatch{
			SessionID: "session-" + workspaceID, TaskID: "task-" + workspaceID,
			ProjectPath: "group/project", MRIID: 7,
		}); err != nil {
			t.Fatal(err)
		}
	}

	result, err := store.ResetWorkspaceE2E(t.Context(), "ws-a")
	if err != nil {
		t.Fatalf("reset workspace: %v", err)
	}
	if result.ReviewWatches != 1 || result.IssueWatches != 1 {
		t.Fatalf("reset result = %+v", result)
	}
	assertWorkspaceGitLabRows(t, store, "ws-a", 0)
	assertWorkspaceGitLabRows(t, store, "ws-b", 1)
	if cfg, err := store.GetConfigForWorkspace(t.Context(), "ws-a"); err != nil || cfg == nil {
		t.Fatalf("store reset should leave config for credential-aware service cleanup: cfg=%#v err=%v", cfg, err)
	}
}

func TestServiceResetWorkspaceE2EAlsoClearsCredentialAndMockClient(t *testing.T) {
	store := newTestStore(t)
	seedWorkspace(t, store, "ws-a")
	if err := store.UpsertConfigForWorkspace(t.Context(), "ws-a", &GitLabConfig{
		Host: "https://gitlab-a.example.com", AuthMethod: AuthMethodPAT,
	}); err != nil {
		t.Fatal(err)
	}
	secrets := &configTestSecrets{values: map[string]string{SecretKeyForWorkspace("ws-a"): "token"}}
	mock := NewMockClient("https://gitlab-a.example.com")
	mock.SeedMR("group/project", &MR{IID: 7})
	service := NewService(DefaultHost, NewMockClient(DefaultHost), "mock", nil, newTestLogger(t))
	service.SetStore(store)
	service.SetWorkspaceSecretStore(secrets)
	service.workspaceClients["ws-a"] = mock

	if _, err := service.ResetWorkspaceE2E(t.Context(), "ws-a"); err != nil {
		t.Fatalf("service reset: %v", err)
	}
	if cfg, err := store.GetConfigForWorkspace(t.Context(), "ws-a"); err != nil || cfg != nil {
		t.Fatalf("config after reset = %#v, err=%v", cfg, err)
	}
	if _, ok := secrets.values[SecretKeyForWorkspace("ws-a")]; ok {
		t.Fatal("workspace token survived reset")
	}
	if got := mock.Stats(); got != "mrs=0 discussions=0 issues=0" {
		t.Fatalf("mock after reset = %q", got)
	}
}

func assertWorkspaceGitLabRows(t *testing.T, store *Store, workspaceID string, want int) {
	t.Helper()
	checks := []struct {
		query string
		args  []any
	}{
		{`SELECT COUNT(*) FROM gitlab_review_watches WHERE workspace_id = ?`, []any{workspaceID}},
		{`SELECT COUNT(*) FROM gitlab_issue_watches WHERE workspace_id = ?`, []any{workspaceID}},
		{`SELECT COUNT(*) FROM gitlab_action_presets WHERE workspace_id = ?`, []any{workspaceID}},
		{`SELECT COUNT(*) FROM gitlab_task_mrs WHERE task_id = ?`, []any{"task-" + workspaceID}},
		{`SELECT COUNT(*) FROM gitlab_mr_watches WHERE task_id = ?`, []any{"task-" + workspaceID}},
	}
	for _, check := range checks {
		var got int
		if err := store.ro.Get(&got, check.query, check.args...); err != nil {
			t.Fatalf("count %q: %v", check.query, err)
		}
		if got != want {
			t.Fatalf("count %q for %s = %d, want %d", check.query, workspaceID, got, want)
		}
	}
}
