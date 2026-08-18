package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/testutil"
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
	} else if !strings.Contains(err.Error(), "canonical positive integer") {
		t.Fatalf("corrupt epoch error = %q, want diagnostic metadata cause", err)
	}
}

func TestPostgresNextPendingActionProjectionEpochPreservesMonotonicCorruptValue(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	first, err := repo.NextPendingActionProjectionEpoch(ctx)
	if err != nil {
		t.Fatalf("allocate first postgres epoch: %v", err)
	}
	second, err := repo.NextPendingActionProjectionEpoch(ctx)
	if err != nil {
		t.Fatalf("allocate second postgres epoch: %v", err)
	}
	if first != 1 || second != 2 {
		t.Fatalf("allocated postgres epochs = %d, %d; want 1, 2", first, second)
	}

	const corrupt = "not-a-generation"
	if _, err := db.ExecContext(
		ctx,
		db.Rebind(`UPDATE kandev_meta SET value = ? WHERE key = ?`),
		corrupt,
		pendingActionProjectionEpochMetaKey,
	); err != nil {
		t.Fatalf("seed corrupt postgres epoch: %v", err)
	}
	if _, err := repo.NextPendingActionProjectionEpoch(ctx); err == nil {
		t.Fatal("corrupt postgres epoch was silently reset")
	}
	var stored string
	if err := db.QueryRowContext(
		ctx,
		db.Rebind(`SELECT value FROM kandev_meta WHERE key = ?`),
		pendingActionProjectionEpochMetaKey,
	).Scan(&stored); err != nil {
		t.Fatalf("read corrupt postgres epoch: %v", err)
	}
	if stored != corrupt {
		t.Fatalf("stored postgres epoch = %q, want unchanged %q", stored, corrupt)
	}
}
