package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestPostgresTaskInboxOrderIndexAndPagination(t *testing.T) {
	repo := openPostgresRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	seedPostgresSession(t, repo, "task-inbox-pg", "session-inbox-pg", "turn-inbox-pg", base)

	createInboxMessage(t, repo, "pg-m1", "task-inbox-pg", "session-inbox-pg", "turn-inbox-pg", models.MessageAuthorUser, base)
	createInboxMessage(t, repo, "pg-m2", "task-inbox-pg", "session-inbox-pg", "turn-inbox-pg", models.MessageAuthorUser, base.Add(time.Microsecond))

	var indexExists bool
	require.NoError(t, repo.ro.Get(&indexExists, `
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE schemaname = current_schema()
			  AND indexname = 'idx_messages_task_inbox_order'
		)
	`))
	require.True(t, indexExists)

	page, hasMore, counts, err := repo.ListTaskInboxMessages(ctx, "task-inbox-pg", models.TaskInboxMessagesOptions{Limit: 1})
	require.NoError(t, err)
	require.True(t, hasMore)
	require.Equal(t, []string{"pg-m1"}, inboxMessageIDs(page))
	require.Equal(t, map[string]int{"session-inbox-pg": 2}, counts)

	page, hasMore, _, err = repo.ListTaskInboxMessages(ctx, "task-inbox-pg", models.TaskInboxMessagesOptions{
		Limit:          1,
		AfterCreatedAt: page[0].CreatedAt,
		AfterID:        page[0].ID,
	})
	require.NoError(t, err)
	require.False(t, hasMore)
	require.Equal(t, []string{"pg-m2"}, inboxMessageIDs(page))
}
