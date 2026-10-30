package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

func unrecordedFixture() *models.UnrecordedPluginExecutor {
	running := descriptorFixture(1, "install-a", "cleanup_pending", "", "")
	running.Metadata["plugin_executor"].(map[string]interface{})["resource"] = map[string]interface{}{"resource_handle": "opaque-resource"}
	return &models.UnrecordedPluginExecutor{
		ExecutionID: running.AgentExecutionID, SessionID: running.SessionID, TaskID: running.TaskID,
		EnvironmentID: "environment-descriptor-1", Running: running,
	}
}

// The archive-during-provision leak: while a cleanup claim holds the
// environment the launch cannot write its inventory row, so the resource it
// provisioned has to be recorded somewhere that does not depend on the claim.
func TestLaunchCannotCheckpointWhileCleanupHoldsTheEnvironmentButCanRecordTheResource(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedDescriptorEnvironmentSQLite(t, repo, 1)
	ctx := context.Background()
	acquireClaimForDescriptorEnvironment(t, repo, "op-archive:cleanup")

	provisioned := descriptorFixture(1, "install-a", "allocating", "", "")
	if err := repo.CheckpointPluginExecutorInventory(ctx, provisioned); !errors.Is(err, recoveryclaim.ErrBusy) {
		t.Fatalf("checkpoint under a held claim error = %v, want a busy refusal", err)
	}
	if err := repo.RecordUnrecordedPluginExecutor(ctx, unrecordedFixture()); err != nil {
		t.Fatalf("RecordUnrecordedPluginExecutor() error = %v", err)
	}
}

func TestUnrecordedPluginExecutorSurvivesAndIsListedByEnvironment(t *testing.T) {
	exerciseUnrecordedPluginExecutorRoundTrip(t, newRepoForSessionTests(t), seedDescriptorEnvironmentSQLite)
}

func exerciseUnrecordedPluginExecutorRoundTrip(t *testing.T, repo *Repository, seed descriptorSeeder) {
	t.Helper()
	seed(t, repo, 1)
	ctx := context.Background()
	if err := repo.RecordUnrecordedPluginExecutor(ctx, unrecordedFixture()); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := repo.RecordUnrecordedPluginExecutor(ctx, unrecordedFixture()); err != nil {
		t.Fatalf("recording the same execution twice must be idempotent: %v", err)
	}
	other, err := repo.ListUnrecordedPluginExecutors(ctx, "task-descriptor-1", "environment-other")
	if err != nil || len(other) != 0 {
		t.Fatalf("other environment listed %d, err=%v", len(other), err)
	}
	listed, err := repo.ListUnrecordedPluginExecutors(ctx, "task-descriptor-1", "environment-descriptor-1")
	if err != nil || len(listed) != 1 {
		t.Fatalf("listed %d, err=%v, want 1", len(listed), err)
	}
	got := listed[0]
	envelope, _ := got.Running.Metadata["plugin_executor"].(map[string]interface{})
	resource, _ := envelope["resource"].(map[string]interface{})
	if got.ExecutionID != "execution-descriptor-1" || got.SessionID != "session-descriptor-1" || resource["resource_handle"] != "opaque-resource" {
		t.Fatalf("round trip lost the record: %+v / %+v", got, envelope)
	}
	if err := repo.DeleteUnrecordedPluginExecutor(ctx, got.ExecutionID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := repo.DeleteUnrecordedPluginExecutor(ctx, got.ExecutionID); err != nil {
		t.Fatalf("deleting twice must be idempotent: %v", err)
	}
	if listed, _ := repo.ListUnrecordedPluginExecutors(ctx, "task-descriptor-1", "environment-descriptor-1"); len(listed) != 0 {
		t.Fatalf("record survived its delete: %d", len(listed))
	}
}
