package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresListUnresolvedClarificationBundlesEarlierTurns proves the
// earlier-turn scope's COALESCE turn comparison and explicit-pending test on
// the PostgreSQL dialect, where the JSON extraction differs from SQLite.
func TestPostgresListUnresolvedClarificationBundlesEarlierTurns(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	seedEarlierTurnFixture(t, repo, time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond))

	page, err := repo.ListUnresolvedClarificationBundles(context.Background(), earlierTurnOpts("ws-E"))
	if err != nil {
		t.Fatalf("earlier-turn list: %v", err)
	}
	if got := pendingIDs(page.Bundles); len(got) != 1 || got[0] != "pending-E1-old" {
		t.Fatalf("earlier-turn bundles = %v, want only pending-E1-old", got)
	}
}

// TestPostgresListOpenWorkspaceRecoveryBlocks proves the recovery list's
// generation subquery and timestamp scan on the PostgreSQL dialect.
func TestPostgresListOpenWorkspaceRecoveryBlocks(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	inc := seedRecoverySession(t, repo, "task-PR1", "sess-PR1", "ws-PR", models.TaskSessionStateWaitingForInput)
	seedRecoveryBlock(t, repo, "blk-pg-current", "sess-PR1", inc, 2, models.RecoveryBlockOpen, base)
	seedRecoveryBlock(t, repo, "blk-pg-old", "sess-PR1", inc, 1, models.RecoveryBlockOpen, base)

	blocks, hasMore, err := repo.ListOpenWorkspaceRecoveryBlocks(context.Background(), "ws-PR", 50)
	if err != nil {
		t.Fatalf("ListOpenWorkspaceRecoveryBlocks: %v", err)
	}
	if hasMore || len(blocks) != 1 || blocks[0].BlockID != "blk-pg-current" || !blocks[0].CreatedAt.Equal(base) {
		t.Fatalf("blocks = %+v (hasMore=%v), want blk-pg-current at %v", blocks, hasMore, base)
	}
}
