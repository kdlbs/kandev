package github

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

func TestStoreWorkspaceConnectionRoundTrip(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "ws-1")
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	installationID := int64(12345)
	connection := &WorkspaceConnection{
		WorkspaceID:              "ws-1",
		Source:                   ConnectionSourceGitHubAppInstallation,
		GitHubHost:               "github.com",
		InstallationID:           &installationID,
		InstallationAccountLogin: "acme",
		InstallationAccountType:  "Organization",
		Status:                   ConnectionStatusActive,
		CredentialGeneration:     3,
		CreatedAt:                now,
		UpdatedAt:                now,
	}

	if err := store.UpsertWorkspaceConnection(ctx, connection); err != nil {
		t.Fatalf("upsert workspace connection: %v", err)
	}
	got, err := store.GetWorkspaceConnection(ctx, "ws-1")
	if err != nil {
		t.Fatalf("get workspace connection: %v", err)
	}
	if got == nil || got.Source != connection.Source || got.InstallationID == nil || *got.InstallationID != installationID {
		t.Fatalf("workspace connection = %+v, want source %q installation %d", got, connection.Source, installationID)
	}

	connection.Source = ConnectionSourcePAT
	connection.InstallationID = nil
	connection.InstallationAccountLogin = ""
	connection.InstallationAccountType = ""
	connection.Login = "octocat"
	connection.CredentialGeneration = 4
	if err := store.UpsertWorkspaceConnection(ctx, connection); err != nil {
		t.Fatalf("replace workspace connection: %v", err)
	}
	got, err = store.GetWorkspaceConnection(ctx, "ws-1")
	if err != nil {
		t.Fatalf("get replaced workspace connection: %v", err)
	}
	if got == nil || got.Source != ConnectionSourcePAT || got.Login != "octocat" || got.CredentialGeneration != 4 {
		t.Fatalf("replaced workspace connection = %+v", got)
	}
}

func TestStoreInstallationTransitionRejectsReplacedWorkspaceConnection(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "ws-1")
	ctx := context.Background()
	installationID := int64(12345)
	expected := &WorkspaceConnection{
		WorkspaceID: "ws-1", Source: ConnectionSourceGitHubAppInstallation,
		GitHubHost: "github.com", InstallationID: &installationID,
		InstallationAccountLogin: "acme", InstallationAccountType: "Organization",
		Status: ConnectionStatusActive, CredentialGeneration: 2,
	}
	if err := store.UpsertWorkspaceConnection(ctx, expected); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertWorkspaceConnection(ctx, &WorkspaceConnection{
		WorkspaceID: "ws-1", Source: ConnectionSourcePAT, GitHubHost: "github.com",
		Login: "octocat", Status: ConnectionStatusActive, CredentialGeneration: 3,
	}); err != nil {
		t.Fatal(err)
	}
	next := *expected
	next.Status = ConnectionStatusSuspended
	next.CredentialGeneration++
	updated, err := store.TransitionWorkspaceInstallationConnection(ctx, expected, &next)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := store.GetWorkspaceConnection(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if updated || connection == nil || connection.Source != ConnectionSourcePAT ||
		connection.Login != "octocat" || connection.CredentialGeneration != 3 {
		t.Fatalf("updated = %v, connection = %+v", updated, connection)
	}
}

func TestStoreUserConnectionRoundTrip(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "ws-1")
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	refreshExpiry := now.Add(30 * 24 * time.Hour)
	connection := &UserConnection{
		WorkspaceID:          "ws-1",
		UserID:               "default-user",
		GitHubUserID:         42,
		Login:                "octocat",
		Status:               ConnectionStatusActive,
		AccessExpiresAt:      now.Add(time.Hour),
		RefreshExpiresAt:     &refreshExpiry,
		CredentialGeneration: 2,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	if err := store.UpsertUserConnection(ctx, connection); err != nil {
		t.Fatalf("upsert user connection: %v", err)
	}
	got, err := store.GetUserConnection(ctx, "ws-1", "default-user")
	if err != nil {
		t.Fatalf("get user connection: %v", err)
	}
	if got == nil || got.GitHubUserID != 42 || got.Login != "octocat" || got.RefreshExpiresAt == nil {
		t.Fatalf("user connection = %+v", got)
	}

	if err := store.DeleteUserConnection(ctx, "ws-1", "default-user"); err != nil {
		t.Fatalf("delete user connection: %v", err)
	}
	got, err = store.GetUserConnection(ctx, "ws-1", "default-user")
	if err != nil || got != nil {
		t.Fatalf("user connection after delete = %+v, err %v", got, err)
	}
}

func TestStoreListsConnectionsByGitHubPrincipal(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "ws-1", "ws-2", "ws-other")
	ctx := context.Background()
	now := time.Now().UTC()
	installationID := int64(12345)
	otherInstallationID := int64(99999)
	for workspaceID, appInstallationID := range map[string]*int64{
		"ws-1": &installationID, "ws-2": &installationID, "ws-other": &otherInstallationID,
	} {
		if err := store.UpsertWorkspaceConnection(ctx, &WorkspaceConnection{
			WorkspaceID: workspaceID, Source: ConnectionSourceGitHubAppInstallation,
			GitHubHost: "github.com", InstallationID: appInstallationID,
			InstallationAccountLogin: "acme", InstallationAccountType: "Organization",
			Status: ConnectionStatusActive, CreatedAt: now,
		}); err != nil {
			t.Fatalf("seed workspace connection %s: %v", workspaceID, err)
		}
	}
	for workspaceID, githubUserID := range map[string]int64{"ws-1": 42, "ws-2": 42, "ws-other": 99} {
		if err := store.UpsertUserConnection(ctx, &UserConnection{
			WorkspaceID: workspaceID, UserID: "default-user", GitHubUserID: githubUserID,
			Login: "octocat", Status: ConnectionStatusActive,
			AccessExpiresAt: now.Add(time.Hour), CreatedAt: now,
		}); err != nil {
			t.Fatalf("seed user connection %s: %v", workspaceID, err)
		}
	}

	workspaces, err := store.ListWorkspaceConnectionsByInstallation(ctx, installationID)
	if err != nil {
		t.Fatalf("list workspaces by installation: %v", err)
	}
	if len(workspaces) != 2 || workspaces[0].WorkspaceID != "ws-1" || workspaces[1].WorkspaceID != "ws-2" {
		t.Fatalf("workspaces by installation = %+v", workspaces)
	}
	users, err := store.ListUserConnectionsByGitHubUser(ctx, 42)
	if err != nil {
		t.Fatalf("list connections by GitHub user: %v", err)
	}
	if len(users) != 2 || users[0].WorkspaceID != "ws-1" || users[1].WorkspaceID != "ws-2" {
		t.Fatalf("users by GitHub principal = %+v", users)
	}
}

func TestStoreConnectionConstraints(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "ws-invalid-source", "ws-invalid-app", "ws-invalid-status")
	ctx := context.Background()
	now := time.Now().UTC()

	for _, connection := range []*WorkspaceConnection{
		{WorkspaceID: "ws-invalid-source", Source: "oauth", GitHubHost: "github.com", Status: ConnectionStatusActive, CreatedAt: now, UpdatedAt: now},
		{WorkspaceID: "ws-invalid-app", Source: ConnectionSourceGitHubAppInstallation, GitHubHost: "github.com", Status: ConnectionStatusActive, CreatedAt: now, UpdatedAt: now},
		{WorkspaceID: "ws-invalid-status", Source: ConnectionSourcePAT, GitHubHost: "github.com", Status: "connected", CreatedAt: now, UpdatedAt: now},
	} {
		if err := store.UpsertWorkspaceConnection(ctx, connection); err == nil {
			t.Fatalf("expected invalid connection to fail: %+v", connection)
		}
	}
}

func TestStoreLegacyMigrationSeedsExistingWorkspacesAndBackfillsOwnership(t *testing.T) {
	db := openLegacyGitHubDB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO workspaces (id) VALUES ('ws-1'), ('ws-2');
		INSERT INTO tasks (id, workspace_id) VALUES ('task-1', 'ws-1'), ('task-2', 'ws-2');
		INSERT INTO github_pr_watches (
			id, session_id, task_id, repository_id, owner, repo, pr_number, branch, created_at, updated_at
		) VALUES
			('watch-owned', 'session-1', 'task-1', '', 'acme', 'repo', 1, 'main', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
			('watch-orphan', 'session-x', 'missing-task', '', 'acme', 'repo', 2, 'other', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
		INSERT INTO github_task_prs (
			id, task_id, repository_id, owner, repo, pr_number, pr_url, pr_title, head_branch,
			base_branch, author_login, created_at, updated_at
		) VALUES
			('pr-owned', 'task-2', '', 'acme', 'repo', 3, 'url', 'title', 'head', 'main', 'octocat', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
			('pr-orphan', 'missing-task', '', 'acme', 'repo', 4, 'url', 'title', 'head', 'main', 'octocat', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
		INSERT INTO github_review_watches (
			id, workspace_id, workflow_id, workflow_step_id, repos, agent_profile_id,
			executor_profile_id, created_at, updated_at
		) VALUES ('review-watch-1', 'ws-1', 'workflow-1', 'step-1', '[]', 'agent-1', 'executor-1', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("seed legacy rows: %v", err)
	}

	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("migrate legacy store: %v", err)
	}
	for _, workspaceID := range []string{"ws-1", "ws-2"} {
		got, getErr := store.GetWorkspaceConnection(ctx, workspaceID)
		if getErr != nil {
			t.Fatalf("get legacy connection %s: %v", workspaceID, getErr)
		}
		if got == nil || got.Source != ConnectionSourceLegacyShared || got.Status != ConnectionStatusActive {
			t.Fatalf("legacy connection %s = %+v", workspaceID, got)
		}
	}

	assertStoredWorkspaceID(t, db, "github_pr_watches", "watch-owned", "ws-1")
	assertStoredWorkspaceID(t, db, "github_task_prs", "pr-owned", "ws-2")
	assertStoredWorkspaceID(t, db, "github_pr_watches", "watch-orphan", "")
	assertStoredWorkspaceID(t, db, "github_task_prs", "pr-orphan", "")
	var targetLogin string
	if err := db.GetContext(ctx, &targetLogin,
		`SELECT target_login FROM github_review_watches WHERE id = 'review-watch-1'`); err != nil {
		t.Fatalf("read migrated review target login: %v", err)
	}
	if targetLogin != "" {
		t.Fatalf("migrated review target login = %q, want empty", targetLogin)
	}
	for _, indexName := range []string{
		"idx_github_task_prs_workspace", "idx_github_pr_watches_workspace",
	} {
		var indexCount int
		if err := db.GetContext(ctx, &indexCount,
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, indexName); err != nil {
			t.Fatalf("read ownership index %s: %v", indexName, err)
		}
		if indexCount != 1 {
			t.Fatalf("ownership index %s count = %d, want 1", indexName, indexCount)
		}
	}

	if _, err := NewStore(db, db); err != nil {
		t.Fatalf("replay migrated schema: %v", err)
	}
	var count int
	if err := db.GetContext(ctx, &count, `SELECT COUNT(*) FROM github_workspace_connections`); err != nil {
		t.Fatalf("count replayed connections: %v", err)
	}
	if count != 2 {
		t.Fatalf("connection count after replay = %d, want 2", count)
	}
}

func TestStoreFreshSchemaDoesNotCreateLegacyConnection(t *testing.T) {
	store := newTestStore(t)
	got, err := store.GetWorkspaceConnection(context.Background(), "ws-new")
	if err != nil {
		t.Fatalf("get fresh connection: %v", err)
	}
	if got != nil {
		t.Fatalf("fresh workspace inherited connection: %+v", got)
	}
}

func TestStorePersistsWorkspaceOwnershipOnNewPRRecords(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	watch := &PRWatch{
		WorkspaceID: "ws-1", SessionID: "session-1", TaskID: "task-1",
		Owner: "acme", Repo: "repo", Branch: "feature",
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}
	gotWatch, err := store.GetPRWatchBySession(ctx, "session-1")
	if err != nil || gotWatch == nil || gotWatch.WorkspaceID != "ws-1" {
		t.Fatalf("stored PR watch = %+v, err %v", gotWatch, err)
	}
	pr := &TaskPR{
		WorkspaceID: "ws-1", TaskID: "task-1", Owner: "acme", Repo: "repo", PRNumber: 7,
		PRURL: "url", PRTitle: "title", HeadBranch: "feature", BaseBranch: "main",
		CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateTaskPR(ctx, pr); err != nil {
		t.Fatalf("create task PR: %v", err)
	}
	gotPR, err := store.GetTaskPR(ctx, "task-1")
	if err != nil || gotPR == nil || gotPR.WorkspaceID != "ws-1" {
		t.Fatalf("stored task PR = %+v, err %v", gotPR, err)
	}
}

func TestStoreDeleteWorkspaceAuthData(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "ws-1")
	ctx := context.Background()
	now := time.Now().UTC()
	if err := store.UpsertWorkspaceConnection(ctx, &WorkspaceConnection{
		WorkspaceID: "ws-1", Source: ConnectionSourcePAT, GitHubHost: "github.com", Login: "octocat",
		Status: ConnectionStatusActive, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed workspace connection: %v", err)
	}
	if err := store.UpsertUserConnection(ctx, &UserConnection{
		WorkspaceID: "ws-1", UserID: "user-1", GitHubUserID: 1, Login: "octocat",
		Status: ConnectionStatusActive, AccessExpiresAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed user connection: %v", err)
	}

	if err := store.DeleteWorkspaceAuthData(ctx, "ws-1"); err != nil {
		t.Fatalf("delete workspace auth data: %v", err)
	}
	workspace, err := store.GetWorkspaceConnection(ctx, "ws-1")
	if err != nil || workspace != nil {
		t.Fatalf("workspace connection after delete = %+v, err %v", workspace, err)
	}
	user, err := store.GetUserConnection(ctx, "ws-1", "user-1")
	if err != nil || user != nil {
		t.Fatalf("user connection after delete = %+v, err %v", user, err)
	}
}

func TestGitHubSecretKeysAreWorkspaceAndUserScoped(t *testing.T) {
	if got, want := WorkspacePATSecretKey("ws-1"), "github:workspace:ws-1:pat"; got != want {
		t.Fatalf("workspace PAT key = %q, want %q", got, want)
	}
	if got, want := UserAccessTokenSecretKey("ws-1", "user-1"), "github:user:ws-1:user-1:access"; got != want {
		t.Fatalf("user access key = %q, want %q", got, want)
	}
	if got, want := UserRefreshTokenSecretKey("ws-1", "user-1"), "github:user:ws-1:user-1:refresh"; got != want {
		t.Fatalf("user refresh key = %q, want %q", got, want)
	}
}

func TestStoreAuthFlowIsSingleUse(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "ws-1")
	ctx := context.Background()
	now := time.Now().UTC()
	if err := store.CreateAuthFlow(ctx, &AuthFlow{
		StateHash: "state-hash", WorkspaceID: "ws-1", UserID: "user-1",
		Kind: AuthFlowKindPersonal, PKCEVerifier: "verifier", ExpiresAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("create auth flow: %v", err)
	}
	flow, err := store.ConsumeAuthFlow(ctx, "state-hash", now)
	if err != nil || flow == nil || flow.ConsumedAt == nil {
		t.Fatalf("consume auth flow = %+v, err %v", flow, err)
	}
	if _, err := store.ConsumeAuthFlow(ctx, "state-hash", now); !errors.Is(err, ErrAuthFlowUnavailable) {
		t.Fatalf("reconsume auth flow error = %v, want %v", err, ErrAuthFlowUnavailable)
	}
}

func TestStoreMigratesAndPersistsAuthFlowExpectations(t *testing.T) {
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "auth-flow-migration.db"))
	if err != nil {
		t.Fatal(err)
	}
	database := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Exec(`
		CREATE TABLE workspaces (id TEXT PRIMARY KEY);
		CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT);
		CREATE TABLE github_auth_flows (
			state_hash TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, user_id TEXT NOT NULL,
			kind TEXT NOT NULL, pkce_verifier TEXT NOT NULL DEFAULT '', expires_at TIMESTAMP NOT NULL,
			consumed_at TIMESTAMP, created_at TIMESTAMP NOT NULL
		);
		INSERT INTO workspaces (id) VALUES ('ws-1');
	`); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(database, database)
	if err != nil {
		t.Fatal(err)
	}
	installationID := int64(42)
	flow := &AuthFlow{
		StateHash: "state", WorkspaceID: "ws-1", UserID: "user-1", Kind: AuthFlowKindPersonal,
		ExpectedWorkspaceSource:     ConnectionSourceGitHubAppInstallation,
		ExpectedWorkspaceGeneration: 3, ExpectedInstallationID: &installationID,
		ExpectedPersonalGeneration: 7, ExpiresAt: time.Now().UTC().Add(time.Minute),
	}
	if err := store.CreateAuthFlow(context.Background(), flow); err != nil {
		t.Fatal(err)
	}
	stored, err := store.GetAuthFlow(context.Background(), flow.StateHash)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ExpectedWorkspaceSource != ConnectionSourceGitHubAppInstallation ||
		stored.ExpectedWorkspaceGeneration != 3 || stored.ExpectedInstallationID == nil ||
		*stored.ExpectedInstallationID != 42 || stored.ExpectedPersonalGeneration != 7 {
		t.Fatalf("stored flow = %+v", stored)
	}
}

func TestStoreWebhookDeliveryDeduplicates(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	delivery := &WebhookDelivery{DeliveryID: "delivery-1", Event: "installation"}
	inserted, err := store.RecordWebhookDelivery(ctx, delivery)
	if err != nil || !inserted {
		t.Fatalf("record first delivery: inserted %v, err %v", inserted, err)
	}
	inserted, err = store.RecordWebhookDelivery(ctx, delivery)
	if err != nil || inserted {
		t.Fatalf("record duplicate delivery: inserted %v, err %v", inserted, err)
	}
}

func TestStoreWebhookDeliveryClaimRetriesOnlyFailedOrStale(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	delivery := &WebhookDelivery{
		DeliveryID: "delivery-claim", Event: "installation", Status: WebhookDeliveryStatusReceived,
		ReceivedAt: now,
	}
	claim, err := store.ClaimWebhookDelivery(ctx, delivery, now.Add(-time.Minute))
	if err != nil || !claim.Acquired {
		t.Fatalf("first claim = %+v, err %v", claim, err)
	}
	claim, err = store.ClaimWebhookDelivery(ctx, delivery, now.Add(-time.Minute))
	if err != nil || claim.Acquired || claim.Status != WebhookDeliveryStatusReceived {
		t.Fatalf("fresh duplicate claim = %+v, err %v", claim, err)
	}
	if err := store.CompleteWebhookDelivery(
		ctx, delivery.DeliveryID, WebhookDeliveryStatusFailed, "temporary", now,
	); err != nil {
		t.Fatal(err)
	}
	claim, err = store.ClaimWebhookDelivery(ctx, delivery, now.Add(-time.Minute))
	if err != nil || !claim.Acquired {
		t.Fatalf("failed delivery retry claim = %+v, err %v", claim, err)
	}
	old := now.Add(-2 * time.Minute)
	if _, err := store.db.ExecContext(ctx, store.db.Rebind(`
		UPDATE github_webhook_deliveries SET received_at = ? WHERE delivery_id = ?`), old, delivery.DeliveryID); err != nil {
		t.Fatal(err)
	}
	claim, err = store.ClaimWebhookDelivery(ctx, delivery, now.Add(-time.Minute))
	if err != nil || !claim.Acquired {
		t.Fatalf("stale delivery retry claim = %+v, err %v", claim, err)
	}
}

func TestStoreReviewWatchTargetLoginRoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	watch := &ReviewWatch{
		WorkspaceID: "ws-1", WorkflowID: "workflow-1", WorkflowStepID: "step-1",
		AgentProfileID: "agent-1", ExecutorProfileID: "executor-1",
		ReviewScope: "user_and_teams", TargetLogin: "octocat", Enabled: true,
		PollIntervalSeconds: 300,
	}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("create review watch: %v", err)
	}
	got, err := store.GetReviewWatch(ctx, watch.ID)
	if err != nil || got == nil || got.TargetLogin != "octocat" {
		t.Fatalf("review watch after create = %+v, err %v", got, err)
	}
	got.TargetLogin = "hubot"
	if err := store.UpdateReviewWatch(ctx, got); err != nil {
		t.Fatalf("update review watch: %v", err)
	}
	got, err = store.GetReviewWatch(ctx, watch.ID)
	if err != nil || got == nil || got.TargetLogin != "hubot" {
		t.Fatalf("review watch after update = %+v, err %v", got, err)
	}
}

func seedConnectionWorkspaces(t *testing.T, store *Store, workspaceIDs ...string) {
	t.Helper()
	if _, err := store.db.Exec(`CREATE TABLE IF NOT EXISTS workspaces (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create workspaces table: %v", err)
	}
	for _, workspaceID := range workspaceIDs {
		if _, err := store.db.Exec(`INSERT INTO workspaces (id) VALUES (?)`, workspaceID); err != nil {
			t.Fatalf("seed workspace %s: %v", workspaceID, err)
		}
	}
}

func TestStoreWorkspaceConnectionHealthIncludesDisconnectedWorkspaces(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "ws-active", "ws-invalid", "ws-suspended", "ws-disconnected")
	ctx := context.Background()
	installationID := int64(42)
	for _, connection := range []*WorkspaceConnection{
		{WorkspaceID: "ws-active", Source: ConnectionSourcePAT, GitHubHost: defaultGitHubHost, Login: "active", Status: ConnectionStatusActive},
		{WorkspaceID: "ws-invalid", Source: ConnectionSourcePAT, GitHubHost: defaultGitHubHost, Login: "invalid", Status: ConnectionStatusInvalid},
		{WorkspaceID: "ws-suspended", Source: ConnectionSourceGitHubAppInstallation, GitHubHost: defaultGitHubHost,
			InstallationID: &installationID, InstallationAccountLogin: "acme", InstallationAccountType: "Organization",
			Status: ConnectionStatusSuspended},
	} {
		if err := store.UpsertWorkspaceConnection(ctx, connection); err != nil {
			t.Fatalf("UpsertWorkspaceConnection(%s): %v", connection.WorkspaceID, err)
		}
	}
	health, err := store.GetWorkspaceConnectionHealth(ctx)
	if err != nil {
		t.Fatalf("GetWorkspaceConnectionHealth() error = %v", err)
	}
	if health.WorkspaceCount != 4 || health.Active != 1 || health.Invalid != 1 ||
		health.Suspended != 1 || health.Disconnected != 1 || health.Revoked != 0 {
		t.Fatalf("GetWorkspaceConnectionHealth() = %+v", health)
	}
}

func openLegacyGitHubDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "legacy-github.db"))
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`
		CREATE TABLE workspaces (id TEXT PRIMARY KEY);
		CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL);
		CREATE TABLE github_workspace_settings (workspace_id TEXT PRIMARY KEY);
		CREATE TABLE github_review_watches (
			id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, workflow_id TEXT NOT NULL,
			workflow_step_id TEXT NOT NULL, repos TEXT NOT NULL DEFAULT '[]', agent_profile_id TEXT NOT NULL,
			executor_profile_id TEXT NOT NULL, prompt TEXT DEFAULT '', review_scope TEXT NOT NULL DEFAULT 'user_and_teams',
			custom_query TEXT NOT NULL DEFAULT '', enabled BOOLEAN DEFAULT 1, poll_interval_seconds INTEGER DEFAULT 300,
			cleanup_policy TEXT NOT NULL DEFAULT 'auto', last_polled_at DATETIME, last_error TEXT NOT NULL DEFAULT '',
			last_error_at DATETIME, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL
		);
		CREATE TABLE github_pr_watches (
			id TEXT PRIMARY KEY, session_id TEXT NOT NULL, task_id TEXT NOT NULL,
			repository_id TEXT NOT NULL DEFAULT '', owner TEXT NOT NULL, repo TEXT NOT NULL,
			pr_number INTEGER NOT NULL, branch TEXT NOT NULL, last_checked_at DATETIME,
			last_comment_at DATETIME, last_check_status TEXT DEFAULT '', last_review_state TEXT DEFAULT '',
			created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL,
			UNIQUE(session_id, repository_id, branch)
		);
		CREATE TABLE github_task_prs (
			id TEXT PRIMARY KEY, task_id TEXT NOT NULL, repository_id TEXT NOT NULL DEFAULT '',
			owner TEXT NOT NULL, repo TEXT NOT NULL, pr_number INTEGER NOT NULL, pr_url TEXT NOT NULL,
			pr_title TEXT NOT NULL, head_branch TEXT NOT NULL, base_branch TEXT NOT NULL,
			author_login TEXT NOT NULL, state TEXT NOT NULL DEFAULT 'open', review_state TEXT NOT NULL DEFAULT '',
			checks_state TEXT NOT NULL DEFAULT '', mergeable_state TEXT NOT NULL DEFAULT '', review_count INTEGER DEFAULT 0,
			pending_review_count INTEGER DEFAULT 0, required_reviews INTEGER, comment_count INTEGER DEFAULT 0,
			unresolved_review_threads INTEGER DEFAULT 0, checks_total INTEGER DEFAULT 0, checks_passing INTEGER DEFAULT 0,
			additions INTEGER DEFAULT 0, deletions INTEGER DEFAULT 0, created_at DATETIME NOT NULL,
			merged_at DATETIME, closed_at DATETIME, last_synced_at DATETIME, updated_at DATETIME NOT NULL,
			UNIQUE(task_id, repository_id, pr_number)
		)
	`)
	if err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	return db
}

func assertStoredWorkspaceID(t *testing.T, db *sqlx.DB, table, id, want string) {
	t.Helper()
	var got string
	err := db.QueryRow(`SELECT workspace_id FROM `+table+` WHERE id = ?`, id).Scan(&got)
	if err != nil {
		t.Fatalf("read %s workspace_id: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s %s workspace_id = %q, want %q", table, id, got, want)
	}
}
