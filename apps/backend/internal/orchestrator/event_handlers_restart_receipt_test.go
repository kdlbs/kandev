package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestStoreRestartReceipt_WritesKindIntoMetadata verifies the receipt classification
// reaches TaskSession.Metadata under SessionMetaKeyRestartKind. The UI reads the
// session payload, which already serialises Metadata, so persisting here is
// what makes the receipt visible — no new WS action needed.
func TestStoreRestartReceipt_WritesKindIntoMetadata(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	svc.storeRestartReceipt(ctx, "s1", models.RestartKindResumed)

	sess, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	kind, _ := sess.Metadata[models.SessionMetaKeyRestartKind].(string)
	if kind != models.RestartKindResumed {
		t.Errorf("expected %s=%q, got %q",
			models.SessionMetaKeyRestartKind, models.RestartKindResumed, kind)
	}
}

// TestStoreRestartReceipt_EmptyKindSkipsWrite verifies that an empty kind
// (legacy event sources, Mock provider) doesn't overwrite a previously-recorded
// receipt — important on backend restart where the first classified event
// shouldn't be clobbered by a follow-up unclassified one.
func TestStoreRestartReceipt_EmptyKindSkipsWrite(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	// Seed a real receipt first.
	svc.storeRestartReceipt(ctx, "s1", models.RestartKindFresh)
	// Then call with empty kind — must be a no-op.
	svc.storeRestartReceipt(ctx, "s1", "")

	sess, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if kind, _ := sess.Metadata[models.SessionMetaKeyRestartKind].(string); kind != models.RestartKindFresh {
		t.Errorf("expected receipt preserved as %q, got %q", models.RestartKindFresh, kind)
	}
}

// TestStoreRestartReceipt_LatestKindWins verifies that a subsequent classified
// call overwrites the receipt — the receipt reflects the *most recent*
// restart, not the original one.
func TestStoreRestartReceipt_LatestKindWins(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	svc.storeRestartReceipt(ctx, "s1", models.RestartKindResumed)
	svc.storeRestartReceipt(ctx, "s1", models.RestartKindFresh)

	sess, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if kind, _ := sess.Metadata[models.SessionMetaKeyRestartKind].(string); kind != models.RestartKindFresh {
		t.Errorf("expected latest kind to win (%q), got %q", models.RestartKindFresh, kind)
	}
}
