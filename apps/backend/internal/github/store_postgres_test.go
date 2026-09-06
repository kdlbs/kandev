package github

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresStoreSchemaReplay(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := database.Exec(`
		CREATE TABLE workspaces (id TEXT PRIMARY KEY);
		CREATE TABLE tasks (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			archived_at TIMESTAMPTZ
		)`); err != nil {
		t.Fatalf("create prerequisite tables: %v", err)
	}
	if _, err := database.Exec(`
		INSERT INTO workspaces (id) VALUES ('ws-1');
		INSERT INTO tasks (id, workspace_id, archived_at) VALUES ('task-1', 'ws-1', NULL)`); err != nil {
		t.Fatalf("seed prerequisite rows: %v", err)
	}

	if _, err := NewStore(database, database); err != nil {
		t.Fatalf("first GitHub store schema init: %v", err)
	}
	store, err := NewStore(database, database)
	if err != nil {
		t.Fatalf("second GitHub store schema init: %v", err)
	}

	ctx := t.Context()
	now := time.Now().UTC()
	registration := newAppRegistration("registration-postgres", 1001, "PostgreSQL", now)
	if err := store.UpsertDeploymentAppRegistration(ctx, registration); err != nil {
		t.Fatalf("upsert app registration: %v", err)
	}
	gotRegistration, err := store.GetAppRegistration(ctx, registration.ID)
	if err != nil || gotRegistration == nil || gotRegistration.DisplayName != registration.DisplayName {
		t.Fatalf("get app registration = %+v, err %v", gotRegistration, err)
	}

	connection := &WorkspaceConnection{
		WorkspaceID: "ws-1", Source: ConnectionSourcePAT, GitHubHost: "github.com",
		Login: "octocat", Status: ConnectionStatusActive, CreatedAt: now,
	}
	if err := store.UpsertWorkspaceConnection(ctx, connection); err != nil {
		t.Fatalf("upsert workspace connection: %v", err)
	}
	gotConnection, err := store.GetWorkspaceConnection(ctx, "ws-1")
	if err != nil || gotConnection == nil || gotConnection.Login != connection.Login {
		t.Fatalf("get workspace connection = %+v, err %v", gotConnection, err)
	}

	installationID := int64(2001)
	appConnection := &WorkspaceConnection{
		WorkspaceID:              "ws-1",
		Source:                   ConnectionSourceGitHubAppInstallation,
		GitHubHost:               "github.com",
		InstallationID:           &installationID,
		InstallationAccountLogin: "acme",
		InstallationAccountType:  "Organization",
		AppRegistrationID:        registration.ID,
		Status:                   ConnectionStatusActive,
		CredentialGeneration:     2,
	}
	if err := store.UpsertWorkspaceConnection(ctx, appConnection); err != nil {
		t.Fatalf("upsert GitHub App workspace connection: %v", err)
	}
	userConnection := &UserConnection{
		WorkspaceID:          "ws-1",
		UserID:               "user-1",
		AppRegistrationID:    registration.ID,
		GitHubUserID:         2002,
		Login:                "octocat",
		Status:               ConnectionStatusActive,
		AccessExpiresAt:      now.Add(time.Hour),
		CredentialGeneration: 1,
		CreatedAt:            now,
	}
	if err := store.UpsertUserConnection(ctx, userConnection); err != nil {
		t.Fatalf("upsert personal GitHub connection: %v", err)
	}
	gotUserConnection, err := store.GetUserConnection(ctx, "ws-1", "user-1")
	if err != nil || gotUserConnection == nil || gotUserConnection.Login != userConnection.Login {
		t.Fatalf("get personal GitHub connection = %+v, err %v", gotUserConnection, err)
	}
	patConnection := &WorkspaceConnection{
		WorkspaceID:          "ws-1",
		Source:               ConnectionSourcePAT,
		GitHubHost:           "github.com",
		Login:                "octocat",
		Status:               ConnectionStatusActive,
		CredentialGeneration: 3,
	}
	if err := store.UpsertWorkspaceConnection(ctx, patConnection); err == nil {
		t.Fatal("workspace App registration mismatch was accepted with a personal connection")
	}

	watch := &PRWatch{
		WorkspaceID: "ws-1", SessionID: "session-1", TaskID: "task-1", RepositoryID: "repo-1",
		Owner: "acme", Repo: "widget", PRNumber: 42, Branch: "feature/postgres",
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}
	gotWatch, err := store.GetPRWatch(ctx, watch.ID)
	if err != nil || gotWatch == nil || gotWatch.PRNumber != watch.PRNumber {
		t.Fatalf("get PR watch = %+v, err %v", gotWatch, err)
	}

	taskPR := &TaskPR{
		WorkspaceID: "ws-1", TaskID: "task-1", RepositoryID: "repo-1", Owner: "acme", Repo: "widget",
		PRNumber: 42, PRURL: "https://github.com/acme/widget/pull/42", PRTitle: "PostgreSQL parity",
		HeadBranch: "feature/postgres", BaseBranch: "main", AuthorLogin: "octocat", State: "open",
		CreatedAt: now,
	}
	if err := store.CreateTaskPR(ctx, taskPR); err != nil {
		t.Fatalf("create task PR: %v", err)
	}
	gotTaskPR, err := store.GetTaskPRByID(ctx, taskPR.ID)
	if err != nil || gotTaskPR == nil || gotTaskPR.PRNumber != taskPR.PRNumber {
		t.Fatalf("get task PR = %+v, err %v", gotTaskPR, err)
	}

	if err := store.EnsureWorkspaceExecutorDefaults(ctx, "ws-1"); err != nil {
		t.Fatalf("ensure workspace executor defaults: %v", err)
	}
	settings, err := store.GetWorkspaceSettings(ctx, "ws-1")
	if err != nil || settings == nil || settings.TaskGitCredentialsMode != TaskGitCredentialsModeExecutor {
		t.Fatalf("get workspace settings = %+v, err %v", settings, err)
	}
	if err := store.UpsertWorkspaceSettings(ctx, &WorkspaceSettings{
		WorkspaceID:            "ws-1",
		TaskGitCredentialsMode: TaskGitCredentialsModeManaged,
		RepoScopeMode:          RepoScopeModeOrgs,
		RepoScopeOrgs:          []string{"acme"},
		SavedPresets:           []byte(`[{"name":"default"}]`),
	}); err != nil {
		t.Fatalf("upsert workspace settings: %v", err)
	}
	settings, err = store.GetWorkspaceSettings(ctx, "ws-1")
	if err != nil || settings == nil || settings.TaskGitCredentialsMode != TaskGitCredentialsModeManaged {
		t.Fatalf("get updated workspace settings = %+v, err %v", settings, err)
	}

	reviewWatch := &ReviewWatch{
		WorkspaceID:         "ws-1",
		WorkflowID:          "workflow-1",
		WorkflowStepID:      "step-1",
		AgentProfileID:      "agent-1",
		ExecutorProfileID:   "executor-1",
		Repos:               []RepoFilter{{Owner: "acme", Name: "widget"}},
		Enabled:             true,
		PollIntervalSeconds: 60,
	}
	if err := store.CreateReviewWatch(ctx, reviewWatch); err != nil {
		t.Fatalf("create review watch: %v", err)
	}
	gotReviewWatch, err := store.GetReviewWatch(ctx, reviewWatch.ID)
	if err != nil || gotReviewWatch == nil || len(gotReviewWatch.Repos) != 1 {
		t.Fatalf("get review watch = %+v, err %v", gotReviewWatch, err)
	}
	reserved, err := store.ReserveReviewPRTask(ctx, reviewWatch.ID, "acme", "widget", 42, "https://github.com/acme/widget/pull/42")
	if err != nil || !reserved {
		t.Fatalf("reserve review PR task = %v, err %v", reserved, err)
	}
	reserved, err = store.ReserveReviewPRTask(ctx, reviewWatch.ID, "acme", "widget", 42, "https://github.com/acme/widget/pull/42")
	if err != nil || reserved {
		t.Fatalf("duplicate review PR reservation = %v, err %v", reserved, err)
	}
	if err := store.AssignReviewPRTaskID(ctx, reviewWatch.ID, "acme", "widget", 42, "task-1"); err != nil {
		t.Fatalf("assign review PR task: %v", err)
	}

	issueWatch := &IssueWatch{
		WorkspaceID:         "ws-1",
		WorkflowID:          "workflow-1",
		WorkflowStepID:      "step-1",
		AgentProfileID:      "agent-1",
		ExecutorProfileID:   "executor-1",
		Labels:              []string{"bug"},
		Enabled:             true,
		PollIntervalSeconds: 60,
	}
	if err := store.CreateIssueWatch(ctx, issueWatch); err != nil {
		t.Fatalf("create issue watch: %v", err)
	}
	gotIssueWatch, err := store.GetIssueWatch(ctx, issueWatch.ID)
	if err != nil || gotIssueWatch == nil || len(gotIssueWatch.Labels) != 1 {
		t.Fatalf("get issue watch = %+v, err %v", gotIssueWatch, err)
	}
	reserved, err = store.ReserveIssueWatchTask(ctx, issueWatch.ID, "acme", "widget", 7, "https://github.com/acme/widget/issues/7")
	if err != nil || !reserved {
		t.Fatalf("reserve issue task = %v, err %v", reserved, err)
	}
	reserved, err = store.ReserveIssueWatchTask(ctx, issueWatch.ID, "acme", "widget", 7, "https://github.com/acme/widget/issues/7")
	if err != nil || reserved {
		t.Fatalf("duplicate issue reservation = %v, err %v", reserved, err)
	}
	if err := store.AssignIssueWatchTaskID(ctx, issueWatch.ID, "acme", "widget", 7, "task-1"); err != nil {
		t.Fatalf("assign issue task: %v", err)
	}

	autoFix, autoMerge := true, true
	options, err := store.UpdateTaskCIOptions(ctx, "task-1", TaskCIOptionsPatch{
		AutoFixEnabled:        &autoFix,
		AutoFixPromptOverride: stringPointer("fix it"),
	})
	if err != nil || options == nil || options.AutoFixPromptOverride == nil {
		t.Fatalf("update task CI options = %+v, err %v", options, err)
	}
	prOptions, err := store.UpdateTaskPRAutomationOptions(ctx, "task-1", "repo-1", 42,
		TaskPRAutomationOptionsPatch{AutoMergeEnabled: &autoMerge}, false)
	if err != nil || prOptions == nil || !prOptions.AutoMergeEnabled {
		t.Fatalf("update task PR automation options = %+v, err %v", prOptions, err)
	}

	flow := &AuthFlow{
		StateHash:                          "state-hash-postgres",
		WorkspaceID:                        "ws-1",
		UserID:                             "user-1",
		AppRegistrationID:                  registration.ID,
		Kind:                               AuthFlowKindPersonal,
		PKCEVerifier:                       "verifier",
		ExpectedWorkspaceSource:            ConnectionSourceGitHubAppInstallation,
		ExpectedWorkspaceGeneration:        2,
		ExpectedInstallationID:             &installationID,
		ExpectedWorkspaceAppRegistrationID: registration.ID,
		ExpectedPersonalGeneration:         1,
		ExpiresAt:                          now.Add(time.Hour),
	}
	if err := store.CreateAuthFlow(ctx, flow); err != nil {
		t.Fatalf("create auth flow: %v", err)
	}
	if consumed, err := store.ConsumeAuthFlow(ctx, flow.StateHash, registration.ID, flow.Kind, now); err != nil || consumed == nil || consumed.ConsumedAt == nil {
		t.Fatalf("consume auth flow = %+v, err %v", consumed, err)
	}

	stats, err := store.GetPRStats(ctx, &PRStatsRequest{WorkspaceID: "ws-1"})
	if err != nil || stats == nil || stats.TotalPRsCreated != 1 {
		t.Fatalf("get PR stats = %+v, err %v", stats, err)
	}
	if err := store.DeleteReviewWatch(ctx, reviewWatch.ID); err != nil {
		t.Fatalf("delete review watch: %v", err)
	}
	if err := store.DeleteIssueWatch(ctx, issueWatch.ID); err != nil {
		t.Fatalf("delete issue watch: %v", err)
	}
	if err := store.DeleteUserConnection(ctx, "ws-1", "user-1"); err != nil {
		t.Fatalf("delete personal GitHub connection: %v", err)
	}
	if err := store.UpsertUserConnection(ctx, userConnection); err != nil {
		t.Fatalf("re-upsert personal GitHub connection: %v", err)
	}
	if err := store.DeleteUserConnectionsByWorkspace(ctx, "ws-1"); err != nil {
		t.Fatalf("delete workspace GitHub connections: %v", err)
	}
}

func stringPointer(value string) *string {
	return &value
}
