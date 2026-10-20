package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestConversationSearchPostgresCommitOrders(t *testing.T) {
	r := openPostgresRepo(t)
	r.db.SetMaxOpenConns(3)
	r.db.SetMaxIdleConns(3)
	seedPostgresSession(t, r, "race-task", "race-session", "race-turn", time.Now())
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	require.NoError(t, r.CreateMessage(ctx, &models.Message{ID: "race-message", TaskSessionID: "race-session", TurnID: "race-turn", AuthorType: models.MessageAuthorAgent, Content: "original text"}))
	old, err := r.readConversationDocument(ctx, "race-message")
	require.NoError(t, err)
	source, err := r.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = source.Rollback() }()
	_, err = source.ExecContext(ctx, `UPDATE task_session_messages SET content='source first' WHERE id='race-message'`)
	require.NoError(t, err)
	published := make(chan error, 1)
	go func() { published <- r.publishConversationDocument(ctx, old) }()
	require.NoError(t, <-published, "locked dirty rows must remain queued without blocking the worker")
	require.NoError(t, source.Commit())
	var count int
	require.NoError(t, r.db.Get(&count, `SELECT COUNT(*) FROM task_conversation_documents`))
	require.Zero(t, count)
	current, err := r.readConversationDocument(ctx, "race-message")
	require.NoError(t, err)
	worker, err := r.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = worker.Rollback() }()
	require.NoError(t, r.publishConversationTx(ctx, worker, current, current.body.String, conversationIndexKey(current.body.String)))
	updated := make(chan error, 1)
	go func() {
		_, err := r.db.ExecContext(ctx, `UPDATE task_session_messages SET content='source last' WHERE id='race-message'`)
		updated <- err
	}()
	select {
	case err := <-updated:
		t.Fatalf("writer passed the worker lock: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	require.NoError(t, worker.Commit())
	require.NoError(t, <-updated)
	require.NoError(t, r.db.Get(&count, `SELECT COUNT(*) FROM task_conversation_dirty WHERE message_id='race-message'`))
	require.Equal(t, 1, count)
	_, err = r.ProcessConversationSearchBatch(ctx, false)
	require.NoError(t, err)
	var body string
	require.NoError(t, r.db.Get(&body, `SELECT body FROM task_conversation_documents WHERE message_id='race-message'`))
	require.Equal(t, "source last", body)
}

func TestConversationSearchPostgresBatchDeletion(t *testing.T) {
	for _, ineligible := range []bool{false, true} {
		t.Run(fmt.Sprintf("ineligible=%t", ineligible), func(t *testing.T) {
			r := openPostgresRepo(t)
			r.db.SetMaxOpenConns(3)
			r.db.SetMaxIdleConns(3)
			seedPostgresSession(t, r, "batch-task", "batch-session", "batch-turn", time.Now())
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			for _, id := range []string{"batch-a", "batch-b"} {
				require.NoError(t, r.CreateMessage(ctx, &models.Message{ID: id, TaskSessionID: "batch-session", TurnID: "batch-turn", AuthorType: models.MessageAuthorAgent, Content: "old searchable"}))
			}
			_, err := r.ProcessConversationSearchBatch(ctx, false)
			require.NoError(t, err)
			update := `UPDATE task_session_messages SET content='new searchable'`
			if ineligible {
				update = `UPDATE task_session_messages SET type='tool_call'`
			}
			_, err = r.db.ExecContext(ctx, update)
			require.NoError(t, err)
			first, err := r.readConversationDocument(ctx, "batch-a")
			require.NoError(t, err)
			second, err := r.readConversationDocument(ctx, "batch-b")
			require.NoError(t, err)
			source, err := r.db.BeginTxx(ctx, nil)
			require.NoError(t, err)
			defer func() { _ = source.Rollback() }()
			_, err = source.ExecContext(ctx, `DELETE FROM task_session_messages WHERE id='batch-a'`)
			require.NoError(t, err)
			worker, err := r.db.BeginTxx(ctx, nil)
			require.NoError(t, err)
			defer func() { _ = worker.Rollback() }()
			require.NoError(t, r.publishConversationTx(ctx, worker, second, second.body.String, conversationIndexKey(second.body.String)))
			deleted := make(chan error, 1)
			go func() {
				_, err := source.ExecContext(ctx, `DELETE FROM task_sessions WHERE id='batch-session'`)
				if err == nil {
					err = source.Commit()
				}
				deleted <- err
			}()
			require.NoError(t, r.publishConversationTx(ctx, worker, first, first.body.String, conversationIndexKey(first.body.String)))
			require.NoError(t, worker.Commit())
			require.NoError(t, <-deleted)
			var count int
			require.NoError(t, r.db.Get(&count, `SELECT COUNT(*) FROM task_conversation_documents`))
			require.Zero(t, count, "session deletion must remove every projected message")
		})
	}
}
