package sqlite

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestConversationSearchBackfillEligibility(t *testing.T) {
	testConversationBackfillEligibility(t, newConversationRepo(t))
}

func TestConversationSearchPostgresBackfillEligibility(t *testing.T) {
	testConversationBackfillEligibility(t, openPostgresRepo(t))
}

func testConversationBackfillEligibility(t *testing.T, r *Repository) {
	t.Helper()
	seedWorkspace(t, r, "history-ws")
	require.NoError(t, r.CreateTask(t.Context(), &models.Task{ID: "history-task", WorkspaceID: "history-ws", Title: "History"}))
	require.NoError(t, r.CreateTaskSession(t.Context(), &models.TaskSession{ID: "history-session", TaskID: "history-task"}))
	require.NoError(t, r.CreateTurn(t.Context(), &models.Turn{ID: "history-turn", TaskSessionID: "history-session", TaskID: "history-task"}))
	for _, m := range []struct{ id, kind, author, content string }{
		{"a-tool", "tool_call", "agent", strings.Repeat("tool payload ", 30000)},
		{"b-user", "message", "user", "eligible user phrase"},
		{"c-system", "message", "system", "internal system event"},
		{"d-agent", "message", "agent", "eligible agent phrase"},
	} {
		require.NoError(t, r.CreateMessage(t.Context(), &models.Message{ID: m.id, TaskSessionID: "history-session", TurnID: "history-turn", Type: models.MessageType(m.kind), AuthorType: models.MessageAuthorType(m.author), Content: m.content}))
	}
	_, err := r.db.Exec(`DELETE FROM task_conversation_dirty`)
	require.NoError(t, err)
	_, err = r.db.Exec(`UPDATE task_conversation_index_state SET upper_id='zzzz',cursor_id='',complete=0`)
	require.NoError(t, err)
	for _, query := range []string{"eligible user", "eligible agent"} {
		done, err := r.BackfillConversationSearch(t.Context(), 1)
		require.NoError(t, err)
		require.False(t, done)
		_, err = r.ProcessConversationSearchBatch(t.Context(), true)
		require.NoError(t, err)
		page, err := r.SearchTaskConversations(t.Context(), "history-ws", query, 20)
		require.NoError(t, err)
		require.Len(t, page.Hits, 1, "excluded records must not consume an eligible backfill page")
	}
	done, err := r.BackfillConversationSearch(t.Context(), 1)
	require.NoError(t, err)
	require.True(t, done)
	var count int
	require.NoError(t, r.db.Get(&count, `SELECT COUNT(*) FROM task_conversation_documents`))
	require.Equal(t, 2, count)
}
