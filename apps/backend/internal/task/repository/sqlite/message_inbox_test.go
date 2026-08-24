package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
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

func TestListTaskInboxMessagesUsesTaskScopedOrderingIndex(t *testing.T) {
	repo := newRepoForSessionTests(t)
	normalizedCreatedAt := dialect.NormalizedMicrosecond(dialect.SQLite3, "created_at")
	rows, err := repo.ro.QueryxContext(context.Background(), repo.ro.Rebind(`
		EXPLAIN QUERY PLAN
		SELECT id
		FROM task_session_messages
		WHERE task_id = ? AND author_type = ?
		ORDER BY `+normalizedCreatedAt+` ASC, id ASC
		LIMIT ?
	`), "task-inbox", string(models.MessageAuthorUser), 1)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	var details []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &notUsed, &detail))
		details = append(details, detail)
	}
	require.NoError(t, rows.Err())
	require.NotEmpty(t, details)
	require.True(t, slicesContain(details, "idx_messages_task_inbox_order"), "query plan: %v", details)
	for _, detail := range details {
		require.False(t, strings.Contains(detail, "USE TEMP B-TREE FOR ORDER BY"), "query plan: %v", details)
	}
}

func slicesContain(values []string, wanted string) bool {
	for _, value := range values {
		if strings.Contains(value, wanted) {
			return true
		}
	}
	return false
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
