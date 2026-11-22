package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestCurrentClarificationTurnIDUsesClaimAuthority(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const taskID = "task-turn-authority"
	const sessionID = "session-turn-authority"
	seedPendingActionSession(t, repo, taskID, sessionID)
	base := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	completedAt := base.Add(30 * time.Second)
	turns := []*models.Turn{
		{
			ID: "turn-open", TaskSessionID: sessionID, TaskID: taskID,
			StartedAt: base, CreatedAt: base,
		},
		{
			ID: "turn-completed", TaskSessionID: sessionID, TaskID: taskID,
			StartedAt: base.Add(10 * time.Second), CreatedAt: base.Add(10 * time.Second),
			CompletedAt: &completedAt,
		},
		{
			ID: "turn-lifecycle", TaskSessionID: sessionID, TaskID: taskID,
			StartedAt: base.Add(20 * time.Second), CreatedAt: base.Add(20 * time.Second),
			Metadata: map[string]interface{}{models.TurnMetaKeyLifecycleOnly: true},
		},
	}
	for _, turn := range turns {
		if err := repo.CreateTurn(ctx, turn); err != nil {
			t.Fatalf("CreateTurn(%s): %v", turn.ID, err)
		}
	}
	got, err := repo.CurrentClarificationTurnID(ctx, sessionID)
	if err != nil {
		t.Fatalf("CurrentClarificationTurnID: %v", err)
	}
	if got != "turn-open" {
		t.Fatalf("current turn = %q, want open conversational turn", got)
	}

	newOpen := &models.Turn{
		ID: "turn-new-open", TaskSessionID: sessionID, TaskID: taskID,
		StartedAt: base.Add(40 * time.Second), CreatedAt: base.Add(40 * time.Second),
	}
	if err := repo.CreateTurn(ctx, newOpen); err != nil {
		t.Fatalf("CreateTurn(new open): %v", err)
	}
	got, err = repo.CurrentClarificationTurnID(ctx, sessionID)
	if err != nil {
		t.Fatalf("CurrentClarificationTurnID after successor: %v", err)
	}
	if got != "turn-new-open" {
		t.Fatalf("current turn = %q, want newest open conversational turn", got)
	}
}
