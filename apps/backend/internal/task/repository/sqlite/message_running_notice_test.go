package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestLastAgentActivityForTurnSQLite(t *testing.T) {
	testLastAgentActivityForTurn(t, newRepoForSessionTests(t))
}

func TestLastAgentActivityForTurnPostgres(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	testLastAgentActivityForTurn(t, repo)
}

func testLastAgentActivityForTurn(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	base := time.Date(2026, 9, 30, 12, 0, 0, 123456000, time.UTC)
	seedPendingActionSession(t, repo, "task-activity", "session-activity")
	createPendingActionTurn(t, repo, "task-activity", "session-activity", "turn-activity", base, base)
	seedPendingActionSession(t, repo, "task-other", "session-other")
	createPendingActionTurn(t, repo, "task-other", "session-other", "turn-other", base, base)
	assertActivity := func(sessionID, turnID string, want time.Time) {
		t.Helper()
		got, err := repo.GetLastAgentActivityForTurn(ctx, sessionID, turnID)
		if err != nil || !got.Equal(want) {
			t.Fatalf("activity for %s/%s = %v, want %v, err=%v", sessionID, turnID, got, want, err)
		}
	}
	assertActivity("session-activity", "turn-activity", time.Time{})
	ignored := []struct {
		author models.MessageAuthorType
		kind   models.MessageType
	}{
		{models.MessageAuthorUser, models.MessageTypeMessage},
		{models.MessageAuthorAgent, models.MessageTypeStatus},
		{models.MessageAuthorAgent, models.MessageTypeError},
		{models.MessageAuthorAgent, models.MessageTypeLog},
	}
	for i, item := range ignored {
		message := &models.Message{
			ID: fmt.Sprintf("ignored-%d", i), TaskID: "task-activity", TaskSessionID: "session-activity",
			TurnID: "turn-activity", AuthorType: item.author, Type: item.kind,
			CreatedAt: base.Add(time.Hour), UpdatedAt: base.Add(time.Hour),
		}
		if err := repo.CreateMessage(ctx, message); err != nil {
			t.Fatal(err)
		}
	}
	assertActivity("session-activity", "turn-activity", time.Time{})
	types := []models.MessageType{
		models.MessageTypeMessage, models.MessageTypeContent, models.MessageTypeThinking,
		models.MessageTypeToolCall, models.MessageTypeToolRead, models.MessageTypeToolEdit,
		models.MessageTypeToolExecute, models.MessageTypeToolSearch, models.MessageTypeAgentPlan,
		models.MessageTypeTodo, models.MessageTypePermissionRequest,
	}
	for i, kind := range types {
		updated := base.Add(time.Duration(i+1) * time.Second)
		if err := repo.CreateMessage(ctx, &models.Message{
			ID: fmt.Sprintf("activity-%d", i), TaskID: "task-activity", TaskSessionID: "session-activity",
			TurnID: "turn-activity", AuthorType: models.MessageAuthorAgent, Type: kind,
			CreatedAt: base, UpdatedAt: updated,
		}); err != nil {
			t.Fatal(err)
		}
		assertActivity("session-activity", "turn-activity", updated)
	}
	assertActivity("session-other", "turn-activity", time.Time{})
	assertActivity("session-activity", "turn-other", time.Time{})
	created := base.Add(time.Minute)
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: "new-activity", TaskID: "task-activity", TaskSessionID: "session-activity",
		TurnID: "turn-activity", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeContent,
		CreatedAt: created, UpdatedAt: base,
	}); err != nil {
		t.Fatal(err)
	}
	assertActivity("session-activity", "turn-activity", created)
}
