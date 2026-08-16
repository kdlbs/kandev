package sqlite

import (
	"context"
	"testing"
)

func TestNextPendingActionProjectionEpochPersistsGenerationOrder(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()

	first, err := repo.NextPendingActionProjectionEpoch(ctx)
	if err != nil {
		t.Fatalf("allocate first epoch: %v", err)
	}
	second, err := repo.NextPendingActionProjectionEpoch(ctx)
	if err != nil {
		t.Fatalf("allocate second epoch: %v", err)
	}

	if first != 1 || second != 2 {
		t.Fatalf("allocated epochs = %d, %d; want 1, 2", first, second)
	}
	var stored string
	if err := repo.db.QueryRowContext(
		ctx,
		repo.db.Rebind(`SELECT value FROM kandev_meta WHERE key = ?`),
		pendingActionProjectionEpochMetaKey,
	).Scan(&stored); err != nil {
		t.Fatalf("read stored epoch: %v", err)
	}
	if stored != "2" {
		t.Fatalf("stored epoch = %q, want 2", stored)
	}
}

func TestNextPendingActionProjectionEpochRejectsCorruptGeneration(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	if _, err := repo.db.ExecContext(
		ctx,
		repo.db.Rebind(`INSERT INTO kandev_meta (key, value) VALUES (?, ?)`),
		pendingActionProjectionEpochMetaKey,
		"not-a-generation",
	); err != nil {
		t.Fatalf("seed corrupt epoch: %v", err)
	}

	if _, err := repo.NextPendingActionProjectionEpoch(ctx); err == nil {
		t.Fatal("corrupt epoch was silently reset")
	}
}
