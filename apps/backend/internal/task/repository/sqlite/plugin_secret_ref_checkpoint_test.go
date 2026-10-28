package sqlite

import (
	"context"
	"testing"
)

// A checkpoint that adds the agentctl auth-token secret reference must keep it:
// recovery reads the token through that reference after a restart.
func TestPluginCheckpointPersistsTheAuthTokenSecretReference(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedPluginExecutorInventory(t, repo, 7)
	ctx := context.Background()
	taskID := "task-plugin-executor-inventory"
	if err := repo.CheckpointPluginExecutorInventory(ctx, stateVersionFixture(taskID, "execution-secret", 1, "allocating")); err != nil {
		t.Fatalf("allocating: %v", err)
	}
	next := stateVersionFixture(taskID, "execution-secret", 1, "bootstrapping")
	next.ExpectedPluginExecutorRevision = 1
	next.Metadata[pluginAuthTokenSecretMetadataKey] = "secret-auth-1"
	if err := repo.CheckpointPluginExecutorInventory(ctx, next); err != nil {
		t.Fatalf("bootstrapping: %v", err)
	}
	stored, err := repo.GetExecutorRunningBySessionID(ctx, "session-plugin-executor-inventory")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got := stored.Metadata[pluginAuthTokenSecretMetadataKey]; got != "secret-auth-1" {
		t.Fatalf("stored auth token reference = %v, want it kept by the checkpoint", got)
	}
}
