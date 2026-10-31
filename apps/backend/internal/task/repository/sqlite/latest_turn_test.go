package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestGetLatestTurnBySessionIDMatchesHistoryOrder(t *testing.T) {
	assertLatestTurnMatchesHistory(t, newRepoForSessionTests(t), "sqlite")
}

func TestPostgresGetLatestTurnBySessionIDMatchesHistoryOrder(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	assertLatestTurnMatchesHistory(t, repo, "postgres")
}

// assertLatestTurnMatchesHistory checks that the single-row latest-turn query
// picks the same turn as the last entry of ListTurnsBySession, including a
// lifecycle turn and a tie on started_at broken by created_at.
func assertLatestTurnMatchesHistory(t *testing.T, repo *Repository, prefix string) {
	t.Helper()
	ctx := context.Background()
	taskID, sessionID := prefix+"-task-latest-turn", prefix+"-session-latest-turn"
	seedSessionForTurns(t, repo, taskID, sessionID)

	empty, err := repo.GetLatestTurnBySessionID(ctx, sessionID)
	if err != nil || empty != nil {
		t.Fatalf("GetLatestTurnBySessionID on a session without turns = %+v, %v; want nil, nil", empty, err)
	}

	base := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	turns := []*models.Turn{
		{ID: prefix + "-turn-first", StartedAt: base, CreatedAt: base},
		{
			ID: prefix + "-turn-lifecycle", StartedAt: base.Add(time.Minute), CreatedAt: base.Add(time.Minute),
			Metadata: map[string]interface{}{models.TurnMetaKeyLifecycleOnly: true},
		},
		{ID: prefix + "-turn-tie-early", StartedAt: base.Add(2 * time.Minute), CreatedAt: base.Add(2 * time.Minute)},
		{ID: prefix + "-turn-tie-late", StartedAt: base.Add(2 * time.Minute), CreatedAt: base.Add(3 * time.Minute)},
	}
	for _, turn := range turns {
		turn.TaskSessionID, turn.TaskID = sessionID, taskID
		if err := repo.CreateTurn(ctx, turn); err != nil {
			t.Fatalf("CreateTurn(%s): %v", turn.ID, err)
		}
	}

	history, err := repo.ListTurnsBySession(ctx, sessionID)
	if err != nil || len(history) == 0 {
		t.Fatalf("ListTurnsBySession = %d turns, %v", len(history), err)
	}
	latest, err := repo.GetLatestTurnBySessionID(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetLatestTurnBySessionID: %v", err)
	}
	want := history[len(history)-1].ID
	if latest == nil || latest.ID != want || want != prefix+"-turn-tie-late" {
		t.Fatalf("latest turn = %+v, want %q (last of history)", latest, want)
	}
}
