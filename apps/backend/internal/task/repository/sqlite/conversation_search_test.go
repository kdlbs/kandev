package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-CONVERSATION-SEARCH-002.3
func TestConversationSearchTracksCanonicalWrites(t *testing.T) {
	r := newConversationRepo(t)
	require.NoError(t, r.createConversationSearchSchema(t.Context()))
	seedForMsgTest(t, r, "search-task", "search-session", "search-turn")
	require.NoError(t, r.CreateMessage(t.Context(), &models.Message{ID: "search-message", TaskSessionID: "search-session", TurnID: "search-turn", Content: "retry budget", AuthorType: models.MessageAuthorAgent}))
	var token string
	require.NoError(t, r.db.Get(&token, `SELECT token FROM task_conversation_dirty WHERE message_id='search-message'`))
	require.NotEmpty(t, token)
	_, err := r.db.Exec(`UPDATE task_session_messages SET content='new budget' WHERE id='search-message'`)
	require.NoError(t, err)
	var next string
	require.NoError(t, r.db.Get(&next, `SELECT token FROM task_conversation_dirty WHERE message_id='search-message'`))
	require.NotEqual(t, token, next)
}

type conversationRepository interface {
	ProcessConversationSearchBatch(context.Context, bool) (int, error)
	BackfillConversationSearch(context.Context, int) (bool, error)
	SearchTaskConversations(context.Context, string, string, int) (*models.ConversationSearchPage, error)
}

// @covers AC-TASKS-CONVERSATION-SEARCH-001.1, AC-TASKS-CONVERSATION-SEARCH-002.1
func TestConversationSearchMatching(t *testing.T) {
	testConversationSearchMatching(t, newConversationRepo(t))
}

func testConversationSearchMatching(t *testing.T, r *Repository) {
	t.Helper()
	search, ok := any(r).(conversationRepository)
	require.True(t, ok, "repository must implement indexed conversation discovery")
	seedWorkspace(t, r, "conversation-ws")
	require.NoError(t, r.CreateTask(t.Context(), &models.Task{ID: "conversation-task", WorkspaceID: "conversation-ws", Title: "Unknown title"}))
	require.NoError(t, r.CreateTaskSession(t.Context(), &models.TaskSession{ID: "conversation-session", TaskID: "conversation-task"}))
	require.NoError(t, r.CreateTurn(t.Context(), &models.Turn{ID: "conversation-turn", TaskSessionID: "conversation-session", TaskID: "conversation-task"}))
	message := &models.Message{ID: "conversation-message", TaskSessionID: "conversation-session", TurnID: "conversation-turn", AuthorType: models.MessageAuthorAgent, Content: `Retry BUDGET 100% under_score path\file "quoted" café 中文测试`}
	require.NoError(t, r.CreateMessage(t.Context(), message))
	_, err := search.ProcessConversationSearchBatch(t.Context(), false)
	require.NoError(t, err)
	for _, query := range []string{"retry budget", "100%", "under_score", `path\file`, `"quoted"`, "CAFÉ", "中文测试"} {
		page, err := search.SearchTaskConversations(t.Context(), "conversation-ws", query, 20)
		require.NoError(t, err, query)
		require.Len(t, page.Hits, 1, query)
		require.Equal(t, "conversation-task", page.Hits[0].TaskID)
		require.Equal(t, "conversation-session", page.Hits[0].Match.SessionID)
		require.NotEmpty(t, page.Hits[0].Match.HighlightRanges)
	}
	for _, query := range []string{"cafe", "unknown", "underXscore"} {
		page, err := search.SearchTaskConversations(t.Context(), "conversation-ws", query, 20)
		require.NoError(t, err)
		require.Empty(t, page.Hits, query)
	}
	page, err := search.SearchTaskConversations(t.Context(), "elsewhere", "budget", 20)
	require.NoError(t, err)
	require.Empty(t, page.Hits)
	_, err = r.db.Exec(`UPDATE task_session_messages SET content='replacement text' WHERE id='conversation-message'`)
	require.NoError(t, err)
	page, err = search.SearchTaskConversations(t.Context(), "conversation-ws", "budget", 20)
	require.NoError(t, err)
	require.Empty(t, page.Hits, "dirty text must disappear before indexing")
	_, err = search.ProcessConversationSearchBatch(t.Context(), false)
	require.NoError(t, err)
	page, err = search.SearchTaskConversations(t.Context(), "conversation-ws", "replacement", 20)
	require.NoError(t, err)
	require.Len(t, page.Hits, 1)
	require.NoError(t, r.DeleteMessage(t.Context(), message.ID))
	var count int
	require.NoError(t, r.db.Get(&count, `SELECT COUNT(*) FROM task_conversation_documents`))
	require.Zero(t, count, "delete must remove copied text synchronously")
	page, err = search.SearchTaskConversations(t.Context(), "conversation-ws", "replacement", 20)
	require.NoError(t, err)
	require.Empty(t, page.Hits)
}

func TestConversationSearchPostgresMatching(t *testing.T) {
	testConversationSearchMatching(t, openPostgresRepo(t))
}

func newConversationRepo(t *testing.T) *Repository {
	t.Helper()
	r := newRepoForSessionTests(t)
	if !r.ConversationSearchReady(t.Context()) {
		t.Skip("SQLite conversation search requires the fts5 build tag")
	}
	return r
}

func TestConversationSearchUnavailable(t *testing.T) {
	r := newRepoForSessionTests(t)
	if r.ConversationSearchReady(t.Context()) {
		t.Skip("run without fts5 to verify missing capability")
	}
	page, err := r.SearchTaskConversations(t.Context(), "workspace", "needle", 20)
	require.NoError(t, err)
	require.Equal(t, models.ConversationSearchUnavailable, page.State)
	require.Empty(t, page.Hits)
}
