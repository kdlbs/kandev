package workflowsync

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
		t.Fatalf("first workflow sync store schema init: %v", err)
	}
	replayed, err := NewStore(database, database)
	if err != nil {
		t.Fatalf("second workflow sync store schema init: %v", err)
	}

	pollEnabled := true
	request := &SetConfigRequest{
		Provider:        ProviderGitHub,
		RepoOwner:       "acme",
		RepoName:        "workflows",
		Branch:          "main",
		Path:            ".github/workflows",
		IntervalSeconds: 120,
		PollEnabled:     &pollEnabled,
	}
	if err := request.Normalize(); err != nil {
		t.Fatalf("normalize workflow sync config: %v", err)
	}
	if _, err := replayed.UpsertConfigForWorkspace(ctx, "ws-1", request); err != nil {
		t.Fatalf("upsert workflow sync config: %v", err)
	}
	updatedAt := time.Now().UTC().Truncate(time.Microsecond)
	if err := replayed.RecordSyncStatus(ctx, "ws-1", true, "", []string{"warning"}, "hash-1", updatedAt); err != nil {
		t.Fatalf("record workflow sync status: %v", err)
	}
	config, err := replayed.GetConfigForWorkspace(ctx, "ws-1")
	if err != nil || config == nil || !config.LastOk || !config.PollEnabled || config.LastHash != "hash-1" {
		t.Fatalf("workflow sync config = %+v, err=%v", config, err)
	}
	configs, err := replayed.ListConfigs(ctx)
	if err != nil || len(configs) != 1 || configs[0].WorkspaceID != "ws-1" {
		t.Fatalf("workflow sync configs = %+v, err=%v", configs, err)
	}
	if err := replayed.DeleteConfigForWorkspace(ctx, "ws-1"); err != nil {
		t.Fatalf("delete workflow sync config: %v", err)
	}
	if config, err := store.GetConfigForWorkspace(ctx, "ws-1"); err != nil || config != nil {
		t.Fatalf("workflow sync config after delete = %+v, err=%v", config, err)
	}
}
