package sentry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresStoreSchemaReplay(t *testing.T) {
	ctx := context.Background()
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))

	store, err := NewStore(database, database)
	if err != nil {
		t.Fatalf("first Sentry store schema init: %v", err)
	}
	if _, err := NewStore(database, database); err != nil {
		t.Fatalf("second Sentry store schema init: %v", err)
	}

	instance := &SentryConfig{
		WorkspaceID: "ws-1",
		Name:        "SaaS",
		AuthMethod:  AuthMethodAuthToken,
		URL:         "https://sentry.io",
	}
	if err := store.CreateInstance(ctx, instance); err != nil {
		t.Fatalf("create Sentry instance: %v", err)
	}
	gotInstance, err := store.GetInstance(ctx, instance.ID)
	if err != nil || gotInstance == nil || gotInstance.Name != instance.Name {
		t.Fatalf("get Sentry instance: instance=%+v err=%v", gotInstance, err)
	}
	if err := store.UpdateAuthHealthForInstance(ctx, instance.ID, true, "", time.Now().UTC()); err != nil {
		t.Fatalf("update Sentry auth health: %v", err)
	}

	watch := &IssueWatch{
		WorkspaceID:       "ws-1",
		SentryInstanceID:  instance.ID,
		WorkflowID:        "workflow-1",
		WorkflowStepID:    "step-1",
		Filter:            SearchFilter{Query: "is:unresolved"},
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "executor-1",
		Enabled:           true,
	}
	if err := store.CreateIssueWatch(ctx, watch); err != nil {
		t.Fatalf("create Sentry issue watch: %v", err)
	}
	gotWatch, err := store.GetIssueWatch(ctx, watch.ID)
	if err != nil || gotWatch == nil || gotWatch.SentryInstanceID != instance.ID {
		t.Fatalf("get Sentry issue watch: watch=%+v err=%v", gotWatch, err)
	}
	if enabled, err := store.ListEnabledIssueWatches(ctx); err != nil || len(enabled) != 1 {
		t.Fatalf("list enabled Sentry issue watches: watches=%d err=%v", len(enabled), err)
	}
	if count, err := store.CountWatchesForInstance(ctx, instance.ID); err != nil || count != 1 {
		t.Fatalf("count Sentry instance watches: count=%d err=%v", count, err)
	}

	reserved, err := store.ReserveIssueWatchTask(ctx, watch.ID, "PROJ-123", "https://sentry.io/issues/PROJ-123")
	if err != nil || !reserved {
		t.Fatalf("reserve Sentry issue: reserved=%v err=%v", reserved, err)
	}
	reserved, err = store.ReserveIssueWatchTask(ctx, watch.ID, "PROJ-123", "https://sentry.io/issues/PROJ-123")
	if err != nil || reserved {
		t.Fatalf("duplicate Sentry issue reservation: reserved=%v err=%v", reserved, err)
	}
	if err := store.AssignIssueWatchTaskID(ctx, watch.ID, "PROJ-123", "task-1"); err != nil {
		t.Fatalf("assign Sentry issue task: %v", err)
	}
	seen, err := store.ListSeenIssueShortIDs(ctx, watch.ID)
	if err != nil || len(seen) != 1 {
		t.Fatalf("list Sentry seen issues: seen=%d err=%v", len(seen), err)
	}
	if err := store.DeleteInstance(ctx, instance.ID); !errors.As(err, &ErrInstanceInUse{}) {
		t.Fatalf("expected Sentry instance FK protection, got %v", err)
	}
}
