package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func seedPendingActionSession(t *testing.T, repo *Repository, taskID, sessionID string) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: taskID}); err != nil {
		t.Fatalf("CreateTask(%s): %v", taskID, err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession(%s): %v", sessionID, err)
	}
}

func createPendingActionTurn(
	t *testing.T,
	repo *Repository,
	taskID, sessionID, turnID string,
	startedAt, createdAt time.Time,
) {
	t.Helper()
	if err := repo.CreateTurn(context.Background(), &models.Turn{
		ID: turnID, TaskSessionID: sessionID, TaskID: taskID,
		StartedAt: startedAt, CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("CreateTurn(%s): %v", turnID, err)
	}
}

func TestFindActiveClarificationMessagesBySessionIDUsesNewestDurableTurn(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 14, 12, 0, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-active", "session-active")
	createPendingActionTurn(t, repo, "task-active", "session-active", "turn-older", base, base)
	createPendingActionMessage(t, repo, "clarification-older", "task-active", "session-active", "turn-older", models.MessageTypeClarificationRequest, "pending", base)
	createPendingActionTurn(t, repo, "task-active", "session-active", "turn-newer", base.Add(time.Minute), base.Add(time.Minute))
	createPendingActionMessage(t, repo, "ordinary-newer", "task-active", "session-active", "turn-newer", models.MessageTypeMessage, "<missing>", base.Add(time.Minute))

	got, err := repo.FindActiveClarificationMessagesBySessionID(ctx, "session-active")
	if err != nil {
		t.Fatalf("FindActiveClarificationMessagesBySessionID: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("active clarifications = %v, want none from older turn", messageIDs(got))
	}
	actions, err := repo.GetPendingActionsBySessionIDs(ctx, []string{"session-active"})
	if err != nil {
		t.Fatalf("GetPendingActionsBySessionIDs: %v", err)
	}
	if _, ok := actions["session-active"]; ok {
		t.Fatalf("pending action reactivated from older turn: %#v", actions)
	}

	if err := repo.DeleteMessage(ctx, "ordinary-newer"); err != nil {
		t.Fatalf("DeleteMessage(newer): %v", err)
	}
	got, err = repo.FindActiveClarificationMessagesBySessionID(ctx, "session-active")
	if err != nil {
		t.Fatalf("FindActiveClarificationMessagesBySessionID after delete: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("message deletion reactivated older clarification: %v", messageIDs(got))
	}
	actions, err = repo.GetPendingActionsBySessionIDs(ctx, []string{"session-active"})
	if err != nil {
		t.Fatalf("GetPendingActionsBySessionIDs after delete: %v", err)
	}
	if _, ok := actions["session-active"]; ok {
		t.Fatalf("message deletion reactivated older pending action: %#v", actions)
	}
}

func TestFindActiveClarificationMessagesSupportsMissingStatusInCurrentTurn(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 14, 13, 0, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-status", "session-status")
	createPendingActionTurn(t, repo, "task-status", "session-status", "turn-status", base, base)
	createPendingActionMessage(t, repo, "clarification-missing", "task-status", "session-status", "turn-status", models.MessageTypeClarificationRequest, "<missing>", base)
	createPendingActionMessage(t, repo, "clarification-pending", "task-status", "session-status", "turn-status", models.MessageTypeClarificationRequest, "pending", base.Add(time.Second))
	createPendingActionMessage(t, repo, "clarification-answered", "task-status", "session-status", "turn-status", models.MessageTypeClarificationRequest, "answered", base.Add(2*time.Second))

	got, err := repo.FindActiveClarificationMessagesBySessionID(ctx, "session-status")
	if err != nil {
		t.Fatalf("FindActiveClarificationMessagesBySessionID: %v", err)
	}
	if ids := messageIDs(got); len(ids) != 2 || ids[0] != "clarification-missing" || ids[1] != "clarification-pending" {
		t.Fatalf("active clarification IDs = %v, want missing and pending current-turn rows", ids)
	}
}

func TestFindActiveClarificationMessagesUsesDeterministicTurnTieBreak(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	stamp := time.Date(2026, time.August, 14, 14, 0, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-tie", "session-tie")
	createPendingActionTurn(t, repo, "task-tie", "session-tie", "turn-a", stamp, stamp)
	createPendingActionMessage(t, repo, "clarification-tie-old", "task-tie", "session-tie", "turn-a", models.MessageTypeClarificationRequest, "pending", stamp)
	createPendingActionTurn(t, repo, "task-tie", "session-tie", "turn-z", stamp, stamp)

	got, err := repo.FindActiveClarificationMessagesBySessionID(ctx, "session-tie")
	if err != nil {
		t.Fatalf("FindActiveClarificationMessagesBySessionID: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("turn id descending tie-break did not select turn-z: %v", messageIDs(got))
	}
}
