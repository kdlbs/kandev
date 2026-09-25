package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestArchiveTaskRejectsRetentionHoldAtMutationBoundary(t *testing.T) {
	repo := newRepoForArchiveTests(t, "task-held-direct")
	ctx := context.Background()
	setTerminalRetentionForArchiveTest(t, repo, "task-held-direct")

	err := repo.ArchiveTask(ctx, "task-held-direct")
	if !errors.Is(err, repoerrors.ErrTaskArchiveHeld) {
		t.Fatalf("ArchiveTask error = %v, want terminal retention hold", err)
	}
	task, err := repo.GetTask(ctx, "task-held-direct")
	if err != nil {
		t.Fatal(err)
	}
	if task.ArchivedAt != nil {
		t.Fatal("held task was archived")
	}
}

func TestArchiveTaskIfActiveRejectsRetentionHoldAtMutationBoundary(t *testing.T) {
	repo := newRepoForArchiveTests(t, "task-held-cascade")
	ctx := context.Background()
	setTerminalRetentionForArchiveTest(t, repo, "task-held-cascade")

	changed, err := repo.ArchiveTaskIfActive(ctx, "task-held-cascade", "cascade")
	if !errors.Is(err, repoerrors.ErrTaskArchiveHeld) {
		t.Fatalf("ArchiveTaskIfActive error = %v, want terminal retention hold", err)
	}
	if changed {
		t.Fatal("held task archive reported a mutation")
	}
	task, err := repo.GetTask(ctx, "task-held-cascade")
	if err != nil {
		t.Fatal(err)
	}
	if task.ArchivedAt != nil {
		t.Fatal("held task was archived")
	}
}

func TestArchiveTaskIfAutoArchiveEligibleRejectsRetentionHoldAtMutationBoundary(t *testing.T) {
	repo := newRepoForArchiveTests(t, "task-held-auto-archive")
	ctx := context.Background()
	setTerminalRetentionForArchiveTest(t, repo, "task-held-auto-archive")

	changed, err := repo.ArchiveTaskIfAutoArchiveEligible(ctx, "task-held-auto-archive", time.Now(), "cascade")
	if !errors.Is(err, repoerrors.ErrTaskArchiveHeld) {
		t.Fatalf("ArchiveTaskIfAutoArchiveEligible error = %v, want terminal retention hold", err)
	}
	if changed {
		t.Fatal("held task archive reported a mutation")
	}
	task, err := repo.GetTask(ctx, "task-held-auto-archive")
	if err != nil {
		t.Fatal(err)
	}
	if task.ArchivedAt != nil {
		t.Fatal("held task was auto-archived")
	}
}

func setTerminalRetentionForArchiveTest(t *testing.T, repo *Repository, taskID string) {
	t.Helper()
	task, err := repo.GetTask(context.Background(), taskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Metadata == nil {
		task.Metadata = make(map[string]interface{})
	}
	task.Metadata[models.MetaKeyTerminalRetention] = true
	if err := repo.UpdateTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
}
