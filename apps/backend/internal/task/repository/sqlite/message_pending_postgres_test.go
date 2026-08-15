package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresActiveClarificationUsesNewestDurableTurn pins the same ownership
// rule as SQLite. It skips unless KANDEV_TEST_POSTGRES_DSN is configured.
func TestPostgresActiveClarificationUsesNewestDurableTurn(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	base := time.Date(2026, time.August, 14, 15, 0, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-active-pg", "session-active-pg")
	createPendingActionTurn(t, repo, "task-active-pg", "session-active-pg", "turn-old-pg", base, base)
	createPendingActionMessage(t, repo, "clarification-old-pg", "task-active-pg", "session-active-pg", "turn-old-pg", models.MessageTypeClarificationRequest, "pending", base)
	createPendingActionTurn(t, repo, "task-active-pg", "session-active-pg", "turn-new-pg", base.Add(time.Minute), base.Add(time.Minute))

	assertNoActivePostgresClarification(t, ctx, repo)
	createPendingActionMessage(t, repo, "clarification-new-pg", "task-active-pg", "session-active-pg", "turn-new-pg", models.MessageTypeClarificationRequest, "<missing>", base.Add(time.Minute))
	active, err := repo.FindActiveClarificationMessagesBySessionID(ctx, "session-active-pg")
	if err != nil {
		t.Fatalf("FindActiveClarificationMessagesBySessionID: %v", err)
	}
	if ids := messageIDs(active); len(ids) != 1 || ids[0] != "clarification-new-pg" {
		t.Fatalf("postgres active clarification IDs = %v", ids)
	}
	actions, err := repo.GetPendingActionsBySessionIDs(ctx, []string{"session-active-pg"})
	if err != nil {
		t.Fatalf("GetPendingActionsBySessionIDs: %v", err)
	}
	if actions["session-active-pg"] != models.TaskPendingActionClarification {
		t.Fatalf("postgres pending action = %#v", actions)
	}

	if err := repo.DeleteMessage(ctx, "clarification-new-pg"); err != nil {
		t.Fatalf("DeleteMessage(new current-turn row): %v", err)
	}
	assertNoActivePostgresClarification(t, ctx, repo)
}

func assertNoActivePostgresClarification(t *testing.T, ctx context.Context, repo *Repository) {
	t.Helper()
	active, err := repo.FindActiveClarificationMessagesBySessionID(ctx, "session-active-pg")
	if err != nil {
		t.Fatalf("FindActiveClarificationMessagesBySessionID: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("postgres reactivated older clarification: %v", messageIDs(active))
	}
	actions, err := repo.GetPendingActionsBySessionIDs(ctx, []string{"session-active-pg"})
	if err != nil {
		t.Fatalf("GetPendingActionsBySessionIDs: %v", err)
	}
	if _, ok := actions["session-active-pg"]; ok {
		t.Fatalf("postgres reactivated older pending action: %#v", actions)
	}
}
