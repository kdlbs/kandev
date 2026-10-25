package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// TestListTurnsBySessionKeepsLifecycleTurnVisible is AC-11's predicate half:
// turnHistoryPredicate SHALL keep including lifecycle turns in visible
// history, so the boot message stays grouped under its own turn in the
// transcript, even though currentTurnAuthority now excludes that same turn
// from current-turn resolution (AC-2/AC-3). GET .../turns
// (internal/task/handlers/task_http_handlers.go) calls ListTurnsBySession
// directly and DTO-converts every row it returns with no further filtering,
// so a lifecycle turn present here is a lifecycle turn present in that
// response.
func TestListTurnsBySessionKeepsLifecycleTurnVisible(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedSessionForTurns(t, repo, "task-turn-history-lifecycle", "session-turn-history-lifecycle")

	base := time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
	lifecycleTurn := &models.Turn{
		ID:            "turn-lifecycle-history",
		TaskSessionID: "session-turn-history-lifecycle",
		TaskID:        "task-turn-history-lifecycle",
		StartedAt:     base,
		CreatedAt:     base,
		CompletedAt:   &base,
		Metadata:      map[string]interface{}{models.TurnMetaKeyLifecycleOnly: true},
	}
	conversationalTurn := &models.Turn{
		ID:            "turn-conversational-history",
		TaskSessionID: "session-turn-history-lifecycle",
		TaskID:        "task-turn-history-lifecycle",
		StartedAt:     base.Add(time.Minute),
		CreatedAt:     base.Add(time.Minute),
	}
	for _, turn := range []*models.Turn{lifecycleTurn, conversationalTurn} {
		if err := repo.CreateTurn(ctx, turn); err != nil {
			t.Fatalf("CreateTurn(%s): %v", turn.ID, err)
		}
	}

	turns, err := repo.ListTurnsBySession(ctx, "session-turn-history-lifecycle")
	if err != nil {
		t.Fatalf("ListTurnsBySession: %v", err)
	}

	ids := make(map[string]*models.Turn, len(turns))
	for _, turn := range turns {
		ids[turn.ID] = turn
	}
	if len(turns) != 2 {
		t.Fatalf("ListTurnsBySession returned %d turns %+v, want both the lifecycle and conversational turn", len(turns), turns)
	}
	got, ok := ids[lifecycleTurn.ID]
	if !ok {
		t.Fatalf("ListTurnsBySession dropped the lifecycle turn %q; turnHistoryPredicate must keep it visible", lifecycleTurn.ID)
	}
	if lifecycleOnly, ok := got.Metadata[models.TurnMetaKeyLifecycleOnly]; !ok || lifecycleOnly != true {
		t.Fatalf("returned lifecycle turn metadata[%q] = %v, want true", models.TurnMetaKeyLifecycleOnly, lifecycleOnly)
	}
	if _, ok := ids[conversationalTurn.ID]; !ok {
		t.Fatalf("ListTurnsBySession dropped the conversational turn %q", conversationalTurn.ID)
	}
}
