package jira

import (
	"context"
	"testing"
)

func configureIssueWatchDedupFixture(t *testing.T, f *svcFixture) {
	t.Helper()
	if _, err := f.svc.SetConfigForWorkspace(context.Background(), "ws-1", &SetConfigRequest{
		SiteURL: "https://a.net", Email: "e",
		AuthMethod: AuthMethodAPIToken, InstanceType: InstanceTypeCloud, Secret: "tok",
	}); err != nil {
		t.Fatalf("set config: %v", err)
	}
}

func TestService_CheckIssueWatch_DeletedTaskEligible(t *testing.T) {
	f := newSvcFixture(t)
	configureIssueWatchDedupFixture(t, f)
	ctx := context.Background()
	f.client.withSearchResults([]JiraTicket{
		{Key: "PROJ-1", Summary: "deleted", URL: "https://a.net/browse/PROJ-1"},
		{Key: "PROJ-2", Summary: "unfinished", URL: "https://a.net/browse/PROJ-2"},
	})
	w, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step", JQL: "project = PROJ",
	})
	if err != nil {
		t.Fatalf("create watch: %v", err)
	}
	seedDedupMapping(t, f.store, w.ID, "PROJ-1", "task-live", func() {
		seedDedupTask(t, f.store, "task-live", false, false)
	})
	seedDedupMapping(t, f.store, w.ID, "PROJ-2", "", nil)

	got, err := f.svc.CheckIssueWatch(ctx, w)
	if err != nil {
		t.Fatalf("first check: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("first check returned %d issues, want 0", len(got))
	}

	if _, err := f.store.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, "task-live"); err != nil {
		t.Fatalf("delete task: %v", err)
	}
	got, err = f.svc.CheckIssueWatch(ctx, w)
	if err != nil {
		t.Fatalf("second check: %v", err)
	}
	if len(got) != 1 || got[0].Key != "PROJ-1" {
		t.Fatalf("second check = %+v, want only PROJ-1", got)
	}
}

func TestService_CheckIssueWatch_TaskLookupFailure(t *testing.T) {
	f := newSvcFixture(t)
	configureIssueWatchDedupFixture(t, f)
	ctx := context.Background()
	f.client.withSearchResults([]JiraTicket{{Key: "PROJ-1", URL: "https://a.net/browse/PROJ-1"}})
	w, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step", JQL: "project = PROJ",
	})
	if err != nil {
		t.Fatalf("create watch: %v", err)
	}
	if _, err := f.store.db.ExecContext(ctx, `DROP TABLE tasks`); err != nil {
		t.Fatalf("drop tasks fixture: %v", err)
	}
	got, err := f.svc.CheckIssueWatch(ctx, w)
	if err == nil {
		t.Fatal("expected task lookup failure")
	}
	if len(got) != 0 {
		t.Fatalf("failed lookup delivered %d issues", len(got))
	}

	if _, err := f.store.db.ExecContext(ctx, `CREATE TABLE tasks (
		id TEXT PRIMARY KEY,
		archived_at DATETIME,
		workflow_step_id TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("recreate tasks fixture: %v", err)
	}
	got, err = f.svc.CheckIssueWatch(ctx, w)
	if err != nil {
		t.Fatalf("retry check: %v", err)
	}
	if len(got) != 1 || got[0].Key != "PROJ-1" {
		t.Fatalf("retry check = %+v, want PROJ-1", got)
	}
}
