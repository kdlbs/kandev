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

func TestSessionHasPendingClarificationIgnoresOlderDurableTurn(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-current-turn", "session-current-turn", "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	base := time.Date(2026, time.August, 14, 16, 0, 0, 0, time.UTC)
	requireNoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-old", TaskSessionID: "session-current-turn", TaskID: "task-current-turn",
		StartedAt: base, CreatedAt: base,
	}))
	requireNoError(t, repo.CreateMessage(ctx, &models.Message{
		ID: "clarification-old", TaskSessionID: "session-current-turn", TaskID: "task-current-turn",
		TurnID: "turn-old", AuthorType: models.MessageAuthorAgent,
		Type: models.MessageTypeClarificationRequest, Content: "old", CreatedAt: base,
		Metadata: map[string]any{"pending_id": "pending-old", "status": "pending"},
	}))
	requireNoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-new", TaskSessionID: "session-current-turn", TaskID: "task-current-turn",
		StartedAt: base.Add(time.Minute), CreatedAt: base.Add(time.Minute),
	}))

	if svc.sessionHasPendingClarification(ctx, "session-current-turn") {
		t.Fatal("older-turn clarification blocked turn completion")
	}
}

func TestSessionHasPendingClarificationFailsClosedOnRepositoryError(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-error", "session-error", "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !svc.sessionHasPendingClarification(ctx, "session-error") {
		t.Fatal("repository error opened the workflow barrier")
	}
}

func TestSessionHasLiveClarificationIgnoresDetachedBundle(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-detached", "session-detached", "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	now := time.Now().UTC()
	requireNoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-detached", TaskSessionID: "session-detached", TaskID: "task-detached",
		StartedAt: now, CreatedAt: now,
	}))
	requireNoError(t, repo.CreateMessage(ctx, &models.Message{
		ID: "clarification-detached", TaskSessionID: "session-detached", TaskID: "task-detached",
		TurnID: "turn-detached", AuthorType: models.MessageAuthorAgent,
		Type: models.MessageTypeClarificationRequest, Content: "old question", CreatedAt: now,
		Metadata: map[string]any{
			"pending_id": "pending-detached", "status": "pending", "agent_disconnected": true,
		},
	}))

	if !svc.sessionHasPendingClarification(ctx, "session-detached") {
		t.Fatal("detached clarification must remain a pending answer barrier")
	}
	if svc.sessionHasLiveClarification(ctx, "session-detached") {
		t.Fatal("detached-only clarification must not block a successor queue drain")
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
