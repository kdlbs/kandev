package sqlite

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestCurrentTurnResolutionFixturePostgres is AC-10/D9: the same shared
// fixture TestCurrentTurnResolutionFixture proves against SQLite, run
// against PostgreSQL so D1's key 1 (open beats completed) and the
// lifecycle_only exclusion resolve identically on both dialects. Skips
// unless KANDEV_TEST_POSTGRES_DSN is set. Neither this test nor the SQLite
// one may filter the shared cases; each may add its own.
func TestCurrentTurnResolutionFixturePostgres(t *testing.T) {
	fixture := loadTurnResolutionFixture(t)
	for i, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
			repo, err := NewWithDB(db, db, nil)
			if err != nil {
				t.Fatalf("init postgres schema: %v", err)
			}
			ctx := context.Background()
			taskID := fmt.Sprintf("task-fixture-pg-%d", i)
			sessionID := fmt.Sprintf("session-fixture-pg-%d", i)
			seedSessionForTurns(t, repo, taskID, sessionID)

			for _, turn := range tc.Turns {
				if err := repo.CreateTurn(ctx, &models.Turn{
					ID:            turn.ID,
					TaskSessionID: sessionID,
					TaskID:        taskID,
					StartedAt:     turn.StartedAt,
					CreatedAt:     turn.CreatedAt,
					CompletedAt:   turn.CompletedAt,
					Metadata:      turn.Metadata,
				}); err != nil {
					t.Fatalf("seed fixture turn %s: %v", turn.ID, err)
				}
			}

			got := resolveCurrentTurnID(t, repo, sessionID)
			switch {
			case tc.ExpectedCurrentTurnID == nil && got != nil:
				t.Fatalf("current turn = %s, want none to resolve", *got)
			case tc.ExpectedCurrentTurnID != nil && got == nil:
				t.Fatalf("current turn = none, want %s", *tc.ExpectedCurrentTurnID)
			case tc.ExpectedCurrentTurnID != nil && got != nil && *got != *tc.ExpectedCurrentTurnID:
				t.Fatalf("current turn = %s, want %s", *got, *tc.ExpectedCurrentTurnID)
			}
		})
	}
}
