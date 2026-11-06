package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestConversationSearchConsistency(t *testing.T) {
	testConversationConsistency(t, newConversationRepo(t))
}
func TestConversationSearchPostgresConsistency(t *testing.T) {
	testConversationConsistency(t, openPostgresRepo(t))
}

func testConversationConsistency(t *testing.T, r *Repository) {
	t.Helper()
	seedWorkspace(t, r, "search-ws")
	require.NoError(t, r.CreateTask(t.Context(), &models.Task{ID: "search-task", WorkspaceID: "search-ws", Title: "Title"}))
	require.NoError(t, r.CreateTaskSession(t.Context(), &models.TaskSession{ID: "search-session", TaskID: "search-task"}))
	require.NoError(t, r.CreateTurn(t.Context(), &models.Turn{ID: "search-turn", TaskSessionID: "search-session", TaskID: "search-task"}))
	message := &models.Message{ID: "search-message", TaskSessionID: "search-session", TurnID: "search-turn", AuthorType: models.MessageAuthorUser, Content: "original needle"}
	require.NoError(t, r.CreateMessage(t.Context(), message))
	old, err := r.readConversationDocument(t.Context(), message.ID)
	require.NoError(t, err)
	_, err = r.db.Exec(`UPDATE task_session_messages SET content='replacement needle' WHERE id='search-message'`)
	require.NoError(t, err)
	require.NoError(t, r.publishConversationDocument(t.Context(), old))
	var count int
	require.NoError(t, r.db.Get(&count, `SELECT COUNT(*) FROM task_conversation_documents`))
	require.Zero(t, count, "obsolete token must not publish")
	_, err = r.ProcessConversationSearchBatch(t.Context(), false)
	require.NoError(t, err)
	require.NoError(t, r.publishConversationDocument(t.Context(), old))
	page, err := r.SearchTaskConversations(t.Context(), "search-ws", "replacement", 20)
	require.NoError(t, err)
	require.Len(t, page.Hits, 1)
	tx, err := r.db.Begin()
	require.NoError(t, err)
	_, err = tx.Exec(`UPDATE task_session_messages SET content='rolled back needle' WHERE id='search-message'`)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
	page, err = r.SearchTaskConversations(t.Context(), "search-ws", "replacement", 20)
	require.NoError(t, err)
	require.Len(t, page.Hits, 1)
	_, err = r.db.Exec(`UPDATE task_session_messages SET type='tool_call' WHERE id='search-message'`)
	require.NoError(t, err)
	_, err = r.ProcessConversationSearchBatch(t.Context(), false)
	require.NoError(t, err)
	page, err = r.SearchTaskConversations(t.Context(), "search-ws", "replacement", 20)
	require.NoError(t, err)
	require.Empty(t, page.Hits)
	// Restart the historical cursor over existing messages and replay schema setup.
	_, err = r.db.Exec(`UPDATE task_session_messages SET type='message' WHERE id='search-message'`)
	require.NoError(t, err)
	_, err = r.db.Exec(`DELETE FROM task_conversation_dirty`)
	require.NoError(t, err)
	_, err = r.db.Exec(`UPDATE task_conversation_index_state SET cursor_id='',upper_id='zzzz',complete=0`)
	require.NoError(t, err)
	complete, err := r.BackfillConversationSearch(t.Context(), 1)
	require.NoError(t, err)
	require.False(t, complete)
	require.NoError(t, r.createConversationSearchSchema(t.Context()))
	complete, err = r.BackfillConversationSearch(t.Context(), 1)
	require.NoError(t, err)
	require.True(t, complete)
	_, err = r.ProcessConversationSearchBatch(t.Context(), true)
	require.NoError(t, err)
	page, err = r.SearchTaskConversations(t.Context(), "search-ws", "replacement", 20)
	require.NoError(t, err)
	require.Len(t, page.Hits, 1)
	seedWorkspace(t, r, "moved-ws")
	_, err = r.db.Exec(`UPDATE tasks SET workspace_id='moved-ws' WHERE id='search-task'`)
	require.NoError(t, err)
	page, err = r.SearchTaskConversations(t.Context(), "search-ws", "replacement", 20)
	require.NoError(t, err)
	require.Empty(t, page.Hits)
	page, err = r.SearchTaskConversations(t.Context(), "moved-ws", "replacement", 20)
	require.NoError(t, err)
	require.Len(t, page.Hits, 1)
	_, err = r.db.Exec(`DELETE FROM task_sessions WHERE id='search-session'`)
	require.NoError(t, err)
	require.NoError(t, r.db.Get(&count, `SELECT COUNT(*) FROM task_conversation_documents`))
	require.Zero(t, count)
}

func TestConversationSearchQueryValidation(t *testing.T) {
	for _, tc := range []struct{ query, state string }{{" ab ", "query_too_short"}, {"a_b_c", "query_too_broad"}, {strings.Repeat("a", 201), "query_too_long"}, {"abc\x00", "query_too_broad"}, {" 中文测 ", "ready"}} {
		_, state := conversationQuery(tc.query)
		require.Equal(t, tc.state, state, tc.query)
	}
}

func TestConversationSearchDeleteAfterEligibilityChange(t *testing.T) {
	r := newConversationRepo(t)
	seedForMsgTest(t, r, "delete-task", "delete-session", "delete-turn")
	require.NoError(t, r.CreateMessage(t.Context(), &models.Message{ID: "delete-message", TaskSessionID: "delete-session", TurnID: "delete-turn", Content: "private text", AuthorType: models.MessageAuthorAgent}))
	_, err := r.ProcessConversationSearchBatch(t.Context(), false)
	require.NoError(t, err)
	_, err = r.db.Exec(`UPDATE task_session_messages SET type='tool_call' WHERE id='delete-message'`)
	require.NoError(t, err)
	require.NoError(t, r.DeleteMessage(t.Context(), "delete-message"))
	var count int
	require.NoError(t, r.db.Get(&count, `SELECT COUNT(*) FROM task_conversation_documents`))
	require.Zero(t, count)
}

func TestConversationSearchCancellation(t *testing.T) {
	r := newConversationRepo(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := r.SearchTaskConversations(ctx, "ws", "needle", 20)
	// Capability checks may classify cancellation as unavailable; neither path holds a connection.
	if err != nil {
		require.ErrorIs(t, err, context.Canceled)
	}
	var value int
	require.NoError(t, r.ro.Get(&value, `SELECT 1`))
	require.Equal(t, 1, value)
}

func TestConversationSearchIndexKey(t *testing.T) {
	require.Equal(t, "common phrase 100%", conversationIndexKey("common phrase 100%"))
	require.Equal(t, "cafuc3ua9", conversationIndexKey("café"))
	require.Equal(t, "ue4ub8uadue6u96u87ue6ub5u8bue8uafu95", conversationIndexKey("中文测试"))
}
