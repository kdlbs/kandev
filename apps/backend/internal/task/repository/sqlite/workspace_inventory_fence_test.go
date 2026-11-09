package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	fenceTaskID    = "task-recovery"
	fenceSessionID = "session-recovery"
)

func TestReadWorkspaceInventoryRepairFenceReportsRouteState(t *testing.T) {
	repo := newRepoForEntityTests(t)
	seedWorkspaceInventoryRecovery(t, repo)
	ctx := context.Background()

	fence, err := repo.ReadWorkspaceInventoryRepairFence(ctx, fenceTaskID, fenceSessionID)
	if err != nil {
		t.Fatalf("read fence without route: %v", err)
	}
	if fence.RouteKnown || fence.RouteGeneration != 0 || fence.RouteState != "" {
		t.Fatalf("fence without a route row = %+v, want unknown route", fence)
	}
	if fence.SessionState != string(models.TaskSessionStateFailed) || fence.TaskUpdatedAt.IsZero() || fence.SessionUpdatedAt.IsZero() {
		t.Fatalf("fence lifecycle state = %+v", fence)
	}

	routeUpdatedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := repo.SaveRouteState(ctx, dynamicruntime.RouteState{
		SessionID: fenceSessionID, LogicalProfileID: "logical", Generation: 3, Status: "active", UpdatedAt: routeUpdatedAt,
	}); err != nil {
		t.Fatal(err)
	}
	fence, err = repo.ReadWorkspaceInventoryRepairFence(ctx, fenceTaskID, fenceSessionID)
	if err != nil {
		t.Fatalf("read fence with route: %v", err)
	}
	if !fence.RouteKnown || fence.RouteGeneration != 3 || fence.RouteState != "active" || !fence.RouteUpdatedAt.Equal(routeUpdatedAt) {
		t.Fatalf("fence with a route row = %+v", fence)
	}

	if _, err := repo.ReadWorkspaceInventoryRepairFence(ctx, "task-other", fenceSessionID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("fence for a session of another task error = %v, want sql.ErrNoRows", err)
	}
}

// The repair transaction re-reads the fence and refuses to write when any
// fenced lifecycle field moved after the caller captured it.
func TestRepairWorkspaceInventoryRejectsFenceDriftInsideTransaction(t *testing.T) {
	later := time.Now().UTC().Add(time.Hour)
	tests := []struct {
		name  string
		drift func(*testing.T, *Repository)
	}{
		{name: "task updated", drift: func(t *testing.T, repo *Repository) {
			execFenceDrift(t, repo, `UPDATE tasks SET updated_at = ? WHERE id = ?`, later, fenceTaskID)
		}},
		{name: "workflow step", drift: func(t *testing.T, repo *Repository) {
			execFenceDrift(t, repo, `UPDATE tasks SET workflow_step_id = ? WHERE id = ?`, "step-moved", fenceTaskID)
		}},
		{name: "session state", drift: func(t *testing.T, repo *Repository) {
			execFenceDrift(t, repo, `UPDATE task_sessions SET state = ? WHERE id = ?`, string(models.TaskSessionStateCancelled), fenceSessionID)
		}},
		{name: "session updated", drift: func(t *testing.T, repo *Repository) {
			execFenceDrift(t, repo, `UPDATE task_sessions SET updated_at = ? WHERE id = ?`, later, fenceSessionID)
		}},
		{name: "route appears", drift: func(t *testing.T, repo *Repository) {
			if err := repo.SaveRouteState(context.Background(), dynamicruntime.RouteState{
				SessionID: fenceSessionID, LogicalProfileID: "logical", Generation: 1, Status: "active", UpdatedAt: later,
			}); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, repair := fencedWorkspaceInventoryRepair(t)
			tt.drift(t, repo)

			if _, err := repo.RepairWorkspaceInventory(context.Background(), repair); !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
				t.Fatalf("repair after %s drift error = %v, want conflict", tt.name, err)
			}
			requireNoFencedRepairWrite(t, repo, repair)
		})
	}

	t.Run("route generation advances", func(t *testing.T) {
		repo := newRepoForEntityTests(t)
		seedWorkspaceInventoryRecovery(t, repo)
		route := dynamicruntime.RouteState{SessionID: fenceSessionID, LogicalProfileID: "logical", Generation: 1, Status: "active", UpdatedAt: later}
		if err := repo.SaveRouteState(context.Background(), route); err != nil {
			t.Fatal(err)
		}
		repair := buildFencedWorkspaceInventoryRepair(t, repo)
		route.Generation = 2
		if err := repo.SaveRouteState(context.Background(), route); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.RepairWorkspaceInventory(context.Background(), repair); !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
			t.Fatalf("repair after route generation drift error = %v, want conflict", err)
		}
		requireNoFencedRepairWrite(t, repo, repair)
	})

	t.Run("unchanged fence commits", func(t *testing.T) {
		repo, repair := fencedWorkspaceInventoryRepair(t)
		receipt, err := repo.RepairWorkspaceInventory(context.Background(), repair)
		if err != nil {
			t.Fatalf("repair with unchanged fence: %v", err)
		}
		if !repair.Preservation.Fence.Equal(receipt.Preservation.Fence) {
			t.Fatalf("receipt fence = %+v, want %+v", receipt.Preservation.Fence, repair.Preservation.Fence)
		}
	})
}

// Pins the CURRENT store behavior for a same-key retry after the fence moved:
// the fence is re-validated before the idempotency lookup, so the retry is a
// conflict rather than a deduplicated replay of the committed receipt. Whether
// a same-key retry should behave this way is an open design question; this
// test documents today's behavior and must be updated with that decision.
func TestRepairWorkspaceInventorySameKeyRetryAfterFenceChangeConflicts(t *testing.T) {
	repo, repair := fencedWorkspaceInventoryRepair(t)
	ctx := context.Background()
	first, err := repo.RepairWorkspaceInventory(ctx, repair)
	if err != nil {
		t.Fatalf("first repair: %v", err)
	}
	replay, err := repo.RepairWorkspaceInventory(ctx, repair)
	if err != nil || replay.ID != first.ID || replay.ResultCode != models.WorkspaceInventoryRecoveryDeduplicated {
		t.Fatalf("same-key retry with unchanged fence = %+v, %v; want deduplicated receipt", replay, err)
	}

	execFenceDrift(t, repo, `UPDATE tasks SET updated_at = ? WHERE id = ?`, time.Now().UTC().Add(time.Hour), fenceTaskID)
	if _, err := repo.RepairWorkspaceInventory(ctx, repair); !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
		t.Fatalf("same-key retry after fence change error = %v, want conflict", err)
	}
	stored, err := repo.GetWorkspaceInventoryRepairReceipt(ctx, fenceTaskID, repair.IdempotencyKey)
	if err != nil || stored == nil || stored.ID != first.ID {
		t.Fatalf("committed receipt after rejected retry = %+v, %v", stored, err)
	}
}

func fencedWorkspaceInventoryRepair(t *testing.T) (*Repository, *models.WorkspaceInventoryRepair) {
	t.Helper()
	repo := newRepoForEntityTests(t)
	seedWorkspaceInventoryRecovery(t, repo)
	return repo, buildFencedWorkspaceInventoryRepair(t, repo)
}

func buildFencedWorkspaceInventoryRepair(t *testing.T, repo *Repository) *models.WorkspaceInventoryRepair {
	t.Helper()
	ctx := context.Background()
	execFenceDrift(t, repo, `UPDATE task_environment_repos SET branch_slug = ? WHERE id = ?`, "stale", "environment-repository-recovery")
	env, err := repo.GetTaskEnvironment(ctx, "environment-recovery")
	if err != nil {
		t.Fatal(err)
	}
	taskRepo, err := repo.GetTaskRepository(ctx, "task-repository-recovery")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := repo.ListTaskEnvironmentRepos(ctx, env.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("load inventory: %+v, %v", rows, err)
	}
	row := rows[0]
	fence, err := repo.ReadWorkspaceInventoryRepairFence(ctx, fenceTaskID, fenceSessionID)
	if err != nil {
		t.Fatal(err)
	}
	return &models.WorkspaceInventoryRepair{
		TaskID: fenceTaskID, WorkspaceID: "workspace-recovery", SessionID: fenceSessionID,
		TaskEnvironmentID: env.ID, TaskRepositoryID: taskRepo.ID, RepositoryID: taskRepo.RepositoryID,
		EnvironmentRepoID: row.ID, BranchSlug: "main",
		WorktreeID: row.WorktreeID, WorktreePath: row.WorktreePath,
		WorktreeBranch: row.WorktreeBranch, Position: row.Position,
		IdempotencyKey: "fenced-repair", RequestHash: "fenced-request-hash",
		ExpectedEnvironmentUpdatedAt:  env.UpdatedAt,
		ExpectedTaskRepositoryUpdate:  taskRepo.UpdatedAt,
		ExpectedEnvironmentRepoUpdate: row.UpdatedAt,
		Preservation: models.WorkspaceInventoryPreservation{
			Fence: fence, HeadOID: "0123456789abcdef", StatusHash: "status", ContentHash: "content",
		},
	}
}

func requireNoFencedRepairWrite(t *testing.T, repo *Repository, repair *models.WorkspaceInventoryRepair) {
	t.Helper()
	ctx := context.Background()
	receipt, err := repo.GetWorkspaceInventoryRepairReceipt(ctx, repair.TaskID, repair.IdempotencyKey)
	if err != nil || receipt != nil {
		t.Fatalf("receipt after rejected repair = %+v, %v; want none", receipt, err)
	}
	rows, err := repo.ListTaskEnvironmentRepos(ctx, repair.TaskEnvironmentID)
	if err != nil || len(rows) != 1 || rows[0].BranchSlug != "stale" {
		t.Fatalf("inventory after rejected repair = %+v, %v; want the stale row untouched", rows, err)
	}
}

func execFenceDrift(t *testing.T, repo *Repository, query string, args ...any) {
	t.Helper()
	if _, err := repo.db.ExecContext(context.Background(), repo.db.Rebind(query), args...); err != nil {
		t.Fatal(err)
	}
}
