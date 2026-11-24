package linear

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresStoreSchemaReplay(t *testing.T) {
	ctx := context.Background()
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))

	store, err := NewStore(database, database)
	if err != nil {
		t.Fatalf("first Linear store schema init: %v", err)
	}
	if _, err := NewStore(database, database); err != nil {
		t.Fatalf("second Linear store schema init: %v", err)
	}

	config := &LinearConfig{AuthMethod: AuthMethodAPIKey, DefaultTeamKey: "ENG"}
	if err := store.UpsertConfigForWorkspace(ctx, "ws-1", config); err != nil {
		t.Fatalf("upsert Linear config: %v", err)
	}
	gotConfig, err := store.GetConfigForWorkspace(ctx, "ws-1")
	if err != nil || gotConfig == nil || gotConfig.DefaultTeamKey != config.DefaultTeamKey {
		t.Fatalf("get Linear config: config=%+v err=%v", gotConfig, err)
	}
	if err := store.UpdateAuthHealthForWorkspace(ctx, "ws-1", true, "", "acme", time.Now().UTC()); err != nil {
		t.Fatalf("update Linear auth health: %v", err)
	}

	watch := &IssueWatch{
		WorkspaceID:       "ws-1",
		WorkflowID:        "workflow-1",
		WorkflowStepID:    "step-1",
		Filter:            SearchFilter{Query: "priority:high"},
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "executor-1",
		Enabled:           true,
		SortBy:            SortByPriorityDesc,
	}
	if err := store.CreateIssueWatch(ctx, watch); err != nil {
		t.Fatalf("create Linear issue watch: %v", err)
	}
	gotWatch, err := store.GetIssueWatch(ctx, watch.ID)
	if err != nil || gotWatch == nil || gotWatch.Filter.Query != watch.Filter.Query {
		t.Fatalf("get Linear issue watch: watch=%+v err=%v", gotWatch, err)
	}
	if enabled, err := store.ListEnabledIssueWatches(ctx); err != nil || len(enabled) != 1 {
		t.Fatalf("list enabled Linear issue watches: watches=%d err=%v", len(enabled), err)
	}

	reserved, err := store.ReserveIssueWatchTask(ctx, watch.ID, "ENG-123", "https://linear.app/acme/issue/ENG-123")
	if err != nil || !reserved {
		t.Fatalf("reserve Linear issue: reserved=%v err=%v", reserved, err)
	}
	reserved, err = store.ReserveIssueWatchTask(ctx, watch.ID, "ENG-123", "https://linear.app/acme/issue/ENG-123")
	if err != nil || reserved {
		t.Fatalf("duplicate Linear issue reservation: reserved=%v err=%v", reserved, err)
	}
	if err := store.AssignIssueWatchTaskID(ctx, watch.ID, "ENG-123", "task-1"); err != nil {
		t.Fatalf("assign Linear issue task: %v", err)
	}
	reservations, err := store.listIssueWatchReservations(ctx, watch.ID)
	if err != nil || len(reservations) != 1 || !reservations["ENG-123"] {
		t.Fatalf("list Linear reservations: rows=%v err=%v", reservations, err)
	}
}

func TestPostgresPendingIssueWatchPollRevision(t *testing.T) {
	ctx := t.Context()
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	database.SetMaxOpenConns(2)
	store, err := NewStore(database, database)
	require.NoError(t, err)

	watch := newTestIssueWatch("ws-1")
	watch.Filter.RequireBlockersDone = true
	require.NoError(t, store.CreateIssueWatch(ctx, watch))
	revision, err := store.issueWatchPollRevision(ctx, watch.ID)
	require.NoError(t, err)

	type result struct {
		issueID string
		applied bool
		err     error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for _, issueID := range []string{"id-1", "id-2"} {
		go func() {
			<-start
			applied, err := store.replacePendingIssueIDs(ctx, watch.ID, revision, []string{issueID})
			results <- result{issueID: issueID, applied: applied, err: err}
		}()
	}
	close(start)
	first, second := <-results, <-results
	require.NoError(t, first.err)
	require.NoError(t, second.err)
	require.NotEqual(t, first.applied, second.applied)
	committed := first
	if second.applied {
		committed = second
	}
	ids, err := store.ListPendingIssueIDs(ctx, watch.ID)
	require.NoError(t, err)
	require.Equal(t, []string{committed.issueID}, ids)

	require.NoError(t, store.ResetIssueWatchState(ctx, watch.ID))
	applied, err := store.replacePendingIssueIDs(ctx, watch.ID, revision+1, ids)
	require.NoError(t, err)
	require.False(t, applied)
	ids, err = store.ListPendingIssueIDs(ctx, watch.ID)
	require.NoError(t, err)
	require.Empty(t, ids)
}
