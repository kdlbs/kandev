package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/task/models"
)

// TestProvideRepositoriesPreservesUsageRollupAcrossReboot closes Review
// Round 2's M3 finding: the task plan's AC-36 row claimed
// TestBackfillSkippedWhenTaskSessionsAlreadyHasUsageColumns (a boot-replay
// test) covered "rollup survives a reboot", but that test was deleted along
// with the one-time backfill migration it exercised (the backfill became
// unnecessary once task_usage_events shipped as a day-one feature with no
// pre-existing rollup data to migrate) - leaving no replacement proving a
// real second boot doesn't touch already-populated rollup columns.
//
// This test boots provideRepositories on a temp on-disk SQLite database,
// writes one ledger row through the real CreateTaskUsageEvent path (so
// task_sessions.tokens_in/tokens_cached_in/tokens_out/cost_subcents are
// populated exactly as production sets them), fully tears the boot down via
// its cleanups (closing the database), then calls provideRepositories again
// on the same on-disk file and asserts all four rollup columns are
// unchanged - schema init and migrations on the second boot must not reset,
// re-derive, or re-backfill them.
func TestProvideRepositoriesPreservesUsageRollupAcrossReboot(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{
		HomeDir:  t.TempDir(),
		Database: config.DatabaseConfig{Driver: "sqlite"},
	}

	firstPool, firstRepos, firstCleanups, err := provideRepositories(ctx, cfg, nil, "test")
	if err != nil {
		t.Fatalf("first provideRepositories: %v", err)
	}
	_ = firstPool
	firstClosed := false
	closeFirst := func() {
		if firstClosed {
			return
		}
		firstClosed = true
		for i := len(firstCleanups) - 1; i >= 0; i-- {
			if firstCleanups[i] != nil {
				_ = firstCleanups[i]()
			}
		}
	}
	t.Cleanup(closeFirst)

	if err := firstRepos.Task.CreateTask(ctx, &models.Task{
		ID:          "task-reboot-rollup",
		WorkspaceID: "ws-1",
		Title:       "Usage rollup reboot test task",
		Priority:    "medium",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := firstRepos.Task.CreateTaskSession(ctx, &models.TaskSession{
		ID:     "session-reboot-rollup",
		TaskID: "task-reboot-rollup",
		State:  models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	tokensCachedRead := int64(20)
	tokensCachedWrite := int64(5)
	tokensOut := int64(30)
	if err := firstRepos.Task.CreateTaskUsageEvent(ctx, &models.TaskUsageEvent{
		UsageEventID:      "evt-reboot-rollup",
		TaskID:            "task-reboot-rollup",
		SessionID:         "session-reboot-rollup",
		AgentType:         "claude",
		Model:             "claude-sonnet-5",
		Provider:          "anthropic",
		TokensIn:          100,
		TokensCachedRead:  &tokensCachedRead,
		TokensCachedWrite: &tokensCachedWrite,
		TokensOut:         &tokensOut,
		TokensTotal:       155,
		CostSubcents:      42,
		CostSource:        "actual",
		ContractVersion:   1,
	}); err != nil {
		t.Fatalf("CreateTaskUsageEvent: %v", err)
	}

	before, err := firstRepos.Task.GetTaskSession(ctx, "session-reboot-rollup")
	if err != nil {
		t.Fatalf("GetTaskSession before reboot: %v", err)
	}
	if before.TokensIn != 100 || before.TokensCachedIn != 25 || before.TokensOut != 30 || before.CostSubcents != 42 {
		t.Fatalf("rollup before reboot = (%d,%d,%d,%d), want (100,25,30,42)",
			before.TokensIn, before.TokensCachedIn, before.TokensOut, before.CostSubcents)
	}

	// Tear the first boot fully down (closes the database) before reopening
	// the same on-disk file, so the second provideRepositories call is a
	// genuine cold reboot rather than a second handle onto a live pool.
	closeFirst()

	_, secondRepos, secondCleanups, err := provideRepositories(ctx, cfg, nil, "test")
	if err != nil {
		t.Fatalf("second provideRepositories: %v", err)
	}
	t.Cleanup(func() {
		for i := len(secondCleanups) - 1; i >= 0; i-- {
			if secondCleanups[i] != nil {
				_ = secondCleanups[i]()
			}
		}
	})

	after, err := secondRepos.Task.GetTaskSession(ctx, "session-reboot-rollup")
	if err != nil {
		t.Fatalf("GetTaskSession after reboot: %v", err)
	}
	if after.TokensIn != before.TokensIn || after.TokensCachedIn != before.TokensCachedIn ||
		after.TokensOut != before.TokensOut || after.CostSubcents != before.CostSubcents {
		t.Errorf("rollup after reboot = (%d,%d,%d,%d), want unchanged (%d,%d,%d,%d): "+
			"a second boot's schema init/migrations must not reset or re-derive already-populated usage rollup columns",
			after.TokensIn, after.TokensCachedIn, after.TokensOut, after.CostSubcents,
			before.TokensIn, before.TokensCachedIn, before.TokensOut, before.CostSubcents)
	}
}
