package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// turnResolutionFixture mirrors
// testdata/current_turn_resolution.json (AC-10). It is the shared contract
// between this Go repository test and the TypeScript unit test that loads
// the same file from apps/web/lib/utils/pending-clarification-turn-authority.test.ts
// (or a sibling file it imports) - one artifact both implementations are
// proven to agree with, not two independently-trusted ones.
type turnResolutionFixture struct {
	Cases []turnResolutionCase `json:"cases"`
}

type turnResolutionCase struct {
	Name                  string               `json:"name"`
	Turns                 []turnResolutionTurn `json:"turns"`
	ExpectedCurrentTurnID *string              `json:"expected_current_turn_id"`
}

type turnResolutionTurn struct {
	ID          string                 `json:"id"`
	StartedAt   time.Time              `json:"started_at"`
	CreatedAt   time.Time              `json:"created_at"`
	CompletedAt *time.Time             `json:"completed_at"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// loadTurnResolutionFixture SHALL fail the test (not skip) when the fixture
// is missing, unreadable, or unparseable - a fixture that silently stops
// being read is indistinguishable from two implementations that agree.
func loadTurnResolutionFixture(t *testing.T) turnResolutionFixture {
	t.Helper()
	raw, err := os.ReadFile("testdata/current_turn_resolution.json")
	if err != nil {
		t.Fatalf("read current_turn_resolution.json: %v", err)
	}
	var fixture turnResolutionFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("parse current_turn_resolution.json: %v", err)
	}
	if len(fixture.Cases) == 0 {
		t.Fatal("current_turn_resolution.json has no cases")
	}
	return fixture
}

// resolveCurrentTurnID runs currentTurnAuthority's predicate/orderBy directly
// against the turns table, the same CTE shape
// FindActiveClarificationMessagesBySessionID uses internally, without its
// join to messages - so it resolves a session's current turn regardless of
// whether any message references it. Returns nil when no turn resolves.
func resolveCurrentTurnID(t *testing.T, repo *Repository, sessionID string) *string {
	t.Helper()
	predicate, orderBy := currentTurnAuthority(repo.ro.DriverName(), "turn_row")
	query := fmt.Sprintf(`
		SELECT turn_row.id
		FROM task_session_turns turn_row
		WHERE turn_row.task_session_id = ?
		  AND %s
		ORDER BY %s
		LIMIT 1
	`, predicate, orderBy)
	row := repo.ro.QueryRowContext(context.Background(), repo.ro.Rebind(query), sessionID)
	var id string
	switch err := row.Scan(&id); {
	case errors.Is(err, sql.ErrNoRows):
		return nil
	case err != nil:
		t.Fatalf("resolve current turn for session %s: %v", sessionID, err)
		return nil
	default:
		return &id
	}
}

// TestCurrentTurnResolutionFixture is AC-10: every case in the shared
// fixture, run against the real Go currentTurnAuthority resolution. The
// TypeScript counterpart runs the identical cases through
// newestDurableTurnId. Neither test may filter the shared cases; each may
// add its own.
func TestCurrentTurnResolutionFixture(t *testing.T) {
	fixture := loadTurnResolutionFixture(t)
	for i, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			ctx := context.Background()
			taskID := fmt.Sprintf("task-fixture-%d", i)
			sessionID := fmt.Sprintf("session-fixture-%d", i)
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
