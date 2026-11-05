package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// A handed-off resource has no inventory row, so the environment row is the
// only thing that still names it: deleting the row first would orphan it.
func TestEnvironmentDeleteIsRefusedWhileAHandoffRemains(t *testing.T) {
	exerciseHandoffGuard(t, newRepoForSessionTests(t), seedDescriptorEnvironmentSQLite)
}

func exerciseHandoffGuard(t *testing.T, repo *Repository, seed descriptorSeeder) {
	t.Helper()
	seed(t, repo, 1)
	ctx := context.Background()
	if err := repo.RecordUnrecordedPluginExecutor(ctx, unrecordedFixture()); err != nil {
		t.Fatalf("record: %v", err)
	}
	var generation int64
	if err := repo.db.QueryRowContext(ctx, repo.db.Rebind(`SELECT ownership_generation FROM task_environments WHERE id = ?`),
		"environment-descriptor-1").Scan(&generation); err != nil {
		t.Fatalf("read generation: %v", err)
	}
	if err := repo.TransferTaskEnvironmentOwnership(ctx, "environment-descriptor-1", "task-descriptor-1", generation, "task-other-owner"); !errors.Is(err, ErrUnrecordedPluginExecutorsRemain) {
		t.Fatalf("TransferTaskEnvironmentOwnership error = %v, want ErrUnrecordedPluginExecutorsRemain", err)
	}
	if err := repo.DeleteTaskEnvironment(ctx, "environment-descriptor-1"); !errors.Is(err, ErrUnrecordedPluginExecutorsRemain) {
		t.Fatalf("DeleteTaskEnvironment error = %v, want ErrUnrecordedPluginExecutorsRemain", err)
	}
	if err := repo.DeleteTaskEnvironmentsByTask(ctx, "task-descriptor-1"); !errors.Is(err, ErrUnrecordedPluginExecutorsRemain) {
		t.Fatalf("DeleteTaskEnvironmentsByTask error = %v, want ErrUnrecordedPluginExecutorsRemain", err)
	}
	if err := repo.DeleteUnrecordedPluginExecutor(ctx, unrecordedFixture().ExecutionID); err != nil {
		t.Fatalf("settle handoff: %v", err)
	}
	if err := repo.DeleteTaskEnvironment(ctx, "environment-descriptor-1"); err != nil {
		t.Fatalf("DeleteTaskEnvironment after settling: %v", err)
	}
}

func TestListAllUnrecordedPluginExecutorsSpansEnvironments(t *testing.T) {
	exerciseListAllUnrecorded(t, newRepoForSessionTests(t), seedDescriptorEnvironmentSQLite)
}

func exerciseListAllUnrecorded(t *testing.T, repo *Repository, seed descriptorSeeder) {
	t.Helper()
	seed(t, repo, 1)
	ctx := context.Background()
	first := unrecordedFixture()
	second := unrecordedFixture()
	second.ExecutionID, second.EnvironmentID = first.ExecutionID+"-other", "environment-other"
	for _, item := range []*models.UnrecordedPluginExecutor{first, second} {
		if err := repo.RecordUnrecordedPluginExecutor(ctx, item); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	listed, err := repo.ListAllUnrecordedPluginExecutors(ctx)
	if err != nil || len(listed) != 2 {
		t.Fatalf("listed %d, err=%v, want both environments", len(listed), err)
	}
}
