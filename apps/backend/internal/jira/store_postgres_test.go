package jira

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresStoreSchemaReplay(t *testing.T) {
	ctx := context.Background()
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))

	store, err := NewStore(database, database)
	if err != nil {
		t.Fatalf("first Jira store schema init: %v", err)
	}
	if _, err := NewStore(database, database); err != nil {
		t.Fatalf("second Jira store schema init: %v", err)
	}

	config := &JiraConfig{
		SiteURL:    "https://jira.example.com",
		Email:      "alice@example.com",
		AuthMethod: AuthMethodAPIToken,
	}
	if err := store.UpsertConfigForWorkspace(ctx, "ws-1", config); err != nil {
		t.Fatalf("upsert Jira config: %v", err)
	}
	gotConfig, err := store.GetConfigForWorkspace(ctx, "ws-1")
	if err != nil || gotConfig == nil || gotConfig.SiteURL != config.SiteURL {
		t.Fatalf("get Jira config: config=%+v err=%v", gotConfig, err)
	}
	if err := store.UpdateAuthHealthForWorkspace(ctx, "ws-1", true, "", time.Now().UTC()); err != nil {
		t.Fatalf("update Jira auth health: %v", err)
	}

	watch := &IssueWatch{
		WorkspaceID:       "ws-1",
		WorkflowID:        "workflow-1",
		WorkflowStepID:    "step-1",
		JQL:               "project = ENG",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "executor-1",
		Enabled:           true,
	}
	if err := store.CreateIssueWatch(ctx, watch); err != nil {
		t.Fatalf("create Jira issue watch: %v", err)
	}
	gotWatch, err := store.GetIssueWatch(ctx, watch.ID)
	if err != nil || gotWatch == nil || gotWatch.JQL != watch.JQL {
		t.Fatalf("get Jira issue watch: watch=%+v err=%v", gotWatch, err)
	}
	if enabled, err := store.ListEnabledIssueWatches(ctx); err != nil || len(enabled) != 1 {
		t.Fatalf("list enabled Jira issue watches: watches=%d err=%v", len(enabled), err)
	}

	reserved, err := store.ReserveIssueWatchTask(ctx, watch.ID, "ENG-123", "https://jira.example.com/browse/ENG-123")
	if err != nil || !reserved {
		t.Fatalf("reserve Jira issue: reserved=%v err=%v", reserved, err)
	}
	reserved, err = store.ReserveIssueWatchTask(ctx, watch.ID, "ENG-123", "https://jira.example.com/browse/ENG-123")
	if err != nil || reserved {
		t.Fatalf("duplicate Jira issue reservation: reserved=%v err=%v", reserved, err)
	}
	if err := store.AssignIssueWatchTaskID(ctx, watch.ID, "ENG-123", "task-1"); err != nil {
		t.Fatalf("assign Jira issue task: %v", err)
	}
	seen, err := store.ListSeenIssueKeys(ctx, watch.ID)
	if err != nil || len(seen) != 1 {
		t.Fatalf("list Jira seen issues: seen=%d err=%v", len(seen), err)
	}
}
