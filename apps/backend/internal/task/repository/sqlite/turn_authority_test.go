package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

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
