package github

import (
	"testing"

	"github.com/kandev/kandev/internal/testutil"
)

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.2
// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.4
func TestEnterpriseHostMigrationPostgres(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := database.Exec(`CREATE TABLE workspaces(id TEXT PRIMARY KEY);
 CREATE TABLE tasks(id TEXT PRIMARY KEY,workspace_id TEXT NOT NULL,archived_at TIMESTAMPTZ);
 INSERT INTO workspaces VALUES('public'),('enterprise');
 INSERT INTO tasks(id,workspace_id) VALUES('public-task','public'),('enterprise-task','enterprise');`); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(database, database)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	for _, tc := range []struct{ workspace, host string }{{"public", "github.com"}, {"enterprise", "git.example.com"}} {
		taskID := tc.workspace + "-task"
		if err := store.CreateTaskPR(ctx, &TaskPR{GitHubHost: tc.host, TaskID: taskID, WorkspaceID: tc.workspace, RepositoryID: "same-id", Owner: "acme", Repo: "widget", PRNumber: 7, PRURL: "https://" + tc.host + "/acme/widget/pull/7"}); err != nil {
			t.Fatal(err)
		}
		watch := &PRWatch{GitHubHost: tc.host, TaskID: taskID, WorkspaceID: tc.workspace, RepositoryID: "same-id", Owner: "acme", Repo: "widget", PRNumber: 7, Branch: "main"}
		if err := store.CreatePRWatch(ctx, watch); err != nil {
			t.Fatal(err)
		}
		rows, err := store.ListTaskPRsByPRNumberForHost(ctx, tc.workspace, tc.host, "acme", "widget", 7)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0].GitHubHost != tc.host || rows[0].TaskID != taskID {
			t.Fatalf("host identity collision: %+v", rows)
		}
		wrong, err := store.ListTaskPRsByPRNumberForHost(ctx, tc.workspace, "wrong.example.com", "acme", "widget", 7)
		if err != nil {
			t.Fatal(err)
		}
		if len(wrong) != 0 {
			t.Fatal("foreign-host association returned")
		}
	}
	replay, err := NewStore(database, database)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := replay.GetTaskPRByRepository(ctx, "enterprise-task", "same-id")
	if err != nil {
		t.Fatal(err)
	}
	if persisted.GitHubHost != "git.example.com" {
		t.Fatalf("restart lost Enterprise host: %+v", persisted)
	}
	if _, err := database.Exec("ALTER TABLE github_task_prs DROP COLUMN github_host"); err != nil {
		t.Fatal(err)
	}
	upgraded, err := NewStore(database, database)
	if err != nil {
		t.Fatal(err)
	}
	migrated, err := upgraded.GetTaskPRByRepository(ctx, "enterprise-task", "same-id")
	if err != nil {
		t.Fatal(err)
	}
	if migrated.GitHubHost != "git.example.com" {
		t.Fatalf("PostgreSQL upgrade lost explicit origin: %+v", migrated)
	}
}
