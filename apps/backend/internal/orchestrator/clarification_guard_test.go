package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestSessionHasPendingClarification(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	if svc.sessionHasPendingClarification(ctx, "s1") {
		t.Fatal("expected no pending clarification")
	}

	now := time.Now().UTC()
	requireNoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "turn-1", TaskSessionID: "s1", TaskID: "t1", StartedAt: now}))
	requireNoError(t, repo.CreateMessage(ctx, &models.Message{
		ID:            "clarify-1",
		TaskSessionID: "s1",
		TaskID:        "t1",
		TurnID:        "turn-1",
		AuthorType:    models.MessageAuthorAgent,
		Type:          "clarification_request",
		Content:       "Q?",
		CreatedAt:     now,
		Metadata: map[string]interface{}{
			"pending_id": "pending-1",
			"status":     "pending",
		},
	}))

	if !svc.sessionHasPendingClarification(ctx, "s1") {
		t.Fatal("expected pending clarification")
	}
}

func seedPendingClarificationMessage(t *testing.T, repo interface {
	CreateTurn(ctx context.Context, turn *models.Turn) error
	CreateMessage(ctx context.Context, message *models.Message) error
}, taskID, sessionID string) {
	t.Helper()
	now := time.Now().UTC()
	turnID := "turn-clarification-" + sessionID
	requireNoError(t, repo.CreateTurn(context.Background(), &models.Turn{
		ID:            turnID,
		TaskSessionID: sessionID,
		TaskID:        taskID,
		StartedAt:     now,
	}))
	requireNoError(t, repo.CreateMessage(context.Background(), &models.Message{
		ID:            "clarification-" + sessionID,
		TaskSessionID: sessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorAgent,
		Type:          "clarification_request",
		Content:       "Which approach?",
		CreatedAt:     now,
		Metadata: map[string]interface{}{
			"pending_id":  "pending-" + sessionID,
			"question_id": "q1",
			"status":      "pending",
		},
	}))
}
