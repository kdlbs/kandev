package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func newArchivedRecoveryBlockFixture(t *testing.T, taskID, sessionID, blockID string, sessionState models.TaskSessionState) *Repository {
	t.Helper()
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: taskID}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: sessionState,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{
		ID: blockID, SessionID: sessionID, IncarnationID: "incarnation-1",
		ExpectedGeneration: 0, Reason: "unknown_prompt_outcome",
		State: models.RecoveryBlockOpen, ConsumerReference: "agent_delivery",
	}); err != nil {
		t.Fatalf("UpsertSessionRecoveryBlock: %v", err)
	}
	return repo
}

func TestArchivedRecoveryBlockClosesAndUnarchiveReopens(t *testing.T) {
	repo := newArchivedRecoveryBlockFixture(t, "task-archived-block", "session-archived-block", "block-archived", models.TaskSessionStateCancelled)
	ctx := context.Background()
	if err := repo.ArchiveTask(ctx, "task-archived-block"); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}

	ids, err := repo.ListOpenRecoveryBlocksForArchivedTasks(ctx, 10)
	if err != nil {
		t.Fatalf("ListOpenRecoveryBlocksForArchivedTasks: %v", err)
	}
	if len(ids) != 1 || ids[0] != "block-archived" {
		t.Fatalf("archived open blocks = %v, want [block-archived]", ids)
	}

	changed, err := repo.ResolveSessionRecoveryBlockForArchivedTask(
		ctx, "block-archived", models.RecoveryBlockActionTaskArchived, time.Now().UTC(),
	)
	if err != nil || !changed {
		t.Fatalf("ResolveSessionRecoveryBlockForArchivedTask = %v, %v; want true", changed, err)
	}
	block, err := repo.GetSessionRecoveryBlock(ctx, "block-archived")
	if err != nil {
		t.Fatalf("GetSessionRecoveryBlock: %v", err)
	}
	if block.State != models.RecoveryBlockResolved || block.AuthorizedAction != models.RecoveryBlockActionTaskArchived || block.ResolvedAt == nil {
		t.Fatalf("archived block after close = %+v", block)
	}

	restored, err := repo.UnarchiveTask(ctx, "task-archived-block")
	if err != nil || !restored {
		t.Fatalf("UnarchiveTask = %v, %v; want true", restored, err)
	}
	block, err = repo.GetSessionRecoveryBlock(ctx, "block-archived")
	if err != nil {
		t.Fatalf("GetSessionRecoveryBlock after unarchive: %v", err)
	}
	if block.State != models.RecoveryBlockOpen || block.AuthorizedAction != "" || block.ResolvedAt != nil {
		t.Fatalf("block after unarchive = %+v, want open with cleared action", block)
	}
}

func TestArchivedRecoveryBlockCascadeUnarchiveReopens(t *testing.T) {
	repo := newArchivedRecoveryBlockFixture(t, "task-cascade-block", "session-cascade-block", "block-cascade", models.TaskSessionStateCancelled)
	ctx := context.Background()
	changed, err := repo.ArchiveTaskIfActive(ctx, "task-cascade-block", "cascade-1")
	if err != nil || !changed {
		t.Fatalf("ArchiveTaskIfActive = %v, %v; want true", changed, err)
	}
	if _, err := repo.ResolveSessionRecoveryBlockForArchivedTask(
		ctx, "block-cascade", models.RecoveryBlockActionTaskArchived, time.Now().UTC(),
	); err != nil {
		t.Fatalf("ResolveSessionRecoveryBlockForArchivedTask: %v", err)
	}
	restored, err := repo.UnarchiveTaskByCascade(ctx, "task-cascade-block", "cascade-1")
	if err != nil || !restored {
		t.Fatalf("UnarchiveTaskByCascade = %v, %v; want true", restored, err)
	}
	block, err := repo.GetSessionRecoveryBlock(ctx, "block-cascade")
	if err != nil {
		t.Fatalf("GetSessionRecoveryBlock: %v", err)
	}
	if block.State != models.RecoveryBlockOpen || block.AuthorizedAction != "" || block.ResolvedAt != nil {
		t.Fatalf("cascade block after unarchive = %+v, want open with cleared action", block)
	}
}

func TestArchivedRecoveryBlockLeavesLiveNonPrimaryAndActiveSessionsOpen(t *testing.T) {
	t.Run("unarchived non-primary", func(t *testing.T) {
		repo := newArchivedRecoveryBlockFixture(t, "task-live-nonprimary", "session-live-nonprimary", "block-live-nonprimary", models.TaskSessionStateCancelled)
		ctx := context.Background()
		ids, err := repo.ListOpenRecoveryBlocksForArchivedTasks(ctx, 10)
		if err != nil {
			t.Fatalf("ListOpenRecoveryBlocksForArchivedTasks: %v", err)
		}
		if len(ids) != 0 {
			t.Fatalf("unarchived task blocks = %v, want none", ids)
		}
		changed, err := repo.ResolveSessionRecoveryBlockForArchivedTask(
			ctx, "block-live-nonprimary", models.RecoveryBlockActionTaskArchived, time.Now().UTC(),
		)
		if err != nil || changed {
			t.Fatalf("resolve unarchived block = %v, %v; want false", changed, err)
		}
	})
	t.Run("archived active session", func(t *testing.T) {
		repo := newArchivedRecoveryBlockFixture(t, "task-live-session", "session-live-session", "block-live-session", models.TaskSessionStateRunning)
		ctx := context.Background()
		if err := repo.ArchiveTask(ctx, "task-live-session"); err != nil {
			t.Fatalf("ArchiveTask: %v", err)
		}
		ids, err := repo.ListOpenRecoveryBlocksForArchivedTasks(ctx, 10)
		if err != nil {
			t.Fatalf("ListOpenRecoveryBlocksForArchivedTasks: %v", err)
		}
		if len(ids) != 0 {
			t.Fatalf("active-session blocks = %v, want none", ids)
		}
		changed, err := repo.ResolveSessionRecoveryBlockForArchivedTask(
			ctx, "block-live-session", models.RecoveryBlockActionTaskArchived, time.Now().UTC(),
		)
		if err != nil || changed {
			t.Fatalf("resolve active-session block = %v, %v; want false", changed, err)
		}
	})
}
