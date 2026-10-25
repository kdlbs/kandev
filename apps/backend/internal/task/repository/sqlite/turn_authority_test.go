package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// seedAuthorityTurn creates a turn with explicit started_at/created_at and an
// optional completed_at, so a test can control every key of D1's total order
// independently. A nil completedAt leaves the turn open.
func seedAuthorityTurn(
	t *testing.T,
	repo *Repository,
	taskID, sessionID, turnID string,
	startedAt, createdAt time.Time,
	completedAt *time.Time,
	metadata map[string]interface{},
) {
	t.Helper()
	if err := repo.CreateTurn(context.Background(), &models.Turn{
		ID: turnID, TaskID: taskID, TaskSessionID: sessionID,
		StartedAt: startedAt, CreatedAt: createdAt, CompletedAt: completedAt,
		Metadata: metadata,
	}); err != nil {
		t.Fatalf("seed authority turn %s: %v", turnID, err)
	}
}

// TestCurrentTurnAuthorityOrdering exercises D1's total order end-to-end
// through FindActiveClarificationMessagesBySessionID, which applies
// currentTurnAuthority with no extra completed_at filter (unlike
// GetActiveTurnBySessionID) so both open and completed turns compete. Each
// case seeds a pending clarification on exactly one of two candidate turns
// and asserts whether that message survives, which is only true when its
// turn resolves as current.
func TestCurrentTurnAuthorityOrdering(t *testing.T) {
	base := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	completed := base.Add(-time.Hour)
	for _, tt := range []struct {
		name          string
		winnerID      string
		winnerStarted time.Time
		winnerCreated time.Time
		winnerDone    *time.Time
		loserID       string
		loserStarted  time.Time
		loserCreated  time.Time
		loserDone     *time.Time
	}{
		{
			// AC-5/AC-6: an open turn outranks a completed one regardless of
			// which started earlier (R2). The completed turn here carries no
			// lifecycle_only marker, matching a pre-existing legacy row.
			name:          "open turn beats later completed turn",
			winnerID:      "turn-ord-open",
			winnerStarted: base,
			winnerCreated: base,
			winnerDone:    nil,
			loserID:       "turn-ord-completed-later",
			loserStarted:  base.Add(time.Hour),
			loserCreated:  base.Add(time.Hour),
			loserDone:     &completed,
		},
		{
			name:          "later started_at wins among completed turns",
			winnerID:      "turn-ord-started-later",
			winnerStarted: base.Add(time.Hour),
			winnerCreated: base,
			winnerDone:    &completed,
			loserID:       "turn-ord-started-earlier",
			loserStarted:  base,
			loserCreated:  base,
			loserDone:     &completed,
		},
		{
			name:          "tie on started_at broken by later created_at",
			winnerID:      "turn-ord-created-later",
			winnerStarted: base,
			winnerCreated: base.Add(time.Minute),
			winnerDone:    &completed,
			loserID:       "turn-ord-created-earlier",
			loserStarted:  base,
			loserCreated:  base,
			loserDone:     &completed,
		},
		{
			name:          "tie on started_at and created_at broken by higher id",
			winnerID:      "turn-ord-tie-2",
			winnerStarted: base,
			winnerCreated: base,
			winnerDone:    &completed,
			loserID:       "turn-ord-tie-1",
			loserStarted:  base,
			loserCreated:  base,
			loserDone:     &completed,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			ctx := context.Background()
			taskID := "task-" + tt.name
			sessionID := "session-" + tt.name
			seedSessionForTurns(t, repo, taskID, sessionID)

			seedAuthorityTurn(t, repo, taskID, sessionID, tt.winnerID, tt.winnerStarted, tt.winnerCreated, tt.winnerDone, nil)
			seedAuthorityTurn(t, repo, taskID, sessionID, tt.loserID, tt.loserStarted, tt.loserCreated, tt.loserDone, nil)
			insertClarificationMessage(t, repo, "msg-"+tt.winnerID, sessionID, taskID, tt.winnerID, "pending-"+tt.winnerID, "q1", "pending", 0, tt.winnerStarted)
			insertClarificationMessage(t, repo, "msg-"+tt.loserID, sessionID, taskID, tt.loserID, "pending-"+tt.loserID, "q1", "pending", 0, tt.loserStarted)

			messages, err := repo.FindActiveClarificationMessagesBySessionID(ctx, sessionID)
			if err != nil {
				t.Fatalf("FindActiveClarificationMessagesBySessionID: %v", err)
			}
			if len(messages) != 1 || messages[0].TurnID != tt.winnerID {
				t.Fatalf("messages = %+v, want exactly the winner turn %s's message", messages, tt.winnerID)
			}
		})
	}
}

// TestCurrentTurnAuthorityTwoOpenTurnsResolvesDeterministically covers D2's
// observed half: at most one open turn per session is an invariant the
// repository does not enforce with a constraint, so resolution must still
// behave deterministically (not error) if it is ever violated. With both
// candidates open, D1's first key ties and the remaining keys (started_at,
// created_at, id, all descending) pick the winner exactly as they would for
// two completed turns.
func TestCurrentTurnAuthorityTwoOpenTurnsResolvesDeterministically(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const taskID = "task-two-open-turns"
	const sessionID = "session-two-open-turns"
	seedSessionForTurns(t, repo, taskID, sessionID)
	base := time.Date(2026, time.August, 21, 9, 0, 0, 0, time.UTC)

	seedAuthorityTurn(t, repo, taskID, sessionID, "turn-open-older", base, base, nil, nil)
	seedAuthorityTurn(t, repo, taskID, sessionID, "turn-open-newer", base.Add(time.Minute), base.Add(time.Minute), nil, nil)

	active, err := repo.GetActiveTurnBySessionID(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetActiveTurnBySessionID: %v", err)
	}
	if active == nil || active.ID != "turn-open-newer" {
		t.Fatalf("active turn = %#v, want turn-open-newer (later started_at wins deterministically)", active)
	}
}

func TestTurnAuthorityToleratesStringFlagEncodings(t *testing.T) {
	for _, tt := range []struct {
		name          string
		pending       interface{}
		attempted     interface{}
		wantCurrentID string
	}{
		{name: "pending string true", pending: "true", wantCurrentID: "turn-previous"},
		{name: "pending string one", pending: "1", wantCurrentID: "turn-previous"},
		{name: "attempted string true", pending: true, attempted: "true", wantCurrentID: "turn-reserved"},
		{name: "attempted string one", pending: true, attempted: "1", wantCurrentID: "turn-reserved"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			ctx := context.Background()
			const taskID = "task-string-flags"
			const sessionID = "session-string-flags"
			seedSessionForTurns(t, repo, taskID, sessionID)
			base := time.Date(2026, time.August, 15, 22, 0, 0, 0, time.UTC)
			createRecoveryTurn(t, repo, taskID, sessionID, "turn-previous", base, nil)
			createRecoveryTurn(t, repo, taskID, sessionID, "turn-reserved", base.Add(time.Minute), map[string]interface{}{
				models.TurnMetaKeyPromptDispatchPending:   tt.pending,
				models.TurnMetaKeyPromptDispatchAttempted: tt.attempted,
			})

			active, err := repo.GetActiveTurnBySessionID(ctx, sessionID)
			if err != nil {
				t.Fatalf("GetActiveTurnBySessionID: %v", err)
			}
			if active == nil || active.ID != tt.wantCurrentID {
				t.Fatalf("active turn = %#v, want %s", active, tt.wantCurrentID)
			}
		})
	}
}
