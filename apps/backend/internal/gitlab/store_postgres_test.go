package gitlab

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresStoreSchemaReplay(t *testing.T) {
	ctx := context.Background()
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := database.Exec(`
		CREATE TABLE workspaces (id TEXT PRIMARY KEY);
		CREATE TABLE tasks (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			archived_at TIMESTAMPTZ
		);
		INSERT INTO workspaces (id) VALUES ('ws-1');
		INSERT INTO tasks (id, workspace_id, archived_at) VALUES ('task-1', 'ws-1', NULL)`); err != nil {
		t.Fatalf("create prerequisite tables: %v", err)
	}

	store, err := NewStore(database, database)
	if err != nil {
		t.Fatalf("first GitLab store schema init: %v", err)
	}
	if _, err := NewStore(database, database); err != nil {
		t.Fatalf("second GitLab store schema init: %v", err)
	}

	config := &GitLabConfig{
		Host:       "https://gitlab.example.com",
		AuthMethod: "pat",
		Username:   "alice",
	}
	if err := store.UpsertConfigForWorkspace(ctx, "ws-1", config); err != nil {
		t.Fatalf("upsert GitLab config: %v", err)
	}
	gotConfig, err := store.GetConfigForWorkspace(ctx, "ws-1")
	if err != nil || gotConfig == nil || gotConfig.Host != config.Host {
		t.Fatalf("get GitLab config: config=%+v err=%v", gotConfig, err)
	}
	if updated, err := store.UpdateConfigHealthForRevision(
		ctx, "ws-1", "alice", true, "", time.Now().UTC(), gotConfig.Revision,
	); err != nil || !updated {
		t.Fatalf("update GitLab config health: updated=%v err=%v", updated, err)
	}

	if err := store.UpsertMentionScope(ctx, &MentionScope{
		WorkspaceID: "ws-1",
		Host:        "https://gitlab.example.com",
		Projects:    []MentionProjectScope{{ID: 7, Path: "group/project"}},
	}); err != nil {
		t.Fatalf("upsert GitLab mention scope: %v", err)
	}
	gotScope, err := store.GetMentionScope(ctx, "ws-1")
	if err != nil || gotScope == nil || len(gotScope.Projects) != 1 {
		t.Fatalf("get GitLab mention scope: scope=%+v err=%v", gotScope, err)
	}

	mergeRequest := &TaskMR{
		TaskID:      "task-1",
		ProjectPath: "group/project",
		MRIID:       42,
		MRURL:       "https://gitlab.example.com/group/project/-/merge_requests/42",
		MRTitle:     "Parity",
		HeadBranch:  "feature/parity",
		BaseBranch:  "main",
		Draft:       true,
	}
	if err := store.UpsertTaskMR(ctx, mergeRequest); err != nil {
		t.Fatalf("upsert GitLab task MR: %v", err)
	}
	gotMR, err := store.GetTaskMR(ctx, "task-1", "", "group/project", 42)
	if err != nil || gotMR == nil || !gotMR.Draft {
		t.Fatalf("get GitLab task MR: mr=%+v err=%v", gotMR, err)
	}
	if err := store.UpdateTaskMRUnresolvedDiscussions(ctx, mergeRequest.ID, 3); err != nil {
		t.Fatalf("update GitLab task MR discussions: %v", err)
	}

	identity := MRIdentity{ProjectPath: "group/project", MRIID: 42}
	trueValue := true
	options, err := store.UpdateTaskMRAutomationOptionsForMR(ctx, "task-1", identity, TaskMRAutomationSwitchPatch{
		AutoMergeEnabled:        &trueValue,
		PromptOnReviewRequested: &trueValue,
	})
	if err != nil || options == nil || !options.AutoMergeEnabled || !options.PromptOnReviewRequested {
		t.Fatalf("update GitLab MR automation: options=%+v err=%v", options, err)
	}
	if err := store.SetTaskMRReviewRequestState(ctx, "task-1", "", "group/project", 42, true); err != nil {
		t.Fatalf("set GitLab MR review state: %v", err)
	}
	if err := store.SetTaskMRObservedState(ctx, "task-1", "", "group/project", 42, gitlabStateMerged); err != nil {
		t.Fatalf("set GitLab MR observed state: %v", err)
	}
	if err := store.RecordTaskMRLifecyclePrompt(ctx, TaskMRLifecyclePrompt{
		TaskID:          "task-1",
		ProjectPath:     "group/project",
		MRIID:           42,
		Event:           mrLifecycleEventReviewRequested,
		ReviewRequested: true,
	}); err != nil {
		t.Fatalf("record GitLab MR lifecycle prompt: %v", err)
	}
	if _, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/project", 42); err != nil {
		t.Fatalf("get GitLab MR lifecycle state: %v", err)
	}
	if subscribed, err := store.ListAutomationSubscribedTaskMRs(ctx); err != nil || len(subscribed) != 1 {
		t.Fatalf("list GitLab automation MRs: mrs=%d err=%v", len(subscribed), err)
	}

	refreshWatch := &MRWatch{
		SessionID:   "session-1",
		TaskID:      "task-1",
		ProjectPath: "group/project",
		MRIID:       42,
		Branch:      "feature/parity",
	}
	if err := store.CreateMRWatch(ctx, refreshWatch); err != nil {
		t.Fatalf("create GitLab MR watch: %v", err)
	}
	if got, err := store.GetMRWatchBySession(ctx, "session-1"); err != nil || got == nil {
		t.Fatalf("get GitLab MR watch: watch=%+v err=%v", got, err)
	}

	reviewWatch := &ReviewWatch{
		WorkspaceID:       "ws-1",
		WorkflowID:        "workflow-1",
		WorkflowStepID:    "step-1",
		Projects:          []ProjectFilter{{Path: "group/project"}},
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "executor-1",
		Enabled:           true,
	}
	if err := store.CreateReviewWatch(ctx, reviewWatch); err != nil {
		t.Fatalf("create GitLab review watch: %v", err)
	}
	if _, err := store.ListEnabledReviewWatchesForWorkspace(ctx, "ws-1"); err != nil {
		t.Fatalf("list GitLab review watches: %v", err)
	}
	reserved, err := store.ReserveReviewMRTask(ctx, reviewWatch.ID, "group/project", 43, "https://gitlab.example.com/mr/43")
	if err != nil || !reserved {
		t.Fatalf("reserve GitLab review MR task: reserved=%v err=%v", reserved, err)
	}
	if err := store.AssignReviewMRTaskID(ctx, reviewWatch.ID, "group/project", 43, "task-1"); err != nil {
		t.Fatalf("assign GitLab review MR task: %v", err)
	}

	issueWatch := &IssueWatch{
		WorkspaceID:       "ws-1",
		WorkflowID:        "workflow-1",
		WorkflowStepID:    "step-1",
		Projects:          []ProjectFilter{{Path: "group/project"}},
		Labels:            []string{"bug"},
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "executor-1",
		Enabled:           true,
	}
	if err := store.CreateIssueWatch(ctx, issueWatch); err != nil {
		t.Fatalf("create GitLab issue watch: %v", err)
	}
	if _, err := store.ListEnabledIssueWatchesForWorkspace(ctx, "ws-1"); err != nil {
		t.Fatalf("list GitLab issue watches: %v", err)
	}
	reserved, err = store.ReserveIssueWatchTask(ctx, issueWatch.ID, "group/project", 9, "https://gitlab.example.com/issue/9")
	if err != nil || !reserved {
		t.Fatalf("reserve GitLab issue task: reserved=%v err=%v", reserved, err)
	}
	if err := store.AssignIssueWatchTaskID(ctx, issueWatch.ID, "group/project", 9, "task-1"); err != nil {
		t.Fatalf("assign GitLab issue task: %v", err)
	}

	if err := store.UpsertActionPresets(ctx, &ActionPresets{
		WorkspaceID: "ws-1",
		MR:          DefaultMRActionPresets(),
		Issue:       DefaultIssueActionPresets(),
	}); err != nil {
		t.Fatalf("upsert GitLab action presets: %v", err)
	}
	if _, err := store.GetActionPresets(ctx, "ws-1"); err != nil {
		t.Fatalf("get GitLab action presets: %v", err)
	}
}

func TestPostgresRegistrationUnlinkFence(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := database.Exec(`
		CREATE TABLE workspaces (id TEXT PRIMARY KEY);
		CREATE TABLE tasks (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			archived_at TIMESTAMPTZ
		);
		CREATE TABLE repositories (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			provider_repo_id TEXT NOT NULL,
			provider_host TEXT NOT NULL,
			provider_owner TEXT NOT NULL,
			provider_name TEXT NOT NULL,
			remote_url TEXT NOT NULL,
			local_path TEXT NOT NULL
		);
		CREATE TABLE task_repositories (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			repository_id TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		);
		INSERT INTO workspaces (id) VALUES ('ws-registration-pg');
		INSERT INTO tasks (id, workspace_id) VALUES ('task-registration-pg', 'ws-registration-pg');
		INSERT INTO repositories (id, workspace_id, provider, provider_repo_id, provider_host, provider_owner, provider_name, remote_url, local_path)
		VALUES ('repo-registration-pg', 'ws-registration-pg', 'gitlab', 'provider-project-42', 'https://gitlab.example.test', 'group', 'project', 'https://gitlab.example.test/group/project.git', '/repo');
		INSERT INTO repositories (id, workspace_id, provider, provider_repo_id, provider_host, provider_owner, provider_name, remote_url, local_path)
		VALUES ('repo-registration-rebound-pg', 'ws-registration-pg', 'gitlab', 'provider-project-42', 'https://gitlab.example.test', 'renamed', 'project', 'https://gitlab.example.test/renamed/project.git', '/renamed');
		INSERT INTO task_repositories (id, task_id, repository_id, created_at, updated_at)
		VALUES ('task-repo-registration-pg', 'task-registration-pg', 'repo-registration-pg', NOW(), NOW()),
		('task-repo-registration-rebound-pg', 'task-registration-pg', 'repo-registration-rebound-pg', NOW(), NOW())`); err != nil {
		t.Fatalf("create registration fixtures: %v", err)
	}
	store, err := NewStore(database, database)
	if err != nil {
		t.Fatalf("create GitLab store: %v", err)
	}
	ctx := context.Background()
	registration := &TaskMR{
		TaskID: "task-registration-pg", RepositoryID: "repo-registration-pg",
		Host: "https://gitlab.example.test", ProjectPath: "group/project", MRIID: 43,
		MRURL: "https://gitlab.example.test/group/project/-/merge_requests/43", MRTitle: "registration fence",
		State: "opened", CreatedAt: time.Now().UTC(), Source: TaskMRSourceAgentRegistration,
		ProviderRepositoryID: "provider-project-42", ConnectionScope: "builtin:ws-registration-pg",
		RepositoryPath: "group/project", ComparisonPolicy: TaskMRComparisonPolicyPreserve,
	}
	created, changed, err := store.RegisterTaskMRObservation(ctx, registration, false, 0)
	if err != nil || !changed || created.AssociationRevision != 1 {
		t.Fatalf("create observation = (%+v, changed=%v, err=%v)", created, changed, err)
	}
	detached, changed, err := store.DetachTaskMRForWorkspace(ctx, "ws-registration-pg", created.ID)
	if err != nil || !changed || detached.AssociationRevision != 2 || detached.DetachedAt == nil {
		t.Fatalf("detach observation = (%+v, changed=%v, err=%v)", detached, changed, err)
	}
	if _, _, err := store.RegisterTaskMRObservation(ctx, registration, true, 1); err != ErrTaskMRRegistrationRevisionChanged {
		t.Fatalf("stale PostgreSQL restore error = %v, want revision changed", err)
	}
	start := make(chan struct{})
	results := make(chan struct {
		changed bool
		err     error
	}, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			candidate := *registration
			_, restored, restoreErr := store.RegisterTaskMRObservation(ctx, &candidate, true, 2)
			results <- struct {
				changed bool
				err     error
			}{restored, restoreErr}
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	changedCount := 0
	for result := range results {
		if result.err != nil && result.err != ErrTaskMRRegistrationRevisionChanged {
			t.Fatalf("concurrent restore failed unexpectedly: %v", result.err)
		}
		if result.changed {
			changedCount++
		}
	}
	stored, err := store.GetTaskMRIncludingDetached(ctx, created.TaskID, created.RepositoryID, created.ProjectPath, created.MRIID)
	if err != nil || changedCount != 1 || stored == nil || stored.AssociationRevision != 3 || stored.DetachedAt != nil {
		t.Fatalf("concurrent restore result: changed=%d stored=%+v err=%v", changedCount, stored, err)
	}

	canonical := *registration
	canonical.ID = ""
	canonical.MRIID = 44
	canonical.MRURL = "https://gitlab.example.test/group/project/-/merge_requests/44"
	firstCanonical, changed, err := store.RegisterTaskMRObservation(ctx, &canonical, false, 0)
	if err != nil || !changed {
		t.Fatalf("create canonical identity: row=%+v changed=%v err=%v", firstCanonical, changed, err)
	}
	rebound := canonical
	rebound.RepositoryID = "repo-registration-rebound-pg"
	rebound.RepositoryPath = "renamed/project"
	rebound.ProjectPath = "renamed/project"
	rebound.MRURL = "https://gitlab.example.test/renamed/project/-/merge_requests/44"
	secondCanonical, changed, err := store.RegisterTaskMRObservation(ctx, &rebound, false, firstCanonical.AssociationRevision)
	if err != nil || changed || secondCanonical.ID != firstCanonical.ID || secondCanonical.RepositoryID != firstCanonical.RepositoryID {
		t.Fatalf("canonical identity changed after repository rename: first=%+v second=%+v changed=%v err=%v", firstCanonical, secondCanonical, changed, err)
	}
	if _, _, err := store.DetachTaskMRForWorkspace(ctx, "ws-registration-pg", firstCanonical.ID); err != nil {
		t.Fatalf("detach canonical identity: %v", err)
	}
	if _, err := database.Exec(`UPDATE repositories SET provider_repo_id = 'provider-project-replacement' WHERE id = 'repo-registration-rebound-pg'`); err != nil {
		t.Fatalf("reuse path with different immutable identity: %v", err)
	}
	reusedPath := rebound
	reusedPath.ID = ""
	reusedPath.ProviderRepositoryID = "provider-project-replacement"
	replacement, changed, err := store.RegisterTaskMRObservation(ctx, &reusedPath, false, 0)
	if err != nil || !changed || replacement.ID == firstCanonical.ID || replacement.DetachedAt != nil {
		t.Fatalf("path reuse inherited detached identity: replacement=%+v changed=%v err=%v", replacement, changed, err)
	}

	legacy := &TaskMR{
		TaskID: registration.TaskID, RepositoryID: registration.RepositoryID, Host: registration.Host,
		ProjectPath: registration.ProjectPath, MRIID: 45, MRURL: "https://gitlab.example.test/group/project/-/merge_requests/45",
		MRTitle: "legacy registration", State: "opened", CreatedAt: time.Now().UTC(),
	}
	if err := store.UpsertTaskMR(ctx, legacy); err != nil {
		t.Fatalf("seed unresolved legacy association: %v", err)
	}
	legacyCandidate := canonical
	legacyCandidate.ID = ""
	legacyCandidate.MRIID = legacy.MRIID
	legacyCandidate.MRURL = legacy.MRURL
	if _, _, err := store.RegisterTaskMRObservation(ctx, &legacyCandidate, true, 1); err != ErrTaskMRRegistrationIdentityUnresolved {
		t.Fatalf("legacy identity repair error = %v, want unresolved", err)
	}
}
