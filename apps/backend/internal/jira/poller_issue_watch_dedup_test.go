package jira

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func TestPoller_CheckIssueWatches_RecoversDeletedTask(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "jira-poller.db")
	raw, err := dbutil.OpenSQLite(path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	taskRepo, err := tasksqlite.NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("task repository: %v", err)
	}
	store, err := NewStore(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("jira store: %v", err)
	}
	if err := taskRepo.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := taskRepo.CreateWorkflow(ctx, &taskmodels.Workflow{
		ID: "wf-1", WorkspaceID: "ws-1", Name: "Workflow",
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	now := time.Now().UTC()
	if _, err := sqlxDB.ExecContext(ctx, `INSERT INTO workflow_steps
		(id, workflow_id, name, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"step-1", "wf-1", "Step", 0, now, now); err != nil {
		t.Fatalf("create workflow step: %v", err)
	}
	if err := taskRepo.CreateTask(ctx, &taskmodels.Task{
		ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1",
		WorkflowStepID: "step-1", Title: "Existing task",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	secrets := newFakeSecretStore()
	client := &fakeClient{}
	svc := NewService(store, secrets, func(_ *JiraConfig, _ string) Client {
		return client
	}, logger.Default())
	eb := bus.NewMemoryEventBus(logger.Default())
	t.Cleanup(eb.Close)
	svc.SetEventBus(eb)
	poller := NewPoller(svc, logger.Default())

	f := &pollerFixture{store: store, secrets: secrets, client: client, svc: svc, poller: poller}
	f.saveConfigForWorkspace(t, "ws-1", "tok")
	sub, err := newRecordingSubscriber(eb, 1)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	client.searchFn = func(_ string) (*SearchResult, error) {
		return &SearchResult{
			Tickets: []JiraTicket{{Key: "PROJ-1", Summary: "mapped", URL: "https://x/browse/PROJ-1"}},
			IsLast:  true,
		}, nil
	}

	w := newTestIssueWatch("ws-1")
	if err := store.CreateIssueWatch(ctx, w); err != nil {
		t.Fatalf("create watch: %v", err)
	}
	seedDedupMapping(t, store, w.ID, "PROJ-1", "task-1", nil)
	poller.checkIssueWatches(ctx)
	if got := len(sub.snapshot()); got != 0 {
		t.Fatalf("mapped task published %d issues, want 0", got)
	}
	stamped, err := store.GetIssueWatch(ctx, w.ID)
	if err != nil {
		t.Fatalf("get watch: %v", err)
	}
	if stamped.LastPolledAt == nil {
		t.Fatal("expected last_polled_at after a poll")
	}
	if err := store.UpdateIssueWatchLastPolled(ctx, w.ID, time.Now().UTC().Add(-2*time.Hour)); err != nil {
		t.Fatalf("make watch due: %v", err)
	}

	if err := taskRepo.DeleteTask(ctx, "task-1"); err != nil {
		t.Fatalf("delete task natively: %v", err)
	}
	poller.checkIssueWatches(ctx)
	select {
	case <-sub.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for recovered issue publication")
	}
	events := sub.snapshot()
	if len(events) != 1 || events[0].Issue.Key != "PROJ-1" || events[0].IssueWatchID != w.ID {
		t.Fatalf("recovered events = %+v", events)
	}
	if events[0].Issue.URL != "https://x/browse/PROJ-1" {
		t.Fatalf("recovered issue URL = %q", events[0].Issue.URL)
	}
}
