package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	analyticssqlite "github.com/kandev/kandev/internal/analytics/repository/sqlite"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

const (
	turnReaderTaskID    = "turn-reader-task"
	turnReaderSessionID = "turn-reader-session"
)

func TestTaskRepoTurnReadersUsageBinding(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(fmt.Sprintf("postgres=%t", postgres), func(t *testing.T) {
			repo, turns := turnReaderBindingFixture(t, postgres)
			t.Run("existing_readers", func(t *testing.T) {
				for _, turn := range turns {
					got, err := repo.GetTurn(t.Context(), turn.ID)
					require.NoError(t, err)
					assertTurnReaderBinding(t, got, turn)
				}
				listed, err := repo.ListTurnsBySession(t.Context(), turnReaderSessionID)
				require.NoError(t, err)
				require.Len(t, listed, 3, "an unattempted reservation stays out of history")
				for i, turn := range listed {
					assertTurnReaderBinding(t, turn, turns[i])
				}
				latest, err := repo.GetLatestTurnBySessionID(t.Context(), turnReaderSessionID)
				require.NoError(t, err)
				assertTurnReaderBinding(t, latest, turns[2])
				active, err := repo.GetActiveTurnBySessionID(t.Context(), turnReaderSessionID)
				require.NoError(t, err)
				assertTurnReaderBinding(t, active, turns[2])
			})
			t.Run("conversation_pages", func(t *testing.T) {
				assertTurnReaderPages(t, repo, turns)
			})
			t.Run("message_window", func(t *testing.T) {
				assertTurnReaderWindow(t, repo, turns)
			})
			t.Run("recovery_outbox", func(t *testing.T) {
				assertTurnReaderRecovery(t, repo, turns)
			})
		})
	}
}

func turnReaderBindingFixture(t *testing.T, postgres bool) (*Repository, []*models.Turn) {
	t.Helper()
	var repo *Repository
	if postgres {
		conn := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
		var version string
		require.NoError(t, conn.Get(&version, `SHOW server_version`))
		t.Logf("PostgreSQL %s", version)
		var err error
		repo, err = NewWithDB(conn, conn, nil)
		require.NoError(t, err)
	} else {
		repo = newRepoForSessionTests(t)
	}
	retainer, err := analyticssqlite.NewWithDB(repo.db, repo.ro)
	require.NoError(t, err)
	repo.SetUsageHistoryRetainer(retainer)
	at := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	require.NoError(t, repo.CreateWorkspace(t.Context(), &models.Workspace{ID: "turn-reader-ws", Name: "Readers", OrgID: "turn-reader-org"}))
	require.NoError(t, repo.CreateTask(t.Context(), &models.Task{ID: turnReaderTaskID, WorkspaceID: "turn-reader-ws", Title: "Readers", CreatedAt: at.Add(-time.Hour), UpdatedAt: at.Add(-time.Hour)}))
	require.NoError(t, repo.CreateTaskSession(t.Context(), &models.TaskSession{ID: turnReaderSessionID, TaskID: turnReaderTaskID, StartedAt: at.Add(-time.Minute)}))
	actor := authn.WithIdentity(t.Context(), authn.Identity{UserID: "turn-reader-user", Role: authn.RoleMember})
	var turns []*models.Turn
	for i := range 4 {
		started := at.Add(time.Duration(i) * time.Minute)
		turn := &models.Turn{
			ID: fmt.Sprintf("turn-reader-%d", i+1), TaskID: turnReaderTaskID, TaskSessionID: turnReaderSessionID,
			RouteGeneration: int64(i + 1), StartedAt: started, CreatedAt: started.Add(time.Second), UpdatedAt: started.Add(2 * time.Second),
			Metadata: map[string]interface{}{"reader_row": fmt.Sprintf("row-%d", i+1)},
		}
		if i < 2 {
			completed := started.Add(10 * time.Second)
			turn.CompletedAt = &completed
		}
		if i == 3 {
			turn.Metadata[models.TurnMetaKeyPromptDispatchPending] = true
		}
		if i == 0 {
			receipt, err := repo.CreateTurnWithConversationReceipt(actor, turn)
			require.NoError(t, err)
			require.Len(t, receipt.Operations, 1)
			assertTurnReaderBinding(t, receipt.Operations[0].Turn, turn)
		} else {
			require.NoError(t, repo.CreateTurn(actor, turn))
		}
		require.NotEmpty(t, turn.UsageBindingID, "the real retainer must capture the binding")
		var capturedActor string
		require.NoError(t, repo.db.Get(&capturedActor, repo.db.Rebind(`SELECT actor_id FROM usage_execution_bindings WHERE binding_id = ?`), turn.UsageBindingID))
		require.Equal(t, "turn-reader-user", capturedActor)
		turns = append(turns, turn)
	}
	for i := range 2 {
		require.NoError(t, repo.CreateMessage(t.Context(), &models.Message{
			ID: fmt.Sprintf("turn-reader-message-%d", i+1), TaskID: turnReaderTaskID, TaskSessionID: turnReaderSessionID,
			TurnID: turns[i].ID, AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeMessage,
			Content: "reader context", CreatedAt: turns[i].StartedAt.Add(5 * time.Second),
		}))
	}
	restarted, err := NewWithDB(repo.db, repo.ro, nil)
	require.NoError(t, err)
	restarted.SetUsageHistoryRetainer(retainer)
	return restarted, turns
}

func assertTurnReaderBinding(t *testing.T, got, want *models.Turn) {
	t.Helper()
	require.NotNil(t, got)
	require.NotEmpty(t, want.UsageBindingID)
	require.Equal(t, want.UsageBindingID, got.UsageBindingID)
	require.Equal(t, want.ID, got.ID)
	require.Equal(t, want.TaskID, got.TaskID)
	require.Equal(t, want.TaskSessionID, got.TaskSessionID)
	require.Equal(t, want.RouteGeneration, got.RouteGeneration)
	require.True(t, want.StartedAt.Equal(got.StartedAt))
	require.True(t, want.CreatedAt.Equal(got.CreatedAt))
	require.Equal(t, want.CompletedAt, got.CompletedAt)
	require.Equal(t, want.Metadata["reader_row"], got.Metadata["reader_row"])
	encoded, err := json.Marshal(got)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), want.UsageBindingID)
	require.NotContains(t, string(encoded), "usage_binding_id")
}

func assertTurnReaderPages(t *testing.T, repo *Repository, turns []*models.Turn) {
	t.Helper()
	for _, sort := range []string{"asc", "desc"} {
		cursor := ""
		var revision int64
		for i := range 3 {
			page, err := repo.ReadConversationTurnsPage(t.Context(), models.ConversationTurnPageRequest{SessionID: turnReaderSessionID, Sort: sort, CursorID: cursor, Limit: 1})
			require.NoError(t, err)
			require.Len(t, page.Turns, 1)
			index := i
			if sort == "desc" {
				index = 2 - i
			}
			assertTurnReaderBinding(t, page.Turns[0], turns[index])
			require.Equal(t, i < 2, page.HasMore)
			if i == 0 {
				revision = page.Revision
			}
			require.Equal(t, revision, page.Revision)
			cursor = page.CursorID
		}
	}
	wrongTask := "another-task"
	page, err := repo.ReadConversationTurnsPage(t.Context(), models.ConversationTurnPageRequest{SessionID: turnReaderSessionID, TaskID: &wrongTask, Limit: 1})
	require.NoError(t, err)
	require.Empty(t, page.Turns)
}

func assertTurnReaderWindow(t *testing.T, repo *Repository, turns []*models.Turn) {
	t.Helper()
	for _, test := range []struct {
		options models.ListMessagesOptions
		index   int
		more    bool
	}{
		{models.ListMessagesOptions{Sort: "desc", Limit: 1}, 1, true},
		{models.ListMessagesOptions{Sort: "desc", Before: "turn-reader-message-2", Limit: 1}, 0, false},
		{models.ListMessagesOptions{Sort: "asc", After: "turn-reader-message-1", Limit: 1}, 1, false},
	} {
		window, err := repo.ReadMessageTurnWindow(t.Context(), turnReaderSessionID, test.options)
		require.NoError(t, err)
		require.Len(t, window.Messages, 1)
		require.Equal(t, fmt.Sprintf("turn-reader-message-%d", test.index+1), window.Messages[0].ID)
		require.Equal(t, test.more, window.HasMore)
		require.Len(t, window.Turns, 2, "only the referenced turn and authoritative active turn belong to the window")
		assertTurnReaderBinding(t, window.Turns[0], turns[test.index])
		assertTurnReaderBinding(t, window.Turns[1], turns[2])
		require.Equal(t, turns[2].ID, window.Coverage.ActiveTurnID)
	}
}

func assertTurnReaderRecovery(t *testing.T, repo *Repository, turns []*models.Turn) {
	t.Helper()
	at := turns[3].StartedAt.Add(time.Minute)
	turn := &models.Turn{
		ID: "turn-reader-recovered", TaskID: turnReaderTaskID, TaskSessionID: turnReaderSessionID, RouteGeneration: 5,
		StartedAt: at, CreatedAt: at.Add(time.Second), UpdatedAt: at.Add(2 * time.Second),
		Metadata: map[string]interface{}{"reader_row": "recovered", models.TurnMetaKeyPromptDispatchPending: true, models.TurnMetaKeyPromptDispatchAttempted: true},
	}
	actor := authn.WithIdentity(t.Context(), authn.Identity{UserID: "turn-reader-user", Role: authn.RoleMember})
	require.NoError(t, repo.CreateTurn(actor, turn))
	require.NoError(t, repo.CreateMessage(t.Context(), &models.Message{
		ID: "turn-reader-recovery-output", TaskID: turnReaderTaskID, TaskSessionID: turnReaderSessionID,
		TurnID: turn.ID, AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeMessage, Content: "accepted response", CreatedAt: at.Add(3 * time.Second),
	}))
	reconciled, err := repo.ReconcileUnpublishedPromptTurns(t.Context())
	require.NoError(t, err)
	require.Equal(t, 2, reconciled, "discard the empty unattempted reservation and publish the accepted turn")
	_, err = repo.GetTurn(t.Context(), turns[3].ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
	pending, err := repo.ListTurnsPendingStartEvent(t.Context())
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assertTurnReaderBinding(t, pending[0], turn)
	require.NotContains(t, pending[0].Metadata, models.TurnMetaKeyPromptDispatchPending)
	require.Equal(t, true, pending[0].Metadata[models.TurnMetaKeyPromptDispatchStartEventPending])
	changed, _, _, err := repo.ClearTurnPromptDispatchMetadata(t.Context(), turnReaderSessionID, turn.ID)
	require.NoError(t, err)
	require.True(t, changed)
	pending, err = repo.ListTurnsPendingStartEvent(t.Context())
	require.NoError(t, err)
	require.Empty(t, pending)
	stored, err := repo.GetTurn(t.Context(), turn.ID)
	require.NoError(t, err)
	assertTurnReaderBinding(t, stored, turn)
}
