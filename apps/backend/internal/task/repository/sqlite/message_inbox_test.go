package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestListTaskInboxMessagesScopesAuthorsAndPaginatesByTask(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-inbox", "session-a", "turn-a")
	seedForMsgTest(t, repo, "task-inbox", "session-b", "turn-b")
	seedForMsgTest(t, repo, "task-other", "session-other", "turn-other")

	base := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	createInboxMessage(t, repo, "m1", "task-inbox", "session-a", "turn-a", models.MessageAuthorUser, base)
	createInboxMessage(t, repo, "agent", "task-inbox", "session-a", "turn-a", models.MessageAuthorAgent, base.Add(time.Microsecond))
	createInboxMessage(t, repo, "m2", "task-inbox", "session-b", "turn-b", models.MessageAuthorUser, base.Add(2*time.Microsecond))
	createInboxMessage(t, repo, "m3", "task-inbox", "session-a", "turn-a", models.MessageAuthorUser, base.Add(3*time.Microsecond))
	createInboxMessage(t, repo, "foreign", "task-other", "session-other", "turn-other", models.MessageAuthorUser, base.Add(4*time.Microsecond))

	page, hasMore, counts, err := repo.ListTaskInboxMessages(ctx, "task-inbox", models.TaskInboxMessagesOptions{Limit: 2})
	require.NoError(t, err)
	require.True(t, hasMore)
	require.Equal(t, []string{"m1", "m2"}, inboxMessageIDs(page))
	require.Equal(t, map[string]int{"session-a": 2, "session-b": 1}, counts)

	page, hasMore, counts, err = repo.ListTaskInboxMessages(ctx, "task-inbox", models.TaskInboxMessagesOptions{
		Limit:          2,
		AfterCreatedAt: page[len(page)-1].CreatedAt,
		AfterID:        page[len(page)-1].ID,
	})
	require.NoError(t, err)
	require.False(t, hasMore)
	require.Equal(t, []string{"m3"}, inboxMessageIDs(page))
	require.Equal(t, map[string]int{"session-a": 2, "session-b": 1}, counts)
}

func createInboxMessage(t *testing.T, repo *Repository, id, taskID, sessionID, turnID string, author models.MessageAuthorType, createdAt time.Time) {
	t.Helper()
	require.NoError(t, repo.CreateMessage(context.Background(), &models.Message{
		ID:            id,
		TaskID:        taskID,
		TaskSessionID: sessionID,
		TurnID:        turnID,
		AuthorType:    author,
		Content:       id,
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
	}))
}

func inboxMessageIDs(messages []*models.Message) []string {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	return ids
}
